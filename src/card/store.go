package card

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"finux/src/code"

	"gorm.io/gorm"
)

var (
	// ErrNotFound means no credit card matches the id or code.
	ErrNotFound = errors.New("credit card not found")
	// ErrBillNotFound means no bill matches.
	ErrBillNotFound = errors.New("bill not found")
	// ErrCodeTaken means a credit card or an account, active or
	// archived, has the code.
	ErrCodeTaken = errors.New("code is already in use")
)

// Store reads and writes credit cards and bills in the database.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store that uses db.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) prepare(card *Card) error {
	card.Name = strings.TrimSpace(card.Name)
	card.Description = strings.TrimSpace(card.Description)
	card.Code = code.Normalize(card.Code)
	if err := card.Validate(); err != nil {
		return err
	}
	taken, err := s.CodeTaken(card.Code, card.ID)
	if err != nil {
		return err
	}
	if taken {
		return fmt.Errorf("%w: %s", ErrCodeTaken, card.Code)
	}
	return nil
}

// CodeTaken reports whether a credit card other than exceptID, or an
// account, has the code. Archived records keep their codes.
func (s *Store) CodeTaken(value string, exceptID uint) (bool, error) {
	value = code.Normalize(value)
	var count int64
	err := s.db.Unscoped().Model(&Card{}).Where("code = ? AND id <> ?", value, exceptID).Count(&count).Error
	if err != nil || count > 0 {
		return count > 0, err
	}
	// The accounts table belongs to the account package.
	if !s.db.Migrator().HasTable("accounts") {
		return false, nil
	}
	err = s.db.Table("accounts").Where("code = ?", value).Count(&count).Error
	return count > 0, err
}

// Create validates and inserts a credit card.
func (s *Store) Create(card *Card) error {
	if err := s.prepare(card); err != nil {
		return err
	}
	return s.db.Create(card).Error
}

// Update saves the name, code, description, color, limit and days of a
// credit card. The currency does not change after creation. New days
// apply only to the bills that do not exist yet.
func (s *Store) Update(card *Card) error {
	if err := s.prepare(card); err != nil {
		return err
	}
	return s.db.Model(card).Select("Name", "Code", "Description", "Color", "Limit", "ClosingDay", "DueDay").Updates(card).Error
}

// List returns credit cards ordered by name. Archived cards are
// included only if includeArchived is true.
func (s *Store) List(includeArchived bool) ([]Card, error) {
	query := s.db
	if includeArchived {
		query = query.Unscoped()
	}
	var cards []Card
	if err := query.Order("name COLLATE NOCASE, id").Find(&cards).Error; err != nil {
		return nil, err
	}
	return cards, s.usage(cards)
}

// ListArchived returns only archived credit cards ordered by name.
func (s *Store) ListArchived() ([]Card, error) {
	var cards []Card
	err := s.db.Unscoped().Where("deleted_at IS NOT NULL").Order("name COLLATE NOCASE, id").Find(&cards).Error
	if err != nil {
		return nil, err
	}
	return cards, s.usage(cards)
}

// Find returns the credit card with the given id (all digits) or code.
func (s *Store) Find(reference string, includeArchived bool) (Card, error) {
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
	var card Card
	if err := query.Take(&card).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return Card{}, ErrNotFound
	} else if err != nil {
		return Card{}, err
	}
	found := []Card{card}
	if err := s.usage(found); err != nil {
		return Card{}, err
	}
	return found[0], nil
}

// usage fills the Used of cards: the total of their bills that are not
// paid.
func (s *Store) usage(cards []Card) error {
	if len(cards) == 0 {
		return nil
	}
	var bills []Bill
	if err := s.db.Where("paid_on = ''").Find(&bills).Error; err != nil {
		return err
	}
	if err := s.spent(bills); err != nil {
		return err
	}
	used := map[uint]int64{}
	for _, bill := range bills {
		used[bill.CardID] += bill.Total()
	}
	for i := range cards {
		cards[i].Used = used[cards[i].ID]
	}
	return nil
}

// spent fills the Spent of bills from the transactions of the cards.
// The transactions table belongs to the transaction package; a
// database from before that package does not have it.
func (s *Store) spent(bills []Bill) error {
	if len(bills) == 0 || !s.db.Migrator().HasTable("transactions") {
		return nil
	}
	var sums []struct {
		BillID uint
		Total  int64
	}
	err := s.db.Table("transactions").Select("bill_id, -SUM(amount) AS total").
		Where("card_id <> 0").Group("bill_id").Scan(&sums).Error
	if err != nil {
		return err
	}
	totals := map[uint]int64{}
	for _, sum := range sums {
		totals[sum.BillID] = sum.Total
	}
	for i := range bills {
		bills[i].Spent = totals[bills[i].ID]
	}
	return nil
}

