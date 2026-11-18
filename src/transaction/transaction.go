// Package transaction holds the transaction model, its rules and its
// storage.
package transaction

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"finux/src/account"
	"finux/src/category"
)

// The types of a transaction.
const (
	Expense  = "expense"
	Income   = "income"
	Transfer = "transfer"
)

const (
	// MaxTitle and MaxDescription are the size limits in characters.
	MaxTitle       = 60
	MaxDescription = 200
	// MaxTag is the size limit of a tag, without the # sign.
	MaxTag = 30

	isoDate     = "2006-01-02"
	displayDate = "02/01/2006"
)

// Types returns the transaction types in display order.
func Types() []string { return []string{Expense, Income, Transfer} }

// ValidType reports whether kind is a transaction type.
func ValidType(kind string) bool {
	return kind == Expense || kind == Income || kind == Transfer
}

// Accepts reports whether a transaction of the type kind can use a
// category of the given color. Red is for expenses, green for incomes
// and grey for all types. A transfer uses only grey.
func Accepts(kind, color string) bool {
	switch kind {
	case Expense:
		return color == "red" || color == "grey"
	case Income:
		return color == "green" || color == "grey"
	case Transfer:
		return color == "grey"
	}
	return false
}

// Transaction is one movement of money in an account. A transfer between
// accounts is two transactions that point to each other with PairID: one
// that takes the money out of an account and one that puts it in the
// other account.
type Transaction struct {
	ID          uint   `gorm:"primaryKey"`
	Title       string `gorm:"size:60"`
	Description string `gorm:"size:200"`
	// Amount is in minor units of the currency of the account. It is
	// negative when money leaves the account.
	Amount     int64
	Type       string
	CategoryID uint `gorm:"index"`
	AccountID  uint `gorm:"index"`
	// Date is the day of the transaction as YYYY-MM-DD.
	Date string `gorm:"size:10;index"`
	// Tags holds the tags separated by one space, for example
	// "#market #weekly".
	Tags string
	// PairID is the other transaction of a transfer, or 0.
	PairID    uint
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

// TagList returns the tags as a list.
func (t Transaction) TagList() []string { return strings.Fields(t.Tags) }

// View is a transaction with the records it points to.
type View struct {
	Transaction
	Account  account.Account
	Category category.Category
	// Other is the account on the other side of a transfer. It is nil
	// for an expense or an income.
	Other *account.Account
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
	// ToAccountID is the account that receives a transfer.
	ToAccountID uint
	// Received is the amount the other account receives, for a transfer
	// between currencies. 0 means the same as Value.
	Received int64
}

// DraftOf returns the draft that describes a transaction as it is.
func DraftOf(t Transaction) Draft {
	return Draft{
		Title: t.Title, Description: t.Description, Type: t.Type, Value: t.Value(),
		AccountID: t.AccountID, CategoryID: t.CategoryID, Date: t.Date, Tags: t.Tags,
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
	return nil
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

// Today returns the current local date as YYYY-MM-DD.
func Today() string { return time.Now().Format(isoDate) }

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
