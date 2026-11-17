package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"

	"finux/src/account"
	"finux/src/money"
	"finux/src/ui"
)

// usageError is a mistake in the command line. It gives exit status 2.
type usageError string

func (e usageError) Error() string { return string(e) }

// notice ends a command with a message and exit status 0, for example
// when the user cancels a prompt.
type notice string

func (n notice) Error() string { return string(n) }

const cancelled = notice("Cancelled.")

func accountsHelp() string {
	return "Manage accounts\n\n" +
		green.Render("Usage:") + " finux accounts [command] [arguments]\n" +
		"       finux ac [command] [arguments]\n\n" +
		green.Render("Commands:") + `
  list, l [--all]              List accounts; select one to see its details (default)
  <code|id>                    Show the details of an account
  show <code|id>               Same as <code|id>
  add, a [options]             Create an account
  edit, e [code|id] [options]  Change the name, code, description or color
  delete, d [code|id]          Archive an account
  restore, r [code|id]         Restore an archived account
  help, --help, -h             Show this help

` + green.Render("Options for list:") + `
  --all                 Include archived accounts

` + green.Render("Options for add:") + `
  --name <text>         Name of the account
  --code <code>         1 to 5 letters and digits (default: made from the name)
  --description <text>  Description
  --currency <code>     ` + strings.Join(money.Codes(), ", ") + `
  --balance <amount>    Opening balance, for example 1234.56 (default: 0)
  --color <name>        ` + strings.Join(account.ColorKeys()[:8], ", ") + `,
                        ` + strings.Join(account.ColorKeys()[8:], ", ") + `

` + green.Render("Options for edit:") + `
  --name, --code, --description, --color

` + green.Render("Options for delete:") + `
  --hard                Delete permanently; also works on archived accounts
  --yes                 Do not ask for confirmation

Without options, add and edit open a form.
Without a code or id, edit, delete and restore open a selector.
A command name wins over an account code: use 'finux ac show L' for an
account with the code L.
`
}

// accountsCommand runs one accounts subcommand.
type accountsCommand struct {
	streams
	store *account.Store
}

func runAccounts(args []string, io streams) int {
	command := &accountsCommand{streams: io}
	handlers := map[string]func([]string) error{
		"list": command.list, "l": command.list,
		"add": command.add, "a": command.add,
		"show": command.show,
		"edit": command.edit, "e": command.edit,
		"delete": command.delete, "d": command.delete,
		"restore": command.restore, "r": command.restore,
	}
	// No argument or an option lists accounts. A word that is not a
	// command is the code or id of an account to show.
	handler, rest := command.list, args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if known, ok := handlers[args[0]]; ok {
			handler, rest = known, args[1:]
		} else {
			handler = command.show
		}
	}
	// Help needs no database, so look for it before the database opens.
	for i, arg := range rest {
		if arg == "-h" || arg == "--help" || i == 0 && isHelp(arg) {
			return write(io, accountsHelp())
		}
	}
	db, err := openDatabase()
	if err != nil {
		return command.fail(err)
	}
	command.store = account.NewStore(db)
	return command.fail(handler(rest))
}

// fail reports err and returns the exit status: 0 for nil and notices,
// 2 for usage errors, 1 for the rest.
func (c *accountsCommand) fail(err error) int {
	var usage usageError
	var message notice
	switch {
	case err == nil:
		return 0
	case errors.As(err, &message):
		return write(c.streams, ui.Hint(string(message))+"\n")
	case errors.As(err, &usage):
		fmt.Fprintf(c.stderr, "finux: accounts: %v\nRun 'finux accounts --help' for usage.\n", err)
		return 2
	}
	fmt.Fprintf(c.stderr, "finux: %v\n", err)
	return 1
}

func (c *accountsCommand) print(text string) error {
	_, err := fmt.Fprint(c.stdout, text)
	return err
}

// parse reads flags and positional arguments in any order. It returns the
// positional arguments and the names of the flags that were given.
func parse(name string, args []string, define func(*flag.FlagSet), maxPositional int) ([]string, map[string]bool, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if define != nil {
		define(flags)
	}
	var positional []string
	for {
		if err := flags.Parse(args); err != nil {
			return nil, nil, usageError(name + ": " + err.Error())
		}
		if flags.NArg() == 0 {
			break
		}
		positional = append(positional, flags.Arg(0))
		args = flags.Args()[1:]
	}
	if len(positional) > maxPositional {
		return nil, nil, usageError(name + ": unexpected argument: " + positional[maxPositional])
	}
	given := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { given[f.Name] = true })
	return positional, given, nil
}

func reference(positional []string) string {
	if len(positional) == 0 {
		return ""
	}
	return positional[0]
}

// program runs an interactive screen and returns its final state.
func program[M tea.Model](model M) (M, error) {
	final, err := tea.NewProgram(model).Run()
	if err != nil {
		return model, fmt.Errorf("run screen: %w", err)
	}
	return final.(M), nil
}

type scope int

const (
	scopeActive scope = iota
	scopeArchived
	scopeAll
)

