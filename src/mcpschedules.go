package main

import (
	"errors"
	"fmt"
	"strconv"

	"finux/src/code"
	"finux/src/mcp"
	"finux/src/schedule"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

type scheduleJSON struct {
	ID          uint     `json:"id"`
	Code        string   `json:"code"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Value       cashJSON `json:"value"`
	Currency    string   `json:"currency"`
	// Account is the code of the account. A schedule of a credit card
	// has credit_card in its place.
	Account    string   `json:"account,omitempty"`
	CreditCard string   `json:"credit_card,omitempty"`
	Category   string   `json:"category"`
	Tags       []string `json:"tags"`
	Goal       string   `json:"goal,omitempty"`
	// Repeats is the period in words: "every month" or "one time".
	Repeats string `json:"repeats"`
	EndDate string `json:"end_date,omitempty"`
	// TimesLeft is the number of occurrences that wait. A schedule
	// without an end has none.
	TimesLeft *int   `json:"times_left,omitempty"`
	Automatic bool   `json:"automatic"`
	Archived  bool   `json:"archived"`
	Status    string `json:"status"`
	// Due holds the dates of the occurrences that wait, up to today. Next
	// is the date of the first occurrence after today.
	Due  []string `json:"due"`
	Next string   `json:"next,omitempty"`
}

func scheduleResult(view ui.ScheduleView) scheduleJSON {
	result := scheduleJSON{
		ID: view.ID, Code: view.Code, Title: view.Title, Description: view.Description, Type: view.Type,
		Value: cash(view.Value, view.Account.Currency), Currency: view.Account.Currency, Category: view.Category.Code,
		Tags: append([]string{}, transaction.Transaction{Tags: view.Tags}.TagList()...), Repeats: view.Recurrence(), EndDate: view.EndDate,
		Automatic: view.Auto, Archived: view.Archived(), Status: view.Status(today()), Due: []string{},
	}
	if view.OnCard {
		result.CreditCard = view.Account.Code
	} else {
		result.Account = view.Account.Code
	}
	if view.Goal != nil {
		result.Goal = view.Goal.Code
	}
	if left := view.Left(); left >= 0 {
		result.TimesLeft = &left
	}
	later := view.Schedule
	if !view.Archived() {
		result.Due = append(result.Due, view.Pending(today())...)
		later.Done += len(result.Due)
	}
	if next, ok := later.Next(); ok {
		result.Next = next
	}
	return result
}

// reload returns a schedule as a result, read again from the database.
func (c *scheduledCommand) reload(id uint) (map[string]any, error) {
	found, err := c.store.Find(strconv.FormatUint(uint64(id), 10), true)
	if err != nil {
		return nil, err
	}
	view, err := c.view(found)
	if err != nil {
		return nil, err
	}
	return map[string]any{"schedule": scheduleResult(view)}, nil
}

func scheduleTools() []mcp.Tool {
	type referenceArguments struct {
		Schedule string `json:"schedule" mcp:"Code or id of the schedule"`
	}
	type addArguments struct {
		Title       string  `json:"title" mcp:"Title of the transactions, at most 60 characters"`
		Value       string  `json:"value" mcp:"Positive amount in the currency of the account, as a decimal string. The type gives the sign"`
		Account     string  `json:"account" mcp:"Code or id of the account or the credit card of the transactions"`
		Category    string  `json:"category" mcp:"Code or id of the category"`
		Type        *string `json:"type,omitempty" mcp:"Default: expense for a red category, income for a green one. Necessary for a grey category" enum:"expense,income"`
		Date        *string `json:"date,omitempty" mcp:"Date of the first occurrence, YYYY-MM-DD or DD/MM/YYYY. Default: today"`
		Every       *string `json:"every,omitempty" mcp:"Time between two occurrences: day, week, month, year, or a number of them, for example \"2 weeks\". Without it the schedule is for one time"`
		EndDate     *string `json:"end_date,omitempty" mcp:"With every: the last day an occurrence can have"`
		Times       *int    `json:"times,omitempty" mcp:"With every: the number of occurrences, from 1 to 1000"`
		Automatic   *bool   `json:"automatic,omitempty" mcp:"True creates each transaction on its date without a confirmation. Default: false. Set it only when the user asks for it"`
		Tags        *string `json:"tags,omitempty" mcp:"Tags of the transactions, divided by spaces"`
		Goal        *string `json:"goal,omitempty" mcp:"Code or id of the goal the transactions count toward"`
		Code        *string `json:"code,omitempty" mcp:"Short code, 1 to 5 letters and digits. Default: made from the title"`
		Description *string `json:"description,omitempty" mcp:"Description, at most 200 characters"`
	}
	type editArguments struct {
		Schedule    string  `json:"schedule" mcp:"Code or id of the schedule"`
		Title       *string `json:"title,omitempty" mcp:"New title"`
		Code        *string `json:"code,omitempty" mcp:"New code, 1 to 5 letters and digits"`
		Description *string `json:"description,omitempty" mcp:"New description"`
		Value       *string `json:"value,omitempty" mcp:"New positive amount as a decimal string"`
		Account     *string `json:"account,omitempty" mcp:"Code or id of the new account or credit card. Give value also when the currency is different"`
		Category    *string `json:"category,omitempty" mcp:"Code or id of the new category"`
		Type        *string `json:"type,omitempty" mcp:"New type" enum:"expense,income"`
		Date        *string `json:"date,omitempty" mcp:"New date of the next occurrence. The occurrences after it count from this date"`
		Every       *string `json:"every,omitempty" mcp:"New time between two occurrences. \"none\" makes the schedule for one time"`
		EndDate     *string `json:"end_date,omitempty" mcp:"New last day. An empty string removes it"`
		Times       *int    `json:"times,omitempty" mcp:"New number of occurrences from now on. 0 removes it"`
		Automatic   *bool   `json:"automatic,omitempty" mcp:"True creates each transaction on its date without a confirmation; false stops it"`
		Tags        *string `json:"tags,omitempty" mcp:"New tags. An empty string removes all tags"`
		Goal        *string `json:"goal,omitempty" mcp:"Code or id of the new goal. An empty string takes the goal away"`
	}
	type payArguments struct {
		Schedule string  `json:"schedule" mcp:"Code or id of the schedule"`
		Value    *string `json:"value,omitempty" mcp:"Real amount, when it is not the amount of the schedule"`
		Date     *string `json:"date,omitempty" mcp:"Real date. Default: the date of the occurrence, or today for an occurrence that is not due yet"`
		Account  *string `json:"account,omitempty" mcp:"Code or id of the account or credit card that paid, when it is another one"`
	}
	type deleteArguments struct {
		Schedule string `json:"schedule" mcp:"Code or id of the schedule"`
		Confirm  bool   `json:"confirm,omitempty" mcp:"Set to true to delete. Without it the tool only tells what it will delete."`
	}
	open := func(db *gorm.DB) *scheduledCommand { return newScheduledCommand(db, quiet) }
	// times is the text of a number of occurrences. 0 removes it in an edit.
	times := func(value *int) string {
		if number(value) == 0 {
			return noValue
		}
		return strconv.Itoa(*value)
	}
	return []mcp.Tool{
		reads(tool("schedule_list", "List the scheduled transactions: bills to pay and incomes to receive, one time or again and again. A schedule is not a transaction: it changes no balance until schedule_pay. Each schedule has due, the dates of the occurrences that wait for a confirmation, and next. The result has due: the number of occurrences that wait in all schedules. Without include_archived, the schedules that are done or archived stay out.",
			func(db *gorm.DB, a listArguments) (any, error) {
				c := open(db)
				schedules, err := c.store.List(a.IncludeArchived)
				if err != nil {
					return nil, err
				}
				views, err := scheduleViews(db, schedules)
				if err != nil {
					return nil, err
				}
				results, due := []scheduleJSON{}, 0
				for _, view := range views {
					if !a.IncludeArchived && view.Status(today()) == schedule.StatusDone {
						continue
					}
					result := scheduleResult(view)
					results, due = append(results, result), due+len(result.Due)
				}
				return map[string]any{"schedules": results, "due": due}, nil
			})),
		reads(tool("schedule_show", "Show one schedule and the transactions it created, the newest first.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := open(db)
				found, err := c.resolve("show", a.Schedule, "", scopeAll)
				if err != nil {
					return nil, err
				}
				result, err := c.reload(found.ID)
				if err != nil {
					return nil, err
				}
				created, err := c.tx.store.BySchedule(found.ID)
				if err != nil {
					return nil, err
				}
				result["transactions"] = transactionResults(created)
				return result, nil
			})),
		tool("schedule_add", "Schedule a transaction that will happen: an expense or an income, one time (no every) or again and again (every). It creates no transaction. Each occurrence waits until schedule_pay creates its transaction or schedule_skip drops it.",
			func(db *gorm.DB, a addArguments) (any, error) {
				c := open(db)
				values := scheduleFlags{
					title: a.Title, value: a.Value, account: a.Account, category: a.Category, kind: text(a.Type), date: text(a.Date),
					every: text(a.Every), until: text(a.EndDate), times: strconv.Itoa(number(a.Times)), auto: a.Automatic != nil && *a.Automatic,
					tags: text(a.Tags), goal: text(a.Goal), code: text(a.Code), description: text(a.Description),
				}
				sent := given(map[string]bool{
					"title": true, "value": true, "account": true, "category": true, "type": a.Type != nil, "date": a.Date != nil,
					"every": a.Every != nil, "until": a.EndDate != nil, "times": a.Times != nil, "auto": a.Automatic != nil,
					"tags": a.Tags != nil, "goal": a.Goal != nil, "code": a.Code != nil, "description": a.Description != nil,
				})
				var draft schedule.Schedule
				if err := c.apply("add", values, sent, &draft); err != nil {
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
		tool("schedule_edit", "Change a schedule. Give only the arguments to change. The change is for the occurrences that wait; the transactions it already created stay as they are.",
			func(db *gorm.DB, a editArguments) (any, error) {
				c := open(db)
				values := scheduleFlags{
					title: text(a.Title), code: text(a.Code), description: text(a.Description), value: text(a.Value), account: text(a.Account),
					category: text(a.Category), kind: text(a.Type), date: text(a.Date), every: text(a.Every), until: text(a.EndDate),
					times: times(a.Times), auto: a.Automatic != nil && *a.Automatic, tags: text(a.Tags), goal: text(a.Goal),
				}
				sent := given(map[string]bool{
					"title": a.Title != nil, "code": a.Code != nil, "description": a.Description != nil, "value": a.Value != nil,
					"account": a.Account != nil, "category": a.Category != nil, "type": a.Type != nil, "date": a.Date != nil,
					"every": a.Every != nil, "until": a.EndDate != nil, "times": a.Times != nil, "auto": a.Automatic != nil,
					"tags": a.Tags != nil, "goal": a.Goal != nil,
				})
				if len(sent) == 0 {
					return nil, errors.New("give at least one of title, code, description, value, account, category, type, date, every, end_date, times, automatic, tags, goal")
				}
				current, err := c.active("edit", a.Schedule, "")
				if err != nil {
					return nil, err
				}
				if err := c.apply("edit", values, sent, &current); err != nil {
					return nil, err
				}
				if err := c.store.Update(&current); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		tool("schedule_pay", "Confirm the next occurrence of a schedule: create its transaction. Use it when the user paid the bill or got the income. The occurrences go in order, the oldest first. Give value, date or account when the real ones are different. The result has the transaction, the schedule as it is now, and budget_warnings.",
			func(db *gorm.DB, a payArguments) (any, error) {
				c := open(db)
				current, err := c.active("pay", a.Schedule, "")
				if err != nil {
					return nil, err
				}
				sent := given(map[string]bool{"value": a.Value != nil, "date": a.Date != nil, "account": a.Account != nil})
				draft, err := c.payDraft(current, text(a.Value), text(a.Date), text(a.Account), sent)
				if err != nil {
					return nil, err
				}
				created, err := settle(db, current, draft)
				if err != nil {
					return nil, err
				}
				result, err := c.tx.savedResult(created.ID)
				if err != nil {
					return nil, err
				}
				after, err := c.reload(current.ID)
				if err != nil {
					return nil, err
				}
				result.(map[string]any)["schedule"] = after["schedule"]
				return result, nil
			}),
		tool("schedule_skip", "Skip the next occurrence of a schedule: it creates no transaction and the occurrence does not wait any more. Use it when the bill did not come or the user will not pay it.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := open(db)
				current, err := c.active("skip", a.Schedule, "")
				if err != nil {
					return nil, err
				}
				skipped, ok := current.Next()
				if !ok {
					return nil, errNoOccurrence(current)
				}
				if err := c.store.Advance(current.ID); err != nil {
					return nil, err
				}
				result, err := c.reload(current.ID)
				if err != nil {
					return nil, err
				}
				result["skipped"] = skipped
				return result, nil
			}),
		tool("schedule_archive", "Archive a schedule: it stops and leaves the lists. Its transactions stay. schedule_restore brings it back.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := open(db)
				current, err := c.resolve("delete", a.Schedule, "", scopeAll)
				if err != nil {
					return nil, err
				}
				if current.Archived() {
					return nil, fmt.Errorf("schedule %s is already archived", current.Code)
				}
				if err := c.store.Archive(current.ID); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		tool("schedule_restore", "Restore an archived schedule. The occurrences whose dates passed are due again.",
			func(db *gorm.DB, a referenceArguments) (any, error) {
				c := open(db)
				current, err := c.resolve("restore", a.Schedule, "", scopeAll)
				if err != nil {
					return nil, err
				}
				if !current.Archived() {
					return nil, fmt.Errorf("schedule %s is not archived", current.Code)
				}
				if err := c.store.Restore(current.ID); err != nil {
					return nil, err
				}
				return c.reload(current.ID)
			}),
		deletes(tool("schedule_delete", "Delete a schedule permanently. No transaction is deleted: the transactions it created stay and only lose the link. To keep the schedule, use schedule_archive.",
			func(db *gorm.DB, a deleteArguments) (any, error) {
				c := open(db)
				current, err := c.resolve("delete", a.Schedule, "", scopeAll)
				if err != nil {
					return nil, err
				}
				if !a.Confirm {
					return confirmation(fmt.Sprintf("Delete schedule %s (%s) permanently? Its transactions stay.", current.Code, current.Title)), nil
				}
				if err := purgeSchedule(db, current.ID); err != nil {
					return nil, err
				}
				return map[string]any{"deleted": true, "schedule": current.Code}, nil
			})),
	}
}
