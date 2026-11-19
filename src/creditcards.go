package main

import (
	"errors"
	"flag"
	"fmt"
	"slices"
	"strings"

	"finux/src/account"
	"finux/src/card"
	"finux/src/category"
	"finux/src/code"
	"finux/src/money"
	"finux/src/schedule"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

func creditCardsHelp() string {
	return "Manage credit cards\n\n" +
		green.Render("Usage:") + " finux credit-card [command] [arguments]\n" +
		"       finux cc [command] [arguments]\n\n" +
		green.Render("Commands:") + `
  list, l [--all]              List credit cards; select one to see its details (default)
  <code|id>                    Show a credit card and the transactions of its open bill
  show <code|id>               Same as <code|id>
  add, a [options]             Create a credit card
  edit, e [code|id] [options]  Change a credit card
  bills, b [code|id]           List the bills of a credit card
  pay, p [code|id] [options]   Pay a bill from an account
  unpay [code|id] [options]    Delete the payment of a bill
  delete, d [code|id]          Archive a credit card
  restore, r [code|id]         Restore an archived credit card
  help, --help, -h             Show this help

` + green.Render("Options for list:") + `
  --all                 Include archived credit cards

` + green.Render("Options for add:") + `
  --name <text>         Name of the credit card
  --code <code>         1 to 5 letters and digits (default: made from the name);
                        an account cannot have the same code
  --description <text>  Description
  --currency <code>     ` + strings.Join(money.Codes(), ", ") + `
  --limit <amount>      Credit limit, for example 5000
  --closing <day>       Day of the month the bill closes, 1 to 31
  --due <day>           Day of the month the bill is due, 1 to 31
  --color <name>        ` + strings.Join(account.ColorKeys()[:8], ", ") + `,
                        ` + strings.Join(account.ColorKeys()[8:], ", ") + `

` + green.Render("Options for edit:") + `
  --name, --code, --description, --color, --limit, --closing, --due

` + green.Render("Options for pay:") + `
  --from <code>         Account that pays the bill
  --value <amount>      Value to pay (default: the total of the bill)
  --date <date>         Date of the payment (default: today)
  --bill <MM/YYYY>      Bill to pay (default: the oldest bill that is closed or late)

` + green.Render("Options for unpay:") + `
  --bill <MM/YYYY>      Bill (default: the last bill that is paid)
  --yes                 Do not ask for confirmation

` + green.Render("Options for delete:") + `
  --hard                Delete permanently, with the bills and the transactions
                        of the credit card; also works on archived credit cards
  --yes                 Do not ask for confirmation

Without options, add and edit open a form.
Without a code or id, the commands open a selector.

To add a purchase, use 'finux tx add --account <code of the card>'. A credit
card has expenses (purchases) and incomes (refunds), no transfers.

Each transaction goes to a bill. A transaction on or before the closing day
goes to the bill that closes in that month; a later one goes to the next bill.
A bill has the name of the month it is due. It is open until its closing
date, then closed until its due date, then late, until you pay it.

A payment is a transaction of the type payment in the account. With a value
smaller than the total, the bill is paid and the remaining value goes to the
next bill. A paid bill is locked: to change its transactions, delete its
payment first ('finux cc unpay' or 'finux tx delete' of the payment).

New closing and due days apply to the bills that do not exist yet.
A permanent delete keeps each payment in its account, as an expense of the
category OTHER.
`
}

// creditCardsCommand runs one credit-card subcommand.
type creditCardsCommand struct {
	streams
	db           *gorm.DB
	store        *card.Store
	transactions *transaction.Store
}

func runCreditCards(args []string, io streams) int {
	command := &creditCardsCommand{streams: io}
	handlers := map[string]func([]string) error{
		"list": command.list, "l": command.list,
		"add": command.add, "a": command.add,
		"show": command.show,
		"edit": command.edit, "e": command.edit,
		"bills": command.bills, "b": command.bills,
		"pay": command.pay, "p": command.pay,
		"unpay":  command.unpay,
		"delete": command.delete, "d": command.delete,
		"restore": command.restore, "r": command.restore,
	}
	// No argument or an option lists credit cards. A word that is not a
	// command is the code or id of a credit card to show.
	handler, rest := command.list, args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if known, ok := handlers[args[0]]; ok {
			handler, rest = known, args[1:]
		} else {
			handler = command.show
		}
	}
	// Help needs no database, so look for it before the database opens.
	for i, arg := range rest {
		if arg == "-h" || arg == "--help" || i == 0 && isHelp(arg) {
			return write(io, creditCardsHelp())
		}
	}
	db, err := openLedger(io)
	if err != nil {
		return exitStatus(io, "credit-card", err)
	}
	command.db, command.store, command.transactions = db, card.NewStore(db), transaction.NewStore(db)
	return exitStatus(io, "credit-card", handler(rest))
}

