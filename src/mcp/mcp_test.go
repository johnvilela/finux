package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type echoArguments struct {
	Text   string   `json:"text" mcp:"Text to return"`
	Times  int      `json:"times,omitempty" mcp:"Number of copies"`
	Mode   *string  `json:"mode,omitempty" mcp:"Letter case" enum:"upper,lower"`
	Loud   bool     `json:"loud,omitempty"`
	Labels []string `json:"labels,omitempty"`
}

func testServer() *Server {
	echo := New("echo", "Return the text", func(a echoArguments) (any, error) {
		if a.Text == "fail" {
			return nil, errors.New("text is not accepted")
		}
		if a.Text == "panic" {
			panic("broken")
		}
		return map[string]any{"text": a.Text, "times": a.Times}, nil
	})
	echo.ReadOnly = true
	remove := New("remove", "Delete all", func(struct{}) (any, error) { return map[string]bool{"deleted": true}, nil })
	remove.Destructive = true
	return &Server{Name: "finux", Version: "1.2.3", Instructions: "Use the tools.", Tools: []Tool{echo, remove}}
}

// exchange sends the lines to a server and returns the decoded lines of
// its output.
func exchange(t *testing.T, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := testServer().Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var responses []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var response map[string]any
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("output line is not JSON: %q", line)
		}
		if response["jsonrpc"] != "2.0" {
			t.Fatalf("response lacks jsonrpc 2.0: %q", line)
		}
		responses = append(responses, response)
	}
	return responses
}

func one(t *testing.T, line string) map[string]any {
	t.Helper()
	responses := exchange(t, line)
	if len(responses) != 1 {
		t.Fatalf("got %d responses, want 1: %v", len(responses), responses)
	}
	return responses[0]
}

