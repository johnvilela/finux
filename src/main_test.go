package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const plainWelcome = `███████╗██╗███╗   ██╗██╗   ██╗██╗  ██╗
██╔════╝██║████╗  ██║██║   ██║╚██╗██╔╝
█████╗  ██║██╔██╗ ██║██║   ██║ ╚███╔╝
██╔══╝  ██║██║╚██╗██║██║   ██║ ██╔██╗
██║     ██║██║ ╚████║╚██████╔╝██╔╝ ██╗
╚═╝     ╚═╝╚═╝  ╚═══╝ ╚═════╝ ╚═╝  ╚═╝

Your personal finance manager

Usage: finux [command]

Commands:
  init              Set up the database
  doctor            Check the database
  reseed            Add missing base data
  accounts, ac      Manage accounts
  categories, ct    Manage categories
  transactions, tx  Manage transactions
  credit-card, cc   Manage credit cards
  goals, gl         Manage goals
  budgets, bg       Manage budgets
  scheduled, sc     Manage scheduled transactions
  resume, rs        Show your financial resume
  mcp               Run the MCP server for AI agents
  skills            Install the skills for AI agents
  help              Show this help

Options:
  -h, --help        Show this help
`

var (
	buildOnce   sync.Once
	buildDir    string
	buildOutput []byte
	buildErr    error
)

// finuxBinary builds the CLI once for all tests and returns its path.
func finuxBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		buildDir, buildErr = os.MkdirTemp("", "finux-test-")
		if buildErr == nil {
			buildOutput, buildErr = exec.Command("go", "build", "-o", filepath.Join(buildDir, "finux"), ".").CombinedOutput()
		}
	})
	if buildErr != nil {
		t.Fatalf("build finux: %v\n%s", buildErr, buildOutput)
	}
	return filepath.Join(buildDir, "finux")
}

func TestMain(m *testing.M) {
	code := m.Run()
	if buildDir != "" {
		os.RemoveAll(buildDir)
	}
	os.Exit(code)
}

func TestFinuxCLI(t *testing.T) {
	binary := finuxBinary(t)
	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}, {"unknown"}, {"--unknown"}, {"help", "extra"}} {
		name := strings.Join(args, " ")
		if name == "" {
			name = "startup"
		}
		t.Run(name, func(t *testing.T) {
			command := exec.Command(binary, args...)
			command.Env = []string{"TERM=xterm-256color", "NO_COLOR=", "CLICOLOR_FORCE=0"}
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			err := command.Run()
			invalid := len(args) > 1 || len(args) == 1 && args[0] != "help" && args[0] != "--help" && args[0] != "-h"
			if invalid {
				if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 2 {
					t.Fatalf("exit = %v, want status 2", err)
				}
				want := "finux: unsupported arguments: " + strings.Join(args, " ") + "\nRun 'finux --help' for usage.\n"
				if stdout.Len() != 0 || stderr.String() != want {
					t.Fatalf("stdout = %q, stderr = %q, want empty stdout and %q", stdout.String(), stderr.String(), want)
				}
				return
			}
			if err != nil || stderr.Len() != 0 || stdout.String() != plainWelcome {
				t.Fatalf("exit = %v, stderr = %q, stdout = %q", err, stderr.String(), stdout.String())
			}
		})
	}
	for _, tc := range []struct {
		name, noColor string
		colored       bool
	}{
		{"forced color", "", true},
		{"NO_COLOR", "1", false},
		{"NO_COLOR arbitrary value", "disabled", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := exec.Command(binary)
			command.Env = []string{"TERM=xterm-256color", "CLICOLOR_FORCE=1", "NO_COLOR=" + tc.noColor}
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatal(err)
			}
			if tc.colored {
				if !bytes.Contains(output, []byte("\x1b[32m")) {
					t.Fatalf("missing green styling: %q", output)
				}
				if !bytes.Contains(output, []byte("\n\nYour personal finance manager\n")) {
					t.Fatalf("tagline should use default color: %q", output)
				}
			} else if string(output) != plainWelcome {
				t.Fatalf("output = %q, want plain help", output)
			}
		})
	}
}
