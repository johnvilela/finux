package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"finux/src/account"
	"finux/src/budget"
	"finux/src/card"
	"finux/src/category"
	"finux/src/goal"
	"finux/src/money"
	"finux/src/note"
	"finux/src/schedule"
	"finux/src/telegram"
	"finux/src/transaction"

	"gorm.io/gorm"
)

const (
	// botButtons is the largest number of items that a question shows.
	botButtons = 30
	// botRecent is the number of transactions that a question shows.
	botRecent = 10
	// botRow is the number of buttons in one row.
	botRow = 3
)

// choice is one button of a question. value is the argument that the
// command line gets.
type choice struct {
	label, value string
}

// step is one question of a guide. Its answer is the value of the
// option flag; a step with no flag gives a positional argument.
type step struct {
	flag, label, ask string
	// choices gives the buttons. A person can also write the answer.
	choices func(db *gorm.DB) ([]choice, error)
	// optional adds the button Skip.
	optional bool
	// when tells if the step is necessary, from the answers before it.
	when func(db *gorm.DB, answers map[string]string) bool
}

// guide is the list of questions that makes the options of a command,
// for a person who sent the command without them.
type guide struct {
	title string
	steps []step
}

func codes(values ...string) func(*gorm.DB) ([]choice, error) {
	return func(*gorm.DB) ([]choice, error) {
		choices := make([]choice, len(values))
		for i, value := range values {
			choices[i] = choice{value, value}
		}
		return choices, nil
	}
}

func accountChoices(db *gorm.DB) ([]choice, error) {
	accounts, err := account.NewStore(db).List(false, today())
	if err != nil {
		return nil, err
	}
	choices := make([]choice, len(accounts))
	for i, current := range accounts {
		choices[i] = choice{current.Code, current.Code}
	}
	return choices, nil
}

func cardChoices(db *gorm.DB) ([]choice, error) {
	cards, err := card.NewStore(db).List(false)
	if err != nil {
		return nil, err
	}
	choices := make([]choice, len(cards))
	for i, current := range cards {
		choices[i] = choice{current.Code, current.Code}
	}
	return choices, nil
}

// placeChoices gives the accounts, then the credit cards.
func placeChoices(db *gorm.DB) ([]choice, error) {
	accounts, err := accountChoices(db)
	if err != nil {
		return nil, err
	}
	cards, err := cardChoices(db)
	return append(accounts, cards...), err
}

func categoryChoices(db *gorm.DB) ([]choice, error) {
	categories, err := category.NewStore(db).List()
	if err != nil {
		return nil, err
	}
	choices := make([]choice, len(categories))
	for i, current := range categories {
		choices[i] = choice{current.Code, current.Code}
	}
	return choices, nil
}

func dateChoices(*gorm.DB) ([]choice, error) {
	now, err := time.Parse("2006-01-02", today())
	if err != nil {
		return nil, err
	}
	return []choice{
		{"Today", transaction.FormatDate(today())},
		{"Yesterday", transaction.FormatDate(now.AddDate(0, 0, -1).Format("2006-01-02"))},
	}, nil
}

// greyCategory reports whether the category of the answers accepts all
// the types. Then the type is a question: the color does not give it.
func greyCategory(db *gorm.DB, answers map[string]string) bool {
	categories, err := category.NewStore(db).List()
	if err != nil {
		return false
	}
	for _, current := range categories {
		if strings.EqualFold(current.Code, answers["--category"]) {
			return current.Color == "grey"
		}
	}
	return false
}

// onCard reports whether the place of the answers is a credit card.
// Then the virtual card and the limit are questions: an account has
// none.
func onCard(db *gorm.DB, answers map[string]string) bool {
	cards, err := card.NewStore(db).List(false)
	if err != nil {
		return false
	}
	for _, current := range cards {
		if strings.EqualFold(current.Code, strings.TrimSpace(answers["--account"])) {
			return true
		}
	}
	return false
}