// resolve finds the account for a code or id. Without a reference it
// opens a selector with the accounts of the scope.
func (c *accountsCommand) resolve(command, ref, title string, in scope) (account.Account, error) {
	if ref != "" {
		found, err := c.store.Find(ref, true)
		if errors.Is(err, account.ErrNotFound) {
			return found, fmt.Errorf("account not found: %s", ref)
		}
		return found, err
	}
	if !c.interactive {
		return account.Account{}, usageError(command + ": an account code or id is necessary when input is not a terminal")
	}
	var accounts []account.Account
	var err error
	empty := notice("No accounts yet. Run 'finux ac add'.")
	switch in {
	case scopeActive:
		accounts, err = c.store.List(false)
	case scopeAll:
		accounts, err = c.store.List(true)
	case scopeArchived:
		accounts, err = c.store.ListArchived()
		empty = "No archived accounts."
	}
	if err != nil {
		return account.Account{}, err
	}
	if len(accounts) == 0 {
		return account.Account{}, empty
	}
	selector, err := program(ui.NewSelector(title, accounts))
	if err != nil {
		return account.Account{}, err
	}
	selected, ok := selector.Selected()
	if !ok {
		return selected, cancelled
	}
	return selected, nil
}

func (c *accountsCommand) taken(exceptID uint) func(string) bool {
	return func(code string) bool {
		taken, err := c.store.CodeTaken(code, exceptID)
		return err == nil && taken
	}
}

func (c *accountsCommand) list(args []string) error {
	var all bool
	if _, _, err := parse("list", args, func(f *flag.FlagSet) { f.BoolVar(&all, "all", false, "") }, 0); err != nil {
		return err
	}
	accounts, err := c.store.List(all)
	if err != nil {
		return err
	}
	if len(accounts) == 0 || !c.interactive {
		return c.print(ui.AccountTable(accounts, c.plain))
	}
	selector, err := program(ui.NewSelector("Accounts", accounts))
	if err != nil {
		return err
	}
	if selected, ok := selector.Selected(); ok {
		return c.print(ui.AccountCard(selected, c.plain))
	}
	return nil
}

func (c *accountsCommand) show(args []string) error {
	positional, _, err := parse("show", args, nil, 1)
	if err != nil {
		return err
	}
	found, err := c.resolve("show", reference(positional), "Show account", scopeActive)
	if err != nil {
		return err
	}
	return c.print(ui.AccountCard(found, c.plain))
}

// checkFlags validates the values of the flags that were given.
func checkFlags(command string, draft account.Account, given map[string]bool) error {
	var err error
	switch {
	case given["name"] && draft.Name == "":
		err = errors.New("name is empty")
	case given["code"]:
		err = account.ValidateCode(draft.Code)
	}
	if err == nil && given["color"] {
		if _, ok := account.ColorByKey(draft.Color); !ok {
			err = fmt.Errorf("unknown color %q: use one of %s", draft.Color, strings.Join(account.ColorKeys(), ", "))
		}
	}
	if err == nil && given["currency"] && !money.Valid(draft.Currency) {
		err = fmt.Errorf("unknown currency %q: use one of %s", draft.Currency, strings.Join(money.Codes(), ", "))
	}
	if err != nil {
		return usageError(command + ": " + err.Error())
	}
	return nil
}

func (c *accountsCommand) add(args []string) error {
	var draft account.Account
	var balance string
	_, given, err := parse("add", args, func(f *flag.FlagSet) {
		f.StringVar(&draft.Name, "name", "", "")
		f.StringVar(&draft.Code, "code", "", "")
		f.StringVar(&draft.Description, "description", "", "")
		f.StringVar(&draft.Currency, "currency", "", "")
		f.StringVar(&balance, "balance", "", "")
		f.StringVar(&draft.Color, "color", "", "")
	}, 0)
	if err != nil {
		return err
	}
	draft.Name = strings.TrimSpace(draft.Name)
	draft.Code = account.NormalizeCode(draft.Code)
	draft.Currency = strings.ToUpper(strings.TrimSpace(draft.Currency))
	draft.Color = strings.ToLower(strings.TrimSpace(draft.Color))
	if err := checkFlags("add", draft, given); err != nil {
		return err
	}
	if given["balance"] && given["currency"] {
		if draft.OpeningBalance, err = money.Parse(balance, draft.Currency); err != nil {
			return usageError("add: " + err.Error())
		}
	}
	if !given["color"] {
		if draft.Color, err = c.unusedColor(); err != nil {
			return err
		}
	}
	if given["name"] && given["currency"] {
		if !given["code"] {
			if draft.Code = account.SuggestCode(draft.Name, c.taken(0)); draft.Code == "" {
				return usageError(fmt.Sprintf("add: cannot make a code from the name %q; use --code", draft.Name))
			}
		}
	} else {
		if !c.interactive {
			return usageError("add: --name and --currency are necessary when input is not a terminal")
		}
		var preset []ui.Field
		for flagName, field := range map[string]ui.Field{
			"name": ui.FieldName, "code": ui.FieldCode, "description": ui.FieldDescription,
			"currency": ui.FieldCurrency, "color": ui.FieldColor,
		} {
			if given[flagName] {
				preset = append(preset, field)
			}
		}
		form, err := program(ui.NewForm(ui.FormOptions{Title: "New account", Initial: draft, Balance: balance, Preset: preset, Taken: c.taken(0)}))
		if err != nil {
			return err
		}
		if !form.Submitted() {
			return cancelled
		}
		draft = form.Account()
	}
	if err := c.store.Create(&draft); err != nil {
		return err
	}
	return c.print(ui.Success("Account "+draft.Code+" created") + "\n")
}

