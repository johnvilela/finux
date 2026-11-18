package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"

	"time"

	"finux/src/account"
	"finux/src/card"
	"finux/src/category"
	"finux/src/code"
	"finux/src/money"
	"finux/src/storage"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

// devDatabase is the database of a development build. 'make dev' sets it
// with -ldflags to a file in the project root. It is empty in a
// production build, which then has no seed command.
var devDatabase string

// databasePath returns the database file of this build: FINUX_DB, then
// the development database in a development build, then ~/.finux/finux.db.
func databasePath() (string, error) {
	if devDatabase != "" && os.Getenv("FINUX_DB") == "" {
		return devDatabase, nil
	}
	return storage.Path()
}

// seedAccounts is the base data of a new development database. It has
// all currencies, a negative balance, an archived account and more
// accounts than one page of the selector.
var seedAccounts = []struct {
	account  account.Account
	archived bool
}{
	{account: account.Account{Name: "Banco Inter", Code: "INTER", Description: "Main checking account", Color: "orange", Currency: "BRL", OpeningBalance: 123456}},
	{account: account.Account{Name: "Nubank", Code: "NUBAN", Description: "Credit card", Color: "purple", Currency: "BRL", OpeningBalance: -35075}},
	{account: account.Account{Name: "Itaú", Code: "ITAU", Description: "Savings", Color: "amber", Currency: "BRL", OpeningBalance: 890000}},
	{account: account.Account{Name: "Cash", Code: "CASH", Color: "green", Currency: "BRL", OpeningBalance: 8500}},
	{account: account.Account{Name: "Wise", Code: "WISE", Description: "Travel money", Color: "lime", Currency: "USD", OpeningBalance: 50000}},
	{account: account.Account{Name: "Revolut", Code: "REVOL", Color: "sky", Currency: "EUR", OpeningBalance: 12040}},
	{account: account.Account{Name: "Cold Wallet", Code: "BTC", Description: "Hardware wallet", Color: "yellow", Currency: "BTC", OpeningBalance: 12345678}},
	{account: account.Account{Name: "Old Bank", Code: "OLD", Description: "Closed account", Color: "slate", Currency: "USD"}, archived: true},
}

// seedBatch is the number of accounts a seed adds to a database that
// exists. seedMovements is the number of transactions it adds: one
// transfer, which is two transactions, and expenses or incomes.
const (
	seedBatch     = 3
	seedMovements = 5
)

// seedEntry is one sample transaction. The account and the category are
// codes. to is the destination of a transfer and received its amount in
// the other currency.
type seedEntry struct {
	title, account, category, tags string
	value                          int64
	daysAgo                        int
	to                             string
	received                       int64
}

// seedTransactions is the base data of a new development database. It
// has all types, tags in use by many transactions, a transfer in one
// currency and a transfer between two currencies.
var seedTransactions = []seedEntry{
	{title: "Salary", account: "INTER", category: "SAL", tags: "#work", value: 650000, daysAgo: 20},
	{title: "Rent", account: "INTER", category: "HOME", tags: "#fixed", value: 180000, daysAgo: 18},
	{title: "Electricity", account: "INTER", category: "BILLS", tags: "#fixed", value: 21050, daysAgo: 15},
	{title: "Internet", account: "INTER", category: "BILLS", tags: "#fixed", value: 9990, daysAgo: 15},
	{title: "Travel money", account: "INTER", category: "XFER", tags: "#travel", value: 55000, daysAgo: 14, to: "WISE", received: 10000},
	{title: "Weekly market", account: "NUBAN", category: "FOOD", tags: "#market #weekly", value: 34270, daysAgo: 12},
	{title: "Headphones", account: "WISE", category: "SHOP", tags: "#tech", value: 7999, daysAgo: 11},
	{title: "Cinema", account: "NUBAN", category: "FUN", tags: "#weekend", value: 6400, daysAgo: 9},
	{title: "Website project", account: "INTER", category: "FREE", tags: "#work", value: 240000, daysAgo: 8},
	{title: "Fuel", account: "NUBAN", category: "TRANS", tags: "#car", value: 25000, daysAgo: 7},
	{title: "Weekly market", account: "NUBAN", category: "FOOD", tags: "#market #weekly", value: 29815, daysAgo: 5},
	{title: "Bakery", account: "CASH", category: "FOOD", tags: "#market", value: 1850, daysAgo: 3},
	{title: "Card payment", account: "INTER", category: "XFER", tags: "#fixed", value: 35075, daysAgo: 2, to: "NUBAN"},
	{title: "Concert tickets", account: "REVOL", category: "FUN", tags: "#weekend", value: 4500, daysAgo: 30},
	{title: "Mining reward", account: "BTC", category: "INV", value: 250000, daysAgo: 40},
}

