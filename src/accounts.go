package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"finux/src/account"
	"finux/src/category"
	"finux/src/code"
	"finux/src/money"
	"finux/src/note"
	"finux/src/schedule"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

func accountsHelp() string {
	return "Manage accounts\n\n" +
		green.Render("Usage:") + " finux accounts [command] [arguments]\n" +
		"       finux ac [command] [arguments]\n\n" +
		green.Render("Commands:") + `
  list, l [--all]              List accounts; select one to see its details (default)
  <code|id>                    Show the details of an account
  show <code|id>               Same as <code|id>
  add, a [options]             Create an account
  edit, e [code|id] [options]  Change the name, code, description, color or owner
  delete, d [code|id]          Archive an account
  restore, r [code|id]         Restore an archived account
  help, --help, -h             Show this help

` + green.Render("Options for list:") + `
  --all                 Include archived accounts and the accounts of other persons

` + green.Render("Options for add:") + `
  --name <text>         Name of the account
  --code <code>         1 to 5 letters and digits (default: made from the name)
  --description <text>  Description
  --currency <code>     ` + strings.Join(money.Codes(), ", ") + `
  --balance <amount>    Opening balance, for example 1234.56 (default: 0)
  --color <name>        ` + strings.Join(account.ColorKeys()[:8], ", ") + `,
                        ` + strings.Join(account.ColorKeys()[8:], ", ") + `
  --owner <name>        Person the account belongs to (default: you)

` + green.Render("Options for edit:") + `
  --name, --code, --description, --color, --owner

` + green.Render("Options for delete:") + `
  --hard                Delete permanently, with the transactions of the account;
                        also works on archived accounts
  --yes                 Do not ask for confirmation

Without options, add and edit open a form.
Without a code or id, edit, delete and restore open a selector.
A command name wins over an account code: use 'finux ac show L' for an
account with the code L.
An archived account keeps its transactions. A permanent delete also deletes
them. The other side of each transfer stays in its account as an expense or
an income of the category OTHER.
An account with an owner belongs to another person. The lists, the resume,
the budgets and the goals leave it out. Use --owner "" to make it yours.
`
}

// accountsCommand runs one accounts subcommand.
type accountsCommand struct {
	streams
	db    *gorm.DB
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
	defer closeDatabase(db)
	if !db.Migrator().HasColumn(&account.Account{}, "owner") {
		return command.fail(errOutdated)
	}
	announceAutomatic(db, io)
	command.db, command.store = db, account.NewStore(db)
	return command.fail(handler(rest))
}

func (c *accountsCommand) fail(err error) int {
	return exitStatus(c.streams, "accounts", err)
}

func (c *accountsCommand) print(text string) error {
	_, err := fmt.Fprint(c.stdout, text)
	return err
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
		found, err := c.store.Find(ref, true, today())
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
		accounts, err = c.store.List(false, today())
	case scopeAll:
		accounts, err = c.store.List(true, today())
	case scopeArchived:
		accounts, err = c.store.ListArchived(today())
		empty = "No archived accounts."
	}
	if err != nil {
		return account.Account{}, err
	}
	if len(accounts) == 0 {
		return account.Account{}, empty
	}
	index, err := pick(title, "account", "accounts", ui.AccountEntries(accounts))
	if err != nil {
		return account.Account{}, err
	}
	return accounts[index], nil
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
	accounts, err := c.store.List(all, today())
	if err != nil {
		return err
	}
	if !all {
		accounts = account.Mine(accounts)
	}
	if len(accounts) == 0 || !c.interactive {
		return c.print(ui.AccountTable(accounts, c.plain))
	}
	index, err := pick("Accounts", "account", "accounts", ui.AccountEntries(accounts))
	if errors.Is(err, cancelled) {
		// Leaving the list is not worth a message.
		return nil
	}
	if err != nil {
		return err
	}
	return c.card(accounts[index])
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
	return c.card(found)
}

// card prints the details of an account, then its open notes.
func (c *accountsCommand) card(current account.Account) error {
	notes := ""
	if transactionsReady(c.db) {
		var err error
		if notes, err = openNotes(c.db, note.LinkAccount, current.ID, c.plain); err != nil {
			return err
		}
	}
	return c.print(ui.AccountCard(current, c.plain) + notes)
}

