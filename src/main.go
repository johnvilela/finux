package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

func main() {
	output := colorprofile.NewWriter(os.Stdout, os.Environ())
	if os.Getenv("NO_COLOR") != "" {
		output.Profile = colorprofile.NoTTY
	}
	os.Exit(run(os.Args[1:], output, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 1 || len(args) == 1 && args[0] != "help" && args[0] != "--help" && args[0] != "-h" {
		fmt.Fprintf(stderr, "finux: unsupported arguments: %s\nRun 'finux --help' for usage.\n", strings.Join(args, " "))
		return 2
	}
	if _, err := io.WriteString(stdout, welcome()); err != nil {
		fmt.Fprintf(stderr, "finux: write output: %v\n", err)
		return 1
	}
	return 0
}

func welcome() string {
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
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
	return strings.Join(lines, "\n") + "\n\nYour personal finance manager\n\n" +
		green.Render("Usage:") + " finux [command]\n\n" +
		green.Render("Commands:") + "\n  help        Show this help\n\n" +
		green.Render("Options:") + "\n  -h, --help  Show this help\n"
}
