package main

import (
	"path/filepath"
	"strings"
	"testing"
)

const testToken = "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw_"

func TestConfigTelegram(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	fail(t, home, 1, "Run 'finux init'", "config", "telegram")
	succeed(t, home, "init")
	const empty = "Token    not set. Run 'finux config telegram token <token>'.\n" +
		"Allowed  nobody. Run 'finux config telegram invite [name]'.\n"
	if got := succeed(t, home, "config", "telegram"); got != empty {
		t.Fatalf("config telegram = %q", got)
	}
	if got := succeed(t, home, "config", "telegram", "token", testToken); got != "✓ Telegram token saved\n" {
		t.Fatalf("token = %q", got)
	}
	if got := succeed(t, home, "config", "telegram", "allow", "900", "My", "wife"); got != "✓ 900 (My wife) can use the bot\n" {
		t.Fatalf("allow = %q", got)
	}
	if got := succeed(t, home, "config", "telegram", "allow", "42"); got != "✓ 42 can use the bot\n" {
		t.Fatalf("allow without name = %q", got)
	}
	// A code comes from the bot: see TestBotRefusesPeopleWhoAreNotAllowed.
	fail(t, home, 2, "config telegram: pair: give the code that the bot showed", "config", "telegram", "pair")
	const list = "Token    set\n" +
		"Allowed  42\n" +
		"         900  My wife\n"
	if got := succeed(t, home, "config", "telegram"); got != list {
		t.Fatalf("config telegram = %q", got)
	}
	if got := succeed(t, home, "config", "telegram", "deny", "42"); got != "✓ 42 cannot use the bot\n" {
		t.Fatalf("deny = %q", got)
	}
	if got := succeed(t, home, "config", "telegram"); strings.Contains(got, "42") {
		t.Fatalf("config telegram after deny = %q", got)
	}
	if got := succeed(t, home, "config", "telegram", "--help"); !strings.Contains(got, "finux config telegram") || !strings.Contains(got, "@BotFather") {
		t.Fatalf("help = %q", got)
	}
	fail(t, home, 1, "42 is not in the list of allowed people", "config", "telegram", "deny", "42")
	fail(t, home, 2, `config telegram: invalid user ID "wife": use the number that the bot shows`, "config", "telegram", "allow", "wife")
	fail(t, home, 2, "config telegram: allow: give the Telegram user ID", "config", "telegram", "allow")
	fail(t, home, 2, "config telegram: unknown value for telegram-token", "config", "telegram", "token", "hello")
	fail(t, home, 2, "config telegram: unknown command: nope", "config", "telegram", "nope")
}
