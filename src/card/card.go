// Package card holds the credit card and bill models, their rules and
// their storage.
package card

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"finux/src/account"
	"finux/src/code"
	"finux/src/money"

	"gorm.io/gorm"
)

const (
	isoDate   = "2006-01-02"
	isoPeriod = "2006-01"
)

// Card is a credit card. Its purchases go to bills; it holds no money.
type Card struct {
	ID          uint `gorm:"primaryKey"`
	Name        string
	Description string
	// Code is unique across credit cards and accounts.
	Code     string `gorm:"uniqueIndex;size:5"`
	Color    string
	Currency string
	// Limit is the credit limit in minor units.
	Limit int64
	// ClosingDay and DueDay are days of the month, 1 to 31. A month
	// with fewer days uses its last day.
	ClosingDay int
	DueDay     int
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
	// Used is the total of the bills that are not paid. The store fills
	// it; it is not a column.
	Used int64 `gorm:"-"`
}

// Available returns the part of the limit that is free.
func (c Card) Available() int64 { return c.Limit - c.Used }

// Archived reports whether the card is soft-deleted.
func (c Card) Archived() bool { return c.DeletedAt.Valid }

// CheckDay checks a closing day or a due day.
func CheckDay(label string, day int) error {
	if day < 1 || day > 31 {
		return fmt.Errorf("%s must be from 1 to 31", label)
	}
	return nil
}

// Validate checks the rules that do not need the database.
func (c Card) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("name is required")
	}
	if err := code.Validate(c.Code); err != nil {
		return err
	}
	if _, ok := account.ColorByKey(c.Color); !ok {
		return fmt.Errorf("unknown color %q: use one of %s", c.Color, strings.Join(account.ColorKeys(), ", "))
	}
	if !money.Valid(c.Currency) {
		return fmt.Errorf("unknown currency %q: use one of %s", c.Currency, strings.Join(money.Codes(), ", "))
	}
	if c.Limit < 0 {
		return errors.New("limit cannot be negative")
	}
	if err := CheckDay("closing day", c.ClosingDay); err != nil {
		return err
	}
	return CheckDay("due day", c.DueDay)
}

// dayOf returns the given day of a month, or the last day of the month
// if the month is shorter.
func dayOf(year int, month time.Month, day int) time.Time {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	return time.Date(year, month, min(day, last), 0, 0, 0, 0, time.UTC)
}

// dueSameMonth reports whether the bill is due in the month it closes.
func (c Card) dueSameMonth() bool { return c.DueDay > c.ClosingDay }

// cycle returns the bill that closes in the given month.
func (c Card) cycle(year int, month time.Month) (period, closesOn, dueOn string) {
	closes := dayOf(year, month, c.ClosingDay)
	if !c.dueSameMonth() {
		next := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)
		year, month = next.Year(), next.Month()
	}
	due := dayOf(year, month, c.DueDay)
	return due.Format(isoPeriod), closes.Format(isoDate), due.Format(isoDate)
}

// Cycle returns the bill of a transaction on the given YYYY-MM-DD date:
// its period (the month of the due date, as YYYY-MM), its closing date
// and its due date. A date after the closing day goes to the bill that
// closes in the next month.
func (c Card) Cycle(date string) (period, closesOn, dueOn string) {
	day, err := time.Parse(isoDate, date)
	if err != nil {
		return "", "", ""
	}
	if day.After(dayOf(day.Year(), day.Month(), c.ClosingDay)) {
		day = time.Date(day.Year(), day.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	}
	return c.cycle(day.Year(), day.Month())
}

// CycleOf returns the closing date and the due date of the bill of a
// period.
func (c Card) CycleOf(period string) (closesOn, dueOn string) {
	month, err := time.Parse(isoPeriod, period)
	if err != nil {
		return "", ""
	}
	if !c.dueSameMonth() {
		month = month.AddDate(0, -1, 0)
	}
	_, closesOn, dueOn = c.cycle(month.Year(), month.Month())
	return closesOn, dueOn
}

// NextPeriod returns the period after the given one.
func NextPeriod(period string) string {
	month, err := time.Parse(isoPeriod, period)
	if err != nil {
		return ""
	}
	return month.AddDate(0, 1, 0).Format(isoPeriod)
}

// FormatPeriod shows a YYYY-MM period as MM/YYYY.
func FormatPeriod(period string) string {
	month, err := time.Parse(isoPeriod, period)
	if err != nil {
		return period
	}
	return month.Format("01/2006")
}

// ParsePeriod reads a bill period as MM/YYYY, YYYY/MM or YYYY-MM and
// returns it as YYYY-MM.
func ParsePeriod(text string) (string, error) {
	text = strings.TrimSpace(text)
	for _, layout := range []string{"1/2006", "2006/1", "2006-1"} {
		if month, err := time.Parse(layout, text); err == nil {
			return month.Format(isoPeriod), nil
		}
	}
	return "", fmt.Errorf("invalid bill %q: use MM/YYYY", text)
}

// The statuses of a bill.
const (
	StatusOpen   = "open"
	StatusClosed = "closed"
	StatusLate   = "late"
	StatusPaid   = "paid"
)

// Bill is the statement of a credit card for one period.
type Bill struct {
	ID     uint `gorm:"primaryKey"`
	CardID uint `gorm:"uniqueIndex:idx_bills_card_period"`
	// Period is the month of the due date as YYYY-MM.
	Period string `gorm:"size:7;uniqueIndex:idx_bills_card_period"`
	// ClosesOn and DueOn are YYYY-MM-DD dates. They do not change when
	// the days of the card change.
	ClosesOn string `gorm:"size:10"`
	DueOn    string `gorm:"size:10"`
	// Carried is the value that the payment of the bill before did not
	// cover.
	Carried int64
	// PaidOn is the date of the payment, or empty.
	PaidOn string `gorm:"size:10"`
	// Paid is the value of the payment. Remaining is the value that the
	// payment sent to the next bill.
	Paid      int64
	Remaining int64
	// CarriedTo is the bill that received Remaining, or 0.
	CarriedTo uint
	// PaymentID is the payment transaction, or 0.
	PaymentID uint
	CreatedAt time.Time
	UpdatedAt time.Time
	// Spent is the sum of the transactions of the bill: purchases less
	// refunds. The store fills it; it is not a column.
	Spent int64 `gorm:"-"`
}

// Total returns the value to pay.
func (b Bill) Total() int64 { return b.Carried + b.Spent }

// IsPaid reports whether the bill has a payment.
func (b Bill) IsPaid() bool { return b.PaidOn != "" }

// Status returns the status of the bill on the YYYY-MM-DD date today.
// A bill is open until its closing date, then closed until its due
// date, then late. A bill that owes nothing is never late.
func (b Bill) Status(today string) string {
	switch {
	case b.IsPaid():
		return StatusPaid
	case today <= b.ClosesOn:
		return StatusOpen
	case today <= b.DueOn || b.Total() <= 0:
		return StatusClosed
	}
	return StatusLate
}
