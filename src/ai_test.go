package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// aiHome is a home directory with fake 'claude' and 'codex' commands in
// the PATH. Each one writes its arguments to the file log.
type aiHome struct {
	t              *testing.T
	home, bin, log string
}

func newAIHome(t *testing.T) aiHome {
	t.Helper()
	root := t.TempDir()
	h := aiHome{t: t, home: filepath.Join(root, "home"), bin: filepath.Join(root, "bin"), log: filepath.Join(root, "agents.log")}
	for _, folder := range []string{h.home, h.bin} {
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	h.agent("claude", "")
	h.agent("codex", "")
	return h
}

// agent writes a fake command of an agent. With a failure text, its
// 'mcp add' prints the text and fails.
func (h aiHome) agent(name, failure string) {
	h.t.Helper()
	script := "#!/bin/sh\necho \"" + name + " $*\" >> '" + h.log + "'\n"
	if failure != "" {
		script += "if [ \"$2\" = add ]; then echo '" + failure + "' >&2; exit 1; fi\n"
	}
	if err := os.WriteFile(filepath.Join(h.bin, name), []byte(script), 0o755); err != nil {
		h.t.Fatal(err)
	}
}

func (h aiHome) run(args ...string) (stdout, stderr string, status int) {
	h.t.Helper()
	return finuxEnv(h.t, []string{"HOME=" + h.home, "PATH=" + h.bin + ":/usr/bin:/bin"}, args...)
}

func (h aiHome) calls() string {
	content, _ := os.ReadFile(h.log)
	os.Remove(h.log)
	return string(content)
}

// skills returns the number of Finux skills in a skills folder.
func (h aiHome) skills(folder ...string) int {
	found, _ := filepath.Glob(filepath.Join(append(append([]string{h.home}, folder...), "finux-*", "SKILL.md")...))
	return len(found)
}

func TestConfigAI(t *testing.T) {
	h := newAIHome(t)
	binary := finuxBinary(t)

	stdout, stderr, status := h.run("config", "ai", "claude")
	want := "✓ MCP server added to Claude Code\n✓ 5 skills installed in " + filepath.Join(h.home, ".claude", "skills") + "\nStart a new session of Claude Code to use Finux.\n"
	if status != 0 || stderr != "" || stdout != want {
		t.Fatalf("config ai claude: status %d, stderr %q, stdout %q", status, stderr, stdout)
	}
	// It replaces a server that is there, so a second run works.
	if got, want := h.calls(), "claude mcp remove --scope user finux\nclaude mcp add --scope user finux -- "+binary+" config mcp\n"; got != want {
		t.Fatalf("claude calls = %q, want %q", got, want)
	}
	if got := h.skills(".claude", "skills"); got != 5 {
		t.Fatalf("%d skills for claude, want 5", got)
	}
	if stdout, _, status = h.run("config", "ai", "claude", "--read-only"); status != 0 || !strings.Contains(stdout, "✓ MCP server added to Claude Code (read-only)\n") {
		t.Fatalf("config ai claude --read-only: status %d, stdout %q", status, stdout)
	}
	if got := h.calls(); !strings.HasSuffix(got, " config mcp --read-only\n") {
		t.Fatalf("read-only call = %q", got)
	}

	if stdout, stderr, status = h.run("config", "ai", "codex"); status != 0 || stderr != "" || !strings.HasPrefix(stdout, "✓ MCP server added to Codex\n✓ 5 skills installed in ") {
		t.Fatalf("config ai codex: status %d, stderr %q, stdout %q", status, stderr, stdout)
	}
	if got, want := h.calls(), "codex mcp remove finux\ncodex mcp add finux -- "+binary+" config mcp\n"; got != want {
		t.Fatalf("codex calls = %q, want %q", got, want)
	}
	if got := h.skills(".codex", "skills"); got != 5 {
		t.Fatalf("%d skills for codex, want 5", got)
	}

	// A command of the agent that fails stops the setup, with its message.
	h.agent("claude", "No permission to write the settings")
	if _, stderr, status = h.run("config", "ai", "claude"); status != 1 || !strings.Contains(stderr, "No permission to write the settings") || !strings.Contains(stderr, "finux config mcp config claude") {
		t.Fatalf("config ai with a failed agent: status %d, stderr %q", status, stderr)
	}
	// An agent that is not installed.
	os.Remove(filepath.Join(h.bin, "codex"))
	if _, stderr, status = h.run("config", "ai", "codex"); status != 1 || !strings.Contains(stderr, "the command 'codex' is not in your PATH") {
		t.Fatalf("config ai without the agent: status %d, stderr %q", status, stderr)
	}

	for want, args := range map[string][]string{
		"config ai: give the agent: claude, codex, opencode": {"config", "ai"},
		"config ai: unknown agent \"gemini\"":                {"config", "ai", "gemini"},
		"config ai: unexpected argument: extra":              {"config", "ai", "claude", "extra"},
	} {
		if stdout, stderr, status = h.run(args...); status != 2 || stdout != "" || !strings.Contains(stderr, want) {
			t.Errorf("%v: status %d, stdout %q, stderr %q; want %q", args, status, stdout, stderr, want)
		}
	}
	help, _, _ := h.run("config", "ai", "--help")
	for _, want := range []string{"Usage: finux config ai [agent]", "claude", "codex", "opencode", "--read-only"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q:\n%s", want, help)
		}
	}
	if general, _, _ := h.run("config", "--help"); !strings.Contains(general, "ai [agent]") {
		t.Errorf("config help lacks ai:\n%s", general)
	}
}

