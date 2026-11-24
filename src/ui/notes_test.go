package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"finux/src/account"
	"finux/src/goal"
	"finux/src/note"
)

func TestNoteFormCreates(t *testing.T) {
	var model tea.Model = NewNoteForm(NoteFormOptions{})
	view := plain(form(t, model).View().Content)
	for _, want := range []string{"New note", "Title", "Text", "Reminder", "Tags"} {
		if !strings.Contains(view, want) {
			t.Errorf("form has no %q:\n%s", want, view)
		}
	}
	for _, other := range []string{"Code", "Currency", "Color", "Target"} {
		if strings.Contains(view, other) {
			t.Errorf("form of a note shows %q:\n%s", other, view)
		}
	}
	// The title is necessary; the rest is optional.
	model = send(t, model, keyTab)
	if f := form(t, model); f.Focus() != FieldName || f.Error() == "" {
		t.Fatalf("tab with no title: focus %v, error %q", f.Focus(), f.Error())
	}
	model = send(t, model, append(text("Ask about the fee"), keyTab)...)
	model = send(t, model, append(text("R$ 12 in November."), keyTab)...)
	model = send(t, model, append(text("soon"), keyTab)...)
	if f := form(t, model); f.Focus() != FieldDate || f.Error() == "" {
		t.Fatalf("a wrong date: focus %v, error %q", f.Focus(), f.Error())
	}
	model = send(t, model, keyBack, keyBack, keyBack, keyBack)
	model = send(t, model, append(text("05/12/2026"), keyTab)...)
	model = send(t, model, append(text("bank fee"), keyTab)...)
	f := form(t, model)
	got := f.Note()
	if !f.Submitted() || got.Title != "Ask about the fee" || got.Text != "R$ 12 in November." || got.RemindOn != "2026-12-05" || got.Tags != "#bank #fee" {
		t.Fatalf("note of the form = %+v, submitted %v", got, f.Submitted())
	}
}

func TestNoteFormEditKeepsTheRest(t *testing.T) {
	initial := note.Note{ID: 4, Title: "Fee", Text: "Old", RemindOn: "2026-12-05", Tags: "#bank", GoalID: 2, Source: note.SourceAgent}
	var model tea.Model = NewNoteForm(NoteFormOptions{Initial: initial, Edit: true})
	if view := plain(form(t, model).View().Content); !strings.Contains(view, "Edit note") || !strings.Contains(view, "05/12/2026") {
		t.Fatalf("edit form:\n%s", view)
	}
	model = send(t, model, append(text(" of the bank"), keySave)...)
	want := initial
	want.Title = "Fee of the bank"
	if got := form(t, model).Note(); got != want {
		t.Fatalf("edited = %+v, want %+v", got, want)
	}
}

func TestNoteTableAndCard(t *testing.T) {
	created := time.Date(2026, 11, 10, 9, 30, 0, 0, time.Local)
	views := []NoteView{
		{Note: note.Note{ID: 2, Title: "Ask about the fee", Text: "R$ 12.", RemindOn: "2026-11-10", Tags: "#bank", Source: note.SourceUser, CreatedAt: created, UpdatedAt: created},
			Goal: &goal.Goal{Code: "TRIP", Title: "Trip"}, Place: &account.Account{Code: "VISA", Name: "Visa Gold"}, OnCard: true},
		{Note: note.Note{ID: 1, Title: "Fact", Source: note.SourceAgent, CreatedAt: created, UpdatedAt: created}},
	}
	table := NoteTable(views, "2026-11-17", false, true)
	for _, want := range []string{"ID  TITLE", "2   Ask about the fee  10/11/2026  due     TRIP VISA  #bank\n", "1   Fact               -           open    -          -\n", "2 notes\n"} {
		if !strings.Contains(table, want) {
			t.Errorf("table has no %q:\n%s", want, table)
		}
	}
	if got := NoteTable(nil, "2026-11-17", true, true); got != "No notes match the filter.\n" {
		t.Errorf("empty table with a filter = %q", got)
	}
	card := NoteCard(views[0], "2026-11-17", true)
	for _, want := range []string{"Ask about the fee\nR$ 12.\n", "Status    due\n", "Goal      TRIP  Trip\n", "Card      VISA  Visa Gold\n", "Source    user\n"} {
		if !strings.Contains(card, want) {
			t.Errorf("card has no %q:\n%s", want, card)
		}
	}
	styled := plain(NoteCard(views[1], "2026-11-17", false))
	if !strings.Contains(styled, "OPEN") || !strings.Contains(styled, "Source   agent") {
		t.Errorf("styled card:\n%s", styled)
	}
	if got := NoteSection(nil, "2026-11-17", true); got != "" {
		t.Errorf("section with no notes = %q", got)
	}
	if got := plain(NoteSection([]note.Note{views[0].Note}, "2026-11-17", false)); !strings.Contains(got, "NOTES") || !strings.Contains(got, "#2  Ask about the fee") {
		t.Errorf("styled section:\n%s", got)
	}
}