func (c *creditCardsCommand) print(text string) error {
	_, err := fmt.Fprint(c.stdout, text)
	return err
}

// resolve finds the credit card for a code or id. Without a reference
// it opens a selector with the credit cards of the scope.
func (c *creditCardsCommand) resolve(command, ref, title string, in scope) (card.Card, error) {
	if ref != "" {
		found, err := c.store.Find(ref, true)
		if errors.Is(err, card.ErrNotFound) {
			return found, fmt.Errorf("credit card not found: %s", ref)
		}
		return found, err
	}
	if !c.interactive {
		return card.Card{}, usageError(command + ": a credit card code or id is necessary when input is not a terminal")
	}
	var cards []card.Card
	var err error
	empty := notice("No credit cards yet. Run 'finux cc add'.")
	switch in {
	case scopeActive:
		cards, err = c.store.List(false)
	case scopeAll:
		cards, err = c.store.List(true)
	case scopeArchived:
		cards, err = c.store.ListArchived()
		empty = "No archived credit cards."
	}
	if err != nil {
		return card.Card{}, err
	}
	if len(cards) == 0 {
		return card.Card{}, empty
	}
	index, err := pick(title, "credit card", "credit cards", ui.CardEntries(cards))
	if err != nil {
		return card.Card{}, err
	}
	return cards[index], nil
}

func (c *creditCardsCommand) taken(exceptID uint) func(string) bool {
	return func(code string) bool {
		taken, err := c.store.CodeTaken(code, exceptID)
		return err == nil && taken
	}
}

// details renders a credit card with its open bill and the bills that
// wait for a payment.
func (c *creditCardsCommand) details(current card.Card) (string, error) {
	details, err := c.gather(current)
	if err != nil {
		return "", err
	}
	return ui.CardDetailsView(details, c.plain), nil
}

// gather reads the open bill of a credit card, its transactions and the
// bills that wait for a payment.
func (c *creditCardsCommand) gather(current card.Card) (ui.CardDetails, error) {
	details := ui.CardDetails{Card: current, Today: today()}
	var err error
	if details.Bill, err = c.store.Current(current, details.Today); err != nil {
		return details, err
	}
	if details.Bill.ID != 0 {
		if details.Transactions, err = c.transactions.ByBill(details.Bill.ID); err != nil {
			return details, err
		}
	}
	bills, err := c.store.Bills(current.ID)
	if err != nil {
		return details, err
	}
	// The oldest bill comes first: it is the most urgent.
	slices.Reverse(bills)
	for _, bill := range bills {
		if status := bill.Status(details.Today); bill.ID != details.Bill.ID && (status == card.StatusClosed || status == card.StatusLate) && bill.Total() != 0 {
			details.Pending = append(details.Pending, bill)
		}
	}
	return details, nil
}

func (c *creditCardsCommand) list(args []string) error {
	var all bool
	if _, _, err := parse("list", args, func(f *flag.FlagSet) { f.BoolVar(&all, "all", false, "") }, 0); err != nil {
		return err
	}
	cards, err := c.store.List(all)
	if err != nil {
		return err
	}
	if len(cards) == 0 || !c.interactive {
		return c.print(ui.CardTable(cards, c.plain))
	}
	index, err := pick("Credit cards", "credit card", "credit cards", ui.CardEntries(cards))
	if errors.Is(err, cancelled) {
		// Leaving the list is not worth a message.
		return nil
	}
	if err != nil {
		return err
	}
	text, err := c.details(cards[index])
	if err != nil {
		return err
	}
	return c.print(text)
}