func TestInitialize(t *testing.T) {
	for requested, want := range map[string]string{
		"2024-11-05": "2024-11-05",
		"2025-06-18": "2025-06-18",
		"2099-01-01": LatestProtocol,
	} {
		response := one(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"`+requested+`","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`)
		result := response["result"].(map[string]any)
		if result["protocolVersion"] != want {
			t.Errorf("protocol for %s = %v, want %s", requested, result["protocolVersion"], want)
		}
		info := result["serverInfo"].(map[string]any)
		if info["name"] != "finux" || info["version"] != "1.2.3" || result["instructions"] != "Use the tools." {
			t.Errorf("result = %v", result)
		}
		if _, ok := result["capabilities"].(map[string]any)["tools"].(map[string]any); !ok {
			t.Errorf("capabilities = %v, want tools", result["capabilities"])
		}
		if response["id"] != float64(1) {
			t.Errorf("id = %v, want 1", response["id"])
		}
	}
}

func TestNotificationsAndResponsesGetNoAnswer(t *testing.T) {
	responses := exchange(t,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/unknown","params":{}}`,
		`{"jsonrpc":"2.0","id":7,"result":{}}`,
		``,
		`{"jsonrpc":"2.0","id":"a","method":"ping"}`,
	)
	if len(responses) != 1 || responses[0]["id"] != "a" {
		t.Fatalf("responses = %v, want only the ping answer", responses)
	}
	if result, ok := responses[0]["result"].(map[string]any); !ok || len(result) != 0 {
		t.Fatalf("ping result = %v, want {}", responses[0]["result"])
	}
}

func TestToolsList(t *testing.T) {
	response := one(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools := response["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(tools))
	}
	echo := tools[0].(map[string]any)
	if echo["name"] != "echo" || echo["description"] != "Return the text" {
		t.Errorf("tool = %v", echo)
	}
	schema := echo["inputSchema"].(map[string]any)
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Errorf("schema = %v", schema)
	}
	if !reflect.DeepEqual(schema["required"], []any{"text"}) {
		t.Errorf("required = %v, want [text]", schema["required"])
	}
	properties := schema["properties"].(map[string]any)
	wants := map[string]map[string]any{
		"text":   {"type": "string", "description": "Text to return"},
		"times":  {"type": "integer", "description": "Number of copies"},
		"mode":   {"type": "string", "description": "Letter case", "enum": []any{"upper", "lower"}},
		"loud":   {"type": "boolean"},
		"labels": {"type": "array", "items": map[string]any{"type": "string"}},
	}
	if !reflect.DeepEqual(properties, map[string]any{"text": wants["text"], "times": wants["times"], "mode": wants["mode"], "loud": wants["loud"], "labels": wants["labels"]}) {
		t.Errorf("properties = %v", properties)
	}
	annotations := echo["annotations"].(map[string]any)
	if annotations["readOnlyHint"] != true || annotations["destructiveHint"] != false {
		t.Errorf("echo annotations = %v", annotations)
	}
	annotations = tools[1].(map[string]any)["annotations"].(map[string]any)
	if annotations["readOnlyHint"] != false || annotations["destructiveHint"] != true {
		t.Errorf("remove annotations = %v", annotations)
	}
	// A tool without arguments still has a full object schema.
	schema = tools[1].(map[string]any)["inputSchema"].(map[string]any)
	if _, ok := schema["properties"].(map[string]any); !ok || schema["type"] != "object" {
		t.Errorf("empty schema = %v", schema)
	}
}

// call runs a tool and returns its text and its error flag.
func call(t *testing.T, name, arguments string) (string, bool) {
	t.Helper()
	response := one(t, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"`+name+`","arguments":`+arguments+`}}`)
	result, ok := response["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", response)
	}
	content := result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content = %v, want one item", content)
	}
	item := content[0].(map[string]any)
	if item["type"] != "text" {
		t.Fatalf("content type = %v, want text", item["type"])
	}
	return item["text"].(string), result["isError"] == true
}

func TestToolsCall(t *testing.T) {
	text, failed := call(t, "echo", `{"text":"hello","times":2}`)
	if failed || text != `{"text":"hello","times":2}` {
		t.Errorf("echo = %q, error %v", text, failed)
	}
	for arguments, want := range map[string]string{
		`{"text":"fail"}`:           "text is not accepted",
		`{"text":"panic"}`:          "internal error: broken",
		`{}`:                        `missing argument "text"`,
		`{"text":"a","other":1}`:    `unknown argument "other"`,
		`{"text":5}`:                `argument "text" must be a string`,
		`{"text":"a","times":"x"}`:  `argument "times" must be an integer`,
		`{"text":"a","mode":"mid"}`: `argument "mode" must be one of upper, lower`,
		`[1]`:                       "arguments must be an object",
	} {
		if text, failed := call(t, "echo", arguments); !failed || text != want {
			t.Errorf("echo %s = %q, error %v; want error %q", arguments, text, failed, want)
		}
	}
	// The values of an enum match in all letter cases.
	if text, failed := call(t, "echo", `{"text":"a","mode":"UPPER"}`); failed {
		t.Errorf("enum in capitals failed: %s", text)
	}
	// A call without arguments is a call with no arguments.
	response := one(t, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"remove"}}`)
	if result := response["result"].(map[string]any); result["isError"] == true {
		t.Errorf("remove without arguments failed: %v", result)
	}
}

func TestProtocolErrors(t *testing.T) {
	for line, want := range map[string]float64{
		`{"jsonrpc":"2.0","id":4,"method":"resources/list"}`:                         -32601,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"missing"}}`: -32602,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":"text"}`:             -32602,
		`{not json`: -32700,
		`[{"jsonrpc":"2.0","id":4,"method":"ping"}]`: -32600,
	} {
		response := one(t, line)
		failure, ok := response["error"].(map[string]any)
		if !ok || failure["code"] != want || failure["message"] == "" {
			t.Errorf("%s: response = %v, want error %v", line, response, want)
		}
		if _, has := response["id"]; !has {
			t.Errorf("%s: response has no id", line)
		}
	}
}

func TestServeReadsLongLines(t *testing.T) {
	long := strings.Repeat("x", 200_000)
	text, failed := call(t, "echo", `{"text":"`+long+`"}`)
	if failed || !strings.Contains(text, long) {
		t.Fatalf("long line failed: error %v, %d bytes", failed, len(text))
	}
}
