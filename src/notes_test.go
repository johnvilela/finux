package main

import (
	"strings"
	"testing"
)

// The tests use the database of newCardsHome: the account NU and the
// credit card VISA. The date of today is 17/11/2026.

func newNotesHome(t *testing.T) cardsHome {
	t.Helper()
	h := newCardsHome(t)
	h.ok("gl", "add", "--title", "Trip", "--type", "save", "--target", "1000", "--currency", "BRL", "--code", "TRIP")
	h.ok("bg", "add", "--title", "Food", "--limit", "800", "--currency", "BRL", "--categories", "FOOD", "--every", "month", "--code", "FOOD")
	return h
}

func TestNotesLifecycle(t *testing.T) {
	h := newNotesHome(t)
	const empty = "No notes yet. Run 'finux nt add'.\n"
	for _, args := range [][]string{{"notes"}, {"note"}, {"nt"}, {"nt", "list"}} {
		if got := h.ok(args...); got != empty {
			t.Fatalf("finux %v = %q", args, got)
		}
	}
	if got := h.ok("nt", "add", "--title", "Energy bill is shared", "--text", "Ana pays half of the energy bill each month.", "--tags", "home ana"); got != "✓ Note 1 created\n" {
		t.Fatalf("add output = %q", got)
	}
	h.ok("nt", "a", "--title", "Ask about the fee", "--remind", "10/11/2026", "--account", "nu", "--tags", "bank")
	h.ok("notes", "add", "--title", "Check the limit", "--remind", "05/12/2026", "--account", "VISA", "--goal", "trip", "--budget", "food")

	// The due note comes first, then the other reminders, then the rest.
	list := h.ok("nt")
	h.has(list, "ID  TITLE                  REMIND      STATUS  LINKS           TAGS\n",
		"2   Ask about the fee      10/11/2026  due     NU              #bank\n",
		"3   Check the limit        05/12/2026  open    TRIP FOOD VISA  -\n",
		"1   Energy bill is shared  -           open    -               #home #ana\n", "3 notes\n")
	if strings.Index(list, "Ask about") > strings.Index(list, "Check the") || strings.Index(list, "Check the") > strings.Index(list, "Energy") {
		t.Fatalf("order of the list:\n%s", list)
	}
	details := h.ok("nt", "3")
	h.has(details, "Check the limit\n", "ID        3\n", "Status    open\n", "Remind    05/12/2026\n", "Goal      TRIP  Trip\n",
		"Budget    FOOD  Food\n", "Card      VISA  Visa Gold\n", "Source    user\n")
	if got := h.ok("nt", "show", "3"); got != details {
		t.Fatalf("show = %q", got)
	}
	h.has(h.ok("nt", "1"), "Energy bill is shared\nAna pays half of the energy bill each month.\n", "Tags      #home #ana\n")
	h.has(h.ok("nt", "2"), "Account   NU  Nubank\n", "Status    due\n")

	// Filters.
	for want, args := range map[string][]string{
		"Ask about the fee":     {"--due"},
		"Energy bill is shared": {"--tag", "ana"},
		"Check the limit":       {"--goal", "TRIP"},
	} {
		got := h.ok(append([]string{"nt", "list"}, args...)...)
		if !strings.Contains(got, want) || !strings.Contains(got, "1 note\n") {
			t.Fatalf("list %v:\n%s", args, got)
		}
	}
	h.has(h.ok("nt", "--account", "VISA"), "Check the limit", "1 note\n")
	h.has(h.ok("nt", "--budget", "FOOD"), "Check the limit", "1 note\n")
	h.has(h.ok("nt", "--search", "half"), "Energy bill", "1 note\n")
	if got := h.ok("nt", "--tag", "none"); got != "No notes match the filter.\n" {
		t.Fatalf("empty filter = %q", got)
	}

	// Done takes a note out of the list; --all shows it; reopen brings it back.
	if got := h.ok("nt", "done", "2"); got != "✓ Note 2 done\n" {
		t.Fatalf("done output = %q", got)
	}
	if list = h.ok("nt"); strings.Contains(list, "Ask about") || !strings.Contains(list, "2 notes\n") {
		t.Fatalf("list after done:\n%s", list)
	}
	h.has(h.ok("nt", "--all"), "2   Ask about the fee      10/11/2026  done ", "3 notes\n")
	h.fail(1, "note 2 is already done", "nt", "done", "2")
	if got := h.ok("nt", "reopen", "2"); got != "✓ Note 2 open again\n" {
		t.Fatalf("reopen output = %q", got)
	}
	h.fail(1, "note 2 is not done", "nt", "reopen", "2")

	// Edit changes only what is given; none removes a date or a link.
	if got := h.ok("nt", "edit", "3", "--title", "Check the card limit", "--remind", "none", "--goal", "none", "--account", "NU", "--tags", "card"); got != "✓ Note 3 updated\n" {
		t.Fatalf("edit output = %q", got)
	}
	details = h.ok("nt", "3")
	h.has(details, "Check the card limit\n", "Account   NU  Nubank\n", "Budget    FOOD  Food\n", "Tags      #card\n")
	if strings.Contains(details, "Remind") || strings.Contains(details, "Goal") || strings.Contains(details, "Card  ") {
		t.Fatalf("details after the edit:\n%s", details)
	}
	h.ok("nt", "edit", "3", "--account", "none", "--budget", "none", "--tags", "")
	if details = h.ok("nt", "3"); strings.Contains(details, "Account") || strings.Contains(details, "Budget") || strings.Contains(details, "Tags") {
		t.Fatalf("details without links:\n%s", details)
	}

	h.fail(2, "delete: --yes is necessary when input is not a terminal", "nt", "delete", "1")
	if got := h.ok("nt", "delete", "1", "--yes"); got != "✓ Note 1 deleted\n" {
		t.Fatalf("delete output = %q", got)
	}
	h.fail(1, "note not found: 1", "nt", "1")
}

