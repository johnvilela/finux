package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"finux/src/account"
	"finux/src/category"
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

func TestCategoryFormCreateFlow(t *testing.T) {
	var model tea.Model = NewCategoryForm(CategoryFormOptions{Taken: free})
	view := plain(form(t, model).View().Content)
	for _, want := range []string{"New category", "Name", "Code", "Description", "Color", "red expense", "green income", "grey neutral", "esc"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"Currency", "Opening balance", "R$", "$0"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("category form shows %q:\n%s", unwanted, view)
		}
	}
	model = send(t, model, text("Pet Care")...)
	if got := form(t, model).Value(FieldCode); got != "PCARE" {
		t.Fatalf("suggested code = %q, want PCARE", got)
	}
	model = send(t, model, keyTab, keyTab)
	model = send(t, model, append(text("vet"), keyTab)...)
	if f := form(t, model); f.Focus() != FieldColor || f.Value(FieldColor) != "red" {
		t.Fatalf("focus = %v, color = %q; want the color field with red", f.Focus(), f.Value(FieldColor))
	}
	model = send(t, model, keyDown, keyUp, keyRight)
	if got := form(t, model).Value(FieldColor); got != "green" {
		t.Fatalf("color after right = %q, want green (up and down do nothing)", got)
	}
	model = send(t, model, keyRight, keyRight, keyRight)
	if got := form(t, model).Value(FieldColor); got != "grey" {
		t.Fatalf("color at the right end = %q, want grey", got)
	}
	if view := plain(form(t, model).View().Content); !strings.Contains(view, "PCARE  Pet Care  neutral") {
		t.Fatalf("preview lacks the category line:\n%s", view)
	}
	model = send(t, model, keyLeft, keyEnter)
	f := form(t, model)
	want := category.Category{Name: "Pet Care", Code: "PCARE", Description: "vet", Color: "green"}
	if !f.Submitted() || f.Category() != want {
		t.Fatalf("category = %+v, submitted %v; want %+v", f.Category(), f.Submitted(), want)
	}
}

func TestCategoryFormValidationAndEdit(t *testing.T) {
	taken := func(code string) bool { return code == "FOOD" }
	var model tea.Model = NewCategoryForm(CategoryFormOptions{Taken: taken})
	model = send(t, model, keySave)
	if f := form(t, model); f.Submitted() || f.Focus() != FieldName || f.Error() == "" {
		t.Fatalf("empty category saved: focus %v", f.Focus())
	}
	model = send(t, model, append(text("Food"), keySave)...)
	if f := form(t, model); !f.Submitted() || f.Category().Code != "FOOD2" {
		t.Fatalf("suggestion with taken code = %q, error %q", f.Category().Code, f.Error())
	}

	initial := category.Category{ID: 7, Name: "Food", Code: "FOOD", Description: "Groceries", Color: "grey"}
	model = NewCategoryForm(CategoryFormOptions{Title: "Edit FOOD", Initial: initial, Edit: true, Taken: free})
	if view := plain(form(t, model).View().Content); !strings.Contains(view, "Edit FOOD") {
		t.Fatalf("edit title:\n%s", view)
	}
	model = send(t, model, append(text(" and drinks"), keySave)...)
	got := form(t, model).Category()
	initial.Name = "Food and drinks"
	if got != initial {
		t.Fatalf("edited = %+v, want %+v (code and color kept)", got, initial)
	}

	model = NewCategoryForm(CategoryFormOptions{Initial: category.Category{Name: "Gifts", Color: "green"}, Preset: []Field{FieldName, FieldColor}, Taken: free})
	if f := form(t, model); f.Focus() != FieldCode || f.Value(FieldCode) != "GIFTS" {
		t.Fatalf("preset form: focus %v, code %q", f.Focus(), f.Value(FieldCode))
	}
	model = send(t, model, keyTab, keyEnter)
	if f := form(t, model); !f.Submitted() || f.Category() != (category.Category{Name: "Gifts", Code: "GIFTS", Color: "green"}) {
		t.Fatalf("preset category = %+v", f.Category())
	}
}

// boxWidths returns the width of the narrowest and the widest line of a
// screen in a box.
func boxWidths(view string) (narrowest, widest int) {
	lines := strings.Split(strings.TrimRight(plain(view), "\n"), "\n")
	narrowest = lipglossWidth(lines[0])
	for _, line := range lines {
		narrowest, widest = min(narrowest, lipglossWidth(line)), max(widest, lipglossWidth(line))
	}
	return narrowest, widest
}

func TestFormUsesTheTerminalWidth(t *testing.T) {
	long := strings.Repeat("long text ", 7)
	var model tea.Model = NewForm(FormOptions{Title: "New account", Taken: free})
	model = send(t, model, append(text("Banco Inter"), keyTab, keyTab)...)
	model = send(t, model, text(long)...)
	for _, width := range []int{60, 120, 200} {
		view := form(t, send(t, model, tea.WindowSizeMsg{Width: width, Height: 40})).View().Content
		if narrowest, widest := boxWidths(view); narrowest != width || widest != width {
			t.Fatalf("width %d: the lines have %d to %d columns:\n%s", width, narrowest, widest, plain(view))
		}
		// A wide form shows all of a long text.
		if got := strings.Contains(plain(view), strings.TrimSpace(long)); got != (width > 60) {
			t.Fatalf("width %d: long text shown = %v:\n%s", width, got, plain(view))
		}
	}
}
