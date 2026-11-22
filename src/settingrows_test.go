package main

import (
	"slices"
	"testing"

	"finux/src/setting"
)

func TestSettingRows(t *testing.T) {
	values := map[string]string{setting.AIProvider: "claude", setting.TelegramToken: "1:secret"}
	rows := settingRows(values)
	// The screen has all the settings, also the text ones.
	if len(rows) != len(setting.Definitions()) {
		t.Fatalf("%d rows for %d settings", len(rows), len(setting.Definitions()))
	}
	for _, row := range rows {
		switch row.Key {
		case setting.TelegramToken:
			if !row.Secret || row.Value != "1:secret" || row.Check == nil {
				t.Errorf("token row = %+v", row)
			}
		case setting.AIModel:
			// The models are the ones of the agent on the screen.
			if got := row.Options(values); !slices.Equal(got, setting.Models("claude")) {
				t.Errorf("models of claude = %v", got)
			}
			if got := row.Options(map[string]string{setting.AIProvider: "codex"}); !slices.Equal(got, setting.Models("codex")) {
				t.Errorf("models of codex = %v", got)
			}
		case setting.AIProvider:
			if len(row.Values) == 0 || row.Options != nil {
				t.Errorf("provider row = %+v", row)
			}
		}
	}
}
