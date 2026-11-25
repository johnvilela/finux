package budget

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"finux/src/code"

	"gorm.io/gorm"
)

var (
	// ErrNotFound means no budget matches the id or code.
	ErrNotFound = errors.New("budget not found")
	// ErrCodeTaken means a budget, active or archived, has the code.
	ErrCodeTaken = errors.New("budget code is already in use")
)

// IncomeColor is the color of the income categories. A budget for all
// categories leaves them out.
const IncomeColor = "green"

// Store reads and writes budgets in the database.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store that uses db.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) prepare(budget *Budget) error {
	budget.Title = strings.TrimSpace(budget.Title)
	budget.Description = strings.TrimSpace(budget.Description)
	budget.Code = code.Normalize(budget.Code)
	budget.CategoryIDs = slices.Clone(budget.CategoryIDs)
	slices.Sort(budget.CategoryIDs)
	budget.CategoryIDs = slices.Compact(budget.CategoryIDs)
	if err := budget.Validate(); err != nil {
		return err
	}
	taken, err := s.CodeTaken(budget.Code, budget.ID)
	if err != nil {
		return err
	}
	if taken {
		return fmt.Errorf("%w: %s", ErrCodeTaken, budget.Code)
	}
	return nil
}

// CodeTaken reports whether a budget other than exceptID has the code.
// Archived budgets keep their codes.
func (s *Store) CodeTaken(value string, exceptID uint) (bool, error) {
	var count int64
	err := s.db.Unscoped().Model(&Budget{}).
		Where("code = ? AND id <> ?", code.Normalize(value), exceptID).Count(&count).Error
	return count > 0, err
}

// link writes the categories of a budget in place of the ones it had.
func link(tx *gorm.DB, budget *Budget) error {
	if err := tx.Where("budget_id = ?", budget.ID).Delete(&Link{}).Error; err != nil {
		return err
	}
	if len(budget.CategoryIDs) == 0 {
		return nil
	}
	links := make([]Link, len(budget.CategoryIDs))
	for i, id := range budget.CategoryIDs {
		links[i] = Link{BudgetID: budget.ID, CategoryID: id}
	}
	return tx.Create(&links).Error
}

// Create validates and inserts a budget and its categories.
func (s *Store) Create(budget *Budget) error {
	if err := s.prepare(budget); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(budget).Error; err != nil {
			return err
		}
		return link(tx, budget)
	})
}

// Update saves the fields and the categories of a budget. The currency
// does not change after creation.
func (s *Store) Update(budget *Budget) error {
	if err := s.prepare(budget); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Model(budget).
			Select("Title", "Code", "Description", "Limit", "StartDate", "Every", "Unit", "EndDate", "Rollover").
			Updates(budget).Error
		if err != nil {
			return err
		}
		return link(tx, budget)
	})
}

// List returns budgets ordered by title, with their periods up to
// today, as YYYY-MM-DD. Archived budgets are included only if
// includeArchived is true.
func (s *Store) List(includeArchived bool, today string) ([]Budget, error) {
	query := s.db
	if includeArchived {
		query = query.Unscoped()
	}
	var budgets []Budget
	if err := query.Order("title COLLATE NOCASE, id").Find(&budgets).Error; err != nil {
		return nil, err
	}
	return budgets, s.load(budgets, today)
}

// ListArchived returns only archived budgets ordered by title.
func (s *Store) ListArchived(today string) ([]Budget, error) {
	var budgets []Budget
	err := s.db.Unscoped().Where("deleted_at IS NOT NULL").Order("title COLLATE NOCASE, id").Find(&budgets).Error
	if err != nil {
		return nil, err
	}
	return budgets, s.load(budgets, today)
}

// Find returns the budget with the given id (all digits) or code, with
// its periods up to today.
func (s *Store) Find(reference string, includeArchived bool, today string) (Budget, error) {
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
	var budget Budget
	if err := query.Take(&budget).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return Budget{}, ErrNotFound
	} else if err != nil {
		return Budget{}, err
	}
	found := []Budget{budget}
	if err := s.load(found, today); err != nil {
		return Budget{}, err
	}
	return found[0], nil
}

// tables says which tables of the other packages the database has. A
// database from before a module does not have the tables of that module.
type tables struct {
	transactions, accounts, cards, categories bool
	// accountOwner and cardOwner tell that the accounts and the cards
	// have the column of the owner.
	accountOwner, cardOwner bool
}

// mine is the condition that leaves out the accounts or the cards of
// other persons, for a table that has the column of the owner.
func mine(hasOwner bool) string {
	if !hasOwner {
		return ""
	}
	return " AND owner = ''"
}

func (s *Store) tables() tables {
	migrator := s.db.Migrator()
	return tables{
		transactions: migrator.HasTable("transactions"),
		accounts:     migrator.HasTable("accounts"),
		cards:        migrator.HasTable("cards"),
		categories:   migrator.HasTable("categories"),
		accountOwner: migrator.HasTable("accounts") && migrator.HasColumn("accounts", "owner"),
		cardOwner:    migrator.HasTable("cards") && migrator.HasColumn("cards", "owner"),
	}
}

