// Package setting holds the settings of Finux: the choices of the user
// that change how commands behave. A setting that the user never
// changed has its default value.
package setting

import (
	"errors"
	"fmt"
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
	value = strings.ToLower(strings.TrimSpace(value))
	if word, ok := words[value]; ok && slices.Contains(definition.Values, word) {
		value = word
	}
	if !slices.Contains(definition.Values, value) {
		return fmt.Errorf("unknown value %q for %s: use one of %s", value, key, strings.Join(definition.Values, ", "))
	}
	return s.db.Save(&Setting{Key: key, Value: value}).Error
}
