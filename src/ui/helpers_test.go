package ui

import (
	"regexp"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"finux/src/account"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;:]*[A-Za-z]`)

func plain(text string) string { return ansiPattern.ReplaceAllString(text, "") }

var (
	keyTab      = tea.KeyPressMsg{Code: tea.KeyTab}
	keyShiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	keyEnter    = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEscape   = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyBack     = tea.KeyPressMsg{Code: tea.KeyBackspace}
	keyLeft     = tea.KeyPressMsg{Code: tea.KeyLeft}
	keyRight    = tea.KeyPressMsg{Code: tea.KeyRight}
	keyUp       = tea.KeyPressMsg{Code: tea.KeyUp}
	keyDown     = tea.KeyPressMsg{Code: tea.KeyDown}
	keySave     = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
)

func text(value string) []tea.Msg {
	var keys []tea.Msg
	for _, r := range value {
		keys = append(keys, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return keys
}

// send feeds messages to a model. It also runs the commands the model
// returns and feeds their results back, except commands that wait, such as
// the cursor blink timer.
func send(t *testing.T, model tea.Model, msgs ...tea.Msg) tea.Model {
	t.Helper()
	for _, msg := range msgs {
		var cmd tea.Cmd
		model, cmd = model.Update(msg)
		model = settle(t, model, cmd, 0)
	}
	return model
}

func settle(t *testing.T, model tea.Model, cmd tea.Cmd, depth int) tea.Model {
	t.Helper()
	if cmd == nil || depth > 8 {
		return model
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-result:
	case <-time.After(20 * time.Millisecond):
		return model
	}
	switch msg := msg.(type) {
	case nil, tea.QuitMsg:
		return model
	case tea.BatchMsg:
		for _, next := range msg {
			model = settle(t, model, next, depth+1)
		}
		return model
	default:
		var next tea.Cmd
		model, next = model.Update(msg)
		return settle(t, model, next, depth+1)
	}
}

func sampleAccounts() []account.Account {
	return []account.Account{
		{ID: 1, Name: "Banco Inter", Description: "main account", Code: "INTER", Color: "orange", Currency: "BRL", OpeningBalance: 123456},
		{ID: 2, Name: "Nubank", Code: "NUBAN", Color: "purple", Currency: "BRL", OpeningBalance: -1005},
		{ID: 3, Name: "Wise", Code: "WISE", Color: "green", Currency: "USD", OpeningBalance: 50000},
	}
}
