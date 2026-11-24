package main

import (
	"errors"
	"fmt"
	"strconv"

	"finux/src/mcp"
	"finux/src/note"
	"finux/src/ui"

	"gorm.io/gorm"
)

// defaultNotes is the number of notes of one page.
const defaultNotes = 50

// noteJSON is a note in a result. A field with no value is absent.
type noteJSON struct {
	ID       uint     `json:"id"`
	Title    string   `json:"title"`
	Text     string   `json:"text,omitempty"`
	Status   string   `json:"status"`
	RemindOn string   `json:"remind_on,omitempty"`
	Tags     []string `json:"tags"`
	Goal     string   `json:"goal,omitempty"`
	Budget   string   `json:"budget,omitempty"`
	// Account is the code of the account. A note of a credit card has
	// CreditCard in its place.
	Account    string `json:"account,omitempty"`
	CreditCard string `json:"credit_card,omitempty"`
	Source     string `json:"source"`
	Created    string `json:"created"`
	Done       string `json:"done,omitempty"`
}

func noteResult(view ui.NoteView) noteJSON {
	result := noteJSON{
		ID: view.ID, Title: view.Title, Text: view.Text, Status: view.Status(today()), RemindOn: view.RemindOn,
		Tags: append([]string{}, view.TagList()...), Source: view.Source, Created: view.CreatedAt.Local().Format("2006-01-02"),
	}
	if view.Goal != nil {
		result.Goal = view.Goal.Code
	}
	if view.Budget != nil {
		result.Budget = view.Budget.Code
	}
	switch {
	case view.Place != nil && view.OnCard:
		result.CreditCard = view.Place.Code
	case view.Place != nil:
		result.Account = view.Place.Code
	}
	if view.DoneAt != nil {
		result.Done = view.DoneAt.Local().Format("2006-01-02")
	}
	return result
}

func noteResults(db *gorm.DB, notes []note.Note) ([]noteJSON, error) {
	views, err := noteViews(db, notes)
	if err != nil {
		return nil, err
	}
	results := make([]noteJSON, len(views))
	for i, view := range views {
		results[i] = noteResult(view)
	}
	return results, nil
}

// linkedNotes returns the open notes of one record, for the result of
// the tool that shows the record. link is one of the links of a note.
func linkedNotes(db *gorm.DB, link string, id uint) ([]noteJSON, error) {
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
		return nil, err
	}
	return noteResults(db, notes)
}

// reload returns a note as a result, read again from the database.
func (c *notesCommand) reload(id uint) (map[string]any, error) {
	found, err := c.store.Find(id)
	if err != nil {
		return nil, err
	}
	results, err := noteResults(c.db, []note.Note{found})
	if err != nil {
		return nil, err
	}
	return map[string]any{"note": results[0]}, nil
}

