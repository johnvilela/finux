// Package goal holds the goals of the user: an amount of money to save,
// to pay off or to put aside for a purchase.
package goal

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"finux/src/code"
	"finux/src/money"

	"gorm.io/gorm"
)

// The types of a goal. The type tells how a linked transaction counts.
const (
	// Save is money to put aside. An income adds; an expense removes.
	Save = "save"
	// PayOff is a debt to pay. An expense adds; an income removes.
	PayOff = "payoff"
	// Purchase is money to put aside for one thing. It counts as Save.
	Purchase = "purchase"
)

// The statuses of a goal. They come from its progress and its dates.
const (
	StatusActive  = "active"
	StatusReached = "reached"
	StatusOverdue = "overdue"
)

const (
	// MaxTitle and MaxDescription are the limits of the texts, in
	// characters.
	MaxTitle       = 60
	MaxDescription = 200

	isoDate = "2006-01-02"
)

// Types returns the types of a goal.
func Types() []string { return []string{Save, PayOff, Purchase} }

// ValidType reports whether kind is a type of a goal.
func ValidType(kind string) bool { return slices.Contains(Types(), kind) }

// Goal is an amount of money the user wants to reach.
type Goal struct {
	ID          uint   `gorm:"primaryKey"`
	Title       string `gorm:"size:60"`
	Description string `gorm:"size:200"`
	Code        string `gorm:"uniqueIndex;size:5"`
	Type        string
	Currency    string
	// Target is the amount to reach, in minor units.
	Target int64
	// DueDate is the final date as YYYY-MM-DD. It is empty for a goal
	// without a final date.
	DueDate   string `gorm:"size:10"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	// Saved is what the linked transactions add up to; it can be below
	// zero. Links is the number of linked transactions, where a transfer
	// counts as one. The store fills them.
	Saved int64 `gorm:"-"`
	Links int64 `gorm:"-"`
}

// Progress returns the amount that counts toward the target. It is
// never below zero.
func (g Goal) Progress() int64 { return max(g.Saved, 0) }

// Remaining returns what the goal still needs.
func (g Goal) Remaining() int64 { return max(g.Target-g.Progress(), 0) }

// Percent returns the progress as a whole percentage, from 0 to 100.
func (g Goal) Percent() int {
	if g.Target <= 0 || g.Progress() >= g.Target {
		if g.Target > 0 {
			return 100
		}
		return 0
	}
	// Divide first: the product can be too large for the largest targets.
	return int(float64(g.Progress()) / float64(g.Target) * 100)
}

// Status returns the status of the goal on the date of today, as
// YYYY-MM-DD. A goal that reached its target is never overdue.
func (g Goal) Status(today string) string {
	switch {
	case g.Progress() >= g.Target:
		return StatusReached
	case g.DueDate != "" && today > g.DueDate:
		return StatusOverdue
	}
	return StatusActive
}

// Archived reports whether the goal is archived.
func (g Goal) Archived() bool { return g.DeletedAt.Valid }

// Validate checks the fields of a goal.
func (g Goal) Validate() error {
	title := strings.TrimSpace(g.Title)
	if title == "" {
		return errors.New("title is required")
	}
	if utf8.RuneCountInString(title) > MaxTitle {
		return fmt.Errorf("title is too long: the limit is %d characters", MaxTitle)
	}
	if utf8.RuneCountInString(strings.TrimSpace(g.Description)) > MaxDescription {
		return fmt.Errorf("description is too long: the limit is %d characters", MaxDescription)
	}
	if err := code.Validate(g.Code); err != nil {
		return err
	}
	if !ValidType(g.Type) {
		return fmt.Errorf("unknown type %q: use one of %s", g.Type, strings.Join(Types(), ", "))
	}
	if !money.Valid(g.Currency) {
		return fmt.Errorf("unknown currency %q: use one of %s", g.Currency, strings.Join(money.Codes(), ", "))
	}
	if g.Target <= 0 {
		return errors.New("target must be more than zero")
	}
	if g.DueDate != "" {
		if _, err := time.Parse(isoDate, g.DueDate); err != nil {
			return fmt.Errorf("invalid final date %q", g.DueDate)
		}
	}
	return nil
}

// Effect returns what one linked transaction adds to the goal: above
// zero when it adds, below zero when it removes. kind and amount are the
// type and the signed amount of the transaction. A transfer counts one
// time, on the side that leaves its account.
func (g Goal) Effect(kind string, amount int64) int64 {
	switch {
	case kind == "transfer" && amount < 0:
		return -amount
	case kind == "income" || kind == "expense":
		if g.Type == PayOff {
			return -amount
		}
		return amount
	}
	return 0
}
