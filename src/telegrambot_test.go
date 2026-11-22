package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// sent is a message that the bot sent or changed.
type sent struct {
	method string
	chat   int64
	id     int64
	text   string
	// html tells that the message has the marks of Telegram.
	html    bool
	buttons []string
	// data has the callback data of each button.
	data []string
}

// fakeTelegram is a Telegram Bot API for the bot of a test.
type fakeTelegram struct {
	t       *testing.T
	server  *httptest.Server
	lock    sync.Mutex
	updates []map[string]any
	nextID  int64
	sent    chan sent
	other   []string
	// files has the content of each file that a person sent, by its ID.
	files map[string]string
	// typing counts the times the bot showed that it writes.
	typing int
}

func newFakeTelegram(t *testing.T) *fakeTelegram {
	t.Helper()
	f := &fakeTelegram{t: t, sent: make(chan sent, 100), nextID: 100, files: map[string]string{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeTelegram) serve(w http.ResponseWriter, r *http.Request) {
	if path, isFile := strings.CutPrefix(r.URL.Path, "/file/bot"+testToken+"/"); isFile {
		f.lock.Lock()
		content, known := f.files[path]
		f.lock.Unlock()
		if !known {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, content)
		return
	}
	method, ok := strings.CutPrefix(r.URL.Path, "/bot"+testToken+"/")
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"ok":false,"error_code":401,"description":"Unauthorized"}`)
		return
	}
	var body struct {
		Chat   int64  `json:"chat_id"`
		ID     int64  `json:"message_id"`
		Text   string `json:"text"`
		Mode   string `json:"parse_mode"`
		FileID string `json:"file_id"`
		Markup struct {
			Keyboard [][]struct {
				Text string `json:"text"`
				Data string `json:"callback_data"`
			} `json:"inline_keyboard"`
		} `json:"reply_markup"`
	}
	raw, _ := io.ReadAll(r.Body)
	json.Unmarshal(raw, &body)
	switch method {
	case "getUpdates":
		// A real server keeps the request open. This one waits a short time.
		var updates []map[string]any
		for range 20 {
			f.lock.Lock()
			updates, f.updates = f.updates, nil
			f.lock.Unlock()
			if len(updates) > 0 {
				break
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
		if updates == nil {
			updates = []map[string]any{}
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": updates})
		return
	case "sendMessage", "editMessageText":
		message := sent{method: method, chat: body.Chat, id: body.ID, text: body.Text, html: body.Mode == "HTML"}
		if method == "sendMessage" {
			f.lock.Lock()
			f.nextID++
			message.id = f.nextID
			f.lock.Unlock()
		}
		for _, row := range body.Markup.Keyboard {
			for _, button := range row {
				message.buttons = append(message.buttons, button.Text)
				message.data = append(message.data, button.Data)
			}
		}
		f.sent <- message
	case "getFile":
		// The path of a file on the server is its ID.
		f.lock.Lock()
		content, known := f.files[body.FileID]
		f.lock.Unlock()
		if !known {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"ok":false,"error_code":400,"description":"Bad Request: wrong file_id"}`)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"file_id": body.FileID, "file_size": len(content), "file_path": body.FileID}})
		return
	case "sendChatAction":
		f.lock.Lock()
		f.typing++
		f.lock.Unlock()
	default:
		f.lock.Lock()
		f.other = append(f.other, method)
		f.lock.Unlock()
	}
	io.WriteString(w, `{"ok":true,"result":true}`)
}

func (f *fakeTelegram) push(update map[string]any) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.nextID++
	update["update_id"] = f.nextID
	f.updates = append(f.updates, update)
}

// say sends a text to the bot in the private chat of a person.
func (f *fakeTelegram) say(user int64, text string) {
	f.push(map[string]any{"message": map[string]any{
		"message_id": 1, "from": map[string]any{"id": user, "first_name": "Person"},
		"chat": map[string]any{"id": user, "type": "private"}, "text": text,
	}})
}

// sendFile sends a document with a caption to the bot. content is what
// the bot gets when it downloads the file; an empty content is a file
// that Telegram does not have.
func (f *fakeTelegram) sendFile(user int64, id, name, content, caption string) {
	f.lock.Lock()
	if content != "" {
		f.files[id] = content
	}
	f.lock.Unlock()
	f.push(map[string]any{"message": map[string]any{
		"message_id": 1, "from": map[string]any{"id": user, "first_name": "Person"},
		"chat": map[string]any{"id": user, "type": "private"}, "caption": caption,
		"document": map[string]any{"file_id": id, "file_name": name, "file_size": len(content)},
	}})
}

// press presses the button with the label below a message of the bot.
func (f *fakeTelegram) press(user int64, message sent, label string) {
	f.t.Helper()
	for i, button := range message.buttons {
		if button == label {
			f.push(map[string]any{"callback_query": map[string]any{
				"id": "cb", "from": map[string]any{"id": user, "first_name": "Person"}, "data": message.data[i],
				"message": map[string]any{"message_id": message.id, "chat": map[string]any{"id": user, "type": "private"}},
			}})
			return
		}
	}
	f.t.Fatalf("no button %q in %v", label, message.buttons)
}

