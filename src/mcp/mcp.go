// Package mcp is a Model Context Protocol server for tools. It speaks
// JSON-RPC 2.0, one message for each line, as the stdio transport of the
// protocol specifies. It has no resources, prompts or sampling.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
)

// protocols lists the protocol revisions this server speaks, the oldest
// first. The tools part of the protocol is the same in all of them.
var protocols = []string{"2024-11-05", "2025-03-26", "2025-06-18"}

// LatestProtocol is the revision the server offers to a client that asks
// for a revision the server does not know.
var LatestProtocol = protocols[len(protocols)-1]

// JSON-RPC error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeNoMethod       = -32601
	codeInvalidParams  = -32602
)

// Tool is one operation an agent can call.
type Tool struct {
	Name, Description string
	// InputSchema is the JSON Schema of the arguments.
	InputSchema map[string]any
	// ReadOnly is true when the tool changes nothing. Destructive is true
	// when the tool can delete data.
	ReadOnly, Destructive bool
	// Call runs the tool. The value it returns goes to the agent as JSON.
	// An error goes to the agent as the text of a failed call.
	Call func(arguments json.RawMessage) (any, error)
}

// Server answers the requests of one client.
type Server struct {
	Name, Version string
	// Instructions tell the agent how to use the tools.
	Instructions string
	Tools        []Tool
}

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads requests from in until it ends and writes the responses to
// out. Each message is one line.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	for {
		line, err := reader.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			if answer := s.handle(line); answer != nil {
				encoded, encodeErr := json.Marshal(answer)
				if encodeErr != nil {
					return fmt.Errorf("encode response: %w", encodeErr)
				}
				if _, writeErr := out.Write(append(encoded, '\n')); writeErr != nil {
					return fmt.Errorf("write response: %w", writeErr)
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read request: %w", err)
		}
	}
}

var nullID = json.RawMessage("null")

// handle answers one message. It returns nil for a message that gets no
// answer: a notification or a response of the client.
func (s *Server) handle(line []byte) *response {
	if line[0] == '[' {
		return failure(nullID, codeInvalidRequest, "batches are not supported")
	}
	var message request
	if err := json.Unmarshal(line, &message); err != nil {
		return failure(nullID, codeParse, "parse error: "+err.Error())
	}
	if message.Method == "" || len(message.ID) == 0 {
		return nil
	}
	switch message.Method {
	case "initialize":
		return success(message.ID, s.initialize(message.Params))
	case "ping":
		return success(message.ID, struct{}{})
	case "tools/list":
		return success(message.ID, map[string]any{"tools": s.descriptions()})
	case "tools/call":
		return s.call(message)
	}
	return failure(message.ID, codeNoMethod, "method not found: "+message.Method)
}

func success(id json.RawMessage, result any) *response {
	return &response{JSONRPC: "2.0", ID: id, Result: result}
}

func failure(id json.RawMessage, code int, message string) *response {
	return &response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

func (s *Server) initialize(params json.RawMessage) map[string]any {
	var asked struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	// A request without a readable revision gets the latest one.
	_ = json.Unmarshal(params, &asked)
	protocol := LatestProtocol
	if slices.Contains(protocols, asked.ProtocolVersion) {
		protocol = asked.ProtocolVersion
	}
	result := map[string]any{
		"protocolVersion": protocol,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
	}
	if s.Instructions != "" {
		result["instructions"] = s.Instructions
	}
	return result
}

func (s *Server) descriptions() []map[string]any {
	tools := make([]map[string]any, len(s.Tools))
	for i, tool := range s.Tools {
		tools[i] = map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": tool.InputSchema,
			"annotations": map[string]any{
				"readOnlyHint":    tool.ReadOnly,
				"destructiveHint": tool.Destructive,
				"openWorldHint":   false,
			},
		}
	}
	return tools
}

func (s *Server) call(message request) *response {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(message.Params, &params); err != nil {
		return failure(message.ID, codeInvalidParams, "invalid params: "+err.Error())
	}
	index := slices.IndexFunc(s.Tools, func(tool Tool) bool { return tool.Name == params.Name })
	if index < 0 {
		return failure(message.ID, codeInvalidParams, "unknown tool: "+params.Name)
	}
	text, failed := run(s.Tools[index], params.Arguments)
	return success(message.ID, map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
		"isError": failed,
	})
}

// run calls a tool and returns the text for the agent. A tool that fails
// gives its error as the text, so the agent can correct the call.
func run(tool Tool, arguments json.RawMessage) (text string, failed bool) {
	defer func() {
		if cause := recover(); cause != nil {
			text, failed = fmt.Sprintf("internal error: %v", cause), true
		}
	}()
	result, err := tool.Call(arguments)
	if err != nil {
		return err.Error(), true
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "internal error: encode result: " + err.Error(), true
	}
	return string(encoded), false
}
