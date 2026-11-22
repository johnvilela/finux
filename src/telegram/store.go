// Package telegram holds the people who can use the Telegram bot of
// Finux, the reminders that the bot sent, and the client of the
// Telegram Bot API.
package telegram

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MaxName is the size limit of the name of a person in characters.
const MaxName = 40

// ErrNotFound means the person is not in the list of allowed people.
var ErrNotFound = errors.New("not in the list of allowed people")

// User is a person who can use the bot. ID is the Telegram user ID: the
// number that Telegram gives to each account. In a private chat it is
// also the ID of the chat.
type User struct {
	ID        int64 `gorm:"primaryKey;autoIncrement:false"`
	Name      string
	CreatedAt time.Time
}

// TableName keeps the table of the people apart from other users.
func (User) TableName() string { return "telegram_users" }

// Notice is one reminder that the bot sent. Its key names the event, so
// the bot sends each event one time.
type Notice struct {
	Key    string `gorm:"primaryKey"`
	SentAt time.Time
}

// TableName keeps the table of the reminders with the Telegram tables.
func (Notice) TableName() string { return "telegram_notices" }

// Store reads and writes the people and the reminders in the database.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store that uses db.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Allow lets the person with the Telegram user ID use the bot. For a
// person who is in the list already, it changes the name.
func (s *Store) Allow(id int64, name string) error {
	name = strings.Join(strings.Fields(name), " ")
	switch {
	case id <= 0:
		return errors.New("a Telegram user ID is a number more than zero")
	case len([]rune(name)) > MaxName:
		return errors.New("name is too long: use 40 characters or fewer")
	}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"name"}),
	}).Create(&User{ID: id, Name: name}).Error
}

// Deny removes a person from the list.
func (s *Store) Deny(id int64) error {
	result := s.db.Delete(&User{}, "id = ?", id)
	if result.Error == nil && result.RowsAffected == 0 {
		return ErrNotFound
	}
	return result.Error
}

// Users returns the allowed people, the lowest ID first.
func (s *Store) Users() ([]User, error) {
	var users []User
	return users, s.db.Order("id").Find(&users).Error
}

// Allowed reports whether the person with the ID can use the bot. It
// is false when the list cannot be read.
func (s *Store) Allowed(id int64) bool {
	var count int64
	err := s.db.Model(&User{}).Where("id = ?", id).Count(&count).Error
	return err == nil && count > 0
}

// Once records the reminder with the key. It reports whether this is
// the first time: the caller sends the reminder only then.
func (s *Store) Once(key string) (bool, error) {
	result := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&Notice{Key: key, SentAt: time.Now()})
	return result.Error == nil && result.RowsAffected > 0, result.Error
}
