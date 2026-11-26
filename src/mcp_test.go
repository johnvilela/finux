package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"finux/src/account"
	"finux/src/category"
	"finux/src/goal"
	"finux/src/money"
	"finux/src/note"
	"finux/src/schedule"
	"finux/src/transaction"
)

// mcpHome is a database that the tests use through 'finux mcp'. The date
// of today is 17/11/2026.
type mcpHome struct {
	t    *testing.T
	home string
}

func newMCPHome(t *testing.T) mcpHome {
	t.Helper()
	h := mcpHome{t: t, home: filepath.Join(t.TempDir(), "data")}
	h.ok("database_init", nil)
	return h
}

func (h mcpHome) path() string { return filepath.Join(h.home, "finux.db") }

// serve starts 'finux mcp', sends the requests and returns the responses.
func (h mcpHome) serve(flags []string, requests ...string) []map[string]any {
	h.t.Helper()
	command := exec.Command(finuxBinary(h.t), append([]string{"config", "mcp"}, flags...)...)
	command.Env = []string{"FINUX_DB=" + h.path(), "HOME=" + h.home, "FINUX_TODAY=2026-11-17"}
	command.Stdin = strings.NewReader(strings.Join(requests, "\n") + "\n")
	var out, errOut bytes.Buffer
	command.Stdout, command.Stderr = &out, &errOut
	if err := command.Run(); err != nil || errOut.Len() > 0 {
		h.t.Fatalf("finux mcp: %v, stderr %q", err, errOut.String())
	}
	var responses []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var response map[string]any
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			h.t.Fatalf("output line is not JSON: %q", line)
		}
		responses = append(responses, response)
	}
	return responses
}

// call runs one tool. It returns the text of the result and its error
// flag.
func (h mcpHome) call(tool string, arguments map[string]any) (string, bool) {
	h.t.Helper()
	if arguments == nil {
		arguments = map[string]any{}
	}
	request, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": arguments}})
	if err != nil {
		h.t.Fatal(err)
	}
	responses := h.serve(nil, string(request))
	if len(responses) != 1 {
		h.t.Fatalf("%s: got %d responses", tool, len(responses))
	}
	result, ok := responses[0]["result"].(map[string]any)
	if !ok {
		h.t.Fatalf("%s: no result: %v", tool, responses[0])
	}
	return result["content"].([]any)[0].(map[string]any)["text"].(string), result["isError"] == true
}

// ok runs a tool that must succeed and returns its decoded result.
func (h mcpHome) ok(tool string, arguments map[string]any) map[string]any {
	h.t.Helper()
	text, failed := h.call(tool, arguments)
	if failed {
		h.t.Fatalf("%s %v failed: %s", tool, arguments, text)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		h.t.Fatalf("%s: result is not a JSON object: %q", tool, text)
	}
	return result
}

// fail runs a tool that must fail with a text that has want.
func (h mcpHome) fail(want, tool string, arguments map[string]any) {
	h.t.Helper()
	if text, failed := h.call(tool, arguments); !failed || !strings.Contains(text, want) {
		h.t.Fatalf("%s %v = %q, error %v; want an error with %q", tool, arguments, text, failed, want)
	}
}

// at returns the value at a path of keys and indexes, as text.
func at(t *testing.T, value any, path ...any) string {
	t.Helper()
	for _, step := range path {
		switch key := step.(type) {
		case string:
			object, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("no object at %v in path %v", step, path)
			}
			value = object[key]
		case int:
			list, ok := value.([]any)
			if !ok || key >= len(list) {
				t.Fatalf("no item %d in path %v: %v", key, path, value)
			}
			value = list[key]
		}
	}
	if number, ok := value.(float64); ok {
		return fmt.Sprintf("%.0f", number)
	}
	return fmt.Sprint(value)
}

func (h mcpHome) want(result any, want string, path ...any) {
	h.t.Helper()
	if got := at(h.t, result, path...); got != want {
		h.t.Fatalf("%v = %q, want %q\nin %v", path, got, want, result)
	}
}

func TestMCPCommand(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	if got := succeed(t, home, "config", "mcp", "--help"); !strings.Contains(got, "finux config mcp") || !strings.Contains(got, "--read-only") {
		t.Fatalf("help = %q", got)
	}
	binary := finuxBinary(t)
	for agent, wants := range map[string][]string{
		"claude":   {"claude mcp add", "finux", binary + " config mcp"},
		"codex":    {"[mcp_servers.finux]", `command = "` + binary + `"`, `args = ["config", "mcp"]`},
		"opencode": {`"finux"`, `"type": "local"`, `"command": ["` + binary + `", "config", "mcp"]`},
	} {
		got := succeed(t, home, "config", "mcp", "config", agent)
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("config %s lacks %q:\n%s", agent, want, got)
			}
		}
	}
	if got := succeed(t, home, "config", "mcp", "config", "claude", "--read-only"); !strings.Contains(got, binary+" config mcp --read-only") {
		t.Errorf("read-only config = %q", got)
	}
	for _, args := range [][]string{{"config", "mcp", "config", "other"}, {"config", "mcp", "config"}, {"config", "mcp", "extra"}, {"config", "mcp", "--unknown"}} {
		if stdout, stderr, status := finux(t, home, args...); status != 2 || stdout != "" || !strings.HasPrefix(stderr, "finux: config mcp: ") {
			t.Errorf("finux %v: status %d, stdout %q, stderr %q", args, status, stdout, stderr)
		}
	}
}