// next returns the next message that the bot sent or changed.
func (f *fakeTelegram) next() sent {
	f.t.Helper()
	select {
	case message := <-f.sent:
		return message
	case <-time.After(10 * time.Second):
		f.t.Fatal("the bot sent no message")
	}
	return sent{}
}

// ask sends a text and returns the text of the answer of the bot.
func (f *fakeTelegram) ask(user int64, text string) string {
	f.t.Helper()
	f.say(user, text)
	answer := f.next()
	if answer.chat != user {
		f.t.Fatalf("the answer to %q went to the chat %d, want %d", text, answer.chat, user)
	}
	return answer.text
}

// quiet checks that the bot sent no message.
func (f *fakeTelegram) quiet() {
	f.t.Helper()
	select {
	case message := <-f.sent:
		f.t.Fatalf("the bot sent a message: %+v", message)
	case <-time.After(300 * time.Millisecond):
	}
}

// startDaemon runs 'finux daemon run' on the database of home until the
// test ends. It returns a function that stops the daemon and gives its
// log.
func startDaemon(t *testing.T, home string, f *fakeTelegram, env ...string) func() string {
	t.Helper()
	command := exec.Command(finuxBinary(t), "daemon", "run")
	command.Env = append([]string{"FINUX_DB=" + filepath.Join(home, "finux.db"), "HOME=" + home, "FINUX_TODAY=2026-11-17", "FINUX_DAEMON_TICK=50ms"}, env...)
	if f != nil {
		command.Env = append(command.Env, "FINUX_TELEGRAM_API="+f.server.URL)
	}
	var log bytes.Buffer
	command.Stdout, command.Stderr = &log, &log
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	stop := func() string {
		once.Do(func() {
			command.Process.Signal(os.Interrupt)
			done := make(chan error, 1)
			go func() { done <- command.Wait() }()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("daemon: %v, log:\n%s", err, log.String())
				}
			case <-time.After(10 * time.Second):
				command.Process.Kill()
				t.Errorf("the daemon did not stop, log:\n%s", log.String())
			}
		})
		return log.String()
	}
	t.Cleanup(func() { stop() })
	return stop
}

// botHome makes a database with accounts, a bot token and one allowed
// person: 42.
func botHome(t *testing.T) string {
	t.Helper()
	home := ledger(t)
	succeed(t, home, "config", "telegram", "token", testToken)
	succeed(t, home, "config", "telegram", "allow", "42", "John")
	return home
}

func pre(text string) string { return "<pre>" + text + "</pre>" }

func TestBotRefusesPeopleWhoAreNotAllowed(t *testing.T) {
	home, f := botHome(t), newFakeTelegram(t)
	startDaemon(t, home, f)
	const refusal = "You have no access to this bot. Your Telegram user ID is 7.\n\nThe owner can give you access with:\n<code>finux config telegram allow 7</code>"
	if got := f.ask(7, "/ac"); got != refusal {
		t.Fatalf("answer to a stranger = %q", got)
	}
	// The bot tells it one time, and runs nothing for the person.
	f.say(7, "/tx add --title Gift --value 10 --account INTER --category OTHER --type expense")
	f.quiet()
	// A group is not a private chat, also for an allowed person.
	f.push(map[string]any{"message": map[string]any{
		"message_id": 1, "from": map[string]any{"id": 42}, "chat": map[string]any{"id": -500, "type": "group"}, "text": "/ac",
	}})
	f.quiet()
	if got := succeed(t, home, "tx"); !strings.Contains(got, "No transactions yet") {
		t.Fatalf("a stranger changed the data:\n%s", got)
	}
	// Access starts with no restart of the daemon.
	succeed(t, home, "config", "telegram", "allow", "7")
	if got := f.ask(7, "/version"); !strings.Contains(got, "finux") {
		t.Fatalf("answer after allow = %q", got)
	}
}

