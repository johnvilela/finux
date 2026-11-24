package main

import (
	"errors"
	"flag"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"finux/src/account"
	"finux/src/budget"
	"finux/src/card"
	"finux/src/category"
	"finux/src/code"
	"finux/src/goal"
	"finux/src/money"
	"finux/src/schedule"
	"finux/src/setting"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

func scheduledHelp() string {
	return "Manage scheduled transactions\n\n" +
		green.Render("Usage:") + " finux scheduled [command] [arguments]\n" +
		"       finux sc [command] [arguments]\n\n" +
		green.Render("Commands:") + `
  list, l [--all]              List schedules; select one to see its details (default)
  <code|id>                    Show a schedule, what is due and its transactions
  show <code|id>               Same as <code|id>
  add, a [options]             Create a schedule
  edit, e [code|id] [options]  Change a schedule
  pay, p [code|id] [options]   Confirm the next occurrence: create its transaction.
                               Also: confirm
  skip, s [code|id]            Skip the next occurrence: no transaction
  delete, d [code|id]          Archive a schedule
  restore, r [code|id]         Restore an archived schedule
  help, --help, -h             Show this help

` + green.Render("Options for list:") + `
  --all                 Include the schedules that are done or archived

` + green.Render("Options for add and edit:") + `
  --title <text>        Title of the transactions, at most ` + strconv.Itoa(schedule.MaxTitle) + ` characters
  --code <code>         1 to 5 letters and digits (default: made from the title)
  --description <text>  Description, at most ` + strconv.Itoa(schedule.MaxDescription) + ` characters
  --value <amount>      Amount in the currency of the account, for example 1500
  --account <code>      Account or credit card of the transactions. Without it,
                        the schedule waits for the account until you pay it.
                        In edit, 'none' takes the account away
  --currency <code>     Currency of a schedule without an account (default: the
                        currency of your accounts, when they have only one)
  --category <code>     Category; red is for expenses, green for incomes
  --type <type>         ` + strings.Join(schedule.Types(), ", ") + ` (default: from the color of the category)
  --tags <tags>         Tags, for example "fixed home"
  --goal <code>         Goal the transactions count toward; 'none' takes it away
  --date <date>         Date of the first occurrence, DD/MM/YYYY or YYYY/MM/DD
                        (default: today). In edit: the date of the next occurrence
  --every <period>      Time between two occurrences: day, week, month, year, or
                        a number of them, for example "2 weeks" or "15 days".
                        Without it the schedule is for one time. In edit, 'none'
                        makes it for one time
  --until <date>        Last day an occurrence can have. In edit, 'none' removes it
  --times <number>      Number of occurrences. In edit, 'none' removes it
  --auto                Create each transaction on its date, without your
                        confirmation. It needs the setting automatic-schedules;
                        see 'finux config'. In edit, --auto=false stops it

` + green.Render("Options for pay:") + `
  --value <amount>      Real amount, when it is not the amount of the schedule
  --date <date>         Real date (default: the date of the occurrence, or today
                        for an occurrence that is not due yet)
  --account <code>      Account or credit card that paid, when it is another
                        one. Necessary for a schedule without an account

` + green.Render("Options for delete:") + `
  --hard                Delete permanently; also works on archived schedules
  --yes                 Do not ask for confirmation

A schedule is a transaction that will happen: a bill to pay or an income to
receive, one time or again and again. A schedule is not a transaction: it
does not change a balance, a bill or a budget. Each occurrence waits until
you confirm it. A schedule can wait without an account, for a bill that you
did not decide how to pay: then 'finux sc pay' needs --account, and --auto
is not possible. 'finux sc pay' creates the transaction of the next
occurrence, with the values of the schedule or the real ones you give.
'finux sc skip' drops the next occurrence. The occurrences go in order: the
oldest one first.

An occurrence is due on its date. An occurrence that you did not pay or skip
stays due, so a schedule can have many of them. 'finux tx' and 'finux resume'
tell you when occurrences are due. A budget shows what the schedules of its
categories still take from the current period.

With --auto, Finux creates the transaction of each due occurrence the next
time you run a finux command, with the date of the occurrence. It does this
only when the setting automatic-schedules is on: 'finux config set
automatic-schedules on'. The setting is off by default, and without --auto
nothing becomes a transaction by itself.

The occurrences count from the date of the first one: a monthly schedule
that starts on 31/01 uses the last day of a shorter month. A change of the
date or of the period counts again from the next occurrence.

Without a code or id, edit, pay, skip, delete and restore open a selector.
An archived schedule stops. A delete never changes a transaction.
`
}

// scheduledCommand runs one scheduled subcommand.
type scheduledCommand struct {
	streams
	db    *gorm.DB
	store *schedule.Store
	// tx finds accounts, categories and goals and gives the budget
	// warnings, as the transactions module does.
	tx *transactionsCommand
}

func newScheduledCommand(db *gorm.DB, io streams) *scheduledCommand {
	return &scheduledCommand{streams: io, db: db, store: schedule.NewStore(db), tx: newTransactionsCommand(db)}
}

func runScheduled(args []string, io streams) int {
	command := &scheduledCommand{streams: io}
	handlers := map[string]func([]string) error{
		"list": command.list, "l": command.list,
		"add": command.add, "a": command.add,
		"show": command.show,
		"edit": command.edit, "e": command.edit,
		"pay": command.pay, "p": command.pay, "confirm": command.pay,
		"skip": command.skip, "s": command.skip,
		"delete": command.delete, "d": command.delete,
		"restore": command.restore, "r": command.restore,
	}
	// No argument or an option lists schedules. A word that is not a
	// command is the code or id of a schedule to show.
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
			return write(io, scheduledHelp())
		}
	}
	db, err := openLedger(io)
	if err != nil {
		return exitStatus(io, "scheduled", err)
	}
	defer closeDatabase(db)
	command.db, command.store, command.tx = db, schedule.NewStore(db), newTransactionsCommand(db)
	return exitStatus(io, "scheduled", handler(rest))
}

