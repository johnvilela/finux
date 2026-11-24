package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// versionedBinary builds the CLI with a version, as a release does.
func versionedBinary(t *testing.T, version string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "finux")
	output, err := exec.Command("go", "build", "-ldflags", "-X 'main.version="+version+"'", "-o", binary, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("build finux %s: %v\n%s", version, err, output)
	}
	return binary
}

func TestVersionOrder(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            bool
	}{
		{"v0.10.0", "v0.10.1", true}, {"v0.10.0", "v0.11.0", true}, {"v0.9.9", "v0.10.0", true}, {"v0.10.0", "v1.0.0", true},
		{"0.10.0", "v0.10.1", true}, {"v0.10.0-3-gabc123", "v0.10.1", true},
		{"v0.10.0", "v0.10.0", false}, {"v0.10.1", "v0.10.0", false}, {"v1.0.0", "v0.99.99", false}, {"v0.10.0-3-gabc123", "v0.10.0", false},
	} {
		got, err := newer(tc.current, tc.latest)
		if err != nil || got != tc.want {
			t.Errorf("newer(%q, %q) = %v, %v; want %v", tc.current, tc.latest, got, err, tc.want)
		}
	}
	for _, bad := range []string{"dev", "", "v1", "v1.2", "v1.2.x", "1.2.3.4"} {
		if _, err := newer(bad, "v1.0.0"); err == nil {
			t.Errorf("newer accepted the version %q", bad)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	home := filepath.Join(t.TempDir(), "unused")
	// A build without a version, as 'go build' makes it.
	for _, args := range [][]string{{"version"}, {"--version"}} {
		if got := succeed(t, home, args...); got != "finux dev\n" {
			t.Fatalf("finux %v = %q", args, got)
		}
	}
	fail(t, home, 2, "unsupported arguments: version extra", "version", "extra")
	fail(t, home, 1, "this build has no version", "update")
	binary := versionedBinary(t, "v0.10.0")
	if stdout, stderr, status := runBinary(t, binary, nil, "version"); status != 0 || stderr != "" || stdout != "finux v0.10.0\n" {
		t.Fatalf("version: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if stdout, _, _ := runBinary(t, binary, nil); !strings.Contains(stdout, "\n  update            Update Finux to the newest version\n  version           Show the version\n") {
		t.Fatalf("help lacks update and version:\n%s", stdout)
	}
}

// updateServer serves the newest release and the install script, as
// GitHub does. The script writes what it got to the file log.
func updateServer(t *testing.T, latest, log string) *httptest.Server {
	t.Helper()
	script := "#!/bin/sh\necho \"version=$FINUX_VERSION dir=$FINUX_INSTALL_DIR\" > '" + log + "'\necho installed by the script\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case latest == "":
			http.NotFound(w, r)
		case r.URL.Path == "/latest":
			if r.Header.Get("User-Agent") == "" {
				t.Error("request without a User-Agent")
			}
			w.Write([]byte(`{"tag_name": "` + latest + `", "name": "Release"}`))
		case r.URL.Path == "/"+latest+"/install.sh":
			w.Write([]byte(script))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestUpdateCommand(t *testing.T) {
	binary := versionedBinary(t, "v0.10.0")
	log := filepath.Join(t.TempDir(), "script.log")
	run := func(latest string, args ...string) (stdout, stderr string, status int) {
		t.Helper()
		server := updateServer(t, latest, log)
		return runBinary(t, binary, []string{"FINUX_UPDATE_SOURCE=" + server.URL, "PATH=" + os.Getenv("PATH")}, append([]string{"update"}, args...)...)
	}
	ran := func() bool {
		_, err := os.Stat(log)
		return err == nil
	}

	for _, latest := range []string{"v0.10.0", "v0.9.0"} {
		if stdout, stderr, status := run(latest); status != 0 || stderr != "" || stdout != "✓ Finux v0.10.0 is up to date\n" {
			t.Fatalf("update with %s: status %d, stdout %q, stderr %q", latest, status, stdout, stderr)
		}
	}
	// --check tells and changes nothing.
	want := "Finux v0.11.0 is available. You have v0.10.0.\nRun 'finux update' to install it.\n"
	if stdout, stderr, status := run("v0.11.0", "--check"); status != 0 || stderr != "" || stdout != want {
		t.Fatalf("update --check: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if stdout, _, status := run("v0.10.0", "--check"); status != 0 || stdout != "✓ Finux v0.10.0 is up to date\n" {
		t.Fatalf("update --check when up to date: status %d, stdout %q", status, stdout)
	}
	if ran() {
		t.Fatal("the install script ran without an update")
	}

	// A newer release runs its install script, for this version and in the
	// folder of this binary.
	stdout, stderr, status := run("v0.11.0")
	if status != 0 || stderr != "" || !strings.HasPrefix(stdout, "Finux v0.11.0 is available. You have v0.10.0.\n") || !strings.Contains(stdout, "installed by the script\n") {
		t.Fatalf("update: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	got, err := os.ReadFile(log)
	if err != nil || strings.TrimSpace(string(got)) != "version=v0.11.0 dir="+filepath.Dir(binary) {
		t.Fatalf("script got %q, %v", got, err)
	}

	// The install script of the release refreshes the daemon and the
	// skills, with the new binary. The update itself calls no systemd.
	home := t.TempDir()
	path, calls := fakeSystemd(t)
	server := updateServer(t, "v0.11.0", log)
	if _, stderr, status := runBinary(t, binary, []string{"FINUX_UPDATE_SOURCE=" + server.URL, "PATH=" + path, "HOME=" + home}, "update"); status != 0 || stderr != "" || calls() != "" {
		t.Fatalf("update without the daemon: status %d, stderr %q", status, stderr)
	}
	unit := filepath.Join(home, ".config", "systemd", "user", "finux.service")
	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unit, []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, status := runBinary(t, binary, []string{"FINUX_UPDATE_SOURCE=" + server.URL, "PATH=" + path, "HOME=" + home}, "update"); status != 0 || stderr != "" {
		t.Fatalf("update with the daemon: status %d, stderr %q", status, stderr)
	}
	if got := calls(); got != "" {
		t.Fatalf("calls of update = %q", got)
	}

	if _, stderr, status := run(""); status != 1 || !strings.Contains(stderr, "finux: update: no release found") {
		t.Fatalf("update without a release: status %d, stderr %q", status, stderr)
	}
	if _, stderr, status := run("nonsense"); status != 1 || !strings.Contains(stderr, `invalid version "nonsense"`) {
		t.Fatalf("update with a bad release: status %d, stderr %q", status, stderr)
	}
	if _, stderr, status := runBinary(t, binary, []string{"FINUX_UPDATE_SOURCE=http://127.0.0.1:1"}, "update"); status != 1 || !strings.Contains(stderr, "finux: update: cannot reach GitHub") {
		t.Fatalf("update without a network: status %d, stderr %q", status, stderr)
	}
	if _, stderr, status := run("v0.11.0", "extra"); status != 2 || !strings.Contains(stderr, "unexpected argument: extra") {
		t.Fatalf("update extra: status %d, stderr %q", status, stderr)
	}
	if stdout, _, status := run("v0.11.0", "--help"); status != 0 || !strings.Contains(stdout, "Usage: finux update") || !strings.Contains(stdout, "--check") {
		t.Fatalf("update --help: status %d, stdout %q", status, stdout)
	}

	// A development build does not update.
	dev := devBinary(t, filepath.Join(t.TempDir(), "finux-dev.db"))
	if _, stderr, status := runBinary(t, dev, nil, "update"); status != 1 || !strings.Contains(stderr, "development build") {
		t.Fatalf("update of a dev build: status %d, stderr %q", status, stderr)
	}
}

// releaseServer serves the files of a release, as GitHub does: a fake
// finux binary and its checksum. The fake writes its arguments to log.
func releaseServer(t *testing.T, log string, goodChecksum bool) (server *httptest.Server, requests *[]string) {
	t.Helper()
	// With FAKE_INIT_FAILS, its init breaks the database and fails.
	binary := []byte("#!/bin/sh\nif [ \"$1\" = version ]; then echo 'finux v0.11.0'; exit 0; fi\necho \"$@\" >> '" + log + "'\n" +
		"if [ -n \"$FAKE_INIT_FAILS\" ]; then echo broken > \"$FINUX_DB\"; echo 'migration failed' >&2; exit 1; fi\n" +
		"if [ \"$1\" = update ] && [ -n \"$FAKE_REFRESH_FAILS\" ]; then echo 'refresh failed' >&2; exit 1; fi\n")
	sum := sha256.Sum256(binary)
	checksum := hex.EncodeToString(sum[:])
	if !goodChecksum {
		checksum = strings.Repeat("0", 64)
	}
	requests = &[]string{}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r.URL.Path)
		switch filepath.Base(r.URL.Path) {
		case "finux-linux-amd64":
			w.Write(binary)
		case "checksums.txt":
			w.Write([]byte(checksum + "  finux-linux-amd64\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, requests
}

func TestInstallScript(t *testing.T) {
	script, err := filepath.Abs("../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	install := func(server *httptest.Server, env ...string) (output string, status int) {
		t.Helper()
		command := exec.Command("sh", script)
		command.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "FINUX_RELEASES_URL=" + server.URL}, env...)
		out, err := command.CombinedOutput()
		if exit, ok := err.(*exec.ExitError); ok {
			status = exit.ExitCode()
		} else if err != nil {
			t.Fatalf("run install.sh: %v", err)
		}
		return string(out), status
	}

	// Without a version, the script takes the newest release.
	log := filepath.Join(t.TempDir(), "finux.log")
	server, requests := releaseServer(t, log, true)
	target := filepath.Join(t.TempDir(), "bin")
	output, status := install(server, "FINUX_INSTALL_DIR="+target)
	if status != 0 || !strings.Contains(output, "finux v0.11.0 installed in "+target) {
		t.Fatalf("install: status %d\n%s", status, output)
	}
	info, err := os.Stat(filepath.Join(target, "finux"))
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed binary: %v, %v", info, err)
	}
	if got := strings.Join(*requests, " "); got != "/latest/download/finux-linux-amd64 /latest/download/checksums.txt" {
		t.Fatalf("requests = %q", got)
	}
	// It sets up the database: init creates it or applies the migrations.
	// Then the new binary refreshes the skills and the daemon.
	if got, _ := os.ReadFile(log); string(got) != "init\nupdate --refresh\n" {
		t.Fatalf("the new binary ran %q, want init and update --refresh", got)
	}
	// A refresh that fails does not fail the install.
	server, _ = releaseServer(t, log, true)
	output, status = install(server, "FINUX_INSTALL_DIR="+target, "FAKE_REFRESH_FAILS=1")
	if status != 0 || !strings.Contains(output, "refresh failed") || !strings.Contains(output, "Run 'finux update --refresh'") || !strings.Contains(output, "installed in "+target) {
		t.Fatalf("install with a failed refresh: status %d\n%s", status, output)
	}
	if !strings.Contains(output, "is not in your PATH") {
		t.Errorf("no advice about the PATH:\n%s", output)
	}
	if files, _ := os.ReadDir(target); len(files) != 1 {
		t.Errorf("the script left files in %s: %v", target, files)
	}

	// With a version it takes that release. It replaces a binary that exists.
	server, requests = releaseServer(t, log, true)
	output, status = install(server, "FINUX_INSTALL_DIR="+target, "FINUX_VERSION=v0.11.0", "PATH="+target+":"+os.Getenv("PATH"))
	if status != 0 || strings.Contains(output, "is not in your PATH") {
		t.Fatalf("install of a version: status %d\n%s", status, output)
	}
	if got := strings.Join(*requests, " "); got != "/download/v0.11.0/finux-linux-amd64 /download/v0.11.0/checksums.txt" {
		t.Fatalf("requests = %q", got)
	}

	// A database that exists gets a copy before the migrations run.
	server, _ = releaseServer(t, log, true)
	database := filepath.Join(t.TempDir(), "data", "finux.db")
	if err := os.MkdirAll(filepath.Dir(database), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(database, []byte("my data"), 0o600); err != nil {
		t.Fatal(err)
	}
	backups := func() []string {
		found, _ := filepath.Glob(database + ".backup-*")
		return found
	}
	output, status = install(server, "FINUX_INSTALL_DIR="+target, "FINUX_DB="+database)
	copies := backups()
	if status != 0 || len(copies) != 1 || !strings.Contains(output, "Copy of the database: "+copies[0]) {
		t.Fatalf("install with a database: status %d, copies %v\n%s", status, copies, output)
	}
	if got, _ := os.ReadFile(copies[0]); string(got) != "my data" {
		t.Fatalf("copy of the database = %q", got)
	}
	if info, _ := os.Stat(copies[0]); info.Mode().Perm() != 0o600 {
		t.Errorf("copy of the database has the mode %v", info.Mode().Perm())
	}
	// When the migrations fail, the database comes back and the binary
	// that was there stays.
	before, _ := os.ReadFile(filepath.Join(target, "finux"))
	if err := os.WriteFile(filepath.Join(target, "finux"), append(before, []byte("# old\n")...), 0o755); err != nil {
		t.Fatal(err)
	}
	server, _ = releaseServer(t, log, true)
	output, status = install(server, "FINUX_INSTALL_DIR="+target, "FINUX_DB="+database, "FAKE_INIT_FAILS=1")
	if status == 0 || !strings.Contains(output, "migration failed") || !strings.Contains(output, "The database is as it was") {
		t.Fatalf("install with a failed init: status %d\n%s", status, output)
	}
	if got, _ := os.ReadFile(database); string(got) != "my data" {
		t.Fatalf("database after a failed init = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(target, "finux")); !strings.HasSuffix(string(got), "# old\n") {
		t.Fatal("a failed init replaced the binary")
	}
	if files, _ := os.ReadDir(target); len(files) != 1 {
		t.Errorf("a failed init left files in %s: %v", target, files)
	}

	// The default folder is ~/.local/bin.
	server, _ = releaseServer(t, log, true)
	home := t.TempDir()
	if output, status = install(server, "HOME="+home); status != 0 {
		t.Fatalf("install in the default folder: status %d\n%s", status, output)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "finux")); err != nil {
		t.Fatalf("no binary in ~/.local/bin: %v", err)
	}

	// A file with a wrong checksum is not installed.
	server, _ = releaseServer(t, log, false)
	empty := filepath.Join(t.TempDir(), "bin")
	output, status = install(server, "FINUX_INSTALL_DIR="+empty)
	if status == 0 || !strings.Contains(output, "checksum") {
		t.Fatalf("install with a bad checksum: status %d\n%s", status, output)
	}
	if files, _ := os.ReadDir(empty); len(files) != 0 {
		t.Fatalf("a bad download left files: %v", files)
	}
	// A release that does not exist is an error.
	missing := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(missing.Close)
	if output, status = install(missing, "FINUX_INSTALL_DIR="+empty); status == 0 || !strings.Contains(output, "download") {
		t.Fatalf("install without a release: status %d\n%s", status, output)
	}
	// Only the systems that have a release can install.
	if output, status = install(server, "FINUX_INSTALL_DIR="+empty, "FINUX_SYSTEM=Darwin arm64"); status == 0 || !strings.Contains(output, "no finux release for Darwin arm64") {
		t.Fatalf("install on another system: status %d\n%s", status, output)
	}
}

// fakeProcesses makes a folder as /proc with one process for each
// command line. It returns the folder; the processes have the ids 101,
// 102 and so on.
func fakeProcesses(t *testing.T, commands ...[]string) string {
	t.Helper()
	folder := t.TempDir()
	for i, command := range commands {
		process := filepath.Join(folder, strconv.Itoa(101+i))
		if err := os.MkdirAll(process, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(process, "cmdline"), []byte(strings.Join(command, "\x00")+"\x00"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// /proc has also entries that are not processes.
	if err := os.WriteFile(filepath.Join(folder, "uptime"), []byte("1 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return folder
}

func TestUpdateRefresh(t *testing.T) {
	home := t.TempDir()
	path, calls := fakeSystemd(t)
	processes := fakeProcesses(t)
	refresh := func(args ...string) (stdout, stderr string, status int) {
		t.Helper()
		env := []string{"HOME=" + home, "PATH=" + path, "FINUX_PROC=" + processes}
		return finuxEnv(t, env, append([]string{"update", "--refresh"}, args...)...)
	}

	// Nothing is installed: the refresh does and tells nothing.
	if stdout, stderr, status := refresh(); status != 0 || stdout != "" || stderr != "" || calls() != "" {
		t.Fatalf("refresh of nothing: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if _, stderr, status := refresh("--check"); status != 2 || !strings.Contains(stderr, "--refresh") {
		t.Fatalf("refresh --check: status %d, stderr %q", status, stderr)
	}
	if stdout, _, _ := finuxEnv(t, nil, "update", "--help"); !strings.Contains(stdout, "--refresh") {
		t.Fatalf("help lacks --refresh:\n%s", stdout)
	}

	// The skills of an agent that has them get the files of this binary.
	// An agent without the skills gets none.
	if _, stderr, status := finuxEnv(t, []string{"HOME=" + home}, "config", "skills", "install", "claude"); status != 0 {
		t.Fatalf("install the skills: %s", stderr)
	}
	claude := filepath.Join(home, ".claude", "skills")
	changed := filepath.Join(claude, skillNames[0], "SKILL.md")
	want, err := os.ReadFile(changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, []byte("old text"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claude, skillNames[0], "old-file.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(claude, "other-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A skill that is a link stays a link.
	elsewhere := t.TempDir()
	if err := os.RemoveAll(filepath.Join(claude, skillNames[1])); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(claude, skillNames[1])); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, status := refresh()
	if status != 0 || stderr != "" || calls() != "" {
		t.Fatalf("refresh of the skills: status %d, stderr %q, stdout %q", status, stderr, stdout)
	}
	for _, text := range []string{"✓ Skills updated for claude (" + strconv.Itoa(len(skillNames)-1) + " skills)\n", "Start a new session of your AI agents"} {
		if !strings.Contains(stdout, text) {
			t.Errorf("refresh output lacks %q:\n%s", text, stdout)
		}
	}
	if strings.Contains(stdout, "codex") || strings.Contains(stdout, "opencode") || strings.Contains(stdout, "MCP server") {
		t.Errorf("refresh output tells too much:\n%s", stdout)
	}
	if got, _ := os.ReadFile(changed); string(got) != string(want) {
		t.Errorf("the refresh did not write the skill again: %q", got)
	}
	if _, err := os.Stat(filepath.Join(claude, skillNames[0], "old-file.md")); err == nil {
		t.Error("the refresh kept an old file of a skill")
	}
	if _, err := os.Stat(filepath.Join(claude, "other-skill")); err != nil {
		t.Error("the refresh removed a skill that is not of Finux")
	}
	if info, err := os.Lstat(filepath.Join(claude, skillNames[1])); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("the refresh replaced a link")
	}
	if files, _ := os.ReadDir(elsewhere); len(files) != 0 {
		t.Errorf("the refresh wrote through a link: %v", files)
	}
	for _, folder := range []string{filepath.Join(home, ".codex"), filepath.Join(home, ".config", "opencode")} {
		if _, err := os.Stat(folder); err == nil {
			t.Errorf("the refresh made %s", folder)
		}
	}

	// A daemon that runs starts again. One that is stopped stays stopped.
	unit := filepath.Join(home, ".config", "systemd", "user", "finux.service")
	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unit, []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, status = refresh()
	if status != 0 || stderr != "" || !strings.Contains(stdout, "✓ Finux daemon started again, with its MCP server\n") {
		t.Fatalf("refresh with the daemon: status %d, stderr %q, stdout %q", status, stderr, stdout)
	}
	if got := calls(); got != "systemctl --user is-active --quiet finux.service\nsystemctl --user restart finux.service\n" {
		t.Fatalf("calls of refresh = %q", got)
	}
	stopped := t.TempDir()
	script := "#!/bin/sh\necho \"systemctl $*\" >> '" + filepath.Join(stopped, "calls.log") + "'\n[ \"$2\" != is-active ]\n"
	if err := os.WriteFile(filepath.Join(stopped, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, status = finuxEnv(t, []string{"HOME=" + home, "PATH=" + stopped + ":" + os.Getenv("PATH"), "FINUX_PROC=" + processes}, "update", "--refresh")
	if status != 0 || stderr != "" || strings.Contains(stdout, "daemon") {
		t.Fatalf("refresh with a stopped daemon: status %d, stderr %q, stdout %q", status, stderr, stdout)
	}
	if got, _ := os.ReadFile(filepath.Join(stopped, "calls.log")); string(got) != "systemctl --user is-active --quiet finux.service\n" {
		t.Fatalf("calls with a stopped daemon = %q", got)
	}
	// A restart that fails is an error, after the other steps.
	broken := t.TempDir()
	script = "#!/bin/sh\nif [ \"$2\" = restart ]; then echo 'no bus' >&2; exit 1; fi\n"
	if err := os.WriteFile(filepath.Join(broken, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, status = finuxEnv(t, []string{"HOME=" + home, "PATH=" + broken + ":" + os.Getenv("PATH"), "FINUX_PROC=" + processes}, "update", "--refresh")
	if status != 1 || !strings.Contains(stderr, "no bus") || !strings.Contains(stdout, "Skills updated for claude") {
		t.Fatalf("refresh with a failed restart: status %d, stderr %q, stdout %q", status, stderr, stdout)
	}
	calls()

	// The MCP servers that run keep the old version: the refresh tells.
	processes = fakeProcesses(t,
		[]string{"/home/me/.local/bin/finux", "config", "mcp"},
		[]string{"finux", "config", "mcp", "--read-only"},
		[]string{"/usr/bin/finux", "config", "mcp", "--http", "127.0.0.1:8765"},
		[]string{"/usr/bin/finux", "config", "mcp", "config", "claude"},
		[]string{"/usr/bin/finux", "daemon", "run"},
		[]string{"/usr/bin/finux-dev", "config", "mcp"},
		[]string{"vim", "config", "mcp"},
	)
	stdout, stderr, status = refresh()
	if status != 0 || stderr != "" {
		t.Fatalf("refresh with MCP servers: status %d, stderr %q", status, stderr)
	}
	for _, text := range []string{"2 agent sessions still run the old MCP server.", "Start a new session of your AI agents", "A 'finux config mcp --http' server still runs the old version (PID 103). Start it again."} {
		if !strings.Contains(stdout, text) {
			t.Errorf("refresh output lacks %q:\n%s", text, stdout)
		}
	}
	// One session, and no skills: the refresh still tells.
	lone := t.TempDir()
	processes = fakeProcesses(t, []string{"finux", "config", "mcp"})
	stdout, _, status = finuxEnv(t, []string{"HOME=" + lone, "PATH=" + path, "FINUX_PROC=" + processes}, "update", "--refresh")
	if status != 0 || !strings.Contains(stdout, "1 agent session still runs the old MCP server.") || !strings.Contains(stdout, "Start a new session") {
		t.Fatalf("refresh with one MCP server: status %d, stdout %q", status, stdout)
	}
}
