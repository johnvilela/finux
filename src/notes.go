package main

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"

	"finux/src/account"
	"finux/src/budget"
	"finux/src/card"
	"finux/src/goal"
	"finux/src/note"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

func notesHelp() string {
	return "Manage notes\n\n" +
		green.Render("Usage:") + " finux notes [command] [arguments]\n" +
		"       finux nt [command] [arguments]\n\n" +
		green.Render("Commands:") + `
  list, l [filters]       List the open notes; select one to see it (default)
  <id>                    Show a note
  show <id>               Same as <id>
  add, a [options]        Create a note
  edit, e [id] [options]  Change a note
  done [id]               Mark a note as done
  reopen [id]             Open a note that is done again
  delete, d [id]          Delete a note permanently
  help, --help, -h        Show this help

` + green.Render("Filters for list:") + `
  --all                 Include the notes that are done
  --due                 Only the notes with a reminder date up to today
  --tag <tag>           Notes that have this tag
  --search <text>       Notes whose title or text contains the text
  --goal <code>         Notes of a goal
  --budget <code>       Notes of a budget
  --account <code>      Notes of an account or a credit card

` + green.Render("Options for add and edit:") + `
  --title <text>        Title, at most ` + strconv.Itoa(note.MaxTitle) + ` characters
  --text <text>         What the note tells, at most ` + strconv.Itoa(note.MaxText) + ` characters
  --remind <date>       Reminder date, DD/MM/YYYY or YYYY/MM/DD; optional.
                        In edit, 'none' removes it
  --tags <tags>         Tags, for example "bank fee"
  --goal <code>         Goal the note is about; 'none' takes it away
  --budget <code>       Budget the note is about; 'none' takes it away
  --account <code>      Account or credit card the note is about; 'none'
                        takes it away

` + green.Render("Options for delete:") + `
  --yes                 Do not ask for confirmation

A note is a short text about your finances: a fact, a decision or a thing to
do. You write it, or an AI agent writes it for you. All its options but the
title are optional.

A note is open until you mark it as done. An open note with a reminder date
up to today is due: 'finux resume' and 'finux tx' tell you, and the Telegram
bot sends one message. The details of a goal, a budget, an account and a
credit card list their open notes.

Without options, add and edit open a form with the title, the text, the
reminder date and the tags. Without an id, edit, done, reopen and delete
open a selector. A note has only an id, not a code.
`
}

// notesCommand runs one notes subcommand.
type notesCommand struct {
	streams
	db    *gorm.DB
	store *note.Store
	// source is who writes the notes: the user, or an agent for the tools.
	source string
}

func newNotesCommand(db *gorm.DB, io streams) *notesCommand {
	return &notesCommand{streams: io, db: db, store: note.NewStore(db), source: note.SourceUser}
}

func runNotes(args []string, io streams) int {
	handlers := map[string]func(*notesCommand, []string) error{
		"list": (*notesCommand).list, "l": (*notesCommand).list,
		"add": (*notesCommand).add, "a": (*notesCommand).add,
		"show": (*notesCommand).show,
		"edit": (*notesCommand).edit, "e": (*notesCommand).edit,
		"done":   func(c *notesCommand, args []string) error { return c.mark("done", args, true) },
		"reopen": func(c *notesCommand, args []string) error { return c.mark("reopen", args, false) },
		"delete": (*notesCommand).delete, "d": (*notesCommand).delete,
	}
	// No argument or an option lists notes. A word that is not a command
	// is the id of a note to show.
	handler, rest := (*notesCommand).list, args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if known, ok := handlers[args[0]]; ok {
			handler, rest = known, args[1:]
		} else {
			handler = (*notesCommand).show
		}
	}
	// Help needs no database, so look for it before the database opens.
	for i, arg := range rest {
		if arg == "-h" || arg == "--help" || i == 0 && isHelp(arg) {
			return write(io, notesHelp())
		}
	}
	db, err := openLedger(io)
	if err != nil {
		return exitStatus(io, "notes", err)
	}
	defer closeDatabase(db)
	return exitStatus(io, "notes", handler(newNotesCommand(db, io), rest))
}

func (c *notesCommand) print(text string) error {
	_, err := fmt.Fprint(c.stdout, text)
	return err
}

