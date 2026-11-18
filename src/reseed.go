package main

import (
	"fmt"
	"strings"

	"finux/src/category"
	"finux/src/ui"
)

func reseedHelp() string {
	return "Add missing base data\n\n" +
		green.Render("Usage:") + ` finux reseed

Adds the base categories that are not in the database. It finds them by
code. It does not change or delete the categories you have.

'finux init' adds the base categories one time only, on the first setup.
Use reseed to get a base category back after you deleted it.
`
}

func runReseed(args []string, io streams) int {
	if len(args) > 0 {
		if isHelp(args[0]) {
			return write(io, reseedHelp())
		}
		fmt.Fprintf(io.stderr, "finux: reseed: unexpected argument: %s\nRun 'finux reseed --help' for usage.\n", args[0])
		return 2
	}
	db, err := openDatabase()
	if err != nil {
		fmt.Fprintf(io.stderr, "finux: %v\n", err)
		return 1
	}
	added, err := category.NewStore(db).AddMissingBase()
	if err != nil {
		fmt.Fprintf(io.stderr, "finux: reseed: %v\n", err)
		return 1
	}
	if len(added) == 0 {
		return write(io, ui.Success("Base data is complete")+"\n")
	}
	return write(io, ui.Success(fmt.Sprintf("Base categories added: %d (%s)", len(added), strings.Join(added, ", ")))+"\n")
}
