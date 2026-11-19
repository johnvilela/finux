// Package budget holds the budgets of the user: a limit of money for
// some categories in a period. Every expense and income of those
// categories in the period counts; a transaction has no link to a budget.
package budget

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"finux/src/code"
	"finux/src/money"

	"gorm.io/gorm"
)

// The units of the period of a recurring budget.
const (
	Day   = "day"
	Week  = "week"
	Month = "month"
	Year  = "year"
)

// The levels of a period, from what it spent and what it had.
const (
	LevelOK = "ok"
	// LevelNear is a period that spent NearPercent of its amount or more.
	LevelNear = "near"
	// LevelOver is a period that spent more than its amount.
	LevelOver = "over"
)

// The statuses of a budget that has no period on a date. A budget with
// a period on the date has the level of that period as its status.
const (
	StatusUpcoming = "upcoming"
	StatusEnded    = "ended"
)

const (
	// MaxTitle and MaxDescription are the limits of the texts, in
	// characters.
	MaxTitle       = 60
	MaxDescription = 200
	// MaxEvery is the largest number of units in one period.
	MaxEvery = 1000
	// NearPercent is the part of the amount of a period that makes it near
	// its limit.
	NearPercent = 80

	isoDate = "2006-01-02"
)

// Units returns the units of a period.
func Units() []string { return []string{Day, Week, Month, Year} }

// Budget is a limit of money for some categories in a period.
type Budget struct {
	ID          uint   `gorm:"primaryKey"`
	Title       string `gorm:"size:60"`
	Description string `gorm:"size:200"`
	Code        string `gorm:"uniqueIndex;size:5"`
	Currency    string
	// Limit is the amount of one period, in minor units.
	Limit int64
	// StartDate is the first day of the first period, as YYYY-MM-DD.
	StartDate string `gorm:"size:10"`
	// Every and Unit are the length of a period: every 2 weeks is 2 and
	// Week. A one-time budget has 0 and an empty unit.
	Every int
	Unit  string
	// EndDate is the last day of the budget, as YYYY-MM-DD. It is empty
	// for a recurring budget without an end.
	EndDate string `gorm:"size:10"`
	// Rollover moves what a period did not spend, or spent too much, to
	// the next period.
	Rollover  bool
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	// CategoryIDs are the categories that count. An empty list means all
	// categories. Usages are the periods from the start to a date, in
	// order. The store fills both.
	CategoryIDs []uint  `gorm:"-"`
	Usages      []Usage `gorm:"-"`
}

// Link is one category of a budget.
type Link struct {
	BudgetID   uint `gorm:"primaryKey"`
	CategoryID uint `gorm:"primaryKey"`
}

// TableName is the name of the table of the links.
func (Link) TableName() string { return "budget_categories" }

// Period is one period of a budget. Index counts from zero; From and To
// are its first and last days as YYYY-MM-DD.
type Period struct {
	Index    int
	From, To string
}

// Usage is what one period had and what it spent, in minor units.
type Usage struct {
	Period
	// Limit is the limit of the budget. Carried is what the periods before
	// left, below zero when they spent too much. Spent is never below zero.
	Limit, Carried, Spent int64
}

// Available returns the amount of the period: the limit and what the
// periods before left.
func (u Usage) Available() int64 { return u.Limit + u.Carried }

// Remaining returns what the period can still spend.
func (u Usage) Remaining() int64 { return max(u.Available()-u.Spent, 0) }

// Over returns what the period spent above its amount.
func (u Usage) Over() int64 { return max(u.Spent-u.Available(), 0) }

// Percent returns the spent part of the amount as a whole percentage.
// It is more than 100 for a period over its amount, and 100 for a period
// that started with nothing.
func (u Usage) Percent() int {
	if u.Available() <= 0 {
		return 100
	}
	// Divide first: the product can be too large for the largest amounts.
	return int(float64(u.Spent) / float64(u.Available()) * 100)
}

// Level returns the level of the period.
func (u Usage) Level() string {
	switch {
	case u.Spent > u.Available():
		return LevelOver
	case u.Percent() >= NearPercent:
		return LevelNear
	}
	return LevelOK
}

// Counts reports whether a transaction counts on the budget. kind is the
// type of the transaction and currency the one of its account or card.
// incomeCategory tells that its category is an income category: a budget
// for all categories leaves those out, so a salary does not pay for the
// expenses.
func (b Budget) Counts(kind string, categoryID uint, incomeCategory bool, currency string) bool {
	if (kind != "expense" && kind != "income") || currency != b.Currency {
		return false
	}
	if len(b.CategoryIDs) == 0 {
		return !incomeCategory
	}
	return slices.Contains(b.CategoryIDs, categoryID)
}

