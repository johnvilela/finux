// Package account holds the account model, its rules and its storage.
package account

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"finux/src/money"

	"gorm.io/gorm"
)

// MaxCodeLength is the maximum number of characters in an account code.
const MaxCodeLength = 5

// Account is a place that holds money, such as a bank account or a wallet.
type Account struct {
	ID             uint `gorm:"primaryKey"`
	Name           string
	Description    string
	Code           string `gorm:"uniqueIndex;size:5"`
	Color          string
	Currency       string
	OpeningBalance int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

// Balance returns the current balance in minor units.
// It is the opening balance until transactions exist.
func (a Account) Balance() int64 { return a.OpeningBalance }

// Archived reports whether the account is soft-deleted.
func (a Account) Archived() bool { return a.DeletedAt.Valid }

// Validate checks the rules that do not need the database.
func (a Account) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return errors.New("name is required")
	}
	if err := ValidateCode(a.Code); err != nil {
		return err
	}
	if _, ok := ColorByKey(a.Color); !ok {
		return fmt.Errorf("unknown color %q: use one of %s", a.Color, strings.Join(ColorKeys(), ", "))
	}
	if !money.Valid(a.Currency) {
		return fmt.Errorf("unknown currency %q: use one of %s", a.Currency, strings.Join(money.Codes(), ", "))
	}
	return nil
}

// NormalizeCode trims a code and makes it uppercase.
func NormalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// ValidateCode checks that a code has 1 to 5 characters from A-Z and 0-9
// and at least one letter, so that a code is never mistaken for an id.
func ValidateCode(code string) error {
	if code == "" {
		return errors.New("code is required")
	}
	if len(code) > MaxCodeLength {
		return fmt.Errorf("code %q is too long: maximum is %d characters", code, MaxCodeLength)
	}
	hasLetter := false
	for _, r := range code {
		switch {
		case r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
		default:
			return fmt.Errorf("code %q is invalid: use only letters A-Z and digits", code)
		}
	}
	if !hasLetter {
		return fmt.Errorf("code %q is invalid: use at least one letter", code)
	}
	return nil
}

var accents = strings.NewReplacer(
	"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A", "Å", "A",
	"É", "E", "È", "E", "Ê", "E", "Ë", "E",
	"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
	"Ó", "O", "Ò", "O", "Ô", "O", "Õ", "O", "Ö", "O",
	"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
	"Ç", "C", "Ñ", "N",
)

// SuggestCode makes a code from an account name. One word gives its first
// five characters. Many words give the initials of all words but the last,
// then characters of the last word. If the code is taken, a number replaces
// its tail. It returns "" when the name gives no valid code.
func SuggestCode(name string, taken func(string) bool) string {
	words := strings.FieldsFunc(accents.Replace(strings.ToUpper(name)), func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
	if len(words) == 0 {
		return ""
	}
	var base string
	for _, word := range words[:len(words)-1] {
		if len(base) < MaxCodeLength-1 {
			base += word[:1]
		}
	}
	base += words[len(words)-1]
	if len(base) > MaxCodeLength {
		base = base[:MaxCodeLength]
	}
	if ValidateCode(base) != nil {
		return ""
	}
	if !taken(base) {
		return base
	}
	for number := 2; number <= 99; number++ {
		suffix := strconv.Itoa(number)
		stem := base
		if len(stem) > MaxCodeLength-len(suffix) {
			stem = stem[:MaxCodeLength-len(suffix)]
		}
		if code := stem + suffix; !taken(code) {
			return code
		}
	}
	return ""
}

// Color is one entry of the account color palette.
type Color struct {
	Key, Hex string
}

// Palette is the fixed list of colors an account can use.
var Palette = []Color{
	{"red", "#EF4444"}, {"orange", "#F97316"}, {"amber", "#F59E0B"}, {"yellow", "#EAB308"},
	{"lime", "#84CC16"}, {"green", "#22C55E"}, {"emerald", "#10B981"}, {"teal", "#14B8A6"},
	{"cyan", "#06B6D4"}, {"sky", "#0EA5E9"}, {"blue", "#3B82F6"}, {"indigo", "#6366F1"},
	{"violet", "#8B5CF6"}, {"purple", "#A855F7"}, {"pink", "#EC4899"}, {"slate", "#64748B"},
}

// ColorByKey finds a palette color by its key.
func ColorByKey(key string) (Color, bool) {
	for _, color := range Palette {
		if color.Key == key {
			return color, true
		}
	}
	return Color{}, false
}

// ColorKeys returns the palette keys in palette order.
func ColorKeys() []string {
	keys := make([]string, len(Palette))
	for i, color := range Palette {
		keys[i] = color.Key
	}
	return keys
}
