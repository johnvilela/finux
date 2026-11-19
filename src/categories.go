package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"finux/src/budget"
	"finux/src/category"
	"finux/src/code"
	"finux/src/schedule"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

func categoriesHelp() string {
	var colors []string
	for _, color := range category.Colors {
		colors = append(colors, color.Key+" = "+color.Meaning)
	}
	return "Manage categories\n\n" +
		green.Render("Usage:") + " finux categories [command] [arguments]\n" +
		"       finux ct [command] [arguments]\n\n" +
		green.Render("Commands:") + `
  list, l                      List categories; select one to see its details (default)
  <code|id>                    Show the details of a category
  show <code|id>               Same as <code|id>
  add, a [options]             Create a category
  edit, e [code|id] [options]  Change the name, code, description or color
  delete, d [code|id]          Delete a category permanently
  help, --help, -h             Show this help

` + green.Render("Options for add and edit:") + `
  --name <text>         Name of the category
  --code <code>         1 to 5 letters and digits (default: made from the name)
  --description <text>  Description
  --color <name>        ` + strings.Join(category.ColorKeys(), ", ") + `
                        (` + strings.Join(colors, ", ") + `)

` + green.Render("Options for delete:") + `
  --yes                 Do not ask for confirmation

Without options, add and edit open a form.
Without a code or id, edit and delete open a selector.
A command name wins over a category code: use 'finux ct show L' for a
category with the code L.
'finux init' adds the base categories. 'finux reseed' adds the base
categories that are missing.
The color gives the transactions a category can have: red is for expenses,
green for incomes and grey for all types. A color that the transactions of
the category do not accept is refused. When you delete a category, its
transactions go to the category OTHER.
`
}

// categoriesCommand runs one categories subcommand.
type categoriesCommand struct {
	streams
	db    *gorm.DB
	store *category.Store
}

func runCategories(args []string, io streams) int {
	command := &categoriesCommand{streams: io}
	handlers := map[string]func([]string) error{
		"list": command.list, "l": command.list,
		"add": command.add, "a": command.add,
		"show": command.show,
		"edit": command.edit, "e": command.edit,
		"delete": command.delete, "d": command.delete,
	}
	// No argument or an option lists categories. A word that is not a
	// command is the code or id of a category to show.
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
			return write(io, categoriesHelp())
		}
	}
	db, err := openDatabase()
	if err != nil {
		return exitStatus(io, "categories", err)
	}
	announceAutomatic(db, io)
	command.db, command.store = db, category.NewStore(db)
	return exitStatus(io, "categories", handler(rest))
}

func (c *categoriesCommand) print(text string) error {
	_, err := fmt.Fprint(c.stdout, text)
	return err
}

// resolve finds the category for a code or id. Without a reference it
// opens a selector.
func (c *categoriesCommand) resolve(command, ref, title string) (category.Category, error) {
	if ref != "" {
		found, err := c.store.Find(ref)
		if errors.Is(err, category.ErrNotFound) {
			return found, fmt.Errorf("category not found: %s", ref)
		}
		return found, err
	}
	if !c.interactive {
		return category.Category{}, usageError(command + ": a category code or id is necessary when input is not a terminal")
	}
	categories, err := c.store.List()
	if err != nil {
		return category.Category{}, err
	}
	if len(categories) == 0 {
		return category.Category{}, notice("No categories yet. Run 'finux reseed' or 'finux ct add'.")
	}
	index, err := pick(title, "category", "categories", ui.CategoryEntries(categories))
	if err != nil {
		return category.Category{}, err
	}
	return categories[index], nil
}

func (c *categoriesCommand) taken(exceptID uint) func(string) bool {
	return func(value string) bool {
		taken, err := c.store.CodeTaken(value, exceptID)
		return err == nil && taken
	}
}

func (c *categoriesCommand) list(args []string) error {
	if _, _, err := parse("list", args, nil, 0); err != nil {
		return err
	}
	categories, err := c.store.List()
	if err != nil {
		return err
	}
	if len(categories) == 0 || !c.interactive {
		return c.print(ui.CategoryTable(categories, c.plain))
	}
	index, err := pick("Categories", "category", "categories", ui.CategoryEntries(categories))
	if errors.Is(err, cancelled) {
		// Leaving the list is not worth a message.
		return nil
	}
	if err != nil {
		return err
	}
	return c.print(ui.CategoryCard(categories[index], c.plain))
}

