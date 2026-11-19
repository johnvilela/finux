package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"finux/src/code"
	"finux/src/goal"
	"finux/src/money"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

// noGoal is the word that stands for no goal and for no final date in
// an option. A goal cannot have it as its code.
const noGoal = "none"

func goalsHelp() string {
	return "Manage goals\n\n" +
		green.Render("Usage:") + " finux goals [command] [arguments]\n" +
		"       finux gl [command] [arguments]\n\n" +
		green.Render("Commands:") + `
  list, l [--all]              List goals; select one to see its details (default)
  <code|id>                    Show a goal, its progress and its transactions
  show <code|id>               Same as <code|id>
  add, a [options]             Create a goal
  edit, e [code|id] [options]  Change a goal
  delete, d [code|id]          Archive a goal
  restore, r [code|id]         Restore an archived goal
  help, --help, -h             Show this help

` + green.Render("Options for list:") + `
  --all                 Include archived goals

` + green.Render("Options for add and edit:") + `
  --title <text>        Title of the goal, at most ` + fmt.Sprint(goal.MaxTitle) + ` characters
  --code <code>         1 to 5 letters and digits (default: made from the title)
  --description <text>  Description, at most ` + fmt.Sprint(goal.MaxDescription) + ` characters
  --type <type>         ` + strings.Join(goal.Types(), ", ") + `
  --target <amount>     Amount to reach, for example 10000
  --currency <code>     ` + strings.Join(money.Codes(), ", ") + ` (add only)
  --due <date>          Final date, DD/MM/YYYY or YYYY/MM/DD; optional.
                        In edit, 'none' removes it

` + green.Render("Options for delete:") + `
  --hard                Delete permanently; also works on archived goals
  --yes                 Do not ask for confirmation

Without options, add and edit open a form.
Without a code or id, edit, delete and restore open a selector.

A goal gets its progress from the transactions you link to it:
'finux tx add ... --goal <code>' or 'finux tx edit <id> --goal <code>'.
'--goal none' takes the goal away. A transaction has at most one goal, in
the currency of the goal.

The type tells how a linked transaction counts:
  save, purchase   an income adds, an expense removes
  payoff           an expense adds, an income removes
A transfer adds its value one time. The payment of a credit card bill
cannot have a goal.

A goal is active, then reached when the progress is the target or more. A
goal with a final date in the past that is not reached is overdue.

An archived goal keeps its transactions and takes no new ones. A permanent
delete never deletes a transaction: the transactions only lose the goal.
`
}

// goalsCommand runs one goals subcommand.
type goalsCommand struct {
	streams
	db    *gorm.DB
	store *goal.Store
}

func runGoals(args []string, io streams) int {
	command := &goalsCommand{streams: io}
	handlers := map[string]func([]string) error{
		"list": command.list, "l": command.list,
		"add": command.add, "a": command.add,
		"show": command.show,
		"edit": command.edit, "e": command.edit,
		"delete": command.delete, "d": command.delete,
		"restore": command.restore, "r": command.restore,
	}
	// No argument or an option lists goals. A word that is not a command
	// is the code or id of a goal to show.
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
			return write(io, goalsHelp())
		}
	}
	db, err := openDatabase()
	if err == nil && !transactionsReady(db) {
		err = errOutdated
	}
	if err != nil {
		return exitStatus(io, "goals", err)
	}
	command.db, command.store = db, goal.NewStore(db)
	return exitStatus(io, "goals", handler(rest))
}

func (c *goalsCommand) print(text string) error {
	_, err := fmt.Fprint(c.stdout, text)
	return err
}

// resolve finds the goal for a code or id. Without a reference it opens
// a selector with the goals of the scope.
func (c *goalsCommand) resolve(command, ref, title string, in scope) (goal.Goal, error) {
	if ref != "" {
		found, err := c.store.Find(ref, true)
		if errors.Is(err, goal.ErrNotFound) {
			return found, fmt.Errorf("goal not found: %s", ref)
		}
		return found, err
	}
	if !c.interactive {
		return goal.Goal{}, usageError(command + ": a goal code or id is necessary when input is not a terminal")
	}
	var goals []goal.Goal
	var err error
	empty := notice("No goals yet. Run 'finux gl add'.")
	switch in {
	case scopeActive:
		goals, err = c.store.List(false)
	case scopeAll:
		goals, err = c.store.List(true)
	case scopeArchived:
		goals, err = c.store.ListArchived()
		empty = "No archived goals."
	}
	if err != nil {
		return goal.Goal{}, err
	}
	if len(goals) == 0 {
		return goal.Goal{}, empty
	}
	index, err := pick(title, "goal", "goals", ui.GoalEntries(goals, today()))
	if err != nil {
		return goal.Goal{}, err
	}
	return goals[index], nil
}

func (c *goalsCommand) taken(exceptID uint) func(string) bool {
	return func(code string) bool {
		taken, err := c.store.CodeTaken(code, exceptID)
		return err == nil && taken
	}
}

