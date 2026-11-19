package main

import (
	"errors"
	"flag"
	"fmt"
	"slices"
	"strings"

	"finux/src/budget"
	"finux/src/category"
	"finux/src/code"
	"finux/src/money"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

const (
	// noValue is the word that stands for no period and for no end date
	// in an option.
	noValue = "none"
	// allCategories is the word that stands for all categories in an
	// option.
	allCategories = "all"
	// defaultHistory is the number of past periods that show prints.
	defaultHistory = 6
)

func budgetsHelp() string {
	return "Manage budgets\n\n" +
		green.Render("Usage:") + " finux budgets [command] [arguments]\n" +
		"       finux bg [command] [arguments]\n\n" +
		green.Render("Commands:") + `
  list, l [--all]              List budgets; select one to see its details (default)
  <code|id>                    Show a budget, its current period and its history
  show <code|id> [options]     Same as <code|id>
  add, a [options]             Create a budget
  edit, e [code|id] [options]  Change a budget
  delete, d [code|id]          Archive a budget
  restore, r [code|id]         Restore an archived budget
  help, --help, -h             Show this help

` + green.Render("Options for list:") + `
  --all                 Include archived budgets

` + green.Render("Options for show:") + `
  --period <date>       Show the period that has this date, not the current one
  --history <number>    Number of periods before it to list (default: ` + fmt.Sprint(defaultHistory) + `)

` + green.Render("Options for add and edit:") + `
  --title <text>        Title of the budget, at most ` + fmt.Sprint(budget.MaxTitle) + ` characters
  --code <code>         1 to 5 letters and digits (default: made from the title)
  --description <text>  Description, at most ` + fmt.Sprint(budget.MaxDescription) + ` characters
  --limit <amount>      Amount of one period, for example 800
  --currency <code>     ` + strings.Join(money.Codes(), ", ") + ` (add only)
  --categories <codes>  Categories that count, for example FOOD,FUN. 'all' or
                        no option means all categories
  --every <period>      Length of a period: day, week, month, year, or a number
                        of them, for example "2 weeks" or "15 days".
                        'none' makes a one-time budget
  --start <date>        First day of the first period, DD/MM/YYYY or YYYY/MM/DD
                        (default: the first day of this week, month or year
                        for a period of one of them; today for the others)
  --until <date>        Last day of the budget. Optional with --every;
                        necessary without it. In edit, 'none' removes it
  --rollover            Move what a period did not spend, or spent too much,
                        to the next period. In edit, --rollover=false stops it

` + green.Render("Options for delete:") + `
  --hard                Delete permanently; also works on archived budgets
  --yes                 Do not ask for confirmation

Without options, add and edit open a form.
Without a code or id, edit, delete and restore open a selector.

A budget counts every expense and income of its categories in its currency,
in the period that has the date of the transaction. You do not link a
transaction to a budget. An expense adds to what the period spent; an income
gives money back. A transfer and the payment of a credit card bill do not
count: a purchase with a card counts on the day of the purchase. A budget for
all categories leaves out the income categories.

The periods start on the start date: a monthly budget that starts on
05/11/2026 has periods from day 5 to day 4. A period is ok, then near when it
spent ` + fmt.Sprint(budget.NearPercent) + `% of its amount or more, then over. 'finux tx add' and 'finux tx edit'
tell you when a budget is near or over.

An archived budget keeps its history and gives no warnings. A delete never
changes a transaction.
`
}

// budgetsCommand runs one budgets subcommand.
type budgetsCommand struct {
	streams
	db    *gorm.DB
	store *budget.Store
}

func runBudgets(args []string, io streams) int {
	command := &budgetsCommand{streams: io}
	handlers := map[string]func([]string) error{
		"list": command.list, "l": command.list,
		"add": command.add, "a": command.add,
		"show": command.show,
		"edit": command.edit, "e": command.edit,
		"delete": command.delete, "d": command.delete,
		"restore": command.restore, "r": command.restore,
	}
	// No argument or an option lists budgets. A word that is not a command
	// is the code or id of a budget to show.
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
			return write(io, budgetsHelp())
		}
	}
	db, err := openDatabase()
	if err == nil && !transactionsReady(db) {
		err = errOutdated
	}
	if err != nil {
		return exitStatus(io, "budgets", err)
	}
	command.db, command.store = db, budget.NewStore(db)
	return exitStatus(io, "budgets", handler(rest))
}

