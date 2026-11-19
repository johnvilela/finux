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
  config            Change the settings and set up AI agents
  accounts, ac      Manage accounts
  categories, ct    Manage categories
  transactions, tx  Manage transactions
  credit-card, cc   Manage credit cards
  goals, gl         Manage goals
  budgets, bg       Manage budgets
  scheduled, sc     Manage scheduled transactions
  resume, rs        Show your financial resume
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

func TestConfigCommand(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	help := succeed(t, home, "config")
	for _, want := range []string{"Usage: finux config [command]", "doctor", "Check the database", "mcp", "Run the MCP server for AI agents", "skills", "Install the skills for AI agents"} {
		if !strings.Contains(help, want) {
			t.Errorf("config help lacks %q:\n%s", want, help)
		}
	}
	for _, flag := range []string{"help", "--help", "-h"} {
		if got := succeed(t, home, "config", flag); got != help {
			t.Errorf("finux config %s differs from 'finux config'", flag)
		}
	}
	fail(t, home, 2, "config: unknown command: other", "config", "other")
	// The commands of config are not commands of finux any more.
	for _, old := range []string{"doctor", "mcp", "skills"} {
		fail(t, home, 2, "unsupported arguments: "+old, old)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("the commands created a database: %v", err)
	}
}

func TestConfigSettings(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	fail(t, home, 1, "Run 'finux init'", "config", "get")
	succeed(t, home, "init")
	// A setting that was never changed has its default value.
	if got := succeed(t, home, "config", "get"); got != "automatic-schedules  off\nai-provider          none\n" {
		t.Fatalf("config get = %q", got)
	}
	if got := succeed(t, home, "config", "set", "ai-provider", "Anthropic"); got != "✓ ai-provider set to anthropic\n" {
		t.Fatalf("config set = %q", got)
	}
	if got := succeed(t, home, "config", "set", "automatic-schedules", "on"); got != "✓ automatic-schedules set to on\n" {
		t.Fatalf("config set = %q", got)
	}
	if got := succeed(t, home, "config", "get"); got != "automatic-schedules  on\nai-provider          anthropic\n" {
		t.Fatalf("config get = %q", got)
	}
	if got := succeed(t, home, "config", "get", "ai-provider"); got != "anthropic\n" {
		t.Fatalf("config get ai-provider = %q", got)
	}
	fail(t, home, 2, `config: unknown value "skynet" for ai-provider: use one of none, anthropic, openai, google, ollama`, "config", "set", "ai-provider", "skynet")
	fail(t, home, 2, `config: unknown setting "color": use one of automatic-schedules, ai-provider`, "config", "set", "color", "red")
	fail(t, home, 2, `config: unknown setting "color"`, "config", "get", "color")
	fail(t, home, 2, "config: set: give a setting and its value", "config", "set", "ai-provider")
	fail(t, home, 2, "config: set: unexpected argument: extra", "config", "set", "ai-provider", "none", "extra")
	fail(t, home, 2, "config: get: unexpected argument: extra", "config", "get", "ai-provider", "extra")
	// Without a terminal, config shows the help; it names the settings.
	help := succeed(t, home, "config")
	for _, want := range []string{"get [setting]", "set <setting> <value>", "automatic-schedules", "off, on", "ai-provider", "none, anthropic, openai, google, ollama", "Without a command"} {
		if !strings.Contains(help, want) {
			t.Errorf("config help lacks %q:\n%s", want, help)
		}
	}
}
