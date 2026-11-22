package main

import (
	"net"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// messages returns the texts of the next count messages of the bot,
// in alphabetical order: the order of the reminders is not a rule.
func (f *fakeTelegram) messages(count int) []string {
	f.t.Helper()
	texts := make([]string, count)
	for i := range texts {
		texts[i] = f.next().text
	}
	sort.Strings(texts)
	return texts
}

// remindersHome makes a database for the reminders: a credit card with
// a purchase, a schedule that is due on 17/11/2026 and a budget near
// its limit.
func remindersHome(t *testing.T) string {
	t.Helper()
	home := botHome(t)
	at := func(args ...string) { on(t, home, "2026-11-17", args...) }
	at("cc", "add", "--name", "Visa Gold", "--code", "VISA", "--currency", "BRL", "--limit", "5000", "--closing", "25", "--due", "5")
	at("tx", "add", "--title", "Shoes", "--value", "300", "--account", "VISA", "--category", "OTHER", "--type", "expense", "--date", "17/11/2026")
	at("sc", "add", "--title", "Rent", "--code", "RENT", "--value", "900", "--account", "INTER", "--category", "OTHER", "--type", "expense", "--date", "17/11/2026", "--every", "month")
	at("bg", "add", "--title", "Food", "--code", "FOOD", "--limit", "50", "--currency", "BRL", "--categories", "FOOD", "--every", "month")
	at("tx", "add", "--title", "Market", "--value", "45", "--account", "INTER", "--category", "FOOD", "--date", "17/11/2026")
	return home
}

func TestDaemonSendsReminders(t *testing.T) {
	home, f := remindersHome(t), newFakeTelegram(t)
	// The reminders are off by default.
	stop := startDaemon(t, home, f)
	f.quiet()
	stop()

	succeed(t, home, "config", "set", "telegram-reminders", "on")
	stop = startDaemon(t, home, f)
	got := f.messages(2)
	want := []string{
		"Budget FOOD: 90% spent, R$ 5,00 left until 30/11/2026",
		"Schedule RENT (Rent) is due since 17/11/2026. Send /sc pay RENT or /sc skip RENT.",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("reminders = %q", got)
	}
	// Each reminder comes one time, also after a restart.
	f.quiet()
	stop()
	stop = startDaemon(t, home, f)
	f.quiet()
	stop()

	// The bill closes on 25/11/2026 and is due on 05/12/2026.
	stop = startDaemon(t, home, f, "FINUX_TODAY=2026-11-26")
	if got := f.next().text; got != "The bill 12/2026 of VISA closed: R$ 300,00. It is due on 05/12/2026. Send /cc pay VISA." {
		t.Fatalf("reminder of a closed bill = %q", got)
	}
	f.quiet()
	stop()
	stop = startDaemon(t, home, f, "FINUX_TODAY=2026-12-04")
	if got := f.next().text; got != "The bill 12/2026 of VISA is due tomorrow: R$ 300,00. Send /cc pay VISA." {
		t.Fatalf("reminder of a bill that is due = %q", got)
	}
	f.quiet()
	stop()
	stop = startDaemon(t, home, f, "FINUX_TODAY=2026-12-06")
	if got := f.next().text; got != "The bill 12/2026 of VISA is late: R$ 300,00 was due on 05/12/2026. Send /cc pay VISA." {
		t.Fatalf("reminder of a late bill = %q", got)
	}
	f.quiet()
	stop()
}

func TestDaemonPaysAutomaticSchedules(t *testing.T) {
	home := botHome(t)
	on(t, home, "2026-11-17", "sc", "add", "--title", "Rent", "--code", "RENT", "--value", "900", "--account", "INTER", "--category", "OTHER", "--type", "expense", "--date", "17/11/2026", "--auto")
	succeed(t, home, "config", "set", "automatic-schedules", "on")
	succeed(t, home, "config", "set", "telegram-reminders", "on")
	f := newFakeTelegram(t)
	stop := startDaemon(t, home, f)
	const paid = "✓ Schedule RENT paid: transaction 1 created (17/11/2026, -R$ 900,00)"
	if got := f.next().text; got != paid {
		t.Fatalf("message = %q", got)
	}
	f.quiet()
	if log := stop(); !strings.Contains(log, paid+"\n") || strings.Contains(log, "\x1b") {
		t.Fatalf("log:\n%s", log)
	}
	if got := on(t, home, "2026-11-17", "tx"); strings.Count(got, "Rent") != 1 {
		t.Fatalf("transactions:\n%s", got)
	}
}

func TestDaemonSendsTheResume(t *testing.T) {
	home, f := botHome(t), newFakeTelegram(t)
	succeed(t, home, "config", "telegram", "allow", "900", "Wife")
	succeed(t, home, "config", "set", "telegram-resume-time", "00:00")

	// Off by default, and monthly only on day 1.
	stop := startDaemon(t, home, f)
	f.quiet()
	stop()
	succeed(t, home, "config", "set", "telegram-resume", "monthly")
	stop = startDaemon(t, home, f)
	f.quiet()
	stop()

	succeed(t, home, "config", "set", "telegram-resume", "daily")
	stop = startDaemon(t, home, f)
	first, second := f.next(), f.next()
	if first.chat+second.chat != 942 || first.text != second.text {
		t.Fatalf("the two people did not get the same resume: %+v, %+v", first, second)
	}
	want := on(t, home, "2026-11-17", "rs")
	if first.text != pre(strings.TrimRight(want, "\n")) {
		t.Fatalf("resume = %q, want the output of 'finux rs':\n%s", first.text, want)
	}
	f.quiet()
	stop()
	// One resume for each day.
	stop = startDaemon(t, home, f)
	f.quiet()
	stop()
	// It is too early for a time that did not come.
	succeed(t, home, "config", "set", "telegram-resume-time", "23:59")
	if time.Now().Format("15:04") < "23:59" {
		stop = startDaemon(t, home, f, "FINUX_TODAY=2026-11-18")
		f.quiet()
		stop()
	}
	succeed(t, home, "config", "set", "telegram-resume-time", "00:00")
	succeed(t, home, "config", "set", "telegram-resume", "monthly")
	startDaemon(t, home, f, "FINUX_TODAY=2026-12-01")
	if got := f.next().text; !strings.HasPrefix(got, "<pre>") {
		t.Fatalf("monthly resume = %q", got)
	}
}

func TestDaemonServesMCP(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	succeed(t, home, "init")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	succeed(t, home, "config", "set", "daemon-mcp", address)
	stop := startDaemon(t, home, nil)
	const request = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"account_list","arguments":{}}}`
	var status int
	var body string
	for range 100 {
		if connection, err := net.Dial("tcp", address); err == nil {
			connection.Close()
			status, body = send(t, "http://"+address+"/mcp", "", request)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status != 200 || !strings.Contains(body, "accounts") {
		t.Fatalf("MCP answer = %d %q", status, body)
	}
	if log := stop(); !strings.Contains(log, "Finux MCP server listens on http://"+address+"/mcp") {
		t.Fatalf("log:\n%s", log)
	}
}
