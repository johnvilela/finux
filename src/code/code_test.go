package code

import "testing"

func TestSuggest(t *testing.T) {
	none := func(string) bool { return false }
	for _, tc := range []struct{ name, want string }{
		{"Nubank", "NUBAN"},
		{"Banco Inter", "BINTE"},
		{"inter", "INTER"},
		{"C6", "C6"},
		{"Itaú", "ITAU"},
		{"Poupança São João", "PSJOA"},
		{"Banco do Brasil", "BDBRA"},
		{"  Caixa   Econômica  ", "CECON"},
		{"One Two Three Four Five Six", "OTTFS"},
		{"Wise-USD", "WUSD"},
		{"", ""},
		{"123", ""},
		{"!!!", ""},
	} {
		if got := Suggest(tc.name, none); got != tc.want {
			t.Errorf("Suggest(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSuggestAvoidsTakenCodes(t *testing.T) {
	taken := map[string]bool{"BINTE": true}
	isTaken := func(code string) bool { return taken[code] }
	if got := Suggest("Banco Inter", isTaken); got != "BINT2" {
		t.Fatalf("first collision = %q, want BINT2", got)
	}
	for digit := '2'; digit <= '9'; digit++ {
		taken["BINT"+string(digit)] = true
	}
	if got := Suggest("Banco Inter", isTaken); got != "BIN10" {
		t.Fatalf("after digits 2-9 taken = %q, want BIN10", got)
	}
	taken["C6"] = true
	if got := Suggest("C6", isTaken); got != "C62" {
		t.Fatalf("short collision = %q, want C62", got)
	}
}

func TestNormalizeAndValidate(t *testing.T) {
	if got := Normalize(" inter "); got != "INTER" {
		t.Fatalf("Normalize = %q, want INTER", got)
	}
	for _, code := range []string{"INTER", "C6", "A", "1A2B3"} {
		if err := Validate(code); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", code, err)
		}
	}
	for _, code := range []string{"", "TOOLONG", "inter", "12345", "A B", "A-B", "ÇA"} {
		if err := Validate(code); err == nil {
			t.Errorf("Validate(%q) = nil, want error", code)
		}
	}
}
