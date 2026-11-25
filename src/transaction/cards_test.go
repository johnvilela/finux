package transaction

import (
	"strings"
	"testing"

	"finux/src/card"
)

func (f fixture) purchase(title, cardCode, categoryCode string, value int64, date string) Draft {
	return Draft{Title: title, Type: Expense, Value: value, CardID: f.card[cardCode], CategoryID: f.category[categoryCode], Date: date}
}

func (f fixture) visa(t *testing.T) card.Card {
	t.Helper()
	found, err := f.cards.Find("VISA", true)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func (f fixture) bill(t *testing.T, id uint) card.Bill {
	t.Helper()
	found, err := f.cards.Bill(id)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func wantError(t *testing.T, name string, err error, text string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), text) {
		t.Fatalf("%s: error = %v, want %q", name, err, text)
	}
}

func TestCardTransactionGetsItsBill(t *testing.T) {
	f := newFixture(t)
	before := f.balance(t, "INTER")
	market := f.create(t, f.purchase("Market", "VISA", "FOOD", 10000, "2026-11-17"))
	if market.CardID != f.card["VISA"] || market.AccountID != 0 || market.BillID == 0 || market.Amount != -10000 {
		t.Fatalf("purchase = %+v", market)
	}
	refund := f.purchase("Refund", "VISA", "OTHER", 2500, "2026-11-20")
	refund.Type = Income
	if created := f.create(t, refund); created.BillID != market.BillID || created.Amount != 2500 {
		t.Fatalf("refund = %+v", created)
	}
	later := f.create(t, f.purchase("Shoes", "VISA", "OTHER", 30000, "2026-11-26"))
	if later.BillID == market.BillID {
		t.Fatal("a purchase after the closing day is in the same bill")
	}

	first := f.bill(t, market.BillID)
	if first.Period != "2026-12" || first.Spent != 7500 || first.Total() != 7500 {
		t.Fatalf("first bill = %+v", first)
	}
	if visa := f.visa(t); visa.Used != 37500 || visa.Available() != 462500 {
		t.Fatalf("card used = %d, available = %d", visa.Used, visa.Available())
	}
	if got := f.balance(t, "INTER"); got != before {
		t.Fatalf("a purchase changed an account balance: %d, want %d", got, before)
	}

	view, err := f.store.Find(market.ID)
	if err != nil || view.Card == nil || view.Card.Code != "VISA" || view.Bill == nil || view.Bill.Period != "2026-12" ||
		view.Account.Code != "VISA" || view.Account.Currency != "BRL" || view.Account.ID != 0 {
		t.Fatalf("view = %+v, %v", view, err)
	}
	found, err := f.store.List(Filter{Account: "VISA"})
	if err != nil || len(found) != 3 {
		t.Fatalf("List(@VISA) = %d transactions, %v", len(found), err)
	}
	inBill, err := f.store.ByBill(market.BillID)
	if err != nil || len(inBill) != 2 || inBill[0].Title != "Refund" {
		t.Fatalf("ByBill = %+v, %v", inBill, err)
	}
}

func TestCardTransactionRules(t *testing.T) {
	f := newFixture(t)
	both := f.purchase("Market", "VISA", "FOOD", 100, "2026-11-17")
	both.AccountID = f.account["INTER"]
	_, err := f.store.Create(both)
	wantError(t, "account and card", err, "an account or a credit card")

	transfer := f.purchase("Move", "VISA", "XFER", 100, "2026-11-17")
	transfer.Type, transfer.ToAccountID = Transfer, f.account["INTER"]
	_, err = f.store.Create(transfer)
	wantError(t, "transfer from a card", err, "credit card cannot have a transfer")

	payment := f.draft("Pay", Payment, "INTER", "XFER", 100)
	_, err = f.store.Create(payment)
	wantError(t, "payment by hand", err, "unknown type")

	if err := f.cards.Archive(f.card["VISA"]); err != nil {
		t.Fatal(err)
	}
	_, err = f.store.Create(f.purchase("Market", "VISA", "FOOD", 100, "2026-11-17"))
	wantError(t, "archived card", err, "credit card not found")
}

