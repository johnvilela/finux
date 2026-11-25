// Package transaction holds the transaction model, its rules and its
// storage.
package transaction

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"finux/src/account"
	"finux/src/card"
	"finux/src/category"
	"finux/src/goal"
	"finux/src/schedule"
)

// The types of a transaction.
const (
	Expense  = "expense"
	Income   = "income"
	Transfer = "transfer"
	// Payment is the payment of a credit card bill from an account.
	// Only PayBill creates it.
	Payment = "payment"
)

const (
	// MaxTitle and MaxDescription are the size limits in characters.
	MaxTitle       = 60
	MaxDescription = 200
	// MaxTag is the size limit of a tag, without the # sign.
	MaxTag = 30
	// MaxInstallments is the largest number of parts of a transaction
	// in installments.
	MaxInstallments = 120

	isoDate     = "2006-01-02"
	displayDate = "02/01/2006"
)

// Types returns the transaction types that a user can choose, in
// display order.
func Types() []string { return []string{Expense, Income, Transfer} }

// Neutral reports whether a type moves money without being an expense
// or an income: a transfer or a payment of a bill.
func Neutral(kind string) bool { return kind == Transfer || kind == Payment }

// ValidType reports whether kind is a type that a user can choose.
func ValidType(kind string) bool {
	return kind == Expense || kind == Income || kind == Transfer
}

// Accepts reports whether a transaction of the type kind can use a
// category of the given color. Red is for expenses, green for incomes
// and grey for all types. A transfer and a payment use only grey.
func Accepts(kind, color string) bool {
	switch kind {
	case Expense:
		return color == "red" || color == "grey"
	case Income:
		return color == "green" || color == "grey"
	case Transfer, Payment:
		return color == "grey"
	}
	return false
}

