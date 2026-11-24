package transaction

import (
	"regexp"
	"strings"
	"time"

	"finux/src/category"

	"gorm.io/gorm"
)

// The sources of a correction: who made its last change.
const (
	SourceUser  = "user"
	SourceAgent = "agent"
)

// Correction is what the changes to transactions teach: the title, the
// category, the tags and the description that the transactions with one
// title must have. An AI agent reads the corrections when it names new
// transactions. There is one correction for each pattern, so the table
// grows with the number of names, not with the number of changes.
type Correction struct {
	ID uint `gorm:"primaryKey"`
	// Pattern is the title the transaction had, as Pattern cleans it.
	Pattern string `gorm:"not null;uniqueIndex"`
	// Title is the title to use. It is empty when the title did not change.
	Title string
	// CategoryID is the category to use, or 0 when it did not change.
	CategoryID uint
	// Tags and Description are nil when they did not change. Empty text
	// means that the transaction must have none.
	Tags        *string
	Description *string
	// Source tells who made the last change: the user or an AI agent.
	Source string `gorm:"not null"`
	// Count is the number of changes that made the correction.
	Count     int `gorm:"not null"`
	UpdatedAt time.Time
}

// empty reports whether the correction teaches nothing.
func (c Correction) empty() bool {
	return c.Title == "" && c.CategoryID == 0 && c.Tags == nil && c.Description == nil
}

// CorrectionView is a correction with the code of its category.
type CorrectionView struct {
	Correction
	// Category is the code of the category, or "" when there is none.
	Category string
}

var (
	patternDate    = regexp.MustCompile(`\d{1,2}/\d{1,2}(/\d{2,4})?`)
	patternAccents = strings.NewReplacer(
		"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A",
		"É", "E", "È", "E", "Ê", "E", "Ë", "E",
		"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
		"Ó", "O", "Ò", "O", "Ô", "O", "Õ", "O", "Ö", "O",
		"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
		"Ç", "C", "Ñ", "N",
	)
)

// Pattern returns the part of a title that stays the same from one
// transaction to the next: in capitals, without accents, without dates
// and without the words that have a digit, such as a code or the end of
// a card number. "PAY LAMEI 21/11" and "PAY LAMEI 23/11" give "PAY
// LAMEI". A title that has nothing else keeps its words.
func Pattern(title string) string {
	upper := patternAccents.Replace(strings.ToUpper(title))
	var words []string
	for _, word := range strings.Fields(patternDate.ReplaceAllString(upper, " ")) {
		if !strings.ContainsAny(word, "0123456789") {
			words = append(words, word)
		}
	}
	if len(words) == 0 {
		words = strings.Fields(upper)
	}
	return strings.Join(words, " ")
}

// actor returns the source of the changes of the store.
func (s *Store) actor() string {
	if s.Actor == "" {
		return SourceUser
	}
	return s.Actor
}

// learn records what the change of a transaction teaches. The change
// goes to the correction of the title the transaction had, and to the
// corrections that gave it that title: when "PAY X" became "Bakery" and
// "Bakery" now becomes "Padaria", "PAY X" must give "Padaria".
func (s *Store) learn(before, after Transaction) error {
	titled, categorized := before.Title != after.Title, before.CategoryID != after.CategoryID
	tagged, described := before.Tags != after.Tags, before.Description != after.Description
	if Neutral(before.Type) || !titled && !categorized && !tagged && !described {
		return nil
	}
	pattern := Pattern(before.Title)
	var rules []Correction
	if err := s.db.Where("pattern = ? OR title = ?", pattern, before.Title).Find(&rules).Error; err != nil {
		return err
	}
	own := false
	// The title goes back to a title that a correction changed: the
	// user took the correction away, so its title teaches nothing.
	undone := false
	for _, rule := range rules {
		own = own || rule.Pattern == pattern
		undone = undone || titled && rule.Title == before.Title && rule.Pattern == Pattern(after.Title)
	}
	if !own {
		rules = append(rules, Correction{Pattern: pattern})
	}
	for _, rule := range rules {
		switch {
		case undone:
			rule.Title = ""
		case titled:
			rule.Title = after.Title
		}
		if categorized {
			rule.CategoryID = after.CategoryID
		}
		if tagged {
			tags := after.Tags
			rule.Tags = &tags
		}
		if described {
			description := after.Description
			rule.Description = &description
		}
		rule.Source = s.actor()
		rule.Count++
		var err error
		switch {
		case !rule.empty():
			err = s.db.Save(&rule).Error
		case rule.ID != 0:
			err = s.db.Delete(&rule).Error
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Corrections returns the corrections, the last one changed first, and
// their number. With titles it returns only the corrections of these
// titles; a title can be a raw line of a statement. A limit of 0 means
// no limit.
func (s *Store) Corrections(titles []string, limit, offset int) ([]CorrectionView, int64, error) {
	scope := func() *gorm.DB {
		query := s.db.Model(&Correction{})
		if len(titles) > 0 {
			patterns := make([]string, len(titles))
			for i, title := range titles {
				patterns[i] = Pattern(title)
			}
			query = query.Where("pattern IN ?", patterns)
		}
		return query
	}
	var total int64
	if err := scope().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	query := scope().Order("updated_at DESC, id DESC").Offset(offset)
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rules []Correction
	if err := query.Find(&rules).Error; err != nil {
		return nil, 0, err
	}
	var categories []category.Category
	if err := s.db.Find(&categories).Error; err != nil {
		return nil, 0, err
	}
	codes := map[uint]string{}
	for _, current := range categories {
		codes[current.ID] = current.Code
	}
	views := make([]CorrectionView, len(rules))
	for i, rule := range rules {
		// A category that was deleted teaches nothing.
		views[i] = CorrectionView{Correction: rule, Category: codes[rule.CategoryID]}
	}
	return views, total, nil
}