func (c *scheduledCommand) print(text string) error {
	_, err := fmt.Fprint(c.stdout, text)
	return err
}

// scheduleViews loads the records that schedules point to.
func scheduleViews(db *gorm.DB, schedules []schedule.Schedule) ([]ui.ScheduleView, error) {
	views := make([]ui.ScheduleView, len(schedules))
	if len(schedules) == 0 {
		return views, nil
	}
	accounts, err := account.NewStore(db).List(true, today())
	if err != nil {
		return nil, err
	}
	cards, err := card.NewStore(db).List(true)
	if err != nil {
		return nil, err
	}
	categories, err := category.NewStore(db).List()
	if err != nil {
		return nil, err
	}
	goals, err := goal.NewStore(db).List(true)
	if err != nil {
		return nil, err
	}
	for i, current := range schedules {
		view := ui.ScheduleView{Schedule: current, OnCard: current.CardID != 0}
		if !current.Placed() {
			// Only the currency of the value is known.
			view.Account = account.Account{Currency: current.Currency}
		} else if index := slices.IndexFunc(accounts, func(a account.Account) bool { return a.ID == current.AccountID }); index >= 0 && !view.OnCard {
			view.Account = accounts[index]
		}
		if index := slices.IndexFunc(cards, func(c card.Card) bool { return c.ID == current.CardID }); index >= 0 && view.OnCard {
			owner := cards[index]
			view.Account = account.Account{Name: owner.Name, Code: owner.Code, Color: owner.Color, Currency: owner.Currency, DeletedAt: owner.DeletedAt}
		}
		if index := slices.IndexFunc(categories, func(c category.Category) bool { return c.ID == current.CategoryID }); index >= 0 {
			view.Category = categories[index]
		}
		if index := slices.IndexFunc(goals, func(g goal.Goal) bool { return g.ID == current.GoalID }); index >= 0 && current.GoalID != 0 {
			view.Goal = &goals[index]
		}
		views[i] = view
	}
	return views, nil
}

func (c *scheduledCommand) view(current schedule.Schedule) (ui.ScheduleView, error) {
	views, err := scheduleViews(c.db, []schedule.Schedule{current})
	if err != nil {
		return ui.ScheduleView{}, err
	}
	return views[0], nil
}

