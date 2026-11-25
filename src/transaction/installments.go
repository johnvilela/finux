package transaction

import (
	"errors"
	"fmt"

	"finux/src/card"

	"gorm.io/gorm"
)

// Scope tells which parts of a transaction in installments an operation
// changes.
type Scope int

// The scopes: the part itself, the part and the parts after it, or all
// the parts.
const (
	One Scope = iota
	Next
	All
)

// Changed tells which values of a draft UpdateParts gives to the parts.
// Place is the account or the credit card. OffLimit goes to all the
// parts of the transaction, not only to the parts of the scope.
type Changed struct {
	Title, Description, Type, Category, Tags, Goal, Place, Value, Date, Virtual, OffLimit bool
}

// createParts inserts the parts of a transaction in installments, from
// the part From to the last one. The date of the draft is the date of
// the first part it creates; each part after it is one month later. On
// a credit card each part goes to the bill after the one before. A paid
// bill stops all the parts.
func (s *Store) createParts(draft Draft) (Transaction, error) {
	var owner card.Card
	var currency string
	if draft.CardID != 0 {
		found, err := s.card(draft.CardID, false)
		if err != nil {
			return Transaction{}, err
		}
		owner, currency = found, found.Currency
	} else {
		from, err := s.account(draft.AccountID, false)
		if err != nil {
			return Transaction{}, err
		}
		if draft.ToAccountID != 0 {
			return Transaction{}, errors.New("only a transfer has a destination account")
		}
		currency = from.Currency
	}
	if _, err := s.category(draft); err != nil {
		return Transaction{}, err
	}
	if err := s.linkable(draft.GoalID, currency, false); err != nil {
		return Transaction{}, err
	}
	values := split(draft.Value, draft.Installments)
	period, _, _ := owner.Cycle(draft.Date)
	var first Transaction
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// The bills come from the same database transaction, so a bill
		// that one part creates is there for the next part.
		bound := &Store{db: tx}
		for number := draft.From; number <= draft.Installments; number++ {
			part := Transaction{
				Title: draft.Title, Description: draft.Description, Type: draft.Type, Amount: signed(draft.Type, values[number-1]),
				AccountID: draft.AccountID, CardID: draft.CardID, CategoryID: draft.CategoryID, Tags: draft.Tags, GoalID: draft.GoalID, Virtual: draft.Virtual, OffLimit: draft.OffLimit,
				Date: monthsLater(draft.Date, number-draft.From), InstallmentID: first.ID, Part: number, Parts: draft.Installments,
			}
			if draft.CardID != 0 {
				bill, err := bound.billOf(owner, period)
				if err != nil {
					return fmt.Errorf("installment %s: %w", part.Installment(), err)
				}
				part.BillID, period = bill.ID, card.NextPeriod(period)
			}
			if err := tx.Create(&part).Error; err != nil {
				return err
			}
			if first.ID == 0 {
				part.InstallmentID = part.ID
				if err := tx.Model(&part).Update("InstallmentID", part.ID).Error; err != nil {
					return err
				}
				first = part
			}
		}
		return nil
	})
	if err != nil {
		return Transaction{}, err
	}
	return first, nil
}

// Parts returns the parts of the scope for the transaction id, the
// lowest number first. The scope One returns the transaction itself;
// the other scopes need a transaction in installments.
func (s *Store) Parts(id uint, scope Scope) ([]Transaction, error) {
	current, err := s.take(id)
	if err != nil {
		return nil, err
	}
	if scope == One {
		return []Transaction{current}, nil
	}
	if current.InstallmentID == 0 {
		return nil, fmt.Errorf("transaction %d is not an installment", id)
	}
	query := s.db.Where("installment_id = ?", current.InstallmentID)
	if scope == Next {
		query = query.Where("part >= ?", current.Part)
	}
	var parts []Transaction
	return parts, query.Order("part, id").Find(&parts).Error
}

// inPart adds the part to an error of one part of a scope.
func inPart(part Transaction, err error) error {
	return fmt.Errorf("installment %s (transaction %d): %w", part.Installment(), part.ID, err)
}

