package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"finux/src/setting"
	"finux/src/ui"
)

// aiAgent is an AI agent that 'finux config ai' can set up.
type aiAgent struct {
	// key is the word of the command line. name is the name of the agent.
	key, name string
}

var aiAgents = []aiAgent{{"claude", "Claude Code"}, {"codex", "Codex"}, {"opencode", "opencode"}}

// aiOptions are the options of 'finux config ai'.
type aiOptions struct {
	// readOnly gives the agent the tools that read data only.
	readOnly bool
	// noMemory skips ai-memory. yes installs ai-memory without a question.
	noMemory, yes bool
}

func aiHelp() string {
	return "Connect Finux to an AI agent\n\n" +
		green.Render("Usage:") + " finux config ai [agent] [--read-only]\n\n" +
		green.Render("Agents:") + `
  claude            Claude Code
  codex             Codex
  opencode          opencode

` + green.Render("Options:") + `
  --read-only       The agent can read your data but not change it
  --no-memory       Do not connect ai-memory to the agent
  --yes             Install ai-memory without a question, when it is missing
  -h, --help        Show this help

Without an agent, a selector opens.

The command does the steps of the setup:

1. It adds the Finux MCP server to the agent, so the agent has the tools
   that read and change your data. For Claude Code and Codex it runs their
   own command ('claude mcp add', 'codex mcp add'). For opencode it writes
   ~/.config/opencode/opencode.json and keeps a copy of the file as it was.
2. It installs the Finux skills in the skills folder of the agent.
3. It connects ai-memory to the agent, so the AI keeps what it learns and
   what you decide between sessions. It runs 'ai-memory setup' for the
   agent. When the ai-memory program is not installed, it asks before it
   runs the install script of the ai-memory project; without a terminal
   it installs only with --yes.
4. It makes the agent the one that answers /ai in the Telegram bot: the
   setting ai-provider. See 'finux config telegram --help'.

Run it again after you update Finux or move its binary: it replaces the
server and the skills. Then start a new session of the agent.

To do one step only, or to connect to a server on another machine, see
'finux config mcp --help' and 'finux config skills --help'.
`
}

func runAI(args []string, io streams) int {
	for _, arg := range args {
		if isHelp(arg) {
			return write(io, aiHelp())
		}
	}
	var options aiOptions
	positional, _, err := parse("ai", args, func(f *flag.FlagSet) {
		f.BoolVar(&options.readOnly, "read-only", false, "")
		f.BoolVar(&options.noMemory, "no-memory", false, "")
		f.BoolVar(&options.yes, "yes", false, "")
	}, 1)
	if err == nil {
		err = setupAgent(reference(positional), options, io)
	}
	// The module name is in the message of a usage error already.
	var usage usageError
	if errors.As(err, &usage) {
		err = usageError(strings.TrimPrefix(string(usage), "ai: "))
	}
	return exitStatus(io, "config ai", err)
}

// chooseAgent returns the agent for a word of the command line. Without
// a word it opens a selector.
func chooseAgent(key string, io streams) (aiAgent, error) {
	keys := make([]string, len(aiAgents))
	entries := make([]ui.Entry, len(aiAgents))
	for i, agent := range aiAgents {
		keys[i] = agent.key
		entries[i] = ui.Entry{Code: agent.key, Name: agent.name}
	}
	switch {
	case key == "" && !io.interactive:
		return aiAgent{}, usageError("give the agent: " + strings.Join(keys, ", "))
	case key == "":
		index, err := pick("Connect Finux to an AI agent", "agent", "agents", entries)
		if err != nil {
			return aiAgent{}, err
		}
		return aiAgents[index], nil
	}
	index := slices.Index(keys, strings.ToLower(key))
	if index < 0 {
		return aiAgent{}, usageError(fmt.Sprintf("unknown agent %q: use one of %s", key, strings.Join(keys, ", ")))
	}
	return aiAgents[index], nil
}