func TestCardTransactionUpdate(t *testing.T) {
	f := newFixture(t)
	market := f.create(t, f.purchase("Market", "VISA", "FOOD", 10000, "2026-11-17"))

	// A new date moves the transaction to the bill of that date.
	draft := DraftOf(market)
	draft.Date = "2026-11-30"
	moved, err := f.store.Update(market.ID, draft)
	if err != nil || moved.BillID == market.BillID || moved.CardID != market.CardID {
		t.Fatalf("moved = %+v, %v", moved, err)
	}
	if old := f.bill(t, market.BillID); old.Spent != 0 {
		t.Fatalf("old bill spent = %d", old.Spent)
	}

	// A card transaction can go to an account, and back.
	before := f.balance(t, "INTER")
	draft.CardID, draft.AccountID = 0, f.account["INTER"]
	inAccount, err := f.store.Update(market.ID, draft)
	if err != nil || inAccount.CardID != 0 || inAccount.BillID != 0 || inAccount.AccountID != f.account["INTER"] {
		t.Fatalf("in account = %+v, %v", inAccount, err)
	}
	if got := f.balance(t, "INTER"); got != before-10000 {
		t.Fatalf("balance = %d, want %d", got, before-10000)
	}
	draft.CardID, draft.AccountID = f.card["VISA"], 0
	back, err := f.store.Update(market.ID, draft)
	if err != nil || back.CardID != f.card["VISA"] || back.AccountID != 0 || back.BillID != moved.BillID {
		t.Fatalf("back in card = %+v, %v", back, err)
	}

	// A transfer cannot go to a card.
	transfer := f.draft("Move", Transfer, "INTER", "XFER", 500)
	transfer.ToAccountID = f.account["NUBAN"]
	created := f.create(t, transfer)
	change := DraftOf(created)
	change.AccountID, change.CardID = 0, f.card["VISA"]
	_, err = f.store.Update(created.ID, change)
	wantError(t, "transfer to card", err, "transfer")
}

func TestPayBill(t *testing.T) {
	f := newFixture(t)
	market := f.create(t, f.purchase("Market", "VISA", "FOOD", 10000, "2026-11-17"))
	f.create(t, f.purchase("Fuel", "VISA", "OTHER", 5000, "2026-11-20"))
	before := f.balance(t, "INTER")

	_, err := f.store.PayBill(market.BillID, f.account["INTER"], 0, "2026-11-25")
	wantError(t, "open bill", err, "is still open")
	_, err = f.store.PayBill(market.BillID, f.account["INTER"], 15001, "2026-12-01")
	wantError(t, "too much", err, "more than the total")
	_, err = f.store.PayBill(market.BillID, f.account["WISE"], 0, "2026-12-01")
	wantError(t, "other currency", err, "USD")
	_, err = f.store.PayBill(99, f.account["INTER"], 0, "2026-12-01")
	wantError(t, "no bill", err, "bill not found")

	// The value 0 pays the total.
	payment, err := f.store.PayBill(market.BillID, f.account["INTER"], 0, "01/12/2026")
	if err != nil {
		t.Fatal(err)
	}
	if payment.Type != Payment || payment.Amount != -15000 || payment.AccountID != f.account["INTER"] || payment.CardID != 0 ||
		payment.BillID != market.BillID || payment.Title != "Payment VISA 12/2026" || payment.CategoryID != f.category["XFER"] || payment.Date != "2026-12-01" {
		t.Fatalf("payment = %+v", payment)
	}
	paid := f.bill(t, market.BillID)
	if paid.StatusOn("2027-01-01") != card.StatusPaid || paid.PaidOn != "2026-12-01" || paid.Paid != 15000 || paid.Remaining != 0 || paid.PaymentID != payment.ID || paid.CarriedTo != 0 {
		t.Fatalf("paid bill = %+v", paid)
	}
	if got := f.balance(t, "INTER"); got != before-15000 {
		t.Fatalf("balance = %d, want %d", got, before-15000)
	}
	if visa := f.visa(t); visa.Used != 0 {
		t.Fatalf("card used = %d, want 0", visa.Used)
	}
	_, err = f.store.PayBill(market.BillID, f.account["INTER"], 0, "2026-12-02")
	wantError(t, "second payment", err, "already paid")

	view, err := f.store.Find(payment.ID)
	if err != nil || view.Account.Code != "INTER" || view.Card == nil || view.Card.Code != "VISA" || view.Bill == nil || view.Other == nil || view.Other.Code != "VISA" {
		t.Fatalf("payment view = %+v, %v", view, err)
	}
}

