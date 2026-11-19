package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"finux/skills"
	"finux/src/ui"
)

// skillFolders gives the folder of each agent that holds its skills,
// below the home directory of the user.
var skillFolders = map[string]string{
	"claude":   filepath.Join(".claude", "skills"),
	"codex":    filepath.Join(".codex", "skills"),
	"opencode": filepath.Join(".config", "opencode", "skills"),
}

func skillsHelp() string {
	return "Install the Finux skills for AI agents\n\n" +
		green.Render("Usage:") + ` finux config skills [list]
       finux config skills install <claude|codex|opencode>
       finux config skills install --dir <folder>
       finux config skills remove <claude|codex|opencode>
       finux config skills remove --dir <folder>

` + green.Render("Commands:") + `
  list, l                      List the skills (default)
  install, i <agent> | --dir   Install or update the skills
  remove, r <agent> | --dir    Remove the skills
  help, --help, -h             Show this help

` + green.Render("Agents:") + `
  claude                ~/.claude/skills
  codex                 ~/.codex/skills
  opencode              ~/.config/opencode/skills

` + green.Render("Options for install and remove:") + `
  --dir <folder>        Use this skills folder in place of the folder of an
                        agent, for example .claude/skills in a project

A skill is a workflow for an agent that manages your finances with Finux,
for example to record a transaction or to import a statement. The skills
are in the finux binary: install needs no other file.

Install writes one folder for each skill. Run it again after you update
Finux: it replaces the Finux skills and keeps all other skills. A skill
folder that is a link stays as it is. The skills use the MCP server; see
'finux config mcp --help'.
`
}

func runSkills(args []string, io streams) int {
	handlers := map[string]func([]string, streams) error{
		"list": listSkills, "l": listSkills,
		"install": installSkills, "i": installSkills,
		"remove": removeSkills, "r": removeSkills,
	}
	handler, rest := listSkills, args
	if len(args) > 0 {
		if isHelp(args[0]) {
			return write(io, skillsHelp())
		}
		known, ok := handlers[args[0]]
		if !ok {
			return exitStatus(io, "config skills", usageError("unknown command: "+args[0]))
		}
		handler, rest = known, args[1:]
	}
	return exitStatus(io, "config skills", handler(rest, io))
}

// skillList returns the names of the skills in the binary.
func skillList() ([]string, error) {
	entries, err := skills.Files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

func listSkills(args []string, io streams) error {
	if _, _, err := parse("list", args, nil, 0); err != nil {
		return err
	}
	names, err := skillList()
	if err != nil {
		return err
	}
	var out strings.Builder
	for _, name := range names {
		content, err := skills.Files.ReadFile(name + "/SKILL.md")
		if err != nil {
			return err
		}
		// The first sentence of the description tells what the skill does.
		_, description, _ := strings.Cut(string(content), "\ndescription: ")
		description, _, _ = strings.Cut(description, "\n")
		summary, _, _ := strings.Cut(description, ". ")
		fmt.Fprintf(&out, "%s\n  %s.\n", name, strings.TrimSuffix(summary, "."))
	}
	out.WriteString("\nRun 'finux config ai <claude|codex|opencode>' to install them and the MCP server.\n")
	_, err = fmt.Fprint(io.stdout, out.String())
	return err
}

// skillsFolder reads the arguments of install and remove and returns the
// skills folder they select.
func skillsFolder(command string, args []string) (string, error) {
	var dir string
	positional, given, err := parse(command, args, func(f *flag.FlagSet) { f.StringVar(&dir, "dir", "", "") }, 1)
	if err != nil {
		return "", err
	}
	agents := []string{"claude", "codex", "opencode"}
	switch {
	case given["dir"] && len(positional) > 0:
		return "", usageError(command + ": give an agent or --dir, not the two")
	case given["dir"] && dir == "":
		return "", usageError(command + ": --dir is empty")
	case given["dir"]:
		return dir, nil
	case len(positional) == 0 || !slices.Contains(agents, positional[0]):
		return "", usageError(command + ": give the agent (" + strings.Join(agents, ", ") + ") or --dir <folder>")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, skillFolders[positional[0]]), nil
}

// isLink reports whether the path is a symbolic link.
func isLink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&fs.ModeSymlink != 0
}

func installSkills(args []string, io streams) error {
	folder, err := skillsFolder("install", args)
	if err != nil {
		return err
	}
	_, report, err := writeSkills(folder)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(io.stdout, report+"\nSkills folder: "+folder+"\nStart a new session of the agent to use them.\n")
	return err
}

// writeSkills writes the skills of the binary to a skills folder. It
// returns the number of skills it wrote and one line for each skill.
func writeSkills(folder string) (written int, report string, err error) {
	names, err := skillList()
	if err != nil {
		return 0, "", err
	}
	var out strings.Builder
	for _, name := range names {
		target := filepath.Join(folder, name)
		if isLink(target) {
			out.WriteString(ui.Hint(name+" is a link: not changed") + "\n")
			continue
		}
		action := "installed"
		if _, err := os.Stat(target); err == nil {
			action = "updated"
		}
		// Start from an empty folder, so that an old file does not stay.
		if err := os.RemoveAll(target); err != nil {
			return written, "", fmt.Errorf("replace skill %s: %w", name, err)
		}
		if err := copySkill(name, target); err != nil {
			return written, "", fmt.Errorf("write skill %s: %w", name, err)
		}
		written++
		out.WriteString(ui.Success(name+" "+action) + "\n")
	}
	return written, out.String(), nil
}

// copySkill writes the files of one skill of the binary to target.
func copySkill(name, target string) error {
	return fs.WalkDir(skills.Files, name, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		destination := filepath.Join(target, filepath.FromSlash(strings.TrimPrefix(path, name)))
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		content, err := skills.Files.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, content, 0o644)
	})
}

func removeSkills(args []string, io streams) error {
	folder, err := skillsFolder("remove", args)
	if err != nil {
		return err
	}
	names, err := skillList()
	if err != nil {
		return err
	}
	var out strings.Builder
	for _, name := range names {
		target := filepath.Join(folder, name)
		if _, err := os.Lstat(target); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		// For a link, this removes the link and keeps what it points to.
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove skill %s: %w", name, err)
		}
		out.WriteString(ui.Success(name+" removed") + "\n")
	}
	if out.Len() == 0 {
		return notice("No Finux skills in " + folder + ".")
	}
	_, err = fmt.Fprint(io.stdout, out.String())
	return err
}