// resolve finds the schedule for a code or id. Without a reference it
// opens a selector with the schedules of the scope.
func (c *scheduledCommand) resolve(command, ref, title string, in scope) (schedule.Schedule, error) {
	if ref != "" {
		found, err := c.store.Find(ref, true)
		if errors.Is(err, schedule.ErrNotFound) {
			return found, fmt.Errorf("schedule not found: %s", ref)
		}
		return found, err
	}
	if !c.interactive {
		return schedule.Schedule{}, usageError(command + ": a schedule code or id is necessary when input is not a terminal")
	}
	var schedules []schedule.Schedule
	var err error
	empty := notice("No scheduled transactions yet. Run 'finux sc add'.")
	switch in {
	case scopeActive:
		schedules, err = c.store.List(false)
	case scopeAll:
		schedules, err = c.store.List(true)
	case scopeArchived:
		schedules, err = c.store.ListArchived()
		empty = "No archived schedules."
	}
	if err != nil {
		return schedule.Schedule{}, err
	}
	if len(schedules) == 0 {
		return schedule.Schedule{}, empty
	}
	views, err := scheduleViews(c.db, schedules)
	if err != nil {
		return schedule.Schedule{}, err
	}
	index, err := pick(title, "schedule", "schedules", ui.ScheduleEntries(views, today()))
	if err != nil {
		return schedule.Schedule{}, err
	}
	return schedules[index], nil
}

// active is resolve for a command that needs a schedule that is not
// archived.
func (c *scheduledCommand) active(command, ref, title string) (schedule.Schedule, error) {
	current, err := c.resolve(command, ref, title, scopeActive)
	if err == nil && current.Archived() {
		err = fmt.Errorf("schedule not found: %s (it is archived; restore it first)", current.Code)
	}
	return current, err
}

func (c *scheduledCommand) taken(exceptID uint) func(string) bool {
	return func(code string) bool {
		taken, err := c.store.CodeTaken(code, exceptID)
		return err == nil && taken
	}
}

func (c *scheduledCommand) details(current schedule.Schedule) (string, error) {
	view, err := c.view(current)
	if err != nil {
		return "", err
	}
	created, err := c.tx.store.BySchedule(current.ID)
	if err != nil {
		return "", err
	}
	return ui.ScheduleDetailsView(ui.ScheduleDetails{
		View: view, Transactions: created, Today: today(),
		AutomaticOff: !setting.NewStore(c.db).Enabled(setting.AutomaticSchedules),
	}, c.plain), nil
}

func (c *scheduledCommand) list(args []string) error {
	var all bool
	if _, _, err := parse("list", args, func(f *flag.FlagSet) { f.BoolVar(&all, "all", false, "") }, 0); err != nil {
		return err
	}
	schedules, err := c.store.List(all)
	if err != nil {
		return err
	}
	// A schedule that is done has nothing to show in the usual list.
	if !all {
		schedules = slices.DeleteFunc(schedules, func(current schedule.Schedule) bool { return current.Status(today()) == schedule.StatusDone })
	}
	views, err := scheduleViews(c.db, schedules)
	if err != nil {
		return err
	}
	if len(views) == 0 || !c.interactive {
		return c.print(ui.ScheduleTable(views, today(), c.plain))
	}
	index, err := pick("Scheduled transactions", "schedule", "schedules", ui.ScheduleEntries(views, today()))
	if errors.Is(err, cancelled) {
		// Leaving the list is not worth a message.
		return nil
	}
	if err != nil {
		return err
	}
	text, err := c.details(schedules[index])
	if err != nil {
		return err
	}
	return c.print(text)
}

func (c *scheduledCommand) show(args []string) error {
	positional, _, err := parse("show", args, nil, 1)
	if err != nil {
		return err
	}
	found, err := c.resolve("show", reference(positional), "Show schedule", scopeAll)
	if err != nil {
		return err
	}
	text, err := c.details(found)
	if err != nil {
		return err
	}
	return c.print(text)
}

// scheduleFlags holds the values of the options of add and edit.
type scheduleFlags struct {
	title, code, description, value, kind, category, tags, account, date, goal, every, until, times string
	currency                                                                                        string
	auto                                                                                            bool
}

