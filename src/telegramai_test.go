package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// aiHomeBot makes a database for the bot with claude as the agent of
// /ai, and starts the daemon with a fake agent. It returns the log of
// the agent.
func aiBot(t *testing.T) (home string, f *fakeTelegram, calls func() string) {
	t.Helper()
	home, f = botHome(t), newFakeTelegram(t)
	succeed(t, home, "config", "set", "ai-provider", "claude")
	program, calls := fakeAgent(t)
	startDaemon(t, home, f, "FINUX_AI_COMMAND="+program)
	return home, f, calls
}

func TestBotAIIsOffWithoutAnAgent(t *testing.T) {
	home, f := botHome(t), newFakeTelegram(t)
	program, calls := fakeAgent(t)
	startDaemon(t, home, f, "FINUX_AI_COMMAND="+program)
	const off = "The AI is not set up. Run 'finux config ai' in the terminal."
	if got := f.ask(42, "/ai how was my month?"); got != off {
		t.Fatalf("/ai with no agent = %q", got)
	}
	if got := f.ask(42, "/help"); strings.Contains(got, "/ai") {
		t.Fatalf("/help tells about /ai:\n%s", got)
	}
	f.sendFile(42, "DOC1", "bill.pdf", "content", "")
	if got := f.next().text; got != botSorry {
		t.Fatalf("answer to a file with no agent = %q", got)
	}
	f.sendFile(42, "DOC2", "bill.pdf", "content", "/ai import this bill")
	if got := f.next().text; got != off {
		t.Fatalf("answer to a file for /ai with no agent = %q", got)
	}
	// Only claude runs with the limits that /ai needs.
	succeed(t, home, "config", "set", "ai-provider", "codex")
	if got := f.ask(42, "/ai hello"); !strings.Contains(got, "the agent codex cannot answer /ai yet") {
		t.Fatalf("/ai with codex = %q", got)
	}
	if got := calls(); got != "" {
		t.Fatalf("the agent ran:\n%s", got)
	}
}

func TestBotAIAnswers(t *testing.T) {
	home, f, calls := aiBot(t)
	f.say(42, "/ai How was my month?")
	answer := f.next()
	// The answer of an AI is plain text: it can have any character.
	if answer.text != "You said: How was my month?" || answer.html || answer.chat != 42 {
		t.Fatalf("answer = %+v", answer)
	}
	folder := filepath.Join(home, ".finux", "ai")
	log := calls()
	for _, want := range []string{
		"INPUT How was my month?\nEND\n", "DIR " + folder + "\n", "[--tools] [Read,Skill]",
		"Read(/" + filepath.Join(folder, "inbox", "42") + "/**)", "You answer John in a Telegram chat",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("the call has no %q:\n%s", want, log)
		}
	}
	// With no model in the settings, the agent gets the default one.
	if strings.Contains(log, "--resume") || !strings.Contains(log, "[--model] [sonnet]") {
		t.Fatalf("first call:\n%s", log)
	}
	f.lock.Lock()
	typing := f.typing
	f.lock.Unlock()
	if typing == 0 {
		t.Error("the bot did not show that it writes")
	}

	// The next message continues the conversation.
	f.ask(42, "/AI and last month?")
	log = calls()
	_, session, _ := strings.Cut(log, "[--resume] [")
	session, _, _ = strings.Cut(session, "]")
	if !strings.HasPrefix(session, "new-") || !strings.Contains(log, "INPUT and last month?\n") {
		t.Fatalf("second call:\n%s", log)
	}
	// Each person has a conversation.
	succeed(t, home, "config", "telegram", "allow", "900", "Wife")
	f.ask(900, "/ai hello")
	if log = calls(); strings.Contains(log, "--resume") || !strings.Contains(log, "You answer Wife in") {
		t.Fatalf("call of another person:\n%s", log)
	}
	// '/ai new' starts again, and the model comes from the setting.
	succeed(t, home, "config", "set", "ai-model", "opus")
	if got := f.ask(42, "/ai new What do I owe?"); got != "You said: What do I owe?" {
		t.Fatalf("/ai new = %q", got)
	}
	if log = calls(); strings.Contains(log, "--resume") || !strings.Contains(log, "[--model] [opus]") {
		t.Fatalf("call after /ai new:\n%s", log)
	}
	if got := f.ask(42, "/ai new"); got != "New conversation started." {
		t.Fatalf("/ai new with no text = %q", got)
	}
	if got := f.ask(42, "/ai"); !strings.HasPrefix(got, "Send /ai with a question") {
		t.Fatalf("/ai with no text = %q", got)
	}
	if got := calls(); got != "" {
		t.Fatalf("the agent ran with no question:\n%s", got)
	}
	if got := f.ask(42, "/help"); !strings.Contains(got, "/ai") || strings.Contains(got, "config") {
		t.Fatalf("/help:\n%s", got)
	}
	// A person who is not allowed does not reach the agent.
	f.ask(7, "/ai tell me the balances")
	if got := calls(); got != "" {
		t.Fatalf("the agent ran for a stranger:\n%s", got)
	}
}