// unusedColor returns the first palette color that no active account
// uses, or the first color if all are in use.
func (c *accountsCommand) unusedColor() (string, error) {
	accounts, err := c.store.List(false)
	if err != nil {
		return "", err
	}
	used := map[string]bool{}
	for _, current := range accounts {
		used[current.Color] = true
	}
	for _, color := range account.Palette {
		if !used[color.Key] {
			return color.Key, nil
		}
	}
	return account.Palette[0].Key, nil
}

func (c *accountsCommand) edit(args []string) error {
	var changes account.Account
	positional, given, err := parse("edit", args, func(f *flag.FlagSet) {
		f.StringVar(&changes.Name, "name", "", "")
		f.StringVar(&changes.Code, "code", "", "")
		f.StringVar(&changes.Description, "description", "", "")
		f.StringVar(&changes.Color, "color", "", "")
	}, 1)
	if err != nil {
		return err
	}
	changes.Name = strings.TrimSpace(changes.Name)
	changes.Code = account.NormalizeCode(changes.Code)
	changes.Color = strings.ToLower(strings.TrimSpace(changes.Color))
	if err := checkFlags("edit", changes, given); err != nil {
		return err
	}
	if len(given) == 0 && !c.interactive {
		return usageError("edit: give --name, --code, --description or --color when input is not a terminal")
	}
	current, err := c.resolve("edit", reference(positional), "Edit account", scopeActive)
	if err != nil {
		return err
	}
	if current.Archived() {
		return fmt.Errorf("account not found: %s (it is archived; restore it first)", current.Code)
	}
	if len(given) == 0 {
		form, err := program(ui.NewForm(ui.FormOptions{Title: "Edit " + current.Code, Initial: current, Edit: true, Taken: c.taken(current.ID)}))
		if err != nil {
			return err
		}
		if !form.Submitted() {
			return cancelled
		}
		current = form.Account()
	}
	if given["name"] {
		current.Name = changes.Name
	}
	if given["code"] {
		current.Code = changes.Code
	}
	if given["description"] {
		current.Description = changes.Description
	}
	if given["color"] {
		current.Color = changes.Color
	}
	if err := c.store.Update(&current); err != nil {
		return err
	}
	return c.print(ui.Success("Account "+current.Code+" updated") + "\n")
}

func (c *accountsCommand) delete(args []string) error {
	var hard, yes bool
	positional, _, err := parse("delete", args, func(f *flag.FlagSet) {
		f.BoolVar(&hard, "hard", false, "")
		f.BoolVar(&yes, "yes", false, "")
	}, 1)
	if err != nil {
		return err
	}
	if !yes && !c.interactive {
		return usageError("delete: --yes is necessary when input is not a terminal")
	}
	in, title := scopeActive, "Archive account"
	if hard {
		in, title = scopeAll, "Delete account permanently"
	}
	current, err := c.resolve("delete", reference(positional), title, in)
	if err != nil {
		return err
	}
	if current.Archived() && !hard {
		return fmt.Errorf("account %s is already archived; use --hard to delete it permanently", current.Code)
	}
	if !yes {
		question, label := fmt.Sprintf("Archive account %s (%s)?", current.Code, current.Name), "Archive"
		if hard {
			question, label = fmt.Sprintf("Delete account %s (%s) permanently?", current.Code, current.Name), "Delete permanently"
		}
		confirm, err := program(ui.NewConfirm(question, label, hard))
		if err != nil {
			return err
		}
		if !confirm.Confirmed() {
			return cancelled
		}
	}
	if hard {
		if err := c.store.Delete(current.ID); err != nil {
			return err
		}
		return c.print(ui.Success("Account "+current.Code+" deleted permanently") + "\n")
	}
	if err := c.store.Archive(current.ID); err != nil {
		return err
	}
	return c.print(ui.Success("Account "+current.Code+" archived") + "\n")
}

func (c *accountsCommand) restore(args []string) error {
	positional, _, err := parse("restore", args, nil, 1)
	if err != nil {
		return err
	}
	current, err := c.resolve("restore", reference(positional), "Restore account", scopeArchived)
	if err != nil {
		return err
	}
	if !current.Archived() {
		return fmt.Errorf("account %s is not archived", current.Code)
	}
	if err := c.store.Restore(current.ID); err != nil {
		return err
	}
	return c.print(ui.Success("Account "+current.Code+" restored") + "\n")
}