func (c *creditCardsCommand) show(args []string) error {
	positional, _, err := parse("show", args, nil, 1)
	if err != nil {
		return err
	}
	found, err := c.resolve("show", reference(positional), "Show credit card", scopeActive)
	if err != nil {
		return err
	}
	text, err := c.details(found)
	if err != nil {
		return err
	}
	return c.print(text)
}

func (c *creditCardsCommand) bills(args []string) error {
	positional, _, err := parse("bills", args, nil, 1)
	if err != nil {
		return err
	}
	found, err := c.resolve("bills", reference(positional), "Bills of credit card", scopeAll)
	if err != nil {
		return err
	}
	bills, err := c.store.Bills(found.ID)
	if err != nil {
		return err
	}
	return c.print(ui.BillTable(found, bills, today(), c.plain))
}

// cardFlags holds the text of the options of add and edit.
type cardFlags struct {
	name, code, description, currency, limit, color string
	closing, due                                    int
}

func (v *cardFlags) define(flags *flag.FlagSet, create bool) {
	flags.StringVar(&v.name, "name", "", "")
	flags.StringVar(&v.code, "code", "", "")
	flags.StringVar(&v.description, "description", "", "")
	flags.StringVar(&v.limit, "limit", "", "")
	flags.IntVar(&v.closing, "closing", 0, "")
	flags.IntVar(&v.due, "due", 0, "")
	flags.StringVar(&v.color, "color", "", "")
	if create {
		flags.StringVar(&v.currency, "currency", "", "")
	}
}

// apply validates the options that were given and writes them to the
// credit card. The limit needs the currency of the card.
func (v *cardFlags) apply(command string, given map[string]bool, current *card.Card) error {
	var err error
	if given["name"] {
		if current.Name = strings.TrimSpace(v.name); current.Name == "" {
			err = errors.New("name is empty")
		}
	}
	if err == nil && given["code"] {
		current.Code = code.Normalize(v.code)
		err = code.Validate(current.Code)
	}
	if given["description"] {
		current.Description = strings.TrimSpace(v.description)
	}
	if err == nil && given["color"] {
		current.Color = strings.ToLower(strings.TrimSpace(v.color))
		if _, ok := account.ColorByKey(current.Color); !ok {
			err = fmt.Errorf("unknown color %q: use one of %s", current.Color, strings.Join(account.ColorKeys(), ", "))
		}
	}
	if err == nil && given["currency"] {
		if current.Currency = strings.ToUpper(strings.TrimSpace(v.currency)); !money.Valid(current.Currency) {
			err = fmt.Errorf("unknown currency %q: use one of %s", current.Currency, strings.Join(money.Codes(), ", "))
		}
	}
	if err == nil && given["closing"] {
		current.ClosingDay = v.closing
		err = card.CheckDay("closing day", v.closing)
	}
	if err == nil && given["due"] {
		current.DueDay = v.due
		err = card.CheckDay("due day", v.due)
	}
	if err == nil && given["limit"] && current.Currency != "" {
		if current.Limit, err = money.Parse(v.limit, current.Currency); err == nil && current.Limit < 0 {
			err = errors.New("limit cannot be negative")
		}
	}
	if err != nil {
		return usageError(command + ": " + err.Error())
	}
	return nil
}

