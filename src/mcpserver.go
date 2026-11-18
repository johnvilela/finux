package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"finux/src/mcp"
)

// mcpVersion is the version the MCP server reports to its client.
const mcpVersion = "0.1.0"

// mcpTokenVariable names the environment variable that holds the bearer
// token of the HTTP transport.
const mcpTokenVariable = "FINUX_MCP_TOKEN"

// mcpPath is the path of the endpoint of the HTTP transport.
const mcpPath = "/mcp"

// mcpAgents lists the agents 'finux mcp config' knows.
var mcpAgents = []string{"claude", "codex", "opencode"}

func mcpHelp() string {
	return "Run the Finux MCP server for AI agents\n\n" +
		green.Render("Usage:") + ` finux mcp [--read-only] [--http <address>]
       finux mcp config <` + strings.Join(mcpAgents, "|") + `> [--read-only] [--url <url>]

` + green.Render("Commands:") + `
  (no command)          Start the server. It talks to the agent through its
                        standard input and output; the agent starts it.
  config <agent>        Show how to install the server in an agent
  help, --help, -h      Show this help

` + green.Render("Options:") + `
  --read-only           Give the agent only the tools that change nothing
  --http <address>      Serve HTTP on the address, for example 127.0.0.1:8765,
                        and do not use the standard input and output
  --url <url>           For config: the URL of a server that --http started

The server gives an agent the tools of the CLI: accounts, categories,
transactions, credit cards and the database commands. Each tool returns
JSON. A tool that deletes data needs the argument confirm: without it, the
tool tells what it will delete and changes nothing.

With --http, the server is one process that many agents can use. Its
endpoint is http://<address>/mcp. When the ` + mcpTokenVariable + ` environment
variable has a value, each request needs that value as a bearer token. An
address that other computers can reach needs the token. The server does not
encrypt: for a different computer, put it behind a tunnel or a proxy that
has HTTPS.

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
	var address, url string
	positional, given, err := parse("mcp", args, func(f *flag.FlagSet) {
		f.BoolVar(&readOnly, "read-only", false, "")
		f.StringVar(&address, "http", "", "")
		f.StringVar(&url, "url", "", "")
	}, 2)
	if err != nil {
		return exitStatus(io, "mcp", err)
	}
	server := &mcp.Server{Name: "finux", Version: mcpVersion, Instructions: mcpInstructions, Tools: mcpTools(readOnly)}
	switch {
	case len(positional) == 0 && given["url"]:
		err = usageError("--url is only for config")
	case len(positional) == 0 && given["http"]:
		err = serveHTTP(server, address, io)
	case len(positional) == 0:
		err = server.Serve(os.Stdin, io.stdout)
	case positional[0] != "config":
		err = usageError("unexpected argument: " + positional[0])
	case len(positional) == 1 || !slices.Contains(mcpAgents, positional[1]):
		err = usageError("config: give the agent: " + strings.Join(mcpAgents, ", "))
	case given["http"]:
		err = usageError("config: give the URL of the server with --url, not --http")
	default:
		var text string
		if text, err = mcpConfig(positional[1], readOnly, url); err == nil {
			return write(io, text)
		}
	}
	var usage usageError
	if err != nil && !errors.As(err, &usage) {
		err = fmt.Errorf("mcp: %w", err)
	}
	return exitStatus(io, "mcp", err)
}

// serveHTTP serves the HTTP transport on the address until the process
// gets an interrupt. It writes the URL of the endpoint to stderr.
func serveHTTP(server *mcp.Server, address string, io streams) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return usageError(fmt.Sprintf("invalid address %q: use host:port, for example 127.0.0.1:8765", address))
	}
	token := os.Getenv(mcpTokenVariable)
	if token == "" && !mcp.Loopback(host) {
		return usageError(fmt.Sprintf("other computers can reach the address %s: set a token in the %s environment variable", address, mcpTokenVariable))
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	routes := http.NewServeMux()
	routes.Handle(mcpPath, server.Handler(token))
	web := &http.Server{Handler: routes, ReadHeaderTimeout: 10 * time.Second}

	interrupt, release := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer release()
	stopped := make(chan error, 1)
	go func() {
		<-interrupt.Done()
		deadline, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stopped <- web.Shutdown(deadline)
	}()
	protection := "no token"
	if token != "" {
		protection = "token from " + mcpTokenVariable
	}
	fmt.Fprintf(io.stderr, "Finux MCP server listens on http://%s%s (%s)\n", listener.Addr(), mcpPath, protection)
	if err := web.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return <-stopped
}

// mcpConfig returns the instructions to install the server in an agent.
// Without a URL, the agent starts this binary. With a URL, the agent
// connects to a server that 'finux mcp --http' started.
func mcpConfig(agent string, readOnly bool, url string) (string, error) {
	if url != "" {
		return mcpRemoteConfig(agent, url), nil
	}
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

// mcpRemoteConfig returns the instructions to connect an agent to the
// HTTP transport. It names the token variable, never the token.
func mcpRemoteConfig(agent, url string) string {
	withToken := os.Getenv(mcpTokenVariable) != ""
	note := "\nThe server must run: start it with 'finux mcp --http <address>'.\n"
	if withToken {
		note += "The agent reads the token from the " + mcpTokenVariable + " environment variable.\n"
	}
	switch agent {
	case "claude":
		header := ""
		if withToken {
			header = ` --header "Authorization: Bearer $` + mcpTokenVariable + `"`
		}
		return "Run this command. It adds the server for all your projects:\n\n" +
			"  claude mcp add --transport http --scope user finux " + url + header + "\n" + note
	case "codex":
		token := ""
		if withToken {
			token = "bearer_token_env_var = " + strconv.Quote(mcpTokenVariable) + "\n"
		}
		return "Add this to ~/.codex/config.toml:\n\n" +
			"[mcp_servers.finux]\n" +
			"url = " + strconv.Quote(url) + "\n" + token + note
	}
	headers := ""
	if withToken {
		headers = ",\n      \"headers\": {\n        \"Authorization\": \"Bearer {env:" + mcpTokenVariable + "}\"\n      }"
	}
	return "Add this to ~/.config/opencode/opencode.json, or to opencode.json in a project:\n\n" +
		"{\n" +
		"  \"$schema\": \"https://opencode.ai/config.json\",\n" +
		"  \"mcp\": {\n" +
		"    \"finux\": {\n" +
		"      \"type\": \"remote\",\n" +
		"      \"url\": " + strconv.Quote(url) + ",\n" +
		"      \"enabled\": true" + headers + "\n" +
		"    }\n" +
		"  }\n" +
		"}\n" + note
}
