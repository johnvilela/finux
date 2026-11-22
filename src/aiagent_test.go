package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeAgent writes a program that acts as an AI agent and makes it the
// agent of this test process. The program writes its arguments and its
// input to a log, and answers with the first line of the input. An
// input with "slow" waits, one with "fail" fails, and one with "refuse"
// answers with an error. calls gives the log and empties it.
func fakeAgent(t *testing.T) (program string, calls func() string) {
	t.Helper()
	folder := t.TempDir()
	program, log := filepath.Join(folder, "agent"), filepath.Join(folder, "agent.log")
	script := `#!/bin/sh
PATH=/usr/bin:/bin
input=$(cat)
first=$(printf '%s\n' "$input" | head -n 1)
session="new-$$"
previous=""
for argument in "$@"; do
	if [ "$previous" = "--resume" ]; then session="$argument"; fi
	previous="$argument"
done
{
	printf 'ARGS'
	printf ' [%s]' "$@"
	printf '\nDIR %s\nINPUT %s\nEND\n' "$PWD" "$input"
} >> '` + log + `'
case "$input" in
*slow*) sleep 30 ;;
*fail*) echo "the model is not available" >&2; exit 1 ;;
*refuse*) printf '{"type":"result","is_error":true,"result":"Credit balance is too low","session_id":"%s"}\n' "$session"; exit 1 ;;
*gone*) if [ "$session" = "old" ]; then echo "No conversation found with session ID: old" >&2; exit 1; fi ;;
esac
printf '{"type":"result","is_error":false,"result":"You said: %s","session_id":"%s"}\n' "$first" "$session"
`
	if err := os.WriteFile(program, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return program, func() string {
		text, _ := os.ReadFile(log)
		os.Remove(log)
		return string(text)
	}
}

func TestAskAgent(t *testing.T) {
	program, calls := fakeAgent(t)
	t.Setenv("FINUX_AI_COMMAND", program)
	t.Setenv("HOME", t.TempDir())
	folder := t.TempDir()
	inbox := filepath.Join(folder, "inbox", "42")
	request := agentRequest{agent: "claude", prompt: "How was my month?\nBe short.", rules: "You answer John.", folder: folder, inbox: inbox}

	answer, err := askAgent(context.Background(), request)
	if err != nil || answer.text != "You said: How was my month?" || !strings.HasPrefix(answer.session, "new-") {
		t.Fatalf("askAgent = %+v, %v", answer, err)
	}
	log := calls()
	for _, want := range []string{
		"[-p]", "[--output-format] [json]",
		// The agent has no shell and writes no file.
		"[--tools] [Read,Skill]", "[--permission-mode] [dontAsk]",
		"[--allowedTools] [mcp__finux] [mcp__ai-memory] [Skill] [Read(/" + inbox + "/**)]",
		"[--append-system-prompt] [You answer John.]",
		"DIR " + folder + "\n", "INPUT How was my month?\nBe short.\nEND\n",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("the call has no %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "--resume") || strings.Contains(log, "--model") || strings.Contains(log, "Bash") {
		t.Errorf("first call:\n%s", log)
	}

	// The next message continues the session, with the model of the setting.
	request.session, request.model = answer.session, "sonnet"
	again, err := askAgent(context.Background(), request)
	if err != nil || again.session != answer.session {
		t.Fatalf("second askAgent = %+v, %v", again, err)
	}
	if log := calls(); !strings.Contains(log, "[--resume] ["+answer.session+"]") || !strings.Contains(log, "[--model] [sonnet]") {
		t.Fatalf("second call:\n%s", log)
	}

	// A session that the agent does not have any more starts a new one.
	request.session, request.prompt = "old", "gone"
	if fresh, err := askAgent(context.Background(), request); err != nil || !strings.HasPrefix(fresh.session, "new-") {
		t.Fatalf("askAgent with a lost session = %+v, %v", fresh, err)
	}
	if log := calls(); strings.Count(log, "ARGS") != 2 {
		t.Fatalf("calls for a lost session:\n%s", log)
	}

	request.session, request.prompt = "", "fail"
	if _, err := askAgent(context.Background(), request); err == nil || !strings.Contains(err.Error(), "the model is not available") {
		t.Fatalf("askAgent of a failed agent = %v", err)
	}
	request.prompt = "refuse"
	if _, err := askAgent(context.Background(), request); err == nil || !strings.Contains(err.Error(), "Credit balance is too low") {
		t.Fatalf("askAgent of an agent that answers an error = %v", err)
	}
	// A stop ends the program.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	request.prompt = "slow"
	start := time.Now()
	if _, err := askAgent(ctx, request); err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("askAgent with a timeout = %v after %v", err, time.Since(start))
	}

	// Only claude has the limits that /ai needs.
	for _, agent := range []string{"codex", "opencode"} {
		request.agent = agent
		if _, err := askAgent(context.Background(), request); err == nil || !strings.Contains(err.Error(), "cannot answer /ai yet") {
			t.Errorf("askAgent(%s) = %v", agent, err)
		}
	}
	t.Setenv("FINUX_AI_COMMAND", "")
	t.Setenv("PATH", t.TempDir())
	request.agent = "claude"
	if _, err := askAgent(context.Background(), request); err == nil || !strings.Contains(err.Error(), "the command 'claude' is not in the PATH of the daemon") {
		t.Fatalf("askAgent without the program = %v", err)
	}
}
