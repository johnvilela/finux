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
	"finux/src/money"
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
	command := exec.Command(finuxBinary(h.t), append([]string{"mcp"}, flags...)...)
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
	if got := succeed(t, home, "mcp", "--help"); !strings.Contains(got, "finux mcp") || !strings.Contains(got, "--read-only") {
		t.Fatalf("help = %q", got)
	}
	binary := finuxBinary(t)
	for agent, wants := range map[string][]string{
		"claude":   {"claude mcp add", "finux", binary + " mcp"},
		"codex":    {"[mcp_servers.finux]", `command = "` + binary + `"`, `args = ["mcp"]`},
		"opencode": {`"finux"`, `"type": "local"`, `"command": ["` + binary + `", "mcp"]`},
	} {
		got := succeed(t, home, "mcp", "config", agent)
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("config %s lacks %q:\n%s", agent, want, got)
			}
		}
	}
	if got := succeed(t, home, "mcp", "config", "claude", "--read-only"); !strings.Contains(got, binary+" mcp --read-only") {
		t.Errorf("read-only config = %q", got)
	}
	for _, args := range [][]string{{"mcp", "config", "other"}, {"mcp", "config"}, {"mcp", "extra"}, {"mcp", "--unknown"}} {
		if stdout, stderr, status := finux(t, home, args...); status != 2 || stdout != "" || !strings.HasPrefix(stderr, "finux: mcp: ") {
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
		"credit_card_list", "credit_card_show", "credit_card_bills", "credit_card_add", "credit_card_edit", "credit_card_pay",
		"credit_card_unpay", "credit_card_archive", "credit_card_restore", "credit_card_delete",
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
		"transaction_list", "transaction_show", "transaction_suggestions", "credit_card_list", "credit_card_show", "credit_card_bills",
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
		"transaction_list.type": append(transaction.Types(), transaction.Payment),
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
	if checked != 11 {
		t.Errorf("checked %d enums, want 11", checked)
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
	command := exec.Command(finuxBinary(h.t), "mcp", "--http", "127.0.0.1:0")
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
		stdout, stderr, status := finux(t, home, "mcp", "--http", address)
		if status != 2 || stdout != "" || !strings.Contains(stderr, "FINUX_MCP_TOKEN") {
			t.Errorf("--http %s: status %d, stdout %q, stderr %q", address, status, stdout, stderr)
		}
	}
	if _, stderr, status := finux(t, home, "mcp", "--http", "nonsense"); status != 2 || !strings.Contains(stderr, "invalid address") {
		t.Errorf("--http nonsense: status %d, stderr %q", status, stderr)
	}
	if _, stderr, status := finux(t, home, "mcp", "config", "claude", "--http", "127.0.0.1:1"); status != 2 || !strings.Contains(stderr, "--url") {
		t.Errorf("config --http: status %d, stderr %q", status, stderr)
	}

	url := "http://127.0.0.1:8765/mcp"
	for agent, wants := range map[string][]string{
		"claude":   {"claude mcp add --transport http --scope user finux " + url},
		"codex":    {"[mcp_servers.finux]", `url = "` + url + `"`},
		"opencode": {`"type": "remote"`, `"url": "` + url + `"`},
	} {
		got := succeed(t, home, "mcp", "config", agent, "--url", url)
		for _, want := range append(wants, "finux mcp --http") {
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
		stdout, stderr, status := finuxEnv(t, []string{"HOME=" + home, "FINUX_MCP_TOKEN=secret-token"}, "mcp", "config", agent, "--url", url)
		if status != 0 || stderr != "" || !strings.Contains(stdout, want) || strings.Contains(stdout, "secret-token") {
			t.Errorf("config %s with token: status %d, stderr %q, stdout:\n%s", agent, status, stderr, stdout)
		}
	}
}
