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

` + green.Render("Options:") + `
  --check           Tell if a newer version exists; change nothing
  -h, --help        Show this help

Finux asks GitHub for the newest release of ` + repository + `. When it
is newer than this version, Finux gets the install script of that release
and runs it. The script downloads the new binary and checks it. It makes a
copy of your database beside it (finux.db.backup-<date>-<time>), then the new
version applies its database migrations. When that works, the script puts
the new binary in the place of this one. When it fails, the script puts the
database back and this version stays.

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
	var check bool
	if _, _, err := parse("update", args, func(f *flag.FlagSet) { f.BoolVar(&check, "check", false, "") }, 0); err != nil {
		return exitStatus(io, "update", err)
	}
	if err := update(check, io); err != nil {
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
	// binary and then puts that binary in the place of this one.
	command := exec.Command("sh", "-s")
	command.Stdin, command.Stdout, command.Stderr = bytes.NewReader(script), io.stdout, io.stderr
	command.Env = append(os.Environ(), "FINUX_VERSION="+latest, "FINUX_INSTALL_DIR="+filepath.Dir(binary))
	if err := command.Run(); err != nil {
		return fmt.Errorf("the install script failed: %w", err)
	}
	restartDaemon()
	return nil
}
