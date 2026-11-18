package category

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"finux/src/code"
	"finux/src/storage"
)

func TestColors(t *testing.T) {
	want := []Color{{"red", "#EF4444", "expense"}, {"green", "#22C55E", "income"}, {"grey", "#9CA3AF", "neutral"}}
	if len(Colors) != len(want) {
		t.Fatalf("Colors = %+v, want %+v", Colors, want)
	}
	for i, color := range want {
		if Colors[i] != color {
			t.Fatalf("Colors[%d] = %+v, want %+v", i, Colors[i], color)
		}
		if got, ok := ColorByKey(color.Key); !ok || got != color {
			t.Fatalf("ColorByKey(%q) = %+v, %v", color.Key, got, ok)
		}
	}
	if _, ok := ColorByKey("blue"); ok {
		t.Fatal("ColorByKey accepts unknown color")
	}
	if got := strings.Join(ColorKeys(), ","); got != "red,green,grey" {
		t.Fatalf("ColorKeys = %s", got)
	}
	if got := (Category{Color: "green"}).Meaning(); got != "income" {
		t.Fatalf("Meaning = %q, want income", got)
	}
}

func TestValidate(t *testing.T) {
	valid := Category{Name: "Food", Code: "FOOD", Color: "red"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid category: %v", err)
	}
	for name, change := range map[string]func(*Category){
		"empty name":    func(c *Category) { c.Name = " " },
		"bad code":      func(c *Category) { c.Code = "food!" },
		"unknown color": func(c *Category) { c.Color = "blue" },
		"no color":      func(c *Category) { c.Color = "" },
	} {
		category := valid
		change(&category)
		if err := category.Validate(); err == nil {
			t.Errorf("%s: Validate() = nil, want error", name)
		}
	}
}

func TestBase(t *testing.T) {
	if len(Base) != 14 {
		t.Fatalf("Base has %d categories, want 14", len(Base))
	}
	codes, counts := map[string]bool{}, map[string]int{}
	for _, category := range Base {
		if err := category.Validate(); err != nil {
			t.Errorf("base category %+v: %v", category, err)
		}
		if codes[category.Code] || category.ID != 0 || category.Description == "" || code.Normalize(category.Code) != category.Code {
			t.Errorf("base category %+v: duplicate code, id set, no description or code not normalized", category)
		}
		codes[category.Code] = true
		counts[category.Color]++
	}
	if counts["red"] != 9 || counts["green"] != 3 || counts["grey"] != 2 {
		t.Fatalf("base colors = %v, want 9 red, 3 green, 2 grey", counts)
	}
	for _, want := range []string{"FOOD", "HOME", "TRANS", "HEALT", "EDU", "FUN", "SHOP", "BILLS", "TAX", "SAL", "FREE", "INV", "XFER", "OTHER"} {
		if !codes[want] {
			t.Errorf("base categories lack %s", want)
		}
	}
}

func newStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finux.db")
	if _, err := storage.Init(path, &Category{}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

func TestStoreCRUD(t *testing.T) {
	store := newStore(t)
	pets := Category{Name: "  Pets ", Description: " vet ", Code: " pets ", Color: "red"}
	if err := store.Create(&pets); err != nil {
		t.Fatal(err)
	}
	if pets.ID == 0 || pets.Name != "Pets" || pets.Code != "PETS" || pets.Description != "vet" || pets.CreatedAt.IsZero() || pets.UpdatedAt.IsZero() {
		t.Fatalf("created category not normalized: %+v", pets)
	}
	gifts := Category{Name: "gifts", Code: "GIFT", Color: "green"}
	if err := store.Create(&gifts); err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{"PETS", "pets", "1"} {
		if got, err := store.Find(reference); err != nil || got.ID != pets.ID {
			t.Fatalf("Find(%q) = %+v, %v", reference, got, err)
		}
	}
	for _, reference := range []string{"NOPE", "99", ""} {
		if _, err := store.Find(reference); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Find(%q) error = %v, want ErrNotFound", reference, err)
		}
	}
	duplicate := Category{Name: "Other", Code: "pets", Color: "grey"}
	if err := store.Create(&duplicate); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate code error = %v, want ErrCodeTaken", err)
	}
	if err := store.Create(&Category{Name: "", Code: "X", Color: "red"}); err == nil {
		t.Fatal("invalid category created")
	}
	if taken, err := store.CodeTaken("pets", 0); err != nil || !taken {
		t.Fatalf("CodeTaken(pets) = %v, %v", taken, err)
	}
	if taken, err := store.CodeTaken("PETS", pets.ID); err != nil || taken {
		t.Fatalf("CodeTaken for the category itself = %v, %v", taken, err)
	}

	list, err := store.List()
	if err != nil || len(list) != 2 || list[0].Code != "GIFT" || list[1].Code != "PETS" {
		t.Fatalf("List = %+v, %v; want GIFT then PETS (by name)", list, err)
	}

	pets.Name, pets.Code, pets.Description, pets.Color = "Animals", "anim", "", "grey"
	if err := store.Update(&pets); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Find("ANIM"); err != nil || got.Name != "Animals" || got.Description != "" || got.Color != "grey" {
		t.Fatalf("after update = %+v, %v", got, err)
	}
	gifts.Code = "ANIM"
	if err := store.Update(&gifts); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("update to taken code error = %v, want ErrCodeTaken", err)
	}

	if err := store.Delete(pets.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Find("ANIM"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted category found: %v", err)
	}
	if err := store.Delete(pets.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete error = %v, want ErrNotFound", err)
	}
	if err := store.Create(&Category{Name: "Reuse", Code: "ANIM", Color: "red"}); err != nil {
		t.Fatalf("code free after delete: %v", err)
	}
}

func TestStoreAddMissingBase(t *testing.T) {
	store := newStore(t)
	added, err := store.AddMissingBase()
	if err != nil || len(added) != 14 || added[0] != "FOOD" || added[13] != "OTHER" {
		t.Fatalf("first AddMissingBase = %v, %v; want the 14 base codes in order", added, err)
	}
	if added, err := store.AddMissingBase(); err != nil || len(added) != 0 {
		t.Fatalf("second AddMissingBase = %v, %v; want nothing", added, err)
	}

	food, _ := store.Find("FOOD")
	tax, _ := store.Find("TAX")
	if err := store.Delete(food.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(tax.ID); err != nil {
		t.Fatal(err)
	}
	home, _ := store.Find("HOME")
	home.Name, home.Color = "My home", "grey"
	if err := store.Update(&home); err != nil {
		t.Fatal(err)
	}
	custom := Category{Name: "Pets", Code: "PETS", Color: "red"}
	if err := store.Create(&custom); err != nil {
		t.Fatal(err)
	}

	added, err = store.AddMissingBase()
	if err != nil || strings.Join(added, ",") != "FOOD,TAX" {
		t.Fatalf("AddMissingBase after deletes = %v, %v; want FOOD,TAX", added, err)
	}
	if got, _ := store.Find("HOME"); got.Name != "My home" || got.Color != "grey" {
		t.Fatalf("AddMissingBase changed an existing category: %+v", got)
	}
	if list, _ := store.List(); len(list) != 15 {
		t.Fatalf("total = %d, want 15 (14 base and 1 custom)", len(list))
	}
	if len(Base) != 14 || Base[0].ID != 0 {
		t.Fatalf("AddMissingBase changed the Base list: %+v", Base[0])
	}
}