func (v *scheduleFlags) define(flags *flag.FlagSet) {
	flags.StringVar(&v.title, "title", "", "")
	flags.StringVar(&v.code, "code", "", "")
	flags.StringVar(&v.description, "description", "", "")
	flags.StringVar(&v.value, "value", "", "")
	flags.StringVar(&v.kind, "type", "", "")
	flags.StringVar(&v.category, "category", "", "")
	flags.StringVar(&v.tags, "tags", "", "")
	flags.StringVar(&v.account, "account", "", "")
	flags.StringVar(&v.currency, "currency", "", "")
	flags.StringVar(&v.date, "date", "", "")
	flags.StringVar(&v.goal, "goal", "", "")
	flags.StringVar(&v.every, "every", "", "")
	flags.StringVar(&v.until, "until", "", "")
	flags.StringVar(&v.times, "times", "", "")
	flags.BoolVar(&v.auto, "auto", false, "")
}

// isNone reports whether the text of an option removes a value.
func isNone(text string) bool {
	text = strings.TrimSpace(text)
	return text == "" || strings.EqualFold(text, noValue)
}

// place returns the account of a schedule, or its credit card shown as
// an account. A schedule without them gives only its currency.
func (c *scheduledCommand) place(current schedule.Schedule) (account.Account, error) {
	if !current.Placed() {
		return account.Account{Currency: current.Currency}, nil
	}
	ref := strconv.FormatUint(uint64(current.AccountID), 10)
	if current.CardID != 0 {
		owner, err := c.tx.cards.Find(strconv.FormatUint(uint64(current.CardID), 10), true)
		return account.Account{Name: owner.Name, Code: owner.Code, Color: owner.Color, Currency: owner.Currency, DeletedAt: owner.DeletedAt}, err
	}
	return c.tx.accounts.Find(ref, true, today())
}

