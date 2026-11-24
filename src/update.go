package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"finux/src/storage"
	"finux/src/ui"
)

// version is the version of this build, such as v0.10.0. A release sets
// it at build time; a build without it is "dev".
var version = "dev"

const (
	// repository is the GitHub repository that has the releases.
	repository = "johnvilela/finux"
	// updateTimeout is the longest time a request to GitHub can take.
	updateTimeout = 30 * time.Second
)

func updateHelp() string {
	return "Update Finux to the newest version\n\n" +
		green.Render("Usage:") + ` finux update [--check]
       finux update --refresh

` + green.Render("Options:") + `
  --check           Tell if a newer version exists; change nothing
  --refresh         Do only the steps after an install, with this version
  -h, --help        Show this help

Finux asks GitHub for the newest release of ` + repository + `. When it
is newer than this version, Finux gets the install script of that release
and runs it. The script downloads the new binary and checks it. It makes a
copy of your database beside it (finux.db.backup-<date>-<time>), then the new
version applies its database migrations. When that works, the script puts
the new binary in the place of this one. When it fails, the script puts the
database back and this version stays.

After the install, the new version refreshes what Finux installed:
- It applies its database migrations, with a copy of the database first,
  when the database is of an older version. After the install script this
  step finds nothing to do.
- It writes the skills again for each agent that has them (claude, codex,
  opencode). A skills folder you gave with --dir needs
  'finux config skills install --dir' again.
- It starts the daemon again when it runs. The MCP server of the daemon
  starts again with it.
- It tells you when an AI agent or a 'finux config mcp --http' server still
  runs the old MCP server. An agent gets the new one in a new session.
'finux update --refresh' does these steps again, without a new version.

'finux version' shows the version you have.
`
}

func runVersion(args []string, io streams) int {
	if len(args) > 0 {
		fmt.Fprintf(io.stderr, "finux: unsupported arguments: version %s\nRun 'finux --help' for usage.\n", strings.Join(args, " "))
		return 2
	}
	return write(io, "finux "+version+"\n")
}

// parseVersion reads a version such as v0.10.0 or 0.10.0. Text after a
// dash, such as the commit of a build between two releases, does not count.
func parseVersion(text string) ([3]int, error) {
	var numbers [3]int
	core, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(text), "v"), "-")
	parts := strings.Split(core, ".")
	if len(parts) != len(numbers) {
		return numbers, fmt.Errorf("invalid version %q", text)
	}
	for i, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return numbers, fmt.Errorf("invalid version %q", text)
		}
		numbers[i] = number
	}
	return numbers, nil
}

// newer reports whether the version latest is after the version current.
func newer(current, latest string) (bool, error) {
	have, err := parseVersion(current)
	if err != nil {
		return false, err
	}
	found, err := parseVersion(latest)
	if err != nil {
		return false, err
	}
	for i := range have {
		if found[i] != have[i] {
			return found[i] > have[i], nil
		}
	}
	return false, nil
}

// updateSource tells where the newest release and its install script
// are. The FINUX_UPDATE_SOURCE environment variable gives another
// place, for tests.
func updateSource() (latest string, script func(tag string) string) {
	if base := strings.TrimRight(os.Getenv("FINUX_UPDATE_SOURCE"), "/"); base != "" {
		return base + "/latest", func(tag string) string { return base + "/" + tag + "/install.sh" }
	}
	return "https://api.github.com/repos/" + repository + "/releases/latest", func(tag string) string {
		return "https://raw.githubusercontent.com/" + repository + "/" + tag + "/install.sh"
	}
}

// errNoRelease means GitHub has no release, or the repository is not public.
var errNoRelease = errors.New("no release found for " + repository)

// fetch reads a URL of GitHub.
func fetch(url string) ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "finux")
	response, err := (&http.Client{Timeout: updateTimeout}).Do(request)
	if err != nil {
		return nil, fmt.Errorf("cannot reach GitHub: %w", err)
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusNotFound:
		return nil, errNoRelease
	case response.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GitHub answered %s", response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 1<<20))
}

// latestRelease asks GitHub for the version of the newest release.
func latestRelease(url string) (string, error) {
	body, err := fetch(url)
	if err != nil {
		return "", err
	}
	var release struct {
		Tag string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("read the answer of GitHub: %w", err)
	}
	return release.Tag, nil
}

func runUpdate(args []string, io streams) int {
	for _, arg := range args {
		if isHelp(arg) {
			return write(io, updateHelp())
		}
	}
	var check, again bool
	_, _, err := parse("update", args, func(f *flag.FlagSet) {
		f.BoolVar(&check, "check", false, "")
		f.BoolVar(&again, "refresh", false, "")
	}, 0)
	if err == nil && check && again {
		err = usageError("update: give --check or --refresh, not the two")
	}
	if err != nil {
		return exitStatus(io, "update", err)
	}
	run := func() error { return update(check, io) }
	if again {
		run = func() error { return refresh(io) }
	}
	if err := run(); err != nil {
		fmt.Fprintf(io.stderr, "finux: update: %v\n", err)
		return 1
	}
	return 0
}

