package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"finux/src/account"
	"finux/src/budget"
	"finux/src/card"
	"finux/src/category"
	"finux/src/code"
	"finux/src/goal"
	"finux/src/mcp"
	"finux/src/money"
	"finux/src/note"
	"finux/src/schedule"
	"finux/src/storage"
	"finux/src/transaction"

	"gorm.io/gorm"
)

// The MCP tools use the commands of the CLI as a library. quiet makes a
// command behave as it does without a terminal: no forms and no prompts.
var quiet = streams{stdout: io.Discard, stderr: io.Discard, plain: true}

// defaultPage is the number of transactions transaction_list returns
// when the agent gives no limit.
const defaultPage = 100

// --- Results -------------------------------------------------------------

// cashJSON is an amount of money in a result.
type cashJSON struct {
	Minor     int64  `json:"minor"`
	Decimal   string `json:"decimal"`
	Formatted string `json:"formatted"`
}

func cash(amount int64, currency string) cashJSON {
	return cashJSON{
		Minor:     amount,
		Decimal:   strings.Replace(money.Input(amount, currency), ",", ".", 1),
		Formatted: money.Format(amount, currency),
	}
}

type accountJSON struct {
	ID          uint   `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Currency    string `json:"currency"`
	Archived    bool   `json:"archived"`
	// Owner is the person the account belongs to. It is empty for an
	// account of the user.
	Owner          string   `json:"owner"`
	OpeningBalance cashJSON `json:"opening_balance"`
	Balance        cashJSON `json:"balance"`
}

func accountResult(a account.Account) accountJSON {
	return accountJSON{
		ID: a.ID, Code: a.Code, Name: a.Name, Description: a.Description, Color: a.Color, Currency: a.Currency,
		Archived: a.Archived(), Owner: a.Owner, OpeningBalance: cash(a.OpeningBalance, a.Currency), Balance: cash(a.Balance(), a.Currency),
	}
}

type categoryJSON struct {
	ID          uint   `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	// Accepts is the type of transaction the category can have: expense,
	// income or all.
	Accepts string `json:"accepts"`
}

func categoryResult(c category.Category) categoryJSON {
	accepts := c.Meaning()
	if c.Color == "grey" {
		accepts = "all"
	}
	return categoryJSON{ID: c.ID, Code: c.Code, Name: c.Name, Description: c.Description, Color: c.Color, Accepts: accepts}
}

type cardJSON struct {
	ID          uint   `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Currency    string `json:"currency"`
	Archived    bool   `json:"archived"`
	// Owner is the person the credit card belongs to. It is empty for a
	// credit card of the user.
	Owner      string   `json:"owner"`
	Limit      cashJSON `json:"limit"`
	Used       cashJSON `json:"used"`
	Available  cashJSON `json:"available"`
	ClosingDay int      `json:"closing_day"`
	DueDay     int      `json:"due_day"`
}

func cardResult(c card.Card) cardJSON {
	return cardJSON{
		ID: c.ID, Code: c.Code, Name: c.Name, Description: c.Description, Color: c.Color, Currency: c.Currency,
		Archived: c.Archived(), Owner: c.Owner, Limit: cash(c.Limit, c.Currency), Used: cash(c.Used, c.Currency),
		Available: cash(c.Available(), c.Currency), ClosingDay: c.ClosingDay, DueDay: c.DueDay,
	}
}

type billJSON struct {
	Period   string   `json:"period"`
	ClosesOn string   `json:"closes_on"`
	DueOn    string   `json:"due_on"`
	Status   string   `json:"status"`
	Carried  cashJSON `json:"carried"`
	Spent    cashJSON `json:"spent"`
	Total    cashJSON `json:"total"`
	// The fields below are only in a paid bill.
	PaidOn    string    `json:"paid_on,omitempty"`
	Paid      *cashJSON `json:"paid,omitempty"`
	Remaining *cashJSON `json:"remaining,omitempty"`
	PaymentID uint      `json:"payment_id,omitempty"`
	// Transactions are only in the open bill of credit_card_show.
	Transactions *[]transactionJSON `json:"transactions,omitempty"`
	// VirtualTotal is the sum of the transactions made with a virtual
	// card. It is only in an open bill that has them.
	VirtualTotal *cashJSON `json:"virtual_total,omitempty"`
}

func billResult(owner card.Card, bill card.Bill) billJSON {
	result := billJSON{
		Period: bill.Period, ClosesOn: bill.ClosesOn, DueOn: bill.DueOn, Status: bill.StatusOn(today()),
		Carried: cash(bill.Carried, owner.Currency), Spent: cash(bill.Spent, owner.Currency), Total: cash(bill.Total(), owner.Currency),
	}
	if bill.IsPaid() {
		paid, remaining := cash(bill.Paid, owner.Currency), cash(bill.Remaining, owner.Currency)
		result.PaidOn, result.Paid, result.Remaining, result.PaymentID = bill.PaidOn, &paid, &remaining, bill.PaymentID
	}
	return result
}

type transactionJSON struct {
	ID          uint     `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Amount      cashJSON `json:"amount"`
	Currency    string   `json:"currency"`
	Date        string   `json:"date"`
	// Status is pending for a date after today and cleared for the others.
	Status   string   `json:"status"`
	Tags     []string `json:"tags"`
	Category string   `json:"category"`
	// Account is the code of the account. A purchase of a credit card has
	// credit_card and bill in its place. A payment has the three.
	Account    string `json:"account,omitempty"`
	CreditCard string `json:"credit_card,omitempty"`
	Bill       string `json:"bill,omitempty"`
	// PairID and OtherAccount are the other side of a transfer.
	PairID       uint   `json:"pair_id,omitempty"`
	OtherAccount string `json:"other_account,omitempty"`
	// Goal is the code of the goal the transaction counts toward.
	Goal string `json:"goal,omitempty"`
	// Virtual is true for a purchase made with a virtual card.
	Virtual bool `json:"virtual,omitempty"`
	// Installment is the number of the part of a transaction in
	// installments, Installments is the number of parts and InstallmentID
	// is the same in all the parts.
	Installment   int  `json:"installment,omitempty"`
	Installments  int  `json:"installments,omitempty"`
	InstallmentID uint `json:"installment_id,omitempty"`
	// Schedule is the code of the schedule that created the transaction.
	Schedule string `json:"schedule,omitempty"`
}

type goalJSON struct {
	ID          uint     `json:"id"`
	Code        string   `json:"code"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Currency    string   `json:"currency"`
	Archived    bool     `json:"archived"`
	Target      cashJSON `json:"target"`
	Progress    cashJSON `json:"progress"`
	Remaining   cashJSON `json:"remaining"`
	Percent     int      `json:"percent"`
	// DueDate is only in a goal with a final date.
	DueDate string `json:"due_date,omitempty"`
	Status  string `json:"status"`
	// Transactions is the number of linked transactions.
	Transactions int64 `json:"transactions"`
}

func goalResult(g goal.Goal) goalJSON {
	return goalJSON{
		ID: g.ID, Code: g.Code, Title: g.Title, Description: g.Description, Type: g.Type, Currency: g.Currency,
		Archived: g.Archived(), Target: cash(g.Target, g.Currency), Progress: cash(g.Progress(), g.Currency),
		Remaining: cash(g.Remaining(), g.Currency), Percent: g.Percent(), DueDate: g.DueDate, Status: g.Status(today()),
		Transactions: g.Links,
	}
}

// usageJSON is one period of a budget: what it had and what it spent.
type usageJSON struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Carried is what the periods before left, below zero when they spent
	// too much. It is zero for a budget without rollover.
	Carried   cashJSON `json:"carried"`
	Available cashJSON `json:"available"`
	Spent     cashJSON `json:"spent"`
	Remaining cashJSON `json:"remaining"`
	Over      cashJSON `json:"over"`
	Percent   int      `json:"percent"`
	// Level is ok, near or over.
	Level string `json:"level"`
}

func usageResult(usage budget.Usage, currency string) usageJSON {
	return usageJSON{
		From: usage.From, To: usage.To, Carried: cash(usage.Carried, currency), Available: cash(usage.Available(), currency),
		Spent: cash(usage.Spent, currency), Remaining: cash(usage.Remaining(), currency), Over: cash(usage.Over(), currency),
		Percent: usage.Percent(), Level: usage.Level(),
	}
}

type budgetJSON struct {
	ID          uint     `json:"id"`
	Code        string   `json:"code"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Currency    string   `json:"currency"`
	Archived    bool     `json:"archived"`
	Limit       cashJSON `json:"limit"`
	// Categories are the codes of the categories that count. An empty
	// list means all categories but the income ones.
	Categories []string `json:"categories"`
	// Period is the length of a period in words: "every 2 weeks" or "one
	// time". Every and Unit are only in a recurring budget.
	Period    string `json:"period"`
	Every     int    `json:"every,omitempty"`
	Unit      string `json:"unit,omitempty"`
	StartDate string `json:"start_date"`
	// EndDate is only in a budget with an end date.
	EndDate  string `json:"end_date,omitempty"`
	Rollover bool   `json:"rollover"`
	// Status is the level of the current period, or upcoming or ended.
	Status string `json:"status"`
	// Current is the current period, or the last one of a budget that
	// ended. A budget that did not start has none.
	Current *usageJSON `json:"current,omitempty"`
}

func transactionResult(view transaction.View) transactionJSON {
	result := transactionJSON{
		ID: view.ID, Title: view.Title, Description: view.Description, Type: view.Type,
		Amount: cash(view.Amount, view.Account.Currency), Currency: view.Account.Currency, Date: view.Date, Status: view.Status,
		Tags: append([]string{}, view.TagList()...), Category: view.Category.Code, PairID: view.PairID,
		Installment: view.Part, Installments: view.Parts, InstallmentID: view.InstallmentID, Virtual: view.Virtual,
	}
	if view.CardID == 0 {
		result.Account = view.Account.Code
	}
	if view.Card != nil && view.Bill != nil {
		result.CreditCard, result.Bill = view.Card.Code, view.Bill.Period
	}
	if view.Other != nil && view.PairID != 0 {
		result.OtherAccount = view.Other.Code
	}
	if view.Goal != nil {
		result.Goal = view.Goal.Code
	}
	if view.Schedule != nil {
		result.Schedule = view.Schedule.Code
	}
	return result
}

