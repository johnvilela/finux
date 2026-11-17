package main

import (
	"errors"
	"fmt"

	"finux/src/account"
	"finux/src/storage"
	"finux/src/ui"

	"gorm.io/gorm"
)

// models lists the tables that 'finux init' migrates.
var models = []any{&account.Account{}}

func initHelp() string {
	return "Set up the Finux database\n\n" +
		green.Render("Usage:") + ` finux init

Creates the database file if it does not exist, then applies the database
migrations. It is safe to run again: it keeps your data.

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
	if err == nil {
		var created bool
		if created, err = storage.Init(path, models...); err == nil {
			state := "up to date"
			if created {
				state = "created"
			}
			return write(io, ui.Success("Database "+state+": "+path)+"\n")
		}
	}
	fmt.Fprintf(io.stderr, "finux: %v\n", err)
	return 1
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
