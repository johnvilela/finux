package ui

import (
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
