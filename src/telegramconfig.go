package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"finux/src/setting"
	"finux/src/telegram"
	"finux/src/ui"

	"gorm.io/gorm"
)

// telegramTokenVariable names the environment variable that replaces
// the setting telegram-token.
const telegramTokenVariable = "FINUX_TELEGRAM_TOKEN"

func telegramHelp() string {
	return "Set up the Telegram bot\n\n" +
		green.Render("Usage:") + " finux config telegram [command] [arguments]\n\n" +
		green.Render("Commands:") + `
  token <token>            Save the token of your bot
  pair <code> [name]       Let in the person who got the code from the bot
  invite [name]            Make a link that lets one person in
  allow <user-id> [name]   Let a person use the bot
  deny <user-id>           Remove a person
  help, --help, -h         Show this help

Without a command, it shows if the token is set and the allowed people.

The setup:

1. In Telegram, talk to @BotFather and send /newbot. It gives a token.
2. Run 'finux config telegram token <token>'. The ` + telegramTokenVariable + `
   environment variable replaces the saved token.
3. Start the daemon: 'finux daemon run', or 'finux daemon install' to
   keep it running.
4. Let each person in. There are three ways:
   - The person sends a message to the bot. The bot answers with a code.
     Run 'finux config telegram pair <code> [name]'.
   - Run 'finux config telegram invite [name]' and send the link to the
     person. The person opens it and presses Start.
   - Run 'finux config telegram allow <user-id> [name]' with the
     Telegram user ID, which the bot also shows.

A code and a link work one time and for 10 minutes. The daemon must run.

All the allowed people use the same database: yours. They can use the
commands of Finux that read and change data, but not config, init,
update and daemon. Send /help to the bot for the list.

With an AI agent set up, /ai in the bot sends a message, and the files
you send, to the AI. 'finux config ai <agent>' sets it up. See
'finux config ai --help'.

The settings telegram-reminders, telegram-resume and telegram-resume-time
tell what the daemon sends by itself. See 'finux config --help'.
`
}

// openTelegram opens a database that has the tables of the bot.
func openTelegram() (*gorm.DB, *telegram.Store, error) {
	db, err := openDatabase()
	if err == nil && !(db.Migrator().HasTable(&telegram.User{}) && db.Migrator().HasTable(&telegram.Notice{}) && db.Migrator().HasTable(&telegram.Session{}) && db.Migrator().HasTable(&telegram.Pairing{}) && db.Migrator().HasTable(&setting.Setting{})) {
		closeDatabase(db)
		err = errOutdated
	}
	if err != nil {
		return nil, nil, err
	}
	return db, telegram.NewStore(db), nil
}

func runTelegram(args []string, io streams) int {
	for _, arg := range args {
		if isHelp(arg) {
			return write(io, telegramHelp())
		}
	}
	return exitStatus(io, "config telegram", telegramCommand(args, io))
}

