package main

import (
	"errors"
	"fmt"
	"strings"

	"finux/src/storage"
	"finux/src/ui"
)

func doctorHelp() string {
	return "Check the Finux setup\n\n" +
		green.Render("Usage:") + ` finux doctor

Reports if the database exists and if it has all the database migrations.
It does not change the database. To correct a problem, run 'finux init'.

The exit status is 0 when all checks pass and 1 when a check fails.
`
}

func runDoctor(args []string, io streams) int {
	if len(args) > 0 {
		if isHelp(args[0]) {
			return write(io, doctorHelp())
		}
		fmt.Fprintf(io.stderr, "finux: doctor: unexpected argument: %s\nRun 'finux doctor --help' for usage.\n", args[0])
		return 2
	}
	report, healthy, err := checkDatabase()
	if err != nil {
		fmt.Fprintf(io.stderr, "finux: %v\n", err)
		return 1
	}
	if status := write(io, report); status != 0 || healthy {
		return status
	}
	return 1
}

// checkDatabase reports if the database exists and has all migrations.
func checkDatabase() (report string, healthy bool, err error) {
	path, err := databasePath()
	if err != nil {
		return "", false, err
	}
	db, err := storage.Open(path)
	if errors.Is(err, storage.ErrNotInitialized) {
		return ui.Failure("Database not found: "+path) + "\n  Run 'finux init' to create it.\n", false, nil
	} else if err != nil {
		return "", false, err
	}
	report = ui.Success("Database found: "+path) + "\n"
	pending, err := storage.Pending(db, models...)
	if err != nil {
		return "", false, err
	}
	if len(pending) > 0 {
		return report + ui.Failure("Migrations pending: "+strings.Join(pending, ", ")) + "\n  Run 'finux init' to apply them.\n", false, nil
	}
	return report + ui.Success("Migrations up to date") + "\n", true, nil
}
