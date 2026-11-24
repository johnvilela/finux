package transaction

import (
	"testing"
)

func TestPattern(t *testing.T) {
	cases := map[string]string{
		"PAY LAMEI 21/11":          "PAY LAMEI",
		"PAY LAMEI 23/11":          "PAY LAMEI",
		"pay lamei 23/11/2026":     "PAY LAMEI",
		"PIX QRS EMILLE PAES16/11": "PIX QRS EMILLE PAES",
		"PIX TRANSF Moto Li23/11":  "PIX TRANSF MOTO LI",
		"1 Cartão (2it4w4)":        "CARTAO",
		"1 Cartão (2ite78)":        "CARTAO",
		"INT PRE-PAGOXXXXX 7198":   "INT PRE-PAGOXXXXX",
		"SEGURO CARTAO 0000":       "SEGURO CARTAO",
		"  Drogaria   São Paulo ":  "DROGARIA SAO PAULO",
		"Pão de Açúcar":            "PAO DE ACUCAR",
		// A title with numbers only keeps them: it must have a key.
		"1234 21/11": "1234 21/11",
	}
	for title, want := range cases {
		if got := Pattern(title); got != want {
			t.Errorf("Pattern(%q) = %q, want %q", title, got, want)
		}
	}
}

// rules returns the corrections by pattern.
func (f fixture) rules(t *testing.T, titles ...string) map[string]CorrectionView {
	t.Helper()
	views, _, err := f.store.Corrections(titles, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	rules := map[string]CorrectionView{}
	for _, view := range views {
		rules[view.Pattern] = view
	}
	return rules
}

func (f fixture) change(t *testing.T, id uint, edit func(*Draft)) Transaction {
	t.Helper()
	current, err := f.store.Find(id)
	if err != nil {
		t.Fatal(err)
	}
	draft := DraftOf(current.Transaction)
	edit(&draft)
	updated, err := f.store.Update(id, draft)
	if err != nil {
		t.Fatalf("Update(%+v): %v", draft, err)
	}
	return updated
}

func text(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}

func TestCorrectionsLearnFromEdits(t *testing.T) {
	f := newFixture(t)
	created := f.create(t, f.draft("PAY LAMEI 21/11", Expense, "INTER", "OTHER", 1600))
	if rules := f.rules(t); len(rules) != 0 {
		t.Fatalf("a new transaction made rules: %+v", rules)
	}
	// A change of the value or the date teaches nothing.
	f.change(t, created.ID, func(d *Draft) { d.Value, d.Date = 1700, "2026-11-18" })
	if rules := f.rules(t); len(rules) != 0 {
		t.Fatalf("a change of the value made rules: %+v", rules)
	}

	f.change(t, created.ID, func(d *Draft) { d.Title, d.CategoryID = "Lamen House", f.category["FOOD"] })
	rule, ok := f.rules(t)["PAY LAMEI"]
	if !ok || rule.Title != "Lamen House" || rule.Category != "FOOD" || rule.Tags != nil || rule.Description != nil || rule.Source != SourceUser || rule.Count != 1 {
		t.Fatalf("rule after the rename = %+v (found %v)", rule, ok)
	}

	// The tags of the new name go to its own rule and to the rule that
	// gave the name.
	f.change(t, created.ID, func(d *Draft) { d.Tags, d.Description = "#lunch", "with the team" })
	rules := f.rules(t)
	if len(rules) != 2 {
		t.Fatalf("rules = %+v, want PAY LAMEI and LAMEN HOUSE", rules)
	}
	first, second := rules["PAY LAMEI"], rules["LAMEN HOUSE"]
	if first.Title != "Lamen House" || text(first.Tags) != "#lunch" || text(first.Description) != "with the team" || first.Count != 2 {
		t.Errorf("PAY LAMEI = %+v", first)
	}
	if second.Title != "" || second.Category != "" || text(second.Tags) != "#lunch" || second.Count != 1 {
		t.Errorf("LAMEN HOUSE = %+v", second)
	}

	// A transaction of another day with the same raw line has the same
	// rule: the table does not grow.
	other := f.create(t, f.draft("PAY LAMEI 23/11", Expense, "INTER", "OTHER", 1800))
	f.change(t, other.ID, func(d *Draft) { d.Title = "Lamen House" })
	if rules := f.rules(t); len(rules) != 2 || rules["PAY LAMEI"].Count != 3 {
		t.Fatalf("rules after the second transaction = %+v", rules)
	}

	// Tags that the user takes away are learned as no tags.
	f.change(t, created.ID, func(d *Draft) { d.Tags = "" })
	if rule := f.rules(t)["PAY LAMEI"]; text(rule.Tags) != "" {
		t.Errorf("tags after the user took them away = %q, want empty", text(rule.Tags))
	}
}

// A name the agent gave and the user then changes: the rule of the raw
// line gets the name of the user.
func TestCorrectionsFollowTheChain(t *testing.T) {
	f := newFixture(t)
	created := f.create(t, f.draft("PAY DROGA 18/11", Expense, "INTER", "OTHER", 24998))
	agent := NewStore(f.store.db)
	agent.Actor = SourceAgent
	draft := DraftOf(created)
	draft.Title = "Drogasil"
	if _, err := agent.Update(created.ID, draft); err != nil {
		t.Fatal(err)
	}
	if rule := f.rules(t)["PAY DROGA"]; rule.Title != "Drogasil" || rule.Source != SourceAgent {
		t.Fatalf("rule of the agent = %+v", rule)
	}
	f.change(t, created.ID, func(d *Draft) { d.Title = "Drogaria São Paulo" })
	rules := f.rules(t)
	if rule := rules["PAY DROGA"]; rule.Title != "Drogaria São Paulo" || rule.Source != SourceUser {
		t.Errorf("PAY DROGA = %+v, want the name of the user", rule)
	}
	if rule := rules["DROGASIL"]; rule.Title != "Drogaria São Paulo" || rule.Source != SourceUser {
		t.Errorf("DROGASIL = %+v, want the name of the user", rule)
	}

	// The raw line comes back: the rule has nothing to teach.
	f.change(t, created.ID, func(d *Draft) { d.Title = "PAY DROGA 18/11" })
	if rule, ok := f.rules(t)["PAY DROGA"]; ok {
		t.Errorf("PAY DROGA = %+v, want no rule after the raw line came back", rule)
	}
}

func TestCorrectionsLookup(t *testing.T) {
	f := newFixture(t)
	for _, title := range []string{"PAY LAMEI 21/11", "PAY MINUT 19/11", "PAY DROGA 18/11"} {
		created := f.create(t, f.draft(title, Expense, "INTER", "OTHER", 1000))
		f.change(t, created.ID, func(d *Draft) { d.CategoryID = f.category["FOOD"] })
	}
	// The titles of other days find the rules.
	rules := f.rules(t, "PAY LAMEI 30/11", "pay droga 02/12", "Unknown")
	if len(rules) != 2 || rules["PAY LAMEI"].Category != "FOOD" || rules["PAY DROGA"].Category != "FOOD" {
		t.Fatalf("lookup = %+v", rules)
	}
	page, total, err := f.store.Corrections(nil, 2, 1)
	if err != nil || total != 3 || len(page) != 2 {
		t.Fatalf("page = %+v, total %d, %v", page, total, err)
	}
}

func TestCorrectionsSkipTransfersAndCountPartsOnce(t *testing.T) {
	f := newFixture(t)
	transfer := f.draft("To Nubank", Transfer, "INTER", "XFER", 1000)
	transfer.ToAccountID = f.account["NUBAN"]
	moved := f.create(t, transfer)
	f.change(t, moved.ID, func(d *Draft) { d.Title = "Savings" })
	if rules := f.rules(t); len(rules) != 0 {
		t.Fatalf("a transfer made rules: %+v", rules)
	}

	purchase := f.draft("LOJA X 0423", Expense, "INTER", "OTHER", 30000)
	purchase.Installments = 3
	first := f.create(t, purchase)
	draft := DraftOf(first)
	draft.Title = "Loja X"
	if _, err := f.store.UpdateParts(first.ID, All, draft, Changed{Title: true}); err != nil {
		t.Fatal(err)
	}
	if rule := f.rules(t)["LOJA X"]; rule.Title != "Loja X" || rule.Count != 1 {
		t.Fatalf("rule of the purchase = %+v, want one count for the three parts", rule)
	}
}
