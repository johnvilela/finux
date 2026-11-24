package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/colorprofile"

	"finux/src/setting"
	"finux/src/telegram"
)

const (
	// botChunk is the size limit of one message of the bot in characters,
	// before the HTML marks. It leaves room below the limit of Telegram.
	botChunk = 3500
	// botMemory is how long the bot keeps a question without an answer.
	botMemory = 10 * time.Minute
	// botRetry is the wait after a failed request for updates.
	botRetry = 5 * time.Second

	botWelcome = "You have access now. Send /help for the commands."
	botSorry   = "Sorry, I did not understand. Send /help for the commands."
)

// botCommands are the commands of the command line that the bot runs,
// by the name of the Telegram command. Telegram permits no dash in a
// command, so credit_card is credit-card.
var botCommands = map[string]string{
	"accounts": "accounts", "ac": "accounts",
	"categories": "categories", "ct": "categories",
	"transactions": "transactions", "tx": "transactions",
	"credit_card": "credit-card", "credit_cards": "credit-card", "cc": "credit-card",
	"goals": "goals", "goal": "goals", "gl": "goals",
	"budgets": "budgets", "budget": "budgets", "bg": "budgets",
	"scheduled": "scheduled", "schedule": "scheduled", "sc": "scheduled",
	"notes": "notes", "note": "notes", "nt": "notes",
	"resume": "resume", "rs": "resume",
	"version": "version",
}

// botRefused are the commands that only the terminal has.
var botRefused = []string{"config", "init", "update", "daemon", "seed", "reseed"}

// botMenu is the menu of commands that Telegram shows.
var botMenu = []telegram.Command{
	{Name: "rs", Description: "Resume of the month"},
	{Name: "tx", Description: "Transactions"},
	{Name: "ac", Description: "Accounts"},
	{Name: "cc", Description: "Credit cards and bills"},
	{Name: "sc", Description: "Scheduled transactions"},
	{Name: "bg", Description: "Budgets"},
	{Name: "gl", Description: "Goals"},
	{Name: "nt", Description: "Notes"},
	{Name: "ct", Description: "Categories"},
	{Name: "help", Description: "Show the commands"},
	{Name: "cancel", Description: "Stop the current question"},
}

// botHelp returns the help of the bot. ai adds the command of the AI.
func botHelp(ai bool) string {
	help := botCommandsHelp
	if ai {
		help += `

<b>AI</b>
/ai with a question asks the AI, which sees your data:
<code>/ai how was my month?</code>
<code>/ai should I buy a new phone now or later?</code>
Send a file with /ai in its caption, for example a bill: <code>/ai import this bill</code>
<code>/ai new</code> starts a new conversation.`
	}
	return help
}

const botCommandsHelp = `<b>Finux</b>

/rs  Resume of the month
/tx  Transactions
/ac  Accounts
/cc  Credit cards and bills
/sc  Scheduled transactions
/bg  Budgets
/gl  Goals
/nt  Notes
/ct  Categories
/cancel  Stop the current question

The commands are the same as in the terminal, without the word finux:

<code>/tx add --title Lunch --value 45 --account NU --category FOOD</code>
<code>/tx --date 11/2026</code>
<code>/cc bills VISA</code>

Send a command with <code>--help</code> for its options, for example <code>/tx --help</code>.
Send <code>/tx add</code> with no options and I ask for each value.`

// question is a command that waits for the press of a button.
type question struct {
	user int64
	args []string
	text string
	// choices has the items of a question that asks which item.
	choices []choice
	expires time.Time
}

// bot answers the messages of the Telegram bot.
type bot struct {
	client *telegram.Client
	// lock is held while the bot uses the database: the jobs of the
	// daemon use the same database.
	lock      *sync.Mutex
	log       func(format string, values ...any)
	questions map[string]question
	// wizards has the guide in progress of each chat.
	wizards map[int64]*wizard
	serial  int
	// kept has the files of each person that wait for an /ai message.
	kept map[int64][]string
	// runs has the answer of the AI in progress of each person. The
	// answers run apart from the bot, so aiLock guards the map.
	runs    map[int64]*aiRun
	aiLock  sync.Mutex
	running sync.WaitGroup
}

func newBot(token string, lock *sync.Mutex, log func(string, ...any)) *bot {
	return &bot{
		client: telegram.NewClient(token), lock: lock, log: log,
		questions: map[string]question{}, wizards: map[int64]*wizard{},
		kept: map[int64][]string{}, runs: map[int64]*aiRun{},
	}
}

