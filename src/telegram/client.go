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