func TestConfigAIOpencode(t *testing.T) {
	h := newAIHome(t)
	binary := finuxBinary(t)
	file := filepath.Join(h.home, ".config", "opencode", "opencode.json")
	read := func() map[string]any {
		t.Helper()
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var config map[string]any
		if err := json.Unmarshal(content, &config); err != nil {
			t.Fatalf("opencode.json is not JSON: %v\n%s", err, content)
		}
		return config
	}
	server := func(config map[string]any) string {
		encoded, _ := json.Marshal(config["mcp"].(map[string]any)["finux"])
		return string(encoded)
	}

	// Without a file, it makes one.
	stdout, stderr, status := h.run("config", "ai", "opencode")
	if status != 0 || stderr != "" || !strings.HasPrefix(stdout, "✓ MCP server added to opencode\n✓ 5 skills installed in ") {
		t.Fatalf("config ai opencode: status %d, stderr %q, stdout %q", status, stderr, stdout)
	}
	config := read()
	if got, want := server(config), `{"command":["`+binary+`","config","mcp"],"enabled":true,"type":"local"}`; got != want {
		t.Fatalf("server = %s, want %s", got, want)
	}
	if config["$schema"] != "https://opencode.ai/config.json" {
		t.Fatalf("new file has no schema: %v", config)
	}
	if got := h.skills(".config", "opencode", "skills"); got != 5 {
		t.Fatalf("%d skills for opencode, want 5", got)
	}

	// A file that exists keeps its other values, and gets a copy first.
	before := `{"theme": "dark", "mcp": {"other": {"type": "remote", "url": "https://example.com"}, "finux": {"command": ["old"]}}}`
	if err := os.WriteFile(file, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, status = h.run("config", "ai", "opencode", "--read-only"); status != 0 || stderr != "" {
		t.Fatalf("config ai opencode again: status %d, stderr %q", status, stderr)
	}
	config = read()
	if config["theme"] != "dark" || config["mcp"].(map[string]any)["other"] == nil || !strings.Contains(server(config), `"config","mcp","--read-only"`) {
		t.Fatalf("config after the change = %v", config)
	}
	if copied, _ := os.ReadFile(file + ".backup"); string(copied) != before {
		t.Fatalf("copy of the file = %q", copied)
	}
	if info, _ := os.Stat(file); info.Mode().Perm() != 0o600 {
		t.Errorf("the file has the mode %v", info.Mode().Perm())
	}

	// A file that Finux cannot read as JSON stays as it is.
	comments := "{\n  // my servers\n  \"mcp\": {}\n}\n"
	if err := os.WriteFile(file, []byte(comments), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, status = h.run("config", "ai", "opencode"); status != 1 || !strings.Contains(stderr, "finux config mcp config opencode") {
		t.Fatalf("config ai with comments: status %d, stderr %q", status, stderr)
	}
	if content, _ := os.ReadFile(file); string(content) != comments {
		t.Fatalf("a file with comments changed: %q", content)
	}
}
