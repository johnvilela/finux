package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"

	"finux/src/account"
	"finux/src/money"
	"finux/src/storage"
	"finux/src/ui"
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
// exists.
const seedBatch = 3

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
  - a database that does not exist is created with a base set of accounts;
  - a database that exists keeps its data and gets ` + strconv.Itoa(seedBatch) + ` more random accounts.
'make dev' runs it after each build.

` + green.Render("Options:") + `
  --new-db  Delete the database first, then create it with the base accounts

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
	created, err := storage.Init(path, models...)
	if err != nil {
		return "", err
	}
	db, err := storage.Open(path)
	if err != nil {
		return "", err
	}
	store := account.NewStore(db)
	if created {
		if err := seedBase(store); err != nil {
			return "", err
		}
		return fmt.Sprintf("Development database created: %s (%d accounts)", path, len(seedAccounts)), nil
	}
	if err := seedMore(store, seedBatch); err != nil {
		return "", err
	}
	all, err := store.List(true)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Development database updated: %s (%d accounts added, %d in total)", path, seedBatch, len(all)), nil
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
			Code:           account.SuggestCode(name, taken),
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