// apply validates the options that were given and writes them to the
// schedule. In an edit, a new date and a new period count again from the
// next occurrence.
func (c *scheduledCommand) apply(command string, values scheduleFlags, given map[string]bool, current *schedule.Schedule) error {
	create := current.ID == 0
	usage := func(err error) error { return usageError(command + ": " + err.Error()) }
	var err error
	if given["title"] {
		if current.Title = strings.TrimSpace(values.title); current.Title == "" {
			return usage(errors.New("title is empty"))
		}
	}
	if given["code"] {
		current.Code = code.Normalize(values.code)
		if err := code.Validate(current.Code); err != nil {
			return usage(err)
		}
	}
	if given["description"] {
		current.Description = strings.TrimSpace(values.description)
	}
	if given["type"] {
		if current.Type = strings.ToLower(strings.TrimSpace(values.kind)); !slices.Contains(schedule.Types(), current.Type) {
			return usage(fmt.Errorf("unknown type %q: use one of %s", values.kind, strings.Join(schedule.Types(), ", ")))
		}
	}
	if given["tags"] {
		if current.Tags, err = transaction.ParseTags(values.tags); err != nil {
			return usage(err)
		}
	}
	next, hasNext := current.Next()
	if given["every"] {
		if current.Every, current.Unit, err = budget.ParseEvery(values.every); err != nil {
			return usage(err)
		}
	}
	if given["until"] {
		current.EndDate = ""
		if !isNone(values.until) {
			if current.EndDate, err = transaction.ParseDate(values.until); err != nil {
				return usage(err)
			}
		}
	}
	if given["times"] {
		current.Times = 0
		if !create && isNone(values.times) {
			// The schedule has no number of occurrences now.
		} else if current.Times, err = strconv.Atoi(strings.TrimSpace(values.times)); err != nil || current.Times < 1 || current.Times > schedule.MaxTimes {
			return usage(fmt.Errorf("--times must be a number from 1 to %d", schedule.MaxTimes))
		} else if !create {
			// The number is of the occurrences from now on.
			current.Times += current.Done
		}
	}
	switch {
	case given["date"]:
		date, err := transaction.ParseDate(values.date)
		if err != nil {
			return usage(err)
		}
		current.Rebase(date)
	case create:
		current.StartDate = today()
	case given["every"] && hasNext:
		current.Rebase(next)
	}
	if given["auto"] {
		current.Auto = values.auto
	}

	// The records the schedule points to, and the rules between them.
	var before account.Account
	if !create {
		if before, err = c.place(*current); err != nil {
			return err
		}
	}
	holder := before
	// In an edit, the text 'none' takes the account away.
	placed := given["account"] && (create || !isNone(values.account))
	switch {
	case placed && given["currency"]:
		return usage(errors.New("give --account or --currency, not the two"))
	case placed:
		found, cardID, err := c.tx.findPlace(values.account, false)
		if err != nil {
			return err
		}
		if !create && found.Currency != before.Currency && !given["value"] {
			return usage(fmt.Errorf("give --value: the account %s is in %s, not in %s", found.Code, found.Currency, before.Currency))
		}
		holder, current.AccountID, current.CardID = found, found.ID, cardID
		if cardID != 0 {
			current.AccountID = 0
		}
	case given["currency"]:
		currency := strings.ToUpper(strings.TrimSpace(values.currency))
		switch {
		case !money.Valid(currency):
			return usage(fmt.Errorf("unknown currency %q: use one of %s", values.currency, strings.Join(money.Codes(), ", ")))
		case current.Placed() && !given["account"]:
			return usage(errors.New("--currency is only for a schedule without an account"))
		case !create && currency != before.Currency && !given["value"]:
			return usage(fmt.Errorf("give --value: the schedule is in %s, not in %s", before.Currency, currency))
		}
		holder = account.Account{Currency: currency}
	case create:
		if holder.Currency, err = c.onlyCurrency(); err != nil {
			return usage(err)
		}
	}
	if !placed && (given["account"] || create) {
		current.AccountID, current.CardID = 0, 0
	}
	current.Currency = holder.Currency
	if given["value"] {
		if current.Value, err = amount(command, values.value, holder.Currency); err != nil {
			return err
		}
	}
	var group category.Category
	if given["category"] {
		if group, err = c.tx.findCategory(values.category); err != nil {
			return err
		}
		current.CategoryID = group.ID
	} else if current.CategoryID != 0 {
		if group, err = c.tx.findCategory(strconv.FormatUint(uint64(current.CategoryID), 10)); err != nil {
			return err
		}
	}
	if current.Type == "" {
		switch group.Color {
		case "red":
			current.Type = schedule.Expense
		case "green":
			current.Type = schedule.Income
		case "grey":
			return usage(fmt.Errorf("--type is necessary: the category %s is for all types", group.Code))
		}
	}
	if group.ID != 0 && current.Type != "" && !transaction.Accepts(current.Type, group.Color) {
		return usage(fmt.Errorf("category %s is for %s transactions: it cannot have a transaction of the type %s", group.Code, group.Meaning(), current.Type))
	}
	target := goal.Goal{ID: current.GoalID}
	if given["goal"] {
		if target, err = c.tx.findGoal(values.goal); err != nil {
			return err
		}
		current.GoalID = target.ID
	} else if target.ID != 0 {
		if target, err = c.tx.findGoal(strconv.FormatUint(uint64(target.ID), 10)); err != nil {
			return err
		}
	}
	if target.ID != 0 && holder.Currency != "" && target.Currency != holder.Currency {
		return usage(fmt.Errorf("goal %s is in %s, not in %s", target.Code, target.Currency, holder.Currency))
	}
	return nil
}

// onlyCurrency returns the currency of the accounts of the user, for a
// schedule without an account. It fails when they have more than one.
func (c *scheduledCommand) onlyCurrency() (string, error) {
	accounts, err := c.tx.accounts.List(false, today())
	if err != nil {
		return "", err
	}
	var currencies []string
	for _, current := range accounts {
		if !slices.Contains(currencies, current.Currency) {
			currencies = append(currencies, current.Currency)
		}
	}
	switch len(currencies) {
	case 0:
		return "", errors.New("give --account or --currency: you have no account")
	case 1:
		return currencies[0], nil
	}
	return "", errors.New("give --account or --currency: your accounts have more than one currency")
}