// seedCards is the base data of a new development database. The days
// of VISA come from the date of today: see seedBaseCards.
var seedCards = []card.Card{
	{Name: "Visa Gold", Code: "VISA", Description: "Main credit card", Color: "blue", Currency: "BRL", Limit: 800000},
	{Name: "Amex", Code: "AMEX", Description: "Travel", Color: "teal", Currency: "USD", Limit: 200000, ClosingDay: 3, DueDay: 10},
}

// seedPurchases are the transactions of the credit cards. With the
// days of VISA, they are in three bills: one that is open, one that is
// late and one that seedBaseCards pays in part.
var seedPurchases = []seedEntry{
	{title: "Streaming", account: "VISA", category: "FUN", tags: "#fixed", value: 5590, daysAgo: 2},
	{title: "Restaurant", account: "VISA", category: "FOOD", tags: "#weekend", value: 17990, daysAgo: 6},
	{title: "Pharmacy", account: "VISA", category: "HEALT", value: 4200, daysAgo: 12},
	{title: "Running shoes", account: "VISA", category: "SHOP", value: 45990, daysAgo: 36},
	{title: "Flight tickets", account: "VISA", category: "TRANS", tags: "#travel", value: 50000, daysAgo: 66},
	{title: "Hotel", account: "AMEX", category: "FUN", tags: "#travel", value: 18000, daysAgo: 2},
}

// The payment of the oldest bill of VISA: a part of its total, from the
// account INTER.
const (
	seedPaymentValue   = 30000
	seedPaymentDaysAgo = 45
)

// seedMovementPool is the pool for the expenses and incomes a seed adds
// to a database that exists.
var seedMovementPool = []struct{ title, category, tags string }{
	{"Weekly market", "FOOD", "#market #weekly"}, {"Bakery", "FOOD", "#market"}, {"Restaurant", "FOOD", "#weekend"},
	{"Fuel", "TRANS", "#car"}, {"Bus pass", "TRANS", ""}, {"Pharmacy", "HEALT", ""}, {"Cinema", "FUN", "#weekend"},
	{"Books", "EDU", ""}, {"Clothes", "SHOP", ""}, {"Phone bill", "BILLS", "#fixed"}, {"Electricity", "BILLS", "#fixed"},
	{"Salary", "SAL", "#work"}, {"Website project", "FREE", "#work"}, {"Dividends", "INV", ""}, {"Gift", "OTHER", ""},
}

// seedNames and seedDescriptions are the pools for the accounts a seed
// adds to a database that exists.
var (
	seedNames = []string{
		"Bradesco", "Santander", "Caixa", "C6 Bank", "PicPay", "Mercado Pago", "XP Investimentos",
		"BTG Pactual", "Banco do Brasil", "Sicredi", "Neon", "PagBank", "Safra", "Rico",
		"Binance", "Coinbase", "Kraken", "Ledger", "PayPal", "Chase", "Citi", "HSBC",
		"Barclays", "ING", "N26", "Monzo", "Bank of America", "Charles Schwab", "Fidelity", "Stripe",
	}
	seedDescriptions = []string{"", "", "Savings", "Investments", "Daily expenses", "Emergency fund", "Business account"}
)

func seedHelp() string {
	return "Fill the development database with sample data\n\n" +
		green.Render("Usage:") + ` finux seed [--new-db]

Applies all migrations to the development database, then adds sample data:
  - a database that does not exist is created with a base set of accounts,
    credit cards and transactions;
  - a database that exists keeps its data and gets ` + strconv.Itoa(seedBatch) + ` more random accounts
    and ` + strconv.Itoa(seedMovements) + ` more random transactions.
'make dev' runs it after each build.

` + green.Render("Options:") + `
  --new-db  Delete the database first, then create it with the base data

It only works on the development database of this build:
  ` + devDatabase + `
It ignores FINUX_DB. A production build does not have this command.
`
}

func runSeed(args []string, io streams) int {
	if len(args) > 0 && isHelp(args[0]) {
		return write(io, seedHelp())
	}
	var fresh bool
	if _, _, err := parse("seed", args, func(f *flag.FlagSet) { f.BoolVar(&fresh, "new-db", false, "") }, 0); err != nil {
		fmt.Fprintf(io.stderr, "finux: %v\nRun 'finux seed --help' for usage.\n", err)
		return 2
	}
	report, err := seed(devDatabase, fresh)
	if err != nil {
		fmt.Fprintf(io.stderr, "finux: seed: %v\n", err)
		return 1
	}
	return write(io, ui.Success(report)+"\n")
}

