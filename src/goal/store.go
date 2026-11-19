package goal

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"finux/src/code"

	"gorm.io/gorm"
)

var (
	// ErrNotFound means no goal matches the id or code.
	ErrNotFound = errors.New("goal not found")
	// ErrCodeTaken means a goal, active or archived, has the code.
	ErrCodeTaken = errors.New("goal code is already in use")
)

// Store reads and writes goals in the database.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store that uses db.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) prepare(goal *Goal) error {
	goal.Title = strings.TrimSpace(goal.Title)
	goal.Description = strings.TrimSpace(goal.Description)
	goal.Code = code.Normalize(goal.Code)
	if err := goal.Validate(); err != nil {
		return err
	}
	taken, err := s.CodeTaken(goal.Code, goal.ID)
	if err != nil {
		return err
	}
	if taken {
		return fmt.Errorf("%w: %s", ErrCodeTaken, goal.Code)
	}
	return nil
}

// CodeTaken reports whether a goal other than exceptID has the code.
// Archived goals keep their codes.
func (s *Store) CodeTaken(value string, exceptID uint) (bool, error) {
	var count int64
	err := s.db.Unscoped().Model(&Goal{}).
		Where("code = ? AND id <> ?", code.Normalize(value), exceptID).Count(&count).Error
	return count > 0, err
}

// Create validates and inserts a goal.
func (s *Store) Create(goal *Goal) error {
	if err := s.prepare(goal); err != nil {
		return err
	}
	return s.db.Create(goal).Error
}

// Update saves the title, code, description, type, target and final
// date of a goal. The currency does not change after creation.
func (s *Store) Update(goal *Goal) error {
	if err := s.prepare(goal); err != nil {
		return err
	}
	return s.db.Model(goal).Select("Title", "Code", "Description", "Type", "Target", "DueDate").Updates(goal).Error
}

// List returns goals ordered by title. Archived goals are included
// only if includeArchived is true.
func (s *Store) List(includeArchived bool) ([]Goal, error) {
	query := s.db
	if includeArchived {
		query = query.Unscoped()
	}
	var goals []Goal
	if err := query.Order("title COLLATE NOCASE, id").Find(&goals).Error; err != nil {
		return nil, err
	}
	return goals, s.progress(goals)
}

// ListArchived returns only archived goals ordered by title.
func (s *Store) ListArchived() ([]Goal, error) {
	var goals []Goal
	err := s.db.Unscoped().Where("deleted_at IS NOT NULL").Order("title COLLATE NOCASE, id").Find(&goals).Error
	if err != nil {
		return nil, err
	}
	return goals, s.progress(goals)
}

// Find returns the goal with the given id (all digits) or code.
func (s *Store) Find(reference string, includeArchived bool) (Goal, error) {
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
	var goal Goal
	if err := query.Take(&goal).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return Goal{}, ErrNotFound
	} else if err != nil {
		return Goal{}, err
	}
	found := []Goal{goal}
	if err := s.progress(found); err != nil {
		return Goal{}, err
	}
	return found[0], nil
}

// progress fills Saved and Links of goals from their linked
// transactions. The transactions table belongs to the transaction
// package; a database without it gives goals without progress.
func (s *Store) progress(goals []Goal) error {
	if len(goals) == 0 || !s.db.Migrator().HasTable("transactions") || !s.db.Migrator().HasColumn("transactions", "goal_id") {
		return nil
	}
	var sums []struct {
		GoalID                 uint
		Income, Expense, Moved int64
		Links                  int64
	}
	// A transfer is two rows. The row that leaves its account stands for
	// the transfer.
	err := s.db.Table("transactions").Select(`goal_id,
		SUM(CASE WHEN type = 'income' THEN amount ELSE 0 END) AS income,
		SUM(CASE WHEN type = 'expense' THEN -amount ELSE 0 END) AS expense,
		SUM(CASE WHEN type = 'transfer' AND amount < 0 THEN -amount ELSE 0 END) AS moved,
		SUM(CASE WHEN type = 'transfer' AND amount > 0 THEN 0 ELSE 1 END) AS links`).
		Where("goal_id <> 0").Group("goal_id").Scan(&sums).Error
	if err != nil {
		return err
	}
	for i := range goals {
		for _, sum := range sums {
			if sum.GoalID != goals[i].ID {
				continue
			}
			goals[i].Links = sum.Links
			goals[i].Saved = sum.Income - sum.Expense + sum.Moved
			if goals[i].Type == PayOff {
				goals[i].Saved = sum.Expense - sum.Income + sum.Moved
			}
		}
	}
	return nil
}

// Archive marks a goal as archived. Its transactions keep the link.
func (s *Store) Archive(id uint) error {
	return affected(s.db.Delete(&Goal{}, id))
}

// Restore makes an archived goal active again.
func (s *Store) Restore(id uint) error {
	return affected(s.db.Unscoped().Model(&Goal{}).
		Where("id = ? AND deleted_at IS NOT NULL", id).Update("deleted_at", nil))
}

// Delete removes a goal from the database, active or archived. The
// caller clears the links of its transactions.
func (s *Store) Delete(id uint) error {
	return affected(s.db.Unscoped().Delete(&Goal{}, id))
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