func (c *scheduledCommand) add(args []string) error {
	var values scheduleFlags
	_, given, err := parse("add", args, values.define, 0)
	if err != nil {
		return err
	}
	if !given["title"] || !given["value"] || !given["category"] {
		return usageError("add: --title, --value and --category are necessary")
	}
	var draft schedule.Schedule
	if err := c.apply("add", values, given, &draft); err != nil {
		return err
	}
	if !given["code"] {
		if draft.Code = code.Suggest(draft.Title, c.taken(0)); draft.Code == "" {
			return usageError(fmt.Sprintf("add: cannot make a code from the title %q; use --code", draft.Title))
		}
	}
	if err := draft.Validate(); err != nil {
		return usageError("add: " + err.Error())
	}
	if err := c.store.Create(&draft); err != nil {
		return err
	}
	return c.print(ui.Success("Schedule "+draft.Code+" created") + "\n" + automaticOff(c.db, draft))
}

func (c *scheduledCommand) edit(args []string) error {
	var values scheduleFlags
	positional, given, err := parse("edit", args, values.define, 1)
	if err != nil {
		return err
	}
	if len(given) == 0 {
		return usageError("edit: give --title, --code, --description, --value, --account, --currency, --category, --type, --tags, --goal, --date, --every, --until, --times or --auto")
	}
	current, err := c.active("edit", reference(positional), "Edit schedule")
	if err != nil {
		return err
	}
	if err := c.apply("edit", values, given, &current); err != nil {
		return err
	}
	if err := current.Validate(); err != nil {
		return usageError("edit: " + err.Error())
	}
	if err := c.store.Update(&current); err != nil {
		return err
	}
	return c.print(ui.Success("Schedule "+current.Code+" updated") + "\n" + automaticOff(c.db, current))
}

// occurrenceDraft returns the transaction of an occurrence of a
// schedule on a date.
func occurrenceDraft(current schedule.Schedule, date string) transaction.Draft {
	return transaction.Draft{
		Title: current.Title, Description: current.Description, Type: current.Type, Value: current.Value,
		AccountID: current.AccountID, CardID: current.CardID, CategoryID: current.CategoryID, GoalID: current.GoalID,
		Tags: current.Tags, Date: date, ScheduleID: current.ID,
	}
}

// settle creates the transaction of the next occurrence of a schedule
// and marks the occurrence as paid, in one step.
func settle(db *gorm.DB, current schedule.Schedule, draft transaction.Draft) (transaction.Transaction, error) {
	var created transaction.Transaction
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		if created, err = transaction.NewStore(tx).Create(draft); err != nil {
			return err
		}
		return schedule.NewStore(tx).Advance(current.ID)
	})
	return created, err
}

// errNoOccurrence is the error of a pay or a skip of a schedule that is done.
func errNoOccurrence(current schedule.Schedule) error {
	return fmt.Errorf("schedule %s has no occurrence left", current.Code)
}

// payDraft makes the transaction that pays the next occurrence of a
// schedule. The options that were given replace its value, its date
// and its account. Without a date, an occurrence that is due keeps its
// date and one that is not due yet gets the date of today.
func (c *scheduledCommand) payDraft(current schedule.Schedule, value, date, place string, given map[string]bool) (transaction.Draft, error) {
	next, ok := current.Next()
	if !ok {
		return transaction.Draft{}, errNoOccurrence(current)
	}
	draft := occurrenceDraft(current, min(next, today()))
	holder, err := c.place(current)
	if err != nil {
		return draft, err
	}
	if !current.Placed() && !given["account"] {
		return draft, usageError(fmt.Sprintf("pay: give --account: the schedule %s has no account", current.Code))
	}
	var err2 error
	if given["date"] {
		if draft.Date, err2 = transaction.ParseDate(date); err2 != nil {
			return draft, usageError("pay: " + err2.Error())
		}
	}
	if given["account"] {
		found, cardID, err := c.tx.findPlace(place, false)
		if err != nil {
			return draft, err
		}
		if found.Currency != holder.Currency && !given["value"] {
			return draft, usageError(fmt.Sprintf("pay: give --value: the account %s is in %s, not in %s", found.Code, found.Currency, holder.Currency))
		}
		holder, draft.AccountID, draft.CardID = found, found.ID, cardID
		if cardID != 0 {
			draft.AccountID = 0
		}
	}
	if given["value"] {
		if draft.Value, err = amount("pay", value, holder.Currency); err != nil {
			return draft, err
		}
	}
	return draft, nil
}