// gather reads a goal with the transactions linked to it.
func (c *goalsCommand) gather(current goal.Goal) (ui.GoalDetails, error) {
	details := ui.GoalDetails{Goal: current, Today: today()}
	var err error
	details.Transactions, err = transaction.NewStore(c.db).ByGoal(current.ID)
	return details, err
}

func (c *goalsCommand) details(current goal.Goal) (string, error) {
	details, err := c.gather(current)
	if err != nil {
		return "", err
	}
	return ui.GoalDetailsView(details, c.plain), nil
}

func (c *goalsCommand) list(args []string) error {
	var all bool
	if _, _, err := parse("list", args, func(f *flag.FlagSet) { f.BoolVar(&all, "all", false, "") }, 0); err != nil {
		return err
	}
	goals, err := c.store.List(all)
	if err != nil {
		return err
	}
	if len(goals) == 0 || !c.interactive {
		return c.print(ui.GoalTable(goals, today(), c.plain))
	}
	index, err := pick("Goals", "goal", "goals", ui.GoalEntries(goals, today()))
	if errors.Is(err, cancelled) {
		// Leaving the list is not worth a message.
		return nil
	}
	if err != nil {
		return err
	}
	text, err := c.details(goals[index])
	if err != nil {
		return err
	}
	return c.print(text)
}

func (c *goalsCommand) show(args []string) error {
	positional, _, err := parse("show", args, nil, 1)
	if err != nil {
		return err
	}
	found, err := c.resolve("show", reference(positional), "Show goal", scopeAll)
	if err != nil {
		return err
	}
	text, err := c.details(found)
	if err != nil {
		return err
	}
	return c.print(text)
}

// goalFlags holds the text of the options of add and edit.
type goalFlags struct {
	title, code, description, kind, target, currency, due string
}

func (v *goalFlags) define(flags *flag.FlagSet, create bool) {
	flags.StringVar(&v.title, "title", "", "")
	flags.StringVar(&v.code, "code", "", "")
	flags.StringVar(&v.description, "description", "", "")
	flags.StringVar(&v.kind, "type", "", "")
	flags.StringVar(&v.target, "target", "", "")
	flags.StringVar(&v.due, "due", "", "")
	if create {
		flags.StringVar(&v.currency, "currency", "", "")
	}
}

// goalType reads the type of a goal. "pay off" and "pay-off" are payoff.
func goalType(text string) string {
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(text)))
}

// apply validates the options that were given and writes them to the
// goal. The target needs the currency of the goal.
func (v *goalFlags) apply(command string, given map[string]bool, current *goal.Goal) error {
	var err error
	if given["title"] {
		if current.Title = strings.TrimSpace(v.title); current.Title == "" {
			err = errors.New("title is empty")
		}
	}
	if err == nil && given["code"] {
		current.Code = code.Normalize(v.code)
		if err = code.Validate(current.Code); err == nil && strings.EqualFold(current.Code, noGoal) {
			err = fmt.Errorf("the code %s is reserved", current.Code)
		}
	}
	if given["description"] {
		current.Description = strings.TrimSpace(v.description)
	}
	if err == nil && given["type"] {
		if current.Type = goalType(v.kind); !goal.ValidType(current.Type) {
			err = fmt.Errorf("unknown type %q: use one of %s", v.kind, strings.Join(goal.Types(), ", "))
		}
	}
	if err == nil && given["currency"] {
		if current.Currency = strings.ToUpper(strings.TrimSpace(v.currency)); !money.Valid(current.Currency) {
			err = fmt.Errorf("unknown currency %q: use one of %s", current.Currency, strings.Join(money.Codes(), ", "))
		}
	}
	if err == nil && given["due"] {
		current.DueDate = ""
		if text := strings.TrimSpace(v.due); text != "" && !strings.EqualFold(text, noGoal) {
			current.DueDate, err = transaction.ParseDate(text)
		}
	}
	if err == nil && given["target"] && current.Currency != "" {
		if current.Target, err = money.Parse(v.target, current.Currency); err == nil && current.Target <= 0 {
			err = errors.New("target must be more than zero")
		}
	}
	if err != nil {
		return usageError(command + ": " + err.Error())
	}
	return nil
}

// suggest makes the code of a new goal from its title.
func (c *goalsCommand) suggest(title string) string {
	taken := c.taken(0)
	return code.Suggest(title, func(value string) bool { return strings.EqualFold(value, noGoal) || taken(value) })
}