// setupAgent adds the MCP server to an agent and installs the skills.
func setupAgent(key string, options aiOptions, io streams) error {
	readOnly := options.readOnly
	agent, err := chooseAgent(key, io)
	if err != nil {
		return err
	}
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find the finux binary: %w", err)
	}
	server := []string{binary, "config", "mcp"}
	if readOnly {
		server = append(server, "--read-only")
	}
	if err := addServer(agent, server); err != nil {
		return fmt.Errorf("%w\nTo add the server by hand, run 'finux config mcp config %s'", err, agent.key)
	}
	added := "MCP server added to " + agent.name
	if readOnly {
		added += " (read-only)"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory: %w", err)
	}
	folder := filepath.Join(home, skillFolders[agent.key])
	written, _, err := writeSkills(folder)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(io.stdout, ui.Success(added)+"\n"+
		ui.Success(fmt.Sprintf("%s installed in %s", plural(int64(written), "skill"), folder))+"\n"); err != nil {
		return err
	}
	memory, err := setupMemory(agent, options, io)
	if err != nil {
		return err
	}
	if _, err := aiFolder(); err != nil {
		return err
	}
	_, err = fmt.Fprint(io.stdout, memory+
		saveAgent(agent)+
		"Start a new session of "+agent.name+" to use Finux.\n")
	return err
}

// saveAgent makes the agent the one that answers /ai in the Telegram
// bot. It returns the line that tells it. Without a database it tells
// how to do it later: the setup of an agent needs no database.
func saveAgent(agent aiAgent) string {
	later := fmt.Sprintf("No database yet: run 'finux init', then 'finux config set %s %s' to use /ai in Telegram.\n", setting.AIProvider, agent.key)
	db, store, err := openSettings()
	if err != nil {
		return later
	}
	defer closeDatabase(db)
	if err := store.Set(setting.AIProvider, agent.key); err != nil {
		return later
	}
	saved := ui.Success(fmt.Sprintf("%s set to %s: /ai in Telegram uses %s", setting.AIProvider, agent.key, agent.name)) + "\n"
	if _, err := installed(); err == nil {
		saved += "Run 'finux daemon install' again from this terminal: the service then has the PATH that finds " + agent.name + ", and the bot shows /ai.\n"
	}
	return saved
}

// addServer adds the MCP server to an agent. server is the command that
// starts it. A server with the name finux that is there is replaced.
func addServer(agent aiAgent, server []string) error {
	switch agent.key {
	case "claude":
		return agentCommand(agent, []string{"mcp", "remove", "--scope", "user", "finux"},
			append([]string{"mcp", "add", "--scope", "user", "finux", "--"}, server...))
	case "codex":
		return agentCommand(agent, []string{"mcp", "remove", "finux"}, append([]string{"mcp", "add", "finux", "--"}, server...))
	}
	return addOpencodeServer(server)
}

// agentCommand runs the command of an agent that removes the server,
// which fails when no server is there, then the one that adds it.
func agentCommand(agent aiAgent, remove, add []string) error {
	path, err := exec.LookPath(agent.key)
	if err != nil {
		return fmt.Errorf("the command '%s' is not in your PATH: install %s first", agent.key, agent.name)
	}
	_ = exec.Command(path, remove...).Run()
	if output, err := exec.Command(path, add...).CombinedOutput(); err != nil {
		return fmt.Errorf("'%s %s' failed: %s", agent.key, strings.Join(add[:2], " "), strings.TrimSpace(string(output)+" "+err.Error()))
	}
	return nil
}

// addOpencodeServer writes the server to the configuration file of
// opencode. It keeps the other values of the file and a copy of the
// file as it was.
func addOpencodeServer(server []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory: %w", err)
	}
	file := filepath.Join(home, ".config", "opencode", "opencode.json")
	config := map[string]any{"$schema": "https://opencode.ai/config.json"}
	mode := fs.FileMode(0o644)
	content, err := os.ReadFile(file)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		// A file with comments is not JSON: leave it to the user.
		if err := json.Unmarshal(content, &config); err != nil {
			return fmt.Errorf("cannot read %s as JSON (%v): the file did not change", file, err)
		}
		if info, err := os.Stat(file); err == nil {
			mode = info.Mode().Perm()
		}
		if err := os.WriteFile(file+".backup", content, mode); err != nil {
			return fmt.Errorf("copy %s: %w", file, err)
		}
	}
	servers, ok := config["mcp"].(map[string]any)
	if !ok {
		servers = map[string]any{}
	}
	servers["finux"] = map[string]any{"type": "local", "command": server, "enabled": true}
	config["mcp"] = servers
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(encoded, '\n'), mode)
}