// UpdateParts changes the parts of the scope for the transaction id and
// returns them. It gives them the values of the draft that changed
// tells. A value is the new total of the parts of the scope. A date is
// the date of the first part of the scope: the others follow by month.
// On a credit card a new date or a new card puts the parts in bills one
// after the other again. A part in a paid bill stops the change of all
// the parts.
func (s *Store) UpdateParts(id uint, scope Scope, draft Draft, changed Changed) ([]Transaction, error) {
	if scope == One {
		updated, err := s.Update(id, draft)
		return []Transaction{updated}, err
	}
	parts, err := s.Parts(id, scope)
	if err != nil {
		return nil, err
	}
	if err := draft.clean(); err != nil {
		return nil, err
	}
	first := parts[0]
	var values []int64
	if changed.Value {
		if draft.Value < int64(len(parts)) {
			return nil, fmt.Errorf("value is too small for %d installments", len(parts))
		}
		values = split(draft.Value, len(parts))
	}
	cardID, start := first.CardID, first.Date
	if changed.Place {
		cardID = draft.CardID
	}
	if changed.Date {
		start = draft.Date
	}
	period := ""
	if cardID != 0 && (changed.Place || changed.Date) {
		owner, err := s.card(cardID, true)
		if err != nil {
			return nil, err
		}
		period, _, _ = owner.Cycle(start)
	}
	updated := make([]Transaction, 0, len(parts))
	err = s.db.Transaction(func(tx *gorm.DB) error {
		bound := &Store{db: tx, Actor: s.Actor}
		for i, part := range parts {
			next := DraftOf(part)
			if changed.Title {
				next.Title = draft.Title
			}
			if changed.Description {
				next.Description = draft.Description
			}
			if changed.Type {
				next.Type = draft.Type
			}
			if changed.Category {
				next.CategoryID = draft.CategoryID
			}
			if changed.Tags {
				next.Tags = draft.Tags
			}
			if changed.Goal {
				next.GoalID = draft.GoalID
			}
			if changed.Place {
				next.AccountID, next.CardID = draft.AccountID, draft.CardID
				// Only a credit card has virtual cards and a limit.
				if next.CardID == 0 {
					next.Virtual, next.OffLimit = false, false
				}
			}
			if changed.Virtual {
				next.Virtual = draft.Virtual
			}
			if changed.OffLimit {
				next.OffLimit = draft.OffLimit
			}
			if changed.Value {
				next.Value = values[i]
			}
			// A part that is gone leaves its month and its bill empty.
			offset := part.Part - first.Part
			if changed.Date {
				next.Date = monthsLater(draft.Date, offset)
			}
			target := period
			for range offset {
				if target != "" {
					target = card.NextPeriod(target)
				}
			}
			result, err := bound.update(part.ID, next, target)
			if err != nil {
				return inPart(part, err)
			}
			updated = append(updated, result)
		}
		if changed.OffLimit {
			if err := bound.spreadOffLimit(updated[0]); err != nil {
				return err
			}
		}
		// The parts are one purchase: they teach one time.
		return bound.learn(first, updated[0])
	})
	if err != nil {
		return nil, err
	}
	stamped := make([]*Transaction, len(updated))
	for i := range updated {
		stamped[i] = &updated[i]
	}
	return updated, s.stamp(nil, stamped...)
}

// DeleteParts removes the parts of the scope for the transaction id
// permanently and returns their ids. The other parts keep their
// numbers. A part in a paid bill stops the delete of all the parts.
func (s *Store) DeleteParts(id uint, scope Scope) ([]uint, error) {
	if scope == One {
		return []uint{id}, s.Delete(id)
	}
	parts, err := s.Parts(id, scope)
	if err != nil {
		return nil, err
	}
	ids := make([]uint, len(parts))
	for i, part := range parts {
		if part.CardID != 0 {
			if err := s.unlocked(part.BillID); err != nil {
				return nil, inPart(part, err)
			}
		}
		ids[i] = part.ID
	}
	return ids, s.stamp(s.db.Delete(&Transaction{}, ids).Error)
}