// checkFlags validates the values of the flags that were given.
func checkFlags(command string, draft account.Account, given map[string]bool) error {
	var err error
	switch {
	case given["name"] && draft.Name == "":
		err = errors.New("name is empty")
	case given["code"]:
		err = code.Validate(draft.Code)
	}
	if err == nil && given["color"] {
		if _, ok := account.ColorByKey(draft.Color); !ok {
			err = fmt.Errorf("unknown color %q: use one of %s", draft.Color, strings.Join(account.ColorKeys(), ", "))
		}
	}
	if err == nil && given["currency"] && !money.Valid(draft.Currency) {
		err = fmt.Errorf("unknown currency %q: use one of %s", draft.Currency, strings.Join(money.Codes(), ", "))
	}
	if err == nil && given["owner"] {
		err = account.CheckOwner(draft.Owner)
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
		f.StringVar(&draft.Owner, "owner", "", "")
	}, 0)
	if err != nil {
		return err
	}
	draft.Name = strings.TrimSpace(draft.Name)
	draft.Code = code.Normalize(draft.Code)
	draft.Currency = strings.ToUpper(strings.TrimSpace(draft.Currency))
	draft.Color = strings.ToLower(strings.TrimSpace(draft.Color))
	draft.Owner = strings.TrimSpace(draft.Owner)
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
			if draft.Code = code.Suggest(draft.Name, c.taken(0)); draft.Code == "" {
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
			"currency": ui.FieldCurrency, "color": ui.FieldColor, "owner": ui.FieldOwner,
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
	accounts, err := c.store.List(false, today())
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
		f.StringVar(&changes.Owner, "owner", "", "")
	}, 1)
	if err != nil {
		return err
	}
	changes.Name = strings.TrimSpace(changes.Name)
	changes.Code = code.Normalize(changes.Code)
	changes.Color = strings.ToLower(strings.TrimSpace(changes.Color))
	changes.Owner = strings.TrimSpace(changes.Owner)
	if err := checkFlags("edit", changes, given); err != nil {
		return err
	}
	if len(given) == 0 && !c.interactive {
		return usageError("edit: give --name, --code, --description, --color or --owner when input is not a terminal")
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
	if given["owner"] {
		current.Owner = changes.Owner
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
	// A permanent delete also deletes the transactions of the account.
	var plain, transfers int64
	if hard && transactionsReady(c.db) {
		if plain, transfers, err = transaction.NewStore(c.db).AccountUsage(current.ID); err != nil {
			return err
		}
	}
	usage := ""
	if plain+transfers > 0 {
		usage = ", with " + plural(plain, "transaction") + " and " + plural(transfers, "transfer")
	}
	if !yes {
		question, label := fmt.Sprintf("Archive account %s (%s)?", current.Code, current.Name), "Archive"
		if hard {
			question, label = fmt.Sprintf("Delete account %s (%s) permanently%s?", current.Code, current.Name, usage), "Delete permanently"
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
		if err := c.purge(current, transfers > 0); err != nil {
			return err
		}
		return c.print(ui.Success("Account "+current.Code+" deleted permanently"+usage) + "\n")
	}
	if err := c.store.Archive(current.ID); err != nil {
		return err
	}
	return c.print(ui.Success("Account "+current.Code+" archived") + "\n")
}

// purge deletes an account and its transactions in one step. The other
// side of each of its transfers goes to the category OTHER.
func (c *accountsCommand) purge(current account.Account, hasTransfers bool) error {
	return c.db.Transaction(func(tx *gorm.DB) error {
		if transactionsReady(tx) {
			var other category.Category
			if hasTransfers {
				var err error
				if other, err = otherCategory(tx); err != nil {
					return fmt.Errorf("account %s has transfers: %w", current.Code, err)
				}
			}
			if err := transaction.NewStore(tx).PurgeAccount(current, other); err != nil {
				return err
			}
			if err := schedule.NewStore(tx).DeleteByAccount(current.ID); err != nil {
				return err
			}
			if err := note.NewStore(tx).ClearLink(note.LinkAccount, current.ID); err != nil {
				return err
			}
		}
		return account.NewStore(tx).Delete(current.ID)
	})
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
