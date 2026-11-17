package account

import "testing"

func TestSuggestCode(t *testing.T) {
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
		if got := SuggestCode(tc.name, none); got != tc.want {
			t.Errorf("SuggestCode(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSuggestCodeAvoidsTakenCodes(t *testing.T) {
	taken := map[string]bool{"BINTE": true}
	isTaken := func(code string) bool { return taken[code] }
	if got := SuggestCode("Banco Inter", isTaken); got != "BINT2" {
		t.Fatalf("first collision = %q, want BINT2", got)
	}
	for digit := '2'; digit <= '9'; digit++ {
		taken["BINT"+string(digit)] = true
	}
	if got := SuggestCode("Banco Inter", isTaken); got != "BIN10" {
		t.Fatalf("after digits 2-9 taken = %q, want BIN10", got)
	}
	taken["C6"] = true
	if got := SuggestCode("C6", isTaken); got != "C62" {
		t.Fatalf("short collision = %q, want C62", got)
	}
}

func TestNormalizeAndValidateCode(t *testing.T) {
	if got := NormalizeCode(" inter "); got != "INTER" {
		t.Fatalf("NormalizeCode = %q, want INTER", got)
	}
	for _, code := range []string{"INTER", "C6", "A", "1A2B3"} {
		if err := ValidateCode(code); err != nil {
			t.Errorf("ValidateCode(%q) = %v, want nil", code, err)
		}
	}
	for _, code := range []string{"", "TOOLONG", "inter", "12345", "A B", "A-B", "ÇA"} {
		if err := ValidateCode(code); err == nil {
			t.Errorf("ValidateCode(%q) = nil, want error", code)
		}
	}
}

func TestPalette(t *testing.T) {
	if len(Palette) != 16 {
		t.Fatalf("palette has %d colors, want 16", len(Palette))
	}
	keys, hexes := map[string]bool{}, map[string]bool{}
	for _, color := range Palette {
		if keys[color.Key] || hexes[color.Hex] || len(color.Hex) != 7 || color.Hex[0] != '#' {
			t.Fatalf("bad or duplicate palette entry %+v", color)
		}
		keys[color.Key], hexes[color.Hex] = true, true
		if got, ok := ColorByKey(color.Key); !ok || got != color {
			t.Fatalf("ColorByKey(%q) = %+v, %v", color.Key, got, ok)
		}
	}
	if _, ok := ColorByKey("magenta"); ok {
		t.Fatal("ColorByKey accepts unknown color")
	}
}

func TestAccountValidate(t *testing.T) {
	valid := Account{Name: "Banco Inter", Code: "INTER", Color: "orange", Currency: "BRL"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid account: %v", err)
	}
	for name, change := range map[string]func(*Account){
		"empty name":       func(a *Account) { a.Name = "  " },
		"bad code":         func(a *Account) { a.Code = "inter!" },
		"unknown color":    func(a *Account) { a.Color = "magenta" },
		"unknown currency": func(a *Account) { a.Currency = "JPY" },
	} {
		account := valid
		change(&account)
		if err := account.Validate(); err == nil {
			t.Errorf("%s: Validate() = nil, want error", name)
		}
	}
}
