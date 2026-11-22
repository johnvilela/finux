package telegram

import (
	"errors"
	"path/filepath"
	"testing"

	"finux/src/storage"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := storage.Init(path, &User{}, &Notice{}, &Session{}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

func TestAllowedPeople(t *testing.T) {
	store := newStore(t)
	if users, err := store.Users(); err != nil || len(users) != 0 {
		t.Fatalf("Users of a new database = %+v, %v", users, err)
	}
	if store.Allowed(42) {
		t.Fatal("a person is allowed in a new database")
	}
	if err := store.Allow(900, "Wife"); err != nil {
		t.Fatal(err)
	}
	if err := store.Allow(42, "Me"); err != nil {
		t.Fatal(err)
	}
	// A second Allow changes the name.
	if err := store.Allow(42, "John"); err != nil {
		t.Fatal(err)
	}
	users, err := store.Users()
	if err != nil || len(users) != 2 || users[0].ID != 42 || users[0].Name != "John" || users[1].ID != 900 {
		t.Fatalf("Users = %+v, %v", users, err)
	}
	if !store.Allowed(42) || !store.Allowed(900) || store.Allowed(7) {
		t.Fatal("Allowed is wrong")
	}
	if err := store.Allow(0, "Nobody"); err == nil {
		t.Fatal("Allow of the ID 0 has no error")
	}
	if err := store.Deny(42); err != nil || store.Allowed(42) {
		t.Fatalf("Deny = %v", err)
	}
	if err := store.Deny(42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Deny = %v", err)
	}
}

func TestNoticeIsSentOnce(t *testing.T) {
	store := newStore(t)
	for i, want := range []bool{true, false, false} {
		first, err := store.Once("bill:1:late")
		if err != nil || first != want {
			t.Fatalf("Once number %d = %v, %v", i+1, first, err)
		}
	}
	if first, err := store.Once("bill:2:late"); err != nil || !first {
		t.Fatalf("Once of another key = %v, %v", first, err)
	}
}

func TestSessions(t *testing.T) {
	store := newStore(t)
	if got := store.Session(42, "claude"); got != "" {
		t.Fatalf("session of a new person = %q", got)
	}
	if err := store.SaveSession(42, "claude", "abc"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSession(900, "claude", "xyz"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSession(42, "claude", "def"); err != nil {
		t.Fatal(err)
	}
	if got := store.Session(42, "claude"); got != "def" {
		t.Fatalf("session = %q", got)
	}
	// A session belongs to one agent: another agent starts a new one.
	if got := store.Session(42, "codex"); got != "" {
		t.Fatalf("session for another agent = %q", got)
	}
	if err := store.ClearSession(42); err != nil {
		t.Fatal(err)
	}
	if store.Session(42, "claude") != "" || store.Session(900, "claude") != "xyz" {
		t.Fatal("ClearSession is wrong")
	}
	if err := store.ClearSession(42); err != nil {
		t.Fatalf("ClearSession with no session = %v", err)
	}
}