// load fills the categories of budgets, and their periods from the
// start to the date until.
func (s *Store) load(budgets []Budget, until string) error {
	if len(budgets) == 0 {
		return nil
	}
	ids := make([]uint, len(budgets))
	for i, budget := range budgets {
		ids[i] = budget.ID
	}
	var links []Link
	if err := s.db.Where("budget_id IN ?", ids).Order("category_id").Find(&links).Error; err != nil {
		return err
	}
	has := s.tables()
	for i := range budgets {
		budgets[i].CategoryIDs = nil
		for _, link := range links {
			if link.BudgetID == budgets[i].ID {
				budgets[i].CategoryIDs = append(budgets[i].CategoryIDs, link.CategoryID)
			}
		}
		periods := budgets[i].Periods(until)
		if len(periods) == 0 {
			budgets[i].Usages = nil
			continue
		}
		spent, err := s.spent(has, budgets[i], periods[0].From, periods[len(periods)-1].To)
		if err != nil {
			return err
		}
		budgets[i].Usages = budgets[i].Fold(periods, spent)
	}
	return nil
}

// spent returns what counts on a budget for each day from one date to
// another: expenses less incomes. A transaction in an account or a card
// of another person does not count. The transactions, accounts, cards and
// categories tables belong to other packages.
func (s *Store) spent(has tables, budget Budget, from, to string) (map[string]int64, error) {
	if !has.transactions {
		return nil, nil
	}
	// A transfer and the payment of a bill move money; they do not spend it.
	query := s.db.Table("transactions").Select("date, SUM(-amount) AS total").
		Where("type IN ('expense', 'income') AND date BETWEEN ? AND ?", from, to)
	switch {
	case len(budget.CategoryIDs) > 0:
		query = query.Where("category_id IN ?", budget.CategoryIDs)
	case has.categories:
		query = query.Where("category_id NOT IN (SELECT id FROM categories WHERE color = ?)", IncomeColor)
	}
	// The currency of a transaction is the one of its account or card.
	var places []string
	var currencies []any
	if has.accounts {
		places = append(places, "account_id IN (SELECT id FROM accounts WHERE currency = ?"+mine(has.accountOwner)+")")
		currencies = append(currencies, budget.Currency)
	}
	if has.cards {
		places = append(places, "card_id IN (SELECT id FROM cards WHERE currency = ?"+mine(has.cardOwner)+")")
		currencies = append(currencies, budget.Currency)
	}
	if len(places) == 0 {
		return nil, nil
	}
	query = query.Where("("+strings.Join(places, " OR ")+")", currencies...)
	var days []struct {
		Date  string
		Total int64
	}
	if err := query.Group("date").Scan(&days).Error; err != nil {
		return nil, err
	}
	spent := make(map[string]int64, len(days))
	for _, day := range days {
		spent[day.Date] = day.Total
	}
	return spent, nil
}

// Matching returns the active budgets that a transaction counts on, with
// their periods up to the date of the transaction. The transaction is an
// expense or an income of the category, in the currency, on the date.
func (s *Store) Matching(categoryID uint, currency, date string) ([]Budget, error) {
	var budgets []Budget
	err := s.db.Where("currency = ?", currency).Order("title COLLATE NOCASE, id").Find(&budgets).Error
	if err != nil {
		return nil, err
	}
	if err := s.load(budgets, date); err != nil {
		return nil, err
	}
	income := false
	if s.db.Migrator().HasTable("categories") {
		var count int64
		err := s.db.Table("categories").Where("id = ? AND color = ?", categoryID, IncomeColor).Count(&count).Error
		if err != nil {
			return nil, err
		}
		income = count > 0
	}
	var matching []Budget
	for _, budget := range budgets {
		if _, ok := budget.UsageAt(date); !ok {
			continue
		}
		if budget.Counts("expense", categoryID, income, currency) {
			matching = append(matching, budget)
		}
	}
	return matching, nil
}

// OnlyCategoryOf returns the budgets, active or archived, that have the
// category as their only category.
func (s *Store) OnlyCategoryOf(categoryID uint) ([]Budget, error) {
	var budgets []Budget
	err := s.db.Unscoped().
		Where(`id IN (SELECT budget_id FROM budget_categories
			GROUP BY budget_id HAVING COUNT(*) = 1 AND MAX(category_id) = ?)`, categoryID).
		Order("title COLLATE NOCASE, id").Find(&budgets).Error
	return budgets, err
}

// DropCategory takes a category out of every budget.
func (s *Store) DropCategory(categoryID uint) error {
	return s.db.Where("category_id = ?", categoryID).Delete(&Link{}).Error
}

// Archive marks a budget as archived.
func (s *Store) Archive(id uint) error {
	return affected(s.db.Delete(&Budget{}, id))
}

// Restore makes an archived budget active again.
func (s *Store) Restore(id uint) error {
	return affected(s.db.Unscoped().Model(&Budget{}).
		Where("id = ? AND deleted_at IS NOT NULL", id).Update("deleted_at", nil))
}

// Delete removes a budget and its categories from the database, active
// or archived. Transactions do not change.
func (s *Store) Delete(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := affected(tx.Unscoped().Delete(&Budget{}, id)); err != nil {
			return err
		}
		return tx.Where("budget_id = ?", id).Delete(&Link{}).Error
	})
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
