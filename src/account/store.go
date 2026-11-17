package account

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

var (
	// ErrNotFound means no account matches the id or code.
	ErrNotFound = errors.New("account not found")
	// ErrCodeTaken means another account, active or archived, has the code.
	ErrCodeTaken = errors.New("account code is already in use")
)

// Store reads and writes accounts in the database.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store that uses db.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) prepare(account *Account) error {
	account.Name = strings.TrimSpace(account.Name)
	account.Description = strings.TrimSpace(account.Description)
	account.Code = NormalizeCode(account.Code)
	if err := account.Validate(); err != nil {
		return err
	}
	taken, err := s.CodeTaken(account.Code, account.ID)
	if err != nil {
		return err
	}
	if taken {
		return fmt.Errorf("%w: %s", ErrCodeTaken, account.Code)
	}
	return nil
}

// CodeTaken reports whether an account other than exceptID has the code.
// Archived accounts keep their codes.
func (s *Store) CodeTaken(code string, exceptID uint) (bool, error) {
	var count int64
	err := s.db.Unscoped().Model(&Account{}).
		Where("code = ? AND id <> ?", NormalizeCode(code), exceptID).Count(&count).Error
	return count > 0, err
}

// Create validates and inserts an account.
func (s *Store) Create(account *Account) error {
	if err := s.prepare(account); err != nil {
		return err
	}
	return s.db.Create(account).Error
}

// Update saves the name, code, description and color of an account.
// Currency and opening balance do not change after creation.
func (s *Store) Update(account *Account) error {
	if err := s.prepare(account); err != nil {
		return err
	}
	return s.db.Model(account).Select("Name", "Code", "Description", "Color").Updates(account).Error
}

// List returns accounts ordered by name. Archived accounts are included
// only if includeArchived is true.
func (s *Store) List(includeArchived bool) ([]Account, error) {
	query := s.db
	if includeArchived {
		query = query.Unscoped()
	}
	var accounts []Account
	return accounts, query.Order("name COLLATE NOCASE, id").Find(&accounts).Error
}

// ListArchived returns only archived accounts ordered by name.
func (s *Store) ListArchived() ([]Account, error) {
	var accounts []Account
	return accounts, s.db.Unscoped().Where("deleted_at IS NOT NULL").
		Order("name COLLATE NOCASE, id").Find(&accounts).Error
}

// Find returns the account with the given id (all digits) or code.
func (s *Store) Find(reference string, includeArchived bool) (Account, error) {
	query := s.db
	if includeArchived {
		query = query.Unscoped()
	}
	reference = NormalizeCode(reference)
	if id, err := strconv.ParseUint(reference, 10, 64); err == nil {
		query = query.Where("id = ?", id)
	} else {
		query = query.Where("code = ?", reference)
	}
	var account Account
	if err := query.Take(&account).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return Account{}, ErrNotFound
	} else if err != nil {
		return Account{}, err
	}
	return account, nil
}

// Archive soft-deletes an account. It stays in the database.
func (s *Store) Archive(id uint) error {
	return affected(s.db.Delete(&Account{}, id))
}

// Restore makes an archived account active again.
func (s *Store) Restore(id uint) error {
	return affected(s.db.Unscoped().Model(&Account{}).
		Where("id = ? AND deleted_at IS NOT NULL", id).Update("deleted_at", nil))
}

// Delete removes an account permanently.
func (s *Store) Delete(id uint) error {
	return affected(s.db.Unscoped().Delete(&Account{}, id))
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
