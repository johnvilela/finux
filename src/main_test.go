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
  daemon            Run the Telegram bot and the background jobs
  update            Update Finux to the newest version
  version           Show the version
  help              Show this help

Options:
  -h, --help        Show this help
      --version     Show the version
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
	for _, want := range []string{"Usage: finux config [command]", "doctor", "Check the database", "mcp", "Run the MCP server for AI agents", "skills", "List, install or remove the skills for AI agents", "ai [agent]"} {
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

// settingsList is the output of 'config get' with the two first
// settings changed and the others at their defaults.
func settingsList(schedules, provider string) string {
	return "automatic-schedules   " + schedules + "\n" +
		"ai-provider           " + provider + "\n" +
		"ai-model              \n" +
		"telegram-token        (not set)\n" +
		"telegram-reminders    off\n" +
		"telegram-resume       off\n" +
		"telegram-resume-time  08:00\n" +
		"daemon-mcp            \n"
}

func TestConfigTextSettings(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	succeed(t, home, "init")
	const token = "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw_"
	// A secret never shows on the output.
	if got := succeed(t, home, "config", "set", "telegram-token", token); got != "✓ telegram-token set to (set)\n" {
		t.Fatalf("config set telegram-token = %q", got)
	}
	if got := succeed(t, home, "config", "get", "telegram-token"); got != "(set)\n" {
		t.Fatalf("config get telegram-token = %q", got)
	}
	if got := succeed(t, home, "config", "get"); strings.Contains(got, token) || !strings.Contains(got, "telegram-token        (set)\n") {
		t.Fatalf("config get = %q", got)
	}
	if got := succeed(t, home, "config", "set", "telegram-resume-time", "7:30"); got != "✓ telegram-resume-time set to 07:30\n" {
		t.Fatalf("config set telegram-resume-time = %q", got)
	}
	fail(t, home, 2, "config: unknown value for telegram-resume-time: give a time as HH:MM, for example 08:00", "config", "set", "telegram-resume-time", "noon")
	fail(t, home, 2, "config: unknown value for daemon-mcp: give an address as host:port, for example 127.0.0.1:8765", "config", "set", "daemon-mcp", "here")
	if got := succeed(t, home, "config", "set", "telegram-token", ""); got != "✓ telegram-token set to (not set)\n" {
		t.Fatalf("config set of an empty token = %q", got)
	}
}

func TestConfigSettings(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	fail(t, home, 1, "Run 'finux init'", "config", "get")
	succeed(t, home, "init")
	// A setting that was never changed has its default value.
	if got := succeed(t, home, "config", "get"); got != settingsList("off", "none") {
		t.Fatalf("config get = %q", got)
	}
	if got := succeed(t, home, "config", "set", "ai-provider", "Claude"); got != "✓ ai-provider set to claude\n" {
		t.Fatalf("config set = %q", got)
	}
	if got := succeed(t, home, "config", "set", "automatic-schedules", "on"); got != "✓ automatic-schedules set to on\n" {
		t.Fatalf("config set = %q", got)
	}
	if got := succeed(t, home, "config", "get"); got != settingsList("on", "claude") {
		t.Fatalf("config get = %q", got)
	}
	if got := succeed(t, home, "config", "get", "ai-provider"); got != "claude\n" {
		t.Fatalf("config get ai-provider = %q", got)
	}
	fail(t, home, 2, `config: unknown value "skynet" for ai-provider: use one of none, claude, codex, opencode`, "config", "set", "ai-provider", "skynet")
	fail(t, home, 2, `config: unknown setting "color": use one of automatic-schedules, ai-provider, ai-model, telegram-token`, "config", "set", "color", "red")
	fail(t, home, 2, `config: unknown setting "color"`, "config", "get", "color")
	fail(t, home, 2, "config: set: give a setting and its value", "config", "set", "ai-provider")
	fail(t, home, 2, "config: set: unexpected argument: extra", "config", "set", "ai-provider", "none", "extra")
	fail(t, home, 2, "config: get: unexpected argument: extra", "config", "get", "ai-provider", "extra")
	// Without a terminal, config shows the help; it names the settings.
	help := succeed(t, home, "config")
	for _, want := range []string{"get [setting]", "set <setting> <value>", "automatic-schedules", "off, on", "ai-provider", "none, claude, codex, opencode", "Without a command"} {
		if !strings.Contains(help, want) {
			t.Errorf("config help lacks %q:\n%s", want, help)
		}
	}
}
