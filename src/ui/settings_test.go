package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func settings(t *testing.T, model tea.Model) Settings {
	t.Helper()
	s, ok := model.(Settings)
	if !ok {
		t.Fatalf("model is %T, want Settings", model)
	}
	return s
}

func sampleSettings() []SettingRow {
	return []SettingRow{
		{Key: "automatic-schedules", Title: "Automatic schedules", Description: "Schedules create transactions.", Values: []string{"off", "on"}, Value: "off"},
		{Key: "ai-provider", Title: "AI provider", Description: "The provider Finux uses.", Values: []string{"none", "anthropic", "openai"}, Value: "anthropic"},
	}
}

func TestSettingsScreen(t *testing.T) {
	var model tea.Model = NewSettings(sampleSettings())
	view := plain(settings(t, model).View().Content)
	for _, want := range []string{"Settings", "Automatic schedules", "off", "on", "AI provider", "none", "anthropic", "openai", "Schedules create transactions.", "enter save", "esc cancel"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "The provider Finux uses.") {
		t.Errorf("view shows the description of a setting without the focus:\n%s", view)
	}
	// Right and left change the value of the setting with the focus. They
	// stop at the ends. Space goes to the next value and around.
	model = send(t, model, keyRight, keyRight, keyDown, keyLeft, keyLeft)
	if got := settings(t, model).Values(); got["automatic-schedules"] != "on" || got["ai-provider"] != "none" {
		t.Fatalf("values = %v", got)
	}
	if view = plain(settings(t, model).View().Content); !strings.Contains(view, "The provider Finux uses.") {
		t.Errorf("view lacks the description of the setting with the focus:\n%s", view)
	}
	model = send(t, model, append(text(" "), text(" ")...)...)
	model = send(t, model, keyDown, keyUp, keyUp, keyUp)
	model = send(t, model, text(" ")...)
	if got := settings(t, model).Values(); got["automatic-schedules"] != "off" || got["ai-provider"] != "openai" {
		t.Fatalf("values after space = %v", got)
	}
	if settings(t, model).Saved() {
		t.Fatal("saved before enter")
	}
	model = send(t, model, keyEnter)
	if s := settings(t, model); !s.Saved() || s.View().Content != "" {
		t.Fatalf("after enter: saved %v, view %q", s.Saved(), s.View().Content)
	}

	// Escape leaves without a save.
	model = send(t, NewSettings(sampleSettings()), keyRight, keyEscape)
	if settings(t, model).Saved() {
		t.Fatal("escape saved the settings")
	}
	model = send(t, NewSettings(sampleSettings()), keySave)
	if !settings(t, model).Saved() {
		t.Fatal("ctrl+s did not save")
	}
}

// textSettings has a list setting, a text setting with a default and
// options that depend on the list setting, and a secret text setting.
func textSettings() []SettingRow {
	models := map[string][]string{"claude": {"sonnet", "opus"}, "codex": {"gpt-sol"}}
	return []SettingRow{
		{Key: "ai-provider", Title: "AI provider", Values: []string{"none", "claude", "codex"}, Value: "claude"},
		{
			Key: "ai-model", Title: "AI model", Description: "The model.",
			Check: func(text string) (string, error) {
				if strings.Contains(text, " ") {
					return "", errors.New("give a model with no spaces")
				}
				return text, nil
			},
			Options: func(values map[string]string) []string { return models[values["ai-provider"]] },
		},
		{Key: "telegram-token", Title: "Telegram token", Secret: true, Value: "111:OLDSECRET"},
		{Key: "resume-time", Title: "Resume time", Value: "08:00", Check: func(text string) (string, error) {
			if len(text) == 4 {
				text = "0" + text
			}
			return text, nil
		}},
	}
}

func TestSettingsScreenText(t *testing.T) {
	var model tea.Model = NewSettings(textSettings())
	view := plain(settings(t, model).View().Content)
	// An empty text setting shows its default: the first option. A secret
	// never shows its saved value.
	for _, want := range []string{"sonnet (default)", "(set)", "08:00"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "OLDSECRET") {
		t.Fatalf("view shows the secret:\n%s", view)
	}
	// The default follows the list setting.
	model = send(t, model, keyRight)
	if view = plain(settings(t, model).View().Content); !strings.Contains(view, "gpt-sol (default)") {
		t.Errorf("view lacks the default of codex:\n%s", view)
	}
	model = send(t, model, keyLeft, keyDown)
	if view = plain(settings(t, model).View().Content); !strings.Contains(view, "type or paste") {
		t.Errorf("view of a text setting lacks its keys:\n%s", view)
	}
	// Right and left go through the options. The default option is the
	// empty value.
	model = send(t, model, keyRight)
	if got := settings(t, model).Values()["ai-model"]; got != "opus" {
		t.Fatalf("model after right = %q", got)
	}
	model = send(t, model, keyLeft)
	if got := settings(t, model).Values()["ai-model"]; got != "" {
		t.Fatalf("model after left = %q", got)
	}
	// Letters write the value, also the letters that move in a list
	// setting. Backspace deletes one and ctrl+u deletes all.
	model = send(t, model, text("hjk lqx")...)
	model = send(t, model, keyBack)
	if got := settings(t, model).Values()["ai-model"]; got != "hjk lq" {
		t.Fatalf("model after letters = %q", got)
	}
	// Enter does not save a value that is not correct.
	model = send(t, model, keyDown, keyEnter)
	s := settings(t, model)
	if view = plain(s.View().Content); s.Saved() || !strings.Contains(view, "give a model with no spaces") {
		t.Fatalf("saved %v with a wrong model:\n%s", s.Saved(), view)
	}
	model = send(t, model, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	model = send(t, model, text("haiku")...)
	if view = plain(settings(t, model).View().Content); strings.Contains(view, "give a model with no spaces") {
		t.Errorf("the error stays after a change:\n%s", view)
	}
	// A paste writes the token. The screen shows only its end.
	model = send(t, model, keyDown, tea.PasteMsg{Content: " 222:NEWSECRET-abcd\n"})
	if view = plain(settings(t, model).View().Content); strings.Contains(view, "NEWSECRET") || !strings.Contains(view, "abcd") {
		t.Errorf("view of a new secret:\n%s", view)
	}
	model = send(t, model, keyDown, keyBack, keyBack, keyBack, keyBack, keyBack)
	model = send(t, model, text("7:30")...)
	model = send(t, model, keyEnter)
	s = settings(t, model)
	got := s.Values()
	if !s.Saved() || got["ai-model"] != "haiku" || got["telegram-token"] != "222:NEWSECRET-abcd" || got["resume-time"] != "07:30" || got["ai-provider"] != "claude" {
		t.Fatalf("saved %v, values = %v", s.Saved(), got)
	}
}
