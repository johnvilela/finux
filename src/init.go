package main

import (
	"errors"
	"fmt"
	"slices"

	"finux/src/account"
	"finux/src/budget"
	"finux/src/card"
	"finux/src/category"
	"finux/src/goal"
	"finux/src/schedule"
	"finux/src/setting"
	"finux/src/storage"
	"finux/src/telegram"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

// models lists the tables that 'finux init' migrates.
var models = []any{&account.Account{}, &category.Category{}, &card.Card{}, &card.Bill{}, &goal.Goal{}, &budget.Budget{}, &budget.Link{}, &schedule.Schedule{}, &transaction.Transaction{}, &setting.Setting{}, &telegram.User{}, &telegram.Notice{}, &telegram.Session{}, &telegram.Pairing{}}

func initHelp() string {
	return "Set up the Finux database\n\n" +
		green.Render("Usage:") + ` finux init

Creates the database file if it does not exist, then applies the database
migrations. The first setup also adds the base categories. It is safe to
run again: it keeps your data and does not add base data again. To get
a base category back after you deleted it, add it with 'finux ct add'.

The database is the file in the FINUX_DB environment variable. Without
FINUX_DB, it is ~/.finux/finux.db.
`
}

func runInit(args []string, io streams) int {
	if len(args) > 0 {
		if isHelp(args[0]) {
			return write(io, initHelp())
		}
		fmt.Fprintf(io.stderr, "finux: init: unexpected argument: %s\nRun 'finux init --help' for usage.\n", args[0])
		return 2
	}
	path, err := databasePath()
	if err != nil {
		fmt.Fprintf(io.stderr, "finux: %v\n", err)
		return 1
	}
	report, err := setupDatabase(path)
	if err != nil {
		fmt.Fprintf(io.stderr, "finux: %v\n", err)
		return 1
	}
	state := "up to date"
	if report.created {
		state = "created"
	}
	text := ui.Success("Database "+state+": "+path) + "\n"
	if report.baseCategories > 0 {
		text += ui.Success(fmt.Sprintf("Base categories added: %d", report.baseCategories)) + "\n"
	}
	return write(io, text)
}

// setupReport says what setupDatabase did.
type setupReport struct {
	// created is true when the database file is new.
	created bool
	// baseCategories is the number of base categories added.
	baseCategories int
}

// setupDatabase creates the database if necessary, applies the
// migrations and adds the base data of each table this run created. A
// later run adds no base data, so records the user deleted stay deleted.
func setupDatabase(path string) (setupReport, error) {
	result, err := storage.Init(path, models...)
	if err != nil {
		return setupReport{}, err
	}
	report := setupReport{created: result.Created}
	if !slices.Contains(result.NewTables, "categories") {
		return report, nil
	}
	db, err := storage.Open(path)
	if err != nil {
		return setupReport{}, err
	}
	added, err := category.NewStore(db).AddMissingBase()
	if err != nil {
		return setupReport{}, fmt.Errorf("add base categories: %w", err)
	}
	report.baseCategories = len(added)
	return report, nil
}

// openDatabase opens the database for a command. Only 'finux init'
// creates and migrates it.
func openDatabase() (*gorm.DB, error) {
	path, err := databasePath()
	if err != nil {
		return nil, err
	}
	db, err := storage.Open(path)
	if errors.Is(err, storage.ErrNotInitialized) {
		return nil, fmt.Errorf("%w. Run 'finux init' to create it", err)
	}
	return db, err
}
