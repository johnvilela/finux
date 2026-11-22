package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const token = "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw_"

// call is one request that the fake API got.
type call struct {
	method string
	body   map[string]any
}

// fakeAPI is a Telegram Bot API that answers each method with the JSON
// text in answers and keeps the requests.
func fakeAPI(t *testing.T, answers map[string]string) (*Client, *[]call) {
	t.Helper()
	var calls []call
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, ok := strings.CutPrefix(r.URL.Path, "/bot"+token+"/")
		if !ok || r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request %s %s with content type %q", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("%s: body is not JSON: %q", method, raw)
		}
		calls = append(calls, call{method, body})
		answer, known := answers[method]
		if !known {
			answer = `{"ok":true,"result":true}`
		}
		if strings.Contains(answer, `"ok":false`) {
			w.WriteHeader(http.StatusForbidden)
		}
		io.WriteString(w, answer)
	}))
	t.Cleanup(server.Close)
	t.Setenv("FINUX_TELEGRAM_API", server.URL)
	return NewClient(token), &calls
}

func TestGetUpdates(t *testing.T) {
	client, calls := fakeAPI(t, map[string]string{"getUpdates": `{"ok":true,"result":[
		{"update_id":7,"message":{"message_id":3,"from":{"id":42,"first_name":"John"},"chat":{"id":42,"type":"private"},"text":"/ac"}},
		{"update_id":8,"callback_query":{"id":"cb1","from":{"id":42,"first_name":"John"},"message":{"message_id":5,"chat":{"id":42,"type":"private"}},"data":"yes"}}
	]}`})
	updates, err := client.GetUpdates(context.Background(), 7)
	if err != nil || len(updates) != 2 {
		t.Fatalf("GetUpdates = %+v, %v", updates, err)
	}
	if message := updates[0].Message; updates[0].ID != 7 || message == nil || message.Text != "/ac" || message.From.ID != 42 || message.Chat.Type != "private" || message.Chat.ID != 42 {
		t.Fatalf("first update = %+v", updates[0])
	}
	if callback := updates[1].Callback; callback == nil || callback.ID != "cb1" || callback.Data != "yes" || callback.From.ID != 42 || callback.Message.ID != 5 {
		t.Fatalf("second update = %+v", updates[1])
	}
	body := (*calls)[0].body
	if body["offset"] != float64(7) || body["timeout"] != float64(50) {
		t.Fatalf("getUpdates body = %v", body)
	}
}

func TestSendMessage(t *testing.T) {
	client, calls := fakeAPI(t, nil)
	keyboard := [][]Button{{{Text: "Confirm", Data: "yes"}, {Text: "Cancel", Data: "no"}}}
	if err := client.SendMessage(context.Background(), 42, "<pre>hello</pre>", keyboard); err != nil {
		t.Fatal(err)
	}
	if err := client.SendMessage(context.Background(), 42, "plain", nil); err != nil {
		t.Fatal(err)
	}
	if err := client.EditMessage(context.Background(), 42, 5, "done", nil); err != nil {
		t.Fatal(err)
	}
	if err := client.AnswerCallback(context.Background(), "cb1"); err != nil {
		t.Fatal(err)
	}
	if err := client.SetCommands(context.Background(), []Command{{Name: "tx", Description: "Transactions"}}); err != nil {
		t.Fatal(err)
	}
	got := *calls
	if len(got) != 5 || got[0].method != "sendMessage" || got[2].method != "editMessageText" || got[3].method != "answerCallbackQuery" || got[4].method != "setMyCommands" {
		t.Fatalf("calls = %+v", got)
	}
	first := got[0].body
	if first["chat_id"] != float64(42) || first["text"] != "<pre>hello</pre>" || first["parse_mode"] != "HTML" {
		t.Fatalf("sendMessage body = %v", first)
	}
	markup, _ := json.Marshal(first["reply_markup"])
	if string(markup) != `{"inline_keyboard":[[{"callback_data":"yes","text":"Confirm"},{"callback_data":"no","text":"Cancel"}]]}` {
		t.Fatalf("reply_markup = %s", markup)
	}
	if _, has := got[1].body["reply_markup"]; has {
		t.Fatalf("a message without buttons has reply_markup: %v", got[1].body)
	}
	if got[2].body["message_id"] != float64(5) || got[3].body["callback_query_id"] != "cb1" {
		t.Fatalf("edit = %v, answer = %v", got[2].body, got[3].body)
	}
	commands, _ := json.Marshal(got[4].body["commands"])
	if string(commands) != `[{"command":"tx","description":"Transactions"}]` {
		t.Fatalf("commands = %s", commands)
	}
}

func TestErrorsDoNotShowTheToken(t *testing.T) {
	client, _ := fakeAPI(t, map[string]string{"sendMessage": `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`})
	err := client.SendMessage(context.Background(), 42, "hello", nil)
	if err == nil || err.Error() != "Telegram answered 403: Forbidden: bot was blocked by the user" {
		t.Fatalf("error = %v", err)
	}
	// A network error has the URL of the request, with the token in it.
	t.Setenv("FINUX_TELEGRAM_API", "http://127.0.0.1:1")
	err = NewClient(token).SendMessage(context.Background(), 42, "hello", nil)
	if err == nil || strings.Contains(err.Error(), token) || !strings.Contains(err.Error(), "bot<token>") {
		t.Fatalf("network error = %v", err)
	}
}