func (c *budgetsCommand) print(text string) error {
	_, err := fmt.Fprint(c.stdout, text)
	return err
}

// resolve finds the budget for a code or id, with its periods up to the
// date until. Without a reference it opens a selector with the budgets
// of the scope.
func (c *budgetsCommand) resolve(command, ref, title string, in scope, until string) (budget.Budget, error) {
	if ref != "" {
		found, err := c.store.Find(ref, true, until)
		if errors.Is(err, budget.ErrNotFound) {
			return found, fmt.Errorf("budget not found: %s", ref)
		}
		return found, err
	}
	if !c.interactive {
		return budget.Budget{}, usageError(command + ": a budget code or id is necessary when input is not a terminal")
	}
	var budgets []budget.Budget
	var err error
	empty := notice("No budgets yet. Run 'finux bg add'.")
	switch in {
	case scopeActive:
		budgets, err = c.store.List(false, until)
	case scopeAll:
		budgets, err = c.store.List(true, until)
	case scopeArchived:
		budgets, err = c.store.ListArchived(until)
		empty = "No archived budgets."
	}
	if err != nil {
		return budget.Budget{}, err
	}
	if len(budgets) == 0 {
		return budget.Budget{}, empty
	}
	index, err := pick(title, "budget", "budgets", ui.BudgetEntries(budgets, today()))
	if err != nil {
		return budget.Budget{}, err
	}
	return budgets[index], nil
}

func (c *budgetsCommand) taken(exceptID uint) func(string) bool {
	return func(code string) bool {
		taken, err := c.store.CodeTaken(code, exceptID)
		return err == nil && taken
	}
}

// gather reads what the details of a budget show: one period, the
// transactions that count in it and the periods before it. date is a
// day of the period, as YYYY-MM-DD; an empty date gives the current
// period, or the last one of a budget that ended. current has its
// periods up to that date or to today, the later of the two.
func (c *budgetsCommand) gather(current budget.Budget, date string, history int) (ui.BudgetDetails, error) {
	details := ui.BudgetDetails{Budget: current, Today: today()}
	categories, err := category.NewStore(c.db).List()
	if err != nil {
		return details, err
	}
	income := map[uint]bool{}
	for _, known := range categories {
		income[known.ID] = known.Color == budget.IncomeColor
		if slices.Contains(current.CategoryIDs, known.ID) {
			details.Categories = append(details.Categories, known)
		}
	}
	usage, ok := current.Shown(details.Today)
	if date != "" {
		if usage, ok = current.UsageAt(date); !ok {
			return details, fmt.Errorf("budget %s has no period on %s", current.Code, transaction.FormatDate(date))
		}
	}
	if !ok {
		return details, nil
	}
	details.Usage = &usage
	for index := usage.Index - 1; index >= 0 && len(details.History) < history; index-- {
		details.History = append(details.History, current.Usages[index])
	}
	views, err := transaction.NewStore(c.db).List(transaction.Filter{})
	if err != nil {
		return details, err
	}
	for _, view := range views {
		if view.Date >= usage.From && view.Date <= usage.To &&
			current.Counts(view.Type, view.CategoryID, income[view.CategoryID], view.Account.Currency) {
			details.Transactions = append(details.Transactions, view)
		}
	}
	return details, nil
}

func (c *budgetsCommand) details(current budget.Budget, date string, history int) (string, error) {
	details, err := c.gather(current, date, history)
	if err != nil {
		return "", err
	}
	return ui.BudgetDetailsView(details, c.plain), nil
}

