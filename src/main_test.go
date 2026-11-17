package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
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
  help        Show this help

Options:
  -h, --help  Show this help
`

func TestFinuxCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "finux")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build finux: %v\n%s", err, output)
	}
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
