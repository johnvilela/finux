package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCategoriesLifecycle(t *testing.T) {
	home := filepath.Join(t.TempDir(), "data")
	fail(t, home, 1, "Run 'finux init'", "ct")
	succeed(t, home, "init")

	list := succeed(t, home, "ct")
	for _, want := range []string{"CODE   NAME         TYPE\n", "FOOD   Food         expense\n", "SAL    Salary       income\n", "XFER   Transfer     neutral\n", "\n14 categories\n"} {
		if !strings.Contains(list, want) {
			t.Fatalf("list lacks %q:\n%s", want, list)
		}
	}
	for _, args := range [][]string{{"ct", "list"}, {"categories", "l"}, {"categories"}} {
		if got := succeed(t, home, args...); got != list {
			t.Fatalf("finux %v differs from 'finux ct':\n%s", args, got)
		}
	}

	for _, args := range [][]string{{"ct", "FOOD"}, {"categories", "food"}, {"ct", "1"}, {"ct", "show", "FOOD"}} {
		card := succeed(t, home, args...)
		for _, want := range []string{"FOOD  Food\n", "Groceries and restaurants\n", "ID        1\n", "Type      expense\n", "Color     red\n", "Created   ", "Updated   "} {
			if !strings.Contains(card, want) {
				t.Fatalf("finux %v lacks %q:\n%s", args, want, card)
			}
		}
	}

	if got := succeed(t, home, "ct", "a", "--name", "Pet Care", "--color", "RED", "--description", "vet"); got != "✓ Category PCARE created\n" {
		t.Fatalf("add output = %q", got)
	}
	if got := succeed(t, home, "categories", "add", "--name", "Gifts", "--color", "green", "--code", "gift"); got != "✓ Category GIFT created\n" {
		t.Fatalf("add with code output = %q", got)
	}
	fail(t, home, 1, "already in use", "ct", "add", "--name", "Other", "--color", "grey", "--code", "gift")
	if got := succeed(t, home, "ct"); !strings.Contains(got, "\n16 categories\n") || !strings.Contains(got, "PCARE  Pet Care     expense\n") {
		t.Fatalf("list after add:\n%s", got)
	}

	if got := succeed(t, home, "ct", "e", "PCARE", "--name", "Pets", "--code", "pets", "--color", "grey", "--description", ""); got != "✓ Category PETS updated\n" {
		t.Fatalf("edit output = %q", got)
	}
	card := succeed(t, home, "ct", "PETS")
	if !strings.Contains(card, "PETS  Pets\n") || !strings.Contains(card, "Type      neutral\n") || strings.Contains(card, "vet") {
		t.Fatalf("after edit:\n%s", card)
	}
	fail(t, home, 1, "category not found: PCARE", "ct", "PCARE")
	fail(t, home, 1, "category not found: PCARE", "ct", "edit", "PCARE", "--name", "X")
	fail(t, home, 1, "already in use", "ct", "edit", "PETS", "--code", "FOOD")

	if got := succeed(t, home, "ct", "d", "PETS", "--yes"); got != "✓ Category PETS deleted\n" {
		t.Fatalf("delete output = %q", got)
	}
	fail(t, home, 1, "category not found: PETS", "ct", "PETS")
	fail(t, home, 1, "category not found: PETS", "ct", "delete", "PETS", "--yes")

	// A base category that the user deletes stays deleted after init.
	// Only reseed brings it back.
	succeed(t, home, "ct", "delete", "--yes", "FOOD")
	if got := succeed(t, home, "init"); strings.Contains(got, "Base categories") {
		t.Fatalf("second init added base categories:\n%s", got)
	}
	fail(t, home, 1, "category not found: FOOD", "ct", "FOOD")
	if got := succeed(t, home, "reseed"); got != "✓ Base categories added: 1 (FOOD)\n" {
		t.Fatalf("reseed = %q", got)
	}
	if got := succeed(t, home, "ct"); !strings.Contains(got, "FOOD   Food") || !strings.Contains(got, "GIFT   Gifts") || !strings.Contains(got, "\n15 categories\n") {
		t.Fatalf("list after reseed:\n%s", got)
	}
}