func (c *budgetsCommand) list(args []string) error {
	var all bool
	if _, _, err := parse("list", args, func(f *flag.FlagSet) { f.BoolVar(&all, "all", false, "") }, 0); err != nil {
		return err
	}
	budgets, err := c.store.List(all, today())
	if err != nil {
		return err
	}
	if len(budgets) == 0 || !c.interactive {
		return c.print(ui.BudgetTable(budgets, today(), c.plain))
	}
	index, err := pick("Budgets", "budget", "budgets", ui.BudgetEntries(budgets, today()))
	if errors.Is(err, cancelled) {
		// Leaving the list is not worth a message.
		return nil
	}
	if err != nil {
		return err
	}
	text, err := c.details(budgets[index], "", defaultHistory)
	if err != nil {
		return err
	}
	return c.print(text)
}

func (c *budgetsCommand) show(args []string) error {
	var period string
	history := defaultHistory
	positional, given, err := parse("show", args, func(f *flag.FlagSet) {
		f.StringVar(&period, "period", "", "")
		f.IntVar(&history, "history", defaultHistory, "")
	}, 1)
	if err != nil {
		return err
	}
	date, until := "", today()
	if given["period"] {
		if date, err = transaction.ParseDate(period); err != nil {
			return usageError("show: " + err.Error())
		}
		until = max(until, date)
	}
	if history < 0 {
		return usageError("show: --history must be 0 or more")
	}
	found, err := c.resolve("show", reference(positional), "Show budget", scopeAll, until)
	if err != nil {
		return err
	}
	text, err := c.details(found, date, history)
	if err != nil {
		return err
	}
	return c.print(text)
}

// budgetFlags holds the values of the options of add and edit.
type budgetFlags struct {
	title, code, description, limit, currency, categories, start, every, until string
	rollover                                                                   bool
}

func (v *budgetFlags) define(flags *flag.FlagSet, create bool) {
	flags.StringVar(&v.title, "title", "", "")
	flags.StringVar(&v.code, "code", "", "")
	flags.StringVar(&v.description, "description", "", "")
	flags.StringVar(&v.limit, "limit", "", "")
	flags.StringVar(&v.categories, "categories", "", "")
	flags.StringVar(&v.start, "start", "", "")
	flags.StringVar(&v.every, "every", "", "")
	flags.StringVar(&v.until, "until", "", "")
	flags.BoolVar(&v.rollover, "rollover", false, "")
	if create {
		flags.StringVar(&v.currency, "currency", "", "")
	}
}

// budgetCategories reads a list of category codes or ids, with commas or
// spaces between them. "all" or an empty text gives no categories: all
// categories count.
func (c *budgetsCommand) budgetCategories(text string) ([]uint, error) {
	words := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' })
	if len(words) == 0 || len(words) == 1 && strings.EqualFold(words[0], allCategories) {
		return nil, nil
	}
	store := category.NewStore(c.db)
	ids := make([]uint, 0, len(words))
	for _, word := range words {
		found, err := store.Find(word)
		if errors.Is(err, category.ErrNotFound) {
			return nil, fmt.Errorf("category not found: %s", strings.ToUpper(word))
		}
		if err != nil {
			return nil, err
		}
		ids = append(ids, found.ID)
	}
	return ids, nil
}

// apply validates the options that were given and writes them to the
// budget. The limit needs the currency of the budget.
func (c *budgetsCommand) apply(command string, values budgetFlags, given map[string]bool, current *budget.Budget) error {
	var err error
	if given["title"] {
		if current.Title = strings.TrimSpace(values.title); current.Title == "" {
			err = errors.New("title is empty")
		}
	}
	if err == nil && given["code"] {
		current.Code = code.Normalize(values.code)
		err = code.Validate(current.Code)
	}
	if given["description"] {
		current.Description = strings.TrimSpace(values.description)
	}
	if err == nil && given["currency"] {
		if current.Currency = strings.ToUpper(strings.TrimSpace(values.currency)); !money.Valid(current.Currency) {
			err = fmt.Errorf("unknown currency %q: use one of %s", current.Currency, strings.Join(money.Codes(), ", "))
		}
	}
	if err == nil && given["categories"] {
		current.CategoryIDs, err = c.budgetCategories(values.categories)
	}
	if err == nil && given["every"] {
		current.Every, current.Unit, err = budget.ParseEvery(values.every)
	}
	if err == nil && given["start"] {
		current.StartDate, err = transaction.ParseDate(values.start)
	}
	if err == nil && given["until"] {
		current.EndDate = ""
		if text := strings.TrimSpace(values.until); text != "" && !strings.EqualFold(text, noValue) {
			current.EndDate, err = transaction.ParseDate(text)
		}
	}
	if given["rollover"] {
		current.Rollover = values.rollover
	}
	if err == nil && given["limit"] && current.Currency != "" {
		if current.Limit, err = money.Parse(values.limit, current.Currency); err == nil && current.Limit <= 0 {
			err = errors.New("limit must be more than zero")
		}
	}
	if err != nil {
		return usageError(command + ": " + err.Error())
	}
	return nil
}