// seed migrates the database at path and adds sample data. A new
// database gets the base accounts. A database that exists keeps its data
// and gets seedBatch random accounts. fresh deletes the database first.
// It returns a one-line report.
func seed(path string, fresh bool) (string, error) {
	if fresh {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("remove old database: %w", err)
		}
	}
	setup, err := setupDatabase(path)
	if err != nil {
		return "", err
	}
	db, err := storage.Open(path)
	if err != nil {
		return "", err
	}
	store, transactions := account.NewStore(db), transaction.NewStore(db)
	if setup.created {
		if err := seedBase(store); err != nil {
			return "", err
		}
		added, err := seedBaseTransactions(db)
		if err != nil {
			return "", err
		}
		purchases, err := seedBaseCards(db)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Development database created: %s (%d accounts, %d credit cards, %d transactions)",
			path, len(seedAccounts), len(seedCards), added+purchases), nil
	}
	if err := seedMore(store, seedBatch); err != nil {
		return "", err
	}
	added, err := seedMoreTransactions(db, seedMovements)
	if err != nil {
		return "", err
	}
	all, err := store.List(true)
	if err != nil {
		return "", err
	}
	movements, err := transactions.List(transaction.Filter{})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Development database updated: %s (%d accounts and %d transactions added; %d accounts and %d transactions in total)",
		path, seedBatch, added, len(all), len(movements)), nil
}

// daysAgo returns the date that many days before today.
func daysAgo(days int) string {
	return time.Now().AddDate(0, 0, -days).Format("2006-01-02")
}

// seedBaseCards adds the base credit cards, their transactions and one
// payment. It returns the number of transactions it added.
//
// The bill of VISA closes in 5 to 8 days from today, so the purchases
// of the last days are in the open bill, the purchase of 36 days ago is
// in a bill that is late, and the purchase of 66 days ago is in the
// bill that gets the payment.
func seedBaseCards(db *gorm.DB) (int, error) {
	cards, categories, store := card.NewStore(db), category.NewStore(db), transaction.NewStore(db)
	closing := min(time.Now().AddDate(0, 0, 8).Day(), 28)
	owners := map[string]card.Card{}
	for _, created := range seedCards {
		if created.ClosingDay == 0 {
			created.ClosingDay, created.DueDay = closing, (closing+6)%28+1
		}
		if err := cards.Create(&created); err != nil {
			return 0, fmt.Errorf("create credit card %s: %w", created.Code, err)
		}
		owners[created.Code] = created
	}
	added := 0
	var oldest transaction.Transaction
	for _, entry := range seedPurchases {
		group, err := categories.Find(entry.category)
		if err != nil {
			return added, fmt.Errorf("category %s: %w", entry.category, err)
		}
		created, err := store.Create(transaction.Draft{
			Title: entry.title, Type: transaction.Expense, Value: entry.value, CardID: owners[entry.account].ID,
			CategoryID: group.ID, Date: daysAgo(entry.daysAgo), Tags: entry.tags,
		})
		if err != nil {
			return added, fmt.Errorf("create transaction %s: %w", entry.title, err)
		}
		added++
		if entry.daysAgo > seedPaymentDaysAgo {
			oldest = created
		}
	}
	payer, err := account.NewStore(db).Find("INTER", false)
	if err != nil {
		return added, fmt.Errorf("account INTER: %w", err)
	}
	if _, err := store.PayBill(oldest.BillID, payer.ID, seedPaymentValue, daysAgo(seedPaymentDaysAgo)); err != nil {
		return added, fmt.Errorf("pay bill: %w", err)
	}
	return added + 1, nil
}

// seedBaseTransactions adds the base transactions. It returns the number
// of transactions in the database: a transfer is two.
func seedBaseTransactions(db *gorm.DB) (int, error) {
	accounts, categories, store := account.NewStore(db), category.NewStore(db), transaction.NewStore(db)
	added := 0
	for _, entry := range seedTransactions {
		from, err := accounts.Find(entry.account, false)
		if err != nil {
			return added, fmt.Errorf("account %s: %w", entry.account, err)
		}
		group, err := categories.Find(entry.category)
		if err != nil {
			return added, fmt.Errorf("category %s: %w", entry.category, err)
		}
		draft := transaction.Draft{
			Title: entry.title, Value: entry.value, AccountID: from.ID, CategoryID: group.ID,
			Date: daysAgo(entry.daysAgo), Tags: entry.tags, Received: entry.received, Type: transaction.Expense,
		}
		added++
		switch {
		case entry.to != "":
			to, err := accounts.Find(entry.to, false)
			if err != nil {
				return added, fmt.Errorf("account %s: %w", entry.to, err)
			}
			draft.Type, draft.ToAccountID = transaction.Transfer, to.ID
			added++
		case group.Color == "green":
			draft.Type = transaction.Income
		}
		if _, err := store.Create(draft); err != nil {
			return added, fmt.Errorf("create transaction %s: %w", entry.title, err)
		}
	}
	return added, nil
}

