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
		case "doctor":
			return runDoctor(args[1:], io)
		case "seed":
			// Only a development build has the seed command.
			if devDatabase != "" {
				return runSeed(args[1:], io)
			}
		case "accounts", "ac":
			return runAccounts(args[1:], io)
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
	commands := "\n  init          Set up the database\n  doctor        Check the database\n"
	if devDatabase != "" {
		commands += "  seed          Add sample data to the development database\n"
	}
	return strings.Join(lines, "\n") + "\n\nYour personal finance manager\n\n" +
		green.Render("Usage:") + " finux [command]\n\n" +
		green.Render("Commands:") + commands + "  accounts, ac  Manage accounts\n  help          Show this help\n\n" +
		green.Render("Options:") + "\n  -h, --help    Show this help\n"
}