func (c *goalsCommand) add(args []string) error {
	var values goalFlags
	_, given, err := parse("add", args, func(f *flag.FlagSet) { values.define(f, true) }, 0)
	if err != nil {
		return err
	}
	var draft goal.Goal
	if err := values.apply("add", given, &draft); err != nil {
		return err
	}
	if given["title"] && given["type"] && given["target"] && given["currency"] {
		if !given["code"] {
			if draft.Code = c.suggest(draft.Title); draft.Code == "" {
				return usageError(fmt.Sprintf("add: cannot make a code from the title %q; use --code", draft.Title))
			}
		}
	} else {
		if !c.interactive {
			return usageError("add: --title, --type, --target and --currency are necessary when input is not a terminal")
		}
		var preset []ui.Field
		for flagName, field := range map[string]ui.Field{
			"title": ui.FieldName, "code": ui.FieldCode, "description": ui.FieldDescription, "type": ui.FieldKind,
			"currency": ui.FieldCurrency, "due": ui.FieldDate,
		} {
			if given[flagName] {
				preset = append(preset, field)
			}
		}
		// The target is final only with its currency.
		if given["target"] && given["currency"] {
			preset = append(preset, ui.FieldTarget)
		}
		form, err := program(ui.NewGoalForm(ui.GoalFormOptions{Title: "New goal", Initial: draft, Target: values.target, Preset: preset, Taken: c.taken(0)}))
		if err != nil {
			return err
		}
		if !form.Submitted() {
			return cancelled
		}
		draft = form.Goal()
	}
	if err := c.store.Create(&draft); err != nil {
		return err
	}
	return c.print(ui.Success("Goal "+draft.Code+" created") + "\n")
}

func (c *goalsCommand) edit(args []string) error {
	var values goalFlags
	positional, given, err := parse("edit", args, func(f *flag.FlagSet) { values.define(f, false) }, 1)
	if err != nil {
		return err
	}
	// Check the options before the selector opens. The target waits for
	// the currency of the goal.
	var probe goal.Goal
	if err := values.apply("edit", given, &probe); err != nil {
		return err
	}
	if len(given) == 0 && !c.interactive {
		return usageError("edit: give --title, --code, --description, --type, --target or --due when input is not a terminal")
	}
	current, err := c.resolve("edit", reference(positional), "Edit goal", scopeActive)
	if err != nil {
		return err
	}
	if current.Archived() {
		return fmt.Errorf("goal not found: %s (it is archived; restore it first)", current.Code)
	}
	if len(given) == 0 {
		form, err := program(ui.NewGoalForm(ui.GoalFormOptions{
			Title: "Edit " + current.Code, Initial: current, Target: money.Input(current.Target, current.Currency), Edit: true, Taken: c.taken(current.ID),
		}))
		if err != nil {
			return err
		}
		if !form.Submitted() {
			return cancelled
		}
		current = form.Goal()
	}
	if err := values.apply("edit", given, &current); err != nil {
		return err
	}
	if err := c.store.Update(&current); err != nil {
		return err
	}
	return c.print(ui.Success("Goal "+current.Code+" updated") + "\n")
}

// lostLinks tells how many transactions lose the link in a permanent
// delete, for a message: "; 2 transactions lose the link".
func lostLinks(current goal.Goal, verb string) string {
	if current.Links == 0 {
		return ""
	}
	return "; " + plural(current.Links, "transaction") + " " + verb + " the link"
}

func (c *goalsCommand) delete(args []string) error {
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
	in, title := scopeActive, "Archive goal"
	if hard {
		in, title = scopeAll, "Delete goal permanently"
	}
	current, err := c.resolve("delete", reference(positional), title, in)
	if err != nil {
		return err
	}
	if current.Archived() && !hard {
		return fmt.Errorf("goal %s is already archived; use --hard to delete it permanently", current.Code)
	}
	if !yes {
		question, label := fmt.Sprintf("Archive goal %s (%s)?", current.Code, current.Title), "Archive"
		if hard {
			question, label = fmt.Sprintf("Delete goal %s (%s) permanently%s?", current.Code, current.Title, lostLinks(current, "lose")), "Delete permanently"
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
		if err := c.purge(current); err != nil {
			return err
		}
		return c.print(ui.Success("Goal "+current.Code+" deleted permanently"+lostLinks(current, "lost")) + "\n")
	}
	if err := c.store.Archive(current.ID); err != nil {
		return err
	}
	return c.print(ui.Success("Goal "+current.Code+" archived") + "\n")
}

// purge deletes a goal in one step. Its transactions stay; they lose
// the link to the goal.
func (c *goalsCommand) purge(current goal.Goal) error {
	return c.db.Transaction(func(tx *gorm.DB) error {
		if err := transaction.NewStore(tx).ClearGoal(current.ID); err != nil {
			return err
		}
		return goal.NewStore(tx).Delete(current.ID)
	})
}

func (c *goalsCommand) restore(args []string) error {
	positional, _, err := parse("restore", args, nil, 1)
	if err != nil {
		return err
	}
	current, err := c.resolve("restore", reference(positional), "Restore goal", scopeArchived)
	if err != nil {
		return err
	}
	if !current.Archived() {
		return fmt.Errorf("goal %s is not archived", current.Code)
	}
	if err := c.store.Restore(current.ID); err != nil {
		return err
	}
	return c.print(ui.Success("Goal "+current.Code+" restored") + "\n")
}