func TestCategoriesUsageErrors(t *testing.T) {
	home := t.TempDir()
	succeed(t, home, "init")
	for name, tc := range map[string]struct {
		want string
		args []string
	}{
		"unknown flag":             {"-currency", []string{"ct", "add", "--name", "X", "--color", "red", "--currency", "BRL"}},
		"unknown flag alone":       {"-all", []string{"ct", "--all"}},
		"add without color":        {"--color", []string{"ct", "add", "--name", "X"}},
		"add without name":         {"--name", []string{"ct", "add", "--color", "red"}},
		"add with nothing":         {"--name", []string{"ct", "a"}},
		"add empty name":           {"name", []string{"ct", "add", "--name", " ", "--color", "red"}},
		"add unknown color":        {"blue", []string{"ct", "add", "--name", "X", "--color", "blue"}},
		"add digits-only code":     {"at least one letter", []string{"ct", "add", "--name", "X", "--color", "red", "--code", "123"}},
		"add name without letters": {"--code", []string{"ct", "add", "--name", "123", "--color", "red"}},
		"add with argument":        {"unexpected argument", []string{"ct", "add", "extra", "--name", "X", "--color", "red"}},
		"reference with argument":  {"unexpected argument", []string{"ct", "FOOD", "extra"}},
		"show without reference":   {"code or id", []string{"ct", "show"}},
		"edit without reference":   {"code or id", []string{"ct", "edit", "--name", "X"}},
		"edit without options":     {"--name", []string{"ct", "edit", "FOOD"}},
		"edit bad code":            {"letters A-Z", []string{"ct", "edit", "FOOD", "--code", "a-b"}},
		"edit unknown color":       {"blue", []string{"ct", "e", "FOOD", "--color", "blue"}},
		"delete without yes":       {"--yes", []string{"ct", "delete", "FOOD"}},
		"delete without reference": {"code or id", []string{"ct", "d", "--yes"}},
		"list with argument":       {"unexpected argument", []string{"ct", "list", "extra"}},
	} {
		t.Run(name, func(t *testing.T) {
			fail(t, home, 2, tc.want, tc.args...)
			_, stderr, _ := finux(t, home, tc.args...)
			if !strings.HasPrefix(stderr, "finux: categories: ") || !strings.HasSuffix(stderr, "\nRun 'finux categories --help' for usage.\n") {
				t.Fatalf("stderr has no module name or usage hint: %q", stderr)
			}
		})
	}
	if got := succeed(t, home, "ct"); !strings.Contains(got, "\n14 categories\n") {
		t.Fatalf("failed commands changed data:\n%s", got)
	}
}

func TestCategoriesHelp(t *testing.T) {
	home := filepath.Join(t.TempDir(), "unused")
	want := succeed(t, home, "categories", "help")
	for _, text := range []string{"Usage: finux categories [command]", "finux ct [command]", "<code|id>", "list, l", "add, a", "edit, e", "delete, d", "show", "--name", "--code", "--description", "--color", "--yes", "red, green, grey", "expense", "income", "neutral", "finux reseed"} {
		if !strings.Contains(want, text) {
			t.Errorf("help lacks %q:\n%s", text, want)
		}
	}
	for _, unwanted := range []string{"restore", "--hard", "--all", "--currency"} {
		if strings.Contains(want, unwanted) {
			t.Errorf("categories help mentions %q:\n%s", unwanted, want)
		}
	}
	for _, args := range [][]string{{"ct", "--help"}, {"ct", "-h"}, {"ct", "help"}, {"categories", "-h"}, {"ct", "add", "--help"}, {"ct", "a", "help"}, {"ct", "l", "help"}} {
		if got := succeed(t, home, args...); got != want {
			t.Errorf("finux %v differs from 'finux categories help'", args)
		}
	}
}