// poll gets the updates of the bot and answers them, until ctx ends.
func (b *bot) poll(ctx context.Context) {
	// The answers of the AI end before the bot ends.
	defer b.running.Wait()
	menu := botMenu
	if b.agentKey() != setting.None {
		menu = append([]telegram.Command{{Name: "ai", Description: "Ask the AI about your finances"}}, botMenu...)
	}
	if err := b.client.SetCommands(ctx, menu); err != nil && ctx.Err() == nil {
		b.log("Telegram: set the menu of commands: %v", err)
	}
	var offset int64
	last := ""
	for ctx.Err() == nil {
		updates, err := b.client.GetUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// The same error again and again is one line in the log.
			if err.Error() != last {
				last = err.Error()
				b.log("Telegram: get updates: %v", err)
			}
			select {
			case <-ctx.Done():
			case <-time.After(botRetry):
			}
			continue
		}
		last = ""
		for _, update := range updates {
			offset = update.ID + 1
			switch {
			case update.Message != nil:
				b.message(ctx, update.Message)
			case update.Callback != nil:
				b.callback(ctx, update.Callback)
			}
		}
	}
}

// allowed reports whether the person can use the bot. It reads the
// database each time, so a change of the list needs no restart.
func (b *bot) allowed(user int64) bool {
	b.lock.Lock()
	defer b.lock.Unlock()
	db, store, err := openTelegram()
	if err != nil {
		b.log("Telegram: read the allowed people: %v", err)
		return false
	}
	defer closeDatabase(db)
	return store.Allowed(user)
}

// stranger answers a person who is not allowed. The link of an invite
// lets the person in. Without it, the person gets a code for the owner
// to type in the terminal: the bot tells it one time while it works.
func (b *bot) stranger(ctx context.Context, message *telegram.Message) {
	user, chat := message.From.ID, message.Chat.ID
	command, code, _ := strings.Cut(strings.TrimSpace(message.Text), " ")
	var (
		joined telegram.User
		fresh  bool
		err    error
	)
	func() {
		b.lock.Lock()
		defer b.lock.Unlock()
		db, store, failure := openTelegram()
		if err = failure; err != nil {
			return
		}
		defer closeDatabase(db)
		// Telegram sends '/start <code>' when a person opens the link.
		if command == "/start" && code != "" {
			if joined, err = store.Accept(code, user, message.From.FirstName, time.Now()); err == nil || !errors.Is(err, telegram.ErrNoCode) {
				return
			}
		}
		code, fresh, err = store.Request(user, message.From.FirstName, time.Now())
	}()
	switch {
	case err != nil:
		b.log("Telegram: a message from %d, who is not allowed: %v", user, err)
	case joined.ID != 0:
		b.log("Telegram: %d got access with an invite", user)
		welcome := "You have access now"
		if joined.Name != "" {
			welcome += ", " + html.EscapeString(joined.Name)
		}
		b.send(ctx, chat, welcome+". Send /help for the commands.")
	case fresh:
		b.log("Telegram: a message from %d, who is not allowed", user)
		code = code[:3] + "-" + code[3:]
		b.send(ctx, chat, fmt.Sprintf("You have no access to this bot. Your Telegram user ID is %d.\n\nThe owner can give you access in the terminal with this code, in the next %d minutes:\n<code>finux config telegram pair %s</code>",
			user, int(telegram.PairingTime.Minutes()), code))
	}
}

// send sends a text in the HTML of Telegram.
func (b *bot) send(ctx context.Context, chat int64, text string, keyboard ...[]telegram.Button) {
	if err := b.client.SendMessage(ctx, chat, text, keyboard); err != nil && !errors.Is(err, context.Canceled) {
		b.log("Telegram: send a message to %d: %v", chat, err)
	}
}

// reply sends the output of a command, as text with a fixed width so
// the columns of a table stay in line.
func (b *bot) reply(ctx context.Context, chat int64, output string) {
	parts := chunks(output, botChunk)
	if len(parts) == 0 {
		parts = []string{"Done."}
	}
	for _, part := range parts {
		b.send(ctx, chat, "<pre>"+html.EscapeString(part)+"</pre>")
	}
}

// execute runs a command of the command line and returns its output,
// without colors, and its exit status.
func (b *bot) execute(args []string) (string, int) {
	b.lock.Lock()
	defer b.lock.Unlock()
	var output bytes.Buffer
	plain := &colorprofile.Writer{Forward: &output, Profile: colorprofile.NoTTY}
	status := run(args, streams{stdout: plain, stderr: plain, plain: true})
	// The command line tells about a terminal. A chat has none.
	return strings.ReplaceAll(output.String(), " when input is not a terminal", ""), status
}

