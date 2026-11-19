package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
)

// streams holds the outputs of a command and what the terminal can do.
type streams struct {
	stdout, stderr io.Writer
	// interactive is true when input and output are terminals, so forms
	// and selectors can open.
	interactive bool
	// plain is true when output is not a terminal. Tables and cards then
	// have no borders.
	plain bool
}

func main() {
	output := colorprofile.NewWriter(os.Stdout, os.Environ())
	if os.Getenv("NO_COLOR") != "" {
		output.Profile = colorprofile.NoTTY
	}
	terminalOutput := term.IsTerminal(os.Stdout.Fd())
	os.Exit(run(os.Args[1:], streams{
		stdout:      output,
		stderr:      os.Stderr,
		interactive: terminalOutput && term.IsTerminal(os.Stdin.Fd()),
		plain:       !terminalOutput,
	}))
}

func run(args []string, io streams) int {
	if len(args) > 0 {
		switch args[0] {
		case "init":
			return runInit(args[1:], io)
		case "config":
			return runConfig(args[1:], io)
		case "update":
			return runUpdate(args[1:], io)
		case "version", "--version":
			return runVersion(args[1:], io)
		case "seed", "reseed":
			// Only a development build has the seed and reseed commands.
			if devDatabase == "" {
				break
			}
			if args[0] == "seed" {
				return runSeed(args[1:], io)
			}
			return runReseed(args[1:], io)
		case "accounts", "ac":
			return runAccounts(args[1:], io)
		case "categories", "ct":
			return runCategories(args[1:], io)
		case "transactions", "tx":
			return runTransactions(args[1:], io)
		case "credit-card", "credit-cards", "cc":
			return runCreditCards(args[1:], io)
		case "goals", "goal", "gl":
			return runGoals(args[1:], io)
		case "budgets", "budget", "bg":
			return runBudgets(args[1:], io)
		case "scheduled", "schedule", "sc":
			return runScheduled(args[1:], io)
		case "resume", "rs":
			return runResume(args[1:], io)
		}
	}
	if len(args) > 1 || len(args) == 1 && !isHelp(args[0]) {
		fmt.Fprintf(io.stderr, "finux: unsupported arguments: %s\nRun 'finux --help' for usage.\n", strings.Join(args, " "))
		return 2
	}
	return write(io, welcome())
}

// isHelp reports whether arg asks for help. All modules accept the same
// three forms.
func isHelp(arg string) bool {
	return arg == "help" || arg == "--help" || arg == "-h"
}

// write prints text to stdout and returns the exit status.
func write(io streams, text string) int {
	if _, err := fmt.Fprint(io.stdout, text); err != nil {
		fmt.Fprintf(io.stderr, "finux: write output: %v\n", err)
		return 1
	}
	return 0
}

var green = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

func welcome() string {
	logo := `███████╗██╗███╗   ██╗██╗   ██╗██╗  ██╗
██╔════╝██║████╗  ██║██║   ██║╚██╗██╔╝
█████╗  ██║██╔██╗ ██║██║   ██║ ╚███╔╝
██╔══╝  ██║██║╚██╗██║██║   ██║ ██╔██╗
██║     ██║██║ ╚████║╚██████╔╝██╔╝ ██╗
╚═╝     ╚═╝╚═╝  ╚═══╝ ╚═════╝ ╚═╝  ╚═╝`
	lines := strings.Split(logo, "\n")
	for i, line := range lines {
		lines[i] = green.Render(line)
	}
	commands := "\n  init              Set up the database\n  config            Change the settings and set up AI agents\n"
	if devDatabase != "" {
		commands += "  seed              Add sample data to the development database\n  reseed            Add missing base data\n"
	}
	return strings.Join(lines, "\n") + "\n\nYour personal finance manager\n\n" +
		green.Render("Usage:") + " finux [command]\n\n" +
		green.Render("Commands:") + commands + "  accounts, ac      Manage accounts\n  categories, ct    Manage categories\n  transactions, tx  Manage transactions\n  credit-card, cc   Manage credit cards\n  goals, gl         Manage goals\n  budgets, bg       Manage budgets\n  scheduled, sc     Manage scheduled transactions\n  resume, rs        Show your financial resume\n  update            Update Finux to the newest version\n  version           Show the version\n  help              Show this help\n\n" +
		green.Render("Options:") + "\n  -h, --help        Show this help\n      --version     Show the version\n"
}