// Recurring reports whether the budget starts a new period when one ends.
func (b Budget) Recurring() bool { return b.Every > 0 }

// Archived reports whether the budget is archived.
func (b Budget) Archived() bool { return b.DeletedAt.Valid }

// Recurrence returns the period in words, for example "every 2 weeks".
func (b Budget) Recurrence() string { return Describe(b.Every, b.Unit) }

// Describe returns a period in words: "every month", "every 2 weeks",
// or "one time" for no period.
func Describe(every int, unit string) string {
	switch {
	case every <= 0:
		return "one time"
	case every == 1:
		return "every " + unit
	}
	return fmt.Sprintf("every %d %ss", every, unit)
}

// Step returns the date that is index periods after the YYYY-MM-DD date
// start. It always counts from start, so the day of the month does not
// move after a short month.
func Step(start string, every int, unit string, index int) string {
	return Budget{StartDate: start, Every: every, Unit: unit}.start(index).Format(isoDate)
}

var (
	everyPattern = regexp.MustCompile(`^(\d+)?[\s-]*(day|week|month|year)s?$`)
	everyWords   = map[string]string{"daily": Day, "weekly": Week, "monthly": Month, "yearly": Year}
)

// ParseEvery reads the period of a recurring budget: "month", "2 weeks",
// "15 days", "weekly". An empty text or "none" gives a one-time budget:
// zero and an empty unit.
func ParseEvery(text string) (every int, unit string, err error) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" || text == "none" {
		return 0, "", nil
	}
	if unit, ok := everyWords[text]; ok {
		return 1, unit, nil
	}
	found := everyPattern.FindStringSubmatch(text)
	if found == nil {
		return 0, "", fmt.Errorf(`invalid period %q: use for example "month", "2 weeks", "15 days" or "none"`, text)
	}
	every = 1
	if found[1] != "" {
		every, err = strconv.Atoi(found[1])
		if err != nil || every < 1 || every > MaxEvery {
			return 0, "", fmt.Errorf("invalid period %q: the number must be from 1 to %d", text, MaxEvery)
		}
	}
	return every, found[2], nil
}

// Validate checks the fields of a budget.
func (b Budget) Validate() error {
	title := strings.TrimSpace(b.Title)
	if title == "" {
		return errors.New("title is required")
	}
	if utf8.RuneCountInString(title) > MaxTitle {
		return fmt.Errorf("title is too long: the limit is %d characters", MaxTitle)
	}
	if utf8.RuneCountInString(strings.TrimSpace(b.Description)) > MaxDescription {
		return fmt.Errorf("description is too long: the limit is %d characters", MaxDescription)
	}
	if err := code.Validate(b.Code); err != nil {
		return err
	}
	if !money.Valid(b.Currency) {
		return fmt.Errorf("unknown currency %q: use one of %s", b.Currency, strings.Join(money.Codes(), ", "))
	}
	if b.Limit <= 0 {
		return errors.New("limit must be more than zero")
	}
	if _, err := time.Parse(isoDate, b.StartDate); err != nil {
		return fmt.Errorf("invalid start date %q", b.StartDate)
	}
	if b.Every < 0 || b.Every > MaxEvery {
		return fmt.Errorf("period must be 1 or more, and %d at most", MaxEvery)
	}
	if b.Recurring() && !slices.Contains(Units(), b.Unit) {
		return fmt.Errorf("unknown period %q: use one of %s", b.Unit, strings.Join(Units(), ", "))
	}
	if b.EndDate != "" {
		if _, err := time.Parse(isoDate, b.EndDate); err != nil {
			return fmt.Errorf("invalid end date %q", b.EndDate)
		}
		if b.EndDate < b.StartDate {
			return errors.New("end date is before the start date")
		}
	}
	if !b.Recurring() && b.EndDate == "" {
		return errors.New("give a period or an end date")
	}
	return nil
}

// day reads a date that Validate accepted. An invalid date gives the
// zero time.
func day(date string) time.Time {
	parsed, _ := time.Parse(isoDate, date)
	return parsed
}

// addMonths returns the day of anchor, months later. A month too short
// for the day gives its last day: 31 January and one month is 28 or 29
// February.
func addMonths(anchor time.Time, months int) time.Time {
	first := time.Date(anchor.Year(), anchor.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(anchor.Day(), last)-1)
}

// start returns the first day of the period with the given index. It
// always counts from the start date, so the day of the month does not
// move after a short month.
func (b Budget) start(index int) time.Time {
	anchor := day(b.StartDate)
	switch b.Unit {
	case Day:
		return anchor.AddDate(0, 0, index*b.Every)
	case Week:
		return anchor.AddDate(0, 0, 7*index*b.Every)
	case Year:
		return addMonths(anchor, 12*index*b.Every)
	}
	return addMonths(anchor, index*b.Every)
}