// upcoming tells the date of the next occurrence of a schedule, read
// again from the database.
func (c *scheduledCommand) upcoming(id uint) (string, error) {
	current, err := c.store.Find(strconv.FormatUint(uint64(id), 10), true)
	if err != nil {
		return "", err
	}
	if next, ok := current.Next(); ok {
		return "Next: " + transaction.FormatDate(next), nil
	}
	return "No occurrence is left.", nil
}

// paidMessage is the feedback of an occurrence that became a transaction.
func paidMessage(current schedule.Schedule, created transaction.View) string {
	verb := "paid"
	if created.Type == transaction.Income {
		verb = "received"
	}
	return fmt.Sprintf("Schedule %s %s: transaction %d created (%s, %s)", current.Code, verb, created.ID,
		transaction.FormatDate(created.Date), money.Format(created.Amount, created.Account.Currency))
}

func (c *scheduledCommand) pay(args []string) error {
	var value, date, place string
	positional, given, err := parse("pay", args, func(f *flag.FlagSet) {
		f.StringVar(&value, "value", "", "")
		f.StringVar(&date, "date", "", "")
		f.StringVar(&place, "account", "", "")
	}, 1)
	if err != nil {
		return err
	}
	current, err := c.active("pay", reference(positional), "Pay scheduled transaction")
	if err != nil {
		return err
	}
	draft, err := c.payDraft(current, value, date, place, given)
	if err != nil {
		return err
	}
	created, err := settle(c.db, current, draft)
	if err != nil {
		return err
	}
	view, err := c.tx.store.Find(created.ID)
	if err != nil {
		return err
	}
	next, err := c.upcoming(current.ID)
	if err != nil {
		return err
	}
	out := ui.Success(paidMessage(current, view)) + "\n" + ui.Hint(next) + "\n"
	warnings, err := budgetWarnings(c.tx.budgets, view)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		out += ui.Warning(warning.Message) + "\n"
	}
	return c.print(out)
}

func (c *scheduledCommand) skip(args []string) error {
	positional, _, err := parse("skip", args, nil, 1)
	if err != nil {
		return err
	}
	current, err := c.active("skip", reference(positional), "Skip scheduled transaction")
	if err != nil {
		return err
	}
	skipped, ok := current.Next()
	if !ok {
		return errNoOccurrence(current)
	}
	if err := c.store.Advance(current.ID); err != nil {
		return err
	}
	next, err := c.upcoming(current.ID)
	if err != nil {
		return err
	}
	return c.print(ui.Success("Schedule "+current.Code+" skipped: "+transaction.FormatDate(skipped)) + "\n" + ui.Hint(next) + "\n")
}

func (c *scheduledCommand) delete(args []string) error {
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
	in, title := scopeActive, "Archive schedule"
	if hard {
		in, title = scopeAll, "Delete schedule permanently"
	}
	current, err := c.resolve("delete", reference(positional), title, in)
	if err != nil {
		return err
	}
	if current.Archived() && !hard {
		return fmt.Errorf("schedule %s is already archived; use --hard to delete it permanently", current.Code)
	}
	if !yes {
		question, label := fmt.Sprintf("Archive schedule %s (%s)? It stops.", current.Code, current.Title), "Archive"
		if hard {
			question, label = fmt.Sprintf("Delete schedule %s (%s) permanently? Its transactions stay.", current.Code, current.Title), "Delete permanently"
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
		if err := purgeSchedule(c.db, current.ID); err != nil {
			return err
		}
		return c.print(ui.Success("Schedule "+current.Code+" deleted permanently") + "\n")
	}
	if err := c.store.Archive(current.ID); err != nil {
		return err
	}
	return c.print(ui.Success("Schedule "+current.Code+" archived") + "\n")
}

// purgeSchedule deletes a schedule in one step. Its transactions stay;
// they lose the link to the schedule.
func purgeSchedule(db *gorm.DB, id uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := transaction.NewStore(tx).ClearSchedule(id); err != nil {
			return err
		}
		return schedule.NewStore(tx).Delete(id)
	})
}

