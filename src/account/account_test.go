package account

import "testing"

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