var (
	stepCurrency = step{flag: "--currency", label: "Currency", ask: "Currency?", choices: codes(money.Codes()...)}
	stepPlace    = step{flag: "--account", label: "Account or credit card", ask: "Account or credit card?", choices: placeChoices}
	stepCategory = step{flag: "--category", label: "Category", ask: "Category?", choices: categoryChoices}
	stepType     = step{
		flag: "--type", label: "Expense or income", ask: "Expense or income?", when: greyCategory,
		choices: func(*gorm.DB) ([]choice, error) {
			return []choice{{"Expense", transaction.Expense}, {"Income", transaction.Income}}, nil
		},
	}
	stepVirtual = step{
		flag: "--virtual", label: "Virtual card", ask: "Paid with a virtual card?", when: onCard,
		choices: func(*gorm.DB) ([]choice, error) {
			return []choice{{"No", "false"}, {"Yes", "true"}}, nil
		},
	}
	stepOffLimit = step{
		flag: "--off-limit", label: "Off limit", ask: "Outside the limit of the card?", when: onCard,
		choices: func(*gorm.DB) ([]choice, error) {
			return []choice{{"No", "false"}, {"Yes", "true"}}, nil
		},
	}
	stepEvery = step{flag: "--every", label: "Period", choices: codes("week", "month", "year")}
)

// every returns the question for the length of a period.
func every(ask string, optional bool) step {
	asked := stepEvery
	asked.ask, asked.optional = ask, optional
	return asked
}

// guides has the questions of each command, by the name of the module
// and the name of the command.
var guides = map[string]guide{
	"transactions add": {"New transaction", []step{
		{flag: "--title", label: "Title", ask: "Title?"},
		{flag: "--value", label: "Value", ask: "Value?"},
		stepPlace, stepVirtual, stepOffLimit, stepCategory, stepType,
		{flag: "--date", label: "Date", ask: "Date? Press a button or send a date as DD/MM/YYYY.", choices: dateChoices},
	}},
	"accounts add": {"New account", []step{
		{flag: "--name", label: "Name", ask: "Name?"},
		stepCurrency,
		{flag: "--balance", label: "Opening balance", ask: "Opening balance? It is optional.", optional: true},
	}},
	"categories add": {"New category", []step{
		{flag: "--name", label: "Name", ask: "Name?"},
		{flag: "--color", label: "Color", ask: "Color? Red is for expenses, green for incomes, grey for all.", choices: codes("red", "green", "grey")},
	}},
	"credit-card add": {"New credit card", []step{
		{flag: "--name", label: "Name", ask: "Name?"},
		stepCurrency,
		{flag: "--limit", label: "Limit", ask: "Credit limit?"},
		{flag: "--closing", label: "Closing day", ask: "Closing day? A day of the month, from 1 to 31."},
		{flag: "--due", label: "Due day", ask: "Due day? A day of the month, from 1 to 31."},
	}},
	"credit-card pay": {"Pay a bill", []step{
		{label: "Credit card", ask: "Which credit card?", choices: cardChoices},
		{flag: "--from", label: "Account", ask: "Which account pays?", choices: accountChoices},
		{flag: "--value", label: "Value", ask: "Value? Skip pays the total of the bill.", optional: true},
	}},
	"goals add": {"New goal", []step{
		{flag: "--title", label: "Title", ask: "Title?"},
		{flag: "--type", label: "Type", ask: "Type? Save and purchase count what you put in. Payoff counts what you pay.", choices: codes(goal.Types()...)},
		{flag: "--target", label: "Target", ask: "Amount to reach?"},
		stepCurrency,
		{flag: "--due", label: "Final date", ask: "Final date, as DD/MM/YYYY? It is optional.", optional: true},
	}},
	"budgets add": {"New budget", []step{
		{flag: "--title", label: "Title", ask: "Title?"},
		{flag: "--limit", label: "Amount", ask: "Amount of one period?"},
		stepCurrency,
		{flag: "--categories", label: "Categories", ask: "Categories? Send the codes with commas, for example FOOD,FUN. Skip counts all of them.", optional: true},
		every("How long is one period? Press a button or send a length, for example 2 weeks.", false),
	}},
	"notes add": {"New note", []step{
		{flag: "--title", label: "Title", ask: "Title?"},
		{flag: "--text", label: "Text", ask: "Text? What the note tells, in a few words. It is optional.", optional: true},
		{flag: "--remind", label: "Reminder", ask: "Reminder date, as DD/MM/YYYY? It is optional.", optional: true},
		{flag: "--tags", label: "Tags", ask: "Tags? It is optional.", optional: true},
	}},
	"scheduled add": {"New schedule", []step{
		{flag: "--title", label: "Title", ask: "Title?"},
		{flag: "--value", label: "Value", ask: "Value?"},
		stepPlace, stepCategory, stepType,
		{flag: "--date", label: "Date", ask: "Date of the first occurrence, as DD/MM/YYYY?"},
		every("How often? Skip makes a schedule for one time.", true),
	}},
}

// wizard is a guide in progress in one chat.
type wizard struct {
	guide
	user int64
	// args and text are the command that started the guide.
	args    []string
	text    string
	index   int
	answers map[string]string
	// key and choices belong to the question that waits for its answer.
	key     string
	choices []choice
	expires time.Time
}