func telegramCommand(args []string, io streams) error {
	db, store, err := openTelegram()
	if err != nil {
		return err
	}
	defer closeDatabase(db)
	if len(args) == 0 {
		return showTelegram(db, store, io)
	}
	switch command, rest := args[0], args[1:]; command {
	case "token":
		if len(rest) != 1 {
			return usageError("token: give the token of the bot")
		}
		if err := setting.NewStore(db).Set(setting.TelegramToken, rest[0]); err != nil {
			return settingError(err)
		}
		message := "Telegram token saved"
		if rest[0] == "" {
			message = "Telegram token removed"
		}
		_, err = fmt.Fprintln(io.stdout, ui.Success(message))
		return err
	case "pair":
		if len(rest) == 0 {
			return usageError("pair: give the code that the bot showed")
		}
		user, err := store.Pair(rest[0], strings.Join(rest[1:], " "), time.Now())
		if errors.Is(err, telegram.ErrNoCode) {
			return err
		} else if err != nil {
			return usageError(err.Error())
		}
		message := ui.Success(person(user) + " can use the bot")
		// The person waits in Telegram: tell that the access started.
		if token := telegramToken(db); token != "" {
			if err := telegram.NewClient(token).SendMessage(context.Background(), user.ID, botWelcome, nil); err != nil {
				message += "\nThe bot could not tell the person: " + err.Error()
			}
		}
		_, err = fmt.Fprintln(io.stdout, message)
		return err
	case "invite":
		token := telegramToken(db)
		if token == "" {
			return errors.New("the token is not set. Run 'finux config telegram token <token>'")
		}
		// The link has the name of the bot, which only Telegram knows.
		me, err := telegram.NewClient(token).GetMe(context.Background())
		if err != nil {
			return fmt.Errorf("ask Telegram for the name of the bot: %w", err)
		}
		name := strings.Join(strings.Fields(strings.Join(rest, " ")), " ")
		code, err := store.Invite(name, time.Now())
		if err != nil {
			return usageError(err.Error())
		}
		who := "one person"
		if name != "" {
			who = name
		}
		_, err = fmt.Fprintf(io.stdout, "Send this link to %s. It works one time and for %d minutes:\n\nhttps://t.me/%s?start=%s\n\nThe person opens it and presses Start. The daemon must run.\n",
			who, int(telegram.PairingTime.Minutes()), me.Username, code)
		return err
	case "allow":
		id, err := telegramID(command, rest)
		if err != nil {
			return err
		}
		name := strings.Join(rest[1:], " ")
		if err := store.Allow(id, name); err != nil {
			return usageError(err.Error())
		}
		name = strings.Join(strings.Fields(name), " ")
		_, err = fmt.Fprintln(io.stdout, ui.Success(person(telegram.User{ID: id, Name: name})+" can use the bot"))
		return err
	case "deny":
		id, err := telegramID(command, rest)
		switch {
		case err != nil:
			return err
		case len(rest) > 1:
			return usageError("deny: unexpected argument: " + rest[1])
		}
		if err := store.Deny(id); errors.Is(err, telegram.ErrNotFound) {
			return fmt.Errorf("%d is %w", id, err)
		} else if err != nil {
			return err
		}
		_, err = fmt.Fprintln(io.stdout, ui.Success(fmt.Sprintf("%d cannot use the bot", id)))
		return err
	}
	return usageError("unknown command: " + args[0])
}

// person names an allowed person: the user ID, then the name.
func person(user telegram.User) string {
	who := strconv.FormatInt(user.ID, 10)
	if user.Name != "" {
		who += " (" + user.Name + ")"
	}
	return who
}

// telegramID reads the Telegram user ID that is the first argument of a
// command.
func telegramID(command string, args []string) (int64, error) {
	if len(args) == 0 {
		return 0, usageError(command + ": give the Telegram user ID")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, usageError(fmt.Sprintf("invalid user ID %q: use the number that the bot shows", args[0]))
	}
	return id, nil
}

// telegramToken returns the token of the bot: the environment variable,
// then the setting. It is empty when no token is set.
func telegramToken(db *gorm.DB) string {
	if token := strings.TrimSpace(os.Getenv(telegramTokenVariable)); token != "" {
		return token
	}
	token, _ := setting.NewStore(db).Get(setting.TelegramToken)
	return token
}

func showTelegram(db *gorm.DB, store *telegram.Store, io streams) error {
	users, err := store.Users()
	if err != nil {
		return err
	}
	out := "Token    set\n"
	if telegramToken(db) == "" {
		out = "Token    not set. Run 'finux config telegram token <token>'.\n"
	}
	if len(users) == 0 {
		out += "Allowed  nobody. Run 'finux config telegram invite [name]'.\n"
	}
	for i, user := range users {
		label := "Allowed"
		if i > 0 {
			label = ""
		}
		out += strings.TrimRight(fmt.Sprintf("%-9s%d  %s", label, user.ID, user.Name), " ") + "\n"
	}
	_, err = fmt.Fprint(io.stdout, out)
	return err
}
