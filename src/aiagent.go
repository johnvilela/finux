package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// agentCommandVariable names the environment variable that replaces
	// the program of the agent, for tests.
	agentCommandVariable = "FINUX_AI_COMMAND"
	// agentErrorSize is the size limit of the text of an error of the agent.
	agentErrorSize = 300
)

// agentRequest is one message for the AI agent of /ai.
type agentRequest struct {
	// agent is the key of the agent: claude, codex or opencode.
	agent string
	// session is the session to continue, or "" for a new one.
	session string
	// prompt is the message of the person. rules tell the agent how to
	// answer. model is the model to use, or "" for the one of the agent.
	prompt, rules, model string
	// folder is where the agent runs. inbox is the folder of the files
	// of the person: the only one the agent can read.
	folder, inbox string
}

// agentAnswer is the answer of the agent and its session.
type agentAnswer struct {
	text, session string
}

// askAgent sends a message to the agent program and returns its answer.
// The agent gets the Finux tools, the ai-memory tools, the skills and
// read access to files: it writes no file. The files that a person
// sends hold text from outside, which can try to give the agent orders,
// so the limits are the protection. Claude Code has no shell and reads
// only the inbox. Codex has a shell in a sandbox with no write and no
// network, so it can read all the files of the user.
func askAgent(ctx context.Context, request agentRequest) (agentAnswer, error) {
	run, lost := runClaude, "No conversation found"
	switch request.agent {
	case "claude":
	case "codex":
		run, lost = runCodex, "no rollout found"
	default:
		return agentAnswer{}, fmt.Errorf("the agent %s cannot answer /ai yet: Finux can limit only claude and codex. Run 'finux config ai claude' or 'finux config ai codex'", request.agent)
	}
	program := os.Getenv(agentCommandVariable)
	if program == "" {
		found, err := exec.LookPath(request.agent)
		if err != nil {
			return agentAnswer{}, fmt.Errorf("the command '%s' is not in the PATH of the daemon: run 'finux daemon install' again from a terminal that has it", request.agent)
		}
		program = found
	}
	answer, err := run(ctx, program, request)
	if err != nil && request.session != "" && ctx.Err() == nil && strings.Contains(err.Error(), lost) {
		// The agent does not have the session any more: start a new one.
		request.session = ""
		return run(ctx, program, request)
	}
	return answer, err
}

// runAgent runs the agent program one time, with the prompt as its
// input. It returns what the program wrote and its failure.
func runAgent(ctx context.Context, program string, args []string, request agentRequest) (stdout, stderr []byte, failure error) {
	command := exec.CommandContext(ctx, program, args...)
	command.Dir = request.folder
	command.Stdin = strings.NewReader(request.prompt)
	// An interrupt lets the agent end its tools; a kill comes after it.
	command.Cancel = func() error { return command.Process.Signal(os.Interrupt) }
	command.WaitDelay = 3 * time.Second
	var out, problems bytes.Buffer
	command.Stdout, command.Stderr = &out, &problems
	failure = command.Run()
	return out.Bytes(), problems.Bytes(), failure
}

// runClaude runs Claude Code one time, with no screen.
func runClaude(ctx context.Context, program string, request agentRequest) (agentAnswer, error) {
	skills := ""
	if home, err := os.UserHomeDir(); err == nil {
		skills = filepath.Join(home, ".claude", "skills")
	}
	// A permission rule with a path from the root starts with "//".
	allowed := []string{"mcp__finux", "mcp__ai-memory", "Skill", "Read(/" + request.inbox + "/**)"}
	if skills != "" {
		allowed = append(allowed, "Read(/"+skills+"/**)")
	}
	args := append([]string{"-p", "--output-format", "json", "--tools", "Read,Skill", "--permission-mode", "dontAsk", "--allowedTools"}, allowed...)
	args = append(args, "--append-system-prompt", request.rules)
	if request.model != "" {
		args = append(args, "--model", request.model)
	}
	if request.session != "" {
		args = append(args, "--resume", request.session)
	}
	stdout, stderr, failure := runAgent(ctx, program, args, request)
	if ctx.Err() != nil {
		return agentAnswer{}, ctx.Err()
	}
	var result struct {
		Text    string `json:"result"`
		Session string `json:"session_id"`
		IsError bool   `json:"is_error"`
	}
	decoded := json.Unmarshal(bytes.TrimSpace(stdout), &result) == nil
	switch {
	case decoded && result.IsError:
		return agentAnswer{}, errors.New(short(result.Text))
	case failure != nil:
		return agentAnswer{}, errors.New(short(strings.TrimSpace(string(stderr) + " " + failure.Error())))
	case !decoded:
		return agentAnswer{}, errors.New("the agent gave an answer that Finux cannot read")
	}
	return agentAnswer{text: strings.TrimSpace(result.Text), session: result.Session}, nil
}