// checkPeriod refuses a budget whose period and dates do not go together.
func checkPeriod(command string, current budget.Budget) error {
	switch {
	case !current.Recurring() && current.EndDate == "":
		return usageError(command + ": give a period or an end date")
	case current.EndDate != "" && current.EndDate < current.StartDate:
		return usageError(command + ": end date is before the start date")
	}
	return nil
}

func (c *budgetsCommand) add(args []string) error {
	var values budgetFlags
	_, given, err := parse("add", args, func(f *flag.FlagSet) { values.define(f, true) }, 0)
	if err != nil {
		return err
	}
	var draft budget.Budget
	if err := c.apply("add", values, given, &draft); err != nil {
		return err
	}
	if given["title"] && given["limit"] && given["currency"] && (given["every"] || given["until"]) {
		if !given["start"] {
			draft.StartDate = budget.DefaultStart(draft.Every, draft.Unit, today())
		}
		if err := checkPeriod("add", draft); err != nil {
			return err
		}
		if !given["code"] {
			if draft.Code = code.Suggest(draft.Title, c.taken(0)); draft.Code == "" {
				return usageError(fmt.Sprintf("add: cannot make a code from the title %q; use --code", draft.Title))
			}
		}
	} else {
		if !c.interactive {
			return usageError("add: --title, --limit and --currency are necessary, with --every or --until, when input is not a terminal")
		}
		if draft, err = c.form("New budget", draft, values, given, false); err != nil {
			return err
		}
	}
	if err := c.store.Create(&draft); err != nil {
		return err
	}
	return c.print(ui.Success("Budget "+draft.Code+" created") + "\n")
}

// form opens the form of a budget. given names the options with a final
// value: the form hides their fields.
func (c *budgetsCommand) form(title string, draft budget.Budget, values budgetFlags, given map[string]bool, edit bool) (budget.Budget, error) {
	categories, err := category.NewStore(c.db).List()
	if err != nil {
		return draft, err
	}
	var preset []ui.Field
	for flagName, field := range map[string]ui.Field{
		"title": ui.FieldName, "code": ui.FieldCode, "description": ui.FieldDescription, "currency": ui.FieldCurrency,
		"categories": ui.FieldCategories, "start": ui.FieldStart, "every": ui.FieldEvery, "until": ui.FieldDate, "rollover": ui.FieldRollover,
	} {
		if given[flagName] {
			preset = append(preset, field)
		}
	}
	limit := values.limit
	if edit {
		limit = money.Input(draft.Limit, draft.Currency)
	}
	// The limit is final only with its currency.
	if given["limit"] && given["currency"] {
		preset = append(preset, ui.FieldLimit)
	}
	form, err := program(ui.NewBudgetForm(ui.BudgetFormOptions{
		Title: title, Initial: draft, Limit: limit, Categories: categories, Today: today(),
		Edit: edit, Preset: preset, Taken: c.taken(draft.ID),
	}))
	if err != nil {
		return draft, err
	}
	if !form.Submitted() {
		return draft, cancelled
	}
	return form.Budget(), nil
}

