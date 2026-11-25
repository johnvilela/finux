package transaction

import (
	"regexp"
	"slices"
	"strings"
)

// Filter selects transactions. An empty field selects all.
type Filter struct {
	// Title is text that the title must contain.
	Title string
	// Category and Account are codes. Type is a transaction type.
	Category, Account, Type string
	// Goal is the code of a goal.
	Goal string
	// Virtual is "yes" for the transactions made with a virtual card
	// and "no" for the other ones. The start of the word is enough.
	Virtual string
	// Tags lists tags the transaction must have, with the # sign.
	Tags []string
	// Period is the start of a YYYY-MM-DD date: a year, a month or a day.
	Period string
	// Mine leaves out the transactions in an account or a credit card
	// of another person. A filter with an Account shows that account in
	// any case.
	Mine bool
	// Prefix accepts the start of a code, a type or a tag. The search
	// input uses it, so the list changes while the user types.
	Prefix bool
}

func (f Filter) same(value, want string) bool {
	value, want = strings.ToLower(value), strings.ToLower(want)
	if f.Prefix {
		return strings.HasPrefix(value, want)
	}
	return value == want
}

// virtualWord is the word of the filter for a mark of a virtual card.
func virtualWord(virtual bool) string {
	if virtual {
		return "yes"
	}
	return "no"
}

// Match reports whether the filter selects the transaction.
func (f Filter) Match(v View) bool {
	switch {
	case !strings.Contains(strings.ToLower(v.Title), strings.ToLower(f.Title)),
		f.Category != "" && !f.same(v.Category.Code, f.Category),
		f.Account != "" && !f.same(v.Account.Code, f.Account),
		f.Type != "" && !f.same(v.Type, f.Type),
		f.Goal != "" && (v.Goal == nil || !f.same(v.Goal.Code, f.Goal)),
		f.Virtual != "" && strings.HasPrefix(virtualWord(!v.Virtual), strings.ToLower(f.Virtual)),
		!strings.HasPrefix(v.Date, f.Period),
		f.Mine && f.Account == "" && !v.Account.Mine():
		return false
	}
	tags := v.TagList()
	for _, want := range f.Tags {
		if !slices.ContainsFunc(tags, func(tag string) bool { return f.same(tag, want) }) {
			return false
		}
	}
	return true
}

// String shows the filter in the syntax of the search input.
func (f Filter) String() string {
	var parts []string
	if f.Title != "" {
		parts = append(parts, f.Title)
	}
	if f.Category != "" {
		parts = append(parts, "cat:"+f.Category)
	}
	parts = append(parts, f.Tags...)
	if f.Account != "" {
		parts = append(parts, "@"+f.Account)
	}
	if f.Type != "" {
		parts = append(parts, "type:"+f.Type)
	}
	if f.Goal != "" {
		parts = append(parts, "goal:"+f.Goal)
	}
	if f.Virtual != "" {
		parts = append(parts, "virtual:"+f.Virtual)
	}
	if f.Period != "" {
		parts = append(parts, f.Period)
	}
	return strings.Join(parts, " ")
}

// partialDate is the start of a YYYY-MM-DD date, as the user types it.
var partialDate = regexp.MustCompile(`^\d{4}(-\d{0,2}(-\d{0,2})?)?$`)

// ParseSearch reads the text of the search input. A word is a filter
// when it is #tag, @ACCOUNT, cat:CODE, type:TYPE, virtual:yes or a date (2026,
// 2026-11, 11/2026, 17/11/2026). The other words are text of the title.
func ParseSearch(text string) Filter {
	filter := Filter{Prefix: true}
	var title []string
	for _, word := range strings.Fields(text) {
		lower := strings.ToLower(word)
		switch {
		case strings.HasPrefix(word, "#"):
			if len(word) > 1 {
				filter.Tags = append(filter.Tags, lower)
			}
		case strings.HasPrefix(word, "@"):
			filter.Account = word[1:]
		case strings.HasPrefix(lower, "cat:"):
			filter.Category = word[len("cat:"):]
		case strings.HasPrefix(lower, "goal:"):
			filter.Goal = word[len("goal:"):]
		case strings.HasPrefix(lower, "type:"):
			filter.Type = word[len("type:"):]
		case strings.HasPrefix(lower, "virtual:"):
			filter.Virtual = lower[len("virtual:"):]
		case partialDate.MatchString(word):
			filter.Period = word
		default:
			if period, err := ParsePeriod(word); err == nil {
				filter.Period = period
			} else {
				title = append(title, word)
			}
		}
	}
	filter.Title = strings.Join(title, " ")
	return filter
}
