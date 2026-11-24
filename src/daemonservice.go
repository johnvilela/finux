package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"finux/src/ui"
)

const (
	// daemonUnit is the name of the systemd user unit of the daemon.
	daemonUnit = "finux.service"
	// databaseVariable names the environment variable with the database.
	databaseVariable = "FINUX_DB"
)

// errNotInstalled means the systemd unit of the daemon does not exist.
var errNotInstalled = errors.New("the daemon is not installed. Run 'finux daemon install'")

// unitPath returns the file of the systemd user unit of the daemon.
func unitPath() (string, error) {
	folder, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the configuration folder: %w", err)
	}
	return filepath.Join(folder, "systemd", "user", daemonUnit), nil
}

// installed returns the file of the unit. It is an error when the file
// does not exist.
func installed() (string, error) {
	path, err := unitPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "", errNotInstalled
	} else if err != nil {
		return "", err
	}
	return path, nil
}

// unitText returns the text of the unit for a binary. The daemon uses
// the database of the command that installed it.
func unitText(binary string) string {
	// systemd reads % as the start of a specifier.
	quote := func(text string) string { return `"` + strings.ReplaceAll(text, "%", "%%") + `"` }
	environment := ""
	if database := os.Getenv(databaseVariable); database != "" {
		if absolute, err := filepath.Abs(database); err == nil {
			database = absolute
		}
		environment = "Environment=" + quote(databaseVariable+"="+database) + "\n"
	}
	// A service has a short PATH. The AI agent of /ai is often in a
	// folder of the user, so the service gets the PATH of this command.
	if path := os.Getenv("PATH"); path != "" {
		environment += "Environment=" + quote("PATH="+path) + "\n"
	}
	return "[Unit]\n" +
		"Description=Finux daemon: Telegram bot and background jobs\n" +
		"After=network-online.target\n" +
		"Wants=network-online.target\n\n" +
		"[Service]\n" +
		"ExecStart=" + quote(binary) + " daemon run\n" +
		environment +
		"Restart=on-failure\n" +
		"RestartSec=10\n\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"
}

// systemd runs a program of systemd for the user. With show, its output
// goes to the output of the command.
func systemd(io streams, show bool, program string, args ...string) error {
	found, err := exec.LookPath(program)
	if err != nil {
		return fmt.Errorf("%s was not found: these commands need systemd. Without it, run 'finux daemon run' in a terminal, or with the tool that starts your programs", program)
	}
	command := exec.Command(found, append([]string{"--user"}, args...)...)
	if show {
		command.Stdin, command.Stdout, command.Stderr = os.Stdin, io.stdout, io.stderr
		// The status of a daemon that is not active is not a failure of
		// this command, and Ctrl+C ends the logs.
		var exit *exec.ExitError
		if err := command.Run(); err != nil && !errors.As(err, &exit) {
			return err
		}
		return nil
	}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %w: %s", program, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

// installDaemon writes the unit of the daemon and starts it.
func installDaemon(io streams) error {
	if devDatabase != "" {
		return errors.New("a development build has no service: run './bin/finux-dev daemon run'")
	}
	db, _, err := openTelegram()
	if err != nil {
		return err
	}
	closeDatabase(db)
	binary, err := os.Executable()
	if err == nil {
		binary, err = filepath.EvalSymlinks(binary)
	}
	if err != nil {
		return fmt.Errorf("find the finux binary: %w", err)
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return systemd(io, false, "systemctl")
	}
	path, err := unitPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(unitText(binary)), 0o644); err != nil {
		return err
	}
	// restart, not start: a second install starts the new unit.
	for _, args := range [][]string{{"daemon-reload"}, {"enable", daemonUnit}, {"restart", daemonUnit}} {
		if err := systemd(io, false, "systemctl", args...); err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(io.stdout, ui.Success("Finux daemon installed and started")+"\n"+
		"It starts when you log in. To keep it running when you are not logged in:\n"+
		"  loginctl enable-linger\n"+
		"See what it does with 'finux daemon status' and 'finux daemon logs'.\n")
	return err
}

// uninstallDaemon stops the daemon and removes its unit.
func uninstallDaemon(io streams) error {
	path, err := installed()
	if err != nil {
		return err
	}
	if err := systemd(io, false, "systemctl", "disable", "--now", daemonUnit); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	if err := systemd(io, false, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	_, err = fmt.Fprintln(io.stdout, ui.Success("Finux daemon removed"))
	return err
}

// serviceCommand runs the commands of the daemon that need its unit.
func serviceCommand(command string, args []string, io streams) error {
	var follow bool
	define := func(f *flag.FlagSet) {}
	if command == "logs" {
		define = func(f *flag.FlagSet) { f.BoolVar(&follow, "follow", false, "") }
	}
	if _, _, err := parse(command, args, define, 0); err != nil {
		return err
	}
	if command == "install" {
		return installDaemon(io)
	}
	if _, err := installed(); err != nil {
		return err
	}
	switch command {
	case "uninstall":
		return uninstallDaemon(io)
	case "status":
		return systemd(io, true, "systemctl", "status", daemonUnit, "--no-pager")
	case "logs":
		last := "--no-pager"
		if follow {
			last = "--follow"
		}
		return systemd(io, true, "journalctl", "--unit", daemonUnit, "--lines", "50", last)
	}
	if err := systemd(io, false, "systemctl", command, daemonUnit); err != nil {
		return err
	}
	done := map[string]string{"start": "started", "stop": "stopped", "restart": "started again"}[command]
	_, err := fmt.Fprintln(io.stdout, ui.Success("Finux daemon "+done))
	return err
}

// restartDaemon starts the daemon again when it is installed and
// active, so it runs a new binary. The MCP server of the daemon is in
// the same process. A daemon that is stopped stays stopped. It does
// nothing without systemd.
func restartDaemon(io streams) (restarted bool, err error) {
	if _, err := installed(); err != nil {
		return false, nil
	}
	found, err := exec.LookPath("systemctl")
	if err != nil {
		return false, nil
	}
	if exec.Command(found, "--user", "is-active", "--quiet", daemonUnit).Run() != nil {
		return false, nil
	}
	if err := systemd(io, false, "systemctl", "restart", daemonUnit); err != nil {
		return false, err
	}
	_, err = fmt.Fprintln(io.stdout, ui.Success("Finux daemon started again, with its MCP server"))
	return true, err
}