func (c *creditCardsCommand) add(args []string) error {
	var values cardFlags
	_, given, err := parse("add", args, func(f *flag.FlagSet) { values.define(f, true) }, 0)
	if err != nil {
		return err
	}
	var draft card.Card
	if err := values.apply("add", given, &draft); err != nil {
		return err
	}
	if !given["color"] {
		if draft.Color, err = c.unusedColor(); err != nil {
			return err
		}
	}
	if given["name"] && given["currency"] && given["limit"] && given["closing"] && given["due"] {
		if !given["code"] {
			if draft.Code = code.Suggest(draft.Name, c.taken(0)); draft.Code == "" {
				return usageError(fmt.Sprintf("add: cannot make a code from the name %q; use --code", draft.Name))
			}
		}
	} else {
		if !c.interactive {
			return usageError("add: --name, --currency, --limit, --closing and --due are necessary when input is not a terminal")
		}
		var preset []ui.Field
		for flagName, field := range map[string]ui.Field{
			"name": ui.FieldName, "code": ui.FieldCode, "description": ui.FieldDescription, "currency": ui.FieldCurrency,
			"closing": ui.FieldClosing, "due": ui.FieldDue, "color": ui.FieldColor,
		} {
			if given[flagName] {
				preset = append(preset, field)
			}
		}
		// The limit is final only with its currency.
		if given["limit"] && given["currency"] {
			preset = append(preset, ui.FieldLimit)
		}
		form, err := program(ui.NewCardForm(ui.CardFormOptions{Title: "New credit card", Initial: draft, Limit: values.limit, Preset: preset, Taken: c.taken(0)}))
		if err != nil {
			return err
		}
		if !form.Submitted() {
			return cancelled
		}
		draft = form.Card()
	}
	if err := c.store.Create(&draft); err != nil {
		return err
	}
	return c.print(ui.Success("Credit card "+draft.Code+" created") + "\n")
}

// unusedColor returns the first palette color that no active credit
// card uses, or the first color if all are in use.
func (c *creditCardsCommand) unusedColor() (string, error) {
	cards, err := c.store.List(false)
	if err != nil {
		return "", err
	}
	used := map[string]bool{}
	for _, current := range cards {
		used[current.Color] = true
	}
	for _, color := range account.Palette {
		if !used[color.Key] {
			return color.Key, nil
		}
	}
	return account.Palette[0].Key, nil
}

func (c *creditCardsCommand) edit(args []string) error {
	var values cardFlags
	positional, given, err := parse("edit", args, func(f *flag.FlagSet) { values.define(f, false) }, 1)
	if err != nil {
		return err
	}
	// Check the options before the selector opens. The limit waits for
	// the currency of the card.
	var probe card.Card
	if err := values.apply("edit", given, &probe); err != nil {
		return err
	}
	if len(given) == 0 && !c.interactive {
		return usageError("edit: give --name, --code, --description, --color, --limit, --closing or --due when input is not a terminal")
	}
	current, err := c.resolve("edit", reference(positional), "Edit credit card", scopeActive)
	if err != nil {
		return err
	}
	if current.Archived() {
		return fmt.Errorf("credit card not found: %s (it is archived; restore it first)", current.Code)
	}
	if len(given) == 0 {
		form, err := program(ui.NewCardForm(ui.CardFormOptions{
			Title: "Edit " + current.Code, Initial: current, Limit: money.Input(current.Limit, current.Currency), Edit: true, Taken: c.taken(current.ID),
		}))
		if err != nil {
			return err
		}
		if !form.Submitted() {
			return cancelled
		}
		current = form.Card()
	}
	if err := values.apply("edit", given, &current); err != nil {
		return err
	}
	if err := c.store.Update(&current); err != nil {
		return err
	}
	return c.print(ui.Success("Credit card "+current.Code+" updated") + "\n")
}

// billName names a bill in a message: "bill 12/2026 of VISA".
func billName(owner card.Card, bill card.Bill) string {
	return "bill " + card.FormatPeriod(bill.Period) + " of " + owner.Code
}

// bill finds the bill of a credit card for the text of a --bill option.
func (c *creditCardsCommand) bill(command string, owner card.Card, text string) (card.Bill, error) {
	period, err := card.ParsePeriod(text)
	if err != nil {
		return card.Bill{}, usageError(command + ": " + err.Error())
	}
	found, err := c.store.Lookup(owner.ID, period)
	if errors.Is(err, card.ErrBillNotFound) {
		return found, fmt.Errorf("bill %s of %s not found", card.FormatPeriod(period), owner.Code)
	}
	return found, err
}

// waiting returns the oldest bill of a credit card that waits for a
// payment on the given date: a bill that is closed or late.
func (c *creditCardsCommand) waiting(owner card.Card, date string) (card.Bill, error) {
	bills, err := c.store.Bills(owner.ID)
	if err != nil {
		return card.Bill{}, err
	}
	slices.Reverse(bills)
	index := slices.IndexFunc(bills, func(current card.Bill) bool {
		status := current.Status(date)
		return status == card.StatusClosed || status == card.StatusLate
	})
	if index < 0 {
		return card.Bill{}, fmt.Errorf("no bill of %s waits for a payment. To pay a different bill, use --bill", owner.Code)
	}
	return bills[index], nil
}