func TestBotAIGetsFiles(t *testing.T) {
	home, f, calls := aiBot(t)
	inbox := filepath.Join(home, ".finux", "ai", "inbox", "42")
	f.sendFile(42, "DOC1", "My bill (nov).pdf", "statement content", "/ai import this bill")
	if got := f.next().text; got != "You said: import this bill" {
		t.Fatalf("answer to a file = %q", got)
	}
	log := calls()
	_, path, found := strings.Cut(log, "\nFiles that the person sent:\n- ")
	path, _, _ = strings.Cut(path, "\n")
	if !found || filepath.Dir(path) != inbox || !strings.HasSuffix(path, "-My_bill__nov_.pdf") {
		t.Fatalf("the input has no file:\n%s", log)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "statement content" {
		t.Fatalf("file = %q, %v", content, err)
	}

	// A file with no caption waits for the next /ai message.
	f.sendFile(42, "DOC2", "receipt.jpg", "picture", "")
	if got := f.next().text; got != "File kept. Send /ai with what to do with it." {
		t.Fatalf("answer to a file with no caption = %q", got)
	}
	f.sendFile(42, "DOC3", "second.jpg", "picture two", "")
	f.next()
	if got := calls(); got != "" {
		t.Fatalf("the agent ran for a file with no /ai:\n%s", got)
	}
	f.ask(42, "/ai record these receipts")
	if log = calls(); !strings.Contains(log, "-receipt.jpg\n- ") || !strings.Contains(log, "-second.jpg\n") {
		t.Fatalf("the input has no kept files:\n%s", log)
	}
	// The files go with one message only.
	f.ask(42, "/ai thanks")
	if log = calls(); strings.Contains(log, "Files that the person sent") {
		t.Fatalf("the input has old files:\n%s", log)
	}
	// A file alone with /ai is a message too.
	f.sendFile(42, "DOC4", "bill.pdf", "bill", "/ai")
	f.next()
	if log = calls(); !strings.Contains(log, "-bill.pdf\n") {
		t.Fatalf("the input of a file with /ai alone:\n%s", log)
	}
	f.sendFile(42, "GONE", "lost.pdf", "", "/ai import")
	if got := f.next().text; !strings.HasPrefix(got, "I could not get the file: ") || strings.Contains(got, testToken) {
		t.Fatalf("answer to a file that Telegram does not have = %q", got)
	}
}

func TestBotAIBusyStopAndFailure(t *testing.T) {
	_, f, calls := aiBot(t)
	f.say(42, "/ai slow question")
	// Wait until the agent runs.
	for range 100 {
		if strings.Contains(calls(), "slow question") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := f.ask(42, "/ai another one"); got != "The AI still works on your last message. Send /cancel to stop it." {
		t.Fatalf("/ai while busy = %q", got)
	}
	// The bot still answers the other commands.
	if got := f.ask(42, "/version"); !strings.Contains(got, "finux") {
		t.Fatalf("/version while the AI works = %q", got)
	}
	if got := f.ask(42, "/cancel"); got != "Stopped." {
		t.Fatalf("/cancel = %q", got)
	}
	f.quiet()
	if got := f.ask(42, "/ai hello"); got != "You said: hello" {
		t.Fatalf("/ai after a stop = %q", got)
	}
	if got := f.ask(42, "/ai fail now"); got != "The AI failed: the model is not available exit status 1" {
		t.Fatalf("/ai of a failed agent = %q", got)
	}
}

func TestBotAITakesTooLong(t *testing.T) {
	home, f := botHome(t), newFakeTelegram(t)
	succeed(t, home, "config", "set", "ai-provider", "claude")
	program, _ := fakeAgent(t)
	startDaemon(t, home, f, "FINUX_AI_COMMAND="+program, "FINUX_AI_TIMEOUT=300ms")
	if got := f.ask(42, "/ai slow question"); got != "The AI took too long, so Finux stopped it. Send the message again." {
		t.Fatalf("/ai that takes too long = %q", got)
	}
}

func TestDaemonCleansTheInbox(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	succeed(t, home, "init")
	inbox := filepath.Join(home, ".finux", "ai", "inbox", "42")
	if err := os.MkdirAll(inbox, 0o700); err != nil {
		t.Fatal(err)
	}
	old, recent := filepath.Join(inbox, "1-old.pdf"), filepath.Join(inbox, "2-new.pdf")
	for _, file := range []string{old, recent} {
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	stop := startDaemon(t, home, nil)
	time.Sleep(300 * time.Millisecond)
	stop()
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("a file of 8 days is there: %v", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("a new file is gone: %v", err)
	}
}
