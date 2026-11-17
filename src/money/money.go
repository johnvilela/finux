// Package money parses and formats amounts stored as integer minor units.
package money

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type currency struct {
	code, symbol       string
	decimals           int
	thousands, decimal string
	spaced             bool
}

var currencies = []currency{
	{"USD", "$", 2, ",", ".", false},
	{"EUR", "€", 2, ".", ",", true},
	{"BRL", "R$", 2, ".", ",", true},
	{"BTC", "₿", 8, ",", ".", true},
}

func find(code string) (currency, bool) {
	for _, c := range currencies {
		if c.code == code {
			return c, true
		}
	}
	return currency{}, false
}

// Codes returns the supported currency codes in display order.
func Codes() []string {
	codes := make([]string, len(currencies))
	for i, c := range currencies {
		codes[i] = c.code
	}
	return codes
}

// Valid reports whether code is a supported currency.
func Valid(code string) bool {
	_, ok := find(code)
	return ok
}

// Placeholder returns a zero amount in the input format of a currency,
// for example "0,00" for BRL. Unknown currencies give "0".
func Placeholder(code string) string {
	c, ok := find(code)
	if !ok {
		return "0"
	}
	return "0" + c.decimal + strings.Repeat("0", c.decimals)
}

// Parse converts text such as "1234.56" or "-1234,56" to minor units.
// It accepts one decimal separator and no thousands separators.
func Parse(text, code string) (int64, error) {
	c, ok := find(code)
	if !ok {
		return 0, fmt.Errorf("unknown currency %q", code)
	}
	text = strings.TrimSpace(text)
	negative := strings.HasPrefix(text, "-")
	digits := text
	if negative || strings.HasPrefix(text, "+") {
		digits = text[1:]
	}
	whole, fraction, hasFraction := strings.Cut(strings.Replace(digits, ",", ".", 1), ".")
	if !allDigits(whole) || hasFraction && !allDigits(fraction) {
		return 0, fmt.Errorf("invalid amount %q: use a number such as 1234.56", text)
	}
	if len(fraction) > c.decimals {
		return 0, fmt.Errorf("invalid amount %q: %s has at most %d decimal places", text, code, c.decimals)
	}
	fraction += strings.Repeat("0", c.decimals-len(fraction))
	magnitude, err := strconv.ParseUint(whole+fraction, 10, 64)
	if err != nil || magnitude > math.MaxInt64 {
		return 0, fmt.Errorf("invalid amount %q: value is too large", text)
	}
	if negative {
		return -int64(magnitude), nil
	}
	return int64(magnitude), nil
}

func allDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Format renders minor units in the style of the currency, for example "R$ 1.234,56".
func Format(amount int64, code string) string {
	c, ok := find(code)
	if !ok {
		return strconv.FormatInt(amount, 10) + " " + code
	}
	magnitude := uint64(amount)
	sign := ""
	if amount < 0 {
		magnitude = -magnitude
		sign = "-"
	}
	digits := strconv.FormatUint(magnitude, 10)
	if len(digits) <= c.decimals {
		digits = strings.Repeat("0", c.decimals-len(digits)+1) + digits
	}
	whole, fraction := digits[:len(digits)-c.decimals], digits[len(digits)-c.decimals:]
	var grouped strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			grouped.WriteString(c.thousands)
		}
		grouped.WriteRune(r)
	}
	space := ""
	if c.spaced {
		space = " "
	}
	return sign + c.symbol + space + grouped.String() + c.decimal + fraction
}
