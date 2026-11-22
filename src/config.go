package main

import (
	"errors"
	"fmt"
	"strings"

	"finux/src/setting"
	"finux/src/ui"

	"gorm.io/gorm"
)

func configHelp() string {
	settings := ""
	for _, definition := range setting.Definitions() {
		values := strings.Join(definition.Values, ", ")
		if definition.Text() {
			values = "text"
		}
		settings += fmt.Sprintf("  %-20s  %s\n", definition.Key, values)
	}
	return "Change the settings, check the database and set up AI agents\n\n" +
		green.Render("Usage:") + " finux config [command] [arguments]\n\n" +
		green.Render("Commands:") + `
  get [setting]          Show the settings, or the value of one setting
  set <setting> <value>  Change a setting
  ai [agent]             Connect Finux to an AI agent: its MCP server and its skills
  telegram               Set up the Telegram bot and its allowed people
  doctor                 Check the database
  mcp                    Run the MCP server for AI agents
  skills                 List, install or remove the skills for AI agents
  help, --help, -h       Show this help

` + green.Render("Settings:") + "\n" + settings + `
Without a command, config opens a screen where you change the settings. It
needs a terminal; without one it shows this help.

automatic-schedules: when it is on, a schedule with --auto creates its
transactions on their dates, without your confirmation. When it is off,
which is the default, no schedule creates a transaction by itself. See
'finux sc --help'.

ai-provider: the AI agent program that answers /ai in the Telegram bot:
claude, codex or opencode. 'finux config ai <agent>' sets it. With none,
which is the default, /ai is off. ai-model is the model that the agent
uses for /ai, for example opus. Empty is the default model of the agent:
sonnet for claude, gpt-6.2-sol for codex, opencode/claude-sonnet-4-6 for
opencode.

telegram-token, telegram-reminders, telegram-resume, telegram-resume-time
and daemon-mcp are for the Telegram bot and the daemon. See
'finux config telegram --help' and 'finux daemon --help'. A setting that
is text changes only with 'finux config set': give "" to clear it.

Run 'finux config <command> --help' for the options of ai, telegram, doctor, mcp and skills.
`
}

// runConfig runs the commands that change the settings, check Finux and
// connect it to AI agents.
func runConfig(args []string, io streams) int {
	if len(args) == 0 && io.interactive {
		return exitStatus(io, "config", editSettings(io))
	}
	if len(args) == 0 || isHelp(args[0]) {
		return write(io, configHelp())
	}
	switch args[0] {
	case "get":
		return exitStatus(io, "config", getSettings(args[1:], io))
	case "set":
		return exitStatus(io, "config", setSetting(args[1:], io))
	case "ai":
		return runAI(args[1:], io)
	case "telegram":
		return runTelegram(args[1:], io)
	case "doctor":
		return runDoctor(args[1:], io)
	case "mcp":
		return runMCP(args[1:], io)
	case "skills":
		return runSkills(args[1:], io)
	}
	return exitStatus(io, "config", usageError(fmt.Sprintf("unknown command: %s", args[0])))
}

// openSettings opens a database that has the table of the settings.
func openSettings() (*gorm.DB, *setting.Store, error) {
	db, err := openDatabase()
	if err == nil && !db.Migrator().HasTable(&setting.Setting{}) {
		err = errOutdated
	}
	if err != nil {
		return nil, nil, err
	}
	return db, setting.NewStore(db), nil
}

// settingError makes a mistake in the name or the value of a setting a
// usage error.
func settingError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, setting.ErrUnknown) || strings.HasPrefix(err.Error(), "unknown value") {
		return usageError(err.Error())
	}
	return err
}

// settingDefinition returns the definition of a setting that exists.
func settingDefinition(key string) setting.Definition {
	for _, definition := range setting.Definitions() {
		if definition.Key == key {
			return definition
		}
	}
	return setting.Definition{}
}

func getSettings(args []string, io streams) error {
	if len(args) > 1 {
		return usageError("get: unexpected argument: " + args[1])
	}
	_, store, err := openSettings()
	if err != nil {
		return err
	}
	if len(args) == 1 {
		value, err := store.Get(args[0])
		if err != nil {
			return settingError(err)
		}
		_, err = fmt.Fprintln(io.stdout, settingDefinition(args[0]).Show(value))
		return err
	}
	values, err := store.All()
	if err != nil {
		return err
	}
	out := ""
	for _, definition := range setting.Definitions() {
		out += fmt.Sprintf("%-20s  %s\n", definition.Key, definition.Show(values[definition.Key]))
	}
	_, err = fmt.Fprint(io.stdout, out)
	return err
}

func setSetting(args []string, io streams) error {
	switch {
	case len(args) < 2:
		return usageError("set: give a setting and its value")
	case len(args) > 2:
		return usageError("set: unexpected argument: " + args[2])
	}
	_, store, err := openSettings()
	if err != nil {
		return err
	}
	if err := store.Set(args[0], args[1]); err != nil {
		return settingError(err)
	}
	value, err := store.Get(args[0])
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(io.stdout, ui.Success(args[0]+" set to "+settingDefinition(args[0]).Show(value)))
	return err
}

// settingRows gives the rows of the settings screen: all the settings,
// with their values.
func settingRows(values map[string]string) []ui.SettingRow {
	var rows []ui.SettingRow
	for _, definition := range setting.Definitions() {
		row := ui.SettingRow{
			Key: definition.Key, Title: definition.Title, Description: definition.Description,
			Values: definition.Values, Value: values[definition.Key],
			Secret: definition.Secret, Check: definition.Check,
		}
		if definition.Key == setting.AIModel {
			// The models are the ones of the agent on the screen.
			row.Options = func(values map[string]string) []string {
				return setting.Models(values[setting.AIProvider])
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// editSettings opens the settings screen and saves what the user changed.
func editSettings(io streams) error {
	_, store, err := openSettings()
	if err != nil {
		return err
	}
	values, err := store.All()
	if err != nil {
		return err
	}
	screen, err := program(ui.NewSettings(settingRows(values)))
	if err != nil {
		return err
	}
	if !screen.Saved() {
		return cancelled
	}
	changed := 0
	for key, value := range screen.Values() {
		if value == values[key] {
			continue
		}
		if err := store.Set(key, value); err != nil {
			return err
		}
		changed++
	}
	if changed == 0 {
		return notice("No setting changed.")
	}
	message := ui.Success("Settings saved")
	if token := screen.Values()[setting.TelegramToken]; token != "" && token != values[setting.TelegramToken] {
		message += "\nThe daemon starts the bot: run 'finux daemon install', or 'finux daemon restart' if it runs."
	}
	_, err = fmt.Fprintln(io.stdout, message)
	return err
}
