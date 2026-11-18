package transaction

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"finux/src/account"
	"finux/src/category"

	"gorm.io/gorm"
)

// ErrNotFound means no transaction has the id.
var ErrNotFound = errors.New("transaction not found")

// Store reads and writes transactions in the database.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store that uses db.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// account loads an account. An archived account is found only when
// archived is true.
func (s *Store) account(id uint, archived bool) (account.Account, error) {
	query := s.db
	if archived {
		query = query.Unscoped()
	}
	var found account.Account
	if err := query.Take(&found, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return found, fmt.Errorf("account not found: id %d", id)
	} else if err != nil {
		return found, err
	}
	return found, nil
}

// category loads the category of a draft and checks that the type of
// the draft can use it.
func (s *Store) category(draft Draft) (category.Category, error) {
	var found category.Category
	if draft.CategoryID == 0 {
		return found, errors.New("category is required")
	}
	if err := s.db.Take(&found, draft.CategoryID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return found, fmt.Errorf("category not found: id %d", draft.CategoryID)
	} else if err != nil {
		return found, err
	}
	if !Accepts(draft.Type, found.Color) {
		return found, fmt.Errorf("category %s is for %s transactions: it cannot have a transaction of the type %s", found.Code, found.Meaning(), draft.Type)
	}
	return found, nil
}

// signed gives a value the sign of its type: negative for an expense.
func signed(kind string, value int64) int64 {
	if kind == Expense {
		return -value
	}
	return value
}

// Create validates and inserts a transaction. For a transfer it inserts
// the two sides and returns the side that leaves the account.
func (s *Store) Create(draft Draft) (Transaction, error) {
	if err := draft.clean(); err != nil {
		return Transaction{}, err
	}
	if draft.AccountID == 0 {
		return Transaction{}, errors.New("account is required")
	}
	from, err := s.account(draft.AccountID, false)
	if err != nil {
		return Transaction{}, err
	}
	if _, err := s.category(draft); err != nil {
		return Transaction{}, err
	}
	created := Transaction{
		Title: draft.Title, Description: draft.Description, Type: draft.Type, Amount: signed(draft.Type, draft.Value),
		AccountID: draft.AccountID, CategoryID: draft.CategoryID, Date: draft.Date, Tags: draft.Tags,
	}
	if draft.Type != Transfer {
		if draft.ToAccountID != 0 {
			return Transaction{}, errors.New("only a transfer has a destination account")
		}
		return created, s.db.Create(&created).Error
	}

	switch draft.ToAccountID {
	case 0:
		return Transaction{}, errors.New("destination account is required for a transfer")
	case draft.AccountID:
		return Transaction{}, errors.New("a transfer needs two different accounts")
	}
	to, err := s.account(draft.ToAccountID, false)
	if err != nil {
		return Transaction{}, err
	}
	received := draft.Received
	switch {
	case from.Currency == to.Currency && received != 0 && received != draft.Value:
		return Transaction{}, fmt.Errorf("accounts %s and %s have the same currency: the received value must be equal to the value", from.Code, to.Code)
	case from.Currency == to.Currency:
		received = draft.Value
	case received == 0:
		return Transaction{}, fmt.Errorf("received value is required: %s is in %s and %s is in %s", from.Code, from.Currency, to.Code, to.Currency)
	}
	created.Amount = -draft.Value
	incoming := created
	incoming.AccountID, incoming.Amount = to.ID, received
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		incoming.PairID = created.ID
		if err := tx.Create(&incoming).Error; err != nil {
			return err
		}
		created.PairID = incoming.ID
		return tx.Model(&created).Update("PairID", incoming.ID).Error
	})
	return created, err
}

func (s *Store) take(id uint) (Transaction, error) {
	var found Transaction
	if err := s.db.Take(&found, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return found, ErrNotFound
	} else if err != nil {
		return found, err
	}
	return found, nil
}

// shared lists the columns that the two sides of a transfer have in common.
var shared = []string{"Title", "Description", "CategoryID", "Date", "Tags"}

