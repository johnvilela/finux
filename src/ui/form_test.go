package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"finux/src/account"
)

func free(string) bool { return false }

func form(t *testing.T, model tea.Model) Form {
	t.Helper()
	f, ok := model.(Form)
	if !ok {
		t.Fatalf("model is %T, want Form", model)
	}
	return f
}

func TestFormCreateFlow(t *testing.T) {
	var model tea.Model = NewForm(FormOptions{Title: "New account", Taken: free})
	if got := form(t, model).Focus(); got != FieldName {
		t.Fatalf("first focus = %v, want name", got)
	}
	model = send(t, model, text("Banco Inter")...)
	if got := form(t, model).Value(FieldCode); got != "BINTE" {
		t.Fatalf("suggested code = %q, want BINTE", got)
	}
	steps := []struct {
		keys  []tea.Msg
		focus Field
	}{
		{[]tea.Msg{keyTab}, FieldCode},
		{[]tea.Msg{keyEnter}, FieldDescription},
		{append(text("main"), keyTab), FieldCurrency},
		{[]tea.Msg{keyRight, keyRight, keyTab}, FieldBalance},
		{append(text("1234,56"), keyTab), FieldColor},
	}
	for _, step := range steps {
		model = send(t, model, step.keys...)
		if got := form(t, model).Focus(); got != step.focus {
			t.Fatalf("focus = %v, want %v (error %q)", got, step.focus, form(t, model).Error())
		}
	}
	if form(t, model).Submitted() {
		t.Fatal("form submitted before the last field")
	}
	model = send(t, model, keyRight, keyDown, keyEnter)
	f := form(t, model)
	if !f.Submitted() || f.Cancelled() {
		t.Fatalf("submitted = %v, cancelled = %v, error %q", f.Submitted(), f.Cancelled(), f.Error())
	}
	got := f.Account()
	want := account.Account{Name: "Banco Inter", Code: "BINTE", Description: "main", Currency: "BRL", OpeningBalance: 123456, Color: account.Palette[5].Key}
	if got != want {
		t.Fatalf("account = %+v, want %+v", got, want)
	}
}

func TestFormEditedCodeIsKept(t *testing.T) {
	var model tea.Model = NewForm(FormOptions{Taken: free})
	model = send(t, model, append(text("Nubank"), keyTab)...)
	model = send(t, model, keyBack, keyBack, keyBack, keyBack, keyBack)
	model = send(t, model, text("nu-1x")...)
	if got := form(t, model).Value(FieldCode); got != "NU1X" {
		t.Fatalf("typed code = %q, want NU1X (uppercase, letters and digits only)", got)
	}
	model = send(t, model, keyShiftTab)
	model = send(t, model, text(" Conta")...)
	f := form(t, model)
	if f.Focus() != FieldName || f.Value(FieldCode) != "NU1X" {
		t.Fatalf("focus = %v, code = %q; want name and NU1X", f.Focus(), f.Value(FieldCode))
	}
	model = send(t, model, text("toolongcode")...)
	if got := form(t, model).Value(FieldName); got != "Nubank Contatoolongcode" {
		t.Fatalf("name = %q", got)
	}
}

func TestFormValidation(t *testing.T) {
	taken := func(code string) bool { return code == "INTER" }
	var model tea.Model = NewForm(FormOptions{Taken: taken})
	model = send(t, model, keyTab)
	if f := form(t, model); f.Focus() != FieldName || f.Error() == "" {
		t.Fatalf("empty name accepted: focus %v, error %q", f.Focus(), f.Error())
	}
	model = send(t, model, append(text("Inter"), keyTab)...)
	if f := form(t, model); f.Focus() != FieldCode || f.Error() != "" {
		t.Fatalf("after name: focus %v, error %q", f.Focus(), f.Error())
	}
	if got := form(t, model).Value(FieldCode); got != "INTE2" {
		t.Fatalf("suggestion with taken code = %q, want INTE2", got)
	}
	model = send(t, model, keyBack, keyBack, keyBack, keyBack, keyBack)
	model = send(t, model, append(text("inter"), keyTab)...)
	if f := form(t, model); f.Focus() != FieldCode || !strings.Contains(f.Error(), "in use") {
		t.Fatalf("taken code accepted: focus %v, error %q", f.Focus(), f.Error())
	}
	model = send(t, model, keyBack, keyBack, keyBack, keyBack, keyBack)
	model = send(t, model, append(text("123"), keyTab)...)
	if f := form(t, model); f.Focus() != FieldCode || f.Error() == "" {
		t.Fatalf("digits-only code accepted: focus %v", f.Focus())
	}
	model = send(t, model, append(text("a"), keyTab, keyTab, keyTab)...)
	if f := form(t, model); f.Focus() != FieldBalance {
		t.Fatalf("focus = %v, want balance (error %q)", f.Focus(), f.Error())
	}
	model = send(t, model, append(text("12.345"), keyTab)...)
	if f := form(t, model); f.Focus() != FieldBalance || f.Error() == "" {
		t.Fatalf("bad balance accepted: focus %v", f.Focus())
	}
	if !strings.Contains(plain(form(t, model).View().Content), form(t, model).Error()) {
		t.Fatal("view does not show the error")
	}
}

