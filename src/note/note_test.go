package note

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"finux/src/storage"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := storage.Init(path, &Note{}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

func add(t *testing.T, store *Store, current Note) Note {
	t.Helper()
	if err := store.Create(&current); err != nil {
		t.Fatalf("create %q: %v", current.Title, err)
	}
	return current
}

func TestValidate(t *testing.T) {
	valid := Note{Title: "Ask about the fee", Text: "The bank took R$ 12.", RemindOn: "2026-12-05", Source: SourceUser}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for want, change := range map[string]func(*Note){
		"title is required":       func(n *Note) { n.Title = "  " },
		"title is too long":       func(n *Note) { n.Title = strings.Repeat("a", MaxTitle+1) },
		"text is too long":        func(n *Note) { n.Text = strings.Repeat("ã", MaxText+1) },
		"invalid date":            func(n *Note) { n.RemindOn = "05/12/2026" },
		"an account or a credit":  func(n *Note) { n.AccountID, n.CardID = 1, 2 },
		"unknown source \"bank\"": func(n *Note) { n.Source = "bank" },
	} {
		current := valid
		change(&current)
		if err := current.Validate(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Validate error = %v, want %q", err, want)
		}
	}
	// The limit of the text counts characters, not bytes.
	valid.Text = strings.Repeat("ã", MaxText)
	if err := valid.Validate(); err != nil {
		t.Errorf("text of %d characters: %v", MaxText, err)
	}
}

func TestStatus(t *testing.T) {
	store := newStore(t)
	plain := add(t, store, Note{Title: "Fact"})
	later := add(t, store, Note{Title: "Later", RemindOn: "2026-12-01"})
	today := add(t, store, Note{Title: "Today", RemindOn: "2026-11-17"})
	for current, want := range map[*Note]string{&plain: StatusOpen, &later: StatusOpen, &today: StatusDue} {
		if got := current.Status("2026-11-17"); got != want {
			t.Errorf("status of %s = %s, want %s", current.Title, got, want)
		}
	}
	if plain.Source != SourceUser {
		t.Errorf("source = %q, want the user by default", plain.Source)
	}
	if err := store.SetDone(today.ID, true); err != nil {
		t.Fatal(err)
	}
	found, err := store.Find(today.ID)
	if err != nil || found.Status("2026-11-17") != StatusDone || found.DoneAt == nil {
		t.Fatalf("after done = %+v, %v", found, err)
	}
	if err := store.SetDone(today.ID, false); err != nil {
		t.Fatal(err)
	}
	if found, _ = store.Find(today.ID); found.Status("2026-11-17") != StatusDue {
		t.Fatalf("after reopen = %+v", found)
	}
	if err := store.SetDone(99, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetDone of a missing note = %v", err)
	}
}

func titles(notes []Note) string {
	var names []string
	for _, current := range notes {
		names = append(names, current.Title)
	}
	return strings.Join(names, ",")
}

func TestStoreListAndFilters(t *testing.T) {
	store := newStore(t)
	add(t, store, Note{Title: "No date", Tags: "#bank", GoalID: 1})
	add(t, store, Note{Title: "December", RemindOn: "2026-12-01", Tags: "#bank #fee", AccountID: 2})
	add(t, store, Note{Title: "Late", Text: "Call the manager", RemindOn: "2026-11-10", CardID: 3, Source: SourceAgent})
	done := add(t, store, Note{Title: "Old", RemindOn: "2026-10-01", BudgetID: 4})
	if err := store.SetDone(done.ID, true); err != nil {
		t.Fatal(err)
	}
	const today = "2026-11-17"
	for name, tc := range map[string]struct {
		filter Filter
		want   string
	}{
		// The notes with a reminder come first, the oldest date first.
		"open":     {Filter{}, "Late,December,No date"},
		"all":      {Filter{Status: StatusAll}, "Late,December,No date,Old"},
		"due":      {Filter{Status: StatusDue}, "Late"},
		"done":     {Filter{Status: StatusDone}, "Old"},
		"tag":      {Filter{Tag: "#bank"}, "December,No date"},
		"tag fee":  {Filter{Tag: "fee"}, "December"},
		"search":   {Filter{Search: "manager"}, "Late"},
		"title":    {Filter{Search: "dece"}, "December"},
		"goal":     {Filter{GoalID: 1}, "No date"},
		"account":  {Filter{AccountID: 2}, "December"},
		"card":     {Filter{CardID: 3}, "Late"},
		"budget":   {Filter{BudgetID: 4, Status: StatusAll}, "Old"},
		"until":    {Filter{Until: "2026-11-30"}, "Late"},
		"by agent": {Filter{Source: SourceAgent}, "Late"},
	} {
		notes, err := store.List(tc.filter, today)
		if err != nil || titles(notes) != tc.want {
			t.Errorf("%s: %q, %v; want %q", name, titles(notes), err, tc.want)
		}
	}
}

func TestStoreUpdateDeleteAndLinks(t *testing.T) {
	store := newStore(t)
	current := add(t, store, Note{Title: "  Fee ", Text: " text ", GoalID: 1, AccountID: 2})
	if current.Title != "Fee" || current.Text != "text" {
		t.Fatalf("created note not trimmed: %+v", current)
	}
	current.Title, current.RemindOn, current.AccountID, current.CardID = "Bank fee", "2026-12-05", 0, 7
	if err := store.Update(&current); err != nil {
		t.Fatal(err)
	}
	found, err := store.Find(current.ID)
	if err != nil || found.Title != "Bank fee" || found.RemindOn != "2026-12-05" || found.AccountID != 0 || found.CardID != 7 || found.GoalID != 1 {
		t.Fatalf("updated = %+v, %v", found, err)
	}
	current.Title = ""
	if err := store.Update(&current); err == nil {
		t.Fatal("update with an empty title worked")
	}

	// A record that is deleted leaves its notes without the link.
	if err := store.ClearLink(LinkGoal, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.ClearLink(LinkCard, 7); err != nil {
		t.Fatal(err)
	}
	if found, _ = store.Find(current.ID); found.GoalID != 0 || found.CardID != 0 {
		t.Fatalf("links after the deletes = %+v", found)
	}
	if err := store.Delete(current.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Find(current.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find after delete = %v", err)
	}
	if err := store.Delete(current.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}
