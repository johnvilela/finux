package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"finux/src/transaction"
)

// samplePart returns part 3 of 10 of a purchase.
func samplePart() transaction.View {
	view := sampleTransactions()[0]
	view.Title, view.Tags = "Notebook", ""
	view.InstallmentID, view.Part, view.Parts = 7, 3, 10
	return view
}

func TestInstallmentShowsItsPart(t *testing.T) {
	part := samplePart()
	if got := TransactionTable([]transaction.View{part}, true); !strings.Contains(got, "12  17/11/2026  Notebook 3/10  FOOD") {
		t.Errorf("plain table:\n%s", got)
	}
	if got := plain(TransactionTable([]transaction.View{part}, false)); !strings.Contains(got, "Notebook 3/10") {
		t.Errorf("styled table:\n%s", got)
	}
	if got := TransactionCard(part, true); !strings.Contains(got, "Date      17/11/2026\nPart      3/10\n") {
		t.Errorf("plain card:\n%s", got)
	}
	if got := plain(TransactionCard(part, false)); !strings.Contains(got, "Part      3/10") {
		t.Errorf("styled card:\n%s", got)
	}
	if got := TransactionCard(sampleTransactions()[0], true); strings.Contains(got, "Part") {
		t.Errorf("card of a normal transaction has a part:\n%s", got)
	}
	var model tea.Model = NewLedger("Transactions", []transaction.View{part})
	if view := ledgerView(t, model); !strings.Contains(view, "Notebook 3/10") {
		t.Errorf("ledger:\n%s", view)
	}
}

func TestTransactionFormInstallments(t *testing.T) {
	var model tea.Model = NewTransactionForm(txOptions())
	if view := plain(txForm(t, model).View().Content); !strings.Contains(view, "Installments") {
		t.Fatalf("form of an expense lacks the installments:\n%s", view)
	}
	model = step(t, model, TxTitle, keyEnter)
	model = step(t, model, TxValue, append(text("Sofa"), keyTab)...)
	model = step(t, model, TxInstallments, append(text("1000"), keyTab)...)
	// The number must be from 2. An empty field is one transaction.
	model = step(t, model, TxInstallments, append(text("1"), keySave)...)
	if got := txForm(t, model).Error(); !strings.Contains(got, "from 2 to 120") {
		t.Fatalf("error = %q", got)
	}
	model = send(t, model, keyBack)
	model = step(t, model, TxAccount, append(text("3"), keyTab)...)
	f := txForm(t, model)
	if view := plain(f.View().Content); !strings.Contains(view, "3 × R$ 333,33") {
		t.Errorf("preview lacks the parts:\n%s", view)
	}
	if draft := f.Draft(); draft.Installments != 3 || draft.Value != 100000 {
		t.Fatalf("draft = %+v", draft)
	}
	model = send(t, model, keySave)
	if f = txForm(t, model); !f.Submitted() {
		t.Fatalf("form not submitted: %q", f.Error())
	}

	// A transfer and an edit have no installments.
	options := txOptions()
	options.Initial.Type = transaction.Transfer
	if view := plain(NewTransactionForm(options).View().Content); strings.Contains(view, "Installments") {
		t.Errorf("form of a transfer shows the installments:\n%s", view)
	}
	options = txOptions()
	options.Edit, options.Initial = true, transaction.Draft{Title: "Sofa", Type: transaction.Expense, AccountID: 1, CategoryID: 2, Date: "2026-11-17"}
	options.Value = "10,00"
	edit := NewTransactionForm(options)
	if view := plain(edit.View().Content); strings.Contains(view, "Installments") || edit.Draft().Installments != 0 {
		t.Errorf("edit form shows the installments:\n%s", view)
	}
	// An option of the command line gives the number.
	options = txOptions()
	options.Initial.Installments, options.Preset = 4, []TxField{TxInstallments}
	preset := NewTransactionForm(options)
	if view := plain(preset.View().Content); strings.Contains(view, "Installments") || preset.Draft().Installments != 4 {
		t.Errorf("preset installments: draft %+v\n%s", preset.Draft(), view)
	}
}