func TestFormSaveShortcutChecksAllFields(t *testing.T) {
	var model tea.Model = NewForm(FormOptions{Taken: free})
	model = send(t, model, keySave)
	if f := form(t, model); f.Submitted() || f.Focus() != FieldName || f.Error() == "" {
		t.Fatalf("empty form saved: submitted %v, focus %v", f.Submitted(), f.Focus())
	}
	model = send(t, model, append(text("Wise"), keySave)...)
	f := form(t, model)
	if !f.Submitted() {
		t.Fatalf("not submitted: %q", f.Error())
	}
	got := f.Account()
	if got.Code != "WISE" || got.Currency != "USD" || got.OpeningBalance != 0 || got.Color != account.Palette[0].Key {
		t.Fatalf("defaults = %+v", got)
	}
}

func TestFormCancel(t *testing.T) {
	var model tea.Model = NewForm(FormOptions{Taken: free})
	model = send(t, model, append(text("Wise"), keyEscape)...)
	if f := form(t, model); !f.Cancelled() || f.Submitted() {
		t.Fatalf("cancelled = %v, submitted = %v", f.Cancelled(), f.Submitted())
	}
}

func TestFormEditMode(t *testing.T) {
	initial := sampleAccounts()[0]
	var model tea.Model = NewForm(FormOptions{Title: "Edit INTER", Initial: initial, Edit: true, Taken: free})
	view := plain(form(t, model).View().Content)
	if !strings.Contains(view, "Edit INTER") || strings.Contains(view, "Currency") || strings.Contains(view, "Opening balance") {
		t.Fatalf("edit view shows fixed fields or no title:\n%s", view)
	}
	var order []Field
	for range 4 {
		order = append(order, form(t, model).Focus())
		model = send(t, model, keyTab)
	}
	want := []Field{FieldName, FieldCode, FieldDescription, FieldColor}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("field order = %v, want %v", order, want)
		}
	}
	if f := form(t, model); !f.Submitted() || f.Account() != initial {
		t.Fatalf("unchanged edit = %+v, submitted %v", f.Account(), f.Submitted())
	}
	model = NewForm(FormOptions{Initial: initial, Edit: true, Taken: free})
	model = send(t, model, append(text(" PJ"), keySave)...)
	got := form(t, model).Account()
	if got.Name != "Banco Inter PJ" || got.Code != "INTER" || got.ID != 1 || got.OpeningBalance != 123456 || got.Currency != "BRL" {
		t.Fatalf("edited = %+v", got)
	}
}

func TestFormPresetFieldsAreSkipped(t *testing.T) {
	initial := account.Account{Name: "Wise", Currency: "EUR", Color: "teal"}
	var model tea.Model = NewForm(FormOptions{Initial: initial, Balance: "10", Preset: []Field{FieldName, FieldCurrency, FieldBalance}, Taken: free})
	f := form(t, model)
	if f.Focus() != FieldCode || f.Value(FieldCode) != "WISE" {
		t.Fatalf("focus = %v, code = %q; want code field with WISE", f.Focus(), f.Value(FieldCode))
	}
	model = send(t, model, keyTab, keyTab, keyEnter)
	f = form(t, model)
	want := account.Account{Name: "Wise", Code: "WISE", Currency: "EUR", Color: "teal", OpeningBalance: 1000}
	if !f.Submitted() || f.Account() != want {
		t.Fatalf("account = %+v, submitted %v, want %+v", f.Account(), f.Submitted(), want)
	}
}

func TestFormViewShowsPreview(t *testing.T) {
	var model tea.Model = NewForm(FormOptions{Title: "New account", Taken: free})
	model = send(t, model, append(text("Banco Inter"), keyTab, keyTab, keyTab, keyRight, keyRight, keyTab)...)
	model = send(t, model, text("1234,56")...)
	view := plain(form(t, model).View().Content)
	for _, want := range []string{"New account", "Name", "Code", "Description", "Currency", "Opening balance", "Color", "BINTE", "R$ 1.234,56", "red", "esc"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
}