// lastPaid returns the newest paid bill of a credit card.
func (c *creditCardsCommand) lastPaid(owner card.Card) (card.Bill, error) {
	bills, err := c.store.Bills(owner.ID)
	if err != nil {
		return card.Bill{}, err
	}
	index := slices.IndexFunc(bills, card.Bill.IsPaid)
	if index < 0 {
		return card.Bill{}, fmt.Errorf("%s has no paid bill", owner.Code)
	}
	return bills[index], nil
}

func (c *creditCardsCommand) pay(args []string) error {
	var from, value, date, billRef string
	positional, given, err := parse("pay", args, func(f *flag.FlagSet) {
		f.StringVar(&from, "from", "", "")
		f.StringVar(&value, "value", "", "")
		f.StringVar(&date, "date", "", "")
		f.StringVar(&billRef, "bill", "", "")
	}, 1)
	if err != nil {
		return err
	}
	if !given["date"] {
		date = today()
	} else if date, err = transaction.ParseDate(date); err != nil {
		return usageError("pay: " + err.Error())
	}
	if !given["from"] && !c.interactive {
		return usageError("pay: --from is necessary when input is not a terminal")
	}
	owner, err := c.resolve("pay", reference(positional), "Pay a bill of credit card", scopeAll)
	if err != nil {
		return err
	}

	var bill card.Bill
	if given["bill"] {
		if bill, err = c.bill("pay", owner, billRef); err != nil {
			return err
		}
	} else if bill, err = c.waiting(owner, date); err != nil {
		return err
	}

	accounts := account.NewStore(c.db)
	var payer account.Account
	if given["from"] {
		if payer, err = accounts.Find(from, false, today()); errors.Is(err, account.ErrNotFound) {
			return fmt.Errorf("account not found: %s", from)
		} else if err != nil {
			return err
		}
	} else {
		active, err := accounts.List(false, today())
		if err != nil {
			return err
		}
		active = slices.DeleteFunc(active, func(current account.Account) bool { return current.Currency != owner.Currency })
		if len(active) == 0 {
			return notice("No account in " + owner.Currency + " to pay the bill. Run 'finux ac add'.")
		}
		title := fmt.Sprintf("Pay %s (%s) from", billName(owner, bill), money.Format(bill.Total(), owner.Currency))
		index, err := pick(title, "account", "accounts", ui.AccountEntries(active))
		if err != nil {
			return err
		}
		payer = active[index]
	}

	var amountToPay int64
	if given["value"] {
		if amountToPay, err = amount("pay", value, owner.Currency); err != nil {
			return err
		}
	}
	payment, err := c.transactions.PayBill(bill.ID, payer.ID, amountToPay, date)
	if err != nil {
		return err
	}
	if bill, err = c.store.Bill(bill.ID); err != nil {
		return err
	}
	message := "B" + billName(owner, bill)[1:] + " paid: "
	if payment.ID != 0 {
		message += fmt.Sprintf("transaction %d", payment.ID)
	} else {
		message += "nothing to pay"
	}
	if bill.CarriedTo != 0 {
		next, err := c.store.Bill(bill.CarriedTo)
		if err != nil {
			return err
		}
		what := money.Format(bill.Remaining, owner.Currency)
		if bill.Remaining < 0 {
			what = money.Format(-bill.Remaining, owner.Currency) + " of credit"
		}
		message += "; " + what + " carried to bill " + card.FormatPeriod(next.Period)
	}
	return c.print(ui.Success(message) + "\n")
}