func noteTools() []mcp.Tool {
	type referenceArguments struct {
		ID uint `json:"id" mcp:"Id of the note"`
	}
	type listArguments struct {
		Status  *string `json:"status,omitempty" mcp:"Default: open, which has the due notes too. due is an open note with a reminder date up to today" enum:"open,due,done,all"`
		Tag     *string `json:"tag,omitempty" mcp:"Notes that have this tag"`
		Search  *string `json:"search,omitempty" mcp:"Text that the title or the text of the note contains"`
		Goal    *string `json:"goal,omitempty" mcp:"Code or id of a goal: the notes about it"`
		Budget  *string `json:"budget,omitempty" mcp:"Code or id of a budget: the notes about it"`
		Account *string `json:"account,omitempty" mcp:"Code or id of an account or a credit card: the notes about it"`
		Source  *string `json:"source,omitempty" mcp:"Notes that the user wrote, or the ones an agent wrote" enum:"user,agent"`
		Limit   *int    `json:"limit,omitempty" mcp:"Most notes to return. Default: 50"`
		Offset  *int    `json:"offset,omitempty" mcp:"Number of notes to skip, to read the next page. Default: 0"`
	}
	type addArguments struct {
		Title    string  `json:"title" mcp:"Title, at most 60 characters: 6 words at most, in the words of the user"`
		Text     *string `json:"text,omitempty" mcp:"What the note tells, at most 500 characters: 1 to 3 short sentences with the facts and the numbers. No advice and no greeting"`
		RemindOn *string `json:"remind_on,omitempty" mcp:"Reminder date, YYYY-MM-DD or DD/MM/YYYY. Give it only when the user must do or check something on a date"`
		Tags     *string `json:"tags,omitempty" mcp:"Tags divided by spaces, for example \"bank fee\". Prefer the tags the user already has"`
		Goal     *string `json:"goal,omitempty" mcp:"Code or id of the goal the note is about"`
		Budget   *string `json:"budget,omitempty" mcp:"Code or id of the budget the note is about"`
		Account  *string `json:"account,omitempty" mcp:"Code or id of the account or the credit card the note is about"`
	}
	type editArguments struct {
		ID       uint    `json:"id" mcp:"Id of the note"`
		Title    *string `json:"title,omitempty" mcp:"New title"`
		Text     *string `json:"text,omitempty" mcp:"New text. An empty string removes it"`
		RemindOn *string `json:"remind_on,omitempty" mcp:"New reminder date. \"none\" removes it"`
		Tags     *string `json:"tags,omitempty" mcp:"New tags. An empty string removes all tags"`
		Goal     *string `json:"goal,omitempty" mcp:"Code or id of the new goal. \"none\" takes the goal away"`
		Budget   *string `json:"budget,omitempty" mcp:"Code or id of the new budget. \"none\" takes the budget away"`
		Account  *string `json:"account,omitempty" mcp:"Code or id of the new account or credit card. \"none\" takes it away"`
	}
	type doneArguments struct {
		ID   uint  `json:"id" mcp:"Id of the note"`
		Done *bool `json:"done,omitempty" mcp:"Default: true. False opens the note again"`
	}
	type deleteArguments struct {
		ID      uint `json:"id" mcp:"Id of the note"`
		Confirm bool `json:"confirm,omitempty" mcp:"Set to true to delete. Without it the tool only tells what it will delete."`
	}
	// open makes the command of a tool: its notes have the agent as
	// their source.
	open := func(db *gorm.DB) *notesCommand {
		c := newNotesCommand(db, quiet)
		c.source = note.SourceAgent
		return c
	}
	ref := func(id uint) string { return strconv.FormatUint(uint64(id), 10) }
	return []mcp.Tool{
		reads(tool("note_list", "List the notes: short texts about the finances that the user or an agent wrote, with an optional reminder date. The due notes come first. The result has due: the number of open notes with a reminder date up to today. Read the due notes at the start of a conversation and tell the user about them.",
			func(db *gorm.DB, a listArguments) (any, error) {
				c := open(db)
				var linked note.Note
				links := noteLinks{goal: text(a.Goal), budget: text(a.Budget), account: text(a.Account)}
				if err := c.link(links, given(map[string]bool{"goal": a.Goal != nil, "budget": a.Budget != nil, "account": a.Account != nil}), &linked); err != nil {
					return nil, err
				}
				filter := note.Filter{
					Status: text(a.Status), Tag: text(a.Tag), Search: text(a.Search), Source: text(a.Source),
					GoalID: linked.GoalID, BudgetID: linked.BudgetID, AccountID: linked.AccountID, CardID: linked.CardID,
				}
				switch filter.Status {
				case "", note.StatusOpen, note.StatusDue, note.StatusDone, note.StatusAll:
				default:
					return nil, fmt.Errorf("unknown status %q: use open, due, done or all", filter.Status)
				}
				notes, err := c.store.List(filter, today())
				if err != nil {
					return nil, err
				}
				due := 0
				for _, current := range notes {
					if current.Status(today()) == note.StatusDue {
						due++
					}
				}
				limit, offset := defaultNotes, min(max(number(a.Offset), 0), len(notes))
				if a.Limit != nil && *a.Limit > 0 {
					limit = *a.Limit
				}
				results, err := noteResults(db, notes[offset:min(offset+limit, len(notes))])
				if err != nil {
					return nil, err
				}
				return map[string]any{"total": len(notes), "offset": offset, "count": len(results), "due": due, "notes": results}, nil
			})),
		reads(tool("note_show", "Show one note: its title, its text, its status, its reminder date, its tags and the records it is about.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := open(db)
				found, err := c.find(ref(a.ID))
				if err != nil {
					return nil, err
				}
				return c.reload(found.ID)
			})),
		tool("note_add", "Write a note about the finances of the user: a fact to remember, a decision, or a thing to do or to check on a date. Keep it short and plain. A note is not a transaction, and it is not a schedule: for a bill that will happen, use schedule_add. All the arguments but the title are optional.",
			func(db *gorm.DB, a addArguments) (any, error) {
				c := open(db)
				values := noteFlags{
					title: a.Title, text: text(a.Text), remind: text(a.RemindOn), tags: text(a.Tags),
					links: noteLinks{goal: text(a.Goal), budget: text(a.Budget), account: text(a.Account)},
				}
				sent := given(map[string]bool{
					"title": true, "text": a.Text != nil, "remind": a.RemindOn != nil, "tags": a.Tags != nil,
					"goal": a.Goal != nil, "budget": a.Budget != nil, "account": a.Account != nil,
				})
				draft := note.Note{Source: c.source}
				if err := c.apply("add", values, sent, &draft); err != nil {
					return nil, err
				}
				if err := c.store.Create(&draft); err != nil {
					return nil, err
				}
				return c.reload(draft.ID)
			}),
		tool("note_edit", "Change a note. Give only the arguments to change.",
			func(db *gorm.DB, a editArguments) (any, error) {
				c := open(db)
				values := noteFlags{
					title: text(a.Title), text: text(a.Text), remind: text(a.RemindOn), tags: text(a.Tags),
					links: noteLinks{goal: text(a.Goal), budget: text(a.Budget), account: text(a.Account)},
				}
				sent := given(map[string]bool{
					"title": a.Title != nil, "text": a.Text != nil, "remind": a.RemindOn != nil, "tags": a.Tags != nil,
					"goal": a.Goal != nil, "budget": a.Budget != nil, "account": a.Account != nil,
				})
				if len(sent) == 0 {
					return nil, errors.New("give at least one of title, text, remind_on, tags, goal, budget, account")
				}
				current, err := c.find(ref(a.ID))
				if err != nil {
					return nil, err
				}
				if err := c.apply("edit", values, sent, &current); err != nil {
					return nil, err
				}
				if err := c.store.Update(&current); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		tool("note_done", "Mark a note as done: the user did what the note tells, or it has no use now. A done note leaves the lists and reminds no more. Use it when the user says so, or when the data shows that it is handled. done: false opens the note again.",
			func(db *gorm.DB, a doneArguments) (any, error) {
				c := open(db)
				current, err := c.find(ref(a.ID))
				if err != nil {
					return nil, err
				}
				done := a.Done == nil || *a.Done
				switch {
				case done && current.Done():
					return nil, fmt.Errorf("note %d is already done", current.ID)
				case !done && !current.Done():
					return nil, fmt.Errorf("note %d is not done", current.ID)
				}
				if err := c.store.SetDone(current.ID, done); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		deletes(tool("note_delete", "Delete a note permanently. To keep the note out of the lists, use note_done.",
			func(db *gorm.DB, a deleteArguments) (any, error) {
				c := open(db)
				current, err := c.find(ref(a.ID))
				if err != nil {
					return nil, err
				}
				if !a.Confirm {
					return confirmation(fmt.Sprintf("Delete note %d (%s) permanently?", current.ID, current.Title)), nil
				}
				if err := c.store.Delete(current.ID); err != nil {
					return nil, err
				}
				return map[string]any{"deleted": true, "note": current.ID}, nil
			})),
	}
}
