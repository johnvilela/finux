package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

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
4. Each person sends a message to the bot. The bot answers with the
   Telegram user ID of the person.
5. Run 'finux config telegram allow <user-id> [name]' for each person.

All the allowed people use the same database: yours. They can use the
commands of Finux that read and change data, but not config, init,
update and daemon. Send /help to the bot for the list.

The settings telegram-reminders, telegram-resume and telegram-resume-time
tell what the daemon sends by itself. See 'finux config --help'.
`
}

// openTelegram opens a database that has the tables of the bot.
func openTelegram() (*gorm.DB, *telegram.Store, error) {
	db, err := openDatabase()
	if err == nil && !(db.Migrator().HasTable(&telegram.User{}) && db.Migrator().HasTable(&telegram.Notice{}) && db.Migrator().HasTable(&setting.Setting{})) {
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
	case "allow":
		id, err := telegramID(command, rest)
		if err != nil {
			return err
		}
		name := strings.Join(rest[1:], " ")
		if err := store.Allow(id, name); err != nil {
			return usageError(err.Error())
		}
		who := strconv.FormatInt(id, 10)
		if name = strings.Join(strings.Fields(name), " "); name != "" {
			who += " (" + name + ")"
		}
		_, err = fmt.Fprintln(io.stdout, ui.Success(who+" can use the bot"))
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
		out += "Allowed  nobody. Run 'finux config telegram allow <user-id> [name]'.\n"
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