func TestNotesChecks(t *testing.T) {
	h := newNotesHome(t)
	h.fail(2, "add: --title is necessary when input is not a terminal", "nt", "add", "--text", "a")
	h.fail(2, "add: title is empty", "nt", "add", "--title", " ")
	h.fail(2, "add: text is too long: the limit is 500 characters", "nt", "add", "--title", "A", "--text", strings.Repeat("a", 501))
	h.fail(2, "add: invalid date", "nt", "add", "--title", "A", "--remind", "soon")
	h.fail(2, "add: invalid tag", "nt", "add", "--title", "A", "--tags", "a-b")
	h.fail(1, "account not found: NOPE", "nt", "add", "--title", "A", "--account", "NOPE")
	h.fail(1, "goal not found: NOPE", "nt", "add", "--title", "A", "--goal", "NOPE")
	h.fail(1, "budget not found: NOPE", "nt", "add", "--title", "A", "--budget", "NOPE")
	h.fail(2, "edit: give --title, --text, --remind, --tags, --goal, --budget or --account when input is not a terminal", "nt", "edit", "1")
	h.fail(1, "note not found: 9", "nt", "edit", "9", "--title", "A")
	h.fail(1, "note not found: abc", "nt", "done", "abc")
	h.fail(2, "done: a note id is necessary when input is not a terminal", "nt", "done")
	h.has(h.ok("nt", "--help"), "Manage notes", "finux nt [command]")
	h.has(h.ok("--help"), "notes, nt")
}

// The details of a record list its open notes, and a record that is
// deleted leaves its notes without the link.
func TestNotesFollowTheirRecords(t *testing.T) {
	h := newNotesHome(t)
	h.ok("nt", "add", "--title", "Fee to check", "--account", "NU", "--remind", "20/11/2026")
	h.ok("nt", "add", "--title", "Card note", "--account", "VISA")
	h.ok("nt", "add", "--title", "Trip plan", "--goal", "TRIP")
	h.ok("nt", "add", "--title", "Food rule", "--budget", "FOOD")
	h.ok("nt", "add", "--title", "Old fact", "--account", "NU")
	h.ok("nt", "done", "5")
	account := h.ok("ac", "NU")
	h.has(account, "\nNotes\n", "1   Fee to check  20/11/2026  open\n")
	if strings.Contains(account, "Old fact") || strings.Contains(account, "Card note") {
		t.Fatalf("account details show a done note or a note of the card:\n%s", account)
	}
	h.has(h.ok("cc", "VISA"), "\nNotes\n", "2   Card note  -  open\n")
	h.has(h.ok("gl", "TRIP"), "\nNotes\n", "3   Trip plan  -  open\n")
	h.has(h.ok("bg", "FOOD"), "\nNotes\n", "4   Food rule  -  open\n")

	// The resume has the notes with a reminder in its days.
	h.has(h.ok("rs"), "Notes\n", "Fee to check")
	// A due note is in the notice of the transactions.
	h.ok("nt", "edit", "1", "--remind", "17/11/2026")
	h.has(h.ok("tx"), "1 note is due. Run 'finux nt'.")

	h.ok("gl", "delete", "TRIP", "--hard", "--yes")
	h.ok("bg", "delete", "FOOD", "--hard", "--yes")
	h.ok("cc", "delete", "VISA", "--hard", "--yes")
	h.ok("ac", "delete", "NU", "--hard", "--yes")
	list := h.ok("nt", "--all")
	h.has(list, "5 notes\n")
	for _, code := range []string{"TRIP", "FOOD", "VISA", "NU"} {
		if strings.Contains(list, code) {
			t.Fatalf("list keeps the link %s:\n%s", code, list)
		}
	}
}
