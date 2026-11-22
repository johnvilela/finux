package setting

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"finux/src/storage"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := storage.Init(path, &Setting{}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

const allKeys = "automatic-schedules ai-provider telegram-token telegram-reminders telegram-resume telegram-resume-time daemon-mcp"

func TestDefinitions(t *testing.T) {
	definitions := Definitions()
	keys := make([]string, len(definitions))
	for i, definition := range definitions {
		keys[i] = definition.Key
	}
	if got := strings.Join(keys, " "); got != allKeys {
		t.Fatalf("definitions = %q", got)
	}
	for _, definition := range definitions {
		if definition.Title == "" || definition.Description == "" {
			t.Errorf("definition %s: no title or no description", definition.Key)
		}
		// A setting has a list of values or it is free text, not the two.
		if definition.Text() {
			if len(definition.Values) != 0 {
				t.Errorf("definition %s: a text setting has no list of values", definition.Key)
			}
			if _, err := definition.Check(definition.Default); err != nil {
				t.Errorf("definition %s: the default is not valid: %v", definition.Key, err)
			}
		} else if len(definition.Values) < 2 || definition.Values[0] != definition.Default {
			t.Errorf("definition %s: the first value must be the default", definition.Key)
		}
	}
	if got := strings.Join(definitions[1].Values, " "); got != "none anthropic openai google ollama" {
		t.Errorf("providers = %q", got)
	}
}

func TestStore(t *testing.T) {
	store := newStore(t)
	// A setting that was never set has its default value.
	if value, err := store.Get(AutomaticSchedules); err != nil || value != Off {
		t.Fatalf("default = %q, %v", value, err)
	}
	if store.Enabled(AutomaticSchedules) {
		t.Fatal("automatic schedules are on by default")
	}
	all, err := store.All()
	if err != nil || all[AutomaticSchedules] != Off || all[AIProvider] != "none" || len(all) != 7 {
		t.Fatalf("All = %v, %v", all, err)
	}
	for input, want := range map[string]string{"on": On, " ON ": On, "true": On, "yes": On, "off": Off, "false": Off, "no": Off} {
		if err := store.Set(AutomaticSchedules, input); err != nil {
			t.Fatalf("Set(%q): %v", input, err)
		}
		if value, _ := store.Get(AutomaticSchedules); value != want || store.Enabled(AutomaticSchedules) != (want == On) {
			t.Fatalf("after Set(%q): %q", input, value)
		}
	}
	if err := store.Set(AIProvider, "Anthropic"); err != nil {
		t.Fatal(err)
	}
	if value, _ := store.Get(AIProvider); value != "anthropic" {
		t.Fatalf("provider = %q", value)
	}
	if err := store.Set(AIProvider, "skynet"); err == nil || !strings.Contains(err.Error(), `unknown value "skynet" for ai-provider: use one of none, anthropic, openai, google, ollama`) {
		t.Fatalf("Set of an unknown value = %v", err)
	}
	if err := store.Set("color", "red"); !errors.Is(err, ErrUnknown) || !strings.Contains(err.Error(), strings.ReplaceAll(allKeys, " ", ", ")) {
		t.Fatalf("Set of an unknown setting = %v", err)
	}
	if _, err := store.Get("color"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Get of an unknown setting = %v", err)
	}
	if value, _ := store.Get(AIProvider); value != "anthropic" {
		t.Fatalf("provider after refused changes = %q", value)
	}
}

func TestTextSettings(t *testing.T) {
	store := newStore(t)
	if value, err := store.Get(TelegramResumeTime); err != nil || value != "08:00" {
		t.Fatalf("default time = %q, %v", value, err)
	}
	const token = "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw_"
	for key, value := range map[string]string{TelegramToken: " " + token + " ", TelegramResumeTime: "7:05", DaemonMCP: "127.0.0.1:8765", TelegramResume: "Daily"} {
		if err := store.Set(key, value); err != nil {
			t.Fatalf("Set(%s, %q): %v", key, value, err)
		}
	}
	// A text value keeps its letters as they are. A time gets two digits.
	all, err := store.All()
	if err != nil || all[TelegramToken] != token || all[TelegramResumeTime] != "07:05" || all[DaemonMCP] != "127.0.0.1:8765" || all[TelegramResume] != "daily" {
		t.Fatalf("All = %v, %v", all, err)
	}
	for key, value := range map[string]string{TelegramToken: "hello", TelegramResumeTime: "25:00", DaemonMCP: "8765"} {
		if err := store.Set(key, value); err == nil || !strings.HasPrefix(err.Error(), "unknown value") {
			t.Errorf("Set(%s, %q) = %v, want an unknown value error", key, value, err)
		}
	}
	// An empty value puts a text setting back to its default.
	for key, want := range map[string]string{TelegramToken: "", TelegramResumeTime: "08:00", DaemonMCP: ""} {
		if err := store.Set(key, ""); err != nil {
			t.Fatalf("Set(%s, empty): %v", key, err)
		}
		if value, _ := store.Get(key); value != want {
			t.Errorf("%s after an empty value = %q, want %q", key, value, want)
		}
	}
}

func TestSecretSettingsAreNotShown(t *testing.T) {
	for _, definition := range Definitions() {
		want := definition.Key == TelegramToken
		if definition.Secret != want {
			t.Errorf("%s: secret = %v", definition.Key, definition.Secret)
		}
	}
	token, _ := find(TelegramToken)
	if got := token.Show("123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw_"); got != "(set)" {
		t.Errorf("Show of a token = %q", got)
	}
	if got := token.Show(""); got != "(not set)" {
		t.Errorf("Show of no token = %q", got)
	}
	provider, _ := find(AIProvider)
	if got := provider.Show("anthropic"); got != "anthropic" {
		t.Errorf("Show of a provider = %q", got)
	}
}