// Update changes the transaction id to the values of the draft. A
// transfer keeps its type and its accounts; the change also goes to its
// other side, except a new value between two currencies.
func (s *Store) Update(id uint, draft Draft) (Transaction, error) {
	current, err := s.take(id)
	if err != nil {
		return Transaction{}, err
	}
	if err := draft.clean(); err != nil {
		return Transaction{}, err
	}
	if (current.Type == Transfer) != (draft.Type == Transfer) {
		return Transaction{}, errors.New("a transfer cannot become an expense or an income, or the opposite: delete the transaction and create a new one")
	}
	if current.Type == Transfer && draft.AccountID != current.AccountID {
		return Transaction{}, errors.New("a transfer cannot change its accounts: delete the transaction and create a new one")
	}
	if _, err := s.category(draft); err != nil {
		return Transaction{}, err
	}
	// The account a transaction already has can be archived.
	if _, err := s.account(draft.AccountID, draft.AccountID == current.AccountID); err != nil {
		return Transaction{}, err
	}

	amount := signed(draft.Type, draft.Value)
	if current.Type == Transfer && current.Outgoing() {
		amount = -draft.Value
	}
	current.Title, current.Description, current.Type, current.Amount = draft.Title, draft.Description, draft.Type, amount
	current.AccountID, current.CategoryID, current.Date, current.Tags = draft.AccountID, draft.CategoryID, draft.Date, draft.Tags
	columns := append([]string{"Type", "Amount", "AccountID"}, shared...)
	if current.PairID == 0 {
		return current, s.db.Model(&current).Select(columns).Updates(&current).Error
	}

	pair, err := s.take(current.PairID)
	if err != nil {
		return Transaction{}, fmt.Errorf("other side of the transfer: %w", err)
	}
	here, err := s.account(current.AccountID, true)
	if err != nil {
		return Transaction{}, err
	}
	there, err := s.account(pair.AccountID, true)
	if err != nil {
		return Transaction{}, err
	}
	pairColumns := shared
	pair.Title, pair.Description, pair.CategoryID, pair.Date, pair.Tags = current.Title, current.Description, current.CategoryID, current.Date, current.Tags
	if here.Currency == there.Currency {
		pair.Amount = -current.Amount
		pairColumns = append([]string{"Amount"}, shared...)
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&current).Select(columns).Updates(&current).Error; err != nil {
			return err
		}
		return tx.Model(&pair).Select(pairColumns).Updates(&pair).Error
	})
	return current, err
}

// Delete removes a transaction permanently. For a transfer it removes
// the two sides.
func (s *Store) Delete(id uint) error {
	current, err := s.take(id)
	if err != nil {
		return err
	}
	ids := []uint{current.ID}
	if current.PairID != 0 {
		ids = append(ids, current.PairID)
	}
	return s.db.Delete(&Transaction{}, ids).Error
}

// views adds the accounts and the categories to transactions.
func (s *Store) views(transactions []Transaction) ([]View, error) {
	var accounts []account.Account
	if err := s.db.Unscoped().Find(&accounts).Error; err != nil {
		return nil, err
	}
	var categories []category.Category
	if err := s.db.Find(&categories).Error; err != nil {
		return nil, err
	}
	accountByID := map[uint]account.Account{}
	for _, current := range accounts {
		accountByID[current.ID] = current
	}
	categoryByID := map[uint]category.Category{}
	for _, current := range categories {
		categoryByID[current.ID] = current
	}
	// The other side of a transfer can be outside the given transactions.
	var pairIDs []uint
	for _, current := range transactions {
		if current.PairID != 0 {
			pairIDs = append(pairIDs, current.PairID)
		}
	}
	pairAccount := map[uint]uint{}
	if len(pairIDs) > 0 {
		var pairs []Transaction
		if err := s.db.Select("id", "account_id").Find(&pairs, pairIDs).Error; err != nil {
			return nil, err
		}
		for _, pair := range pairs {
			pairAccount[pair.ID] = pair.AccountID
		}
	}
	views := make([]View, len(transactions))
	for i, current := range transactions {
		views[i] = View{Transaction: current, Account: accountByID[current.AccountID], Category: categoryByID[current.CategoryID]}
		if other, ok := accountByID[pairAccount[current.PairID]]; ok && current.PairID != 0 {
			views[i].Other = &other
		}
	}
	return views, nil
}

// Find returns the transaction with the given id.
func (s *Store) Find(id uint) (View, error) {
	found, err := s.take(id)
	if err != nil {
		return View{}, err
	}
	views, err := s.views([]Transaction{found})
	if err != nil {
		return View{}, err
	}
	return views[0], nil
}

