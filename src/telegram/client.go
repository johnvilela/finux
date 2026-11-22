package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// apiVariable names the environment variable that replaces the
	// address of the Telegram Bot API, for tests.
	apiVariable = "FINUX_TELEGRAM_API"
	apiAddress  = "https://api.telegram.org"
	// pollSeconds is how long Telegram keeps a getUpdates request open
	// when it has no update.
	pollSeconds = 50
	// MaxMessage is the size limit of the text of a message in characters.
	MaxMessage = 4096
	// MaxFile is the size limit of a file that a bot can get, in bytes.
	MaxFile = 20 << 20
)

// Client calls the Telegram Bot API of one bot.
type Client struct {
	base, token string
	http        *http.Client
}

// NewClient returns a client for the bot with the token.
func NewClient(token string) *Client {
	base := strings.TrimRight(os.Getenv(apiVariable), "/")
	if base == "" {
		base = apiAddress
	}
	return &Client{base: base, token: token, http: &http.Client{Timeout: (pollSeconds + 15) * time.Second}}
}

// Person is a Telegram account.
type Person struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// Chat is where a message is. Type is "private" for a chat between one
// person and the bot.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// Message is a message that the bot got, or the message of the bot
// that has the button a person pressed.
type Message struct {
	ID   int64   `json:"message_id"`
	From *Person `json:"from"`
	Chat Chat    `json:"chat"`
	Text string  `json:"text"`
	// Caption is the text of a message that has a file.
	Caption  string    `json:"caption"`
	Document *Document `json:"document"`
	// Photo has the same picture in some sizes, the smallest first.
	Photo []Photo `json:"photo"`
}

// Document is a file that a person sent as a file.
type Document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

// Photo is one size of a picture that a person sent as a photo.
type Photo struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
}

// File is the file of a message: what Download needs to get it.
type File struct {
	ID, Name string
	Size     int64
}

// File returns the file of the message: its document, or the largest
// size of its photo. It reports false for a message with no file.
func (m *Message) File() (File, bool) {
	switch {
	case m.Document != nil:
		name := m.Document.FileName
		if name == "" {
			name = "file"
		}
		return File{ID: m.Document.FileID, Name: name, Size: m.Document.FileSize}, true
	case len(m.Photo) > 0:
		largest := m.Photo[len(m.Photo)-1]
		return File{ID: largest.FileID, Name: "photo.jpg", Size: largest.FileSize}, true
	}
	return File{}, false
}

// Callback is the press of a button. Data is the data of the button.
type Callback struct {
	ID      string   `json:"id"`
	From    Person   `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

// Update is one event for the bot: a message or the press of a button.
type Update struct {
	ID       int64     `json:"update_id"`
	Message  *Message  `json:"message"`
	Callback *Callback `json:"callback_query"`
}

// Button is a button below a message. Data comes back in the Callback;
// Telegram permits 64 bytes.
type Button struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

// Command is one line of the menu of commands that Telegram shows.
type Command struct {
	Name        string `json:"command"`
	Description string `json:"description"`
}

// call sends one method and decodes its result into result, which can
// be nil.
func (c *Client) call(ctx context.Context, method string, arguments map[string]any, result any) error {
	body, err := json.Marshal(arguments)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/bot"+c.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return c.hide(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return c.hide(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return c.hide(err)
	}
	var answer struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Code        int             `json:"error_code"`
		Description string          `json:"description"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return fmt.Errorf("Telegram answered %s", response.Status)
	}
	if !answer.OK {
		return fmt.Errorf("Telegram answered %d: %s", answer.Code, answer.Description)
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(answer.Result, result)
}

// hide takes the token out of an error. The token is in the address of
// each request, and a network error shows the address.
func (c *Client) hide(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return errors.New(strings.ReplaceAll(err.Error(), c.token, "<token>"))
}

// GetUpdates waits for the updates with the ID offset or more. It
// returns no update when none came in the wait time.
func (c *Client) GetUpdates(ctx context.Context, offset int64) ([]Update, error) {
	var updates []Update
	err := c.call(ctx, "getUpdates", map[string]any{
		"offset": offset, "timeout": pollSeconds, "allowed_updates": []string{"message", "callback_query"},
	}, &updates)
	return updates, err
}

func message(chat int64, html string, keyboard [][]Button) map[string]any {
	arguments := map[string]any{"chat_id": chat, "text": html, "parse_mode": "HTML"}
	if len(keyboard) > 0 {
		arguments["reply_markup"] = map[string]any{"inline_keyboard": keyboard}
	}
	return arguments
}

// SendMessage sends a text in the HTML of Telegram to a chat, with
// rows of buttons below it when keyboard is not empty.
func (c *Client) SendMessage(ctx context.Context, chat int64, html string, keyboard [][]Button) error {
	return c.call(ctx, "sendMessage", message(chat, html, keyboard), nil)
}

// EditMessage replaces the text and the buttons of a message of the bot.
func (c *Client) EditMessage(ctx context.Context, chat, id int64, html string, keyboard [][]Button) error {
	arguments := message(chat, html, keyboard)
	arguments["message_id"] = id
	return c.call(ctx, "editMessageText", arguments, nil)
}

// AnswerCallback tells Telegram that the bot got the press of a button,
// so the button stops its wait animation.
func (c *Client) AnswerCallback(ctx context.Context, id string) error {
	return c.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id}, nil)
}

// SetCommands sets the menu of commands of the bot.
func (c *Client) SetCommands(ctx context.Context, commands []Command) error {
	return c.call(ctx, "setMyCommands", map[string]any{"commands": commands}, nil)
}

// SendText sends a text as it is: Telegram reads no marks in it.
func (c *Client) SendText(ctx context.Context, chat int64, text string) error {
	return c.call(ctx, "sendMessage", map[string]any{"chat_id": chat, "text": text}, nil)
}

// SendAction shows an action of the bot in a chat for some seconds, for
// example "typing".
func (c *Client) SendAction(ctx context.Context, chat int64, action string) error {
	return c.call(ctx, "sendChatAction", map[string]any{"chat_id": chat, "action": action}, nil)
}

// Download gets the file with the ID from Telegram and writes it to the
// path target, which only the owner can read. It makes the folder of
// the file. A failed download leaves no file.
func (c *Client) Download(ctx context.Context, id, target string) error {
	var found struct {
		Path string `json:"file_path"`
		Size int64  `json:"file_size"`
	}
	if err := c.call(ctx, "getFile", map[string]any{"file_id": id}, &found); err != nil {
		return err
	}
	if found.Size > MaxFile || found.Path == "" {
		return errors.New("the file is too large: a bot gets files of 20 MB or less")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/file/bot"+c.token+"/"+found.Path, nil)
	if err != nil {
		return c.hide(err)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return c.hide(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Telegram answered %s for the file", response.Status)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	written, err := io.Copy(file, io.LimitReader(response.Body, MaxFile+1))
	if closed := file.Close(); err == nil {
		err = closed
	}
	if err == nil && written > MaxFile {
		err = errors.New("the file is too large: a bot gets files of 20 MB or less")
	}
	if err != nil {
		os.Remove(target)
		return c.hide(err)
	}
	return nil
}
