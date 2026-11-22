package telegram

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"gorm.io/gorm"
)

// PairingTime is how long a code or an invite works.
const PairingTime = 10 * time.Minute

// ErrNoCode means the code is wrong, was used, or ended.
var ErrNoCode = errors.New("the code is not correct, or it ended")

// Pairing is one way for a person to get access with no user ID to
// type. There are two kinds:
//
//   - A code from the bot. UserID is the person who asked. The owner
//     types the code in the terminal.
//   - An invite from the terminal. UserID is 0. The person who opens
//     the link of the invite gets access.
//
// The terminal is a part of the two, so a stranger gets no access alone.
type Pairing struct {
	Code      string `gorm:"primaryKey"`
	UserID    int64  `gorm:"index"`
	Name      string
	ExpiresAt time.Time
}

// TableName keeps the table of the pairings with the Telegram tables.
func (Pairing) TableName() string { return "telegram_pairings" }

// cleanName makes a name from Telegram fit the limit of a name.
func cleanName(name string) string {
	runes := []rune(strings.Join(strings.Fields(name), " "))
	return strings.TrimSpace(string(runes[:min(len(runes), MaxName)]))
}

// Request gives the code of a person who has no access. The person
// keeps one code until it ends. fresh reports a new code: the bot tells
// the code only then, so a stranger cannot make the bot send many
// messages.
func (s *Store) Request(user int64, name string, now time.Time) (code string, fresh bool, err error) {
	if err := s.db.Delete(&Pairing{}, "expires_at <= ?", now).Error; err != nil {
		return "", false, err
	}
	var current Pairing
	switch err := s.db.Where("user_id = ?", user).Take(&current).Error; {
	case err == nil:
		return current.Code, false, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return "", false, err
	}
	// A person types this code, so it is short. It gives no access by
	// itself: the owner must type it in the terminal.
	for range 20 {
		number, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
		if err != nil {
			return "", false, err
		}
		pairing := Pairing{Code: fmt.Sprintf("%06d", number), UserID: user, Name: cleanName(name), ExpiresAt: now.Add(PairingTime)}
		// Another person can have the same code: try a new one.
		var count int64
		if err := s.db.Model(&Pairing{}).Where("code = ?", pairing.Code).Count(&count).Error; err != nil {
			return "", false, err
		}
		if count > 0 {
			continue
		}
		return pairing.Code, true, s.db.Create(&pairing).Error
	}
	return "", false, errors.New("no free code now: try again in some minutes")
}

// take finds a pairing that still works and deletes it: a code works
// one time. invite tells the kind.
func (s *Store) take(code string, invite bool, now time.Time) (Pairing, error) {
	var found Pairing
	kind := "user_id <> 0"
	if invite {
		kind = "user_id = 0"
	}
	err := s.db.Where("code = ? AND expires_at > ? AND "+kind, code, now).Take(&found).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || code == "" {
		return found, ErrNoCode
	}
	if err != nil {
		return found, err
	}
	result := s.db.Delete(&Pairing{}, "code = ?", code)
	if result.Error == nil && result.RowsAffected == 0 {
		return found, ErrNoCode
	}
	return found, result.Error
}

// Pair gives access to the person who got the code from the bot. name
// replaces the name that the person has in Telegram.
func (s *Store) Pair(code, name string, now time.Time) (User, error) {
	if name = strings.Join(strings.Fields(name), " "); len([]rune(name)) > MaxName {
		return User{}, errors.New("name is too long: use 40 characters or fewer")
	}
	code = strings.NewReplacer(" ", "", "-", "").Replace(code)
	found, err := s.take(code, false, now)
	if err != nil {
		return User{}, err
	}
	if name == "" {
		name = found.Name
	}
	return User{ID: found.UserID, Name: name}, s.Allow(found.UserID, name)
}

// Invite makes an invite and returns its code, for the link that the
// owner sends to a person. name is the name the person gets.
func (s *Store) Invite(name string, now time.Time) (string, error) {
	if name = strings.Join(strings.Fields(name), " "); len([]rune(name)) > MaxName {
		return "", errors.New("name is too long: use 40 characters or fewer")
	}
	if err := s.db.Delete(&Pairing{}, "expires_at <= ?", now).Error; err != nil {
		return "", err
	}
	// The code is the only secret of an invite, so it is long.
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	code := fmt.Sprintf("%x", raw)
	return code, s.db.Create(&Pairing{Code: code, Name: name, ExpiresAt: now.Add(PairingTime)}).Error
}

// Accept gives access to the person who opened the link of an invite.
// name is the name of the person in Telegram, for an invite with no name.
func (s *Store) Accept(code string, user int64, name string, now time.Time) (User, error) {
	found, err := s.take(strings.TrimSpace(code), true, now)
	if err != nil {
		return User{}, err
	}
	if found.Name != "" {
		name = found.Name
	}
	name = cleanName(name)
	if err := s.Allow(user, name); err != nil {
		return User{}, err
	}
	// The code that the bot gave to the person has no use now.
	return User{ID: user, Name: name}, s.db.Delete(&Pairing{}, "user_id = ?", user).Error
}