// findGuide returns the guide for a command that came without the
// options it needs. given has the answers that the command has already.
func findGuide(args []string) (found guide, given map[string]string, ok bool) {
	if len(args) < 2 {
		return guide{}, nil, false
	}
	module, command, rest := args[0], args[1], args[2:]
	switch {
	case (command == "add" || command == "a") && len(rest) == 0:
		found, ok = guides[module+" add"]
		return found, map[string]string{}, ok
	case module == "credit-card" && (command == "pay" || command == "p"):
		given = map[string]string{}
		for i, arg := range rest {
			name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if strings.HasPrefix(arg, "-") && name == "from" {
				return guide{}, nil, false
			}
			if i == 0 && !strings.HasPrefix(arg, "-") {
				// The credit card is in the command: no question for it.
				given[""] = ""
			}
		}
		return guides["credit-card pay"], given, true
	}
	return guide{}, nil, false
}

// database opens the database for a function of the bot.
func (b *bot) database(use func(db *gorm.DB) error) error {
	b.lock.Lock()
	defer b.lock.Unlock()
	db, err := openDatabase()
	if err == nil && !transactionsReady(db) {
		closeDatabase(db)
		err = errOutdated
	}
	if err != nil {
		return err
	}
	defer closeDatabase(db)
	return use(db)
}

// keyboard puts buttons in rows, with the buttons in last in a row at
// the end.
func keyboard(buttons []telegram.Button, last ...telegram.Button) [][]telegram.Button {
	var rows [][]telegram.Button
	for start := 0; start < len(buttons); start += botRow {
		rows = append(rows, buttons[start:min(start+botRow, len(buttons))])
	}
	return append(rows, last)
}

// advance asks the next question of a guide. After the last answer it
// runs the command with the answers as its options.
func (b *bot) advance(ctx context.Context, chat int64, current *wizard) {
	var asked step
	var choices []choice
	err := b.database(func(db *gorm.DB) error {
		for ; current.index < len(current.steps); current.index++ {
			asked = current.steps[current.index]
			if _, answered := current.answers[asked.flag]; answered {
				continue
			}
			if asked.when != nil && !asked.when(db, current.answers) {
				continue
			}
			if asked.choices == nil {
				return nil
			}
			var err error
			choices, err = asked.choices(db)
			return err
		}
		return nil
	})
	if err != nil {
		delete(b.wizards, chat)
		b.reply(ctx, chat, "finux: "+err.Error())
		return
	}
	if current.index >= len(current.steps) {
		delete(b.wizards, chat)
		args := current.args
		for _, done := range current.steps {
			value, answered := current.answers[done.flag]
			switch {
			case !answered || value == "":
			case done.flag == "":
				args = append(args, value)
			default:
				args = append(args, done.flag+"="+value)
			}
		}
		b.command(ctx, current.user, chat, args, current.text)
		return
	}
	b.serial++
	current.key, current.choices, current.expires = strconv.Itoa(b.serial), choices[:min(len(choices), botButtons)], time.Now().Add(botMemory)
	buttons := make([]telegram.Button, len(current.choices))
	for i, option := range current.choices {
		buttons[i] = telegram.Button{Text: option.label, Data: fmt.Sprintf("w:%s:%d", current.key, i)}
	}
	var last []telegram.Button
	if asked.optional {
		last = append(last, telegram.Button{Text: "Skip", Data: "w:" + current.key + ":s"})
	}
	last = append(last, telegram.Button{Text: "Cancel", Data: "w:" + current.key + ":c"})
	text := asked.ask
	if len(current.answers) == 0 {
		text = "<b>" + current.title + "</b>\n" + text
	}
	b.send(ctx, chat, text, keyboard(buttons, last...)...)
}

// answer takes a written answer to the question of a guide.
func (b *bot) answer(ctx context.Context, chat int64, current *wizard, text string) {
	for _, option := range current.choices {
		if strings.EqualFold(text, option.label) || strings.EqualFold(text, option.value) {
			text = option.value
		}
	}
	current.answers[current.steps[current.index].flag] = text
	current.index++
	b.advance(ctx, chat, current)
}

