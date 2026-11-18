package mcp

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
)

// maxBody is the largest request the HTTP transport reads, in bytes.
const maxBody = 1 << 20

// Handler returns the Streamable HTTP transport of the server for one
// endpoint. A client sends each message as a POST and gets the response
// as JSON. The server has no sessions and opens no event streams.
//
// With a token, each request needs the header "Authorization: Bearer
// <token>". A request from a web page of a different host is refused, so
// a page in a browser cannot call a server on the same computer.
func (s *Server) Handler(token string) http.Handler {
	// The tools change one database: they run one at a time.
	var calls sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" && !authorized(r.Header.Get("Authorization"), token) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "a bearer token is necessary", http.StatusUnauthorized)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !localOrigin(origin) {
			http.Error(w, "origin not allowed: "+origin, http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "use POST: this server has no event stream", http.StatusMethodNotAllowed)
			return
		}
		if kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || kind != "application/json" {
			http.Error(w, "the content type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		if version := r.Header.Get("MCP-Protocol-Version"); version != "" && !slices.Contains(protocols, version) {
			http.Error(w, "unsupported protocol version: "+version, http.StatusBadRequest)
			return
		}
		message, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "the request is too large", http.StatusRequestEntityTooLarge)
			return
		}
		if message = bytes.TrimSpace(message); err != nil || len(message) == 0 {
			http.Error(w, "the request has no message", http.StatusBadRequest)
			return
		}
		calls.Lock()
		answer := s.handle(message)
		calls.Unlock()
		if answer == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		encoded, err := json.Marshal(answer)
		if err != nil {
			http.Error(w, "encode response: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if answer.Error != nil && (answer.Error.Code == codeParse || answer.Error.Code == codeInvalidRequest) {
			w.WriteHeader(http.StatusBadRequest)
		}
		w.Write(encoded)
	})
}

// authorized compares the bearer token of a request with the token of
// the server. The time it takes does not depend on the tokens.
func authorized(header, token string) bool {
	given, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return false
	}
	want, got := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(given))
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}

// localOrigin reports whether a web origin is on this computer.
func localOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return Loopback(parsed.Hostname())
}

// Loopback reports whether a host name or an IP address is this
// computer.
func Loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}
