package account

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"finux/src/code"

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
	account.Code = code.Normalize(account.Code)
	account.Owner = strings.TrimSpace(account.Owner)
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

// CodeTaken reports whether an account other than exceptID, or a credit
// card, has the code. Archived records keep their codes.
func (s *Store) CodeTaken(value string, exceptID uint) (bool, error) {
	value = code.Normalize(value)
	var count int64
	err := s.db.Unscoped().Model(&Account{}).Where("code = ? AND id <> ?", value, exceptID).Count(&count).Error
	if err != nil || count > 0 {
		return count > 0, err
	}
	// The cards table belongs to the card package; a database from
	// before that package does not have it.
	if !s.db.Migrator().HasTable("cards") {
		return false, nil
	}
	err = s.db.Table("cards").Where("code = ?", value).Count(&count).Error
	return count > 0, err
}

// Create validates and inserts an account.
func (s *Store) Create(account *Account) error {
	if err := s.prepare(account); err != nil {
		return err
	}
	return s.db.Create(account).Error
}

// Update saves the name, code, description, color and owner of an account.
// Currency and opening balance do not change after creation.
func (s *Store) Update(account *Account) error {
	if err := s.prepare(account); err != nil {
		return err
	}
	return s.db.Model(account).Select("Name", "Code", "Description", "Color", "Owner").Updates(account).Error
}

// List returns accounts ordered by name. Archived accounts are included
// only if includeArchived is true. The balances count the transactions
// up to the date today, as YYYY-MM-DD.
func (s *Store) List(includeArchived bool, today string) ([]Account, error) {
	query := s.db
	if includeArchived {
		query = query.Unscoped()
	}
	var accounts []Account
	if err := query.Order("name COLLATE NOCASE, id").Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, s.movements(accounts, today)
}

// movements fills the Movement of accounts: the sum of the amounts of
// their transactions up to the date today. A transaction with a later
// date changes the balance when its day comes. The transactions table belongs to the transaction
// package; a database from before that package does not have it.
func (s *Store) movements(accounts []Account, today string) error {
	if len(accounts) == 0 || !s.db.Migrator().HasTable("transactions") {
		return nil
	}
	var sums []struct {
		AccountID uint
		Total     int64
	}
	err := s.db.Table("transactions").Select("account_id, SUM(amount) AS total").
		Where("date <= ?", today).Group("account_id").Scan(&sums).Error
	if err != nil {
		return err
	}
	totals := map[uint]int64{}
	for _, sum := range sums {
		totals[sum.AccountID] = sum.Total
	}
	for i := range accounts {
		accounts[i].Movement = totals[accounts[i].ID]
	}
	return nil
}

// ListArchived returns only archived accounts ordered by name.
func (s *Store) ListArchived(today string) ([]Account, error) {
	var accounts []Account
	err := s.db.Unscoped().Where("deleted_at IS NOT NULL").
		Order("name COLLATE NOCASE, id").Find(&accounts).Error
	if err != nil {
		return nil, err
	}
	return accounts, s.movements(accounts, today)
}

// Find returns the account with the given id (all digits) or code. Its
// balance counts the transactions up to the date today.
func (s *Store) Find(reference string, includeArchived bool, today string) (Account, error) {
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
	var account Account
	if err := query.Take(&account).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return Account{}, ErrNotFound
	} else if err != nil {
		return Account{}, err
	}
	found := []Account{account}
	if err := s.movements(found, today); err != nil {
		return Account{}, err
	}
	return found[0], nil
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
