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

func TestDefinitions(t *testing.T) {
	definitions := Definitions()
	if len(definitions) != 2 || definitions[0].Key != AutomaticSchedules || definitions[1].Key != AIProvider {
		t.Fatalf("definitions = %+v", definitions)
	}
	for _, definition := range definitions {
		if definition.Title == "" || definition.Description == "" || len(definition.Values) < 2 || definition.Values[0] != definition.Default {
			t.Errorf("definition %+v: the first value must be the default", definition)
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
	if err != nil || all[AutomaticSchedules] != Off || all[AIProvider] != "none" || len(all) != 2 {
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
	if err := store.Set("color", "red"); !errors.Is(err, ErrUnknown) || !strings.Contains(err.Error(), "automatic-schedules, ai-provider") {
		t.Fatalf("Set of an unknown setting = %v", err)
	}
	if _, err := store.Get("color"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Get of an unknown setting = %v", err)
	}
	if value, _ := store.Get(AIProvider); value != "anthropic" {
		t.Fatalf("provider after refused changes = %q", value)
	}
}