func (c *scheduledCommand) restore(args []string) error {
	positional, _, err := parse("restore", args, nil, 1)
	if err != nil {
		return err
	}
	current, err := c.resolve("restore", reference(positional), "Restore schedule", scopeArchived)
	if err != nil {
		return err
	}
	if !current.Archived() {
		return fmt.Errorf("schedule %s is not archived", current.Code)
	}
	if err := c.store.Restore(current.ID); err != nil {
		return err
	}
	return c.print(ui.Success("Schedule "+current.Code+" restored") + "\n")
}

// openLedger opens a database that has all the tables of this Finux.
// Then it creates the transactions of the automatic schedules that are
// due, and tells them on the output of the command.
func openLedger(io streams) (*gorm.DB, error) {
	db, err := openDatabase()
	if err == nil && !transactionsReady(db) {
		err = errOutdated
	}
	if err != nil {
		return nil, err
	}
	announceAutomatic(db, io)
	return db, nil
}

// announceAutomatic creates the transactions of the automatic schedules
// that are due and tells them on the output of the command. It does
// nothing in a database from an older Finux.
func announceAutomatic(db *gorm.DB, io streams) {
	if !transactionsReady(db) {
		return
	}
	syncStatuses(db)
	for _, line := range runAutomatic(db) {
		fmt.Fprintln(io.stdout, line)
	}
}

// syncStatuses makes the stored statuses of the transactions and the
// bills current for the date of today. It returns the bills that got
// another status. Each command does it, so the statuses are right
// without the daemon.
func syncStatuses(db *gorm.DB) []card.Change {
	changes, _ := transaction.NewStore(db).SyncStatuses(today())
	return changes
}

// dueNotice returns one line that tells how many occurrences of the
// schedules are due, or "" when none is.
func dueNotice(store *schedule.Store) (string, error) {
	schedules, err := store.List(false)
	if err != nil {
		return "", err
	}
	due := 0
	for _, current := range schedules {
		due += len(current.Pending(today()))
	}
	switch due {
	case 0:
		return "", nil
	case 1:
		return ui.Warning("1 scheduled transaction is due. Run 'finux sc'.") + "\n", nil
	}
	return ui.Warning(fmt.Sprintf("%d scheduled transactions are due. Run 'finux sc'.", due)) + "\n", nil
}

// automaticOff returns one line that tells that an automatic schedule
// waits because the setting is off, or "" when that is not so.
func automaticOff(db *gorm.DB, current schedule.Schedule) string {
	if !current.Auto || setting.NewStore(db).Enabled(setting.AutomaticSchedules) {
		return ""
	}
	return ui.Warning(fmt.Sprintf("Automatic schedules are off, so %s waits for your confirmation. Run 'finux config set %s on' to turn them on.", current.Code, setting.AutomaticSchedules)) + "\n"
}

// runAutomatic creates the transactions of the due occurrences of the
// schedules that are automatic, when the setting for them is on. It returns one line of feedback for
// each transaction, and one for a schedule that failed: its
// occurrences stay due.
func runAutomatic(db *gorm.DB) []string {
	if !setting.NewStore(db).Enabled(setting.AutomaticSchedules) {
		return nil
	}
	store := schedule.NewStore(db)
	schedules, err := store.List(false)
	if err != nil {
		return nil
	}
	transactions := transaction.NewStore(db)
	var lines []string
	for _, current := range schedules {
		if !current.Auto {
			continue
		}
		for _, date := range current.Pending(today()) {
			created, err := settle(db, current, occurrenceDraft(current, date))
			if err != nil {
				lines = append(lines, ui.Warning(fmt.Sprintf("Schedule %s is automatic, but its occurrence of %s failed: %v", current.Code, transaction.FormatDate(date), err)))
				break
			}
			view, err := transactions.Find(created.ID)
			if err != nil {
				break
			}
			lines = append(lines, ui.Success(paidMessage(current, view)))
		}
	}
	return lines
}