func (c *budgetsCommand) edit(args []string) error {
	var values budgetFlags
	positional, given, err := parse("edit", args, func(f *flag.FlagSet) { values.define(f, false) }, 1)
	if err != nil {
		return err
	}
	// Check the options before the selector opens. The limit waits for
	// the currency of the budget.
	var probe budget.Budget
	if err := c.apply("edit", values, given, &probe); err != nil {
		return err
	}
	if len(given) == 0 && !c.interactive {
		return usageError("edit: give --title, --code, --description, --limit, --categories, --start, --every, --until or --rollover when input is not a terminal")
	}
	current, err := c.resolve("edit", reference(positional), "Edit budget", scopeActive, today())
	if err != nil {
		return err
	}
	if current.Archived() {
		return fmt.Errorf("budget not found: %s (it is archived; restore it first)", current.Code)
	}
	if len(given) == 0 {
		if current, err = c.form("Edit "+current.Code, current, values, given, true); err != nil {
			return err
		}
	}
	if err := c.apply("edit", values, given, &current); err != nil {
		return err
	}
	if err := checkPeriod("edit", current); err != nil {
		return err
	}
	if err := c.store.Update(&current); err != nil {
		return err
	}
	return c.print(ui.Success("Budget "+current.Code+" updated") + "\n")
}

func (c *budgetsCommand) delete(args []string) error {
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
	in, title := scopeActive, "Archive budget"
	if hard {
		in, title = scopeAll, "Delete budget permanently"
	}
	current, err := c.resolve("delete", reference(positional), title, in, today())
	if err != nil {
		return err
	}
	if current.Archived() && !hard {
		return fmt.Errorf("budget %s is already archived; use --hard to delete it permanently", current.Code)
	}
	if !yes {
		question, label := fmt.Sprintf("Archive budget %s (%s)?", current.Code, current.Title), "Archive"
		if hard {
			question, label = fmt.Sprintf("Delete budget %s (%s) permanently?", current.Code, current.Title), "Delete permanently"
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
		return c.print(ui.Success("Budget "+current.Code+" deleted permanently") + "\n")
	}
	if err := c.store.Archive(current.ID); err != nil {
		return err
	}
	return c.print(ui.Success("Budget "+current.Code+" archived") + "\n")
}

func (c *budgetsCommand) restore(args []string) error {
	positional, _, err := parse("restore", args, nil, 1)
	if err != nil {
		return err
	}
	current, err := c.resolve("restore", reference(positional), "Restore budget", scopeArchived, today())
	if err != nil {
		return err
	}
	if !current.Archived() {
		return fmt.Errorf("budget %s is not archived", current.Code)
	}
	if err := c.store.Restore(current.ID); err != nil {
		return err
	}
	return c.print(ui.Success("Budget "+current.Code+" restored") + "\n")
}

// budgetWarning tells that a transaction left a budget near or over its
// limit.
type budgetWarning struct {
	// Budget is the code of the budget; Level is near or over.
	Budget  string `json:"budget"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// budgetWarnings returns one warning for each active budget that the
// transaction counts on and that is near or over its limit, in the
// period of the date of the transaction.
func budgetWarnings(store *budget.Store, view transaction.View) ([]budgetWarning, error) {
	if view.Type != transaction.Expense && view.Type != transaction.Income {
		return nil, nil
	}
	budgets, err := store.Matching(view.CategoryID, view.Account.Currency, view.Date)
	if err != nil {
		return nil, err
	}
	var warnings []budgetWarning
	for _, current := range budgets {
		usage, ok := current.UsageAt(view.Date)
		if !ok || usage.Level() == budget.LevelOK {
			continue
		}
		message := fmt.Sprintf("Budget %s: %d%% spent, %s left", current.Code, usage.Percent(), money.Format(usage.Remaining(), current.Currency))
		if usage.Level() == budget.LevelOver {
			message = fmt.Sprintf("Budget %s: over the limit by %s", current.Code, money.Format(usage.Over(), current.Currency))
		}
		warnings = append(warnings, budgetWarning{
			Budget: current.Code, Level: usage.Level(), Message: message + " until " + transaction.FormatDate(usage.To),
		})
	}
	return warnings, nil
}
