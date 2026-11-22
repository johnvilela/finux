package telegram

import (
	"errors"
	"regexp"
	"testing"
	"time"
)

func TestPairWithACodeFromTheBot(t *testing.T) {
	store := newStore(t)
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	code, fresh, err := store.Request(7, "  Maria  Silva ", now)
	if err != nil || !fresh || !regexp.MustCompile(`^\d{6}$`).MatchString(code) {
		t.Fatalf("Request = %q, %v, %v", code, fresh, err)
	}
	// The person has one code until it ends.
	if again, fresh, err := store.Request(7, "Maria", now.Add(9*time.Minute)); err != nil || fresh || again != code {
		t.Fatalf("second Request = %q, %v, %v", again, fresh, err)
	}
	if other, _, _ := store.Request(8, "Other", now); other == code {
		t.Fatal("two people got the same code")
	}
	// A code gives no access by itself.
	if store.Allowed(7) {
		t.Fatal("a person with a code is allowed")
	}
	if _, err := store.Pair("000000x", "", now); !errors.Is(err, ErrNoCode) {
		t.Fatalf("Pair of a wrong code = %v", err)
	}
	// Spaces and a dash in the code do not matter. The name comes from
	// Telegram when the owner gives none.
	user, err := store.Pair(" "+code[:3]+"-"+code[3:]+" ", "", now.Add(time.Minute))
	if err != nil || user.ID != 7 || user.Name != "Maria Silva" || !store.Allowed(7) {
		t.Fatalf("Pair = %+v, %v", user, err)
	}
	// A code works one time.
	if _, err := store.Pair(code, "", now.Add(time.Minute)); !errors.Is(err, ErrNoCode) {
		t.Fatalf("second Pair = %v", err)
	}
	// The owner can give the name. A code that ended does not work, and
	// the person gets a new one.
	code, _, _ = store.Request(8, "Other", now)
	if _, err := store.Pair(code, "Wife", now.Add(PairingTime+time.Second)); !errors.Is(err, ErrNoCode) {
		t.Fatalf("Pair of an old code = %v", err)
	}
	renewed, fresh, err := store.Request(8, "Other", now.Add(PairingTime+time.Second))
	if err != nil || !fresh || renewed == "" {
		t.Fatalf("Request after the end = %q, %v, %v", renewed, fresh, err)
	}
	if user, err := store.Pair(renewed, "Wife", now.Add(PairingTime+time.Minute)); err != nil || user.Name != "Wife" {
		t.Fatalf("Pair with a name = %+v, %v", user, err)
	}
}

func TestInvite(t *testing.T) {
	store := newStore(t)
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	code, err := store.Invite("Wife", now)
	// Telegram permits these characters in the text of a start link.
	if err != nil || !regexp.MustCompile(`^[A-Za-z0-9_-]{20,64}$`).MatchString(code) {
		t.Fatalf("Invite = %q, %v", code, err)
	}
	if _, err := store.Invite("a name that is much longer than the forty characters", now); err == nil {
		t.Fatal("Invite took a name that is too long")
	}
	// The code of an invite is not a code for the terminal, and a code
	// from the bot is not an invite.
	if _, err := store.Pair(code, "", now); !errors.Is(err, ErrNoCode) {
		t.Fatalf("Pair of an invite = %v", err)
	}
	asked, _, _ := store.Request(7, "Stranger", now)
	if _, err := store.Accept(asked, 7, "Stranger", now); !errors.Is(err, ErrNoCode) {
		t.Fatalf("Accept of a code from the bot = %v", err)
	}
	if _, err := store.Accept("wrong", 900, "Maria", now); !errors.Is(err, ErrNoCode) || store.Allowed(900) {
		t.Fatalf("Accept of a wrong code = %v", err)
	}
	// The name of the invite wins over the name in Telegram.
	user, err := store.Accept(code, 900, "Maria", now.Add(time.Minute))
	if err != nil || user.ID != 900 || user.Name != "Wife" || !store.Allowed(900) {
		t.Fatalf("Accept = %+v, %v", user, err)
	}
	if _, err := store.Accept(code, 901, "Thief", now.Add(time.Minute)); !errors.Is(err, ErrNoCode) || store.Allowed(901) {
		t.Fatalf("second Accept = %v", err)
	}
	// An invite with no name takes the name in Telegram. An old invite
	// does not work.
	code, _ = store.Invite("", now)
	if user, err := store.Accept(code, 55, "Bob", now.Add(time.Minute)); err != nil || user.Name != "Bob" {
		t.Fatalf("Accept with no name = %+v, %v", user, err)
	}
	code, _ = store.Invite("", now)
	if _, err := store.Accept(code, 56, "Late", now.Add(PairingTime+time.Second)); !errors.Is(err, ErrNoCode) {
		t.Fatalf("Accept of an old invite = %v", err)
	}
}