// Archive soft-deletes a credit card. It stays in the database.
func (s *Store) Archive(id uint) error {
	return affected(s.db.Delete(&Card{}, id))
}

// Restore makes an archived credit card active again.
func (s *Store) Restore(id uint) error {
	return affected(s.db.Unscoped().Model(&Card{}).
		Where("id = ? AND deleted_at IS NOT NULL", id).Update("deleted_at", nil))
}

// Delete removes a credit card permanently. It does not remove its
// bills or its transactions.
func (s *Store) Delete(id uint) error {
	return affected(s.db.Unscoped().Delete(&Card{}, id))
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

func (s *Store) one(bill Bill, err error) (Bill, error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Bill{}, ErrBillNotFound
	} else if err != nil {
		return Bill{}, err
	}
	found := []Bill{bill}
	if err := s.spent(found); err != nil {
		return Bill{}, err
	}
	return found[0], nil
}

// Bill returns the bill with the given id.
func (s *Store) Bill(id uint) (Bill, error) {
	var bill Bill
	return s.one(bill, s.db.Take(&bill, id).Error)
}

// Lookup returns the bill of a card for a period. It returns
// ErrBillNotFound if the bill does not exist.
func (s *Store) Lookup(cardID uint, period string) (Bill, error) {
	var bill Bill
	err := s.db.Where("card_id = ? AND period = ?", cardID, period).Take(&bill).Error
	return s.one(bill, err)
}

// BillOf returns the bill of a card for a period. It creates the bill
// if it does not exist.
func (s *Store) BillOf(card Card, period string) (Bill, error) {
	bill, err := s.Lookup(card.ID, period)
	if !errors.Is(err, ErrBillNotFound) {
		return bill, err
	}
	bill = Bill{CardID: card.ID, Period: period}
	if bill.ClosesOn, bill.DueOn = card.CycleOf(period); bill.ClosesOn == "" {
		return Bill{}, fmt.Errorf("invalid bill period %q", period)
	}
	return bill, s.db.Create(&bill).Error
}

// BillFor returns the bill of a card for a transaction on the given
// YYYY-MM-DD date. It creates the bill if it does not exist.
func (s *Store) BillFor(card Card, date string) (Bill, error) {
	period, _, _ := card.Cycle(date)
	if period == "" {
		return Bill{}, fmt.Errorf("invalid date %q", date)
	}
	return s.BillOf(card, period)
}

// Current returns the bill that receives the transactions of the date
// today. If the bill does not exist yet, the result has the ID 0 and
// the dates the bill will have.
func (s *Store) Current(card Card, today string) (Bill, error) {
	period, closesOn, dueOn := card.Cycle(today)
	bill, err := s.Lookup(card.ID, period)
	if errors.Is(err, ErrBillNotFound) {
		return Bill{CardID: card.ID, Period: period, ClosesOn: closesOn, DueOn: dueOn}, nil
	}
	return bill, err
}

// Bills returns the bills of a card, the newest first.
func (s *Store) Bills(cardID uint) ([]Bill, error) {
	var bills []Bill
	if err := s.db.Where("card_id = ?", cardID).Order("period DESC").Find(&bills).Error; err != nil {
		return nil, err
	}
	return bills, s.spent(bills)
}

// Change is a bill that got another status.
type Change struct {
	Bill Bill
	// From is the status before and To is the status now.
	From, To string
}

// SyncStatuses writes the status that each bill has on the YYYY-MM-DD
// date today. It returns the bills that had another status before. A
// bill that had no status, from an older Finux, is not in the result.
func (s *Store) SyncStatuses(today string) ([]Change, error) {
	var bills []Bill
	if err := s.db.Order("id").Find(&bills).Error; err != nil {
		return nil, err
	}
	if err := s.spent(bills); err != nil {
		return nil, err
	}
	var changes []Change
	for _, bill := range bills {
		before, now := bill.Status, bill.StatusOn(today)
		if before == now {
			continue
		}
		if err := s.db.Model(&Bill{}).Where("id = ?", bill.ID).UpdateColumn("status", now).Error; err != nil {
			return nil, err
		}
		if bill.Status = now; before != "" {
			changes = append(changes, Change{Bill: bill, From: before, To: now})
		}
	}
	return changes, nil
}