func TestPaidBillIsLocked(t *testing.T) {
	f := newFixture(t)
	market := f.create(t, f.purchase("Market", "VISA", "FOOD", 10000, "2026-11-17"))
	payment, err := f.store.PayBill(market.BillID, f.account["INTER"], 0, "2026-12-01")
	if err != nil {
		t.Fatal(err)
	}
	const locked = "bill 12/2026 of VISA is paid"
	_, err = f.store.Create(f.purchase("Late", "VISA", "FOOD", 100, "2026-11-18"))
	wantError(t, "create", err, locked)
	draft := DraftOf(market)
	draft.Title = "Changed"
	_, err = f.store.Update(market.ID, draft)
	wantError(t, "update", err, locked)
	wantError(t, "delete", f.store.Delete(market.ID), locked)

	other := f.create(t, f.purchase("Shoes", "VISA", "OTHER", 300, "2026-12-10"))
	into := DraftOf(other)
	into.Date = "2026-11-18"
	_, err = f.store.Update(other.ID, into)
	wantError(t, "move into a paid bill", err, locked)

	_, err = f.store.Update(payment.ID, DraftOf(payment))
	wantError(t, "edit a payment", err, "payment cannot change")

	// Delete of the payment opens the bill again.
	before := f.balance(t, "INTER")
	if err := f.store.Delete(payment.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.balance(t, "INTER"); got != before+10000 {
		t.Fatalf("balance = %d, want %d", got, before+10000)
	}
	if bill := f.bill(t, market.BillID); bill.IsPaid() || bill.Paid != 0 || bill.PaymentID != 0 {
		t.Fatalf("bill after delete of the payment = %+v", bill)
	}
	if err := f.store.Delete(market.ID); err != nil {
		t.Fatalf("delete after unpay: %v", err)
	}
}

func TestPartialPaymentCarriesTheRest(t *testing.T) {
	f := newFixture(t)
	market := f.create(t, f.purchase("Market", "VISA", "FOOD", 10000, "2026-11-17"))
	payment, err := f.store.PayBill(market.BillID, f.account["INTER"], 6000, "2026-12-01")
	if err != nil || payment.Amount != -6000 {
		t.Fatalf("payment = %+v, %v", payment, err)
	}
	paid := f.bill(t, market.BillID)
	if !paid.IsPaid() || paid.Paid != 6000 || paid.Remaining != 4000 || paid.CarriedTo == 0 {
		t.Fatalf("paid bill = %+v", paid)
	}
	next := f.bill(t, paid.CarriedTo)
	if next.Period != "2027-01" || next.Carried != 4000 || next.Total() != 4000 || next.IsPaid() {
		t.Fatalf("next bill = %+v", next)
	}
	if visa := f.visa(t); visa.Used != 4000 {
		t.Fatalf("card used = %d, want 4000", visa.Used)
	}

	// A paid next bill blocks the delete of the first payment.
	second, err := f.store.PayBill(next.ID, f.account["INTER"], 0, "2027-01-02")
	if err != nil || second.Amount != -4000 {
		t.Fatalf("second payment = %+v, %v", second, err)
	}
	wantError(t, "unpay with a paid next bill", f.store.Unpay(paid.ID), "bill 01/2027 of VISA is paid")
	if err := f.store.Unpay(next.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Unpay(paid.ID); err != nil {
		t.Fatal(err)
	}
	if next = f.bill(t, next.ID); next.Carried != 0 {
		t.Fatalf("next bill carried = %d after unpay", next.Carried)
	}
	if _, err := f.store.Find(payment.ID); err == nil {
		t.Fatal("payment transaction stays after unpay")
	}
	wantError(t, "unpay twice", f.store.Unpay(paid.ID), "is not paid")
}

func TestBillWithCreditIsPaidWithoutTransaction(t *testing.T) {
	f := newFixture(t)
	refund := f.purchase("Refund", "VISA", "OTHER", 2500, "2026-11-17")
	refund.Type = Income
	created := f.create(t, refund)
	payment, err := f.store.PayBill(created.BillID, f.account["INTER"], 0, "2026-12-01")
	if err != nil || payment.ID != 0 {
		t.Fatalf("payment = %+v, %v", payment, err)
	}
	paid := f.bill(t, created.BillID)
	if !paid.IsPaid() || paid.Remaining != -2500 || paid.PaymentID != 0 {
		t.Fatalf("paid bill = %+v", paid)
	}
	if next := f.bill(t, paid.CarriedTo); next.Carried != -2500 {
		t.Fatalf("next bill = %+v", next)
	}
}

func TestPurgeCard(t *testing.T) {
	f := newFixture(t)
	market := f.create(t, f.purchase("Market", "VISA", "FOOD", 10000, "2026-11-17"))
	f.create(t, f.purchase("Shoes", "VISA", "OTHER", 300, "2026-12-10"))
	payment, err := f.store.PayBill(market.BillID, f.account["INTER"], 0, "2026-12-01")
	if err != nil {
		t.Fatal(err)
	}
	purchases, payments, err := f.store.CardUsage(f.card["VISA"])
	if err != nil || purchases != 2 || payments != 1 {
		t.Fatalf("CardUsage = %d, %d, %v", purchases, payments, err)
	}
	before := f.balance(t, "INTER")
	other, err := f.categories.Find("OTHER")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.PurgeCard(f.visa(t), other); err != nil {
		t.Fatal(err)
	}
	all, err := f.store.List(Filter{})
	if err != nil || len(all) != 1 {
		t.Fatalf("after purge: %d transactions, %v", len(all), err)
	}
	kept := all[0]
	if kept.ID != payment.ID || kept.Type != Expense || kept.BillID != 0 || kept.Category.Code != "OTHER" ||
		kept.Description != "Payment of VISA (deleted card)" || kept.Amount != -10000 {
		t.Fatalf("kept payment = %+v", kept.Transaction)
	}
	if got := f.balance(t, "INTER"); got != before {
		t.Fatalf("balance = %d, want %d", got, before)
	}
	if bills, err := f.cards.Bills(f.card["VISA"]); err != nil || len(bills) != 0 {
		t.Fatalf("bills after purge = %d, %v", len(bills), err)
	}
}

func TestPurgeAccountKeepsBillsPaid(t *testing.T) {
	f := newFixture(t)
	market := f.create(t, f.purchase("Market", "VISA", "FOOD", 10000, "2026-11-17"))
	if _, err := f.store.PayBill(market.BillID, f.account["INTER"], 0, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	if plain, _, err := f.store.AccountUsage(f.account["INTER"]); err != nil || plain != 1 {
		t.Fatalf("AccountUsage plain = %d, %v", plain, err)
	}
	inter, err := f.accounts.Find("INTER", true, fixtureToday)
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.categories.Find("OTHER")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.PurgeAccount(inter, other); err != nil {
		t.Fatal(err)
	}
	if bill := f.bill(t, market.BillID); !bill.IsPaid() || bill.PaymentID != 0 {
		t.Fatalf("bill after the account purge = %+v", bill)
	}
	if err := f.store.Unpay(market.BillID); err != nil {
		t.Fatalf("unpay without a payment transaction: %v", err)
	}
}

func TestPaymentCategoryRules(t *testing.T) {
	f := newFixture(t)
	market := f.create(t, f.purchase("Market", "VISA", "FOOD", 10000, "2026-11-17"))
	if _, err := f.store.PayBill(market.BillID, f.account["INTER"], 0, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	// A payment is a grey type: XFER cannot become red or green.
	if count, err := f.store.Mismatched(f.category["XFER"], "red"); err != nil || count != 1 {
		t.Fatalf("Mismatched(XFER, red) = %d, %v", count, err)
	}
	if count, err := f.store.Mismatched(f.category["XFER"], "grey"); err != nil || count != 0 {
		t.Fatalf("Mismatched(XFER, grey) = %d, %v", count, err)
	}
}

func TestVirtualCard(t *testing.T) {
	f := newFixture(t)
	// Only a transaction in a credit card can have the mark.
	inAccount := f.draft("Market", Expense, "INTER", "FOOD", 100)
	inAccount.Virtual = true
	_, err := f.store.Create(inAccount)
	wantError(t, "virtual in an account", err, "only a credit card transaction can be made with a virtual card")

	draft := f.purchase("Shop", "VISA", "SHOP", 9000, "2026-11-17")
	draft.Virtual = true
	shop := f.create(t, draft)
	if stored, err := f.store.Find(shop.ID); err != nil || !shop.Virtual || !stored.Virtual || !DraftOf(stored.Transaction).Virtual {
		t.Fatalf("created = %+v, stored = %+v, %v", shop, stored, err)
	}
	if plain := f.create(t, f.purchase("Market", "VISA", "FOOD", 100, "2026-11-17")); plain.Virtual {
		t.Fatalf("a purchase with no mark = %+v", plain)
	}

	// An edit keeps the mark, and can take it away.
	next := DraftOf(shop)
	next.Title = "Online shop"
	if updated, err := f.store.Update(shop.ID, next); err != nil || !updated.Virtual {
		t.Fatalf("update that keeps the mark = %+v, %v", updated, err)
	}
	next.Virtual = false
	if _, err := f.store.Update(shop.ID, next); err != nil {
		t.Fatal(err)
	}
	if stored, err := f.store.Find(shop.ID); err != nil || stored.Virtual {
		t.Fatalf("after the mark is gone = %+v, %v", stored, err)
	}
	next.Virtual = true
	if _, err := f.store.Update(shop.ID, next); err != nil {
		t.Fatal(err)
	}
	// A transaction with the mark cannot go to an account.
	next.CardID, next.AccountID = 0, f.account["INTER"]
	_, err = f.store.Update(shop.ID, next)
	wantError(t, "virtual moved to an account", err, "only a credit card transaction")

	// Each part of a purchase in installments has the mark.
	parts := f.purchase("Phone", "VISA", "SHOP", 3000, "2026-11-17")
	parts.Virtual, parts.Installments = true, 3
	phone := f.parts(t, f.create(t, parts).ID)
	for _, part := range phone {
		if !part.Virtual {
			t.Fatalf("part %d has no mark", part.Part)
		}
	}
	// A change of the mark goes to the parts of the scope.
	change := DraftOf(phone[1])
	change.Virtual = false
	if _, err := f.store.UpdateParts(phone[1].ID, Next, change, Changed{Virtual: true}); err != nil {
		t.Fatal(err)
	}
	phone = f.parts(t, phone[0].ID)
	if !phone[0].Virtual || phone[1].Virtual || phone[2].Virtual {
		t.Fatalf("marks after a change of the next parts = %v %v %v", phone[0].Virtual, phone[1].Virtual, phone[2].Virtual)
	}
}

func TestOffLimitCard(t *testing.T) {
	f := newFixture(t)
	t.Setenv("FINUX_TODAY", "2026-11-17")
	// Only a transaction in a credit card can have the mark.
	inAccount := f.draft("Repair", Expense, "INTER", "OTHER", 100)
	inAccount.OffLimit = true
	_, err := f.store.Create(inAccount)
	wantError(t, "off limit in an account", err, "only a credit card transaction can be outside the limit")

	f.create(t, f.purchase("Market", "VISA", "FOOD", 10000, "2026-11-17"))
	// The purchase is in the bill after the current one.
	draft := f.purchase("Repair", "VISA", "OTHER", 30000, "2026-11-26")
	draft.OffLimit = true
	repair := f.create(t, draft)
	if stored, err := f.store.Find(repair.ID); err != nil || !repair.OffLimit || !stored.OffLimit || !DraftOf(stored.Transaction).OffLimit {
		t.Fatalf("created = %+v, stored = %+v, %v", repair, stored, err)
	}

	// The bill has the purchase. The limit in use does not, while the
	// bill is a future one.
	if bill := f.bill(t, repair.BillID); bill.Spent != 30000 || bill.Total() != 30000 || bill.OffLimit != 30000 {
		t.Fatalf("bill = %+v", bill)
	}
	if visa := f.visa(t); visa.Used != 10000 || visa.OffLimit != 30000 || visa.Available() != 490000 {
		t.Fatalf("card used = %d, off limit = %d, available = %d", visa.Used, visa.OffLimit, visa.Available())
	}
	// The purchase uses the limit from the day its bill is the current one.
	t.Setenv("FINUX_TODAY", "2026-11-26")
	if visa := f.visa(t); visa.Used != 40000 || visa.OffLimit != 0 {
		t.Fatalf("in the current bill: used = %d, off limit = %d", visa.Used, visa.OffLimit)
	}
	t.Setenv("FINUX_TODAY", "2026-11-17")

	// An edit keeps the mark, and can take it away.
	next := DraftOf(repair)
	next.Title = "Car repair"
	if updated, err := f.store.Update(repair.ID, next); err != nil || !updated.OffLimit {
		t.Fatalf("update that keeps the mark = %+v, %v", updated, err)
	}
	next.OffLimit = false
	if _, err := f.store.Update(repair.ID, next); err != nil {
		t.Fatal(err)
	}
	if visa := f.visa(t); visa.Used != 40000 || visa.OffLimit != 0 {
		t.Fatalf("after the mark is gone: used = %d, off limit = %d", visa.Used, visa.OffLimit)
	}
	next.OffLimit = true
	if _, err := f.store.Update(repair.ID, next); err != nil {
		t.Fatal(err)
	}
	// A transaction with the mark cannot go to an account.
	moved := next
	moved.CardID, moved.AccountID = 0, f.account["INTER"]
	_, err = f.store.Update(repair.ID, moved)
	wantError(t, "off limit moved to an account", err, "only a credit card transaction")

	// The payment of the bill has the purchase.
	payment, err := f.store.PayBill(repair.BillID, f.account["INTER"], 0, "2027-01-02")
	if err != nil || payment.Amount != -30000 {
		t.Fatalf("payment = %+v, %v", payment, err)
	}
	if visa := f.visa(t); visa.Used != 10000 || visa.OffLimit != 0 {
		t.Fatalf("after the payment: used = %d, off limit = %d", visa.Used, visa.OffLimit)
	}
}

func TestOffLimitInstallments(t *testing.T) {
	f := newFixture(t)
	t.Setenv("FINUX_TODAY", "2026-11-17")
	draft := f.purchase("Car repair", "VISA", "OTHER", 120000, "2026-11-17")
	draft.OffLimit, draft.Installments = true, 10
	car := f.parts(t, f.create(t, draft).ID)
	marks := func(want bool) {
		t.Helper()
		for _, part := range f.parts(t, car[0].ID) {
			if part.OffLimit != want {
				t.Fatalf("part %d: off limit = %v, want %v", part.Part, part.OffLimit, want)
			}
		}
	}
	marks(true)
	// The part in the current bill uses the limit. The parts in the
	// future bills do not.
	if visa := f.visa(t); visa.Used != 12000 || visa.OffLimit != 108000 {
		t.Fatalf("card used = %d, off limit = %d", visa.Used, visa.OffLimit)
	}
	// Two months later, the bills of the parts 2 and 3 are not future ones.
	t.Setenv("FINUX_TODAY", "2027-01-10")
	if visa := f.visa(t); visa.Used != 36000 || visa.OffLimit != 84000 {
		t.Fatalf("two months later: used = %d, off limit = %d", visa.Used, visa.OffLimit)
	}
	t.Setenv("FINUX_TODAY", "2026-11-17")
	if _, err := f.store.PayBill(car[0].BillID, f.account["INTER"], 0, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	if visa := f.visa(t); visa.Used != 0 || visa.OffLimit != 108000 {
		t.Fatalf("after one payment: used = %d, off limit = %d", visa.Used, visa.OffLimit)
	}

	// The mark is of the purchase: a change of one part goes to all the
	// parts. A paid bill does not stop it.
	change := DraftOf(car[0])
	change.OffLimit = false
	if _, err := f.store.Update(car[0].ID, change); err != nil {
		t.Fatalf("change of the mark of a part in a paid bill: %v", err)
	}
	marks(false)
	if visa := f.visa(t); visa.Used != 108000 || visa.OffLimit != 0 {
		t.Fatalf("after the mark is gone: used = %d, off limit = %d", visa.Used, visa.OffLimit)
	}
	change = DraftOf(car[4])
	change.OffLimit = true
	if _, err := f.store.UpdateParts(car[4].ID, Next, change, Changed{OffLimit: true}); err != nil {
		t.Fatal(err)
	}
	marks(true)
	change = DraftOf(car[0])
	change.OffLimit = false
	if _, err := f.store.UpdateParts(car[0].ID, All, change, Changed{OffLimit: true}); err != nil {
		t.Fatalf("change of the mark of all the parts, one in a paid bill: %v", err)
	}
	marks(false)

	// The other values of a part in a paid bill stay locked.
	change.Title = "Changed"
	_, err := f.store.Update(car[0].ID, change)
	wantError(t, "title in a paid bill", err, "is paid")
}
