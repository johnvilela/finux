package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const pingRequest = `{"jsonrpc":"2.0","id":1,"method":"ping"}`

// post sends one message to a handler and returns the response.
func post(t *testing.T, handler http.Handler, body string, headers map[string]string) *http.Response {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder.Result()
}

func body(t *testing.T, response *http.Response) string {
	t.Helper()
	text, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

func TestHTTPRequest(t *testing.T) {
	handler := testServer().Handler("")
	response := post(t, handler, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hello"}}}`, nil)
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("status %d, content type %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(body(t, response)), &decoded); err != nil {
		t.Fatal(err)
	}
	result := decoded["result"].(map[string]any)
	if text := result["content"].([]any)[0].(map[string]any)["text"]; text != `{"text":"hello","times":0}` || decoded["id"] != float64(3) {
		t.Fatalf("response = %v", decoded)
	}
}

func TestHTTPNotificationIsAccepted(t *testing.T) {
	response := post(t, testServer().Handler(""), `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil)
	if response.StatusCode != http.StatusAccepted || body(t, response) != "" {
		t.Fatalf("status %d, want 202 without a body", response.StatusCode)
	}
}

func TestHTTPRefusals(t *testing.T) {
	handler := testServer().Handler("")
	for name, tc := range map[string]struct {
		body    string
		headers map[string]string
		status  int
	}{
		"parse error":      {`{not json`, nil, http.StatusBadRequest},
		"batch":            {`[` + pingRequest + `]`, nil, http.StatusBadRequest},
		"content type":     {pingRequest, map[string]string{"Content-Type": "text/plain"}, http.StatusUnsupportedMediaType},
		"unknown protocol": {pingRequest, map[string]string{"MCP-Protocol-Version": "1999-01-01"}, http.StatusBadRequest},
		"known protocol":   {pingRequest, map[string]string{"MCP-Protocol-Version": "2025-06-18"}, http.StatusOK},
		"charset":          {pingRequest, map[string]string{"Content-Type": "application/json; charset=utf-8"}, http.StatusOK},
		"foreign origin":   {pingRequest, map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		"rebound origin":   {pingRequest, map[string]string{"Origin": "http://localhost.evil.example"}, http.StatusForbidden},
		"local origin":     {pingRequest, map[string]string{"Origin": "http://localhost:3000"}, http.StatusOK},
		"loopback origin":  {pingRequest, map[string]string{"Origin": "http://127.0.0.1:8765"}, http.StatusOK},
		"too large":        {`{"jsonrpc":"2.0","id":1,"method":"ping","params":"` + strings.Repeat("x", maxBody) + `"}`, nil, http.StatusRequestEntityTooLarge},
	} {
		if response := post(t, handler, tc.body, tc.headers); response.StatusCode != tc.status {
			t.Errorf("%s: status %d, want %d: %s", name, response.StatusCode, tc.status, body(t, response))
		}
	}
	// A parse error still has a JSON-RPC body.
	if text := body(t, post(t, handler, `{not json`, nil)); !strings.Contains(text, "-32700") {
		t.Errorf("parse error body = %q", text)
	}
}

func TestHTTPMethods(t *testing.T) {
	handler := testServer().Handler("")
	for _, method := range []string{http.MethodGet, http.MethodDelete, http.MethodPut} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, "/mcp", nil))
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != "POST" {
			t.Errorf("%s: status %d, Allow %q", method, recorder.Code, recorder.Header().Get("Allow"))
		}
	}
}

func TestHTTPToken(t *testing.T) {
	handler := testServer().Handler("secret-token")
	for name, tc := range map[string]struct {
		header string
		status int
	}{
		"missing": {"", http.StatusUnauthorized},
		"wrong":   {"Bearer other-token", http.StatusUnauthorized},
		"prefix":  {"Bearer secret", http.StatusUnauthorized},
		"basic":   {"Basic secret-token", http.StatusUnauthorized},
		"correct": {"Bearer secret-token", http.StatusOK},
	} {
		headers := map[string]string{}
		if tc.header != "" {
			headers["Authorization"] = tc.header
		}
		response := post(t, handler, pingRequest, headers)
		if response.StatusCode != tc.status {
			t.Errorf("%s: status %d, want %d", name, response.StatusCode, tc.status)
		}
		if tc.status == http.StatusUnauthorized && response.Header.Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("%s: WWW-Authenticate = %q", name, response.Header.Get("WWW-Authenticate"))
		}
	}
	// The token comes before all other checks.
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/mcp", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("GET without token: status %d, want 401", recorder.Code)
	}
}
