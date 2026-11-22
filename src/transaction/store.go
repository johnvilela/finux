package transaction

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"finux/src/account"
	"finux/src/card"
	"finux/src/category"
	"finux/src/goal"
	"finux/src/money"
	"finux/src/schedule"

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

// card loads a credit card. An archived card is found only when
// archived is true.
func (s *Store) card(id uint, archived bool) (card.Card, error) {
	query := s.db
	if archived {
		query = query.Unscoped()
	}
	var found card.Card
	if err := query.Take(&found, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return found, fmt.Errorf("credit card not found: id %d", id)
	} else if err != nil {
		return found, err
	}
	return found, nil
}

// locked is the error for a change in a bill that is paid.
func locked(owner card.Card, bill card.Bill) error {
	return fmt.Errorf("bill %s of %s is paid: delete its payment first", card.FormatPeriod(bill.Period), owner.Code)
}

// unlocked checks that the bill with the id is not paid.
func (s *Store) unlocked(billID uint) error {
	bill, err := card.NewStore(s.db).Bill(billID)
	if errors.Is(err, card.ErrBillNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if !bill.IsPaid() {
		return nil
	}
	owner, err := s.card(bill.CardID, true)
	if err != nil {
		return err
	}
	return locked(owner, bill)
}

// billFor returns the bill of a card for a date. It creates the bill
// if necessary. A paid bill is an error.
func (s *Store) billFor(owner card.Card, date string) (card.Bill, error) {
	bill, err := card.NewStore(s.db).BillFor(owner, date)
	if err != nil {
		return bill, err
	}
	if bill.IsPaid() {
		return bill, locked(owner, bill)
	}
	return bill, nil
}

// billOf returns the bill of a card for a period. It creates the bill
// if necessary. A paid bill is an error.
func (s *Store) billOf(owner card.Card, period string) (card.Bill, error) {
	bill, err := card.NewStore(s.db).BillOf(owner, period)
	if err != nil {
		return bill, err
	}
	if bill.IsPaid() {
		return bill, locked(owner, bill)
	}
	return bill, nil
}

// linkable checks that a transaction in the given currency can have the
// goal. A goal takes transactions of its currency only. An archived
// goal takes no new transaction; kept is true for a transaction that
// already has the goal.
func (s *Store) linkable(goalID uint, currency string, kept bool) error {
	if goalID == 0 {
		return nil
	}
	var found goal.Goal
	if err := s.db.Unscoped().Take(&found, goalID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("goal not found: id %d", goalID)
	} else if err != nil {
		return err
	}
	if found.Archived() && !kept {
		return fmt.Errorf("goal %s is archived: restore it first", found.Code)
	}
	if found.Currency != currency {
		return fmt.Errorf("goal %s is in %s, not in %s", found.Code, found.Currency, currency)
	}
	return nil
}

// place checks that a draft has an account or a credit card.
func place(draft Draft) error {
	switch {
	case draft.AccountID != 0 && draft.CardID != 0:
		return errors.New("a transaction has an account or a credit card, not the two")
	case draft.AccountID == 0 && draft.CardID == 0:
		return errors.New("account is required")
	case draft.CardID != 0 && draft.Type == Transfer:
		return errors.New("a credit card cannot have a transfer")
	}
	return nil
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
// the two sides and returns the side that leaves the account. For a
// transaction in installments it inserts the parts and returns the
// first one it created.
func (s *Store) Create(draft Draft) (Transaction, error) {
	created, err := s.create(draft)
	return created, s.stamp(err, &created)
}

func (s *Store) create(draft Draft) (Transaction, error) {
	if err := draft.clean(); err != nil {
		return Transaction{}, err
	}
	if err := place(draft); err != nil {
		return Transaction{}, err
	}
	if draft.Installments > 1 {
		return s.createParts(draft)
	}
	created := Transaction{
		Title: draft.Title, Description: draft.Description, Type: draft.Type, Amount: signed(draft.Type, draft.Value),
		AccountID: draft.AccountID, CardID: draft.CardID, CategoryID: draft.CategoryID, Date: draft.Date, Tags: draft.Tags,
		GoalID: draft.GoalID, ScheduleID: draft.ScheduleID,
	}
	if draft.CardID != 0 {
		owner, err := s.card(draft.CardID, false)
		if err != nil {
			return Transaction{}, err
		}
		if _, err := s.category(draft); err != nil {
			return Transaction{}, err
		}
		if err := s.linkable(draft.GoalID, owner.Currency, false); err != nil {
			return Transaction{}, err
		}
		bill, err := s.billFor(owner, draft.Date)
		if err != nil {
			return Transaction{}, err
		}
		created.BillID = bill.ID
		return created, s.db.Create(&created).Error
	}
	from, err := s.account(draft.AccountID, false)
	if err != nil {
		return Transaction{}, err
	}
	if _, err := s.category(draft); err != nil {
		return Transaction{}, err
	}
	if err := s.linkable(draft.GoalID, from.Currency, false); err != nil {
		return Transaction{}, err
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
var shared = []string{"Title", "Description", "CategoryID", "Date", "Tags", "GoalID"}

// Update changes the transaction id to the values of the draft. A
// transfer keeps its type and its accounts; the change also goes to its
// other side, except a new value between two currencies. A transaction
// of a credit card keeps its bill when its date and its card stay.
func (s *Store) Update(id uint, draft Draft) (Transaction, error) {
	updated, err := s.update(id, draft, "")
	return updated, s.stamp(err, &updated)
}

// update is Update for a transaction that goes to the bill of the given
// period. An empty period gives the bill of the date.
func (s *Store) update(id uint, draft Draft, period string) (Transaction, error) {
	current, err := s.take(id)
	if err != nil {
		return Transaction{}, err
	}
	if current.Type == Payment {
		return Transaction{}, errors.New("a payment cannot change: delete it to open the bill again")
	}
	if err := draft.clean(); err != nil {
		return Transaction{}, err
	}
	if (current.Type == Transfer) != (draft.Type == Transfer) {
		return Transaction{}, errors.New("a transfer cannot become an expense or an income, or the opposite: delete the transaction and create a new one")
	}
	if current.Type == Transfer && (draft.AccountID != current.AccountID || draft.CardID != 0) {
		return Transaction{}, errors.New("a transfer cannot change its accounts: delete the transaction and create a new one")
	}
	if err := place(draft); err != nil {
		return Transaction{}, err
	}
	if _, err := s.category(draft); err != nil {
		return Transaction{}, err
	}
	if current.CardID != 0 {
		if err := s.unlocked(current.BillID); err != nil {
			return Transaction{}, err
		}
	}
	// The account or the card a transaction already has can be archived.
	var billID uint
	var currency string
	if draft.CardID != 0 {
		owner, err := s.card(draft.CardID, draft.CardID == current.CardID)
		if err != nil {
			return Transaction{}, err
		}
		// The days of a card can change after a bill exists, and a part of
		// a transaction in installments is not always in the bill of its
		// date. So only a new date or a new card gives another bill.
		billID, currency = current.BillID, owner.Currency
		if period != "" || draft.CardID != current.CardID || draft.Date != current.Date || billID == 0 {
			var bill card.Bill
			if period != "" {
				bill, err = s.billOf(owner, period)
			} else {
				bill, err = s.billFor(owner, draft.Date)
			}
			if err != nil {
				return Transaction{}, err
			}
			billID = bill.ID
		}
	} else {
		holder, err := s.account(draft.AccountID, draft.AccountID == current.AccountID)
		if err != nil {
			return Transaction{}, err
		}
		currency = holder.Currency
	}
	// The goal of a transfer has the currency of the account the money
	// leaves, on the two sides.
	if current.Type == Transfer && !current.Outgoing() {
		pair, err := s.take(current.PairID)
		if err != nil {
			return Transaction{}, fmt.Errorf("other side of the transfer: %w", err)
		}
		source, err := s.account(pair.AccountID, true)
		if err != nil {
			return Transaction{}, err
		}
		currency = source.Currency
	}
	if err := s.linkable(draft.GoalID, currency, draft.GoalID == current.GoalID); err != nil {
		return Transaction{}, err
	}

	amount := signed(draft.Type, draft.Value)
	if current.Type == Transfer && current.Outgoing() {
		amount = -draft.Value
	}
	current.Title, current.Description, current.Type, current.Amount = draft.Title, draft.Description, draft.Type, amount
	current.AccountID, current.CategoryID, current.Date, current.Tags = draft.AccountID, draft.CategoryID, draft.Date, draft.Tags
	current.CardID, current.BillID, current.GoalID = draft.CardID, billID, draft.GoalID
	columns := append([]string{"Type", "Amount", "AccountID", "CardID", "BillID"}, shared...)
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
	pair.GoalID = current.GoalID
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
// the two sides. For a payment it opens the bill again.
func (s *Store) Delete(id uint) error {
	return s.stamp(s.remove(id))
}

func (s *Store) remove(id uint) error {
	current, err := s.take(id)
	if err != nil {
		return err
	}
	if current.Type == Payment {
		return s.unpay(current.BillID)
	}
	if current.CardID != 0 {
		if err := s.unlocked(current.BillID); err != nil {
			return err
		}
	}
	ids := []uint{current.ID}
	if current.PairID != 0 {
		ids = append(ids, current.PairID)
	}
	return s.db.Delete(&Transaction{}, ids).Error
}

// asAccount shows a credit card as an account, for the screens that
// show where a transaction is.
func asAccount(owner card.Card) account.Account {
	return account.Account{Name: owner.Name, Code: owner.Code, Color: owner.Color, Currency: owner.Currency, DeletedAt: owner.DeletedAt}
}

// views adds the accounts, the cards, the bills and the categories to
// transactions.
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
	var cards []card.Card
	if err := s.db.Unscoped().Find(&cards).Error; err != nil {
		return nil, err
	}
	cardByID := map[uint]card.Card{}
	for _, current := range cards {
		cardByID[current.ID] = current
	}
	var bills []card.Bill
	if err := s.db.Find(&bills).Error; err != nil {
		return nil, err
	}
	billByID := map[uint]card.Bill{}
	for _, current := range bills {
		billByID[current.ID] = current
	}
	var goals []goal.Goal
	if err := s.db.Unscoped().Find(&goals).Error; err != nil {
		return nil, err
	}
	goalByID := map[uint]goal.Goal{}
	for _, current := range goals {
		goalByID[current.ID] = current
	}
	// The table of the schedules is newer than the others: read it only
	// for a transaction that a schedule created.
	scheduleByID := map[uint]schedule.Schedule{}
	if slices.ContainsFunc(transactions, func(current Transaction) bool { return current.ScheduleID != 0 }) {
		var schedules []schedule.Schedule
		if err := s.db.Unscoped().Find(&schedules).Error; err != nil {
			return nil, err
		}
		for _, current := range schedules {
			scheduleByID[current.ID] = current
		}
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
		if target, ok := goalByID[current.GoalID]; ok && current.GoalID != 0 {
			views[i].Goal = &target
		}
		if source, ok := scheduleByID[current.ScheduleID]; ok {
			views[i].Schedule = &source
		}
		bill, ok := billByID[current.BillID]
		if !ok || current.BillID == 0 {
			continue
		}
		owner := cardByID[bill.CardID]
		views[i].Bill, views[i].Card = &bill, &owner
		shown := asAccount(owner)
		if current.Type == Payment {
			views[i].Other = &shown
		} else {
			views[i].Account = shown
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
	for _, kind := range append(Types(), Payment) {
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

// AccountUsage returns the number of expenses, incomes and payments of
// an account and the number of its transfer sides.
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
		// A bill that the account paid stays paid, without its payment.
		payments := tx.Model(&Transaction{}).Select("id").Where("account_id = ? AND type = ?", deleted.ID, Payment)
		if err := tx.Model(&card.Bill{}).Where("payment_id IN (?)", payments).Update("payment_id", 0).Error; err != nil {
			return err
		}
		return tx.Where("account_id = ?", deleted.ID).Delete(&Transaction{}).Error
	})
}

// ByGoal returns the transactions linked to a goal, the newest first.
func (s *Store) ByGoal(goalID uint) ([]View, error) {
	var transactions []Transaction
	if err := s.db.Where("goal_id = ?", goalID).Order("date DESC, id DESC").Find(&transactions).Error; err != nil {
		return nil, err
	}
	return s.views(transactions)
}

// BySchedule returns the transactions that a schedule created, the
// newest first.
func (s *Store) BySchedule(scheduleID uint) ([]View, error) {
	var transactions []Transaction
	if err := s.db.Where("schedule_id = ?", scheduleID).Order("date DESC, id DESC").Find(&transactions).Error; err != nil {
		return nil, err
	}
	return s.views(transactions)
}

// ClearSchedule removes the link to a schedule from its transactions.
func (s *Store) ClearSchedule(scheduleID uint) error {
	return s.db.Model(&Transaction{}).Where("schedule_id = ?", scheduleID).Update("schedule_id", 0).Error
}

// ClearGoal removes the link to a goal from its transactions. The
// transactions stay.
func (s *Store) ClearGoal(goalID uint) error {
	return s.db.Model(&Transaction{}).Where("goal_id = ?", goalID).Update("goal_id", 0).Error
}

// ByBill returns the transactions of a bill, the newest date first.
// The payment of the bill is not one of them.
func (s *Store) ByBill(billID uint) ([]View, error) {
	var transactions []Transaction
	err := s.db.Where("bill_id = ? AND card_id <> 0", billID).Order("date DESC, id DESC").Find(&transactions).Error
	if err != nil {
		return nil, err
	}
	return s.views(transactions)
}

// transferCategory returns the category of a payment.
func (s *Store) transferCategory() (category.Category, error) {
	var found category.Category
	err := s.db.Where("code = ?", category.TransferCode).Take(&found).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return found, fmt.Errorf("a payment uses the category %s, but it does not exist. Run 'finux ct add --name Transfer --code %s --color grey' to add it", category.TransferCode, category.TransferCode)
	case err != nil:
		return found, err
	case found.Color != "grey":
		return found, fmt.Errorf("a payment uses the category %s, but it is not grey. Run 'finux ct edit %s --color grey'", category.TransferCode, category.TransferCode)
	}
	return found, nil
}

// PayBill pays a bill from an account on a date. The value 0 pays the
// total. A smaller value is a partial payment: the bill is paid and
// the remaining value goes to the next bill that is not paid. It
// returns the payment transaction. A bill that owes nothing gets no
// transaction: its credit goes to the next bill.
func (s *Store) PayBill(billID, accountID uint, value int64, date string) (Transaction, error) {
	payment, err := s.payBill(billID, accountID, value, date)
	return payment, s.stamp(err, &payment)
}

func (s *Store) payBill(billID, accountID uint, value int64, date string) (Transaction, error) {
	cards := card.NewStore(s.db)
	bill, err := cards.Bill(billID)
	if err != nil {
		return Transaction{}, err
	}
	owner, err := s.card(bill.CardID, true)
	if err != nil {
		return Transaction{}, err
	}
	name := "bill " + card.FormatPeriod(bill.Period) + " of " + owner.Code
	if date, err = ParseDate(date); err != nil {
		return Transaction{}, err
	}
	total := bill.Total()
	switch {
	case bill.IsPaid():
		return Transaction{}, fmt.Errorf("%s is already paid", name)
	case date <= bill.ClosesOn:
		return Transaction{}, fmt.Errorf("%s is still open: it closes on %s", name, FormatDate(bill.ClosesOn))
	case value < 0:
		return Transaction{}, errors.New("value must be more than zero")
	case value > max(total, 0):
		return Transaction{}, fmt.Errorf("value is more than the total of the bill: %s", money.Format(max(total, 0), owner.Currency))
	}
	var payment Transaction
	if total > 0 {
		if value == 0 {
			value = total
		}
		from, err := s.account(accountID, false)
		if err != nil {
			return Transaction{}, err
		}
		if from.Currency != owner.Currency {
			return Transaction{}, fmt.Errorf("account %s is in %s and credit card %s is in %s: pay from an account in %s",
				from.Code, from.Currency, owner.Code, owner.Currency, owner.Currency)
		}
		transfer, err := s.transferCategory()
		if err != nil {
			return Transaction{}, err
		}
		payment = Transaction{
			Title: "Payment " + owner.Code + " " + card.FormatPeriod(bill.Period), Type: Payment, Amount: -value,
			AccountID: from.ID, CategoryID: transfer.ID, Date: date, BillID: bill.ID,
		}
	}
	bill.PaidOn, bill.Paid, bill.Remaining = date, value, total-value
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if payment.Type != "" {
			if err := tx.Create(&payment).Error; err != nil {
				return err
			}
			bill.PaymentID = payment.ID
		}
		if bill.Remaining != 0 {
			// The remaining value goes to the next bill that is not paid.
			bills, period := card.NewStore(tx), bill.Period
			for {
				period = card.NextPeriod(period)
				next, err := bills.BillOf(owner, period)
				if err != nil {
					return err
				}
				if next.IsPaid() {
					continue
				}
				bill.CarriedTo = next.ID
				if err := tx.Model(&next).Update("carried", next.Carried+bill.Remaining).Error; err != nil {
					return err
				}
				break
			}
		}
		return tx.Model(&bill).Select("PaidOn", "Paid", "Remaining", "CarriedTo", "PaymentID").Updates(&bill).Error
	})
	return payment, err
}

// Unpay deletes the payment of a bill: the bill is not paid again and
// the value it sent to the next bill comes back.
func (s *Store) Unpay(billID uint) error {
	return s.stamp(s.unpay(billID))
}

func (s *Store) unpay(billID uint) error {
	cards := card.NewStore(s.db)
	bill, err := cards.Bill(billID)
	if err != nil {
		return err
	}
	owner, err := s.card(bill.CardID, true)
	if err != nil {
		return err
	}
	if !bill.IsPaid() {
		return fmt.Errorf("bill %s of %s is not paid", card.FormatPeriod(bill.Period), owner.Code)
	}
	var next card.Bill
	if bill.CarriedTo != 0 {
		if next, err = cards.Bill(bill.CarriedTo); err != nil {
			return err
		}
		if next.IsPaid() {
			return fmt.Errorf("%w (it has the remaining value of bill %s)", locked(owner, next), card.FormatPeriod(bill.Period))
		}
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if bill.CarriedTo != 0 {
			if err := tx.Model(&next).Update("carried", next.Carried-bill.Remaining).Error; err != nil {
				return err
			}
		}
		if bill.PaymentID != 0 {
			if err := tx.Delete(&Transaction{}, bill.PaymentID).Error; err != nil {
				return err
			}
		}
		bill.PaidOn, bill.Paid, bill.Remaining, bill.CarriedTo, bill.PaymentID = "", 0, 0, 0, 0
		return tx.Model(&bill).Select("PaidOn", "Paid", "Remaining", "CarriedTo", "PaymentID").Updates(&bill).Error
	})
}

// CardUsage returns the number of transactions of a credit card and
// the number of payments of its bills.
func (s *Store) CardUsage(cardID uint) (purchases, payments int64, err error) {
	if err = s.db.Model(&Transaction{}).Where("card_id = ?", cardID).Count(&purchases).Error; err != nil {
		return 0, 0, err
	}
	bills := s.db.Model(&card.Bill{}).Select("id").Where("card_id = ?", cardID)
	err = s.db.Model(&Transaction{}).Where("type = ? AND bill_id IN (?)", Payment, bills).Count(&payments).Error
	return purchases, payments, err
}

// PurgeCard deletes the transactions and the bills of a credit card
// that will be deleted. Each payment of its bills stays in its account:
// it becomes an expense of the category other, with a note in its
// description.
func (s *Store) PurgeCard(deleted card.Card, other category.Category) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var payments []Transaction
		bills := tx.Model(&card.Bill{}).Select("id").Where("card_id = ?", deleted.ID)
		if err := tx.Where("type = ? AND bill_id IN (?)", Payment, bills).Find(&payments).Error; err != nil {
			return err
		}
		for _, payment := range payments {
			payment.Type, payment.BillID, payment.CategoryID = Expense, 0, other.ID
			payment.Description = withNote(payment.Description, "Payment of "+deleted.Code+" (deleted card)")
			if err := tx.Model(&payment).Select("Type", "BillID", "CategoryID", "Description").Updates(&payment).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("card_id = ?", deleted.ID).Delete(&Transaction{}).Error; err != nil {
			return err
		}
		return tx.Where("card_id = ?", deleted.ID).Delete(&card.Bill{}).Error
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

// SyncStatuses writes the status that each transaction and each bill
// has on the YYYY-MM-DD date today. It returns the bills that had
// another status before.
func (s *Store) SyncStatuses(today string) ([]card.Change, error) {
	for status, dates := range map[string]string{StatusCleared: "date <= ?", StatusPending: "date > ?"} {
		err := s.db.Model(&Transaction{}).Where(dates, today).Where("status IS NULL OR status <> ?", status).
			UpdateColumn("status", status).Error
		if err != nil {
			return nil, err
		}
	}
	return card.NewStore(s.db).SyncStatuses(today)
}

// stamp ends a change to the transactions: it gives each transaction
// that changed its status and makes the stored statuses current. It
// does nothing when the change failed with err.
func (s *Store) stamp(err error, changed ...*Transaction) error {
	if err != nil {
		return err
	}
	today := Today()
	for _, current := range changed {
		if current.ID != 0 {
			current.Status = StatusOn(current.Date, today)
		}
	}
	_, err = s.SyncStatuses(today)
	return err
}
