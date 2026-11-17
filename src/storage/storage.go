// Package storage opens the Finux SQLite database.
package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Dir returns the directory that holds Finux data: FINUX_HOME if set,
// otherwise ~/.finux.
func Dir() (string, error) {
	if dir := os.Getenv("FINUX_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".finux"), nil
}

// Open creates dir if necessary, opens finux.db in it and migrates models.
func Open(dir string, models ...any) (*gorm.DB, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	path := filepath.Join(dir, "finux.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	if err := db.AutoMigrate(models...); err != nil {
		return nil, fmt.Errorf("migrate database %s: %w", path, err)
	}
	return db, nil
}
