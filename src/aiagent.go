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
// read access to the inbox: no shell and no file write. The files that
// a person sends hold text from outside, which can try to give the
// agent orders, so the limits are the protection.
func askAgent(ctx context.Context, request agentRequest) (agentAnswer, error) {
	if request.agent != "claude" {
		return agentAnswer{}, fmt.Errorf("the agent %s cannot answer /ai yet: Finux can limit only claude to the Finux tools. Run 'finux config ai claude'", request.agent)
	}
	program := os.Getenv(agentCommandVariable)
	if program == "" {
		found, err := exec.LookPath(request.agent)
		if err != nil {
			return agentAnswer{}, fmt.Errorf("the command '%s' is not in the PATH of the daemon: run 'finux daemon install' again from a terminal that has it", request.agent)
		}
		program = found
	}
	answer, err := runClaude(ctx, program, request)
	if err != nil && request.session != "" && ctx.Err() == nil && strings.Contains(err.Error(), "No conversation found") {
		// The agent does not have the session any more: start a new one.
		request.session = ""
		return runClaude(ctx, program, request)
	}
	return answer, err
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
	command := exec.CommandContext(ctx, program, args...)
	command.Dir = request.folder
	command.Stdin = strings.NewReader(request.prompt)
	// An interrupt lets the agent end its tools; a kill comes after it.
	command.Cancel = func() error { return command.Process.Signal(os.Interrupt) }
	command.WaitDelay = 3 * time.Second
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	failure := command.Run()
	if ctx.Err() != nil {
		return agentAnswer{}, ctx.Err()
	}
	var result struct {
		Text    string `json:"result"`
		Session string `json:"session_id"`
		IsError bool   `json:"is_error"`
	}
	decoded := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result) == nil
	switch {
	case decoded && result.IsError:
		return agentAnswer{}, errors.New(short(result.Text))
	case failure != nil:
		return agentAnswer{}, errors.New(short(strings.TrimSpace(stderr.String() + " " + failure.Error())))
	case !decoded:
		return agentAnswer{}, errors.New("the agent gave an answer that Finux cannot read")
	}
	return agentAnswer{text: strings.TrimSpace(result.Text), session: result.Session}, nil
}

// short cuts the text of an error of the agent.
func short(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if runes := []rune(text); len(runes) > agentErrorSize {
		return string(runes[:agentErrorSize]) + "…"
	}
	return text
}
