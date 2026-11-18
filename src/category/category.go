// Package category holds the category model, its rules, its base data
// and its storage.
package category

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"finux/src/code"
)

// Color is a category color and what it means.
type Color struct {
	Key, Hex, Meaning string
}

// Colors is the fixed list of category colors. The color gives the type
// of the category.
var Colors = []Color{
	{"red", "#EF4444", "expense"},
	{"green", "#22C55E", "income"},
	{"grey", "#9CA3AF", "neutral"},
}

// ColorByKey finds a category color by its key.
func ColorByKey(key string) (Color, bool) {
	for _, color := range Colors {
		if color.Key == key {
			return color, true
		}
	}
	return Color{}, false
}

// ColorKeys returns the color keys in display order.
func ColorKeys() []string {
	keys := make([]string, len(Colors))
	for i, color := range Colors {
		keys[i] = color.Key
	}
	return keys
}

// Category groups transactions, for example Food or Salary.
type Category struct {
	ID          uint `gorm:"primaryKey"`
	Name        string
	Description string
	Code        string `gorm:"uniqueIndex;size:5"`
	Color       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Meaning returns what the color of the category means: expense, income
// or neutral. It is "" for an unknown color.
func (c Category) Meaning() string {
	color, _ := ColorByKey(c.Color)
	return color.Meaning
}

// Validate checks the rules that do not need the database.
func (c Category) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("name is required")
	}
	if err := code.Validate(c.Code); err != nil {
		return err
	}
	if _, ok := ColorByKey(c.Color); !ok {
		return fmt.Errorf("unknown color %q: use one of %s", c.Color, strings.Join(ColorKeys(), ", "))
	}
	return nil
}

// Codes of base categories that Finux uses by itself.
const (
	// OtherCode is the category that receives the transactions of a
	// deleted category.
	OtherCode = "OTHER"
	// TransferCode is the category a new transfer starts with.
	TransferCode = "XFER"
)

// Base is the set of categories a new database starts with.
var Base = []Category{
	{Name: "Food", Code: "FOOD", Color: "red", Description: "Groceries and restaurants"},
	{Name: "Housing", Code: "HOME", Color: "red", Description: "Rent, mortgage and maintenance"},
	{Name: "Transport", Code: "TRANS", Color: "red", Description: "Fuel, public transport and rides"},
	{Name: "Health", Code: "HEALT", Color: "red", Description: "Doctors, pharmacy and insurance"},
	{Name: "Education", Code: "EDU", Color: "red", Description: "Courses, books and tuition"},
	{Name: "Leisure", Code: "FUN", Color: "red", Description: "Entertainment, travel and hobbies"},
	{Name: "Shopping", Code: "SHOP", Color: "red", Description: "Clothes, electronics and other goods"},
	{Name: "Bills", Code: "BILLS", Color: "red", Description: "Electricity, water, internet and phone"},
	{Name: "Taxes", Code: "TAX", Color: "red", Description: "Taxes and government fees"},
	{Name: "Salary", Code: "SAL", Color: "green", Description: "Income from employment"},
	{Name: "Freelance", Code: "FREE", Color: "green", Description: "Income from independent work"},
	{Name: "Investments", Code: "INV", Color: "green", Description: "Dividends, interest and gains"},
	{Name: "Transfer", Code: "XFER", Color: "grey", Description: "Money moved between your accounts"},
	{Name: "Other", Code: "OTHER", Color: "grey", Description: "Anything that fits no other category"},
}
