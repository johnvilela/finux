package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"

	"finux/src/category"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

// usageError is a mistake in the command line. It gives exit status 2.
type usageError string

func (e usageError) Error() string { return string(e) }

// notice ends a command with a message and exit status 0, for example
// when the user cancels a prompt.
type notice string

func (n notice) Error() string { return string(n) }

const cancelled = notice("Cancelled.")

// exitStatus reports the error of a module command and returns the exit
// status: 0 for nil and notices, 2 for usage errors, 1 for the rest.
func exitStatus(io streams, module string, err error) int {
	var usage usageError
	var message notice
	switch {
	case err == nil:
		return 0
	case errors.As(err, &message):
		return write(io, ui.Hint(string(message))+"\n")
	case errors.As(err, &usage):
		fmt.Fprintf(io.stderr, "finux: %s: %v\nRun 'finux %s --help' for usage.\n", module, err, module)
		return 2
	}
	fmt.Fprintf(io.stderr, "finux: %v\n", err)
	return 1
}

// parse reads flags and positional arguments in any order. It returns the
// positional arguments and the names of the flags that were given.
func parse(name string, args []string, define func(*flag.FlagSet), maxPositional int) ([]string, map[string]bool, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if define != nil {
		define(flags)
	}
	var positional []string
	for {
		if err := flags.Parse(args); err != nil {
			return nil, nil, usageError(name + ": " + err.Error())
		}
		if flags.NArg() == 0 {
			break
		}
		positional = append(positional, flags.Arg(0))
		args = flags.Args()[1:]
	}
	if len(positional) > maxPositional {
		return nil, nil, usageError(name + ": unexpected argument: " + positional[maxPositional])
	}
	given := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { given[f.Name] = true })
	return positional, given, nil
}

func reference(positional []string) string {
	if len(positional) == 0 {
		return ""
	}
	return positional[0]
}

// program runs an interactive screen and returns its final state.
func program[M tea.Model](model M) (M, error) {
	final, err := tea.NewProgram(model).Run()
	if err != nil {
		return model, fmt.Errorf("run screen: %w", err)
	}
	return final.(M), nil
}

// pick opens a selector and returns the index of the entry the user
// selected. It returns the cancelled notice when the user leaves.
func pick(title, singular, plural string, entries []ui.Entry) (int, error) {
	selector, err := program(ui.NewSelector(title, singular, plural, entries))
	if err != nil {
		return 0, err
	}
	index, ok := selector.Selected()
	if !ok {
		return 0, cancelled
	}
	return index, nil
}

// today returns the current date as YYYY-MM-DD. The FINUX_TODAY
// environment variable replaces it, for tests and demonstrations.
func today() string { return transaction.Today() }

// plural returns a count with its noun: "1 transaction", "2 transactions".
func plural(count int64, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

// otherCategory returns the category OTHER, which receives the
// transactions that lose their category. It must accept all types.
func otherCategory(db *gorm.DB) (category.Category, error) {
	other, err := category.NewStore(db).Find(category.OtherCode)
	switch {
	case errors.Is(err, category.ErrNotFound):
		return other, fmt.Errorf("the category %s must receive them, but it does not exist. Run 'finux ct add --name Other --code %s --color grey' to add it", category.OtherCode, category.OtherCode)
	case err != nil:
		return other, err
	case other.Color != "grey":
		return other, fmt.Errorf("the category %s must receive them, but it is not grey. Run 'finux ct edit %s --color grey'", category.OtherCode, category.OtherCode)
	}
	return other, nil
}