func toolNames(t *testing.T, response map[string]any) []string {
	t.Helper()
	var names []string
	for _, tool := range response["result"].(map[string]any)["tools"].([]any) {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	return names
}

func TestMCPTools(t *testing.T) {
	h := mcpHome{t: t, home: filepath.Join(t.TempDir(), "data")}
	responses := h.serve(nil,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	)
	if len(responses) != 2 {
		t.Fatalf("got %d responses, want 2", len(responses))
	}
	h.want(responses[0], "finux", "result", "serverInfo", "name")
	if instructions := at(t, responses[0], "result", "instructions"); !strings.Contains(instructions, "confirm") {
		t.Errorf("instructions = %q", instructions)
	}
	want := []string{
		"database_init", "database_doctor", "database_reseed",
		"account_list", "account_show", "account_add", "account_edit", "account_archive", "account_restore", "account_delete",
		"category_list", "category_show", "category_add", "category_edit", "category_delete",
		"transaction_list", "transaction_show", "transaction_add", "transaction_edit", "transaction_delete", "transaction_suggestions",
		"transaction_corrections",
		"credit_card_list", "credit_card_show", "credit_card_bills", "credit_card_add", "credit_card_edit", "credit_card_pay",
		"credit_card_unpay", "credit_card_archive", "credit_card_restore", "credit_card_delete",
		"goal_list", "goal_show", "goal_add", "goal_edit", "goal_archive", "goal_restore", "goal_delete",
		"budget_list", "budget_show", "budget_add", "budget_edit", "budget_archive", "budget_restore", "budget_delete",
		"schedule_list", "schedule_show", "schedule_add", "schedule_edit", "schedule_pay", "schedule_skip",
		"schedule_archive", "schedule_restore", "schedule_delete",
		"note_list", "note_show", "note_add", "note_edit", "note_done", "note_delete",
	}
	if got := toolNames(t, responses[1]); !slices.Equal(got, want) {
		t.Errorf("tools = %v\nwant    %v", got, want)
	}
	for _, tool := range responses[1]["result"].(map[string]any)["tools"].([]any) {
		if description := at(t, tool, "description"); len(description) < 20 {
			t.Errorf("tool %s has a short description: %q", at(t, tool, "name"), description)
		}
	}

	readOnly := h.serve([]string{"--read-only"}, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	want = []string{
		"database_doctor", "account_list", "account_show", "category_list", "category_show",
		"transaction_list", "transaction_show", "transaction_suggestions", "transaction_corrections", "credit_card_list", "credit_card_show", "credit_card_bills",
		"goal_list", "goal_show", "budget_list", "budget_show", "schedule_list", "schedule_show", "note_list", "note_show",
	}
	if got := toolNames(t, readOnly[0]); !slices.Equal(got, want) {
		t.Errorf("read-only tools = %v\nwant              %v", got, want)
	}
}

// The schemas write the values of each enum as text. They must be the
// values the application accepts.
func TestMCPEnums(t *testing.T) {
	wants := map[string][]string{
		"currency":          money.Codes(),
		"account_add.color": account.ColorKeys(), "account_edit.color": account.ColorKeys(),
		"credit_card_add.color": account.ColorKeys(), "credit_card_edit.color": account.ColorKeys(),
		"category_add.color": category.ColorKeys(), "category_edit.color": category.ColorKeys(),
		"transaction_add.type": transaction.Types(), "transaction_edit.type": transaction.Types(),
		"transaction_list.type":   append(transaction.Types(), transaction.Payment),
		"transaction_list.timing": transaction.Timings(),
		"transaction_edit.scope":  {"one", "next", "all"}, "transaction_delete.scope": {"one", "next", "all"},
		"schedule_add.type": schedule.Types(), "schedule_edit.type": schedule.Types(),
		"goal_add.type": goal.Types(), "goal_edit.type": goal.Types(),
		"note_list.status": {note.StatusOpen, note.StatusDue, note.StatusDone, note.StatusAll},
		"note_list.source": {note.SourceUser, note.SourceAgent},
	}
	checked := 0
	for _, tool := range mcpTools(false) {
		for name, property := range tool.InputSchema["properties"].(map[string]any) {
			enum, ok := property.(map[string]any)["enum"].([]string)
			if !ok {
				continue
			}
			want, ok := wants[tool.Name+"."+name]
			if !ok {
				want, ok = wants[name]
			}
			if !ok || !slices.Equal(enum, want) {
				t.Errorf("%s.%s: enum = %v, want %v", tool.Name, name, enum, want)
			}
			checked++
		}
	}
	if checked != 22 {
		t.Errorf("checked %d enums, want 22", checked)
	}
}

func TestMCPDatabase(t *testing.T) {
	h := mcpHome{t: t, home: filepath.Join(t.TempDir(), "data")}
	h.fail("database_init", "account_list", nil)
	doctor := h.ok("database_doctor", nil)
	h.want(doctor, "false", "healthy")
	h.want(doctor, "false", "found")

	created := h.ok("database_init", nil)
	h.want(created, "true", "created")
	h.want(created, "14", "base_categories_added")
	h.want(created, h.path(), "path")
	h.want(h.ok("database_init", nil), "false", "created")

	doctor = h.ok("database_doctor", nil)
	h.want(doctor, "true", "healthy")
	h.want(doctor, "[]", "pending_migrations")

	h.ok("category_delete", map[string]any{"category": "TAX", "confirm": true})
	h.want(h.ok("database_reseed", nil), "[TAX]", "added")
	h.want(h.ok("database_reseed", nil), "[]", "added")
}

func TestMCPAccounts(t *testing.T) {
	h := newMCPHome(t)
	h.want(h.ok("account_list", nil), "[]", "accounts")

	created := h.ok("account_add", map[string]any{"name": "Nubank", "currency": "brl", "balance": "5000"})
	h.want(created, "NUBAN", "account", "code")
	h.want(created, "BRL", "account", "currency")
	h.want(created, "500000", "account", "balance", "minor")
	h.want(created, "5000.00", "account", "balance", "decimal")
	h.want(created, "R$ 5.000,00", "account", "balance", "formatted")
	h.want(created, "red", "account", "color")
	h.ok("account_add", map[string]any{"name": "Wallet", "currency": "USD", "code": "w", "description": "Cash", "color": "green"})

	h.fail(`argument "currency" must be one of USD`, "account_add", map[string]any{"name": "Bad", "currency": "XYZ"})
	h.fail("already in use", "account_add", map[string]any{"name": "Other", "currency": "BRL", "code": "W"})
	h.fail("invalid amount", "account_add", map[string]any{"name": "Bad", "currency": "BRL", "balance": "abc"})
	h.fail(`missing argument "name"`, "account_add", map[string]any{"currency": "BRL"})

	list := h.ok("account_list", nil)
	h.want(list, "NUBAN", "accounts", 0, "code")
	h.want(list, "W", "accounts", 1, "code")
	h.want(list, "Cash", "accounts", 1, "description")

	edited := h.ok("account_edit", map[string]any{"account": "nuban", "code": "NU", "description": "Main", "color": "purple"})
	h.want(edited, "NU", "account", "code")
	h.want(edited, "Nubank", "account", "name")
	h.want(h.ok("account_show", map[string]any{"account": "NU"}), "purple", "account", "color")
	h.fail("account not found: NUBAN", "account_show", map[string]any{"account": "NUBAN"})
	h.fail("give at least one", "account_edit", map[string]any{"account": "NU"})

	h.want(h.ok("account_archive", map[string]any{"account": "W"}), "true", "account", "archived")
	h.fail("already archived", "account_archive", map[string]any{"account": "W"})
	h.fail("archived", "account_edit", map[string]any{"account": "W", "name": "X"})
	if list := h.ok("account_list", nil); len(list["accounts"].([]any)) != 1 {
		t.Fatalf("active accounts = %v, want 1", list)
	}
	if list := h.ok("account_list", map[string]any{"include_archived": true}); len(list["accounts"].([]any)) != 2 {
		t.Fatalf("all accounts = %v, want 2", list)
	}
	h.want(h.ok("account_restore", map[string]any{"account": "W"}), "false", "account", "archived")
	h.fail("not archived", "account_restore", map[string]any{"account": "W"})

	h.ok("transaction_add", map[string]any{"title": "Lunch", "value": "20", "account": "W", "category": "FOOD"})
	preview := h.ok("account_delete", map[string]any{"account": "W"})
	h.want(preview, "true", "confirm_required")
	if message := at(t, preview, "message"); !strings.Contains(message, "Delete account W (Wallet) permanently, with 1 transaction and 0 transfers?") {
		t.Fatalf("preview = %q", message)
	}
	h.ok("account_show", map[string]any{"account": "W"})
	deleted := h.ok("account_delete", map[string]any{"account": "W", "confirm": true})
	h.want(deleted, "true", "deleted")
	h.want(deleted, "1", "transactions_deleted")
	h.fail("account not found", "account_show", map[string]any{"account": "W"})
}

func TestMCPCategories(t *testing.T) {
	h := newMCPHome(t)
	list := h.ok("category_list", nil)
	if count := len(list["categories"].([]any)); count != 14 {
		t.Fatalf("got %d categories, want 14", count)
	}
	h.want(list, "BILLS", "categories", 0, "code")
	h.want(list, "expense", "categories", 0, "accepts")

	created := h.ok("category_add", map[string]any{"name": "Pets", "color": "red"})
	h.want(created, "PETS", "category", "code")
	h.fail(`argument "color" must be one of red, green, grey`, "category_add", map[string]any{"name": "Bad", "color": "blue"})
	h.want(h.ok("category_show", map[string]any{"category": "pets"}), "Pets", "category", "name")
	h.fail("category not found: NONE", "category_show", map[string]any{"category": "NONE"})

	h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU"})
	h.ok("transaction_add", map[string]any{"title": "Vet", "value": "150", "account": "NU", "category": "PETS"})
	h.fail("cannot have", "category_edit", map[string]any{"category": "PETS", "color": "green"})
	edited := h.ok("category_edit", map[string]any{"category": "PETS", "name": "Animals", "code": "PET"})
	h.want(edited, "PET", "category", "code")
	h.want(edited, "Animals", "category", "name")

	preview := h.ok("category_delete", map[string]any{"category": "PET"})
	h.want(preview, "true", "confirm_required")
	if message := at(t, preview, "message"); !strings.Contains(message, "move 1 transaction to OTHER") {
		t.Fatalf("preview = %q", message)
	}
	h.ok("category_show", map[string]any{"category": "PET"})
	deleted := h.ok("category_delete", map[string]any{"category": "PET", "confirm": true})
	h.want(deleted, "1", "transactions_moved")
	h.want(deleted, "OTHER", "moved_to")
	h.want(h.ok("transaction_show", map[string]any{"id": 1}), "OTHER", "transaction", "category")
}

func TestMCPTransactions(t *testing.T) {
	h := newMCPHome(t)
	h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU", "balance": "1000"})
	h.ok("account_add", map[string]any{"name": "Inter", "currency": "BRL", "code": "INTER"})

	created := h.ok("transaction_add", map[string]any{"title": "Market", "value": "123.45", "account": "nu", "category": "food", "tags": "market #weekly", "date": "10/11/2026", "description": "Week 2"})
	h.want(created, "1", "transaction", "id")
	h.want(created, "expense", "transaction", "type")
	h.want(created, "-12345", "transaction", "amount", "minor")
	h.want(created, "-R$ 123,45", "transaction", "amount", "formatted")
	h.want(created, "2026-11-10", "transaction", "date")
	h.want(created, "[#market #weekly]", "transaction", "tags")
	h.want(created, "NU", "transaction", "account")
	h.want(created, "FOOD", "transaction", "category")
	h.want(created, "BRL", "transaction", "currency")

	salary := h.ok("transaction_add", map[string]any{"title": "Salary", "value": "3000", "account": "NU", "category": "SAL"})
	h.want(salary, "income", "transaction", "type")
	h.want(salary, "2026-11-17", "transaction", "date")

	transfer := h.ok("transaction_add", map[string]any{"title": "Savings", "value": "500", "account": "NU", "to": "INTER", "date": "2026-11-12"})
	h.want(transfer, "transfer", "transaction", "type")
	h.want(transfer, "4", "transaction", "pair_id")
	h.want(transfer, "INTER", "transaction", "other_account")
	h.want(transfer, "XFER", "transaction", "category")

	h.fail("type", "transaction_add", map[string]any{"title": "Gift", "value": "10", "account": "NU", "category": "OTHER"})
	h.fail("more than zero", "transaction_add", map[string]any{"title": "Zero", "value": "0", "account": "NU", "category": "FOOD"})
	h.fail("account not found: NONE", "transaction_add", map[string]any{"title": "A", "value": "1", "account": "NONE", "category": "FOOD"})
	h.fail("category", "transaction_add", map[string]any{"title": "A", "value": "1", "account": "NU"})

	list := h.ok("transaction_list", nil)
	h.want(list, "4", "total")
	h.want(list, "4", "count")
	h.want(list, "Salary", "transactions", 0, "title")
	h.want(list, "BRL", "totals", 0, "currency")
	h.want(list, "300000", "totals", 0, "income", "minor")
	h.want(list, "12345", "totals", 0, "expense", "minor")
	h.want(list, "287655", "totals", 0, "net", "minor")

	page := h.ok("transaction_list", map[string]any{"limit": 1, "offset": 1})
	h.want(page, "4", "total")
	h.want(page, "1", "count")
	h.want(page, "Savings", "transactions", 0, "title")

	for _, tc := range []struct {
		filter map[string]any
		total  string
	}{
		{map[string]any{"category": "FOOD"}, "1"},
		{map[string]any{"tags": "weekly"}, "1"},
		{map[string]any{"search": "sal"}, "1"},
		{map[string]any{"date": "2026-11-12"}, "2"},
		{map[string]any{"date": "11/2026"}, "4"},
		{map[string]any{"account": "INTER"}, "1"},
		{map[string]any{"type": "transfer"}, "2"},
		{map[string]any{"type": "expense", "date": "2025"}, "0"},
	} {
		h.want(h.ok("transaction_list", tc.filter), tc.total, "total")
	}
	h.fail("category not found", "transaction_list", map[string]any{"category": "NONE"})
	h.fail("invalid date", "transaction_list", map[string]any{"date": "soon"})

	h.want(h.ok("account_show", map[string]any{"account": "NU"}), "337655", "account", "balance", "minor")

	edited := h.ok("transaction_edit", map[string]any{"id": 1, "value": "100", "title": "Big market", "tags": "", "date": "2026-11-11"})
	h.want(edited, "-10000", "transaction", "amount", "minor")
	h.want(edited, "Big market", "transaction", "title")
	h.want(edited, "[]", "transaction", "tags")
	h.want(h.ok("transaction_show", map[string]any{"id": 1}), "2026-11-11", "transaction", "date")
	h.fail("transaction not found: 99", "transaction_show", map[string]any{"id": 99})
	h.fail("give at least one", "transaction_edit", map[string]any{"id": 1})

	suggestions := h.ok("transaction_suggestions", nil)
	h.want(suggestions, "[Savings Salary Big market]", "titles")
	h.want(h.ok("transaction_add", map[string]any{"title": "Bus", "value": "5", "account": "NU", "category": "TRANS", "tags": "daily"}), "5", "transaction", "id")
	h.want(h.ok("transaction_suggestions", nil), "[#daily]", "tags")

	preview := h.ok("transaction_delete", map[string]any{"id": 3})
	h.want(preview, "true", "confirm_required")
	if message := at(t, preview, "message"); !strings.Contains(message, "transactions 3 and 4") {
		t.Fatalf("preview = %q", message)
	}
	h.want(h.ok("transaction_list", nil), "5", "total")
	deleted := h.ok("transaction_delete", map[string]any{"id": 3, "confirm": true})
	h.want(deleted, "[3 4]", "deleted")
	h.want(h.ok("transaction_list", nil), "3", "total")
}

func TestMCPCreditCards(t *testing.T) {
	h := newMCPHome(t)
	h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU", "balance": "5000"})
	h.want(h.ok("credit_card_list", nil), "[]", "credit_cards")

	created := h.ok("credit_card_add", map[string]any{"name": "Visa Gold", "currency": "BRL", "limit": "5000", "closing_day": 25, "due_day": 5, "code": "visa"})
	h.want(created, "VISA", "credit_card", "code")
	h.want(created, "500000", "credit_card", "limit", "minor")
	h.want(created, "25", "credit_card", "closing_day")
	h.want(created, "5", "credit_card", "due_day")
	h.fail("already in use", "credit_card_add", map[string]any{"name": "Other", "currency": "BRL", "limit": "1", "closing_day": 1, "due_day": 10, "code": "NU"})
	h.fail("closing day", "credit_card_add", map[string]any{"name": "Other", "currency": "BRL", "limit": "1", "closing_day": 40, "due_day": 10})

	// The bill 11/2026 closed on 25/10/2026 and was due on 05/11/2026.
	h.ok("transaction_add", map[string]any{"title": "Fridge", "value": "1000", "account": "VISA", "category": "SHOP", "date": "2026-10-20"})
	current := h.ok("transaction_add", map[string]any{"title": "Dinner", "value": "200", "account": "VISA", "category": "FOOD", "date": "2026-11-10"})
	h.want(current, "VISA", "transaction", "credit_card")
	h.want(current, "2026-12", "transaction", "bill")
	h.fail("transfer", "transaction_add", map[string]any{"title": "Move", "value": "1", "account": "VISA", "to": "NU"})

	shown := h.ok("credit_card_show", map[string]any{"credit_card": "VISA"})
	h.want(shown, "120000", "credit_card", "used", "minor")
	h.want(shown, "380000", "credit_card", "available", "minor")
	h.want(shown, "2026-12", "open_bill", "period")
	h.want(shown, "open", "open_bill", "status")
	h.want(shown, "2026-11-25", "open_bill", "closes_on")
	h.want(shown, "20000", "open_bill", "total", "minor")
	h.want(shown, "Dinner", "open_bill", "transactions", 0, "title")
	h.want(shown, "2026-11", "pending_bills", 0, "period")
	h.want(shown, "late", "pending_bills", 0, "status")

	bills := h.ok("credit_card_bills", map[string]any{"credit_card": "VISA"})
	h.want(bills, "2026-12", "bills", 0, "period")
	h.want(bills, "100000", "bills", 1, "total", "minor")

	h.fail("still open", "credit_card_pay", map[string]any{"credit_card": "VISA", "from": "NU", "bill": "12/2026"})
	h.fail("account not found", "credit_card_pay", map[string]any{"credit_card": "VISA", "from": "NONE"})
	paid := h.ok("credit_card_pay", map[string]any{"credit_card": "VISA", "from": "NU", "value": "800"})
	h.want(paid, "paid", "bill", "status")
	h.want(paid, "2026-11", "bill", "period")
	h.want(paid, "80000", "bill", "paid", "minor")
	h.want(paid, "20000", "bill", "remaining", "minor")
	h.want(paid, "2026-12", "carried_to")
	h.want(paid, "3", "payment", "id")
	h.want(paid, "payment", "payment", "type")
	h.want(paid, "NU", "payment", "account")
	h.want(paid, "VISA", "payment", "credit_card")
	h.want(h.ok("account_show", map[string]any{"account": "NU"}), "420000", "account", "balance", "minor")
	h.want(h.ok("credit_card_show", map[string]any{"credit_card": "VISA"}), "40000", "open_bill", "total", "minor")
	h.fail("no bill of VISA waits for a payment", "credit_card_pay", map[string]any{"credit_card": "VISA", "from": "NU"})
	h.fail("is paid", "transaction_delete", map[string]any{"id": 1, "confirm": true})
	h.fail("payment cannot change", "transaction_edit", map[string]any{"id": 3, "title": "X"})

	preview := h.ok("credit_card_unpay", map[string]any{"credit_card": "VISA"})
	h.want(preview, "true", "confirm_required")
	if message := at(t, preview, "message"); !strings.Contains(message, "bill 11/2026 of VISA (R$ 800,00)") {
		t.Fatalf("preview = %q", message)
	}
	unpaid := h.ok("credit_card_unpay", map[string]any{"credit_card": "VISA", "confirm": true})
	h.want(unpaid, "late", "bill", "status")
	h.fail("VISA has no paid bill", "credit_card_unpay", map[string]any{"credit_card": "VISA", "confirm": true})
	h.want(h.ok("account_show", map[string]any{"account": "NU"}), "500000", "account", "balance", "minor")

	edited := h.ok("credit_card_edit", map[string]any{"credit_card": "VISA", "limit": "8000", "due_day": 7, "name": "Visa Platinum"})
	h.want(edited, "800000", "credit_card", "limit", "minor")
	h.want(edited, "7", "credit_card", "due_day")
	h.want(edited, "Visa Platinum", "credit_card", "name")
	h.fail("give at least one", "credit_card_edit", map[string]any{"credit_card": "VISA"})

	h.want(h.ok("credit_card_archive", map[string]any{"credit_card": "VISA"}), "true", "credit_card", "archived")
	h.want(h.ok("credit_card_list", nil), "[]", "credit_cards")
	h.want(h.ok("credit_card_list", map[string]any{"include_archived": true}), "VISA", "credit_cards", 0, "code")
	h.want(h.ok("credit_card_restore", map[string]any{"credit_card": "VISA"}), "false", "credit_card", "archived")

	h.ok("credit_card_pay", map[string]any{"credit_card": "VISA", "from": "NU"})
	preview = h.ok("credit_card_delete", map[string]any{"credit_card": "VISA"})
	h.want(preview, "true", "confirm_required")
	if message := at(t, preview, "message"); !strings.Contains(message, "with 2 transactions and 1 payment") {
		t.Fatalf("preview = %q", message)
	}
	deleted := h.ok("credit_card_delete", map[string]any{"credit_card": "VISA", "confirm": true})
	h.want(deleted, "true", "deleted")
	h.fail("credit card not found", "credit_card_show", map[string]any{"credit_card": "VISA"})
	list := h.ok("transaction_list", nil)
	h.want(list, "1", "total")
	h.want(list, "expense", "transactions", 0, "type")
	h.want(list, "OTHER", "transactions", 0, "category")
}

// startHTTP starts 'finux mcp --http' on a free port and returns the URL
// of its endpoint. stop ends the server and returns its exit status.
func (h mcpHome) startHTTP(env ...string) (endpoint string, stop func() int) {
	h.t.Helper()
	command := exec.Command(finuxBinary(h.t), "config", "mcp", "--http", "127.0.0.1:0")
	command.Env = append([]string{"FINUX_DB=" + h.path(), "HOME=" + h.home, "FINUX_TODAY=2026-11-17"}, env...)
	stderr, err := command.StderrPipe()
	if err != nil {
		h.t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		h.t.Fatal(err)
	}
	stopped := false
	stop = func() int {
		if stopped {
			return 0
		}
		stopped = true
		command.Process.Signal(os.Interrupt)
		io.Copy(io.Discard, stderr)
		command.Wait()
		return command.ProcessState.ExitCode()
	}
	h.t.Cleanup(func() { stop() })
	line, err := bufio.NewReader(stderr).ReadString('\n')
	_, endpoint, found := strings.Cut(strings.TrimSpace(line), "listens on ")
	endpoint, _, _ = strings.Cut(endpoint, " ")
	if err != nil || !found || !strings.HasPrefix(endpoint, "http://127.0.0.1:") || !strings.HasSuffix(endpoint, "/mcp") {
		h.t.Fatalf("first line of stderr = %q, %v", line, err)
	}
	return endpoint, stop
}

// send posts one message and returns the status and the body.
func send(t *testing.T, endpoint, token, message string) (int, string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(message))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	text, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, string(text)
}

func TestMCPHTTP(t *testing.T) {
	h := newMCPHome(t)
	endpoint, stop := h.startHTTP()
	status, text := send(t, endpoint, "", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"account_add","arguments":{"name":"Nubank","currency":"BRL","code":"NU"}}}`)
	if status != http.StatusOK || !strings.Contains(text, `\"code\":\"NU\"`) {
		t.Fatalf("account_add: status %d, body %s", status, text)
	}
	if status, text := send(t, endpoint, "", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`); status != http.StatusOK || !strings.Contains(text, "credit_card_pay") {
		t.Fatalf("tools/list: status %d, body %s", status, text)
	}
	if status, _ := send(t, strings.TrimSuffix(endpoint, "/mcp")+"/other", "", pingMessage); status != http.StatusNotFound {
		t.Fatalf("other path: status %d, want 404", status)
	}
	if status := stop(); status != 0 {
		t.Fatalf("exit status after interrupt = %d, want 0", status)
	}
	// The account is in the database of the CLI.
	h.want(h.ok("account_list", nil), "NU", "accounts", 0, "code")
}

const pingMessage = `{"jsonrpc":"2.0","id":1,"method":"ping"}`

func TestMCPHTTPToken(t *testing.T) {
	h := newMCPHome(t)
	endpoint, _ := h.startHTTP("FINUX_MCP_TOKEN=secret-token")
	if status, _ := send(t, endpoint, "", pingMessage); status != http.StatusUnauthorized {
		t.Fatalf("without token: status %d, want 401", status)
	}
	if status, _ := send(t, endpoint, "wrong", pingMessage); status != http.StatusUnauthorized {
		t.Fatalf("wrong token: status %d, want 401", status)
	}
	if status, text := send(t, endpoint, "secret-token", pingMessage); status != http.StatusOK {
		t.Fatalf("with token: status %d, body %s", status, text)
	}
}

func TestMCPHTTPOptions(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	// An address that other computers reach needs a token.
	for _, address := range []string{":0", "0.0.0.0:0", "192.168.0.10:8765"} {
		stdout, stderr, status := finux(t, home, "config", "mcp", "--http", address)
		if status != 2 || stdout != "" || !strings.Contains(stderr, "FINUX_MCP_TOKEN") {
			t.Errorf("--http %s: status %d, stdout %q, stderr %q", address, status, stdout, stderr)
		}
	}
	if _, stderr, status := finux(t, home, "config", "mcp", "--http", "nonsense"); status != 2 || !strings.Contains(stderr, "invalid address") {
		t.Errorf("--http nonsense: status %d, stderr %q", status, stderr)
	}
	if _, stderr, status := finux(t, home, "config", "mcp", "config", "claude", "--http", "127.0.0.1:1"); status != 2 || !strings.Contains(stderr, "--url") {
		t.Errorf("config --http: status %d, stderr %q", status, stderr)
	}

	url := "http://127.0.0.1:8765/mcp"
	for agent, wants := range map[string][]string{
		"claude":   {"claude mcp add --transport http --scope user finux " + url},
		"codex":    {"[mcp_servers.finux]", `url = "` + url + `"`},
		"opencode": {`"type": "remote"`, `"url": "` + url + `"`},
	} {
		got := succeed(t, home, "config", "mcp", "config", agent, "--url", url)
		for _, want := range append(wants, "finux config mcp --http") {
			if !strings.Contains(got, want) {
				t.Errorf("config %s --url lacks %q:\n%s", agent, want, got)
			}
		}
		if strings.Contains(got, "FINUX_MCP_TOKEN") {
			t.Errorf("config %s --url has a token without FINUX_MCP_TOKEN:\n%s", agent, got)
		}
	}
	for agent, want := range map[string]string{
		"claude":   `--header "Authorization: Bearer $FINUX_MCP_TOKEN"`,
		"codex":    `bearer_token_env_var = "FINUX_MCP_TOKEN"`,
		"opencode": `"Authorization": "Bearer {env:FINUX_MCP_TOKEN}"`,
	} {
		stdout, stderr, status := finuxEnv(t, []string{"HOME=" + home, "FINUX_MCP_TOKEN=secret-token"}, "config", "mcp", "config", agent, "--url", url)
		if status != 0 || stderr != "" || !strings.Contains(stdout, want) || strings.Contains(stdout, "secret-token") {
			t.Errorf("config %s with token: status %d, stderr %q, stdout:\n%s", agent, status, stderr, stdout)
		}
	}
}

func TestMCPGoals(t *testing.T) {
	h := newMCPHome(t)
	h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU", "balance": "5000"})
	h.ok("account_add", map[string]any{"name": "Inter", "currency": "BRL", "code": "INTER"})
	h.ok("account_add", map[string]any{"name": "Wise", "currency": "USD", "code": "WISE"})
	h.want(h.ok("goal_list", nil), "[]", "goals")

	created := h.ok("goal_add", map[string]any{"title": "Trip to Japan", "type": "save", "target": "1000", "currency": "brl", "due_date": "10/11/2026", "code": "trip", "description": "In 2027"})
	h.want(created, "TRIP", "goal", "code")
	h.want(created, "Trip to Japan", "goal", "title")
	h.want(created, "save", "goal", "type")
	h.want(created, "BRL", "goal", "currency")
	h.want(created, "100000", "goal", "target", "minor")
	h.want(created, "0", "goal", "progress", "minor")
	h.want(created, "100000", "goal", "remaining", "minor")
	h.want(created, "0", "goal", "percent")
	h.want(created, "2026-11-10", "goal", "due_date")
	h.want(created, "overdue", "goal", "status")
	h.want(created, "false", "goal", "archived")
	loan := h.ok("goal_add", map[string]any{"title": "Loan", "type": "payoff", "target": "500", "currency": "BRL"})
	h.want(loan, "LOAN", "goal", "code")
	h.want(loan, "active", "goal", "status")
	if at(t, loan, "goal", "due_date") != "<nil>" {
		t.Fatalf("goal without a final date has due_date: %v", loan)
	}
	h.fail("target must be more than zero", "goal_add", map[string]any{"title": "X", "type": "save", "target": "0", "currency": "BRL"})
	h.fail("already in use", "goal_add", map[string]any{"title": "X", "type": "save", "target": "1", "currency": "BRL", "code": "TRIP"})
	h.fail(`argument "type" must be one of save, payoff, purchase`, "goal_add", map[string]any{"title": "X", "type": "budget", "target": "1", "currency": "BRL"})

	bonus := h.ok("transaction_add", map[string]any{"title": "Bonus", "value": "400", "account": "NU", "category": "SAL", "goal": "trip"})
	h.want(bonus, "TRIP", "transaction", "goal")
	h.ok("transaction_add", map[string]any{"title": "Withdrawal", "value": "100", "account": "NU", "category": "OTHER", "type": "expense", "goal": "TRIP"})
	h.ok("transaction_add", map[string]any{"title": "To savings", "value": "200", "account": "NU", "to": "INTER", "goal": "TRIP"})
	plain := h.ok("transaction_add", map[string]any{"title": "Market", "value": "50", "account": "NU", "category": "FOOD"})
	if at(t, plain, "transaction", "goal") != "<nil>" {
		t.Fatalf("a transaction without a goal has a goal: %v", plain)
	}
	h.fail("goal TRIP is in BRL, not in USD", "transaction_add", map[string]any{"title": "A", "value": "1", "account": "WISE", "category": "FOOD", "goal": "TRIP"})
	h.fail("goal not found: NOPE", "transaction_add", map[string]any{"title": "A", "value": "1", "account": "NU", "category": "FOOD", "goal": "NOPE"})

	shown := h.ok("goal_show", map[string]any{"goal": "TRIP"})
	h.want(shown, "50000", "goal", "progress", "minor")
	h.want(shown, "50", "goal", "percent")
	h.want(shown, "3", "goal", "transactions")
	h.want(shown, "To savings", "transactions", 0, "title")
	h.want(shown, "20000", "transactions", 0, "effect", "minor")
	h.want(shown, "Withdrawal", "transactions", 1, "title")
	h.want(shown, "-10000", "transactions", 1, "effect", "minor")
	h.want(shown, "Bonus", "transactions", 2, "title")
	if count := len(shown["transactions"].([]any)); count != 3 {
		t.Fatalf("goal_show has %d transactions, want 3", count)
	}
	h.want(h.ok("transaction_list", map[string]any{"goal": "TRIP"}), "4", "total")

	moved := h.ok("transaction_edit", map[string]any{"id": 2, "goal": "LOAN"})
	h.want(moved, "LOAN", "transaction", "goal")
	h.want(h.ok("goal_show", map[string]any{"goal": "LOAN"}), "10000", "goal", "progress", "minor")
	unlinked := h.ok("transaction_edit", map[string]any{"id": 2, "goal": ""})
	if at(t, unlinked, "transaction", "goal") != "<nil>" {
		t.Fatalf("an empty goal kept the link: %v", unlinked)
	}
	h.want(h.ok("transaction_edit", map[string]any{"id": 1, "title": "Big bonus"}), "TRIP", "transaction", "goal")

	edited := h.ok("goal_edit", map[string]any{"goal": "TRIP", "title": "Japan", "target": "600", "due_date": "", "type": "purchase"})
	h.want(edited, "Japan", "goal", "title")
	h.want(edited, "purchase", "goal", "type")
	h.want(edited, "reached", "goal", "status")
	h.want(edited, "100", "goal", "percent")
	if at(t, edited, "goal", "due_date") != "<nil>" {
		t.Fatalf("an empty due_date kept the date: %v", edited)
	}
	h.fail("give at least one", "goal_edit", map[string]any{"goal": "TRIP"})
	h.fail("goal not found: NOPE", "goal_show", map[string]any{"goal": "NOPE"})

	h.want(h.ok("goal_archive", map[string]any{"goal": "TRIP"}), "true", "goal", "archived")
	h.fail("already archived", "goal_archive", map[string]any{"goal": "TRIP"})
	h.fail("goal TRIP is archived", "transaction_add", map[string]any{"title": "A", "value": "1", "account": "NU", "category": "SAL", "goal": "TRIP"})
	if list := h.ok("goal_list", nil); len(list["goals"].([]any)) != 1 {
		t.Fatalf("active goals = %v, want 1", list)
	}
	if list := h.ok("goal_list", map[string]any{"include_archived": true}); len(list["goals"].([]any)) != 2 {
		t.Fatalf("all goals = %v, want 2", list)
	}
	h.want(h.ok("goal_restore", map[string]any{"goal": "TRIP"}), "false", "goal", "archived")
	h.fail("not archived", "goal_restore", map[string]any{"goal": "TRIP"})

	preview := h.ok("goal_delete", map[string]any{"goal": "TRIP"})
	h.want(preview, "true", "confirm_required")
	if message := at(t, preview, "message"); !strings.Contains(message, "Delete goal TRIP (Japan) permanently; 2 transactions lose the link? No transaction is deleted.") {
		t.Fatalf("preview = %q", message)
	}
	h.ok("goal_show", map[string]any{"goal": "TRIP"})
	deleted := h.ok("goal_delete", map[string]any{"goal": "TRIP", "confirm": true})
	h.want(deleted, "true", "deleted")
	h.want(deleted, "2", "transactions_unlinked")
	h.fail("goal not found", "goal_show", map[string]any{"goal": "TRIP"})
	list := h.ok("transaction_list", nil)
	h.want(list, "5", "total")
	if at(t, h.ok("transaction_show", map[string]any{"id": 1}), "transaction", "goal") != "<nil>" {
		t.Fatal("the transaction kept the deleted goal")
	}
}

func TestMCPBudgets(t *testing.T) {
	h := newMCPHome(t)
	h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU", "balance": "5000"})
	h.want(h.ok("budget_list", nil), "[]", "budgets")

	created := h.ok("budget_add", map[string]any{"title": "Food", "limit": "800", "currency": "brl", "categories": "food, fun", "every": "month", "code": "food", "description": "Market"})
	for _, want := range []struct {
		value string
		path  []any
	}{
		{"FOOD", []any{"code"}}, {"Food", []any{"title"}}, {"Market", []any{"description"}}, {"BRL", []any{"currency"}},
		{"80000", []any{"limit", "minor"}}, {"every month", []any{"period"}}, {"1", []any{"every"}}, {"month", []any{"unit"}},
		{"2026-11-01", []any{"start_date"}}, {"false", []any{"rollover"}}, {"ok", []any{"status"}}, {"false", []any{"archived"}},
		{"FOOD", []any{"categories", 0}}, {"FUN", []any{"categories", 1}},
		{"2026-11-01", []any{"current", "from"}}, {"2026-11-30", []any{"current", "to"}}, {"0", []any{"current", "spent", "minor"}},
		{"80000", []any{"current", "available", "minor"}}, {"80000", []any{"current", "remaining", "minor"}},
		{"0", []any{"current", "percent"}}, {"ok", []any{"current", "level"}},
	} {
		h.want(created, want.value, append([]any{"budget"}, want.path...)...)
	}
	if at(t, created, "budget", "end_date") != "<nil>" {
		t.Fatalf("a budget without an end date has end_date: %v", created)
	}
	once := h.ok("budget_add", map[string]any{"title": "Trip", "limit": "3000", "currency": "USD", "start_date": "01/12/2026", "end_date": "31/12/2026"})
	h.want(once, "TRIP", "budget", "code")
	h.want(once, "one time", "budget", "period")
	h.want(once, "2026-12-31", "budget", "end_date")
	h.want(once, "upcoming", "budget", "status")
	h.want(once, "[]", "budget", "categories")
	if at(t, once, "budget", "current") != "<nil>" || at(t, once, "budget", "unit") != "<nil>" {
		t.Fatalf("a budget that did not start has a period: %v", once)
	}
	h.fail("give every or end_date", "budget_add", map[string]any{"title": "X", "limit": "1", "currency": "BRL"})
	h.fail("give a period or an end date", "budget_add", map[string]any{"title": "X", "limit": "1", "currency": "BRL", "every": "none"})
	h.fail("limit must be more than zero", "budget_add", map[string]any{"title": "X", "limit": "0", "currency": "BRL", "every": "month"})
	h.fail("category not found: NOPE", "budget_add", map[string]any{"title": "X", "limit": "1", "currency": "BRL", "every": "month", "categories": "NOPE"})
	h.fail("already in use", "budget_add", map[string]any{"title": "X", "limit": "1", "currency": "BRL", "every": "month", "code": "FOOD"})
	h.fail(`invalid period "fortnight"`, "budget_add", map[string]any{"title": "X", "limit": "1", "currency": "BRL", "every": "fortnight"})

	// A transaction tells when it leaves a budget near or over its limit.
	quiet := h.ok("transaction_add", map[string]any{"title": "Market", "value": "600", "account": "NU", "category": "FOOD"})
	if at(t, quiet, "budget_warnings") != "<nil>" {
		t.Fatalf("a transaction below the warning has budget_warnings: %v", quiet)
	}
	near := h.ok("transaction_add", map[string]any{"title": "Bakery", "value": "80", "account": "NU", "category": "FOOD"})
	h.want(near, "FOOD", "budget_warnings", 0, "budget")
	h.want(near, "near", "budget_warnings", 0, "level")
	h.want(near, "Budget FOOD: 85% spent, R$ 120,00 left until 30/11/2026", "budget_warnings", 0, "message")
	over := h.ok("transaction_add", map[string]any{"title": "Dinner", "value": "200", "account": "NU", "category": "FUN"})
	h.want(over, "over", "budget_warnings", 0, "level")
	h.ok("transaction_add", map[string]any{"title": "Old", "value": "50", "account": "NU", "category": "FOOD", "date": "2026-10-10"})
	edited := h.ok("transaction_edit", map[string]any{"id": 3, "value": "250"})
	h.want(edited, "Budget FOOD: over the limit by R$ 130,00 until 30/11/2026", "budget_warnings", 0, "message")

	shown := h.ok("budget_show", map[string]any{"budget": "food"})
	h.want(shown, "over", "budget", "status")
	h.want(shown, "93000", "budget", "current", "spent", "minor")
	h.want(shown, "2026-11-01", "period", "from")
	h.want(shown, "116", "period", "percent")
	h.want(shown, "13000", "period", "over", "minor")
	h.want(shown, "0", "period", "remaining", "minor")
	h.want(shown, "over", "period", "level")
	h.want(shown, "Dinner", "transactions", 0, "title")
	h.want(shown, "25000", "transactions", 0, "counts", "minor")
	h.want(shown, "[]", "history")
	if count := len(shown["transactions"].([]any)); count != 3 {
		t.Fatalf("budget_show has %d transactions, want 3", count)
	}

	rolled := h.ok("budget_edit", map[string]any{"budget": "FOOD", "start_date": "2026-10-01", "rollover": true})
	h.want(rolled, "true", "budget", "rollover")
	h.want(rolled, "75000", "budget", "current", "carried", "minor")
	h.want(rolled, "155000", "budget", "current", "available", "minor")
	h.want(rolled, "ok", "budget", "status")
	shown = h.ok("budget_show", map[string]any{"budget": "FOOD"})
	h.want(shown, "2026-10-01", "history", 0, "from")
	h.want(shown, "5000", "history", 0, "spent", "minor")
	if at(t, h.ok("budget_show", map[string]any{"budget": "FOOD", "history": 0}), "history") != "[]" {
		t.Fatal("history: 0 returned periods")
	}
	past := h.ok("budget_show", map[string]any{"budget": "FOOD", "date": "15/10/2026"})
	h.want(past, "2026-10-31", "period", "to")
	h.want(past, "Old", "transactions", 0, "title")
	h.want(past, "[]", "history")
	h.fail("budget FOOD has no period on 01/09/2026", "budget_show", map[string]any{"budget": "FOOD", "date": "2026-09-01"})
	h.fail("budget not found: NOPE", "budget_show", map[string]any{"budget": "NOPE"})

	changed := h.ok("budget_edit", map[string]any{"budget": "FOOD", "title": "Meals", "limit": "900", "categories": "all", "every": "2 weeks", "end_date": "2026-12-31", "rollover": false})
	h.want(changed, "Meals", "budget", "title")
	h.want(changed, "90000", "budget", "limit", "minor")
	h.want(changed, "[]", "budget", "categories")
	h.want(changed, "every 2 weeks", "budget", "period")
	h.want(changed, "2026-12-31", "budget", "end_date")
	h.want(changed, "false", "budget", "rollover")
	if at(t, h.ok("budget_edit", map[string]any{"budget": "FOOD", "end_date": ""}), "budget", "end_date") != "<nil>" {
		t.Fatal("an empty end_date kept the date")
	}
	h.fail("give a period or an end date", "budget_edit", map[string]any{"budget": "FOOD", "every": "none"})
	h.fail("give at least one", "budget_edit", map[string]any{"budget": "FOOD"})

	h.want(h.ok("budget_archive", map[string]any{"budget": "FOOD"}), "true", "budget", "archived")
	h.fail("already archived", "budget_archive", map[string]any{"budget": "FOOD"})
	h.fail("it is archived; restore it first", "budget_edit", map[string]any{"budget": "FOOD", "title": "X"})
	silent := h.ok("transaction_add", map[string]any{"title": "Snack", "value": "900", "account": "NU", "category": "FOOD"})
	if at(t, silent, "budget_warnings") != "<nil>" {
		t.Fatalf("an archived budget warns: %v", silent)
	}
	if list := h.ok("budget_list", nil); len(list["budgets"].([]any)) != 1 {
		t.Fatalf("active budgets = %v, want 1", list)
	}
	if list := h.ok("budget_list", map[string]any{"include_archived": true}); len(list["budgets"].([]any)) != 2 {
		t.Fatalf("all budgets = %v, want 2", list)
	}
	h.want(h.ok("budget_restore", map[string]any{"budget": "FOOD"}), "false", "budget", "archived")
	h.fail("not archived", "budget_restore", map[string]any{"budget": "FOOD"})

	preview := h.ok("budget_delete", map[string]any{"budget": "FOOD"})
	h.want(preview, "true", "confirm_required")
	if message := at(t, preview, "message"); !strings.Contains(message, "Delete budget FOOD (Meals) permanently? No transaction changes.") {
		t.Fatalf("preview = %q", message)
	}
	h.ok("budget_show", map[string]any{"budget": "FOOD"})
	deleted := h.ok("budget_delete", map[string]any{"budget": "FOOD", "confirm": true})
	h.want(deleted, "true", "deleted")
	h.want(deleted, "FOOD", "budget")
	h.fail("budget not found", "budget_show", map[string]any{"budget": "FOOD"})
	h.want(h.ok("transaction_list", nil), "5", "total")
}

func TestMCPInstallments(t *testing.T) {
	h := newMCPHome(t)
	h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU", "balance": "5000"})
	h.ok("credit_card_add", map[string]any{"name": "Visa Gold", "currency": "BRL", "limit": "5000", "closing_day": 25, "due_day": 5, "code": "VISA"})

	created := h.ok("transaction_add", map[string]any{"title": "Notebook", "value": "1000", "installments": 3, "account": "VISA", "category": "SHOP"})
	h.want(created, "1", "transaction", "id")
	h.want(created, "1", "transaction", "installment")
	h.want(created, "3", "transaction", "installments")
	h.want(created, "1", "transaction", "installment_id")
	h.want(created, "-33334", "transaction", "amount", "minor")
	h.want(created, "3", "transactions", 2, "installment")
	h.want(created, "-33333", "transactions", 2, "amount", "minor")
	h.want(created, "2027-01-17", "transactions", 2, "date")
	h.want(created, "2027-02", "transactions", 2, "bill")
	if list := created["transactions"].([]any); len(list) != 3 {
		t.Fatalf("created %d transactions, want 3", len(list))
	}
	// A transaction without installments has no installment fields.
	normal := h.ok("transaction_add", map[string]any{"title": "Market", "value": "30", "account": "NU", "category": "FOOD"})
	if fields := normal["transaction"].(map[string]any); fields["installment"] != nil || fields["installment_id"] != nil || normal["transactions"] != nil {
		t.Fatalf("normal transaction = %v", normal)
	}
	// The balance of an account counts the transactions up to today.
	h.ok("transaction_add", map[string]any{"title": "Rent", "value": "100", "account": "NU", "category": "HOME", "date": "2026-11-18"})
	h.want(h.ok("account_show", map[string]any{"account": "NU"}), "497000", "account", "balance", "minor")

	progress := h.ok("transaction_add", map[string]any{"title": "Phone", "installment_value": "50", "installments": 6, "first_installment": 5, "account": "NU", "category": "SHOP"})
	h.want(progress, "5", "transaction", "installment")
	h.want(progress, "-5000", "transactions", 1, "amount", "minor")
	h.fail("not the two", "transaction_add", map[string]any{"title": "A", "value": "10", "installment_value": "5", "installments": 2, "account": "NU", "category": "SHOP"})
	h.fail("value or installment_value", "transaction_add", map[string]any{"title": "A", "account": "NU", "category": "SHOP"})
	h.fail("from 2 to 120", "transaction_add", map[string]any{"title": "A", "value": "10", "installments": 1, "account": "NU", "category": "SHOP"})
	h.fail("only for installments", "transaction_add", map[string]any{"title": "A", "value": "10", "first_installment": 2, "account": "NU", "category": "SHOP"})

	edited := h.ok("transaction_edit", map[string]any{"id": 2, "scope": "next", "title": "Laptop", "installment_value": "400"})
	h.want(edited, "2", "transaction", "id")
	h.want(edited, "Laptop", "transactions", 1, "title")
	h.want(edited, "-40000", "transactions", 1, "amount", "minor")
	h.want(h.ok("transaction_show", map[string]any{"id": 1}), "Notebook", "transaction", "title")
	one := h.ok("transaction_edit", map[string]any{"id": 3, "value": "500"})
	h.want(one, "-50000", "transaction", "amount", "minor")
	if one["transactions"] != nil {
		t.Fatalf("edit of one part lists transactions: %v", one)
	}
	h.fail("not an installment", "transaction_edit", map[string]any{"id": 4, "scope": "all", "title": "X"})
	h.fail("installment_value needs the scope next or all", "transaction_edit", map[string]any{"id": 2, "installment_value": "1"})
	h.fail("give at least one", "transaction_edit", map[string]any{"id": 2, "scope": "all"})

	preview := h.ok("transaction_delete", map[string]any{"id": 2, "scope": "next"})
	h.want(preview, "true", "confirm_required")
	if message := at(t, preview, "message"); !strings.Contains(message, "installments 2 to 3 of Laptop (2 transactions)") {
		t.Fatalf("preview = %q", message)
	}
	deleted := h.ok("transaction_delete", map[string]any{"id": 2, "scope": "next", "confirm": true})
	h.want(deleted, "[2 3]", "deleted")
	h.fail("not an installment", "transaction_delete", map[string]any{"id": 4, "scope": "all", "confirm": true})
	h.want(h.ok("transaction_delete", map[string]any{"id": 6, "scope": "all", "confirm": true}), "[6 7]", "deleted")
	h.want(h.ok("transaction_list", nil), "3", "total")
}

func TestMCPSchedules(t *testing.T) {
	h := newMCPHome(t)
	h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU", "balance": "5000"})
	h.ok("credit_card_add", map[string]any{"name": "Visa Gold", "currency": "BRL", "limit": "5000", "closing_day": 25, "due_day": 5, "code": "VISA"})
	h.want(h.ok("schedule_list", nil), "[]", "schedules")

	created := h.ok("schedule_add", map[string]any{"title": "Rent", "value": "1500", "account": "NU", "category": "HOME", "every": "month", "date": "2026-10-05", "code": "rent", "tags": "fixed"})
	h.want(created, "RENT", "schedule", "code")
	h.want(created, "expense", "schedule", "type")
	h.want(created, "150000", "schedule", "value", "minor")
	h.want(created, "NU", "schedule", "account")
	h.want(created, "HOME", "schedule", "category")
	h.want(created, "every month", "schedule", "repeats")
	h.want(created, "false", "schedule", "automatic")
	h.want(created, "due", "schedule", "status")
	h.want(created, "[2026-10-05 2026-11-05]", "schedule", "due")
	h.want(created, "2026-12-05", "schedule", "next")
	h.want(created, "[#fixed]", "schedule", "tags")
	once := h.ok("schedule_add", map[string]any{"title": "Dentist", "value": "300", "account": "VISA", "category": "HEALT", "date": "10/12/2026"})
	h.want(once, "DENTI", "schedule", "code")
	h.want(once, "VISA", "schedule", "credit_card")
	h.want(once, "one time", "schedule", "repeats")
	h.want(once, "upcoming", "schedule", "status")
	h.want(once, "1", "schedule", "times_left")
	h.fail("category", "schedule_add", map[string]any{"title": "A", "value": "1", "account": "NU", "category": "NONE"})
	h.fail("only for a schedule that repeats", "schedule_add", map[string]any{"title": "A", "value": "1", "account": "NU", "category": "HOME", "times": 3})

	list := h.ok("schedule_list", nil)
	h.want(list, "2", "due")
	h.want(list, "DENTI", "schedules", 0, "code")
	// A schedule is not a transaction.
	h.want(h.ok("transaction_list", nil), "0", "total")

	paid := h.ok("schedule_pay", map[string]any{"schedule": "RENT"})
	h.want(paid, "2026-10-05", "transaction", "date")
	h.want(paid, "-150000", "transaction", "amount", "minor")
	h.want(paid, "RENT", "transaction", "schedule")
	h.want(paid, "2026-10-05", "transaction", "due_date")
	h.want(paid, "on_time", "transaction", "timing")
	h.want(paid, "[2026-11-05]", "schedule", "due")
	real := h.ok("schedule_pay", map[string]any{"schedule": "RENT", "value": "1550.50", "date": "2026-11-06"})
	h.want(real, "-155050", "transaction", "amount", "minor")
	h.want(real, "2026-11-06", "transaction", "date")
	h.want(real, "2026-11-05", "transaction", "due_date")
	h.want(real, "late", "transaction", "timing")
	h.want(real, "1", "transaction", "days_late")
	late := h.ok("transaction_list", map[string]any{"timing": "late"})
	h.want(late, "1", "total")
	h.want(late, "2", "transactions", 0, "id")
	h.fail("must be one of on_time, late, early", "transaction_list", map[string]any{"timing": "soon"})
	// An edit of the due date changes the timing; none takes the two away.
	early := h.ok("transaction_edit", map[string]any{"id": 2, "due_date": "2026-11-10"})
	h.want(early, "early", "transaction", "timing")
	h.want(early, "-4", "transaction", "days_late")
	cleared := h.ok("transaction_edit", map[string]any{"id": 2, "due_date": "none"})
	for _, field := range []string{"due_date", "timing", "days_late"} {
		if _, has := cleared["transaction"].(map[string]any)[field]; has {
			t.Fatalf("a transaction without a due date has %s: %v", field, cleared)
		}
	}
	h.want(h.ok("transaction_edit", map[string]any{"id": 2, "due_date": "2026-11-05"}), "late", "transaction", "timing")
	h.want(real, "upcoming", "schedule", "status")
	skipped := h.ok("schedule_skip", map[string]any{"schedule": "RENT"})
	h.want(skipped, "2026-12-05", "skipped")
	h.want(skipped, "2027-01-05", "schedule", "next")
	shown := h.ok("schedule_show", map[string]any{"schedule": "RENT"})
	h.want(shown, "2", "transactions", 0, "id")
	if list := shown["transactions"].([]any); len(list) != 2 {
		t.Fatalf("schedule_show has %d transactions, want 2", len(list))
	}

	edited := h.ok("schedule_edit", map[string]any{"schedule": "RENT", "title": "Home", "value": "1600", "date": "2027-01-10", "times": 2, "automatic": true})
	h.want(edited, "Home", "schedule", "title")
	h.want(edited, "2027-01-10", "schedule", "next")
	h.want(edited, "2", "schedule", "times_left")
	h.want(edited, "true", "schedule", "automatic")
	h.fail("give at least one", "schedule_edit", map[string]any{"schedule": "RENT"})
	h.fail("schedule not found: NONE", "schedule_show", map[string]any{"schedule": "NONE"})

	// A server that can write creates the transactions of an automatic
	// schedule that is due, when the setting is on.
	h.ok("schedule_add", map[string]any{"title": "Water", "value": "50", "account": "NU", "category": "BILLS", "every": "month", "date": "2026-11-10", "automatic": true, "code": "WATER"})
	h.want(h.ok("transaction_list", map[string]any{"search": "Water"}), "0", "total")
	h.ok("schedule_delete", map[string]any{"schedule": "WATER", "confirm": true})
	succeed(t, h.home, "config", "set", "automatic-schedules", "on")
	h.ok("schedule_add", map[string]any{"title": "Internet", "value": "100", "account": "NU", "category": "BILLS", "every": "month", "date": "2026-11-10", "automatic": true, "code": "NET"})
	transactions := h.ok("transaction_list", map[string]any{"search": "Internet"})
	h.want(transactions, "1", "total")
	h.want(transactions, "NET", "transactions", 0, "schedule")

	h.want(h.ok("schedule_archive", map[string]any{"schedule": "NET"}), "true", "schedule", "archived")
	h.fail("it is archived", "schedule_pay", map[string]any{"schedule": "NET"})
	h.want(h.ok("schedule_list", map[string]any{"include_archived": true}), "4", "schedules", 2, "id")
	h.want(h.ok("schedule_restore", map[string]any{"schedule": "NET"}), "false", "schedule", "archived")
	h.want(h.ok("schedule_delete", map[string]any{"schedule": "NET"}), "true", "confirm_required")
	h.want(h.ok("schedule_delete", map[string]any{"schedule": "NET", "confirm": true}), "true", "deleted")
	if fields := h.ok("transaction_show", map[string]any{"id": 3})["transaction"].(map[string]any); fields["schedule"] != nil {
		t.Fatalf("transaction keeps a deleted schedule: %v", fields)
	}
}

func TestMCPVirtualCard(t *testing.T) {
	h := newMCPHome(t)
	h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU", "balance": "1000"})
	h.ok("credit_card_add", map[string]any{"name": "Visa Gold", "currency": "BRL", "limit": "5000", "closing_day": 25, "due_day": 5, "code": "visa"})

	market := h.ok("transaction_add", map[string]any{"title": "Market", "value": "100", "account": "VISA", "category": "FOOD"})
	if _, has := market["transaction"].(map[string]any)["virtual"]; has {
		t.Fatalf("a transaction with no mark has virtual: %v", market)
	}
	stream := h.ok("transaction_add", map[string]any{"title": "Streaming", "value": "40", "account": "VISA", "category": "FOOD", "virtual": true})
	h.want(stream, "true", "transaction", "virtual")
	h.fail("only a credit card transaction", "transaction_add", map[string]any{"title": "Bread", "value": "5", "account": "NU", "category": "FOOD", "virtual": true})

	// The filter gives the transactions with the mark, or the other ones.
	list := h.ok("transaction_list", map[string]any{"virtual": true})
	h.want(list, "1", "total")
	h.want(list, "Streaming", "transactions", 0, "title")
	list = h.ok("transaction_list", map[string]any{"virtual": false})
	h.want(list, "1", "total")
	h.want(list, "Market", "transactions", 0, "title")

	// The open bill has the total of the virtual card.
	card := h.ok("credit_card_show", map[string]any{"credit_card": "VISA"})
	h.want(card, "4000", "open_bill", "virtual_total", "minor")
	h.want(card, "true", "open_bill", "transactions", 0, "virtual")

	edited := h.ok("transaction_edit", map[string]any{"id": 2, "virtual": false})
	if _, has := edited["transaction"].(map[string]any)["virtual"]; has {
		t.Fatalf("the edit did not take the mark away: %v", edited)
	}
	card = h.ok("credit_card_show", map[string]any{"credit_card": "VISA"})
	if _, has := card["open_bill"].(map[string]any)["virtual_total"]; has {
		t.Fatalf("a bill with no virtual transaction has virtual_total: %v", card)
	}
	h.want(h.ok("transaction_edit", map[string]any{"id": 1, "virtual": true}), "true", "transaction", "virtual")
}

func TestMCPOffLimit(t *testing.T) {
	h := newMCPHome(t)
	h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU", "balance": "1000"})
	h.ok("credit_card_add", map[string]any{"name": "Visa Gold", "currency": "BRL", "limit": "5000", "closing_day": 25, "due_day": 5, "code": "visa"})

	market := h.ok("transaction_add", map[string]any{"title": "Market", "value": "100", "account": "VISA", "category": "FOOD"})
	if _, has := market["transaction"].(map[string]any)["off_limit"]; has {
		t.Fatalf("a transaction with no mark has off_limit: %v", market)
	}
	h.fail("only a credit card transaction", "transaction_add", map[string]any{"title": "Bread", "value": "5", "account": "NU", "category": "FOOD", "off_limit": true})
	car := h.ok("transaction_add", map[string]any{"title": "Car repair", "value": "1200", "account": "VISA", "category": "FOOD", "installments": 10, "off_limit": true})
	h.want(car, "true", "transactions", 9, "off_limit")

	// The filter gives the transactions with the mark, or the other ones.
	h.want(h.ok("transaction_list", map[string]any{"off_limit": true}), "10", "total")
	list := h.ok("transaction_list", map[string]any{"off_limit": false})
	h.want(list, "1", "total")
	h.want(list, "Market", "transactions", 0, "title")

	// The part in the open bill uses the limit. The parts in the future
	// bills do not.
	card := h.ok("credit_card_show", map[string]any{"credit_card": "VISA"})
	h.want(card, "22000", "credit_card", "used", "minor")
	h.want(card, "478000", "credit_card", "available", "minor")
	h.want(card, "108000", "credit_card", "off_limit", "minor")
	h.want(card, "22000", "open_bill", "total", "minor")
	h.want(card, "12000", "open_bill", "off_limit", "minor")

	// A change of the mark of one part goes to all the parts.
	edited := h.ok("transaction_edit", map[string]any{"id": 5, "off_limit": false})
	if _, has := edited["transaction"].(map[string]any)["off_limit"]; has {
		t.Fatalf("the edit did not take the mark away: %v", edited)
	}
	h.want(h.ok("transaction_list", map[string]any{"off_limit": true}), "0", "total")
	card = h.ok("credit_card_show", map[string]any{"credit_card": "VISA"})
	h.want(card, "130000", "credit_card", "used", "minor")
	h.want(card, "0", "credit_card", "off_limit", "minor")
	if _, has := card["open_bill"].(map[string]any)["off_limit"]; has {
		t.Fatalf("a bill with no transaction outside the limit has off_limit: %v", card)
	}
	h.want(h.ok("transaction_edit", map[string]any{"id": 1, "off_limit": true}), "true", "transaction", "off_limit")
}

// A transaction that no schedule made can have a due date.
func TestMCPDueDate(t *testing.T) {
	h := newMCPHome(t)
	h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU", "balance": "1000"})
	water := h.ok("transaction_add", map[string]any{"title": "Water", "value": "80", "account": "NU", "category": "BILLS", "date": "2026-11-17", "due_date": "2026-11-10"})
	h.want(water, "2026-11-10", "transaction", "due_date")
	h.want(water, "late", "transaction", "timing")
	h.want(water, "7", "transaction", "days_late")
	h.fail("installments cannot have a due date", "transaction_add", map[string]any{"title": "Sofa", "value": "900", "account": "NU", "category": "SHOP", "installments": 3, "due_date": "2026-11-10"})
	h.ok("transaction_add", map[string]any{"title": "Sofa", "value": "900", "account": "NU", "category": "SHOP", "installments": 3})
	h.fail("a due date is of one transaction", "transaction_edit", map[string]any{"id": 2, "scope": "all", "due_date": "2026-11-10"})
	h.want(h.ok("transaction_edit", map[string]any{"id": 2, "due_date": "2026-11-17"}), "on_time", "transaction", "timing")
}

// The corrections tell an agent how the user changed the transactions
// with a title, by hand or through the agent.
func TestMCPCorrections(t *testing.T) {
	h := newMCPHome(t)
	h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU", "balance": "1000"})
	h.ok("transaction_add", map[string]any{"title": "PAY LAMEI 21/11", "value": "16", "account": "NU", "category": "OTHER", "type": "expense", "date": "2026-11-21"})
	h.ok("transaction_add", map[string]any{"title": "PAY MINUT 19/11", "value": "38.39", "account": "NU", "category": "FOOD", "date": "2026-11-19"})
	h.want(h.ok("transaction_corrections", nil), "0", "total")

	// The user changes a transaction in the CLI.
	succeed(t, h.home, "tx", "edit", "1", "--title", "Lamen House", "--category", "FOOD")
	rules := h.ok("transaction_corrections", map[string]any{"titles": []string{"PAY LAMEI 28/11", "PAY OTHER 01/12"}})
	h.want(rules, "1", "total")
	h.want(rules, "PAY LAMEI", "corrections", 0, "pattern")
	h.want(rules, "Lamen House", "corrections", 0, "title")
	h.want(rules, "FOOD", "corrections", 0, "category")
	h.want(rules, "user", "corrections", 0, "source")
	h.want(rules, "1", "corrections", 0, "count")
	rule := rules["corrections"].([]any)[0].(map[string]any)
	for _, field := range []string{"tags", "description"} {
		if _, ok := rule[field]; ok {
			t.Errorf("the correction has %s, which no change gave: %v", field, rule)
		}
	}

	// A change through the agent has the agent as its source.
	h.ok("transaction_edit", map[string]any{"id": 2, "title": "Minuto", "tags": "market"})
	all := h.ok("transaction_corrections", nil)
	h.want(all, "2", "total")
	h.want(all, "PAY MINUT", "corrections", 0, "pattern")
	h.want(all, "agent", "corrections", 0, "source")
	h.want(all, "[#market]", "corrections", 0, "tags")
	page := h.ok("transaction_corrections", map[string]any{"limit": 1, "offset": 1})
	h.want(page, "2", "total")
	h.want(page, "1", "count")
	h.want(page, "PAY LAMEI", "corrections", 0, "pattern")
}

func TestMCPScheduleWithoutAccount(t *testing.T) {
	h := newMCPHome(t)
	h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU", "balance": "5000"})
	h.ok("credit_card_add", map[string]any{"name": "Visa Gold", "currency": "BRL", "limit": "5000", "closing_day": 25, "due_day": 5, "code": "VISA"})
	// A schedule can wait without an account: the payment gives it.
	undecided := h.ok("schedule_add", map[string]any{"title": "Energy", "value": "187", "category": "BILLS", "date": "2026-11-06", "every": "month", "code": "ENERG"})
	h.want(undecided, "BRL", "schedule", "currency")
	h.want(undecided, "18700", "schedule", "value", "minor")
	for _, field := range []string{"account", "credit_card"} {
		if _, ok := undecided["schedule"].(map[string]any)[field]; ok {
			t.Errorf("schedule without an account has %s: %v", field, undecided)
		}
	}
	h.fail("give account: the schedule ENERG has no account", "schedule_pay", map[string]any{"schedule": "ENERG"})
	h.fail("automatic schedule needs an account", "schedule_edit", map[string]any{"schedule": "ENERG", "automatic": true})
	h.want(h.ok("schedule_edit", map[string]any{"schedule": "ENERG", "account": "NU"}), "NU", "schedule", "account")
	h.want(h.ok("schedule_edit", map[string]any{"schedule": "ENERG", "account": "none"}), "BRL", "schedule", "currency")
	paid := h.ok("schedule_pay", map[string]any{"schedule": "ENERG", "account": "VISA", "date": "2026-11-17"})
	h.want(paid, "VISA", "transaction", "credit_card")
	h.want(paid, "2026-12-06", "schedule", "next")
	h.fail("give account or currency, not the two", "schedule_add", map[string]any{"title": "A", "value": "1", "category": "BILLS", "account": "NU", "currency": "BRL"})
}

func TestMCPNotes(t *testing.T) {
	h := newMCPHome(t)
	h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU", "balance": "5000"})
	h.ok("credit_card_add", map[string]any{"name": "Visa Gold", "currency": "BRL", "limit": "5000", "closing_day": 25, "due_day": 5, "code": "VISA"})
	h.ok("goal_add", map[string]any{"title": "Trip", "type": "save", "target": "1000", "currency": "BRL", "code": "TRIP"})
	h.ok("budget_add", map[string]any{"title": "Food", "limit": "800", "currency": "BRL", "categories": "FOOD", "every": "month", "code": "FOOD"})
	h.want(h.ok("note_list", nil), "[]", "notes")

	created := h.ok("note_add", map[string]any{"title": "Ask about the fee", "text": "The bank took R$ 12 in November.", "remind_on": "10/11/2026", "tags": "bank fee", "account": "NU"})
	h.want(created, "1", "note", "id")
	h.want(created, "Ask about the fee", "note", "title")
	h.want(created, "2026-11-10", "note", "remind_on")
	h.want(created, "due", "note", "status")
	h.want(created, "[#bank #fee]", "note", "tags")
	h.want(created, "NU", "note", "account")
	h.want(created, "agent", "note", "source")
	h.ok("note_add", map[string]any{"title": "Check the limit", "remind_on": "2026-12-05", "account": "VISA", "goal": "TRIP", "budget": "FOOD"})
	h.ok("note_add", map[string]any{"title": "No new installments until March"})
	// The user writes a note in the CLI.
	succeed(t, h.home, "nt", "add", "--title", "Mine", "--tags", "bank")
	h.fail("title is empty", "note_add", map[string]any{"title": " "})
	h.fail("text is too long", "note_add", map[string]any{"title": "A", "text": strings.Repeat("a", 501)})
	h.fail("goal not found: NOPE", "note_add", map[string]any{"title": "A", "goal": "NOPE"})

	list := h.ok("note_list", nil)
	h.want(list, "4", "total")
	h.want(list, "1", "due")
	h.want(list, "Ask about the fee", "notes", 0, "title")
	h.want(list, "VISA", "notes", 1, "credit_card")
	h.want(list, "TRIP", "notes", 1, "goal")
	h.want(list, "FOOD", "notes", 1, "budget")
	for filter, want := range map[string]map[string]any{
		"1": {"status": "due"},
		"2": {"tag": "bank"},
		"0": {"status": "done"},
	} {
		h.want(h.ok("note_list", want), filter, "total")
	}
	h.want(h.ok("note_list", map[string]any{"search": "installments"}), "3", "notes", 0, "id")
	h.want(h.ok("note_list", map[string]any{"account": "VISA"}), "2", "notes", 0, "id")
	h.want(h.ok("note_list", map[string]any{"goal": "TRIP"}), "1", "total")
	h.want(h.ok("note_list", map[string]any{"source": "user"}), "Mine", "notes", 0, "title")
	page := h.ok("note_list", map[string]any{"limit": 1, "offset": 1})
	h.want(page, "4", "total")
	h.want(page, "1", "count")
	h.want(page, "2", "notes", 0, "id")
	h.fail("must be one of open, due, done, all", "note_list", map[string]any{"status": "late"})
	h.want(h.ok("note_show", map[string]any{"id": 1}), "The bank took R$ 12 in November.", "note", "text")
	h.fail("note not found: 99", "note_show", map[string]any{"id": 99})

	// The linked records return their open notes.
	h.want(h.ok("account_show", map[string]any{"account": "NU"}), "Ask about the fee", "notes", 0, "title")
	h.want(h.ok("credit_card_show", map[string]any{"credit_card": "VISA"}), "2", "notes", 0, "id")
	h.want(h.ok("goal_show", map[string]any{"goal": "TRIP"}), "2", "notes", 0, "id")
	h.want(h.ok("budget_show", map[string]any{"budget": "FOOD"}), "2", "notes", 0, "id")

	edited := h.ok("note_edit", map[string]any{"id": 2, "title": "Check the card limit", "remind_on": "none", "goal": "none", "account": "NU", "tags": ""})
	h.want(edited, "Check the card limit", "note", "title")
	h.want(edited, "NU", "note", "account")
	h.want(edited, "[]", "note", "tags")
	h.want(edited, "open", "note", "status")
	for _, field := range []string{"remind_on", "goal", "credit_card"} {
		if _, ok := edited["note"].(map[string]any)[field]; ok {
			t.Errorf("the note keeps %s after the edit: %v", field, edited)
		}
	}
	h.fail("give at least one", "note_edit", map[string]any{"id": 2})

	done := h.ok("note_done", map[string]any{"id": 1})
	h.want(done, "done", "note", "status")
	h.fail("note 1 is already done", "note_done", map[string]any{"id": 1})
	h.want(h.ok("note_list", map[string]any{"status": "due"}), "0", "total")
	h.want(h.ok("account_show", map[string]any{"account": "NU"}), "2", "notes", 0, "id")
	h.want(h.ok("note_done", map[string]any{"id": 1, "done": false}), "due", "note", "status")

	preview := h.ok("note_delete", map[string]any{"id": 3})
	h.want(preview, "true", "confirm_required")
	h.want(h.ok("note_delete", map[string]any{"id": 3, "confirm": true}), "true", "deleted")
	h.want(h.ok("note_list", map[string]any{"status": "all"}), "3", "total")
}

func TestMCPOtherPersons(t *testing.T) {
	h := newMCPHome(t)
	h.want(h.ok("account_add", map[string]any{"name": "Nubank", "currency": "BRL", "code": "NU", "balance": "5000"}), "", "account", "owner")
	h.want(h.ok("account_add", map[string]any{"name": "Conta Ana", "currency": "BRL", "code": "ANA", "owner": " Ana "}), "Ana", "account", "owner")
	card := map[string]any{"name": "Visa", "currency": "BRL", "limit": "5000", "closing_day": 25, "due_day": 5, "code": "VISA"}
	h.want(h.ok("credit_card_add", card), "", "credit_card", "owner")
	card["name"], card["code"], card["owner"] = "Master Ana", "MANA", "Ana"
	h.want(h.ok("credit_card_add", card), "Ana", "credit_card", "owner")
	h.fail("owner must have at most 40 characters", "account_edit", map[string]any{"account": "NU", "owner": strings.Repeat("a", 41)})

	count := func(result map[string]any, key string) int { return len(result[key].([]any)) }
	if list := h.ok("account_list", nil); count(list, "accounts") != 1 || at(t, list, "accounts", 0, "code") != "NU" {
		t.Fatalf("my accounts = %v", list)
	}
	if list := h.ok("account_list", map[string]any{"include_others": true}); count(list, "accounts") != 2 {
		t.Fatalf("all accounts = %v", list)
	}
	if list := h.ok("credit_card_list", nil); count(list, "credit_cards") != 1 || at(t, list, "credit_cards", 0, "code") != "VISA" {
		t.Fatalf("my credit cards = %v", list)
	}
	if list := h.ok("credit_card_list", map[string]any{"include_others": true}); count(list, "credit_cards") != 2 {
		t.Fatalf("all credit cards = %v", list)
	}
	h.want(h.ok("account_show", map[string]any{"account": "ANA"}), "Ana", "account", "owner")

	h.ok("transaction_add", map[string]any{"title": "Market", "value": "30", "account": "NU", "category": "FOOD"})
	h.ok("transaction_add", map[string]any{"title": "Her market", "value": "70", "account": "ANA", "category": "FOOD"})
	h.ok("transaction_add", map[string]any{"title": "Her dinner", "value": "90", "account": "MANA", "category": "FOOD"})
	mine := h.ok("transaction_list", nil)
	h.want(mine, "1", "total")
	h.want(mine, "30.00", "totals", 0, "expense", "decimal")
	all := h.ok("transaction_list", map[string]any{"include_others": true})
	h.want(all, "3", "total")
	h.want(all, "190.00", "totals", 0, "expense", "decimal")
	h.want(h.ok("transaction_list", map[string]any{"account": "ANA"}), "1", "total")

	h.want(h.ok("account_edit", map[string]any{"account": "ANA", "owner": ""}), "", "account", "owner")
	h.want(h.ok("credit_card_edit", map[string]any{"credit_card": "MANA", "owner": ""}), "", "credit_card", "owner")
	h.want(h.ok("transaction_list", nil), "3", "total")
}