// period returns the period with the given index.
func (b Budget) period(index int) Period {
	if !b.Recurring() {
		return Period{From: b.StartDate, To: b.EndDate}
	}
	to := b.start(index+1).AddDate(0, 0, -1).Format(isoDate)
	if b.EndDate != "" && b.EndDate < to {
		to = b.EndDate
	}
	return Period{Index: index, From: b.start(index).Format(isoDate), To: to}
}

// PeriodAt returns the period that has the date, as YYYY-MM-DD. It
// reports false for a date before the start or after the end.
func (b Budget) PeriodAt(date string) (Period, bool) {
	if date < b.StartDate || (b.EndDate != "" && date > b.EndDate) {
		return Period{}, false
	}
	if !b.Recurring() {
		return b.period(0), true
	}
	anchor, target := day(b.StartDate), day(date)
	if anchor.IsZero() || target.IsZero() {
		return Period{}, false
	}
	// Start from an index that is never too large, then go forward.
	days := int(target.Sub(anchor).Hours() / 24)
	index := 0
	switch b.Unit {
	case Day:
		index = days / b.Every
	case Week:
		index = days / (7 * b.Every)
	case Month:
		index = days / (31 * b.Every)
	case Year:
		index = days / (366 * b.Every)
	}
	for !b.start(index + 1).After(target) {
		index++
	}
	return b.period(index), true
}

// Periods returns the periods from the start of the budget to the one
// that has the date until, in order. A budget that ended gives all its
// periods.
func (b Budget) Periods(until string) []Period {
	if b.EndDate != "" && until > b.EndDate {
		until = b.EndDate
	}
	last, ok := b.PeriodAt(until)
	if !ok {
		return nil
	}
	periods := make([]Period, 0, last.Index+1)
	for index := range last.Index {
		periods = append(periods, b.period(index))
	}
	return append(periods, last)
}

// Fold returns what each period had and spent. spent holds the amount of
// each day, as YYYY-MM-DD: expenses less incomes. A period never spends
// less than zero. With rollover, a period gets what the period before
// did not spend, and loses what it spent too much.
func (b Budget) Fold(periods []Period, spent map[string]int64) []Usage {
	usages := make([]Usage, len(periods))
	for i, period := range periods {
		usages[i] = Usage{Period: period, Limit: b.Limit}
	}
	for date, amount := range spent {
		for i := range usages {
			if date >= usages[i].From && date <= usages[i].To {
				usages[i].Spent += amount
				break
			}
		}
	}
	for i := range usages {
		usages[i].Spent = max(usages[i].Spent, 0)
		if b.Rollover && i > 0 {
			usages[i].Carried = usages[i-1].Available() - usages[i-1].Spent
		}
	}
	return usages
}

// UsageAt returns the loaded period that has the date.
func (b Budget) UsageAt(date string) (Usage, bool) {
	for _, usage := range b.Usages {
		if date >= usage.From && date <= usage.To {
			return usage, true
		}
	}
	return Usage{}, false
}

// Current returns the period of today, as YYYY-MM-DD. It reports false
// for a budget that did not start or that ended.
func (b Budget) Current(today string) (Usage, bool) { return b.UsageAt(today) }

// Shown returns the period to show on the date of today: the current
// one, or the last one of a budget that ended. It reports false for a
// budget that did not start.
func (b Budget) Shown(today string) (Usage, bool) {
	if usage, ok := b.Current(today); ok {
		return usage, true
	}
	if len(b.Usages) > 0 && today > b.StartDate {
		return b.Usages[len(b.Usages)-1], true
	}
	return Usage{}, false
}

// Status returns the status of the budget on the date of today: the
// level of the current period, or upcoming or ended.
func (b Budget) Status(today string) string {
	if usage, ok := b.Current(today); ok {
		return usage.Level()
	}
	if today < b.StartDate {
		return StatusUpcoming
	}
	return StatusEnded
}

// DefaultStart returns the start date of a new budget without one: the
// first day of the week (Monday), month or year of today for a period
// of one week, month or year, and today for any other period.
func DefaultStart(every int, unit, today string) string {
	date := day(today)
	if every != 1 || date.IsZero() {
		return today
	}
	switch unit {
	case Week:
		date = date.AddDate(0, 0, -(int(date.Weekday())+6)%7)
	case Month:
		date = date.AddDate(0, 0, 1-date.Day())
	case Year:
		date = time.Date(date.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return date.Format(isoDate)
}
