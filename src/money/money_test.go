package money

import "testing"

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		text, currency string
		want           int64
	}{
		{"1234.56", "BRL", 123456},
		{"1234,56", "BRL", 123456},
		{"1234", "USD", 123400},
		{"0.5", "EUR", 50},
		{"-10,05", "BRL", -1005},
		{"+7", "USD", 700},
		{" 12.30 ", "USD", 1230},
		{"0.12345678", "BTC", 12345678},
		{"1", "BTC", 100000000},
		{"0", "BRL", 0},
	} {
		got, err := Parse(tc.text, tc.currency)
		if err != nil || got != tc.want {
			t.Errorf("Parse(%q, %q) = %d, %v; want %d", tc.text, tc.currency, got, err, tc.want)
		}
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	for _, tc := range []struct{ text, currency string }{
		{"", "BRL"},
		{"abc", "BRL"},
		{"1.234,56", "BRL"},
		{"1,234.56", "USD"},
		{"1.234", "USD"},
		{"0.123456789", "BTC"},
		{"12.", "USD"},
		{".5", "USD"},
		{"-", "USD"},
		{"1 2", "USD"},
		{"99999999999999999999", "USD"},
		{"92233720368547758.08", "USD"},
		{"10", "XYZ"},
	} {
		if got, err := Parse(tc.text, tc.currency); err == nil {
			t.Errorf("Parse(%q, %q) = %d, want error", tc.text, tc.currency, got)
		}
	}
}

func TestFormat(t *testing.T) {
	for _, tc := range []struct {
		amount   int64
		currency string
		want     string
	}{
		{123456, "BRL", "R$ 1.234,56"},
		{123456, "USD", "$1,234.56"},
		{123456, "EUR", "€ 1.234,56"},
		{12345678, "BTC", "₿ 0.12345678"},
		{0, "BRL", "R$ 0,00"},
		{5, "USD", "$0.05"},
		{-1005, "BRL", "-R$ 10,05"},
		{-1005, "USD", "-$10.05"},
		{100000000000, "USD", "$1,000,000,000.00"},
		{99999, "EUR", "€ 999,99"},
		{123400000000, "BTC", "₿ 1,234.00000000"},
		{-9223372036854775808, "USD", "-$92,233,720,368,547,758.08"},
	} {
		if got := Format(tc.amount, tc.currency); got != tc.want {
			t.Errorf("Format(%d, %q) = %q, want %q", tc.amount, tc.currency, got, tc.want)
		}
	}
}

func TestCodes(t *testing.T) {
	want := []string{"USD", "EUR", "BRL", "BTC"}
	got := Codes()
	if len(got) != len(want) {
		t.Fatalf("Codes() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] || !Valid(want[i]) {
			t.Fatalf("Codes() = %v, want %v, all valid", got, want)
		}
	}
	if Valid("usd") || Valid("XYZ") {
		t.Fatal("Valid accepts unknown currency")
	}
}

func TestPlaceholder(t *testing.T) {
	for code, want := range map[string]string{"USD": "0.00", "EUR": "0,00", "BRL": "0,00", "BTC": "0.00000000", "XYZ": "0"} {
		if got := Placeholder(code); got != want {
			t.Errorf("Placeholder(%q) = %q, want %q", code, got, want)
		}
	}
}
