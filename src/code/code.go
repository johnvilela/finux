// Package code holds the rules of the short codes that identify records,
// such as INTER for an account.
package code

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// MaxLength is the maximum number of characters in a code.
const MaxLength = 5

// Normalize trims a code and makes it uppercase.
func Normalize(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// Validate checks that a code has 1 to 5 characters from A-Z and 0-9
// and at least one letter, so that a code is never mistaken for an id.
func Validate(code string) error {
	if code == "" {
		return errors.New("code is required")
	}
	if len(code) > MaxLength {
		return fmt.Errorf("code %q is too long: maximum is %d characters", code, MaxLength)
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

// Suggest makes a code from a name. One word gives its first
// five characters. Many words give the initials of all words but the last,
// then characters of the last word. If the code is taken, a number replaces
// its tail. It returns "" when the name gives no valid code.
func Suggest(name string, taken func(string) bool) string {
	words := strings.FieldsFunc(accents.Replace(strings.ToUpper(name)), func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
	if len(words) == 0 {
		return ""
	}
	var base string
	for _, word := range words[:len(words)-1] {
		if len(base) < MaxLength-1 {
			base += word[:1]
		}
	}
	base += words[len(words)-1]
	if len(base) > MaxLength {
		base = base[:MaxLength]
	}
	if Validate(base) != nil {
		return ""
	}
	if !taken(base) {
		return base
	}
	for number := 2; number <= 99; number++ {
		suffix := strconv.Itoa(number)
		stem := base
		if len(stem) > MaxLength-len(suffix) {
			stem = stem[:MaxLength-len(suffix)]
		}
		if code := stem + suffix; !taken(code) {
			return code
		}
	}
	return ""
}