func TestMessageWithAFile(t *testing.T) {
	client, _ := fakeAPI(t, map[string]string{"getUpdates": `{"ok":true,"result":[
		{"update_id":1,"message":{"message_id":3,"from":{"id":42},"chat":{"id":42,"type":"private"},"caption":"/ai import this",
			"document":{"file_id":"DOC1","file_name":"bill.pdf","mime_type":"application/pdf","file_size":2048}}},
		{"update_id":2,"message":{"message_id":4,"from":{"id":42},"chat":{"id":42,"type":"private"},
			"photo":[{"file_id":"SMALL","file_size":100,"width":90,"height":60},{"file_id":"BIG","file_size":900,"width":900,"height":600}]}}
	]}`})
	updates, err := client.GetUpdates(context.Background(), 0)
	if err != nil || len(updates) != 2 {
		t.Fatalf("GetUpdates = %+v, %v", updates, err)
	}
	document := updates[0].Message
	if file, ok := document.File(); !ok || file.ID != "DOC1" || file.Name != "bill.pdf" || file.Size != 2048 || document.Caption != "/ai import this" {
		t.Fatalf("document = %+v", document)
	}
	// A photo comes in some sizes: the file is the largest one.
	if file, ok := updates[1].Message.File(); !ok || file.ID != "BIG" || file.Name != "photo.jpg" || file.Size != 900 {
		t.Fatalf("photo = %+v", updates[1].Message)
	}
	if _, ok := (&Message{Text: "hello"}).File(); ok {
		t.Fatal("a text message has a file")
	}
}

func TestDownload(t *testing.T) {
	content := strings.Repeat("statement ", 1000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/bot"+token+"/getFile":
			raw, _ := io.ReadAll(r.Body)
			switch {
			case strings.Contains(string(raw), `"GONE"`):
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, `{"ok":false,"error_code":400,"description":"Bad Request: file is too big"}`)
			case strings.Contains(string(raw), `"HUGE"`):
				io.WriteString(w, `{"ok":true,"result":{"file_id":"HUGE","file_size":30000000,"file_path":"documents/huge.pdf"}}`)
			default:
				io.WriteString(w, `{"ok":true,"result":{"file_id":"DOC1","file_size":10000,"file_path":"documents/file_7.pdf"}}`)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/file/bot"+token+"/documents/file_7.pdf":
			io.WriteString(w, content)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("FINUX_TELEGRAM_API", server.URL)
	client := NewClient(token)
	target := filepath.Join(t.TempDir(), "inbox", "bill.pdf")
	if err := client.Download(context.Background(), "DOC1", target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != content {
		t.Fatalf("file has %d bytes, %v", len(got), err)
	}
	// The file has the data of the person: only the owner reads it.
	if info, _ := os.Stat(target); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	other := filepath.Join(t.TempDir(), "other.pdf")
	if err := client.Download(context.Background(), "GONE", other); err == nil || !strings.Contains(err.Error(), "file is too big") {
		t.Fatalf("Download of a file that Telegram refuses = %v", err)
	}
	if err := client.Download(context.Background(), "HUGE", other); err == nil || !strings.Contains(err.Error(), "20 MB") {
		t.Fatalf("Download of a large file = %v", err)
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatal("a failed download left a file")
	}
}

func TestSendTextAndAction(t *testing.T) {
	client, calls := fakeAPI(t, nil)
	if err := client.SendText(context.Background(), 42, "2 < 3 & <b>not bold</b>"); err != nil {
		t.Fatal(err)
	}
	if err := client.SendAction(context.Background(), 42, "typing"); err != nil {
		t.Fatal(err)
	}
	got := *calls
	if _, has := got[0].body["parse_mode"]; has || got[0].method != "sendMessage" || got[0].body["text"] != "2 < 3 & <b>not bold</b>" {
		t.Fatalf("SendText body = %v", got[0].body)
	}
	if got[1].method != "sendChatAction" || got[1].body["action"] != "typing" || got[1].body["chat_id"] != float64(42) {
		t.Fatalf("SendAction = %+v", got[1])
	}
}

func TestGetMe(t *testing.T) {
	client, calls := fakeAPI(t, map[string]string{"getMe": `{"ok":true,"result":{"id":5,"is_bot":true,"first_name":"Finux","username":"my_finux_bot"}}`})
	me, err := client.GetMe(context.Background())
	if err != nil || me.Username != "my_finux_bot" || me.ID != 5 {
		t.Fatalf("GetMe = %+v, %v", me, err)
	}
	if got := *calls; len(got) != 1 || got[0].method != "getMe" {
		t.Fatalf("calls = %+v", got)
	}
}