func transactionResults(views []transaction.View) []transactionJSON {
	results := make([]transactionJSON, len(views))
	for i, view := range views {
		results[i] = transactionResult(view)
	}
	return results
}

// confirmation is the result of a tool that deletes data when the agent
// did not confirm. The tool changed nothing.
func confirmation(question string) map[string]any {
	return map[string]any{"confirm_required": true, "message": question + " Call the tool again with confirm: true to do it."}
}

// --- Plumbing ------------------------------------------------------------

// openReady opens a database that has all the tables the tools use.
func openReady() (*gorm.DB, error) {
	path, err := databasePath()
	if err != nil {
		return nil, err
	}
	db, err := storage.Open(path)
	if errors.Is(err, storage.ErrNotInitialized) {
		return nil, fmt.Errorf("%w. Call the tool database_init to create it", err)
	}
	if err == nil && !transactionsReady(db) {
		err = errors.New("the database is out of date. Call the tool database_init to update it")
	}
	return db, err
}

// agentError rewrites an error of the CLI for an agent: it removes the
// name of the command and the dashes of the options.
func agentError(err error) error {
	var usage usageError
	if !errors.As(err, &usage) {
		return err
	}
	text := string(usage)
	if command, rest, ok := strings.Cut(text, ": "); ok && !strings.Contains(command, " ") {
		text = rest
	}
	return errors.New(strings.ReplaceAll(text, "--", ""))
}

// mcpAutomatic makes each tool call create the transactions of the
// automatic schedules that are due. A read-only server leaves it off.
var mcpAutomatic bool

// tool makes a tool that needs the database.
func tool[A any](name, description string, call func(*gorm.DB, A) (any, error)) mcp.Tool {
	return mcp.New(name, description, func(arguments A) (any, error) {
		db, err := openReady()
		if err != nil {
			return nil, err
		}
		defer closeDatabase(db)
		syncStatuses(db)
		if mcpAutomatic {
			runAutomatic(db)
		}
		result, err := call(db, arguments)
		return result, agentError(err)
	})
}

// closeDatabase closes the connection of a call. The server lives as
// long as its agent, so each call must give its connection back.
func closeDatabase(db *gorm.DB) {
	if connection, err := db.DB(); err == nil {
		connection.Close()
	}
}

func reads(tool mcp.Tool) mcp.Tool {
	tool.ReadOnly = true
	return tool
}

func deletes(tool mcp.Tool) mcp.Tool {
	tool.Destructive = true
	return tool
}

// given reports which optional arguments the agent sent.
func given(arguments map[string]bool) map[string]bool {
	sent := map[string]bool{}
	for name, present := range arguments {
		if present {
			sent[name] = true
		}
	}
	return sent
}

