package category

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"finux/src/code"

	"gorm.io/gorm"
)

var (
	// ErrNotFound means no category matches the id or code.
	ErrNotFound = errors.New("category not found")
	// ErrCodeTaken means another category has the code.
	ErrCodeTaken = errors.New("category code is already in use")
)

// Store reads and writes categories in the database.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store that uses db.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) prepare(category *Category) error {
	category.Name = strings.TrimSpace(category.Name)
	category.Description = strings.TrimSpace(category.Description)
	category.Code = code.Normalize(category.Code)
	if err := category.Validate(); err != nil {
		return err
	}
	taken, err := s.CodeTaken(category.Code, category.ID)
	if err != nil {
		return err
	}
	if taken {
		return fmt.Errorf("%w: %s", ErrCodeTaken, category.Code)
	}
	return nil
}

// CodeTaken reports whether a category other than exceptID has the code.
func (s *Store) CodeTaken(value string, exceptID uint) (bool, error) {
	var count int64
	err := s.db.Model(&Category{}).
		Where("code = ? AND id <> ?", code.Normalize(value), exceptID).Count(&count).Error
	return count > 0, err
}

// Create validates and inserts a category.
func (s *Store) Create(category *Category) error {
	if err := s.prepare(category); err != nil {
		return err
	}
	return s.db.Create(category).Error
}

// Update saves the name, code, description and color of a category.
func (s *Store) Update(category *Category) error {
	if err := s.prepare(category); err != nil {
		return err
	}
	return s.db.Model(category).Select("Name", "Code", "Description", "Color").Updates(category).Error
}

// List returns all categories ordered by name.
func (s *Store) List() ([]Category, error) {
	var categories []Category
	return categories, s.db.Order("name COLLATE NOCASE, id").Find(&categories).Error
}

// Find returns the category with the given id (all digits) or code.
func (s *Store) Find(reference string) (Category, error) {
	query := s.db
	reference = code.Normalize(reference)
	if id, err := strconv.ParseUint(reference, 10, 64); err == nil {
		query = query.Where("id = ?", id)
	} else {
		query = query.Where("code = ?", reference)
	}
	var category Category
	if err := query.Take(&category).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return Category{}, ErrNotFound
	} else if err != nil {
		return Category{}, err
	}
	return category, nil
}

// Delete removes a category permanently.
func (s *Store) Delete(id uint) error {
	result := s.db.Delete(&Category{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// AddMissingBase inserts each base category whose code is not in the
// database. It does not change existing categories. It returns the codes
// it added, in the order of Base.
func (s *Store) AddMissingBase() ([]string, error) {
	var added []string
	for _, base := range Base {
		taken, err := s.CodeTaken(base.Code, 0)
		if err != nil {
			return added, err
		}
		if taken {
			continue
		}
		if err := s.Create(&base); err != nil {
			return added, fmt.Errorf("create category %s: %w", base.Code, err)
		}
		added = append(added, base.Code)
	}
	return added, nil
}