func (c *creditCardsCommand) unpay(args []string) error {
	var billRef string
	var yes bool
	positional, given, err := parse("unpay", args, func(f *flag.FlagSet) {
		f.StringVar(&billRef, "bill", "", "")
		f.BoolVar(&yes, "yes", false, "")
	}, 1)
	if err != nil {
		return err
	}
	if !yes && !c.interactive {
		return usageError("unpay: --yes is necessary when input is not a terminal")
	}
	owner, err := c.resolve("unpay", reference(positional), "Delete a payment of credit card", scopeAll)
	if err != nil {
		return err
	}
	var bill card.Bill
	if given["bill"] {
		if bill, err = c.bill("unpay", owner, billRef); err != nil {
			return err
		}
	} else if bill, err = c.lastPaid(owner); err != nil {
		return err
	}
	if !yes {
		question := fmt.Sprintf("Delete the payment of %s (%s)? The bill opens again.", billName(owner, bill), money.Format(bill.Paid, owner.Currency))
		confirm, err := program(ui.NewConfirm(question, "Delete payment", true))
		if err != nil {
			return err
		}
		if !confirm.Confirmed() {
			return cancelled
		}
	}
	if err := c.transactions.Unpay(bill.ID); err != nil {
		return err
	}
	return c.print(ui.Success("Payment of "+billName(owner, bill)+" deleted") + "\n")
}

func (c *creditCardsCommand) delete(args []string) error {
	var hard, yes bool
	positional, _, err := parse("delete", args, func(f *flag.FlagSet) {
		f.BoolVar(&hard, "hard", false, "")
		f.BoolVar(&yes, "yes", false, "")
	}, 1)
	if err != nil {
		return err
	}
	if !yes && !c.interactive {
		return usageError("delete: --yes is necessary when input is not a terminal")
	}
	in, title := scopeActive, "Archive credit card"
	if hard {
		in, title = scopeAll, "Delete credit card permanently"
	}
	current, err := c.resolve("delete", reference(positional), title, in)
	if err != nil {
		return err
	}
	if current.Archived() && !hard {
		return fmt.Errorf("credit card %s is already archived; use --hard to delete it permanently", current.Code)
	}
	// A permanent delete also deletes the bills and the transactions.
	var purchases, payments int64
	if hard {
		if purchases, payments, err = c.transactions.CardUsage(current.ID); err != nil {
			return err
		}
	}
	usage := ""
	if purchases+payments > 0 {
		usage = ", with " + plural(purchases, "transaction") + " and " + plural(payments, "payment")
	}
	if !yes {
		question, label := fmt.Sprintf("Archive credit card %s (%s)?", current.Code, current.Name), "Archive"
		if hard {
			question, label = fmt.Sprintf("Delete credit card %s (%s) permanently%s?", current.Code, current.Name, usage), "Delete permanently"
		}
		confirm, err := program(ui.NewConfirm(question, label, hard))
		if err != nil {
			return err
		}
		if !confirm.Confirmed() {
			return cancelled
		}
	}
	if hard {
		if err := c.purge(current, payments > 0); err != nil {
			return err
		}
		return c.print(ui.Success("Credit card "+current.Code+" deleted permanently"+usage) + "\n")
	}
	if err := c.store.Archive(current.ID); err != nil {
		return err
	}
	return c.print(ui.Success("Credit card "+current.Code+" archived") + "\n")
}

// purge deletes a credit card, its bills and its transactions in one
// step. Each payment of its bills goes to the category OTHER.
func (c *creditCardsCommand) purge(current card.Card, hasPayments bool) error {
	return c.db.Transaction(func(tx *gorm.DB) error {
		var other category.Category
		if hasPayments {
			var err error
			if other, err = otherCategory(tx); err != nil {
				return fmt.Errorf("credit card %s has payments: %w", current.Code, err)
			}
		}
		if err := transaction.NewStore(tx).PurgeCard(current, other); err != nil {
			return err
		}
		if err := schedule.NewStore(tx).DeleteByCard(current.ID); err != nil {
			return err
		}
		return card.NewStore(tx).Delete(current.ID)
	})
}

func (c *creditCardsCommand) restore(args []string) error {
	positional, _, err := parse("restore", args, nil, 1)
	if err != nil {
		return err
	}
	current, err := c.resolve("restore", reference(positional), "Restore credit card", scopeArchived)
	if err != nil {
		return err
	}
	if !current.Archived() {
		return fmt.Errorf("credit card %s is not archived", current.Code)
	}
	if err := c.store.Restore(current.ID); err != nil {
		return err
	}
	return c.print(ui.Success("Credit card "+current.Code+" restored") + "\n")
}
