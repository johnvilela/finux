// Package storage creates and opens the Finux SQLite database.
package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ErrNotInitialized means the database file does not exist yet.
var ErrNotInitialized = errors.New("database not found")

// Path returns the database file: FINUX_DB if set, otherwise
// ~/.finux/finux.db.
func Path() (string, error) {
	if path := os.Getenv("FINUX_DB"); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".finux", "finux.db"), nil
}

// Init creates the database file and its directory if necessary, then
// migrates models. It keeps existing data. It reports whether it created
// the file.
func Init(path string, models ...any) (created bool, err error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		created = true
	} else if err != nil {
		return false, fmt.Errorf("check database %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, fmt.Errorf("create database directory: %w", err)
	}
	db, err := connect(path)
	if err != nil {
		return false, err
	}
	if err := db.AutoMigrate(models...); err != nil {
		return false, fmt.Errorf("migrate database %s: %w", path, err)
	}
	return created, nil
}

// Open opens a database that Init created. It does not create or migrate
// the database.
func Open(path string) (*gorm.DB, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotInitialized, path)
	} else if err != nil {
		return nil, fmt.Errorf("check database %s: %w", path, err)
	}
	return connect(path)
}

func connect(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	return db, nil
}

// Pending lists what the database lacks for models: tables, columns and
// indexes that Init would create. An empty list means the database is up
// to date. It does not change the database.
func Pending(db *gorm.DB, models ...any) ([]string, error) {
	var pending []string
	migrator := db.Migrator()
	for _, model := range models {
		statement := &gorm.Statement{DB: db}
		if err := statement.Parse(model); err != nil {
			return nil, fmt.Errorf("read model %T: %w", model, err)
		}
		table := statement.Schema.Table
		if !migrator.HasTable(model) {
			pending = append(pending, "table "+table)
			continue
		}
		for _, field := range statement.Schema.Fields {
			if field.DBName != "" && !migrator.HasColumn(model, field.DBName) {
				pending = append(pending, "column "+table+"."+field.DBName)
			}
		}
		for _, index := range statement.Schema.ParseIndexes() {
			if !migrator.HasIndex(model, index.Name) {
				pending = append(pending, "index "+index.Name)
			}
		}
	}
	return pending, nil
}