// List returns the transactions the filter selects, the newest date
// first.
func (s *Store) List(filter Filter) ([]View, error) {
	var transactions []Transaction
	if err := s.db.Order("date DESC, id DESC").Find(&transactions).Error; err != nil {
		return nil, err
	}
	views, err := s.views(transactions)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(views, func(view View) bool { return !filter.Match(view) }), nil
}

// Titles returns the titles in use, the last one used first. Titles
// that differ only in case count as one.
func (s *Store) Titles() ([]string, error) {
	var all []string
	if err := s.db.Model(&Transaction{}).Order("id DESC").Pluck("title", &all).Error; err != nil {
		return nil, err
	}
	var titles []string
	seen := map[string]bool{}
	for _, title := range all {
		if key := strings.ToLower(title); !seen[key] {
			seen[key] = true
			titles = append(titles, title)
		}
	}
	return titles, nil
}

// Tags returns the tags in use, in alphabetical order.
func (s *Store) Tags() ([]string, error) {
	var all []string
	if err := s.db.Model(&Transaction{}).Where("tags <> ''").Distinct().Pluck("tags", &all).Error; err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, tags := range all {
		for _, tag := range strings.Fields(tags) {
			seen[tag] = true
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags, nil
}

// CountByCategory returns the number of transactions of a category.
func (s *Store) CountByCategory(categoryID uint) (int64, error) {
	var count int64
	return count, s.db.Model(&Transaction{}).Where("category_id = ?", categoryID).Count(&count).Error
}

// Mismatched returns the number of transactions of a category that the
// color would not accept. It tells if the category can change its color.
func (s *Store) Mismatched(categoryID uint, color string) (int64, error) {
	var accepted []string
	for _, kind := range Types() {
		if Accepts(kind, color) {
			accepted = append(accepted, kind)
		}
	}
	var count int64
	return count, s.db.Model(&Transaction{}).Where("category_id = ? AND type NOT IN ?", categoryID, accepted).Count(&count).Error
}

// MoveCategory gives the transactions of the category from the category to.
func (s *Store) MoveCategory(from, to uint) error {
	return s.db.Model(&Transaction{}).Where("category_id = ?", from).Update("category_id", to).Error
}

// AccountUsage returns the number of expenses and incomes of an account
// and the number of its transfer sides.
func (s *Store) AccountUsage(accountID uint) (plain, transfers int64, err error) {
	query := s.db.Model(&Transaction{}).Where("account_id = ?", accountID)
	if err = query.Session(&gorm.Session{}).Where("type <> ?", Transfer).Count(&plain).Error; err != nil {
		return 0, 0, err
	}
	err = query.Session(&gorm.Session{}).Where("type = ?", Transfer).Count(&transfers).Error
	return plain, transfers, err
}

// PurgeAccount deletes the transactions of an account that will be
// deleted. The other side of each of its transfers stays in its account:
// it becomes an expense or an income of the category other, with a note
// in its description.
func (s *Store) PurgeAccount(deleted account.Account, other category.Category) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var sides []Transaction
		err := tx.Where("pair_id IN (?)", tx.Model(&Transaction{}).Select("id").Where("account_id = ?", deleted.ID)).Find(&sides).Error
		if err != nil {
			return err
		}
		for _, side := range sides {
			side.Type, side.PairID, side.CategoryID = Income, 0, other.ID
			note := "Transfer from " + deleted.Code + " (deleted account)"
			if side.Outgoing() {
				side.Type, note = Expense, "Transfer to "+deleted.Code+" (deleted account)"
			}
			side.Description = withNote(side.Description, note)
			if err := tx.Model(&side).Select("Type", "PairID", "CategoryID", "Description").Updates(&side).Error; err != nil {
				return err
			}
		}
		return tx.Where("account_id = ?", deleted.ID).Delete(&Transaction{}).Error
	})
}

// withNote adds a note to a description. It cuts the description, not
// the note, to stay in the size limit.
func withNote(description, note string) string {
	if description == "" {
		return note
	}
	note = ". " + note
	text := []rune(description)
	if room := MaxDescription - len([]rune(note)); len(text) > room {
		text = append(text[:max(room-1, 0)], '…')
	}
	return string(text) + note
}
