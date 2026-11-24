package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"finux/src/mcp"
)

// The skills and the documentation for AI agents are text, but they name
// the MCP tools and repeat reference files. These tests keep them true.
// Run 'make docs' to write the generated files again.

const (
	agentDocs   = "../docs/agents"
	agentSkills = "../skills"
)

// skillNames lists the skills the repository must have.
var skillNames = []string{
	"finux-coach", "finux-credit-card-bills", "finux-import-statement", "finux-log-transactions", "finux-period-review",
	"finux-tidy-transactions",
}

// sharedReferences lists the files of docs/agents that each skill has a
// copy of, so that an installed skill works without the repository.
var sharedReferences = []string{"access.md", "concepts.md", "tools.md"}

// nameSkills lists the skills that give a title and tags to a
// transaction. They also have a copy of names.md: how to learn the
// names and the tags of the user.
var nameSkills = []string{"finux-import-statement", "finux-log-transactions", "finux-tidy-transactions"}

// references lists the files of docs/agents that a skill has a copy of.
func references(skill string) []string {
	if slices.Contains(nameSkills, skill) {
		return append(slices.Clone(sharedReferences), "names.md")
	}
	return sharedReferences
}

func updateDocs() bool { return os.Getenv("FINUX_UPDATE_DOCS") != "" }

// toolReference writes the reference of the MCP tools from their schemas.
func toolReference(tools []mcp.Tool) string {
	var out strings.Builder
	out.WriteString("# Finux MCP tools\n\n" +
		"<!-- Generated from the tool schemas by 'make docs'. Do not edit. -->\n\n" +
		"Each tool returns JSON; see [concepts.md](concepts.md) for the objects in a\n" +
		"result. An agent can show a tool with a prefix, for example\n" +
		"`mcp__finux__account_list`. *Read* tools change nothing. *Delete* tools\n" +
		"need `confirm: true`; without it they return `confirm_required` and a\n" +
		"message, and change nothing.\n")
	group := ""
	for _, tool := range tools {
		prefix := tool.Name[:strings.LastIndex(tool.Name, "_")]
		if strings.HasPrefix(tool.Name, "credit_card_") {
			prefix = "credit_card"
		}
		if prefix != group {
			group = prefix
			title := strings.ToUpper(group[:1]) + strings.ReplaceAll(group[1:], "_", " ")
			fmt.Fprintf(&out, "\n## %s\n", title)
		}
		kind := "Change"
		switch {
		case tool.ReadOnly:
			kind = "Read"
		case tool.Destructive:
			kind = "Delete"
		}
		fmt.Fprintf(&out, "\n### `%s`\n\n*%s.* %s\n", tool.Name, kind, tool.Description)
		properties := tool.InputSchema["properties"].(map[string]any)
		if len(properties) == 0 {
			out.WriteString("\nNo arguments.\n")
			continue
		}
		required := tool.InputSchema["required"].([]string)
		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}
		// Required arguments first, in the order of the schema.
		sort.Slice(names, func(i, j int) bool {
			a, b := slices.Index(required, names[i]), slices.Index(required, names[j])
			switch {
			case a >= 0 && b >= 0:
				return a < b
			case a >= 0 || b >= 0:
				return a >= 0
			}
			return names[i] < names[j]
		})
		out.WriteString("\n| Argument | Type | Required | Description |\n| --- | --- | --- | --- |\n")
		for _, name := range names {
			property := properties[name].(map[string]any)
			description, _ := property["description"].(string)
			if enum, ok := property["enum"].([]string); ok {
				description = strings.TrimSuffix(description, ".") + ". One of: `" + strings.Join(enum, "`, `") + "`"
				description = strings.TrimPrefix(description, ". ")
			}
			need := "no"
			if slices.Contains(required, name) {
				need = "yes"
			}
			fmt.Fprintf(&out, "| `%s` | %s | %s | %s |\n", name, property["type"], need, strings.ReplaceAll(description, "|", "\\|"))
		}
	}
	return out.String()
}