// seedMoreTransactions adds count transactions with random values in the
// last 60 days: one transfer, which is two transactions, and expenses or
// incomes. It uses the accounts and the categories the database has, so
// it adds fewer when they are missing. It returns the number it added.
func seedMoreTransactions(db *gorm.DB, count int) (int, error) {
	accounts, err := account.NewStore(db).List(false)
	if err != nil {
		return 0, err
	}
	categories, store := category.NewStore(db), transaction.NewStore(db)
	value := func(currency string) int64 {
		// Up to 500.00 in a normal currency or 0.005 BTC.
		if currency == "BTC" {
			return 1_000 + rand.Int64N(499_000)
		}
		return 100 + rand.Int64N(49_900)
	}
	added := 0
	if transfer, err := categories.Find(category.TransferCode); err == nil && len(accounts) > 1 && count > 1 {
		from, to := rand.IntN(len(accounts)), rand.IntN(len(accounts)-1)
		if to >= from {
			to++
		}
		draft := transaction.Draft{
			Title: "Transfer to " + accounts[to].Name, Type: transaction.Transfer, Value: value(accounts[from].Currency),
			AccountID: accounts[from].ID, ToAccountID: accounts[to].ID, CategoryID: transfer.ID, Date: daysAgo(rand.IntN(60)),
		}
		if accounts[from].Currency != accounts[to].Currency {
			draft.Received = value(accounts[to].Currency)
		}
		if _, err := store.Create(draft); err != nil {
			return added, fmt.Errorf("create transfer: %w", err)
		}
		added += 2
	}
	for len(accounts) > 0 && added < count {
		entry := seedMovementPool[rand.IntN(len(seedMovementPool))]
		group, err := categories.Find(entry.category)
		if err != nil {
			// The user deleted this category: use what is there.
			all, err := categories.List()
			if err != nil || len(all) == 0 {
				return added, err
			}
			group = all[rand.IntN(len(all))]
		}
		kind := transaction.Expense
		if group.Color == "green" {
			kind = transaction.Income
		}
		current := accounts[rand.IntN(len(accounts))]
		_, err = store.Create(transaction.Draft{
			Title: entry.title, Type: kind, Value: value(current.Currency), AccountID: current.ID, CategoryID: group.ID,
			Date: daysAgo(rand.IntN(60)), Tags: entry.tags,
		})
		if err != nil {
			return added, fmt.Errorf("create transaction %s: %w", entry.title, err)
		}
		added++
	}
	return added, nil
}

func seedBase(store *account.Store) error {
	for _, entry := range seedAccounts {
		created := entry.account
		if err := store.Create(&created); err != nil {
			return fmt.Errorf("create account %s: %w", created.Code, err)
		}
		if entry.archived {
			if err := store.Archive(created.ID); err != nil {
				return fmt.Errorf("archive account %s: %w", created.Code, err)
			}
		}
	}
	return nil
}

// seedMore adds count accounts with random values. It prefers names the
// database does not have yet.
func seedMore(store *account.Store, count int) error {
	existing, err := store.List(true)
	if err != nil {
		return err
	}
	used := map[string]bool{}
	for _, current := range existing {
		used[current.Name] = true
	}
	taken := func(code string) bool {
		inUse, err := store.CodeTaken(code, 0)
		return err != nil || inUse
	}
	for range count {
		name := seedNames[rand.IntN(len(seedNames))]
		for attempt := 2; used[name]; attempt++ {
			name = seedNames[rand.IntN(len(seedNames))]
			// After many accounts all names are in use: add a number.
			if attempt > len(seedNames) {
				name = fmt.Sprintf("%s %d", name, len(used)+1)
			}
		}
		used[name] = true
		currency := money.Codes()[rand.IntN(len(money.Codes()))]
		// Up to 15,000.00 in a normal currency or 2 BTC; one in five is negative.
		balance := rand.Int64N(1_500_000)
		if currency == "BTC" {
			balance = rand.Int64N(200_000_000)
		}
		if rand.IntN(5) == 0 {
			balance = -balance / 10
		}
		created := account.Account{
			Name:           name,
			Code:           code.Suggest(name, taken),
			Description:    seedDescriptions[rand.IntN(len(seedDescriptions))],
			Color:          account.Palette[rand.IntN(len(account.Palette))].Key,
			Currency:       currency,
			OpeningBalance: balance,
		}
		if err := store.Create(&created); err != nil {
			return fmt.Errorf("create account %s: %w", name, err)
		}
	}
	return nil
}