// Transaction is one movement of money in an account, or one purchase
// or refund on a credit card. It has an account or a card, never the
// two. A transfer between accounts is two transactions that point to
// each other with PairID: one that takes the money out of an account
// and one that puts it in the other account.
type Transaction struct {
	ID          uint   `gorm:"primaryKey"`
	Title       string `gorm:"size:60"`
	Description string `gorm:"size:200"`
	// Amount is in minor units of the currency of the account or the
	// card. It is negative when money leaves the account, or for a
	// purchase on a card.
	Amount     int64
	Type       string
	CategoryID uint `gorm:"index"`
	// AccountID is 0 for a transaction of a credit card.
	AccountID uint `gorm:"index"`
	// CardID is the credit card, or 0 for a transaction of an account.
	CardID uint `gorm:"index"`
	// BillID is the bill of a card transaction, or the bill that a
	// payment pays. It is 0 for the other transactions.
	BillID uint `gorm:"index"`
	// GoalID is the goal the transaction counts toward, or 0. The two
	// sides of a transfer have the same goal. A payment has no goal.
	GoalID uint `gorm:"index"`
	// Virtual tells that the person paid with a virtual card of the
	// credit card. Only a transaction of a credit card can have it.
	Virtual bool
	// OffLimit tells that the transaction does not use the limit of the
	// credit card. It stays in its bill. Only a transaction of a credit
	// card can have it, and all the parts of a transaction in
	// installments have the same value.
	OffLimit bool
	// Date is the day of the transaction as YYYY-MM-DD.
	Date string `gorm:"size:10;index"`
	// Tags holds the tags separated by one space, for example
	// "#market #weekly".
	Tags string
	// PairID is the other transaction of a transfer, or 0.
	PairID uint
	// InstallmentID joins the parts of a transaction in installments: it
	// is the ID of the first part that was created. Part is the number
	// of this part, from 1, and Parts is the number of parts. The three
	// are 0 for a transaction without installments.
	InstallmentID uint `gorm:"index"`
	Part          int
	Parts         int
	// ScheduleID is the schedule that created the transaction, or 0.
	ScheduleID uint `gorm:"index"`
	// Status tells if the date of the transaction came: StatusOn of its
	// date. SyncStatuses of the store writes it.
	Status    string `gorm:"size:7"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Value returns the amount without its sign.
func (t Transaction) Value() int64 {
	if t.Amount < 0 {
		return -t.Amount
	}
	return t.Amount
}

// Outgoing reports whether the money leaves the account.
func (t Transaction) Outgoing() bool { return t.Amount < 0 }

// Installment returns the part of a transaction in installments, such
// as "3/10". It is empty for a transaction without installments.
func (t Transaction) Installment() string {
	if t.InstallmentID == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", t.Part, t.Parts)
}

// TagList returns the tags as a list.
func (t Transaction) TagList() []string { return strings.Fields(t.Tags) }

// View is a transaction with the records it points to.
type View struct {
	Transaction
	// Account is where the transaction is. For a transaction of a
	// credit card it holds the name, code, color and currency of the
	// card, with the ID 0.
	Account  account.Account
	Category category.Category
	// Other is the account on the other side of a transfer, or the
	// credit card that a payment pays, shown as an account. It is nil
	// for an expense or an income.
	Other *account.Account
	// Card is the credit card of a card transaction or of a payment.
	Card *card.Card
	// Bill is the bill of a card transaction or of a payment.
	Bill *card.Bill
	// Goal is the goal of the transaction, or nil.
	Goal *goal.Goal
	// Schedule is the schedule that created the transaction, or nil.
	Schedule *schedule.Schedule
}

// Draft holds the values of a transaction to create or to change.
type Draft struct {
	Title, Description string
	Type               string
	// Value is the amount, always positive, in minor units of the
	// currency of the account.
	Value                 int64
	AccountID, CategoryID uint
	// Date is YYYY-MM-DD. Tags is text such as "#market #weekly".
	Date, Tags string
	// CardID is the credit card, in place of AccountID.
	CardID uint
	// GoalID is the goal the transaction counts toward, or 0.
	GoalID uint
	// Virtual tells that the person paid with a virtual card of the
	// credit card.
	Virtual bool
	// OffLimit tells that the transaction does not use the limit of the
	// credit card.
	OffLimit bool
	// ToAccountID is the account that receives a transfer.
	ToAccountID uint
	// Received is the amount the other account receives, for a transfer
	// between currencies. 0 means the same as Value.
	Received int64
	// Installments is the number of monthly parts of a new transaction.
	// 0 or 1 means one transaction. Value is then the total of all the
	// parts.
	Installments int
	// From is the first part to create, for a purchase that is already
	// in progress. 0 means 1.
	From int
	// ScheduleID is the schedule that creates the transaction, or 0.
	ScheduleID uint
}

// VirtualTotal returns the sum and the number of the transactions made
// with a virtual card. The sum is positive for money spent, as the
// total of a bill is.
func VirtualTotal(views []View) (total int64, count int) {
	for _, view := range views {
		if view.Virtual {
			total -= view.Amount
			count++
		}
	}
	return total, count
}

// OffLimitTotal returns the sum and the number of the transactions
// outside the limit of the credit card. The sum is positive for money
// spent, as the total of a bill is.
func OffLimitTotal(views []View) (total int64, count int) {
	for _, view := range views {
		if view.OffLimit {
			total -= view.Amount
			count++
		}
	}
	return total, count
}

// DraftOf returns the draft that describes a transaction as it is.
func DraftOf(t Transaction) Draft {
	return Draft{
		Title: t.Title, Description: t.Description, Type: t.Type, Value: t.Value(),
		AccountID: t.AccountID, CardID: t.CardID, CategoryID: t.CategoryID, Date: t.Date, Tags: t.Tags, GoalID: t.GoalID,
		Virtual: t.Virtual, OffLimit: t.OffLimit,
	}
}

// CheckTitle checks the rules of a title.
func CheckTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return errors.New("title is required")
	}
	if utf8.RuneCountInString(strings.TrimSpace(title)) > MaxTitle {
		return fmt.Errorf("title is too long: the limit is %d characters", MaxTitle)
	}
	return nil
}

// CheckDescription checks the rules of a description.
func CheckDescription(description string) error {
	if utf8.RuneCountInString(strings.TrimSpace(description)) > MaxDescription {
		return fmt.Errorf("description is too long: the limit is %d characters", MaxDescription)
	}
	return nil
}

// clean normalizes the text of a draft and checks the rules that do not
// need the database.
func (d *Draft) clean() error {
	d.Title = strings.TrimSpace(d.Title)
	d.Description = strings.TrimSpace(d.Description)
	if err := CheckTitle(d.Title); err != nil {
		return err
	}
	if err := CheckDescription(d.Description); err != nil {
		return err
	}
	if !ValidType(d.Type) {
		return fmt.Errorf("unknown type %q: use one of %s", d.Type, strings.Join(Types(), ", "))
	}
	if d.Value <= 0 {
		return errors.New("value must be more than zero")
	}
	if d.Received < 0 {
		return errors.New("received value must be more than zero")
	}
	date, err := ParseDate(d.Date)
	if err != nil {
		return err
	}
	tags, err := ParseTags(d.Tags)
	if err != nil {
		return err
	}
	d.Date, d.Tags = date, tags
	return d.cleanInstallments()
}

// cleanInstallments checks the installments of a draft. After it,
// Installments is 0 or a number from 2, and From is a number from 1.
func (d *Draft) cleanInstallments() error {
	switch {
	case d.Installments == 0 || d.Installments == 1:
		if d.From > 1 {
			return errors.New("first installment is only for a transaction in installments")
		}
		d.Installments, d.From = 0, 0
		return nil
	case d.Installments < 0 || d.Installments > MaxInstallments:
		return fmt.Errorf("installments must be from 2 to %d", MaxInstallments)
	case d.Type == Transfer:
		return errors.New("a transfer cannot have installments")
	case d.From < 0 || d.From > d.Installments:
		return fmt.Errorf("first installment must be from 1 to %d", d.Installments)
	case d.Value < int64(d.Installments):
		return fmt.Errorf("value is too small for %d installments", d.Installments)
	}
	d.From = max(d.From, 1)
	return nil
}

// split divides a total into n parts. The parts are equal; what remains
// goes to the first part.
func split(total int64, n int) []int64 {
	parts := make([]int64, n)
	for i := range parts {
		parts[i] = total / int64(n)
	}
	parts[0] += total % int64(n)
	return parts
}

// monthsLater returns the day of a YYYY-MM-DD date, months later. A
// month too short for the day gives its last day: 31 January and one
// month is 28 or 29 February.
func monthsLater(date string, months int) string {
	anchor, err := time.Parse(isoDate, date)
	if err != nil {
		return date
	}
	first := time.Date(anchor.Year(), anchor.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(anchor.Day(), last)-1).Format(isoDate)
}

// ParseDate reads a date as DD/MM/YYYY, YYYY/MM/DD or YYYY-MM-DD and
// returns it as YYYY-MM-DD.
func ParseDate(text string) (string, error) {
	text = strings.TrimSpace(text)
	for _, layout := range []string{"2/1/2006", "2006/1/2", "2006-1-2"} {
		if date, err := time.Parse(layout, text); err == nil {
			return date.Format(isoDate), nil
		}
	}
	return "", fmt.Errorf("invalid date %q: use DD/MM/YYYY or YYYY/MM/DD", text)
}

// FormatDate shows a YYYY-MM-DD date as DD/MM/YYYY.
func FormatDate(date string) string {
	parsed, err := time.Parse(isoDate, date)
	if err != nil {
		return date
	}
	return parsed.Format(displayDate)
}

// The statuses of a transaction.
const (
	// StatusPending is a transaction with a date after today: it is not
	// in the balance yet.
	StatusPending = "pending"
	StatusCleared = "cleared"
)

// StatusOn returns the status that a transaction with the YYYY-MM-DD
// date has on the date today.
func StatusOn(date, today string) string {
	if date > today {
		return StatusPending
	}
	return StatusCleared
}

// Today returns the current local date as YYYY-MM-DD. The FINUX_TODAY
// environment variable replaces it, for tests and demonstrations.
func Today() string {
	if date, err := ParseDate(os.Getenv("FINUX_TODAY")); err == nil {
		return date
	}
	return time.Now().Format(isoDate)
}

var yearPattern = regexp.MustCompile(`^\d{4}$`)

// ParsePeriod reads a year (2026), a month (11/2026 or 2026/11) or a
// full date. It returns the start of the YYYY-MM-DD dates in the period:
// "2026", "2026-11" or "2026-11-17".
func ParsePeriod(text string) (string, error) {
	text = strings.TrimSpace(text)
	if yearPattern.MatchString(text) {
		return text, nil
	}
	for _, layout := range []string{"1/2006", "2006/1", "2006-1"} {
		if month, err := time.Parse(layout, text); err == nil {
			return month.Format("2006-01"), nil
		}
	}
	if date, err := ParseDate(text); err == nil {
		return date, nil
	}
	return "", fmt.Errorf("invalid date %q: use a year (2026), a month (11/2026) or a date (17/11/2026)", text)
}

// ParseTags reads tags separated by spaces or commas. The # sign is
// optional. It returns them in lowercase, without duplicates, separated
// by one space: "#market #weekly".
func ParseTags(text string) (string, error) {
	var tags []string
	seen := map[string]bool{}
	for _, word := range strings.FieldsFunc(text, func(r rune) bool { return unicode.IsSpace(r) || r == ',' }) {
		name := strings.ToLower(strings.TrimPrefix(word, "#"))
		if name == "" {
			return "", errors.New("invalid tag \"#\": a tag needs a name")
		}
		if utf8.RuneCountInString(name) > MaxTag {
			return "", fmt.Errorf("invalid tag %q: the limit is %d characters", word, MaxTag)
		}
		for _, r := range name {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
				return "", fmt.Errorf("invalid tag %q: use letters, digits and _", word)
			}
		}
		if tag := "#" + name; !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return strings.Join(tags, " "), nil
}
