// Package account holds the account model, its rules and its storage.
package account

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"finux/src/code"
	"finux/src/money"

	"gorm.io/gorm"
)

// Account is a place that holds money, such as a bank account or a wallet.
type Account struct {
	ID             uint `gorm:"primaryKey"`
	Name           string
	Description    string
	Code           string `gorm:"uniqueIndex;size:5"`
	Color          string
	Currency       string
	OpeningBalance int64
	// Owner is the person the account belongs to. It is empty for an
	// account of the user. The lists and the totals leave out the
	// accounts of other persons unless the user asks for them.
	Owner     string `gorm:"size:40;not null;default:''"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	// Movement is the sum of the transactions of the account. The store
	// fills it; it is not a column.
	Movement int64 `gorm:"-"`
}

// Balance returns the current balance in minor units: the opening
// balance and the transactions of the account.
func (a Account) Balance() int64 { return a.OpeningBalance + a.Movement }

// Mine reports whether the account belongs to the user.
func (a Account) Mine() bool { return a.Owner == "" }

// Mine returns the accounts that belong to the user.
func Mine(accounts []Account) []Account {
	mine := make([]Account, 0, len(accounts))
	for _, current := range accounts {
		if current.Mine() {
			mine = append(mine, current)
		}
	}
	return mine
}

// OwnerSize is the largest number of characters of an owner.
const OwnerSize = 40

// CheckOwner checks the owner of an account or a credit card.
func CheckOwner(owner string) error {
	if utf8.RuneCountInString(owner) > OwnerSize {
		return fmt.Errorf("owner must have at most %d characters", OwnerSize)
	}
	return nil
}

// Archived reports whether the account is soft-deleted.
func (a Account) Archived() bool { return a.DeletedAt.Valid }

// Validate checks the rules that do not need the database.
func (a Account) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return errors.New("name is required")
	}
	if err := code.Validate(a.Code); err != nil {
		return err
	}
	if _, ok := ColorByKey(a.Color); !ok {
		return fmt.Errorf("unknown color %q: use one of %s", a.Color, strings.Join(ColorKeys(), ", "))
	}
	if !money.Valid(a.Currency) {
		return fmt.Errorf("unknown currency %q: use one of %s", a.Currency, strings.Join(money.Codes(), ", "))
	}
	return CheckOwner(a.Owner)
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