func (c *categoriesCommand) show(args []string) error {
	positional, _, err := parse("show", args, nil, 1)
	if err != nil {
		return err
	}
	found, err := c.resolve("show", reference(positional), "Show category")
	if err != nil {
		return err
	}
	return c.print(ui.CategoryCard(found, c.plain))
}

// categoryFlags defines the options of add and edit and returns the
// category that receives their values.
func categoryFlags(flags *flag.FlagSet) *category.Category {
	var draft category.Category
	flags.StringVar(&draft.Name, "name", "", "")
	flags.StringVar(&draft.Code, "code", "", "")
	flags.StringVar(&draft.Description, "description", "", "")
	flags.StringVar(&draft.Color, "color", "", "")
	return &draft
}

// checkCategoryFlags cleans and validates the values of the options
// that were given.
func checkCategoryFlags(command string, draft *category.Category, given map[string]bool) error {
	draft.Name = strings.TrimSpace(draft.Name)
	draft.Code = code.Normalize(draft.Code)
	draft.Color = strings.ToLower(strings.TrimSpace(draft.Color))
	var err error
	switch {
	case given["name"] && draft.Name == "":
		err = errors.New("name is empty")
	case given["code"]:
		err = code.Validate(draft.Code)
	}
	if err == nil && given["color"] {
		if _, ok := category.ColorByKey(draft.Color); !ok {
			err = fmt.Errorf("unknown color %q: use one of %s", draft.Color, strings.Join(category.ColorKeys(), ", "))
		}
	}
	if err != nil {
		return usageError(command + ": " + err.Error())
	}
	return nil
}

// presetFields maps the options that were given to the form fields the
// form must hide.
func presetFields(given map[string]bool) []ui.Field {
	var preset []ui.Field
	for name, field := range map[string]ui.Field{
		"name": ui.FieldName, "code": ui.FieldCode, "description": ui.FieldDescription, "color": ui.FieldColor,
	} {
		if given[name] {
			preset = append(preset, field)
		}
	}
	return preset
}

func (c *categoriesCommand) add(args []string) error {
	var draft *category.Category
	_, given, err := parse("add", args, func(f *flag.FlagSet) { draft = categoryFlags(f) }, 0)
	if err != nil {
		return err
	}
	if err := checkCategoryFlags("add", draft, given); err != nil {
		return err
	}
	if given["name"] && given["color"] {
		if !given["code"] {
			if draft.Code = code.Suggest(draft.Name, c.taken(0)); draft.Code == "" {
				return usageError(fmt.Sprintf("add: cannot make a code from the name %q; use --code", draft.Name))
			}
		}
	} else {
		if !c.interactive {
			return usageError("add: --name and --color are necessary when input is not a terminal")
		}
		form, err := program(ui.NewCategoryForm(ui.CategoryFormOptions{Title: "New category", Initial: *draft, Preset: presetFields(given), Taken: c.taken(0)}))
		if err != nil {
			return err
		}
		if !form.Submitted() {
			return cancelled
		}
		*draft = form.Category()
	}
	if err := c.store.Create(draft); err != nil {
		return err
	}
	return c.print(ui.Success("Category "+draft.Code+" created") + "\n")
}