// pressed takes the press of a button of a guide. data is the data of
// the button after "w:".
func (b *bot) pressed(ctx context.Context, press *telegram.Callback, data string) {
	chat := press.Message.Chat.ID
	key, button, _ := strings.Cut(data, ":")
	current := b.wizards[chat]
	if current == nil || current.key != key || current.user != press.From.ID || time.Now().After(current.expires) {
		return
	}
	asked := current.steps[current.index]
	edit := func(text string) { b.edit(ctx, chat, press.Message.ID, text) }
	switch button {
	case "c":
		delete(b.wizards, chat)
		edit("Cancelled.")
		return
	case "s":
		edit(asked.label + ": <b>skipped</b>")
		current.answers[asked.flag] = ""
	default:
		index, err := strconv.Atoi(button)
		if err != nil || index < 0 || index >= len(current.choices) {
			return
		}
		edit(asked.label + ": <b>" + html.EscapeString(current.choices[index].label) + "</b>")
		current.answers[asked.flag] = current.choices[index].value
	}
	current.index++
	b.advance(ctx, chat, current)
}

// missingReference reports whether the output of a command tells that
// it needs the code or the id of an item.
func missingReference(output string) bool {
	return strings.Contains(output, "is necessary") && (strings.Contains(output, "code or id") || strings.Contains(output, "transaction id") || strings.Contains(output, "note id"))
}

// items returns the items of a module as buttons, and the noun of one
// item. A restore command gets the archived items.
func items(db *gorm.DB, module, command string) (choices []choice, noun string, err error) {
	archived := command == "restore" || command == "r"
	add := func(code string) { choices = append(choices, choice{code, code}) }
	switch module {
	case "accounts":
		noun = "account"
		var found []account.Account
		if archived {
			found, err = account.NewStore(db).ListArchived(today())
		} else {
			found, err = account.NewStore(db).List(false, today())
		}
		for _, current := range found {
			add(current.Code)
		}
	case "categories":
		noun = "category"
		choices, err = categoryChoices(db)
	case "credit-card":
		noun = "credit card"
		var found []card.Card
		if archived {
			found, err = card.NewStore(db).ListArchived()
		} else {
			found, err = card.NewStore(db).List(false)
		}
		for _, current := range found {
			add(current.Code)
		}
	case "goals":
		noun = "goal"
		var found []goal.Goal
		if archived {
			found, err = goal.NewStore(db).ListArchived()
		} else {
			found, err = goal.NewStore(db).List(false)
		}
		for _, current := range found {
			add(current.Code)
		}
	case "budgets":
		noun = "budget"
		var found []budget.Budget
		if archived {
			found, err = budget.NewStore(db).ListArchived(today())
		} else {
			found, err = budget.NewStore(db).List(false, today())
		}
		for _, current := range found {
			add(current.Code)
		}
	case "scheduled":
		noun = "schedule"
		var found []schedule.Schedule
		if archived {
			found, err = schedule.NewStore(db).ListArchived()
		} else {
			found, err = schedule.NewStore(db).List(false)
		}
		for _, current := range found {
			add(current.Code)
		}
	case "notes":
		noun = "note"
		// A note that is done can only open again.
		filter := note.Filter{}
		if command == "reopen" {
			filter.Status = note.StatusDone
		}
		var found []note.Note
		found, err = note.NewStore(db).List(filter, today())
		for _, current := range found {
			id := strconv.FormatUint(uint64(current.ID), 10)
			choices = append(choices, choice{id + " " + current.Title, id})
		}
	case "transactions":
		noun = "transaction"
		var found []transaction.View
		found, err = transaction.NewStore(db).List(transaction.Filter{})
		for _, current := range found[:min(len(found), botRecent)] {
			id := strconv.FormatUint(uint64(current.ID), 10)
			choices = append(choices, choice{id + " " + current.Title, id})
		}
	default:
		err = errors.New("no items")
	}
	return choices[:min(len(choices), botButtons)], noun, err
}

// pick asks which item a command is for, with one button for each
// item. It reports false when it has no item to show.
func (b *bot) pick(ctx context.Context, user, chat int64, args []string, text string) bool {
	if len(args) < 2 {
		return false
	}
	var choices []choice
	var noun string
	err := b.database(func(db *gorm.DB) (err error) {
		choices, noun, err = items(db, args[0], args[1])
		return err
	})
	if err != nil || len(choices) == 0 {
		return false
	}
	key := b.ask(question{user: user, args: args, text: text, choices: choices})
	buttons := make([]telegram.Button, len(choices))
	for i, option := range choices {
		buttons[i] = telegram.Button{Text: option.label, Data: fmt.Sprintf("p:%s:%d", key, i)}
	}
	b.send(ctx, chat, "Which "+noun+"?\n<code>"+html.EscapeString(text)+"</code>",
		keyboard(buttons, telegram.Button{Text: "Cancel", Data: "no:" + key})...)
	return true
}
