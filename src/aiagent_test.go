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

	// opencode has no limits that /ai can use.
	request.agent = "opencode"
	if _, err := askAgent(context.Background(), request); err == nil || !strings.Contains(err.Error(), "cannot answer /ai yet") {
		t.Errorf("askAgent(opencode) = %v", err)
	}
	t.Setenv("FINUX_AI_COMMAND", "")
	t.Setenv("PATH", t.TempDir())
	request.agent = "claude"
	if _, err := askAgent(context.Background(), request); err == nil || !strings.Contains(err.Error(), "the command 'claude' is not in the PATH of the daemon") {
		t.Fatalf("askAgent without the program = %v", err)
	}
}

// fakeCodex writes a program that acts as Codex and makes it the agent
// of this test process. 'mcp get' gives the servers of its config: finux
// always, and ai-memory when memory is true. 'exec' writes its arguments
// and its input to a log and answers with the first line of the input,
// as JSONL events. calls gives the log and empties it.
func fakeCodex(t *testing.T, memory bool) (calls func() string) {
	t.Helper()
	folder := t.TempDir()
	program, log := filepath.Join(folder, "codex"), filepath.Join(folder, "codex.log")
	known := "finux"
	if memory {
		known = "finux|ai-memory"
	}
	script := `#!/bin/sh
PATH=/usr/bin:/bin
if [ "$1" = "mcp" ]; then
	case "$3" in
	` + known + `) ;;
	*) echo "Error: No MCP server named '$3' found." >&2; exit 1 ;;
	esac
	if [ "$3" = "finux" ]; then
		printf '{"name":"finux","transport":{"type":"stdio","command":"/opt/fin ux","args":["config","mcp"]}}\n'
	else
		printf '{"name":"ai-memory","transport":{"type":"streamable_http","url":"http://127.0.0.1:49374/mcp"}}\n'
	fi
	exit 0
fi
input=$(cat)
first=$(printf '%s\n' "$input" | head -n 1)
session="new-$$"
if [ "$2" = "resume" ]; then
	for argument in "$@"; do session="$previous"; previous="$argument"; done
fi
{
	printf 'ARGS'
	printf ' [%s]' "$@"
	printf '\nDIR %s\nINPUT %s\nEND\n' "$PWD" "$input"
} >> '` + log + `'
case "$input" in
*slow*) sleep 30 ;;
*fail*) echo "codex is not logged in" >&2; exit 1 ;;
*refuse*)
	printf '{"type":"thread.started","thread_id":"%s"}\n' "$session"
	printf '{"type":"error","message":"The model is not supported"}\n'
	printf '{"type":"turn.failed","error":{"message":"The model is not supported"}}\n'
	exit 1 ;;
*gone*) if [ "$session" = "old" ]; then echo "Error: thread/resume: thread/resume failed: no rollout found for thread id old (code -32600)" >&2; exit 1; fi ;;
esac
printf '{"type":"thread.started","thread_id":"%s"}\n{"type":"turn.started"}\n' "$session"
printf '{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"I look at the data."}}\n'
printf '{"type":"item.completed","item":{"id":"item_1","type":"mcp_tool_call","server":"finux","tool":"account_list"}}\n'
printf '{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"You said: %s"}}\n' "$first"
printf '{"type":"turn.completed","usage":{"input_tokens":1}}\n'
`
	if err := os.WriteFile(program, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FINUX_AI_COMMAND", program)
	return func() string {
		text, _ := os.ReadFile(log)
		os.Remove(log)
		return string(text)
	}
}

func TestAskCodex(t *testing.T) {
	calls := fakeCodex(t, true)
	t.Setenv("HOME", t.TempDir())
	folder := t.TempDir()
	request := agentRequest{agent: "codex", prompt: "How was my month?\nBe short.", rules: "You answer \"John\".", folder: folder, inbox: filepath.Join(folder, "inbox", "42")}

	// The answer is the last message of the agent.
	answer, err := askAgent(context.Background(), request)
	if err != nil || answer.text != "You said: How was my month?" || !strings.HasPrefix(answer.session, "new-") {
		t.Fatalf("askAgent = %+v, %v", answer, err)
	}
	log := calls()
	for _, want := range []string{
		"ARGS [exec] [--ignore-user-config] [--skip-git-repo-check] [--json]",
		// The shell of the agent writes no file and has no network.
		`[-c] [sandbox_mode="read-only"]`, `[-c] [approval_policy="never"]`,
		// No web search and no apps of the Codex account.
		`[-c] [web_search="disabled"]`, "[--disable] [apps]",
		// Only the servers of Finux and of ai-memory, not the ones of the person.
		`[-c] [mcp_servers.finux.command="/opt/fin ux"]`, `[-c] [mcp_servers.finux.args=["config","mcp"]]`,
		`[-c] [mcp_servers.ai-memory.url="http://127.0.0.1:49374/mcp"]`,
		`[-c] [developer_instructions="You answer \"John\"."]`,
		" [-]\n", "DIR " + folder + "\n", "INPUT How was my month?\nBe short.\nEND\n",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("the call has no %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "resume") || strings.Contains(log, "[-m]") || strings.Contains(log, "danger") {
		t.Errorf("first call:\n%s", log)
	}

	// The next message continues the session, with the model of the setting.
	request.session, request.model = answer.session, "gpt-6-luna"
	again, err := askAgent(context.Background(), request)
	if err != nil || again.session != answer.session {
		t.Fatalf("second askAgent = %+v, %v", again, err)
	}
	log = calls()
	for _, want := range []string{"ARGS [exec] [resume] [--ignore-user-config]", `[sandbox_mode="read-only"]`, "[-m] [gpt-6-luna]", "[" + answer.session + "] [-]\n"} {
		if !strings.Contains(log, want) {
			t.Errorf("the second call has no %q:\n%s", want, log)
		}
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
	if _, err := askAgent(context.Background(), request); err == nil || !strings.Contains(err.Error(), "codex is not logged in") {
		t.Fatalf("askAgent of a failed agent = %v", err)
	}
	request.prompt = "refuse"
	if _, err := askAgent(context.Background(), request); err == nil || !strings.Contains(err.Error(), "The model is not supported") {
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
}

func TestAskCodexServers(t *testing.T) {
	// Without ai-memory, the agent runs with the Finux server only.
	calls := fakeCodex(t, false)
	folder := t.TempDir()
	request := agentRequest{agent: "codex", prompt: "hello", folder: folder, inbox: folder}
	if _, err := askAgent(context.Background(), request); err != nil {
		t.Fatalf("askAgent without ai-memory = %v", err)
	}
	if log := calls(); !strings.Contains(log, "mcp_servers.finux.command") || strings.Contains(log, "ai-memory") {
		t.Fatalf("call without ai-memory:\n%s", log)
	}

	// Without the Finux server, the agent cannot see the data.
	program := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(program, []byte("#!/bin/sh\necho \"Error: No MCP server named '$3' found.\" >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FINUX_AI_COMMAND", program)
	if _, err := askAgent(context.Background(), request); err == nil || !strings.Contains(err.Error(), "Run 'finux config ai codex'") {
		t.Fatalf("askAgent without the Finux server = %v", err)
	}
}
