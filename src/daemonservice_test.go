package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSystemd puts a systemctl and a journalctl that write their
// arguments to a log first in a PATH. It returns the PATH and a
// function that gives the lines of the log and empties it.
func fakeSystemd(t *testing.T) (path string, calls func() string) {
	t.Helper()
	folder := t.TempDir()
	log := filepath.Join(folder, "calls.log")
	for _, name := range []string{"systemctl", "journalctl"} {
		script := "#!/bin/sh\necho \"" + name + " $*\" >> '" + log + "'\necho output of " + name + "\n"
		if err := os.WriteFile(filepath.Join(folder, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return folder + ":" + os.Getenv("PATH"), func() string {
		text, _ := os.ReadFile(log)
		os.Remove(log)
		return string(text)
	}
}

func TestDaemonService(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	path, calls := fakeSystemd(t)
	database := filepath.Join(home, "finux.db")
	run := func(args ...string) (string, string, int) {
		t.Helper()
		return finuxEnv(t, []string{"FINUX_DB=" + database, "HOME=" + home, "PATH=" + path}, append([]string{"daemon"}, args...)...)
	}
	unit := filepath.Join(home, ".config", "systemd", "user", "finux.service")

	// The daemon needs a database.
	if _, stderr, status := run("install"); status != 1 || !strings.Contains(stderr, "Run 'finux init'") || calls() != "" {
		t.Fatalf("install without a database: status %d, stderr %q", status, stderr)
	}
	succeed(t, home, "init")
	for _, command := range []string{"start", "stop", "restart", "status", "logs", "uninstall"} {
		if _, stderr, status := run(command); status != 1 || !strings.Contains(stderr, "finux: the daemon is not installed. Run 'finux daemon install'") || calls() != "" {
			t.Fatalf("%s before install: status %d, stderr %q", command, status, stderr)
		}
	}

	stdout, stderr, status := run("install")
	if status != 0 || stderr != "" || !strings.HasPrefix(stdout, "✓ Finux daemon installed and started\n") || !strings.Contains(stdout, "loginctl enable-linger") || !strings.Contains(stdout, "finux daemon logs") {
		t.Fatalf("install: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if got := calls(); got != "systemctl --user daemon-reload\nsystemctl --user enable finux.service\nsystemctl --user restart finux.service\n" {
		t.Fatalf("calls of install = %q", got)
	}
	binary, err := filepath.EvalSymlinks(finuxBinary(t))
	if err != nil {
		t.Fatal(err)
	}
	want := "[Unit]\n" +
		"Description=Finux daemon: Telegram bot and background jobs\n" +
		"After=network-online.target\n" +
		"Wants=network-online.target\n\n" +
		"[Service]\n" +
		"ExecStart=\"" + binary + "\" daemon run\n" +
		"Environment=\"FINUX_DB=" + database + "\"\n" +
		"Restart=on-failure\n" +
		"RestartSec=10\n\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"
	if got, err := os.ReadFile(unit); err != nil || string(got) != want {
		t.Fatalf("unit = %q, %v", got, err)
	}

	for command, call := range map[string]string{
		"start": "systemctl --user start finux.service\n", "stop": "systemctl --user stop finux.service\n",
		"restart": "systemctl --user restart finux.service\n",
	} {
		if stdout, stderr, status := run(command); status != 0 || stderr != "" || !strings.HasPrefix(stdout, "✓ Finux daemon ") {
			t.Fatalf("%s: status %d, stdout %q, stderr %q", command, status, stdout, stderr)
		}
		if got := calls(); got != call {
			t.Fatalf("calls of %s = %q", command, got)
		}
	}
	// status and logs show the output of systemd.
	if stdout, _, status := run("status"); status != 0 || stdout != "output of systemctl\n" || calls() != "systemctl --user status finux.service --no-pager\n" {
		t.Fatalf("status: status %d, stdout %q", status, stdout)
	}
	if stdout, _, status := run("logs"); status != 0 || stdout != "output of journalctl\n" || calls() != "journalctl --user --unit finux.service --lines 50 --no-pager\n" {
		t.Fatalf("logs: status %d, stdout %q", status, stdout)
	}
	if _, _, status := run("logs", "--follow"); status != 0 || calls() != "journalctl --user --unit finux.service --lines 50 --follow\n" {
		t.Fatalf("logs --follow: status %d", status)
	}

	if stdout, stderr, status := run("uninstall"); status != 0 || stderr != "" || stdout != "✓ Finux daemon removed\n" {
		t.Fatalf("uninstall: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if got := calls(); got != "systemctl --user disable --now finux.service\nsystemctl --user daemon-reload\n" {
		t.Fatalf("calls of uninstall = %q", got)
	}
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Fatalf("the unit is there after uninstall: %v", err)
	}

	if _, stderr, status := run("nope"); status != 2 || !strings.Contains(stderr, "daemon: unknown command: nope") {
		t.Fatalf("unknown command: status %d, stderr %q", status, stderr)
	}
	if stdout, _, status := run("--help"); status != 0 || !strings.Contains(stdout, "finux daemon install") {
		t.Fatalf("help: status %d, stdout %q", status, stdout)
	}
	// Without systemd, the daemon runs in a terminal only.
	if _, stderr, status := finuxEnv(t, []string{"FINUX_DB=" + database, "HOME=" + home, "PATH=" + t.TempDir()}, "daemon", "install"); status != 1 || !strings.Contains(stderr, "systemctl was not found") || !strings.Contains(stderr, "finux daemon run") {
		t.Fatalf("install without systemd: status %d, stderr %q", status, stderr)
	}
	// A development build has no service.
	dev := devBinary(t, filepath.Join(t.TempDir(), "finux-dev.db"))
	if _, stderr, status := runBinary(t, dev, []string{"PATH=" + path}, "daemon", "install"); status != 1 || !strings.Contains(stderr, "development build") {
		t.Fatalf("install of a dev build: status %d, stderr %q", status, stderr)
	}
}
