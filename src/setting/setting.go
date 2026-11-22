// Package setting holds the settings of Finux: the choices of the user
// that change how commands behave. A setting that the user never
// changed has its default value.
package setting

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
)

// The names of the settings.
const (
	// AutomaticSchedules lets the schedules that are automatic create
	// their transactions without a confirmation.
	AutomaticSchedules = "automatic-schedules"
	// AIProvider is the AI provider that Finux uses by default.
	AIProvider = "ai-provider"
	// TelegramToken is the token of the Telegram bot, from @BotFather.
	TelegramToken = "telegram-token"
	// TelegramReminders lets the daemon send reminders in Telegram.
	TelegramReminders = "telegram-reminders"
	// TelegramResume tells how often the daemon sends the resume, at the
	// time in TelegramResumeTime.
	TelegramResume     = "telegram-resume"
	TelegramResumeTime = "telegram-resume-time"
	// DaemonMCP is the address where the daemon serves the MCP server.
	DaemonMCP = "daemon-mcp"
)

// The values of TelegramResume, after Off.
const (
	Daily   = "daily"
	Monthly = "monthly"
)

// The values of a setting that is on or off.
const (
	Off = "off"
	On  = "on"
)

// ErrUnknown means no setting has the name.
var ErrUnknown = errors.New("unknown setting")

// Definition describes one setting.
type Definition struct {
	Key, Title, Description string
	// Values lists the values the setting can have. Default is the first.
	Values  []string
	Default string
	// Check is for a setting with free text: it has no Values. It
	// returns the text as the store keeps it, or tells what is wrong.
	Check func(text string) (string, error)
	// Secret is a setting that Finux does not show, for example a token.
	Secret bool
}

// Text reports whether the setting is free text.
func (d Definition) Text() bool { return d.Check != nil }

// Show returns a value of the setting as a command can print it. A
// secret shows only that it has a value.
func (d Definition) Show(value string) string {
	switch {
	case !d.Secret:
		return value
	case value == "":
		return "(not set)"
	}
	return "(set)"
}

var tokenPattern = regexp.MustCompile(`^\d+:[A-Za-z0-9_-]{30,}$`)

func checkToken(text string) (string, error) {
	if text != "" && !tokenPattern.MatchString(text) {
		return "", errors.New("give the token that @BotFather shows, for example 123456789:AAH...")
	}
	return text, nil
}

func checkTime(text string) (string, error) {
	parsed, err := time.Parse("15:04", text)
	if err != nil {
		return "", errors.New("give a time as HH:MM, for example 08:00")
	}
	return parsed.Format("15:04"), nil
}

func checkAddress(text string) (string, error) {
	if text == "" {
		return "", nil
	}
	if host, port, err := net.SplitHostPort(text); err != nil || host == "" || port == "" {
		return "", errors.New("give an address as host:port, for example 127.0.0.1:8765")
	}
	return text, nil
}

// Definitions returns the settings, in display order.
func Definitions() []Definition {
	return []Definition{
		{
			Key: AutomaticSchedules, Title: "Automatic schedules", Values: []string{Off, On}, Default: Off,
			Description: "On: a schedule with --auto creates its transactions on their dates, without your confirmation. Off: no schedule creates a transaction by itself.",
		},
		{
			Key: AIProvider, Title: "AI provider", Values: []string{"none", "anthropic", "openai", "google", "ollama"}, Default: "none",
			Description: "The AI provider that Finux uses by default.",
		},
		{
			Key: TelegramToken, Title: "Telegram token", Check: checkToken, Secret: true,
			Description: "The token of your Telegram bot, from @BotFather. The daemon starts the bot when the token is set.",
		},
		{
			Key: TelegramReminders, Title: "Telegram reminders", Values: []string{Off, On}, Default: Off,
			Description: "On: the daemon tells the allowed people in Telegram about schedules that are due, bills that close or are late, and budgets near their limit.",
		},
		{
			Key: TelegramResume, Title: "Telegram resume", Values: []string{Off, Daily, Monthly}, Default: Off,
			Description: "Daily or monthly: the daemon sends the resume to the allowed people in Telegram. Monthly is on day 1.",
		},
		{
			Key: TelegramResumeTime, Title: "Telegram resume time", Check: checkTime, Default: "08:00",
			Description: "The time of the day, as HH:MM, when the daemon sends the resume.",
		},
		{
			Key: DaemonMCP, Title: "Daemon MCP address", Check: checkAddress,
			Description: "An address as host:port: the daemon serves the MCP server there. Empty: the daemon does not serve it.",
		},
	}
}

// find returns the definition of a setting.
func find(key string) (Definition, error) {
	definitions := Definitions()
	if index := slices.IndexFunc(definitions, func(d Definition) bool { return d.Key == key }); index >= 0 {
		return definitions[index], nil
	}
	keys := make([]string, len(definitions))
	for i, definition := range definitions {
		keys[i] = definition.Key
	}
	return Definition{}, fmt.Errorf("%w %q: use one of %s", ErrUnknown, key, strings.Join(keys, ", "))
}

// words are other ways to write on and off.
var words = map[string]string{"true": On, "yes": On, "1": On, "false": Off, "no": Off, "0": Off}

// Setting is the value of one setting that the user changed.
type Setting struct {
	Key       string `gorm:"primaryKey"`
	Value     string
	UpdatedAt time.Time
}

// Store reads and writes settings in the database.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store that uses db.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// All returns the value of each setting.
func (s *Store) All() (map[string]string, error) {
	values := map[string]string{}
	for _, definition := range Definitions() {
		values[definition.Key] = definition.Default
	}
	var saved []Setting
	if err := s.db.Find(&saved).Error; err != nil {
		return nil, err
	}
	for _, current := range saved {
		if _, known := values[current.Key]; known {
			values[current.Key] = current.Value
		}
	}
	return values, nil
}

// Get returns the value of a setting.
func (s *Store) Get(key string) (string, error) {
	if _, err := find(key); err != nil {
		return "", err
	}
	values, err := s.All()
	return values[key], err
}

// Enabled reports whether a setting is on. It is false for a setting
// that cannot be read.
func (s *Store) Enabled(key string) bool {
	value, err := s.Get(key)
	return err == nil && value == On
}

// Set validates and saves the value of a setting.
func (s *Store) Set(key, value string) error {
	definition, err := find(key)
	if err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if definition.Text() {
		if value == "" {
			value = definition.Default
		}
		if value, err = definition.Check(value); err != nil {
			return fmt.Errorf("unknown value for %s: %w", key, err)
		}
		return s.db.Save(&Setting{Key: key, Value: value}).Error
	}
	value = strings.ToLower(value)
	if word, ok := words[value]; ok && slices.Contains(definition.Values, word) {
		value = word
	}
	if !slices.Contains(definition.Values, value) {
		return fmt.Errorf("unknown value %q for %s: use one of %s", value, key, strings.Join(definition.Values, ", "))
	}
	return s.db.Save(&Setting{Key: key, Value: value}).Error
}