// runCodex runs Codex one time, with no screen. Codex has no list of
// allowed tools, so the limits are a sandbox and a config that Finux
// gives: the config of the person, with its other servers, its plugins
// and its hooks, is not loaded.
func runCodex(ctx context.Context, program string, request agentRequest) (agentAnswer, error) {
	args := []string{"exec"}
	if request.session != "" {
		args = append(args, "resume")
	}
	args = append(args, "--ignore-user-config", "--skip-git-repo-check", "--json",
		"-c", "sandbox_mode="+toml("read-only"), "-c", "approval_policy="+toml("never"),
		"-c", "web_search="+toml("disabled"), "--disable", "apps")
	finux, err := codexServer(ctx, program, "finux")
	if err != nil {
		return agentAnswer{}, err
	}
	if len(finux) == 0 {
		return agentAnswer{}, errors.New("codex does not have the MCP server of Finux. Run 'finux config ai codex'")
	}
	// ai-memory is optional: without it, the agent keeps no memory.
	memory, err := codexServer(ctx, program, "ai-memory")
	if err != nil {
		return agentAnswer{}, err
	}
	args = append(append(args, finux...), memory...)
	if request.rules != "" {
		args = append(args, "-c", "developer_instructions="+toml(request.rules))
	}
	if request.model != "" {
		args = append(args, "-m", request.model)
	}
	if request.session != "" {
		args = append(args, request.session)
	}
	// "-" makes Codex read the prompt from its input.
	stdout, stderr, failure := runAgent(ctx, program, append(args, "-"), request)
	if ctx.Err() != nil {
		return agentAnswer{}, ctx.Err()
	}
	// The output has one JSON event in each line. The answer is the last
	// message of the agent.
	var answer agentAnswer
	problem, events := "", 0
	for _, line := range bytes.Split(stdout, []byte("\n")) {
		var event struct {
			Type    string `json:"type"`
			Thread  string `json:"thread_id"`
			Message string `json:"message"`
			Error   struct {
				Message string `json:"message"`
			} `json:"error"`
			Item struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		events++
		switch {
		case event.Type == "thread.started":
			answer.session = event.Thread
		case event.Type == "item.completed" && event.Item.Type == "agent_message":
			answer.text = strings.TrimSpace(event.Item.Text)
		case event.Type == "turn.failed":
			problem = event.Error.Message
		case event.Type == "error":
			problem = event.Message
		}
	}
	switch {
	case problem != "" && (failure != nil || answer.text == ""):
		// An error event with an answer after it was a problem that Codex
		// got past, for example a connection that it made again.
		return agentAnswer{}, errors.New(short(problem))
	case failure != nil:
		return agentAnswer{}, errors.New(short(strings.TrimSpace(string(stderr) + " " + failure.Error())))
	case events == 0:
		return agentAnswer{}, errors.New("the agent gave an answer that Finux cannot read")
	}
	return answer, nil
}

// codexServer returns the arguments that give Codex one MCP server of
// its own config, or none when the config does not have the server.
func codexServer(ctx context.Context, program, name string) ([]string, error) {
	output, err := exec.CommandContext(ctx, program, "mcp", "get", name, "--json").Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, nil
	}
	var server struct {
		Transport struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
			URL     string   `json:"url"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(output, &server); err != nil {
		return nil, fmt.Errorf("codex gave an MCP server that Finux cannot read: %w", err)
	}
	key := "mcp_servers." + name
	switch transport := server.Transport; {
	case transport.URL != "":
		return []string{"-c", key + ".url=" + toml(transport.URL)}, nil
	case transport.Command != "":
		if transport.Args == nil {
			transport.Args = []string{}
		}
		return []string{"-c", key + ".command=" + toml(transport.Command), "-c", key + ".args=" + toml(transport.Args)}, nil
	}
	return nil, nil
}

// toml writes a text or a list of texts as a TOML value. JSON without
// the HTML escapes has the same form.
func toml(value any) string {
	var text bytes.Buffer
	encoder := json.NewEncoder(&text)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
	return strings.TrimSpace(text.String())
}

// short cuts the text of an error of the agent.
func short(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if runes := []rune(text); len(runes) > agentErrorSize {
		return string(runes[:agentErrorSize]) + "…"
	}
	return text
}