// noteViews loads the records that notes link to. A record that is
// archived still shows.
func noteViews(db *gorm.DB, notes []note.Note) ([]ui.NoteView, error) {
	views := make([]ui.NoteView, len(notes))
	goals, budgets := goal.NewStore(db), budget.NewStore(db)
	accounts, cards := account.NewStore(db), card.NewStore(db)
	id := func(value uint) string { return strconv.FormatUint(uint64(value), 10) }
	for i, current := range notes {
		view := ui.NoteView{Note: current}
		if current.GoalID != 0 {
			if found, err := goals.Find(id(current.GoalID), true); err == nil {
				view.Goal = &found
			} else if !errors.Is(err, goal.ErrNotFound) {
				return nil, err
			}
		}
		if current.BudgetID != 0 {
			if found, err := budgets.Find(id(current.BudgetID), true, today()); err == nil {
				view.Budget = &found
			} else if !errors.Is(err, budget.ErrNotFound) {
				return nil, err
			}
		}
		if current.AccountID != 0 {
			if found, err := accounts.Find(id(current.AccountID), true, today()); err == nil {
				view.Place = &found
			} else if !errors.Is(err, account.ErrNotFound) {
				return nil, err
			}
		}
		if current.CardID != 0 {
			if owner, err := cards.Find(id(current.CardID), true); err == nil {
				view.Place, view.OnCard = &account.Account{Name: owner.Name, Code: owner.Code, Color: owner.Color, Currency: owner.Currency}, true
			} else if !errors.Is(err, card.ErrNotFound) {
				return nil, err
			}
		}
		views[i] = view
	}
	return views, nil
}

// openNotes returns the section of the open notes of one record, for
// the end of its details. link is one of the links of a note.
func openNotes(db *gorm.DB, link string, id uint, plain bool) (string, error) {
	filter := note.Filter{}
	switch link {
	case note.LinkGoal:
		filter.GoalID = id
	case note.LinkBudget:
		filter.BudgetID = id
	case note.LinkAccount:
		filter.AccountID = id
	case note.LinkCard:
		filter.CardID = id
	}
	notes, err := note.NewStore(db).List(filter, today())
	if err != nil {
		return "", err
	}
	return ui.NoteSection(notes, today(), plain), nil
}

// dueNotes returns one line that tells how many notes are due, or ""
// when none is.
func dueNotes(store *note.Store) (string, error) {
	notes, err := store.List(note.Filter{Status: note.StatusDue}, today())
	if err != nil || len(notes) == 0 {
		return "", err
	}
	if len(notes) == 1 {
		return ui.Warning("1 note is due. Run 'finux nt'.") + "\n", nil
	}
	return ui.Warning(fmt.Sprintf("%d notes are due. Run 'finux nt'.", len(notes))) + "\n", nil
}

// find returns the note for the text of an id.
func (c *notesCommand) find(ref string) (note.Note, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(ref), 10, 32)
	if err != nil {
		return note.Note{}, fmt.Errorf("note not found: %s", ref)
	}
	found, err := c.store.Find(uint(id))
	if errors.Is(err, note.ErrNotFound) {
		return found, fmt.Errorf("note not found: %s", ref)
	}
	return found, err
}

// resolve finds the note for an id. Without an id it opens a selector
// with the notes of the filter.
func (c *notesCommand) resolve(command, ref, title string, filter note.Filter) (note.Note, error) {
	if ref != "" {
		return c.find(ref)
	}
	if !c.interactive {
		return note.Note{}, usageError(command + ": a note id is necessary when input is not a terminal")
	}
	notes, err := c.store.List(filter, today())
	if err != nil {
		return note.Note{}, err
	}
	if len(notes) == 0 {
		return note.Note{}, notice("No notes to choose from.")
	}
	views, err := noteViews(c.db, notes)
	if err != nil {
		return note.Note{}, err
	}
	index, err := pick(title, "note", "notes", ui.NoteEntries(views, today()))
	if err != nil {
		return note.Note{}, err
	}
	return notes[index], nil
}

func (c *notesCommand) card(current note.Note) (string, error) {
	views, err := noteViews(c.db, []note.Note{current})
	if err != nil {
		return "", err
	}
	return ui.NoteCard(views[0], today(), c.plain), nil
}

// links finds the records that the text of the link options names.
// 'none' and an empty text give no record.
type noteLinks struct {
	goal, budget, account string
}

