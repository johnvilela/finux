// Package schedule holds the scheduled transactions of the user: an
// expense or an income that will happen on a date, one time or again
// and again. A schedule is not a transaction. Each of its occurrences
// waits until the user pays it, which creates a transaction, or skips it.
package schedule

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"finux/src/budget"
	"finux/src/code"
	"finux/src/money"

	"gorm.io/gorm"
)

// The types of the transactions a schedule creates.
const (
	Expense = "expense"
	Income  = "income"
)

// The statuses of a schedule on a date.
const (
	// StatusDue is a schedule with an occurrence on or before the date.
	StatusDue      = "due"
	StatusUpcoming = "upcoming"
	// StatusDone is a schedule with no occurrence left.
	StatusDone = "done"
)

const (
	// MaxTitle and MaxDescription are the limits of the texts, in
	// characters.
	MaxTitle       = 60
	MaxDescription = 200
	// MaxTimes is the largest number of occurrences a schedule can have.
	MaxTimes = 1000
	// maxPending is the largest number of occurrences Pending returns.
	maxPending = 500

	isoDate = "2006-01-02"
)

// Types returns the types of a schedule.
func Types() []string { return []string{Expense, Income} }

// Schedule is a transaction that will happen: its values, its first
// date and how it repeats.
type Schedule struct {
	ID          uint   `gorm:"primaryKey"`
	Title       string `gorm:"size:60"`
	Description string `gorm:"size:200"`
	Code        string `gorm:"uniqueIndex;size:5"`
	Type        string
	// Value is the amount, always positive, in minor units of the
	// currency of the account or the card.
	Value int64
	// A schedule has an account or a credit card, never the two. It can
	// have none: the user decides which one pays when the occurrence is
	// paid.
	AccountID uint `gorm:"index"`
	CardID    uint `gorm:"index"`
	// Currency is the currency of the value of a schedule without an
	// account and without a card. A schedule that has one of them uses
	// the currency of that record.
	Currency   string `gorm:"size:3"`
	CategoryID uint   `gorm:"index"`
	// GoalID is the goal its transactions count toward, or 0.
	GoalID uint `gorm:"index"`
	// Tags holds the tags separated by one space: "#fixed #home".
	Tags string
	// StartDate is the date of the first occurrence, as YYYY-MM-DD.
	StartDate string `gorm:"size:10"`
	// Every and Unit are the time between two occurrences: every 2 weeks
	// is 2 and "week". A one-time schedule has 0 and an empty unit.
	Every int
	Unit  string
	// EndDate is the last day an occurrence can have. Times is the number
	// of occurrences. An empty date and 0 mean no end.
	EndDate string `gorm:"size:10"`
	Times   int
	// Auto makes an occurrence a transaction on its date, without a
	// confirmation of the user.
	Auto bool
	// Done is the number of occurrences that the user paid or skipped.
	// They go in order, so the next occurrence has this index.
	Done      int
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// Placed reports whether the schedule has an account or a credit card.
func (s Schedule) Placed() bool { return s.AccountID != 0 || s.CardID != 0 }

// Recurring reports whether the schedule has more than one occurrence.
func (s Schedule) Recurring() bool { return s.Every > 0 }

// Archived reports whether the schedule is archived.
func (s Schedule) Archived() bool { return s.DeletedAt.Valid }

// Recurrence returns the period in words, for example "every 2 weeks".
func (s Schedule) Recurrence() string { return budget.Describe(s.Every, s.Unit) }

// occurrence returns the date of the occurrence with the index. It
// reports false when the schedule ends before it.
func (s Schedule) occurrence(index int) (string, bool) {
	if !s.Recurring() {
		return s.StartDate, index == 0
	}
	if s.Times > 0 && index >= s.Times {
		return "", false
	}
	date := budget.Step(s.StartDate, s.Every, s.Unit, index)
	return date, s.EndDate == "" || date <= s.EndDate
}

// Next returns the date of the first occurrence that is not paid and
// not skipped. It reports false for a schedule with no occurrence left.
func (s Schedule) Next() (string, bool) { return s.occurrence(s.Done) }

// Pending returns the dates, up to the date until, of the occurrences
// that are not paid and not skipped, the oldest first.
func (s Schedule) Pending(until string) []string {
	var dates []string
	for index := s.Done; len(dates) < maxPending; index++ {
		date, ok := s.occurrence(index)
		if !ok || date > until {
			break
		}
		dates = append(dates, date)
	}
	return dates
}

// Overdue returns the number of occurrences with a date before today
// that are not paid and not skipped.
func (s Schedule) Overdue(today string) int {
	return len(slices.DeleteFunc(s.Pending(today), func(date string) bool { return date == today }))
}

// Left returns the number of occurrences that are not paid and not
// skipped, or -1 for a schedule without an end.
func (s Schedule) Left() int {
	if s.Recurring() && s.Times == 0 && s.EndDate == "" {
		return -1
	}
	left := 0
	for index := s.Done; left <= MaxTimes; index++ {
		if _, ok := s.occurrence(index); !ok {
			break
		}
		left++
	}
	return left
}

// Status returns the status of the schedule on the date of today.
func (s Schedule) Status(today string) string {
	next, ok := s.Next()
	switch {
	case !ok:
		return StatusDone
	case next <= today:
		return StatusDue
	}
	return StatusUpcoming
}

// Rebase makes start the date of the next occurrence. The occurrences
// that are paid or skipped leave the count of the schedule.
func (s *Schedule) Rebase(start string) {
	if s.Times > 0 {
		s.Times = max(s.Times-s.Done, 1)
	}
	s.StartDate, s.Done = start, 0
}

// Validate checks the fields of a schedule.
func (s Schedule) Validate() error {
	title := strings.TrimSpace(s.Title)
	if title == "" {
		return errors.New("title is required")
	}
	if utf8.RuneCountInString(title) > MaxTitle {
		return fmt.Errorf("title is too long: the limit is %d characters", MaxTitle)
	}
	if utf8.RuneCountInString(strings.TrimSpace(s.Description)) > MaxDescription {
		return fmt.Errorf("description is too long: the limit is %d characters", MaxDescription)
	}
	if err := code.Validate(s.Code); err != nil {
		return err
	}
	if !slices.Contains(Types(), s.Type) {
		return fmt.Errorf("unknown type %q: use one of %s", s.Type, strings.Join(Types(), ", "))
	}
	if s.Value <= 0 {
		return errors.New("value must be more than zero")
	}
	switch {
	case s.AccountID != 0 && s.CardID != 0:
		return errors.New("a schedule has an account or a credit card, not the two")
	case !s.Placed() && s.Currency == "":
		return errors.New("currency is required: the schedule has no account")
	case !s.Placed() && !money.Valid(s.Currency):
		return fmt.Errorf("unknown currency %q: use one of %s", s.Currency, strings.Join(money.Codes(), ", "))
	case !s.Placed() && s.Auto:
		return errors.New("an automatic schedule needs an account or a credit card")
	case s.CategoryID == 0:
		return errors.New("category is required")
	}
	if _, err := time.Parse(isoDate, s.StartDate); err != nil {
		return fmt.Errorf("invalid date %q", s.StartDate)
	}
	if s.Every < 0 || s.Every > budget.MaxEvery {
		return fmt.Errorf("period must be 1 or more, and %d at most", budget.MaxEvery)
	}
	if s.Recurring() && !slices.Contains(budget.Units(), s.Unit) {
		return fmt.Errorf("unknown period %q: use one of %s", s.Unit, strings.Join(budget.Units(), ", "))
	}
	if s.Times < 0 || s.Times > MaxTimes {
		return fmt.Errorf("times must be from 0 to %d", MaxTimes)
	}
	if !s.Recurring() && (s.Times != 0 || s.EndDate != "") {
		return errors.New("an end date and a number of times are only for a schedule that repeats")
	}
	if s.EndDate != "" {
		if _, err := time.Parse(isoDate, s.EndDate); err != nil {
			return fmt.Errorf("invalid end date %q", s.EndDate)
		}
		if s.EndDate < s.StartDate {
			return errors.New("end date is before the date of the first occurrence")
		}
	}
	return nil
}