func TestBotRunsCommands(t *testing.T) {
	home, f := botHome(t), newFakeTelegram(t)
	stop := startDaemon(t, home, f)
	if got := f.ask(42, "/ac"); !strings.HasPrefix(got, "<pre>CODE") || !strings.Contains(got, "INTER") || strings.Contains(got, "\x1b") {
		t.Fatalf("/ac = %q", got)
	}
	if got := f.ask(42, `/tx add --title "Weekly market" --value 50,25 --account inter --category food --date 17/11/2026`); got != pre("✓ Transaction 1 created") {
		t.Fatalf("/tx add = %q", got)
	}
	// A phone changes two dashes to a long dash, and quotes to curly ones.
	if got := f.ask(42, "/transactions add —title “Bus ticket” —value 5 —account INTER —category OTHER —type expense"); got != pre("✓ Transaction 2 created") {
		t.Fatalf("/tx add from a phone = %q", got)
	}
	if got := f.ask(42, "/tx 2"); !strings.Contains(got, "Bus ticket") || !strings.Contains(got, "Status    cleared") {
		t.Fatalf("/tx 2 = %q", got)
	}
	// Telegram has no dash in a command, and adds the name of the bot.
	for _, command := range []string{"/cc", "/credit_card", "/credit_cards@finux_bot"} {
		if got := f.ask(42, command); !strings.Contains(got, "No credit cards yet") {
			t.Fatalf("%s = %q", command, got)
		}
	}
	// A message shows the special characters of HTML as they are.
	f.ask(42, `/tx add --title "A <b> & c" --value 1 --account INTER --category OTHER --type expense`)
	if got := f.ask(42, "/tx 3"); !strings.Contains(got, "A &lt;b&gt; &amp; c") {
		t.Fatalf("/tx 3 = %q", got)
	}
	// An error of the command line comes back as it is.
	if got := f.ask(42, "/tx 99"); got != pre("finux: transaction not found: 99") {
		t.Fatalf("/tx 99 = %q", got)
	}
	if got := f.ask(42, `/tx add --title "no end`); got != "A quote in the message has no end." {
		t.Fatalf("open quote = %q", got)
	}
	log := stop()
	if !strings.Contains(log, "Telegram bot is on") || strings.Contains(log, testToken) {
		t.Fatalf("log:\n%s", log)
	}
	f.lock.Lock()
	defer f.lock.Unlock()
	if !reflect.DeepEqual(f.other, []string{"setMyCommands"}) {
		t.Fatalf("other calls = %v", f.other)
	}
}

func TestBotDoesNotUnderstand(t *testing.T) {
	home, f := botHome(t), newFakeTelegram(t)
	startDaemon(t, home, f)
	const sorry = "Sorry, I did not understand. Send /help for the commands."
	for _, text := range []string{"hello", "how much did I spend?", "/nope", "/"} {
		if got := f.ask(42, text); got != sorry {
			t.Errorf("answer to %q = %q", text, got)
		}
	}
	for _, command := range []string{"config", "init", "update", "daemon", "seed", "reseed"} {
		want := fmt.Sprintf("The command /%s is not available in Telegram. Use it in the terminal.", command)
		if got := f.ask(42, "/"+command+" get"); got != want {
			t.Errorf("answer to /%s = %q", command, got)
		}
	}
	for _, command := range []string{"/help", "/start"} {
		got := f.ask(42, command)
		for _, want := range []string{"/ac", "/ct", "/tx", "/cc", "/gl", "/bg", "/sc", "/rs", "/cancel", "--help"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s has no %s:\n%s", command, want, got)
			}
		}
		if strings.Contains(got, "config") {
			t.Errorf("%s tells about config:\n%s", command, got)
		}
	}
}

func TestBotAsksBeforeADelete(t *testing.T) {
	home, f := botHome(t), newFakeTelegram(t)
	startDaemon(t, home, f)
	f.ask(42, "/tx add --title Lunch --value 45 --account INTER --category FOOD")
	f.ask(42, "/tx add --title Dinner --value 60 --account INTER --category FOOD")

	f.say(42, "/tx delete 1")
	question := f.next()
	if question.text != "This deletes data and you cannot get it back:\n<code>/tx delete 1</code>" || !reflect.DeepEqual(question.buttons, []string{"Confirm", "Cancel"}) {
		t.Fatalf("question = %+v", question)
	}
	f.press(42, question, "Cancel")
	if changed := f.next(); changed.method != "editMessageText" || changed.id != question.id || changed.text != "Cancelled:\n<code>/tx delete 1</code>" || len(changed.buttons) != 0 {
		t.Fatalf("after Cancel = %+v", changed)
	}
	if got := f.ask(42, "/tx 1"); !strings.Contains(got, "Lunch") {
		t.Fatalf("the transaction is gone after Cancel: %q", got)
	}

	f.say(42, "/tx delete 1")
	question = f.next()
	// Another person cannot press the button.
	succeed(t, home, "config", "telegram", "allow", "900")
	f.press(900, question, "Confirm")
	f.quiet()
	f.press(42, question, "Confirm")
	if changed := f.next(); changed.method != "editMessageText" || changed.text != "Confirmed:\n<code>/tx delete 1</code>" || len(changed.buttons) != 0 {
		t.Fatalf("after Confirm = %+v", changed)
	}
	if result := f.next(); result.text != pre("✓ Transaction 1 deleted") {
		t.Fatalf("result = %+v", result)
	}
	// A second press does nothing.
	f.press(42, question, "Confirm")
	f.quiet()
	// With --yes there is no question.
	if got := f.ask(42, "/tx delete 2 --yes"); got != pre("✓ Transaction 2 deleted") {
		t.Fatalf("/tx delete --yes = %q", got)
	}
}

func TestDaemonRunsWithoutABot(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	fail(t, home, 1, "Run 'finux init'", "daemon", "run")
	succeed(t, home, "init")
	stop := startDaemon(t, home, nil)
	time.Sleep(300 * time.Millisecond)
	if log := stop(); !strings.Contains(log, "Finux daemon started") || !strings.Contains(log, "Telegram bot is off: no token") || !strings.Contains(log, "Finux daemon stopped") {
		t.Fatalf("log:\n%s", log)
	}
}