func text(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func number(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

// mcpTools returns the tools of the server. readOnly keeps only the
// tools that change nothing.
func mcpTools(readOnly bool) []mcp.Tool {
	var tools []mcp.Tool
	for _, group := range [][]mcp.Tool{databaseTools(), accountTools(), categoryTools(), transactionTools(), creditCardTools(), goalTools(), budgetTools(), scheduleTools(), noteTools()} {
		for _, current := range group {
			if current.ReadOnly || !readOnly {
				tools = append(tools, current)
			}
		}
	}
	return tools
}

// --- Database ------------------------------------------------------------

func databaseTools() []mcp.Tool {
	return []mcp.Tool{
		mcp.New("database_init", "Create the Finux database or update it to the current version. The first setup adds the base categories. It is safe to call again: it keeps the data.",
			func(struct{}) (any, error) {
				path, err := databasePath()
				if err != nil {
					return nil, err
				}
				report, err := setupDatabase(path)
				if err != nil {
					return nil, err
				}
				return map[string]any{"path": path, "created": report.created, "base_categories_added": report.baseCategories}, nil
			}),
		reads(mcp.New("database_doctor", "Check that the Finux database exists and is up to date. It changes nothing. When healthy is false, call database_init.",
			func(struct{}) (any, error) {
				path, err := databasePath()
				if err != nil {
					return nil, err
				}
				result := map[string]any{"path": path, "found": false, "healthy": false, "pending_migrations": []string{}}
				db, err := storage.Open(path)
				if errors.Is(err, storage.ErrNotInitialized) {
					return result, nil
				} else if err != nil {
					return nil, err
				}
				defer closeDatabase(db)
				pending, err := storage.Pending(db, models...)
				if err != nil {
					return nil, err
				}
				result["found"], result["healthy"] = true, len(pending) == 0
				result["pending_migrations"] = append([]string{}, pending...)
				return result, nil
			})),
		mcp.New("database_reseed", "Add the base categories that are missing, for example after one was deleted. It finds them by code and does not change the categories that exist.",
			func(struct{}) (any, error) {
				path, err := databasePath()
				if err != nil {
					return nil, err
				}
				db, err := storage.Open(path)
				if errors.Is(err, storage.ErrNotInitialized) {
					return nil, fmt.Errorf("%w. Call the tool database_init to create it", err)
				} else if err != nil {
					return nil, err
				}
				defer closeDatabase(db)
				added, err := category.NewStore(db).AddMissingBase()
				if err != nil {
					return nil, err
				}
				return map[string]any{"added": append([]string{}, added...)}, nil
			}),
	}
}

// --- Accounts ------------------------------------------------------------

type referenceArguments struct {
	Account string `json:"account" mcp:"Code or id of the account"`
}

type listArguments struct {
	IncludeArchived bool `json:"include_archived,omitempty" mcp:"Also return the archived ones"`
}

// placeListArguments are the arguments of the lists of accounts and of
// credit cards.
type placeListArguments struct {
	IncludeArchived bool `json:"include_archived,omitempty" mcp:"Also return the archived ones"`
	IncludeOthers   bool `json:"include_others,omitempty" mcp:"Also return the ones of other persons (the ones with an owner)"`
}

func newAccountsCommand(db *gorm.DB) *accountsCommand {
	return &accountsCommand{streams: quiet, db: db, store: account.NewStore(db)}
}

// find finds an account, also when it is archived.
func (c *accountsCommand) find(ref string) (account.Account, error) {
	found, err := c.store.Find(ref, true, today())
	if errors.Is(err, account.ErrNotFound) {
		return found, fmt.Errorf("account not found: %s", ref)
	}
	return found, err
}

// reload returns the account as the database has it now.
func (c *accountsCommand) reload(id uint) (any, error) {
	found, err := c.store.Find(strconv.FormatUint(uint64(id), 10), true, today())
	if err != nil {
		return nil, err
	}
	return map[string]any{"account": accountResult(found)}, nil
}

func accountTools() []mcp.Tool {
	type addArguments struct {
		Name        string  `json:"name" mcp:"Name of the account"`
		Currency    string  `json:"currency" mcp:"Currency of the account; it cannot change later" enum:"USD,EUR,BRL,BTC"`
		Code        *string `json:"code,omitempty" mcp:"Short code, 1 to 5 letters and digits. Default: made from the name"`
		Description *string `json:"description,omitempty" mcp:"Description"`
		Balance     *string `json:"balance,omitempty" mcp:"Opening balance as a decimal string, for example \"1234.56\". It can be negative. Default: 0"`
		Color       *string `json:"color,omitempty" mcp:"Color. Default: the first color no account uses" enum:"red,orange,amber,yellow,lime,green,emerald,teal,cyan,sky,blue,indigo,violet,purple,pink,slate"`
		Owner       *string `json:"owner,omitempty" mcp:"Name of the person the account belongs to, when it is not the user. Default: the user"`
	}
	type editArguments struct {
		Account     string  `json:"account" mcp:"Code or id of the account"`
		Name        *string `json:"name,omitempty" mcp:"New name"`
		Code        *string `json:"code,omitempty" mcp:"New code, 1 to 5 letters and digits"`
		Description *string `json:"description,omitempty" mcp:"New description"`
		Color       *string `json:"color,omitempty" mcp:"New color" enum:"red,orange,amber,yellow,lime,green,emerald,teal,cyan,sky,blue,indigo,violet,purple,pink,slate"`
		Owner       *string `json:"owner,omitempty" mcp:"Name of the person the account belongs to. An empty string makes it an account of the user"`
	}
	type deleteArguments struct {
		Account string `json:"account" mcp:"Code or id of the account"`
		Confirm bool   `json:"confirm,omitempty" mcp:"Set to true to delete. Without it the tool only tells what it will delete."`
	}
	return []mcp.Tool{
		reads(tool("account_list", "List the accounts with their balances. The balance is the opening balance plus the transactions of the account that have a date up to today. An account with an owner belongs to another person: the list leaves it out unless include_others is true.",
			func(db *gorm.DB, a placeListArguments) (any, error) {
				accounts, err := account.NewStore(db).List(a.IncludeArchived, today())
				if err != nil {
					return nil, err
				}
				if !a.IncludeOthers {
					accounts = account.Mine(accounts)
				}
				results := make([]accountJSON, len(accounts))
				for i, current := range accounts {
					results[i] = accountResult(current)
				}
				return map[string]any{"accounts": results}, nil
			})),
		reads(tool("account_show", "Show one account with its balance. It also finds an archived account.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				found, err := newAccountsCommand(db).find(a.Account)
				if err != nil {
					return nil, err
				}
				notes, err := linkedNotes(db, note.LinkAccount, found.ID)
				if err != nil {
					return nil, err
				}
				return map[string]any{"account": accountResult(found), "notes": notes}, nil
			})),
		tool("account_add", "Create an account, for example a bank account or a wallet. For a credit card, use credit_card_add.",
			func(db *gorm.DB, a addArguments) (any, error) {
				c := newAccountsCommand(db)
				draft := account.Account{
					Name: strings.TrimSpace(a.Name), Code: code.Normalize(text(a.Code)), Description: text(a.Description),
					Currency: strings.ToUpper(strings.TrimSpace(a.Currency)), Color: strings.ToLower(strings.TrimSpace(text(a.Color))),
					Owner: strings.TrimSpace(text(a.Owner)),
				}
				sent := given(map[string]bool{"name": true, "currency": true, "code": a.Code != nil, "color": a.Color != nil, "owner": a.Owner != nil})
				err := checkFlags("add", draft, sent)
				if err != nil {
					return nil, err
				}
				if a.Balance != nil {
					if draft.OpeningBalance, err = money.Parse(*a.Balance, draft.Currency); err != nil {
						return nil, err
					}
				}
				if a.Color == nil {
					if draft.Color, err = c.unusedColor(); err != nil {
						return nil, err
					}
				}
				if a.Code == nil {
					if draft.Code = code.Suggest(draft.Name, c.taken(0)); draft.Code == "" {
						return nil, fmt.Errorf("cannot make a code from the name %q; give code", draft.Name)
					}
				}
				if err := c.store.Create(&draft); err != nil {
					return nil, err
				}
				return c.reload(draft.ID)
			}),
		tool("account_edit", "Change the name, the code, the description, the color or the owner of an account. Give only the arguments to change. The currency and the opening balance cannot change.",
			func(db *gorm.DB, a editArguments) (any, error) {
				c := newAccountsCommand(db)
				sent := given(map[string]bool{"name": a.Name != nil, "code": a.Code != nil, "description": a.Description != nil, "color": a.Color != nil, "owner": a.Owner != nil})
				if len(sent) == 0 {
					return nil, errors.New("give at least one of name, code, description, color, owner")
				}
				changes := account.Account{
					Name: strings.TrimSpace(text(a.Name)), Code: code.Normalize(text(a.Code)), Color: strings.ToLower(strings.TrimSpace(text(a.Color))),
					Owner: strings.TrimSpace(text(a.Owner)),
				}
				if err := checkFlags("edit", changes, sent); err != nil {
					return nil, err
				}
				current, err := c.find(a.Account)
				if err != nil {
					return nil, err
				}
				if current.Archived() {
					return nil, fmt.Errorf("account not found: %s (it is archived; restore it first)", current.Code)
				}
				if a.Name != nil {
					current.Name = changes.Name
				}
				if a.Code != nil {
					current.Code = changes.Code
				}
				if a.Description != nil {
					current.Description = *a.Description
				}
				if a.Color != nil {
					current.Color = changes.Color
				}
				if a.Owner != nil {
					current.Owner = changes.Owner
				}
				if err := c.store.Update(&current); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		tool("account_archive", "Archive an account: it leaves the lists and accepts no new transactions, but it keeps its transactions. account_restore brings it back.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := newAccountsCommand(db)
				current, err := c.find(a.Account)
				if err != nil {
					return nil, err
				}
				if current.Archived() {
					return nil, fmt.Errorf("account %s is already archived", current.Code)
				}
				if err := c.store.Archive(current.ID); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		tool("account_restore", "Restore an archived account.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := newAccountsCommand(db)
				current, err := c.find(a.Account)
				if err != nil {
					return nil, err
				}
				if !current.Archived() {
					return nil, fmt.Errorf("account %s is not archived", current.Code)
				}
				if err := c.store.Restore(current.ID); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		deletes(tool("account_delete", "Delete an account permanently, with its transactions. This cannot be undone. The other side of each transfer stays in its account as an expense or an income of the category OTHER. To keep the data, use account_archive.",
			func(db *gorm.DB, a deleteArguments) (any, error) {
				c := newAccountsCommand(db)
				current, err := c.find(a.Account)
				if err != nil {
					return nil, err
				}
				plain, transfers, err := transaction.NewStore(db).AccountUsage(current.ID)
				if err != nil {
					return nil, err
				}
				if !a.Confirm {
					return confirmation(fmt.Sprintf("Delete account %s (%s) permanently, with %s and %s?", current.Code, current.Name, plural(plain, "transaction"), plural(transfers, "transfer"))), nil
				}
				if err := c.purge(current, transfers > 0); err != nil {
					return nil, err
				}
				return map[string]any{"deleted": true, "account": current.Code, "transactions_deleted": plain, "transfers_converted": transfers}, nil
			})),
	}
}

// --- Categories ----------------------------------------------------------

func newCategoriesCommand(db *gorm.DB) *categoriesCommand {
	return &categoriesCommand{streams: quiet, db: db, store: category.NewStore(db)}
}

func categoryTools() []mcp.Tool {
	type referenceArguments struct {
		Category string `json:"category" mcp:"Code or id of the category"`
	}
	type addArguments struct {
		Name        string  `json:"name" mcp:"Name of the category"`
		Color       string  `json:"color" mcp:"The color gives the transactions the category can have: red = expenses, green = incomes, grey = all types" enum:"red,green,grey"`
		Code        *string `json:"code,omitempty" mcp:"Short code, 1 to 5 letters and digits. Default: made from the name"`
		Description *string `json:"description,omitempty" mcp:"Description"`
	}
	type editArguments struct {
		Category    string  `json:"category" mcp:"Code or id of the category"`
		Name        *string `json:"name,omitempty" mcp:"New name"`
		Code        *string `json:"code,omitempty" mcp:"New code, 1 to 5 letters and digits"`
		Description *string `json:"description,omitempty" mcp:"New description"`
		Color       *string `json:"color,omitempty" mcp:"New color: red = expenses, green = incomes, grey = all types. Refused when the transactions of the category do not accept it" enum:"red,green,grey"`
	}
	type deleteArguments struct {
		Category string `json:"category" mcp:"Code or id of the category"`
		Confirm  bool   `json:"confirm,omitempty" mcp:"Set to true to delete. Without it the tool only tells what it will delete."`
	}
	one := func(found category.Category) map[string]any {
		return map[string]any{"category": categoryResult(found)}
	}
	return []mcp.Tool{
		reads(tool("category_list", "List the categories. The field accepts tells the type of transaction a category can have: expense, income or all.",
			func(db *gorm.DB, _ struct{}) (any, error) {
				categories, err := category.NewStore(db).List()
				if err != nil {
					return nil, err
				}
				results := make([]categoryJSON, len(categories))
				for i, current := range categories {
					results[i] = categoryResult(current)
				}
				return map[string]any{"categories": results}, nil
			})),
		reads(tool("category_show", "Show one category by its code or id.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				found, err := newCategoriesCommand(db).resolve("show", a.Category, "")
				if err != nil {
					return nil, err
				}
				return one(found), nil
			})),
		tool("category_add", "Create a category for transactions.",
			func(db *gorm.DB, a addArguments) (any, error) {
				c := newCategoriesCommand(db)
				draft := category.Category{Name: a.Name, Code: text(a.Code), Description: text(a.Description), Color: a.Color}
				sent := given(map[string]bool{"name": true, "color": true, "code": a.Code != nil})
				if err := checkCategoryFlags("add", &draft, sent); err != nil {
					return nil, err
				}
				if a.Code == nil {
					if draft.Code = code.Suggest(draft.Name, c.taken(0)); draft.Code == "" {
						return nil, fmt.Errorf("cannot make a code from the name %q; give code", draft.Name)
					}
				}
				if err := c.store.Create(&draft); err != nil {
					return nil, err
				}
				return one(draft), nil
			}),
		tool("category_edit", "Change the name, the code, the description or the color of a category. Give only the arguments to change.",
			func(db *gorm.DB, a editArguments) (any, error) {
				c := newCategoriesCommand(db)
				sent := given(map[string]bool{"name": a.Name != nil, "code": a.Code != nil, "description": a.Description != nil, "color": a.Color != nil})
				if len(sent) == 0 {
					return nil, errors.New("give at least one of name, code, description, color")
				}
				changes := category.Category{Name: text(a.Name), Code: text(a.Code), Color: text(a.Color)}
				if err := checkCategoryFlags("edit", &changes, sent); err != nil {
					return nil, err
				}
				current, err := c.resolve("edit", a.Category, "")
				if err != nil {
					return nil, err
				}
				if a.Name != nil {
					current.Name = changes.Name
				}
				if a.Code != nil {
					current.Code = changes.Code
				}
				if a.Description != nil {
					current.Description = *a.Description
				}
				if a.Color != nil {
					current.Color = changes.Color
				}
				if err := c.accepts(current); err != nil {
					return nil, err
				}
				if err := c.store.Update(&current); err != nil {
					return nil, err
				}
				return one(current), nil
			}),
		deletes(tool("category_delete", "Delete a category permanently. Its transactions go to the category OTHER.",
			func(db *gorm.DB, a deleteArguments) (any, error) {
				c := newCategoriesCommand(db)
				current, err := c.resolve("delete", a.Category, "")
				if err != nil {
					return nil, err
				}
				used, other, err := c.usage(current)
				if err != nil {
					return nil, err
				}
				if !a.Confirm {
					question := fmt.Sprintf("Delete category %s (%s) permanently?", current.Code, current.Name)
					if used > 0 {
						question = fmt.Sprintf("Delete category %s (%s) permanently and move %s to %s?", current.Code, current.Name, plural(used, "transaction"), other.Code)
					}
					return confirmation(question), nil
				}
				if err := c.remove(current, used, other); err != nil {
					return nil, err
				}
				result := map[string]any{"deleted": true, "category": current.Code, "transactions_moved": used}
				if used > 0 {
					result["moved_to"] = other.Code
				}
				return result, nil
			})),
	}
}

// --- Transactions --------------------------------------------------------

// defaultCorrections is the number of corrections of one page.
const defaultCorrections = 50

type correctionsArguments struct {
	Titles []string `json:"titles,omitempty" mcp:"Titles or raw lines of a statement, for example [\"PAY LAMEI 21/11\"]. The result then has only the corrections of these titles; dates and codes in a title do not count. Without it, the result has all the corrections, the last one changed first"`
	Limit  *int     `json:"limit,omitempty" mcp:"Most corrections to return. Default: 50"`
	Offset *int     `json:"offset,omitempty" mcp:"Number of corrections to skip, to read the next page. Default: 0"`
}

// correctionJSON is a correction in a result. A field that no change
// taught is absent. Empty tags or an empty description mean that the
// user wants none.
type correctionJSON struct {
	Pattern     string    `json:"pattern"`
	Title       string    `json:"title,omitempty"`
	Category    string    `json:"category,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
	Description *string   `json:"description,omitempty"`
	Source      string    `json:"source"`
	Count       int       `json:"count"`
	Updated     string    `json:"updated"`
}

// agentTransactionsCommand makes the command that the transaction
// tools use. Its changes have the agent as their source in the
// corrections.
func agentTransactionsCommand(db *gorm.DB) *transactionsCommand {
	command := newTransactionsCommand(db)
	command.store.Actor = transaction.SourceAgent
	return command
}

func newTransactionsCommand(db *gorm.DB) *transactionsCommand {
	return &transactionsCommand{
		streams: quiet, store: transaction.NewStore(db), accounts: account.NewStore(db),
		categories: category.NewStore(db), cards: card.NewStore(db), goals: goal.NewStore(db), budgets: budget.NewStore(db),
		schedules: schedule.NewStore(db), notes: note.NewStore(db),
	}
}

// savedResult returns a transaction that was created or changed as a result,
// with the budgets that it leaves near or over their limit.
func (c *transactionsCommand) savedResult(id uint) (any, error) {
	found, err := c.resolve("show", strconv.FormatUint(uint64(id), 10), "")
	if err != nil {
		return nil, err
	}
	result := map[string]any{"transaction": transactionResult(found)}
	warnings, err := budgetWarnings(c.budgets, found)
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		result["budget_warnings"] = warnings
	}
	return result, nil
}

// savedParts is savedResult for the parts of a transaction in
// installments. "transaction" is the part with the id and
// "transactions" holds all the parts given.
func (c *transactionsCommand) savedParts(id uint, parts []transaction.Transaction) (any, error) {
	result, err := c.savedResult(id)
	if err != nil {
		return nil, err
	}
	views := make([]transaction.View, len(parts))
	for i, part := range parts {
		if views[i], err = c.store.Find(part.ID); err != nil {
			return nil, err
		}
	}
	result.(map[string]any)["transactions"] = transactionResults(views)
	return result, nil
}

// scopeFlags reads the scope argument of a tool as the options --all
// and --next.
func scopeFlags(scope *string) (all, next bool) {
	return text(scope) == "all", text(scope) == "next"
}

// one returns the transaction with the id as a result.
func (c *transactionsCommand) one(id uint) (any, error) {
	found, err := c.resolve("show", strconv.FormatUint(uint64(id), 10), "")
	if err != nil {
		return nil, err
	}
	return map[string]any{"transaction": transactionResult(found)}, nil
}

// totalJSON is what came in and went out in one currency.
type totalJSON struct {
	Currency string   `json:"currency"`
	Income   cashJSON `json:"income"`
	Expense  cashJSON `json:"expense"`
	Net      cashJSON `json:"net"`
}

// totals adds the incomes and the expenses of each currency. Transfers
// and payments are not in the totals: the money stays with the user.
func totals(views []transaction.View) []totalJSON {
	in, out := map[string]int64{}, map[string]int64{}
	for _, view := range views {
		switch view.Type {
		case transaction.Income:
			in[view.Account.Currency] += view.Amount
		case transaction.Expense:
			out[view.Account.Currency] -= view.Amount
		}
	}
	results := []totalJSON{}
	for _, currency := range money.Codes() {
		if in[currency] != 0 || out[currency] != 0 {
			results = append(results, totalJSON{
				Currency: currency, Income: cash(in[currency], currency), Expense: cash(out[currency], currency),
				Net: cash(in[currency]-out[currency], currency),
			})
		}
	}
	return results
}

func transactionTools() []mcp.Tool {
	type listArguments struct {
		Category      *string `json:"category,omitempty" mcp:"Only the transactions of this category (code or id)"`
		Tags          *string `json:"tags,omitempty" mcp:"Only the transactions that have all these tags, for example \"market weekly\""`
		Search        *string `json:"search,omitempty" mcp:"Only the transactions whose title contains this text"`
		Date          *string `json:"date,omitempty" mcp:"A year (2026), a month (2026-11 or 11/2026) or a date (2026-11-17)"`
		Account       *string `json:"account,omitempty" mcp:"Only the transactions of this account or credit card (code or id)"`
		Type          *string `json:"type,omitempty" mcp:"Only the transactions of this type" enum:"expense,income,transfer,payment"`
		Goal          *string `json:"goal,omitempty" mcp:"Only the transactions linked to this goal (code or id)"`
		Virtual       *bool   `json:"virtual,omitempty" mcp:"true: only the transactions made with a virtual card of a credit card. false: only the other ones"`
		IncludeOthers bool    `json:"include_others,omitempty" mcp:"Also return and count the transactions in the accounts and the credit cards of other persons. With account, the transactions of that account come in any case"`
		Limit         *int    `json:"limit,omitempty" mcp:"Most transactions to return. Default: 100"`
		Offset        *int    `json:"offset,omitempty" mcp:"Number of transactions to skip, to read the next page. Default: 0"`
	}
	type showArguments struct {
		ID uint `json:"id" mcp:"Id of the transaction"`
	}
	type addArguments struct {
		Title            string  `json:"title" mcp:"Title, at most 60 characters"`
		Value            *string `json:"value,omitempty" mcp:"Positive amount in the currency of the account, as a decimal string, for example \"1234.56\". The type gives the sign. With installments it is the total of all the parts. Necessary, but when installment_value is given"`
		Account          string  `json:"account" mcp:"Code or id of the account or the credit card of the transaction"`
		Category         *string `json:"category,omitempty" mcp:"Code or id of the category. Necessary, but for a transfer, which has the default XFER"`
		Type             *string `json:"type,omitempty" mcp:"Default: expense for a red category, income for a green one, transfer when 'to' is given. Necessary for a grey category" enum:"expense,income,transfer"`
		Description      *string `json:"description,omitempty" mcp:"Description, at most 200 characters"`
		Tags             *string `json:"tags,omitempty" mcp:"Tags divided by spaces, for example \"market weekly\". Use transaction_suggestions to find the tags in use"`
		Date             *string `json:"date,omitempty" mcp:"YYYY-MM-DD or DD/MM/YYYY. Default: today"`
		To               *string `json:"to,omitempty" mcp:"For a transfer: code or id of the account that receives the money. A credit card cannot have a transfer"`
		Received         *string `json:"received,omitempty" mcp:"For a transfer between accounts of different currencies: the amount the other account receives"`
		Goal             *string `json:"goal,omitempty" mcp:"Code or id of the goal the transaction counts toward. The goal must have the currency of the account"`
		Installments     *int    `json:"installments,omitempty" mcp:"For an expense or an income in installments: the number of monthly parts, from 2 to 120. The tool creates one transaction for each part"`
		InstallmentValue *string `json:"installment_value,omitempty" mcp:"With installments: the amount of one part, in place of value"`
		FirstInstallment *int    `json:"first_installment,omitempty" mcp:"With installments, for a purchase that is already in progress: the first part to create. date is then the date of that part. Default: 1"`
		Virtual          *bool   `json:"virtual,omitempty" mcp:"true when the person paid with a virtual card of the credit card. Only for a transaction of a credit card. Default: false"`
	}
	type editArguments struct {
		ID               uint    `json:"id" mcp:"Id of the transaction"`
		Title            *string `json:"title,omitempty" mcp:"New title, at most 60 characters"`
		Description      *string `json:"description,omitempty" mcp:"New description, at most 200 characters"`
		Value            *string `json:"value,omitempty" mcp:"New positive amount as a decimal string"`
		Type             *string `json:"type,omitempty" mcp:"New type. A transfer keeps its type" enum:"expense,income,transfer"`
		Category         *string `json:"category,omitempty" mcp:"Code or id of the new category"`
		Tags             *string `json:"tags,omitempty" mcp:"New tags; they replace the tags of the transaction. An empty string removes all tags"`
		Account          *string `json:"account,omitempty" mcp:"Code or id of the new account or credit card. Give value also when the currency is different"`
		Date             *string `json:"date,omitempty" mcp:"New date, YYYY-MM-DD or DD/MM/YYYY"`
		Goal             *string `json:"goal,omitempty" mcp:"Code or id of the new goal. An empty string takes the goal away"`
		Scope            *string `json:"scope,omitempty" mcp:"For a transaction in installments: one changes this part only, next changes this part and the parts after it, all changes all the parts. With next or all, value is the new total of these parts and date is the new date of the first of them. Default: one" enum:"one,next,all"`
		InstallmentValue *string `json:"installment_value,omitempty" mcp:"With the scope next or all: the new amount of each part, in place of value"`
		Virtual          *bool   `json:"virtual,omitempty" mcp:"true when the person paid with a virtual card of the credit card; false takes the mark away. Only for a transaction of a credit card"`
	}
	type deleteArguments struct {
		ID      uint    `json:"id" mcp:"Id of the transaction"`
		Confirm bool    `json:"confirm,omitempty" mcp:"Set to true to delete. Without it the tool only tells what it will delete."`
		Scope   *string `json:"scope,omitempty" mcp:"For a transaction in installments: one deletes this part only, next deletes this part and the parts after it, all deletes all the parts. Default: one" enum:"one,next,all"`
	}
	return []mcp.Tool{
		reads(tool("transaction_list", "List transactions, the newest first, with the totals of all transactions that match the filters. All filters are optional and combine. Read more pages with limit and offset. The list and the totals leave out the transactions in the accounts and the credit cards of other persons unless include_others is true.",
			func(db *gorm.DB, a listArguments) (any, error) {
				c := agentTransactionsCommand(db)
				sent := given(map[string]bool{"category": a.Category != nil, "tag": a.Tags != nil, "date": a.Date != nil, "account": a.Account != nil, "type": a.Type != nil, "goal": a.Goal != nil, "virtual": a.Virtual != nil})
				filter, err := c.filter(listFlags{category: text(a.Category), tags: text(a.Tags), search: text(a.Search), date: text(a.Date), account: text(a.Account), kind: text(a.Type), goal: text(a.Goal), virtual: a.Virtual != nil && *a.Virtual}, sent)
				if err != nil {
					return nil, err
				}
				filter.Mine = !a.IncludeOthers
				views, err := c.store.List(filter)
				if err != nil {
					return nil, err
				}
				limit, offset := defaultPage, min(max(number(a.Offset), 0), len(views))
				if a.Limit != nil && *a.Limit > 0 {
					limit = *a.Limit
				}
				page := views[offset:min(offset+limit, len(views))]
				return map[string]any{
					"total": len(views), "offset": offset, "count": len(page),
					"totals": totals(views), "transactions": transactionResults(page),
				}, nil
			})),
		reads(tool("transaction_show", "Show one transaction by its id.",
			func(db *gorm.DB, a showArguments) (any, error) {
				return agentTransactionsCommand(db).one(a.ID)
			})),
		tool("transaction_add", "Create a transaction: an expense, an income or a transfer between two accounts. A transaction of a credit card goes to the bill of its date. To pay a bill of a credit card, use credit_card_pay. Give goal to count the transaction toward a goal. Give virtual for a purchase made with a virtual card of a credit card. Give installments for an expense or an income in monthly parts: the result then has all the parts in transactions.",
			func(db *gorm.DB, a addArguments) (any, error) {
				c := agentTransactionsCommand(db)
				values := transactionFlags{
					title: a.Title, value: text(a.Value), account: a.Account, category: text(a.Category), kind: text(a.Type),
					description: text(a.Description), tags: text(a.Tags), date: text(a.Date), to: text(a.To), received: text(a.Received),
					goal: text(a.Goal), installmentValue: text(a.InstallmentValue), installments: number(a.Installments), from: number(a.FirstInstallment),
					virtual: a.Virtual != nil && *a.Virtual,
				}
				sent := given(map[string]bool{
					"title": true, "value": a.Value != nil, "account": true, "category": a.Category != nil, "type": a.Type != nil,
					"description": a.Description != nil, "tags": a.Tags != nil, "date": a.Date != nil, "to": a.To != nil, "received": a.Received != nil,
					"goal": a.Goal != nil, "installments": a.Installments != nil, "installment-value": a.InstallmentValue != nil,
					"from": a.FirstInstallment != nil, "virtual": a.Virtual != nil,
				})
				if a.Value == nil && a.InstallmentValue == nil {
					return nil, errors.New("give value or installment_value")
				}
				draft, complete, err := c.newDraft(values, sent)
				if err != nil {
					return nil, err
				}
				if !complete {
					return nil, errors.New("category is necessary: only a transfer has a default category")
				}
				created, err := c.store.Create(draft)
				if err != nil {
					return nil, err
				}
				if created.InstallmentID == 0 {
					return c.savedResult(created.ID)
				}
				parts, err := c.store.Parts(created.ID, transaction.All)
				if err != nil {
					return nil, err
				}
				return c.savedParts(created.ID, parts)
			}),
		tool("transaction_edit", "Change a transaction. Give only the arguments to change. A change of one side of a transfer also changes the other side. A payment of a bill cannot change: delete it. For a transaction in installments, scope tells which parts change: the result then has them in transactions.",
			func(db *gorm.DB, a editArguments) (any, error) {
				c := agentTransactionsCommand(db)
				values := transactionFlags{
					title: text(a.Title), description: text(a.Description), value: text(a.Value), kind: text(a.Type),
					category: text(a.Category), tags: text(a.Tags), account: text(a.Account), date: text(a.Date), goal: text(a.Goal),
					installmentValue: text(a.InstallmentValue), virtual: a.Virtual != nil && *a.Virtual,
				}
				values.all, values.next = scopeFlags(a.Scope)
				sent := given(map[string]bool{
					"title": a.Title != nil, "description": a.Description != nil, "value": a.Value != nil, "type": a.Type != nil,
					"category": a.Category != nil, "tags": a.Tags != nil, "account": a.Account != nil, "date": a.Date != nil,
					"goal": a.Goal != nil, "installment-value": a.InstallmentValue != nil, "virtual": a.Virtual != nil,
				})
				switch {
				case len(sent) == 0:
					return nil, errors.New("give at least one of title, description, value, installment_value, type, category, tags, account, date, goal, virtual")
				case a.Value != nil && a.InstallmentValue != nil:
					return nil, errors.New("give value or installment_value, not the two")
				case a.InstallmentValue != nil && !values.all && !values.next:
					return nil, errors.New("installment_value needs the scope next or all")
				}
				var changes transaction.Draft
				if err := values.check("edit", sent, &changes); err != nil {
					return nil, err
				}
				current, err := c.resolve("edit", strconv.FormatUint(uint64(a.ID), 10), "")
				if err != nil {
					return nil, err
				}
				if current.Type == transaction.Payment {
					return nil, errors.New("a payment cannot change: delete it to open the bill again")
				}
				scope, err := scopeOf("edit", values.all, values.next, current.Transaction)
				if err != nil {
					return nil, err
				}
				if scope != transaction.One {
					parts, _, err := c.changeParts(current, scope, changes, values, sent)
					if err != nil {
						return nil, err
					}
					return c.savedParts(current.ID, parts)
				}
				draft, err := c.change(current, transaction.DraftOf(current.Transaction), changes, values, sent)
				if err != nil {
					return nil, err
				}
				updated, err := c.store.Update(current.ID, draft)
				if err != nil {
					return nil, err
				}
				return c.savedResult(updated.ID)
			}),
		deletes(tool("transaction_delete", "Delete a transaction permanently. The delete of one side of a transfer also deletes the other side. The delete of a payment opens its bill again. For a transaction in installments, scope tells which parts to delete.",
			func(db *gorm.DB, a deleteArguments) (any, error) {
				c := agentTransactionsCommand(db)
				current, err := c.resolve("delete", strconv.FormatUint(uint64(a.ID), 10), "")
				if err != nil {
					return nil, err
				}
				all, next := scopeFlags(a.Scope)
				scope, err := scopeOf("delete", all, next, current.Transaction)
				if err != nil {
					return nil, err
				}
				parts, err := c.store.Parts(current.ID, scope)
				if err != nil {
					return nil, err
				}
				if !a.Confirm {
					return confirmation(deleteQuestion(current.Transaction, scope, parts)), nil
				}
				if scope != transaction.One {
					deleted, err := c.store.DeleteParts(current.ID, scope)
					if err != nil {
						return nil, err
					}
					return map[string]any{"deleted": deleted}, nil
				}
				if err := c.store.Delete(current.ID); err != nil {
					return nil, err
				}
				deleted := []uint{current.ID}
				if current.PairID != 0 {
					deleted = append(deleted, current.PairID)
				}
				return map[string]any{"deleted": deleted}, nil
			})),
		reads(tool("transaction_suggestions", "List the titles and the tags that the transactions use, the last one used first. Use them to keep the titles and the tags of new transactions consistent.",
			func(db *gorm.DB, _ struct{}) (any, error) {
				titles, tags, err := agentTransactionsCommand(db).suggestions()
				if err != nil {
					return nil, err
				}
				return map[string]any{"titles": append([]string{}, titles...), "tags": append([]string{}, tags...)}, nil
			})),
		reads(tool("transaction_corrections", "List what the changes to transactions taught: for the title a transaction had (pattern), the title, the category, the tags and the description that the user wants. Call it with the titles of the transactions you create or tidy, and use the corrections in place of your own names. A correction with the source user is a change the user made by hand: it wins over what you know. A field that is absent was not taught.",
			func(db *gorm.DB, a correctionsArguments) (any, error) {
				limit := defaultCorrections
				if a.Limit != nil && *a.Limit > 0 {
					limit = *a.Limit
				}
				found, total, err := transaction.NewStore(db).Corrections(a.Titles, limit, max(number(a.Offset), 0))
				if err != nil {
					return nil, err
				}
				corrections := make([]correctionJSON, len(found))
				for i, current := range found {
					corrections[i] = correctionJSON{
						Pattern: current.Pattern, Title: current.Title, Category: current.Category, Description: current.Description,
						Source: current.Source, Count: current.Count, Updated: current.UpdatedAt.Local().Format("2006-01-02"),
					}
					if current.Tags != nil {
						tags := strings.Fields(*current.Tags)
						corrections[i].Tags = &tags
					}
				}
				return map[string]any{"total": total, "offset": max(number(a.Offset), 0), "count": len(corrections), "corrections": corrections}, nil
			})),
	}
}

// --- Credit cards --------------------------------------------------------

func newCreditCardsCommand(db *gorm.DB) *creditCardsCommand {
	return &creditCardsCommand{streams: quiet, db: db, store: card.NewStore(db), transactions: transaction.NewStore(db)}
}

// reload returns the credit card as the database has it now.
func (c *creditCardsCommand) reload(id uint) (any, error) {
	found, err := c.store.Find(strconv.FormatUint(uint64(id), 10), true)
	if err != nil {
		return nil, err
	}
	return map[string]any{"credit_card": cardResult(found)}, nil
}

func creditCardTools() []mcp.Tool {
	type referenceArguments struct {
		CreditCard string `json:"credit_card" mcp:"Code or id of the credit card"`
	}
	type addArguments struct {
		Name        string  `json:"name" mcp:"Name of the credit card"`
		Currency    string  `json:"currency" mcp:"Currency of the credit card; it cannot change later" enum:"USD,EUR,BRL,BTC"`
		Limit       string  `json:"limit" mcp:"Credit limit as a decimal string, for example \"5000\""`
		ClosingDay  int     `json:"closing_day" mcp:"Day of the month the bill closes, 1 to 31. A purchase after this day goes to the next bill"`
		DueDay      int     `json:"due_day" mcp:"Day of the month the bill is due, 1 to 31"`
		Code        *string `json:"code,omitempty" mcp:"Short code, 1 to 5 letters and digits, different from the codes of the accounts. Default: made from the name"`
		Description *string `json:"description,omitempty" mcp:"Description"`
		Color       *string `json:"color,omitempty" mcp:"Color. Default: the first color no credit card uses" enum:"red,orange,amber,yellow,lime,green,emerald,teal,cyan,sky,blue,indigo,violet,purple,pink,slate"`
		Owner       *string `json:"owner,omitempty" mcp:"Name of the person the credit card belongs to, when it is not the user. Default: the user"`
	}
	type editArguments struct {
		CreditCard  string  `json:"credit_card" mcp:"Code or id of the credit card"`
		Name        *string `json:"name,omitempty" mcp:"New name"`
		Code        *string `json:"code,omitempty" mcp:"New code, 1 to 5 letters and digits"`
		Description *string `json:"description,omitempty" mcp:"New description"`
		Color       *string `json:"color,omitempty" mcp:"New color" enum:"red,orange,amber,yellow,lime,green,emerald,teal,cyan,sky,blue,indigo,violet,purple,pink,slate"`
		Limit       *string `json:"limit,omitempty" mcp:"New credit limit as a decimal string"`
		ClosingDay  *int    `json:"closing_day,omitempty" mcp:"New closing day, 1 to 31. It applies to the bills that do not exist yet"`
		DueDay      *int    `json:"due_day,omitempty" mcp:"New due day, 1 to 31. It applies to the bills that do not exist yet"`
		Owner       *string `json:"owner,omitempty" mcp:"Name of the person the credit card belongs to. An empty string makes it a credit card of the user"`
	}
	type payArguments struct {
		CreditCard string  `json:"credit_card" mcp:"Code or id of the credit card"`
		From       string  `json:"from" mcp:"Code or id of the account that pays. It must have the currency of the credit card"`
		Value      *string `json:"value,omitempty" mcp:"Amount to pay as a decimal string. Default: the total of the bill. With a smaller value, the bill is paid and the rest goes to the next bill"`
		Date       *string `json:"date,omitempty" mcp:"Date of the payment, YYYY-MM-DD or DD/MM/YYYY. Default: today"`
		Bill       *string `json:"bill,omitempty" mcp:"Bill to pay, as YYYY-MM or MM/YYYY (the month of its due date). Default: the oldest bill that is closed or late"`
	}
	type unpayArguments struct {
		CreditCard string  `json:"credit_card" mcp:"Code or id of the credit card"`
		Bill       *string `json:"bill,omitempty" mcp:"Bill, as YYYY-MM or MM/YYYY. Default: the last paid bill"`
		Confirm    bool    `json:"confirm,omitempty" mcp:"Set to true to delete the payment. Without it the tool only tells what it will delete."`
	}
	type deleteArguments struct {
		CreditCard string `json:"credit_card" mcp:"Code or id of the credit card"`
		Confirm    bool   `json:"confirm,omitempty" mcp:"Set to true to delete. Without it the tool only tells what it will delete."`
	}
	// find finds a credit card, also when it is archived.
	find := func(c *creditCardsCommand, ref string) (card.Card, error) {
		return c.resolve("show", ref, "", scopeAll)
	}
	return []mcp.Tool{
		reads(tool("credit_card_list", "List the credit cards with the limit, the used value (the total of the bills that are not paid) and the available value. A credit card with an owner belongs to another person: the list leaves it out unless include_others is true.",
			func(db *gorm.DB, a placeListArguments) (any, error) {
				cards, err := card.NewStore(db).List(a.IncludeArchived)
				if err != nil {
					return nil, err
				}
				if !a.IncludeOthers {
					cards = card.Mine(cards)
				}
				results := make([]cardJSON, len(cards))
				for i, current := range cards {
					results[i] = cardResult(current)
				}
				return map[string]any{"credit_cards": results}, nil
			})),
		reads(tool("credit_card_show", "Show one credit card, its open bill with the transactions of that bill, and the bills that wait for a payment (closed or late).",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := newCreditCardsCommand(db)
				found, err := find(c, a.CreditCard)
				if err != nil {
					return nil, err
				}
				details, err := c.gather(found)
				if err != nil {
					return nil, err
				}
				open := billResult(found, details.Bill)
				purchases := transactionResults(details.Transactions)
				open.Transactions = &purchases
				if total, count := transaction.VirtualTotal(details.Transactions); count > 0 {
					sum := cash(total, found.Currency)
					open.VirtualTotal = &sum
				}
				pending := make([]billJSON, len(details.Pending))
				for i, bill := range details.Pending {
					pending[i] = billResult(found, bill)
				}
				notes, err := linkedNotes(db, note.LinkCard, found.ID)
				if err != nil {
					return nil, err
				}
				return map[string]any{"credit_card": cardResult(found), "open_bill": open, "pending_bills": pending, "notes": notes}, nil
			})),
		reads(tool("credit_card_bills", "List the bills of a credit card, the newest first. A bill is open until its closing date, then closed until its due date, then late, until it is paid.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := newCreditCardsCommand(db)
				found, err := find(c, a.CreditCard)
				if err != nil {
					return nil, err
				}
				bills, err := c.store.Bills(found.ID)
				if err != nil {
					return nil, err
				}
				results := make([]billJSON, len(bills))
				for i, bill := range bills {
					results[i] = billResult(found, bill)
				}
				return map[string]any{"credit_card": found.Code, "bills": results}, nil
			})),
		tool("credit_card_add", "Create a credit card. Its purchases are transactions with the code of the card as the account; they go to a monthly bill and do not change the balance of an account.",
			func(db *gorm.DB, a addArguments) (any, error) {
				c := newCreditCardsCommand(db)
				values := cardFlags{
					name: a.Name, currency: a.Currency, limit: a.Limit, closing: a.ClosingDay, due: a.DueDay,
					code: text(a.Code), description: text(a.Description), color: text(a.Color), owner: text(a.Owner),
				}
				sent := given(map[string]bool{
					"name": true, "currency": true, "limit": true, "closing": true, "due": true,
					"code": a.Code != nil, "description": a.Description != nil, "color": a.Color != nil, "owner": a.Owner != nil,
				})
				var draft card.Card
				err := values.apply("add", sent, &draft)
				if err != nil {
					return nil, err
				}
				if a.Color == nil {
					if draft.Color, err = c.unusedColor(); err != nil {
						return nil, err
					}
				}
				if a.Code == nil {
					if draft.Code = code.Suggest(draft.Name, c.taken(0)); draft.Code == "" {
						return nil, fmt.Errorf("cannot make a code from the name %q; give code", draft.Name)
					}
				}
				if err := c.store.Create(&draft); err != nil {
					return nil, err
				}
				return c.reload(draft.ID)
			}),
		tool("credit_card_edit", "Change a credit card. Give only the arguments to change. The currency cannot change.",
			func(db *gorm.DB, a editArguments) (any, error) {
				c := newCreditCardsCommand(db)
				values := cardFlags{
					name: text(a.Name), code: text(a.Code), description: text(a.Description), color: text(a.Color),
					limit: text(a.Limit), closing: number(a.ClosingDay), due: number(a.DueDay), owner: text(a.Owner),
				}
				sent := given(map[string]bool{
					"name": a.Name != nil, "code": a.Code != nil, "description": a.Description != nil, "color": a.Color != nil,
					"limit": a.Limit != nil, "closing": a.ClosingDay != nil, "due": a.DueDay != nil, "owner": a.Owner != nil,
				})
				if len(sent) == 0 {
					return nil, errors.New("give at least one of name, code, description, color, limit, closing_day, due_day, owner")
				}
				current, err := find(c, a.CreditCard)
				if err != nil {
					return nil, err
				}
				if current.Archived() {
					return nil, fmt.Errorf("credit card not found: %s (it is archived; restore it first)", current.Code)
				}
				if err := values.apply("edit", sent, &current); err != nil {
					return nil, err
				}
				if err := c.store.Update(&current); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		tool("credit_card_pay", "Pay a bill of a credit card from an account. It creates a transaction of the type payment in the account and marks the bill as paid. A bill that is still open cannot be paid. A paid bill is locked: its transactions cannot change until credit_card_unpay.",
			func(db *gorm.DB, a payArguments) (any, error) {
				c := newCreditCardsCommand(db)
				date := today()
				var err error
				if a.Date != nil {
					if date, err = transaction.ParseDate(*a.Date); err != nil {
						return nil, err
					}
				}
				owner, err := find(c, a.CreditCard)
				if err != nil {
					return nil, err
				}
				var bill card.Bill
				if a.Bill != nil {
					bill, err = c.bill("pay", owner, *a.Bill)
				} else if bill, err = c.waiting(owner, date); err != nil {
					err = fmt.Errorf("no bill of %s waits for a payment. To pay a different bill, give bill", owner.Code)
				}
				if err != nil {
					return nil, err
				}
				payer, err := newAccountsCommand(db).find(a.From)
				if err != nil {
					return nil, err
				}
				if payer.Archived() {
					return nil, fmt.Errorf("account not found: %s (it is archived; restore it first)", payer.Code)
				}
				var value int64
				if a.Value != nil {
					if value, err = amount("pay", *a.Value, owner.Currency); err != nil {
						return nil, err
					}
				}
				payment, err := c.transactions.PayBill(bill.ID, payer.ID, value, date)
				if err != nil {
					return nil, err
				}
				if bill, err = c.store.Bill(bill.ID); err != nil {
					return nil, err
				}
				result := map[string]any{"credit_card": owner.Code, "bill": billResult(owner, bill)}
				if payment.ID != 0 {
					view, err := c.transactions.Find(payment.ID)
					if err != nil {
						return nil, err
					}
					result["payment"] = transactionResult(view)
				}
				if bill.CarriedTo != 0 {
					next, err := c.store.Bill(bill.CarriedTo)
					if err != nil {
						return nil, err
					}
					result["carried_to"] = next.Period
				}
				return result, nil
			}),
		deletes(tool("credit_card_unpay", "Delete the payment of a bill of a credit card. The payment transaction is deleted, the money goes back to the account and the bill opens again.",
			func(db *gorm.DB, a unpayArguments) (any, error) {
				c := newCreditCardsCommand(db)
				owner, err := find(c, a.CreditCard)
				if err != nil {
					return nil, err
				}
				var bill card.Bill
				if a.Bill != nil {
					bill, err = c.bill("unpay", owner, *a.Bill)
				} else {
					bill, err = c.lastPaid(owner)
				}
				if err != nil {
					return nil, err
				}
				if !bill.IsPaid() {
					return nil, fmt.Errorf("%s is not paid", billName(owner, bill))
				}
				if !a.Confirm {
					return confirmation(fmt.Sprintf("Delete the payment of %s (%s)? The bill opens again.", billName(owner, bill), money.Format(bill.Paid, owner.Currency))), nil
				}
				if err := c.transactions.Unpay(bill.ID); err != nil {
					return nil, err
				}
				if bill, err = c.store.Bill(bill.ID); err != nil {
					return nil, err
				}
				return map[string]any{"credit_card": owner.Code, "bill": billResult(owner, bill)}, nil
			})),
		tool("credit_card_archive", "Archive a credit card: it leaves the lists and accepts no new transactions, but it keeps its bills and transactions. credit_card_restore brings it back.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := newCreditCardsCommand(db)
				current, err := find(c, a.CreditCard)
				if err != nil {
					return nil, err
				}
				if current.Archived() {
					return nil, fmt.Errorf("credit card %s is already archived", current.Code)
				}
				if err := c.store.Archive(current.ID); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		tool("credit_card_restore", "Restore an archived credit card.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := newCreditCardsCommand(db)
				current, err := find(c, a.CreditCard)
				if err != nil {
					return nil, err
				}
				if !current.Archived() {
					return nil, fmt.Errorf("credit card %s is not archived", current.Code)
				}
				if err := c.store.Restore(current.ID); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		deletes(tool("credit_card_delete", "Delete a credit card permanently, with its bills and its transactions. This cannot be undone. Each payment of its bills stays in its account as an expense of the category OTHER. To keep the data, use credit_card_archive.",
			func(db *gorm.DB, a deleteArguments) (any, error) {
				c := newCreditCardsCommand(db)
				current, err := find(c, a.CreditCard)
				if err != nil {
					return nil, err
				}
				purchases, payments, err := c.transactions.CardUsage(current.ID)
				if err != nil {
					return nil, err
				}
				if !a.Confirm {
					return confirmation(fmt.Sprintf("Delete credit card %s (%s) permanently, with %s and %s?", current.Code, current.Name, plural(purchases, "transaction"), plural(payments, "payment"))), nil
				}
				if err := c.purge(current, payments > 0); err != nil {
					return nil, err
				}
				return map[string]any{"deleted": true, "credit_card": current.Code, "transactions_deleted": purchases, "payments_converted": payments}, nil
			})),
	}
}

// --- Goals ---------------------------------------------------------------

func newGoalsCommand(db *gorm.DB) *goalsCommand {
	return &goalsCommand{streams: quiet, db: db, store: goal.NewStore(db)}
}

// reload returns the goal as the database has it now.
func (c *goalsCommand) reload(id uint) (any, error) {
	found, err := c.store.Find(strconv.FormatUint(uint64(id), 10), true)
	if err != nil {
		return nil, err
	}
	return map[string]any{"goal": goalResult(found)}, nil
}

func goalTools() []mcp.Tool {
	type referenceArguments struct {
		Goal string `json:"goal" mcp:"Code or id of the goal"`
	}
	type addArguments struct {
		Title       string  `json:"title" mcp:"Title of the goal, at most 60 characters"`
		Type        string  `json:"type" mcp:"How a linked transaction counts. save and purchase: an income adds and an expense removes. payoff: an expense adds and an income removes" enum:"save,payoff,purchase"`
		Target      string  `json:"target" mcp:"Amount to reach, as a decimal string, for example \"10000\""`
		Currency    string  `json:"currency" mcp:"Currency of the goal; it cannot change later. Only transactions in this currency can link" enum:"USD,EUR,BRL,BTC"`
		DueDate     *string `json:"due_date,omitempty" mcp:"Final date, YYYY-MM-DD or DD/MM/YYYY. Optional"`
		Code        *string `json:"code,omitempty" mcp:"Short code, 1 to 5 letters and digits. Default: made from the title"`
		Description *string `json:"description,omitempty" mcp:"Description, at most 200 characters"`
	}
	type editArguments struct {
		Goal        string  `json:"goal" mcp:"Code or id of the goal"`
		Title       *string `json:"title,omitempty" mcp:"New title"`
		Code        *string `json:"code,omitempty" mcp:"New code, 1 to 5 letters and digits"`
		Description *string `json:"description,omitempty" mcp:"New description"`
		Type        *string `json:"type,omitempty" mcp:"New type; the progress is counted again" enum:"save,payoff,purchase"`
		Target      *string `json:"target,omitempty" mcp:"New amount to reach, as a decimal string"`
		DueDate     *string `json:"due_date,omitempty" mcp:"New final date, YYYY-MM-DD or DD/MM/YYYY. An empty string removes it"`
	}
	type deleteArguments struct {
		Goal    string `json:"goal" mcp:"Code or id of the goal"`
		Confirm bool   `json:"confirm,omitempty" mcp:"Set to true to delete. Without it the tool only tells what it will delete."`
	}
	// find finds a goal, also when it is archived.
	find := func(c *goalsCommand, ref string) (goal.Goal, error) {
		return c.resolve("show", ref, "", scopeAll)
	}
	// linked is a transaction of a goal with what it adds to the goal.
	type linked struct {
		transactionJSON
		Effect cashJSON `json:"effect"`
	}
	return []mcp.Tool{
		reads(tool("goal_list", "List the goals with their target, progress and status. The progress comes from the transactions linked to each goal. Status is active, reached or overdue.",
			func(db *gorm.DB, a listArguments) (any, error) {
				goals, err := goal.NewStore(db).List(a.IncludeArchived)
				if err != nil {
					return nil, err
				}
				results := make([]goalJSON, len(goals))
				for i, current := range goals {
					results[i] = goalResult(current)
				}
				return map[string]any{"goals": results}, nil
			})),
		reads(tool("goal_show", "Show one goal and the transactions linked to it, the newest first. Each transaction has effect: what it adds to the goal, below zero when it removes. A transfer shows one time.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := newGoalsCommand(db)
				found, err := find(c, a.Goal)
				if err != nil {
					return nil, err
				}
				details, err := c.gather(found)
				if err != nil {
					return nil, err
				}
				transactions := []linked{}
				for _, view := range details.Transactions {
					if effect := found.Effect(view.Type, view.Amount); effect != 0 {
						transactions = append(transactions, linked{transactionResult(view), cash(effect, found.Currency)})
					}
				}
				notes, err := linkedNotes(db, note.LinkGoal, found.ID)
				if err != nil {
					return nil, err
				}
				return map[string]any{"goal": goalResult(found), "transactions": transactions, "notes": notes}, nil
			})),
		tool("goal_add", "Create a goal: an amount to save, a debt to pay off or a purchase to save for. Link transactions to it with the argument goal of transaction_add and transaction_edit.",
			func(db *gorm.DB, a addArguments) (any, error) {
				c := newGoalsCommand(db)
				values := goalFlags{
					title: a.Title, kind: a.Type, target: a.Target, currency: a.Currency,
					due: text(a.DueDate), code: text(a.Code), description: text(a.Description),
				}
				sent := given(map[string]bool{
					"title": true, "type": true, "target": true, "currency": true,
					"due": a.DueDate != nil, "code": a.Code != nil, "description": a.Description != nil,
				})
				var draft goal.Goal
				if err := values.apply("add", sent, &draft); err != nil {
					return nil, err
				}
				if a.Code == nil {
					if draft.Code = c.suggest(draft.Title); draft.Code == "" {
						return nil, fmt.Errorf("cannot make a code from the title %q; give code", draft.Title)
					}
				}
				if err := c.store.Create(&draft); err != nil {
					return nil, err
				}
				return c.reload(draft.ID)
			}),
		tool("goal_edit", "Change a goal. Give only the arguments to change. The currency cannot change.",
			func(db *gorm.DB, a editArguments) (any, error) {
				c := newGoalsCommand(db)
				values := goalFlags{
					title: text(a.Title), code: text(a.Code), description: text(a.Description), kind: text(a.Type),
					target: text(a.Target), due: text(a.DueDate),
				}
				sent := given(map[string]bool{
					"title": a.Title != nil, "code": a.Code != nil, "description": a.Description != nil, "type": a.Type != nil,
					"target": a.Target != nil, "due": a.DueDate != nil,
				})
				if len(sent) == 0 {
					return nil, errors.New("give at least one of title, code, description, type, target, due_date")
				}
				current, err := find(c, a.Goal)
				if err != nil {
					return nil, err
				}
				if current.Archived() {
					return nil, fmt.Errorf("goal not found: %s (it is archived; restore it first)", current.Code)
				}
				if err := values.apply("edit", sent, &current); err != nil {
					return nil, err
				}
				if err := c.store.Update(&current); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		tool("goal_archive", "Archive a goal: it leaves the lists and takes no new transactions, but its transactions keep the link. goal_restore brings it back.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := newGoalsCommand(db)
				current, err := find(c, a.Goal)
				if err != nil {
					return nil, err
				}
				if current.Archived() {
					return nil, fmt.Errorf("goal %s is already archived", current.Code)
				}
				if err := c.store.Archive(current.ID); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		tool("goal_restore", "Restore an archived goal.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := newGoalsCommand(db)
				current, err := find(c, a.Goal)
				if err != nil {
					return nil, err
				}
				if !current.Archived() {
					return nil, fmt.Errorf("goal %s is not archived", current.Code)
				}
				if err := c.store.Restore(current.ID); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		deletes(tool("goal_delete", "Delete a goal permanently. No transaction is deleted: the linked transactions stay and only lose the link. To keep the goal and its links, use goal_archive.",
			func(db *gorm.DB, a deleteArguments) (any, error) {
				c := newGoalsCommand(db)
				current, err := find(c, a.Goal)
				if err != nil {
					return nil, err
				}
				if !a.Confirm {
					return confirmation(fmt.Sprintf("Delete goal %s (%s) permanently%s? No transaction is deleted.", current.Code, current.Title, lostLinks(current, "lose"))), nil
				}
				if err := c.purge(current); err != nil {
					return nil, err
				}
				return map[string]any{"deleted": true, "goal": current.Code, "transactions_unlinked": current.Links}, nil
			})),
	}
}

// --- Budgets -------------------------------------------------------------

func newBudgetsCommand(db *gorm.DB) *budgetsCommand {
	return &budgetsCommand{streams: quiet, db: db, store: budget.NewStore(db)}
}

// results makes results from budgets. It reads the categories one time
// for their codes.
func (c *budgetsCommand) results(budgets []budget.Budget) ([]budgetJSON, error) {
	categories, err := category.NewStore(c.db).List()
	if err != nil {
		return nil, err
	}
	results := make([]budgetJSON, len(budgets))
	for i, b := range budgets {
		codes := []string{}
		for _, known := range categories {
			if slices.Contains(b.CategoryIDs, known.ID) {
				codes = append(codes, known.Code)
			}
		}
		results[i] = budgetJSON{
			ID: b.ID, Code: b.Code, Title: b.Title, Description: b.Description, Currency: b.Currency, Archived: b.Archived(),
			Limit: cash(b.Limit, b.Currency), Categories: codes, Period: b.Recurrence(), Every: b.Every, Unit: b.Unit,
			StartDate: b.StartDate, EndDate: b.EndDate, Rollover: b.Rollover, Status: b.Status(today()),
		}
		if usage, ok := b.Shown(today()); ok {
			current := usageResult(usage, b.Currency)
			results[i].Current = &current
		}
	}
	return results, nil
}

// reload returns the budget as the database has it now.
func (c *budgetsCommand) reload(id uint) (any, error) {
	found, err := c.store.Find(strconv.FormatUint(uint64(id), 10), true, today())
	if err != nil {
		return nil, err
	}
	results, err := c.results([]budget.Budget{found})
	if err != nil {
		return nil, err
	}
	return map[string]any{"budget": results[0]}, nil
}

func budgetTools() []mcp.Tool {
	type referenceArguments struct {
		Budget string `json:"budget" mcp:"Code or id of the budget"`
	}
	type showArguments struct {
		Budget  string  `json:"budget" mcp:"Code or id of the budget"`
		Date    *string `json:"date,omitempty" mcp:"A date in the period to show, YYYY-MM-DD or DD/MM/YYYY. Default: the current period"`
		History *int    `json:"history,omitempty" mcp:"Number of periods before it to return, the newest first. Default: 6"`
	}
	type addArguments struct {
		Title       string  `json:"title" mcp:"Title of the budget, at most 60 characters"`
		Limit       string  `json:"limit" mcp:"Amount of one period, as a decimal string, for example \"800\""`
		Currency    string  `json:"currency" mcp:"Currency of the budget; it cannot change later. Only transactions in this currency count" enum:"USD,EUR,BRL,BTC"`
		Categories  *string `json:"categories,omitempty" mcp:"Codes of the categories that count, for example \"FOOD,FUN\". Default, or \"all\": all categories but the income ones"`
		Every       *string `json:"every,omitempty" mcp:"Length of a period of a recurring budget: \"day\", \"week\", \"month\", \"year\" or a number of them, for example \"2 weeks\". Without it the budget is for one time and needs end_date"`
		StartDate   *string `json:"start_date,omitempty" mcp:"First day of the first period. The periods count from it: a monthly budget that starts on day 5 has periods from day 5 to day 4. Default: the first day of this week, month or year for a period of one of them; today for the others"`
		EndDate     *string `json:"end_date,omitempty" mcp:"Last day of the budget. Optional with every; necessary without it"`
		Rollover    *bool   `json:"rollover,omitempty" mcp:"Move what a period did not spend, or spent too much, to the next period. Default: false"`
		Code        *string `json:"code,omitempty" mcp:"Short code, 1 to 5 letters and digits. Default: made from the title"`
		Description *string `json:"description,omitempty" mcp:"Description, at most 200 characters"`
	}
	type editArguments struct {
		Budget      string  `json:"budget" mcp:"Code or id of the budget"`
		Title       *string `json:"title,omitempty" mcp:"New title"`
		Code        *string `json:"code,omitempty" mcp:"New code, 1 to 5 letters and digits"`
		Description *string `json:"description,omitempty" mcp:"New description"`
		Limit       *string `json:"limit,omitempty" mcp:"New amount of one period, as a decimal string. It also changes the periods in the past"`
		Categories  *string `json:"categories,omitempty" mcp:"New codes of the categories that count; they replace the list. \"all\" counts all categories but the income ones"`
		Every       *string `json:"every,omitempty" mcp:"New length of a period, for example \"month\" or \"2 weeks\". \"none\" makes a one-time budget, which needs an end date"`
		StartDate   *string `json:"start_date,omitempty" mcp:"New first day of the first period"`
		EndDate     *string `json:"end_date,omitempty" mcp:"New last day of the budget. An empty string removes it"`
		Rollover    *bool   `json:"rollover,omitempty" mcp:"Start or stop the move of what a period left to the next period"`
	}
	type deleteArguments struct {
		Budget  string `json:"budget" mcp:"Code or id of the budget"`
		Confirm bool   `json:"confirm,omitempty" mcp:"Set to true to delete. Without it the tool only tells what it will delete."`
	}
	// find finds a budget, also when it is archived.
	find := func(c *budgetsCommand, ref string) (budget.Budget, error) {
		return c.resolve("show", ref, "", scopeAll, today())
	}
	// counted is a transaction of a period with what it adds to the spent
	// amount.
	type counted struct {
		transactionJSON
		Counts cashJSON `json:"counts"`
	}
	return []mcp.Tool{
		reads(tool("budget_list", "List the budgets with their limit, categories, period and status. current is the period of today: what it has available, what it spent and what remains. Status is ok, near (80% spent or more), over, upcoming or ended.",
			func(db *gorm.DB, a listArguments) (any, error) {
				c := newBudgetsCommand(db)
				budgets, err := c.store.List(a.IncludeArchived, today())
				if err != nil {
					return nil, err
				}
				results, err := c.results(budgets)
				if err != nil {
					return nil, err
				}
				return map[string]any{"budgets": results}, nil
			})),
		reads(tool("budget_show", "Show one budget with one period, the transactions that count in it, the newest first, and the periods before it. Each transaction has counts: what it adds to the spent amount, below zero for an income. Give date to see the period of a date in the past.",
			func(db *gorm.DB, a showArguments) (any, error) {
				c := newBudgetsCommand(db)
				date, until, history := "", today(), defaultHistory
				if a.Date != nil {
					var err error
					if date, err = transaction.ParseDate(*a.Date); err != nil {
						return nil, err
					}
					until = max(until, date)
				}
				if a.History != nil {
					if history = *a.History; history < 0 {
						return nil, errors.New("history must be 0 or more")
					}
				}
				found, err := c.resolve("show", a.Budget, "", scopeAll, until)
				if err != nil {
					return nil, err
				}
				details, err := c.gather(found, date, history)
				if err != nil {
					return nil, err
				}
				results, err := c.results([]budget.Budget{found})
				if err != nil {
					return nil, err
				}
				notes, err := linkedNotes(db, note.LinkBudget, found.ID)
				if err != nil {
					return nil, err
				}
				result := map[string]any{"budget": results[0], "notes": notes}
				if details.Usage != nil {
					result["period"] = usageResult(*details.Usage, found.Currency)
				}
				transactions := []counted{}
				for _, view := range details.Transactions {
					transactions = append(transactions, counted{transactionResult(view), cash(-view.Amount, found.Currency)})
				}
				past := []usageJSON{}
				for _, usage := range details.History {
					past = append(past, usageResult(usage, found.Currency))
				}
				result["transactions"], result["history"] = transactions, past
				if details.Scheduled > 0 {
					result["scheduled"] = cash(details.Scheduled, found.Currency)
				}
				return result, nil
			})),
		tool("budget_add", "Create a budget: a limit of money for some categories in a period. Every expense and income of those categories counts on it without a link: an expense adds to the spent amount and an income gives money back. Transfers and payments of credit card bills do not count. Give every for a recurring budget, or end_date for a one-time budget.",
			func(db *gorm.DB, a addArguments) (any, error) {
				c := newBudgetsCommand(db)
				if a.Every == nil && a.EndDate == nil {
					return nil, errors.New("give every or end_date")
				}
				values := budgetFlags{
					title: a.Title, limit: a.Limit, currency: a.Currency, categories: text(a.Categories), every: text(a.Every),
					start: text(a.StartDate), until: text(a.EndDate), rollover: a.Rollover != nil && *a.Rollover,
					code: text(a.Code), description: text(a.Description),
				}
				sent := given(map[string]bool{
					"title": true, "limit": true, "currency": true, "categories": a.Categories != nil, "every": a.Every != nil,
					"start": a.StartDate != nil, "until": a.EndDate != nil, "rollover": a.Rollover != nil,
					"code": a.Code != nil, "description": a.Description != nil,
				})
				var draft budget.Budget
				if err := c.apply("add", values, sent, &draft); err != nil {
					return nil, err
				}
				if a.StartDate == nil {
					draft.StartDate = budget.DefaultStart(draft.Every, draft.Unit, today())
				}
				if err := checkPeriod("add", draft); err != nil {
					return nil, err
				}
				if a.Code == nil {
					if draft.Code = code.Suggest(draft.Title, c.taken(0)); draft.Code == "" {
						return nil, fmt.Errorf("cannot make a code from the title %q; give code", draft.Title)
					}
				}
				if err := c.store.Create(&draft); err != nil {
					return nil, err
				}
				return c.reload(draft.ID)
			}),
		tool("budget_edit", "Change a budget. Give only the arguments to change. The currency cannot change. A change of the limit, the categories or the dates counts all the periods again.",
			func(db *gorm.DB, a editArguments) (any, error) {
				c := newBudgetsCommand(db)
				values := budgetFlags{
					title: text(a.Title), code: text(a.Code), description: text(a.Description), limit: text(a.Limit),
					categories: text(a.Categories), every: text(a.Every), start: text(a.StartDate), until: text(a.EndDate),
					rollover: a.Rollover != nil && *a.Rollover,
				}
				sent := given(map[string]bool{
					"title": a.Title != nil, "code": a.Code != nil, "description": a.Description != nil, "limit": a.Limit != nil,
					"categories": a.Categories != nil, "every": a.Every != nil, "start": a.StartDate != nil, "until": a.EndDate != nil,
					"rollover": a.Rollover != nil,
				})
				if len(sent) == 0 {
					return nil, errors.New("give at least one of title, code, description, limit, categories, every, start_date, end_date, rollover")
				}
				current, err := find(c, a.Budget)
				if err != nil {
					return nil, err
				}
				if current.Archived() {
					return nil, fmt.Errorf("budget not found: %s (it is archived; restore it first)", current.Code)
				}
				if err := c.apply("edit", values, sent, &current); err != nil {
					return nil, err
				}
				if err := checkPeriod("edit", current); err != nil {
					return nil, err
				}
				if err := c.store.Update(&current); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		tool("budget_archive", "Archive a budget: it leaves the lists and gives no warnings, but it keeps its history. budget_restore brings it back.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := newBudgetsCommand(db)
				current, err := find(c, a.Budget)
				if err != nil {
					return nil, err
				}
				if current.Archived() {
					return nil, fmt.Errorf("budget %s is already archived", current.Code)
				}
				if err := c.store.Archive(current.ID); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		tool("budget_restore", "Restore an archived budget.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := newBudgetsCommand(db)
				current, err := find(c, a.Budget)
				if err != nil {
					return nil, err
				}
				if !current.Archived() {
					return nil, fmt.Errorf("budget %s is not archived", current.Code)
				}
				if err := c.store.Restore(current.ID); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		deletes(tool("budget_delete", "Delete a budget permanently. No transaction changes: a budget has no link to its transactions. To keep the budget and its history, use budget_archive.",
			func(db *gorm.DB, a deleteArguments) (any, error) {
				c := newBudgetsCommand(db)
				current, err := find(c, a.Budget)
				if err != nil {
					return nil, err
				}
				if !a.Confirm {
					return confirmation(fmt.Sprintf("Delete budget %s (%s) permanently? No transaction changes.", current.Code, current.Title)), nil
				}
				if err := c.purge(current.ID); err != nil {
					return nil, err
				}
				return map[string]any{"deleted": true, "budget": current.Code}, nil
			})),
	}
}