// update compares this version with the newest release and installs the
// release when it is newer.
func update(check bool, io streams) error {
	if devDatabase != "" {
		return errors.New("a development build does not update: get the new code and run 'make dev'")
	}
	if _, err := parseVersion(version); err != nil {
		return fmt.Errorf("this build has no version (%s), so Finux cannot compare it. Install a release: see the README", version)
	}
	latestURL, scriptURL := updateSource()
	latest, err := latestRelease(latestURL)
	if err != nil {
		return err
	}
	available, err := newer(version, latest)
	if err != nil {
		return err
	}
	if !available {
		fmt.Fprintln(io.stdout, ui.Success("Finux "+version+" is up to date"))
		return nil
	}
	fmt.Fprintf(io.stdout, "Finux %s is available. You have %s.\n", latest, version)
	if check {
		fmt.Fprintln(io.stdout, "Run 'finux update' to install it.")
		return nil
	}
	script, err := fetch(scriptURL(latest))
	if err != nil {
		return fmt.Errorf("get the install script: %w", err)
	}
	binary, err := os.Executable()
	if err == nil {
		binary, err = filepath.EvalSymlinks(binary)
	}
	if err != nil {
		return fmt.Errorf("find the finux binary: %w", err)
	}
	// The script copies the database, applies the migrations with the new
	// binary and then puts that binary in the place of this one. Then it
	// runs 'update --refresh' of the new binary: this process is the old
	// version, with the old skills.
	command := exec.Command("sh", "-s")
	command.Stdin, command.Stdout, command.Stderr = bytes.NewReader(script), io.stdout, io.stderr
	command.Env = append(os.Environ(), "FINUX_VERSION="+latest, "FINUX_INSTALL_DIR="+filepath.Dir(binary))
	if err := command.Run(); err != nil {
		return fmt.Errorf("the install script failed: %w", err)
	}
	return nil
}

// migrateDatabase applies the migrations of this version to a database
// of an older one. It keeps a copy of the database first, beside it, and
// puts the database back when the migrations fail. It does nothing
// without a database, or with one that is up to date.
func migrateDatabase(io streams) error {
	path, err := databasePath()
	if err != nil {
		return err
	}
	db, err := storage.Open(path)
	if errors.Is(err, storage.ErrNotInitialized) {
		return nil
	} else if err != nil {
		return err
	}
	pending, err := storage.Pending(db, models...)
	closeDatabase(db)
	if err != nil || len(pending) == 0 {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("copy the database: %w", err)
	}
	backup := path + ".backup-" + time.Now().Format("20060102-150405")
	if err := os.WriteFile(backup, content, info.Mode().Perm()); err != nil {
		return fmt.Errorf("copy the database to %s: %w", backup, err)
	}
	if _, err := setupDatabase(path); err != nil {
		if os.Rename(backup, path) != nil {
			return fmt.Errorf("%w. The copy of the database from before is %s", err, backup)
		}
		return fmt.Errorf("%w. The database is as it was", err)
	}
	_, err = fmt.Fprint(io.stdout, ui.Success("Database updated: "+path)+"\n"+
		ui.Hint("Copy of the database: "+backup)+"\n"+ui.Hint("Delete it when the new version works for you.")+"\n")
	return err
}

// refresh makes what Finux installed use this version: the database,
// the skills of the agents and the daemon. It tells about the MCP
// servers that it cannot start again. A step that fails does not stop
// the next ones.
func refresh(io streams) error {
	databaseErr := migrateDatabase(io)
	changed, skillsErr := refreshSkills(io)
	_, daemonErr := restartDaemon(io)
	stdio, served := mcpProcesses()
	var lines []string
	switch len(stdio) {
	case 0:
	case 1:
		lines = append(lines, "1 agent session still runs the old MCP server.")
	default:
		lines = append(lines, fmt.Sprintf("%d agent sessions still run the old MCP server.", len(stdio)))
	}
	if changed || len(stdio) > 0 {
		lines = append(lines, "Start a new session of your AI agents: open sessions keep the old skills and tools.")
	}
	for _, id := range served {
		lines = append(lines, fmt.Sprintf("A 'finux config mcp --http' server still runs the old version (PID %d). Start it again.", id))
	}
	for _, line := range lines {
		fmt.Fprintln(io.stdout, ui.Hint(line))
	}
	return errors.Join(databaseErr, skillsErr, daemonErr)
}
