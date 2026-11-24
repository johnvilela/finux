package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDetails(t *testing.T) {
	key := func(r rune) tea.Msg { return tea.KeyPressMsg{Code: r, Text: string(r)} }
	cases := []struct {
		name string
		keys []tea.Msg
		want DetailsAction
	}{
		{"enter picks edit, which has the focus first", []tea.Msg{keyEnter}, DetailsEdit},
		{"right then enter picks delete", []tea.Msg{keyRight, keyEnter}, DetailsDelete},
		{"tab two times goes back to edit", []tea.Msg{keyTab, keyTab, keyEnter}, DetailsEdit},
		{"e picks edit", []tea.Msg{keyRight, key('e')}, DetailsEdit},
		{"d picks delete", []tea.Msg{key('d')}, DetailsDelete},
		{"escape closes", []tea.Msg{keyRight, keyEscape}, DetailsClose},
		{"q closes", []tea.Msg{key('q')}, DetailsClose},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := send(t, NewDetails("CARD\n"), tc.keys...)
			details, ok := model.(Details)
			if !ok {
				t.Fatalf("model is %T, want Details", model)
			}
			if got := details.Action(); got != tc.want {
				t.Errorf("Action() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDetailsView(t *testing.T) {
	var model tea.Model = NewDetails("CARD\n")
	open := plain(model.View().Content)
	for _, want := range []string{"CARD\n", "Edit", "Delete", "enter select", "esc close"} {
		if !strings.Contains(open, want) {
			t.Errorf("view lacks %q:\n%s", want, open)
		}
	}
	// The card stays on the screen after the screen closes, and for the
	// confirmation of a delete. The form of an edit replaces it.
	if got := send(t, model, keyEscape).View().Content; got != "CARD\n" {
		t.Errorf("view after close = %q, want the card only", got)
	}
	if got := send(t, model, keyRight, keyEnter).View().Content; got != "CARD\n" {
		t.Errorf("view after delete = %q, want the card only", got)
	}
	if got := send(t, model, keyEnter).View().Content; got != "" {
		t.Errorf("view after edit = %q, want no text", got)
	}
}
