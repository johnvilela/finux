package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FINUX_HOME", "")
	if got, err := Dir(); err != nil || got != filepath.Join(home, ".finux") {
		t.Fatalf("Dir() = %q, %v; want ~/.finux", got, err)
	}
	custom := filepath.Join(home, "custom")
	t.Setenv("FINUX_HOME", custom)
	if got, err := Dir(); err != nil || got != custom {
		t.Fatalf("Dir() = %q, %v; want FINUX_HOME", got, err)
	}
}

func TestOpenCreatesDatabase(t *testing.T) {
	type note struct {
		ID   uint
		Text string
	}
	dir := filepath.Join(t.TempDir(), "nested", "finux")
	db, err := Open(dir, &note{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&note{Text: "hello"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "finux.db")); err != nil {
		t.Fatalf("database file: %v", err)
	}
	reopened, err := Open(dir, &note{})
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := reopened.Model(&note{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("count = %d, %v; want 1", count, err)
	}
}