// current fails when a generated file is not what the code makes now.
// With FINUX_UPDATE_DOCS it writes the file.
func current(t *testing.T, path, want string) {
	t.Helper()
	if updateDocs() {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v. Run 'make docs'", err)
	}
	if string(got) != want {
		t.Errorf("%s is out of date. Run 'make docs'", path)
	}
}

func TestAgentToolReferenceIsCurrent(t *testing.T) {
	current(t, filepath.Join(agentDocs, "tools.md"), toolReference(mcpTools(false)))
}

func TestAgentSkillReferencesAreCopies(t *testing.T) {
	for _, skill := range skillNames {
		for _, name := range references(skill) {
			source, err := os.ReadFile(filepath.Join(agentDocs, name))
			if err != nil {
				t.Fatal(err)
			}
			current(t, filepath.Join(agentSkills, skill, "references", name), string(source))
		}
	}
}

var (
	frontmatter = regexp.MustCompile(`(?s)\A---\nname: ([^\n]+)\ndescription: ([^\n]+)\n---\n\n# `)
	toolMention = regexp.MustCompile("`((?:database|account|category|transaction|credit_card|goal|budget|schedule)_[a-z_]+)`")
	linkTarget  = regexp.MustCompile(`\]\(([^)#]+\.md)(?:#[^)]*)?\)`)
)

func TestAgentSkills(t *testing.T) {
	entries, err := os.ReadDir(agentSkills)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, entry := range entries {
		// The folder also has the Go file that puts the skills in the binary.
		if entry.IsDir() {
			found = append(found, entry.Name())
		}
	}
	if !slices.Equal(found, skillNames) {
		t.Fatalf("skills = %v, want %v", found, skillNames)
	}
	for _, skill := range skillNames {
		path := filepath.Join(agentSkills, skill, "SKILL.md")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		match := frontmatter.FindSubmatch(content)
		if match == nil {
			t.Errorf("%s: no frontmatter with name and description, then a title", path)
			continue
		}
		if name := string(match[1]); name != skill {
			t.Errorf("%s: name = %q, want the name of its folder", path, name)
		}
		if size := len(match[2]); size < 80 || size > 1024 {
			t.Errorf("%s: description has %d characters, want 80 to 1024", path, size)
		}
		if lines := strings.Count(string(content), "\n"); lines > 300 {
			t.Errorf("%s: %d lines; move details to references/", path, lines)
		}
		for _, reference := range references(skill) {
			if !strings.Contains(string(content), "(references/"+reference+")") {
				t.Errorf("%s: no link to references/%s", path, reference)
			}
		}
	}
}

// markdownFiles returns the documentation and the skills, without the
// copies of the references.
func markdownFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(agentDocs, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range skillNames {
		files = append(files, filepath.Join(agentSkills, skill, "SKILL.md"))
	}
	return files
}

func TestAgentDocsNameRealTools(t *testing.T) {
	var real []string
	for _, tool := range mcpTools(false) {
		real = append(real, tool.Name)
	}
	mentioned := map[string]bool{}
	for _, path := range markdownFiles(t) {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range toolMention.FindAllSubmatch(content, -1) {
			name := string(match[1])
			mentioned[filepath.Base(path)+":"+name] = true
			if !slices.Contains(real, name) && !resultFields[name] {
				t.Errorf("%s names the tool %s, which does not exist", path, name)
			}
		}
	}
	// The CLI fallback must cover all tools.
	for _, name := range real {
		if !mentioned["access.md:"+name] {
			t.Errorf("access.md has no CLI command for the tool %s", name)
		}
	}
}

// resultFields are names of result fields that look like tool names.
var resultFields = map[string]bool{"transactions_deleted": true, "transactions_moved": true, "transfers_converted": true, "payments_converted": true, "transactions_unlinked": true, "budget_warnings": true}

func TestAgentDocsLinksResolve(t *testing.T) {
	for _, path := range markdownFiles(t) {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range linkTarget.FindAllSubmatch(content, -1) {
			target := string(match[1])
			if strings.Contains(target, "://") {
				continue
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), target)); err != nil {
				t.Errorf("%s links %s, which does not exist", path, target)
			}
		}
	}
}