// apply writes the links that were given to a note.
func (c *notesCommand) link(values noteLinks, given map[string]bool, current *note.Note) error {
	tx := newTransactionsCommand(c.db)
	if given["goal"] {
		current.GoalID = 0
		if !isNone(values.goal) {
			found, err := tx.findGoal(values.goal)
			if err != nil {
				return err
			}
			current.GoalID = found.ID
		}
	}
	if given["budget"] {
		current.BudgetID = 0
		if !isNone(values.budget) {
			found, err := budget.NewStore(c.db).Find(values.budget, true, today())
			if errors.Is(err, budget.ErrNotFound) {
				return fmt.Errorf("budget not found: %s", values.budget)
			}
			if err != nil {
				return err
			}
			current.BudgetID = found.ID
		}
	}
	if given["account"] {
		current.AccountID, current.CardID = 0, 0
		if !isNone(values.account) {
			found, cardID, err := tx.findPlace(values.account, true)
			if err != nil {
				return err
			}
			if current.AccountID, current.CardID = found.ID, cardID; cardID != 0 {
				current.AccountID = 0
			}
		}
	}
	return nil
}

func (c *notesCommand) list(args []string) error {
	var all, due bool
	var tag, search string
	var links noteLinks
	_, given, err := parse("list", args, func(f *flag.FlagSet) {
		f.BoolVar(&all, "all", false, "")
		f.BoolVar(&due, "due", false, "")
		f.StringVar(&tag, "tag", "", "")
		f.StringVar(&search, "search", "", "")
		f.StringVar(&links.goal, "goal", "", "")
		f.StringVar(&links.budget, "budget", "", "")
		f.StringVar(&links.account, "account", "", "")
	}, 0)
	if err != nil {
		return err
	}
	var linked note.Note
	if err := c.link(links, given, &linked); err != nil {
		return err
	}
	filter := note.Filter{Tag: tag, Search: search, GoalID: linked.GoalID, BudgetID: linked.BudgetID, AccountID: linked.AccountID, CardID: linked.CardID}
	switch {
	case due:
		filter.Status = note.StatusDue
	case all:
		filter.Status = note.StatusAll
	}
	notes, err := c.store.List(filter, today())
	if err != nil {
		return err
	}
	views, err := noteViews(c.db, notes)
	if err != nil {
		return err
	}
	delete(given, "all")
	if len(views) == 0 || !c.interactive {
		return c.print(ui.NoteTable(views, today(), len(given) > 0, c.plain))
	}
	index, err := pick("Notes", "note", "notes", ui.NoteEntries(views, today()))
	if errors.Is(err, cancelled) {
		// Leaving the list is not worth a message.
		return nil
	}
	if err != nil {
		return err
	}
	return c.print(ui.NoteCard(views[index], today(), c.plain))
}

func (c *notesCommand) show(args []string) error {
	positional, _, err := parse("show", args, nil, 1)
	if err != nil {
		return err
	}
	found, err := c.resolve("show", reference(positional), "Show note", note.Filter{Status: note.StatusAll})
	if err != nil {
		return err
	}
	text, err := c.card(found)
	if err != nil {
		return err
	}
	return c.print(text)
}

// noteFlags holds the text of the options of add and edit.
type noteFlags struct {
	title, text, remind, tags string
	links                     noteLinks
}

func (v *noteFlags) define(flags *flag.FlagSet) {
	flags.StringVar(&v.title, "title", "", "")
	flags.StringVar(&v.text, "text", "", "")
	flags.StringVar(&v.remind, "remind", "", "")
	flags.StringVar(&v.tags, "tags", "", "")
	flags.StringVar(&v.links.goal, "goal", "", "")
	flags.StringVar(&v.links.budget, "budget", "", "")
	flags.StringVar(&v.links.account, "account", "", "")
}

// apply validates the options that were given and writes them to the note.
func (c *notesCommand) apply(command string, values noteFlags, given map[string]bool, current *note.Note) error {
	var err error
	if given["title"] {
		if current.Title = strings.TrimSpace(values.title); current.Title == "" {
			err = errors.New("title is empty")
		}
	}
	if given["text"] {
		current.Text = strings.TrimSpace(values.text)
	}
	if err == nil && given["remind"] {
		current.RemindOn = ""
		if !isNone(values.remind) {
			current.RemindOn, err = transaction.ParseDate(values.remind)
		}
	}
	if err == nil && given["tags"] {
		current.Tags, err = transaction.ParseTags(values.tags)
	}
	if err != nil {
		return usageError(command + ": " + err.Error())
	}
	return c.link(values.links, given, current)
}

