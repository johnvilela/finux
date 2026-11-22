package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// on runs finux with the date of today set to the YYYY-MM-DD date.
func on(t *testing.T, home, date string, args ...string) string {
	t.Helper()
	stdout, stderr, status := finuxEnv(t, []string{"FINUX_DB=" + filepath.Join(home, "finux.db"), "HOME=" + home, "FINUX_TODAY=" + date}, args...)
	if status != 0 || stderr != "" {
		t.Fatalf("finux %v on %s: status %d, stderr %q, stdout %q", args, date, status, stderr, stdout)
	}
	return stdout
}

func TestTransactionStatusChangesWithTheDate(t *testing.T) {
	home := ledger(t)
	on(t, home, "2026-11-20", "tx", "add", "--title", "Rent", "--value", "900", "--account", "INTER", "--category", "OTHER", "--type", "expense", "--date", "25/11/2026")
	if card := on(t, home, "2026-11-20", "tx", "1"); !strings.Contains(card, "Status    pending\n") {
		t.Fatalf("card before the date:\n%s", card)
	}
	if card := on(t, home, "2026-11-25", "tx", "1"); !strings.Contains(card, "Status    cleared\n") {
		t.Fatalf("card on the date:\n%s", card)
	}
}

func TestToolsGiveTheStoredStatus(t *testing.T) {
	h := newMCPHome(t)
	h.ok("account_add", map[string]any{"name": "Banco Inter", "code": "INTER", "currency": "BRL"})
	added := h.ok("transaction_add", map[string]any{"title": "Rent", "value": "900", "account": "INTER", "category": "OTHER", "type": "expense", "date": "2026-11-25"})
	if got := at(t, added, "transaction", "status"); got != "pending" {
		t.Fatalf("status of a transaction with a future date = %q", got)
	}
	added = h.ok("transaction_add", map[string]any{"title": "Lunch", "value": "45", "account": "INTER", "category": "FOOD", "date": "2026-11-17"})
	if got := at(t, added, "transaction", "status"); got != "cleared" {
		t.Fatalf("status of a transaction of today = %q", got)
	}
}
