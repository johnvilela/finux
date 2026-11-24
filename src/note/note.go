// Package note holds the notes of the user: a short text about the
// finances, written by the user or by an AI agent. A note can have a
// reminder date, tags and links to a goal, a budget, an account or a
// credit card. It is open until someone marks it as done.
package note

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

const (
	// MaxTitle and MaxText are the limits of the texts, in characters.
	MaxTitle = 60
	MaxText  = 500

	isoDate = "2006-01-02"
)

// The sources of a note: who wrote it.
const (
	SourceUser  = "user"
	SourceAgent = "agent"
)

// The statuses of a note on a date. StatusAll is only for a filter.
const (
	StatusOpen = "open"
	// StatusDue is an open note with a reminder date up to the date.
	StatusDue  = "due"
	StatusDone = "done"
	StatusAll  = "all"
)

// The links of a note, for ClearLink.
const (
	LinkGoal    = "goal_id"
	LinkBudget  = "budget_id"
	LinkAccount = "account_id"
	LinkCard    = "card_id"
)

// ErrNotFound means no note has the id.
var ErrNotFound = errors.New("note not found")

// Note is a short text about the finances of the user.
type Note struct {
	ID    uint   `gorm:"primaryKey"`
	Title string `gorm:"size:60"`
	Text  string `gorm:"size:500"`
	// Tags holds the tags separated by one space: "#bank #fee".
	Tags string
	// RemindOn is the date of the reminder, as YYYY-MM-DD, or "".
	RemindOn string `gorm:"size:10;index"`
	// The records the note is about, or 0. A note has an account or a
	// credit card, never the two.
	GoalID    uint `gorm:"index"`
	BudgetID  uint `gorm:"index"`
	AccountID uint `gorm:"index"`
	CardID    uint `gorm:"index"`
	// Source tells who wrote the note: the user or an AI agent.
	Source string `gorm:"not null"`
	// DoneAt is the time the note was marked as done. An open note has none.
	DoneAt    *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Done reports whether the note is marked as done.
func (n Note) Done() bool { return n.DoneAt != nil }

// Status returns the status of the note on the date of today.
func (n Note) Status(today string) string {
	switch {
	case n.Done():
		return StatusDone
	case n.RemindOn != "" && n.RemindOn <= today:
		return StatusDue
	}
	return StatusOpen
}

// TagList returns the tags of the note.
func (n Note) TagList() []string { return strings.Fields(n.Tags) }

// Validate checks the fields of a note.
func (n Note) Validate() error {
	title := strings.TrimSpace(n.Title)
	switch {
	case title == "":
		return errors.New("title is required")
	case utf8.RuneCountInString(title) > MaxTitle:
		return fmt.Errorf("title is too long: the limit is %d characters", MaxTitle)
	case utf8.RuneCountInString(strings.TrimSpace(n.Text)) > MaxText:
		return fmt.Errorf("text is too long: the limit is %d characters", MaxText)
	case n.AccountID != 0 && n.CardID != 0:
		return errors.New("a note has an account or a credit card, not the two")
	case n.Source != SourceUser && n.Source != SourceAgent:
		return fmt.Errorf("unknown source %q: use %s or %s", n.Source, SourceUser, SourceAgent)
	}
	if n.RemindOn != "" {
		if _, err := time.Parse(isoDate, n.RemindOn); err != nil {
			return fmt.Errorf("invalid date %q", n.RemindOn)
		}
	}
	return nil
}

// Filter chooses the notes of a list. The zero value gives the open notes.
type Filter struct {
	// Status is StatusOpen (the default; it includes the due notes),
	// StatusDue, StatusDone or StatusAll.
	Status string
	// Tag is one tag, with or without #.
	Tag string
	// Search is text that the title or the text of the note contains.
	Search string
	// Until keeps the notes with a reminder date up to this date.
	Until string
	// Source keeps the notes of one source.
	Source                              string
	GoalID, BudgetID, AccountID, CardID uint
}

// Store reads and writes notes in the database.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store that uses db.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func prepare(note *Note) error {
	note.Title, note.Text = strings.TrimSpace(note.Title), strings.TrimSpace(note.Text)
	if note.Source == "" {
		note.Source = SourceUser
	}
	return note.Validate()
}

// Create validates and inserts a note.
func (s *Store) Create(note *Note) error {
	if err := prepare(note); err != nil {
		return err
	}
	return s.db.Create(note).Error
}

// Update saves the title, the text, the tags, the reminder date and the
// links of a note. Its source and its status stay.
func (s *Store) Update(note *Note) error {
	if err := prepare(note); err != nil {
		return err
	}
	return affected(s.db.Model(note).Select("Title", "Text", "Tags", "RemindOn", "GoalID", "BudgetID", "AccountID", "CardID").Updates(note))
}

// Find returns the note with the id.
func (s *Store) Find(id uint) (Note, error) {
	var note Note
	if err := s.db.Take(&note, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return Note{}, ErrNotFound
	} else if err != nil {
		return Note{}, err
	}
	return note, nil
}

// List returns the notes of the filter. The notes with a reminder come
// first, the oldest date first; then the notes without one, the newest
// first. today tells which notes are due.
func (s *Store) List(filter Filter, today string) ([]Note, error) {
	query := s.db.Model(&Note{})
	switch filter.Status {
	case StatusAll:
	case StatusDone:
		query = query.Where("done_at IS NOT NULL")
	case StatusDue:
		query = query.Where("done_at IS NULL AND remind_on <> '' AND remind_on <= ?", today)
	default:
		query = query.Where("done_at IS NULL")
	}
	if tag := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(filter.Tag), "#")); tag != "" {
		// The spaces around the tags make a tag match as a whole word.
		query = query.Where("' ' || tags || ' ' LIKE ?", "% #"+tag+" %")
	}
	if text := strings.TrimSpace(filter.Search); text != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(text) + "%"
		query = query.Where(`title LIKE ? ESCAPE '\' OR text LIKE ? ESCAPE '\'`, like, like)
	}
	if filter.Until != "" {
		query = query.Where("remind_on <> '' AND remind_on <= ?", filter.Until)
	}
	if filter.Source != "" {
		query = query.Where("source = ?", filter.Source)
	}
	for column, id := range map[string]uint{LinkGoal: filter.GoalID, LinkBudget: filter.BudgetID, LinkAccount: filter.AccountID, LinkCard: filter.CardID} {
		if id != 0 {
			query = query.Where(column+" = ?", id)
		}
	}
	var notes []Note
	err := query.Order("done_at IS NOT NULL, remind_on = '', remind_on, id DESC").Find(&notes).Error
	return notes, err
}

// SetDone marks a note as done, or opens it again.
func (s *Store) SetDone(id uint, done bool) error {
	var at *time.Time
	if done {
		now := time.Now()
		at = &now
	}
	return affected(s.db.Model(&Note{}).Where("id = ?", id).Update("done_at", at))
}

// Delete removes a note permanently.
func (s *Store) Delete(id uint) error {
	return affected(s.db.Delete(&Note{}, id))
}

// ClearLink takes a record that is deleted away from its notes. link is
// one of the Link constants. The notes stay.
func (s *Store) ClearLink(link string, id uint) error {
	switch link {
	case LinkGoal, LinkBudget, LinkAccount, LinkCard:
		return s.db.Model(&Note{}).Where(link+" = ?", id).Update(link, 0).Error
	}
	return fmt.Errorf("unknown link %q", link)
}

func affected(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
