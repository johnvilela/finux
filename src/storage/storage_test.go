package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type note struct {
	ID   uint
	Text string
}

func TestPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FINUX_DB", "")
	if got, err := Path(); err != nil || got != filepath.Join(home, ".finux", "finux.db") {
		t.Fatalf("Path() = %q, %v; want ~/.finux/finux.db", got, err)
	}
	custom := filepath.Join(home, "custom", "money.db")
	t.Setenv("FINUX_DB", custom)
	if got, err := Path(); err != nil || got != custom {
		t.Fatalf("Path() = %q, %v; want FINUX_DB", got, err)
	}
}

func TestInitCreatesAndMigrates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "finux.db")
	created, err := Init(path, &note{})
	if err != nil || !created {
		t.Fatalf("first Init = %v, %v; want created", created, err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&note{Text: "hello"}).Error; err != nil {
		t.Fatalf("table not migrated: %v", err)
	}
	created, err = Init(path, &note{})
	if err != nil || created {
		t.Fatalf("second Init = %v, %v; want not created", created, err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := reopened.Model(&note{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("count after second Init = %d, %v; want 1", count, err)
	}
}

func TestOpenNeedsInit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "finux.db")
	if _, err := Open(path); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("Open of missing database error = %v, want ErrNotInitialized", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("Open created the directory: %v", err)
	}
}

type oldEntry struct {
	ID uint
}

func (oldEntry) TableName() string { return "entries" }

type entry struct {
	ID    uint
	Label string `gorm:"index:idx_entries_label"`
}

func TestPending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := Init(path, &oldEntry{}); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Pending(db, &oldEntry{}); err != nil || len(got) != 0 {
		t.Fatalf("Pending for migrated model = %v, %v; want none", got, err)
	}
	got, err := Pending(db, &oldEntry{}, &entry{}, &note{})
	want := []string{"column entries.label", "index idx_entries_label", "table notes"}
	if err != nil || len(got) != len(want) {
		t.Fatalf("Pending = %v, %v; want %v", got, err, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Pending = %v, want %v", got, want)
		}
	}
	if _, err := Init(path, &entry{}, &note{}); err != nil {
		t.Fatal(err)
	}
	if got, err := Pending(db, &entry{}, &note{}); err != nil || len(got) != 0 {
		t.Fatalf("Pending after Init = %v, %v; want none", got, err)
	}
}