// noteFields are the options that the form of a note has.
var noteFields = map[string]ui.Field{"title": ui.FieldName, "text": ui.FieldDescription, "remind": ui.FieldDate, "tags": ui.FieldTags}

func (c *notesCommand) add(args []string) error {
	var values noteFlags
	_, given, err := parse("add", args, values.define, 0)
	if err != nil {
		return err
	}
	draft := note.Note{Source: c.source}
	if err := c.apply("add", values, given, &draft); err != nil {
		return err
	}
	if !given["title"] {
		if !c.interactive {
			return usageError("add: --title is necessary when input is not a terminal")
		}
		var preset []ui.Field
		for name, field := range noteFields {
			if given[name] {
				preset = append(preset, field)
			}
		}
		form, err := program(ui.NewNoteForm(ui.NoteFormOptions{Initial: draft, Preset: preset}))
		if err != nil {
			return err
		}
		if !form.Submitted() {
			return cancelled
		}
		draft = form.Note()
	}
	if err := draft.Validate(); err != nil {
		return usageError("add: " + err.Error())
	}
	if err := c.store.Create(&draft); err != nil {
		return err
	}
	return c.print(ui.Success(fmt.Sprintf("Note %d created", draft.ID)) + "\n")
}

func (c *notesCommand) edit(args []string) error {
	var values noteFlags
	positional, given, err := parse("edit", args, values.define, 1)
	if err != nil {
		return err
	}
	if len(given) == 0 && !c.interactive {
		return usageError("edit: give --title, --text, --remind, --tags, --goal, --budget or --account when input is not a terminal")
	}
	current, err := c.resolve("edit", reference(positional), "Edit note", note.Filter{Status: note.StatusAll})
	if err != nil {
		return err
	}
	if len(given) == 0 {
		form, err := program(ui.NewNoteForm(ui.NoteFormOptions{Title: fmt.Sprintf("Edit note %d", current.ID), Initial: current, Edit: true}))
		if err != nil {
			return err
		}
		if !form.Submitted() {
			return cancelled
		}
		current = form.Note()
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
	return c.print(ui.Success(fmt.Sprintf("Note %d updated", current.ID)) + "\n")
}

// mark makes a note done, or open again.
func (c *notesCommand) mark(command string, args []string, done bool) error {
	positional, _, err := parse(command, args, nil, 1)
	if err != nil {
		return err
	}
	filter, title, result := note.Filter{}, "Mark note as done", "done"
	if !done {
		filter, title, result = note.Filter{Status: note.StatusDone}, "Open note again", "open again"
	}
	current, err := c.resolve(command, reference(positional), title, filter)
	if err != nil {
		return err
	}
	switch {
	case done && current.Done():
		return fmt.Errorf("note %d is already done", current.ID)
	case !done && !current.Done():
		return fmt.Errorf("note %d is not done", current.ID)
	}
	if err := c.store.SetDone(current.ID, done); err != nil {
		return err
	}
	return c.print(ui.Success(fmt.Sprintf("Note %d %s", current.ID, result)) + "\n")
}

func (c *notesCommand) delete(args []string) error {
	var yes bool
	positional, _, err := parse("delete", args, func(f *flag.FlagSet) { f.BoolVar(&yes, "yes", false, "") }, 1)
	if err != nil {
		return err
	}
	if !yes && !c.interactive {
		return usageError("delete: --yes is necessary when input is not a terminal")
	}
	current, err := c.resolve("delete", reference(positional), "Delete note", note.Filter{Status: note.StatusAll})
	if err != nil {
		return err
	}
	if !yes {
		confirm, err := program(ui.NewConfirm(fmt.Sprintf("Delete note %d (%s) permanently?", current.ID, current.Title), "Delete permanently", true))
		if err != nil {
			return err
		}
		if !confirm.Confirmed() {
			return cancelled
		}
	}
	if err := c.store.Delete(current.ID); err != nil {
		return err
	}
	return c.print(ui.Success(fmt.Sprintf("Note %d deleted", current.ID)) + "\n")
}
