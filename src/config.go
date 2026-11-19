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
		settings += fmt.Sprintf("  %-20s  %s\n", definition.Key, strings.Join(definition.Values, ", "))
	}
	return "Change the settings, check the database and set up AI agents\n\n" +
		green.Render("Usage:") + " finux config [command] [arguments]\n\n" +
		green.Render("Commands:") + `
  get [setting]          Show the settings, or the value of one setting
  set <setting> <value>  Change a setting
  doctor                 Check the database
  mcp                    Run the MCP server for AI agents
  skills                 Install the skills for AI agents
  help, --help, -h       Show this help

` + green.Render("Settings:") + "\n" + settings + `
Without a command, config opens a screen where you change the settings. It
needs a terminal; without one it shows this help.

automatic-schedules: when it is on, a schedule with --auto creates its
transactions on their dates, without your confirmation. When it is off,
which is the default, no schedule creates a transaction by itself. See
'finux sc --help'.

ai-provider: the AI provider that Finux uses by default.

Run 'finux config <command> --help' for the options of doctor, mcp and skills.
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
		_, err = fmt.Fprintln(io.stdout, value)
		return err
	}
	values, err := store.All()
	if err != nil {
		return err
	}
	out := ""
	for _, definition := range setting.Definitions() {
		out += fmt.Sprintf("%-19s  %s\n", definition.Key, values[definition.Key])
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
	_, err = fmt.Fprintln(io.stdout, ui.Success(args[0]+" set to "+value))
	return err
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
	definitions := setting.Definitions()
	rows := make([]ui.SettingRow, len(definitions))
	for i, definition := range definitions {
		rows[i] = ui.SettingRow{
			Key: definition.Key, Title: definition.Title, Description: definition.Description,
			Values: definition.Values, Value: values[definition.Key],
		}
	}
	screen, err := program(ui.NewSettings(rows))
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
	_, err = fmt.Fprintln(io.stdout, ui.Success("Settings saved"))
	return err
}