func (c *categoriesCommand) edit(args []string) error {
	var changes *category.Category
	positional, given, err := parse("edit", args, func(f *flag.FlagSet) { changes = categoryFlags(f) }, 1)
	if err != nil {
		return err
	}
	if err := checkCategoryFlags("edit", changes, given); err != nil {
		return err
	}
	if len(given) == 0 && !c.interactive {
		return usageError("edit: give --name, --code, --description or --color when input is not a terminal")
	}
	current, err := c.resolve("edit", reference(positional), "Edit category")
	if err != nil {
		return err
	}
	if len(given) == 0 {
		form, err := program(ui.NewCategoryForm(ui.CategoryFormOptions{Title: "Edit " + current.Code, Initial: current, Edit: true, Taken: c.taken(current.ID)}))
		if err != nil {
			return err
		}
		if !form.Submitted() {
			return cancelled
		}
		current = form.Category()
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
	// The transactions of the category must accept its color.
	if err := c.accepts(current); err != nil {
		return err
	}
	if err := c.store.Update(&current); err != nil {
		return err
	}
	return c.print(ui.Success("Category "+current.Code+" updated") + "\n")
}

// accepts refuses a color that the transactions of the category cannot
// have.
func (c *categoriesCommand) accepts(current category.Category) error {
	if !transactionsReady(c.db) {
		return nil
	}
	refused, err := transaction.NewStore(c.db).Mismatched(current.ID, current.Color)
	if err != nil {
		return err
	}
	if refused > 0 {
		meaning, _ := category.ColorByKey(current.Color)
		return fmt.Errorf("category %s has %s that a %s (%s) category cannot have", current.Code, plural(refused, "transaction"), meaning.Key, meaning.Meaning)
	}
	return nil
}

// usage counts the transactions of a category and finds the category
// OTHER, which receives them when the category is deleted.
func (c *categoriesCommand) usage(current category.Category) (used int64, other category.Category, err error) {
	if transactionsReady(c.db) {
		if used, err = transaction.NewStore(c.db).CountByCategory(current.ID); err != nil {
			return 0, other, err
		}
	}
	if used == 0 {
		return 0, other, nil
	}
	if current.Code == category.OtherCode {
		return used, other, fmt.Errorf("category %s has %s and receives the transactions of deleted categories: it cannot be deleted", current.Code, plural(used, "transaction"))
	}
	if other, err = otherCategory(c.db); err != nil {
		return used, other, fmt.Errorf("category %s has %s: %w", current.Code, plural(used, "transaction"), err)
	}
	return used, other, nil
}

// remove deletes a category and moves its transactions to other.
func (c *categoriesCommand) remove(current category.Category, used int64, other category.Category) error {
	return c.db.Transaction(func(tx *gorm.DB) error {
		if used > 0 {
			if err := transaction.NewStore(tx).MoveCategory(current.ID, other.ID); err != nil {
				return err
			}
		}
		if transactionsReady(tx) {
			if err := budget.NewStore(tx).DropCategory(current.ID); err != nil {
				return err
			}
		}
		return category.NewStore(tx).Delete(current.ID)
	})
}

// schedulesAllow refuses the delete of a category that a schedule has,
// active or archived: its next transactions need the category.
func (c *categoriesCommand) schedulesAllow(current category.Category) error {
	if !transactionsReady(c.db) {
		return nil
	}
	codes, err := schedule.NewStore(c.db).CodesByCategory(current.ID)
	if err != nil || len(codes) == 0 {
		return err
	}
	noun := "schedules"
	if len(codes) == 1 {
		noun = "schedule"
	}
	return fmt.Errorf("category %s is in the %s %s: change their category or delete them first", current.Code, noun, strings.Join(codes, ", "))
}

// budgetsAllow refuses the delete of a category that is the only
// category of a budget, active or archived. Without it, the budget
// would count all categories.
func (c *categoriesCommand) budgetsAllow(current category.Category) error {
	if !transactionsReady(c.db) {
		return nil
	}
	budgets, err := budget.NewStore(c.db).OnlyCategoryOf(current.ID)
	if err != nil || len(budgets) == 0 {
		return err
	}
	codes := make([]string, len(budgets))
	for i, only := range budgets {
		codes[i] = only.Code
	}
	noun := "budget"
	if len(codes) > 1 {
		noun = "budgets"
	}
	return fmt.Errorf("category %s is the only category of the %s %s: change or delete the %s first", current.Code, noun, strings.Join(codes, ", "), noun)
}

func (c *categoriesCommand) delete(args []string) error {
	var yes bool
	positional, _, err := parse("delete", args, func(f *flag.FlagSet) { f.BoolVar(&yes, "yes", false, "") }, 1)
	if err != nil {
		return err
	}
	if !yes && !c.interactive {
		return usageError("delete: --yes is necessary when input is not a terminal")
	}
	current, err := c.resolve("delete", reference(positional), "Delete category")
	if err != nil {
		return err
	}
	used, other, err := c.usage(current)
	if err != nil {
		return err
	}
	if err := c.budgetsAllow(current); err != nil {
		return err
	}
	if err := c.schedulesAllow(current); err != nil {
		return err
	}
	moved := ""
	if used > 0 {
		moved = "; " + plural(used, "transaction") + " moved to " + other.Code
	}
	if !yes {
		question := fmt.Sprintf("Delete category %s (%s) permanently?", current.Code, current.Name)
		if used > 0 {
			question = fmt.Sprintf("Delete category %s (%s) permanently and move %s to %s?", current.Code, current.Name, plural(used, "transaction"), other.Code)
		}
		confirm, err := program(ui.NewConfirm(question, "Delete permanently", true))
		if err != nil {
			return err
		}
		if !confirm.Confirmed() {
			return cancelled
		}
	}
	if err := c.remove(current, used, other); err != nil {
		return err
	}
	return c.print(ui.Success("Category "+current.Code+" deleted"+moved) + "\n")
}
