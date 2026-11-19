package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsHelpAndList(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if got := succeed(t, home, "config", "skills", "--help"); !strings.Contains(got, "finux config skills install") {
		t.Fatalf("help = %q", got)
	}
	// Without a command, skills shows the list.
	for _, args := range [][]string{{"config", "skills"}, {"config", "skills", "list"}} {
		got := succeed(t, home, args...)
		for _, name := range skillNames {
			if !strings.Contains(got, name) {
				t.Errorf("finux %v lacks %s:\n%s", args, name, got)
			}
		}
		if !strings.Contains(got, "Records expenses, incomes and transfers") {
			t.Errorf("finux %v lacks the descriptions:\n%s", args, got)
		}
	}
	for _, args := range [][]string{
		{"config", "skills", "install"}, {"config", "skills", "install", "other"}, {"config", "skills", "remove"}, {"config", "skills", "unknown"},
		{"config", "skills", "install", "claude", "--dir", "x"}, {"config", "skills", "install", "claude", "extra"},
	} {
		if stdout, stderr, status := finux(t, home, args...); status != 2 || stdout != "" || !strings.HasPrefix(stderr, "finux: config skills: ") {
			t.Errorf("finux %v: status %d, stdout %q, stderr %q", args, status, stdout, stderr)
		}
	}
}

func TestSkillsInstall(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	for agent, folder := range map[string]string{
		"claude":   filepath.Join(home, ".claude", "skills"),
		"codex":    filepath.Join(home, ".codex", "skills"),
		"opencode": filepath.Join(home, ".config", "opencode", "skills"),
	} {
		got := succeed(t, home, "config", "skills", "install", agent)
		if strings.Count(got, " installed") != len(skillNames) || !strings.Contains(got, folder) {
			t.Fatalf("install %s output:\n%s", agent, got)
		}
		for _, skill := range skillNames {
			for _, file := range []string{"SKILL.md", "references/tools.md", "references/concepts.md", "references/access.md"} {
				want, err := os.ReadFile(filepath.Join(agentSkills, skill, file))
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(filepath.Join(folder, skill, file))
				if err != nil || string(got) != string(want) {
					t.Errorf("%s: %s/%s is not the file of the repository: %v", agent, skill, file, err)
				}
			}
		}
		// No file of the Go package goes with the skills.
		if entries, _ := os.ReadDir(folder); len(entries) != len(skillNames) {
			t.Errorf("%s has %d entries, want %d", folder, len(entries), len(skillNames))
		}
	}
}

func TestSkillsUpdateAndRemove(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	folder := filepath.Join(t.TempDir(), "project", ".claude", "skills")
	succeed(t, home, "config", "skills", "install", "--dir", folder)

	// An update replaces changed files and removes old ones. It keeps the
	// skills of other tools.
	coach := filepath.Join(folder, "finux-coach")
	stale := filepath.Join(coach, "references", "old.md")
	other := filepath.Join(folder, "other-skill", "SKILL.md")
	for path, text := range map[string]string{filepath.Join(coach, "SKILL.md"): "changed", stale: "old", other: "mine"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A link, for example to the repository, stays as it is.
	linked := filepath.Join(folder, "finux-period-review")
	target := t.TempDir()
	if err := os.RemoveAll(linked); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, linked); err != nil {
		t.Fatal(err)
	}
	got := succeed(t, home, "config", "skills", "install", "--dir", folder)
	if strings.Count(got, " updated") != len(skillNames)-1 || !strings.Contains(got, "finux-period-review is a link: not changed") {
		t.Fatalf("update output:\n%s", got)
	}
	if text, _ := os.ReadFile(filepath.Join(coach, "SKILL.md")); !strings.HasPrefix(string(text), "---\nname: finux-coach\n") {
		t.Errorf("update did not replace SKILL.md: %q", text)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("update kept the old file %s", stale)
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Errorf("update wrote through the link: %v", entries)
	}

	got = succeed(t, home, "config", "skills", "remove", "--dir", folder)
	if strings.Count(got, " removed") != len(skillNames) {
		t.Fatalf("remove output:\n%s", got)
	}
	entries, err := os.ReadDir(folder)
	if err != nil || len(entries) != 1 || entries[0].Name() != "other-skill" {
		t.Fatalf("after remove: %v, %v; want only other-skill", entries, err)
	}
	if text, _ := os.ReadFile(other); string(text) != "mine" {
		t.Errorf("remove changed the skill of another tool")
	}
	if got := succeed(t, home, "config", "skills", "remove", "--dir", folder); !strings.Contains(got, "No Finux skills in") {
		t.Errorf("second remove = %q", got)
	}
}