func (b *bot) message(ctx context.Context, message *telegram.Message) {
	if message.From == nil || message.Chat.Type != "private" {
		return
	}
	user, chat := message.From.ID, message.Chat.ID
	if !b.allowed(user) {
		b.stranger(ctx, message)
		return
	}
	text := strings.TrimSpace(message.Text)
	if file, has := message.File(); has {
		b.receive(ctx, message, file, strings.TrimSpace(message.Caption))
		return
	}
	current := b.wizards[chat]
	if current != nil && time.Now().After(current.expires) {
		delete(b.wizards, chat)
		current = nil
	}
	if !strings.HasPrefix(text, "/") {
		// A text with no command is the answer to a question, or nothing.
		if current != nil && text != "" {
			b.answer(ctx, chat, current, text)
			return
		}
		b.send(ctx, chat, botSorry)
		return
	}
	// A command stops the questions of the command before it.
	delete(b.wizards, chat)
	args, err := splitArgs(text)
	if err != nil {
		b.send(ctx, chat, "A quote in the message has no end.")
		return
	}
	// In a chat with more bots, Telegram adds the name: /tx@finux_bot.
	name, _, _ := strings.Cut(strings.ToLower(strings.TrimPrefix(args[0], "/")), "@")
	switch {
	case name == "help" || name == "start":
		b.send(ctx, chat, botHelp(b.agentKey() != setting.None))
	case name == "ai":
		// The AI gets the text as the person wrote it.
		_, rest, _ := strings.Cut(text, " ")
		b.ai(ctx, user, chat, rest)
	case name == "cancel" && b.stopAI(user):
		b.send(ctx, chat, "Stopped.")
	case name == "cancel" && current != nil:
		b.send(ctx, chat, "Cancelled.")
	case name == "cancel":
		b.send(ctx, chat, "There is nothing to cancel.")
	case slices.Contains(botRefused, name):
		b.send(ctx, chat, fmt.Sprintf("The command /%s is not available in Telegram. Use it in the terminal.", name))
	case botCommands[name] != "":
		b.command(ctx, user, chat, append([]string{botCommands[name]}, args[1:]...), text)
	default:
		b.send(ctx, chat, botSorry)
	}
}

// command runs a command and sends its output. A command that deletes
// data asks first, with buttons.
func (b *bot) command(ctx context.Context, user, chat int64, args []string, text string) {
	if found, given, ok := findGuide(args); ok {
		started := &wizard{guide: found, user: user, args: args, text: text, answers: given}
		b.wizards[chat] = started
		b.advance(ctx, chat, started)
		return
	}
	output, status := b.execute(args)
	if status == 2 && missingReference(output) && b.pick(ctx, user, chat, args, text) {
		return
	}
	if status == 2 && strings.Contains(output, "--yes is necessary") {
		// A delete with no item asks the item first, then asks again.
		if !hasReference(args) && b.pick(ctx, user, chat, args, text) {
			return
		}
		key := b.ask(question{user: user, args: append(args, "--yes"), text: text})
		b.send(ctx, chat, "This deletes data and you cannot get it back:\n<code>"+html.EscapeString(text)+"</code>",
			[]telegram.Button{{Text: "Confirm", Data: "yes:" + key}, {Text: "Cancel", Data: "no:" + key}})
		return
	}
	b.reply(ctx, chat, output)
}

// hasReference reports whether a command has an argument after its
// name that is not an option: the code or the id of an item.
func hasReference(args []string) bool {
	for _, arg := range args[min(len(args), 2):] {
		if !strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

// ask keeps a question and returns its key, for the data of the buttons.
func (b *bot) ask(asked question) string {
	now := time.Now()
	for key, old := range b.questions {
		if now.After(old.expires) {
			delete(b.questions, key)
		}
	}
	b.serial++
	key := strconv.Itoa(b.serial)
	asked.expires = now.Add(botMemory)
	b.questions[key] = asked
	return key
}

func (b *bot) callback(ctx context.Context, press *telegram.Callback) {
	if err := b.client.AnswerCallback(ctx, press.ID); err != nil && !errors.Is(err, context.Canceled) {
		b.log("Telegram: answer a button: %v", err)
	}
	if press.Message == nil || press.Message.Chat.Type != "private" || !b.allowed(press.From.ID) {
		return
	}
	answer, key, _ := strings.Cut(press.Data, ":")
	if answer == "w" {
		b.pressed(ctx, press, key)
		return
	}
	key, item, _ := strings.Cut(key, ":")
	asked, known := b.questions[key]
	if !known || asked.user != press.From.ID || time.Now().After(asked.expires) {
		return
	}
	delete(b.questions, key)
	chat, command := press.Message.Chat.ID, "<code>"+html.EscapeString(asked.text)+"</code>"
	edit := func(text string) { b.edit(ctx, chat, press.Message.ID, text) }
	switch answer {
	case "yes":
		edit("Confirmed:\n" + command)
		output, status := b.execute(asked.args)
		if status == 2 && missingReference(output) && b.pick(ctx, asked.user, chat, asked.args, asked.text) {
			return
		}
		b.reply(ctx, chat, output)
	case "p":
		// The item of the button becomes the reference of the command.
		index, err := strconv.Atoi(item)
		if err != nil || index < 0 || index >= len(asked.choices) {
			return
		}
		value := asked.choices[index].value
		text := asked.text + " " + value
		edit("<code>" + html.EscapeString(text) + "</code>")
		b.command(ctx, asked.user, chat, append(asked.args, value), text)
	default:
		edit("Cancelled:\n" + command)
	}
}

// edit replaces the text of a message of the bot and removes its buttons.
func (b *bot) edit(ctx context.Context, chat, message int64, text string) {
	if err := b.client.EditMessage(ctx, chat, message, text, nil); err != nil && !errors.Is(err, context.Canceled) {
		b.log("Telegram: change a message: %v", err)
	}
}
