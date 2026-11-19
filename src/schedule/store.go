package schedule

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"finux/src/code"

	"gorm.io/gorm"
)

var (
	// ErrNotFound means no schedule matches the id or code.
	ErrNotFound = errors.New("schedule not found")
	// ErrCodeTaken means a schedule, active or archived, has the code.
	ErrCodeTaken = errors.New("schedule code is already in use")
)

// Store reads and writes schedules in the database.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store that uses db.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) prepare(schedule *Schedule) error {
	schedule.Title = strings.TrimSpace(schedule.Title)
	schedule.Description = strings.TrimSpace(schedule.Description)
	schedule.Code = code.Normalize(schedule.Code)
	if err := schedule.Validate(); err != nil {
		return err
	}
	taken, err := s.CodeTaken(schedule.Code, schedule.ID)
	if err != nil {
		return err
	}
	if taken {
		return fmt.Errorf("%w: %s", ErrCodeTaken, schedule.Code)
	}
	return nil
}

// CodeTaken reports whether a schedule other than exceptID has the
// code. Archived schedules keep their codes.
func (s *Store) CodeTaken(value string, exceptID uint) (bool, error) {
	var count int64
	err := s.db.Unscoped().Model(&Schedule{}).
		Where("code = ? AND id <> ?", code.Normalize(value), exceptID).Count(&count).Error
	return count > 0, err
}

// Create validates and inserts a schedule.
func (s *Store) Create(schedule *Schedule) error {
	if err := s.prepare(schedule); err != nil {
		return err
	}
	return s.db.Create(schedule).Error
}

// Update validates and saves all the values of a schedule.
func (s *Store) Update(schedule *Schedule) error {
	if err := s.prepare(schedule); err != nil {
		return err
	}
	return s.db.Model(schedule).Select(
		"Title", "Code", "Description", "Type", "Value", "AccountID", "CardID", "CategoryID", "GoalID", "Tags",
		"StartDate", "Every", "Unit", "EndDate", "Times", "Auto", "Done",
	).Updates(schedule).Error
}

// List returns schedules ordered by title. Archived schedules are
// included only if includeArchived is true.
func (s *Store) List(includeArchived bool) ([]Schedule, error) {
	query := s.db
	if includeArchived {
		query = query.Unscoped()
	}
	var schedules []Schedule
	return schedules, query.Order("title COLLATE NOCASE, id").Find(&schedules).Error
}

// ListArchived returns only archived schedules ordered by title.
func (s *Store) ListArchived() ([]Schedule, error) {
	var schedules []Schedule
	return schedules, s.db.Unscoped().Where("deleted_at IS NOT NULL").Order("title COLLATE NOCASE, id").Find(&schedules).Error
}

// Find returns the schedule with the given id (all digits) or code.
func (s *Store) Find(reference string, includeArchived bool) (Schedule, error) {
	query := s.db
	if includeArchived {
		query = query.Unscoped()
	}
	reference = code.Normalize(reference)
	if id, err := strconv.ParseUint(reference, 10, 64); err == nil {
		query = query.Where("id = ?", id)
	} else {
		query = query.Where("code = ?", reference)
	}
	var schedule Schedule
	if err := query.Take(&schedule).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return Schedule{}, ErrNotFound
	} else if err != nil {
		return Schedule{}, err
	}
	return schedule, nil
}

// Advance marks the next occurrence of a schedule as paid or skipped.
func (s *Store) Advance(id uint) error {
	return affected(s.db.Model(&Schedule{}).Where("id = ?", id).Update("done", gorm.Expr("done + 1")))
}

// CodesByCategory returns the codes of the schedules of a category,
// active or archived, in order.
func (s *Store) CodesByCategory(categoryID uint) ([]string, error) {
	var codes []string
	return codes, s.db.Unscoped().Model(&Schedule{}).Where("category_id = ?", categoryID).Order("code").Pluck("code", &codes).Error
}

// ClearGoal takes a goal away from the schedules that have it.
func (s *Store) ClearGoal(goalID uint) error {
	return s.db.Unscoped().Model(&Schedule{}).Where("goal_id = ?", goalID).Update("goal_id", 0).Error
}

// DeleteByAccount removes the schedules of an account permanently,
// active or archived.
func (s *Store) DeleteByAccount(accountID uint) error {
	return s.db.Unscoped().Where("account_id = ?", accountID).Delete(&Schedule{}).Error
}

// DeleteByCard removes the schedules of a credit card permanently,
// active or archived.
func (s *Store) DeleteByCard(cardID uint) error {
	return s.db.Unscoped().Where("card_id = ?", cardID).Delete(&Schedule{}).Error
}

// Archive marks a schedule as archived. It stops: its occurrences are
// not due.
func (s *Store) Archive(id uint) error {
	return affected(s.db.Delete(&Schedule{}, id))
}

// Restore makes an archived schedule active again.
func (s *Store) Restore(id uint) error {
	return affected(s.db.Unscoped().Model(&Schedule{}).
		Where("id = ? AND deleted_at IS NOT NULL", id).Update("deleted_at", nil))
}

// Delete removes a schedule from the database, active or archived. Its
// transactions stay.
func (s *Store) Delete(id uint) error {
	return affected(s.db.Unscoped().Delete(&Schedule{}, id))
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
