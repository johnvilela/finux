package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"finux/src/mcp"
)

// mcpVersion is the version the MCP server reports to its client.
const mcpVersion = "0.1.0"

// mcpAgents lists the agents 'finux mcp config' knows.
var mcpAgents = []string{"claude", "codex", "opencode"}

func mcpHelp() string {
	return "Run the Finux MCP server for AI agents\n\n" +
		green.Render("Usage:") + ` finux mcp [--read-only]
       finux mcp config <` + strings.Join(mcpAgents, "|") + `> [--read-only]

` + green.Render("Commands:") + `
  (no command)          Start the server. It talks to the agent through its
                        standard input and output; the agent starts it.
  config <agent>        Show how to install the server in an agent
  help, --help, -h      Show this help

` + green.Render("Options:") + `
  --read-only           Give the agent only the tools that change nothing

The server gives an agent the tools of the CLI: accounts, categories,
transactions, credit cards and the database commands. Each tool returns
JSON. A tool that deletes data needs the argument confirm: without it, the
tool tells what it will delete and changes nothing.

The server uses the same database as the CLI: the file in the FINUX_DB
environment variable or ~/.finux/finux.db.
`
}

// mcpInstructions tell the agent the rules that all tools share.
const mcpInstructions = `Finux is a personal finance manager. These tools read and change its database.

- An account, a category and a credit card have a short code, for example NU or FOOD. Arguments accept the code or the numeric id.
- Send money as a decimal string in the currency of the account, for example "1234.56". Results give money as an object: minor (integer minor units), decimal and formatted.
- Send a date as YYYY-MM-DD or DD/MM/YYYY. Results give dates as YYYY-MM-DD and bills as YYYY-MM.
- A transaction is in an account or in a credit card, not in the two. The argument account accepts the code of a credit card.
- An expense has a negative amount and an income a positive one. A transfer is two transactions, one in each account. Transfers and payments are not in the totals.
- A tool that deletes data needs confirm: true. Without it, the tool returns confirm_required and a message, and changes nothing. Ask the user before you confirm.
- If a tool reports that the database is not found or is out of date, call database_init.`

func runMCP(args []string, io streams) int {
	if len(args) > 0 && isHelp(args[0]) {
		return write(io, mcpHelp())
	}
	var readOnly bool
	positional, _, err := parse("mcp", args, func(f *flag.FlagSet) { f.BoolVar(&readOnly, "read-only", false, "") }, 2)
	if err == nil {
		switch {
		case len(positional) == 0:
			server := &mcp.Server{Name: "finux", Version: mcpVersion, Instructions: mcpInstructions, Tools: mcpTools(readOnly)}
			if err := server.Serve(os.Stdin, io.stdout); err != nil {
				fmt.Fprintf(io.stderr, "finux: mcp: %v\n", err)
				return 1
			}
			return 0
		case positional[0] != "config":
			err = usageError("unexpected argument: " + positional[0])
		case len(positional) == 1 || !slices.Contains(mcpAgents, positional[1]):
			err = usageError("config: give the agent: " + strings.Join(mcpAgents, ", "))
		default:
			text, configErr := mcpConfig(positional[1], readOnly)
			if configErr != nil {
				fmt.Fprintf(io.stderr, "finux: mcp: %v\n", configErr)
				return 1
			}
			return write(io, text)
		}
	}
	return exitStatus(io, "mcp", err)
}

// mcpConfig returns the instructions to install the server of this
// binary in an agent.
func mcpConfig(agent string, readOnly bool) (string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find the finux binary: %w", err)
	}
	arguments := []string{"mcp"}
	if readOnly {
		arguments = append(arguments, "--read-only")
	}
	quoted := make([]string, len(arguments))
	for i, argument := range arguments {
		quoted[i] = strconv.Quote(argument)
	}
	line := binary + " " + strings.Join(arguments, " ")
	switch agent {
	case "claude":
		return "Run this command. It adds the server for all your projects:\n\n" +
			"  claude mcp add --scope user finux -- " + line + "\n", nil
	case "codex":
		return "Add this to ~/.codex/config.toml:\n\n" +
			"[mcp_servers.finux]\n" +
			"command = " + strconv.Quote(binary) + "\n" +
			"args = [" + strings.Join(quoted, ", ") + "]\n\n" +
			"Or run this command:\n\n" +
			"  codex mcp add finux -- " + line + "\n", nil
	}
	return "Add this to ~/.config/opencode/opencode.json, or to opencode.json in a project:\n\n" +
		"{\n" +
		"  \"$schema\": \"https://opencode.ai/config.json\",\n" +
		"  \"mcp\": {\n" +
		"    \"finux\": {\n" +
		"      \"type\": \"local\",\n" +
		"      \"command\": [" + strconv.Quote(binary) + ", " + strings.Join(quoted, ", ") + "],\n" +
		"      \"enabled\": true\n" +
		"    }\n" +
		"  }\n" +
		"}\n", nil
}
