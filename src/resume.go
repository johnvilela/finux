package main

import (
	"cmp"
	"flag"
	"fmt"
	"slices"
	"time"

	"finux/src/account"
	"finux/src/budget"
	"finux/src/card"
	"finux/src/goal"
	"finux/src/schedule"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

func resumeHelp() string {
	return "Show your financial resume\n\n" +
		green.Render("Usage:") + " finux resume [options]\n" +
		"       finux rs [options]\n\n" +
		green.Render("Options:") + `
  --month <month>   Month of the incomes and expenses, MM/YYYY or YYYY/MM
                    (default: the month of today)
  -h, --help        Show this help

The resume shows, for each currency, the incomes, the expenses and the top
expense categories of the month. A transfer and the payment of a credit card
bill do not count: a purchase with a card counts on the day of the purchase.
'Per day' is the expenses divided by the days of the month up to today.

The resume also shows the accounts with their balances, the credit card
bills to pay (up to the bill that is open now), the budgets with a period today and the goals. These are always
as they are today, also with --month.

'Scheduled' lists the occurrences of your schedules that are due and the
ones that come until the end of this month. 'After' is the total of the
balances of a currency after all of them. An occurrence on a credit card
is not in a balance. See 'finux sc --help'.
`
}

func runResume(args []string, io streams) int {
	for _, arg := range args {
		if isHelp(arg) {
			return write(io, resumeHelp())
		}
	}
	var month string
	_, given, err := parse("resume", args, func(f *flag.FlagSet) { f.StringVar(&month, "month", "", "") }, 0)
	if err != nil {
		return exitStatus(io, "resume", err)
	}
	resume := ui.Resume{Today: today()}
	resume.Month = resume.Today[:len("2006-01")]
	if given["month"] {
		if resume.Month, err = card.ParsePeriod(month); err != nil {
			return exitStatus(io, "resume", usageError(fmt.Sprintf("invalid month %q: use MM/YYYY or YYYY/MM", month)))
		}
	}
	db, err := openLedger(io)
	if err == nil {
		defer closeDatabase(db)
		err = gatherResume(db, &resume)
	}
	if err != nil {
		return exitStatus(io, "resume", err)
	}
	return write(io, ui.ResumeView(resume, io.plain))
}

// monthDays returns the number of days of a YYYY-MM month up to the
// date of today: all of them for a month that ended, and 0 for a month
// that did not start.
func monthDays(month, today string) int {
	switch current := today[:len(month)]; {
	case month > current:
		return 0
	case month == current:
		day, _ := time.Parse("2006-01-02", today)
		return day.Day()
	}
	first, _ := time.Parse("2006-01", month)
	return first.AddDate(0, 1, -1).Day()
}

// monthFlows adds up the incomes and the expenses of transactions, for
// each currency. A transfer and a payment of a bill do not count.
func monthFlows(views []transaction.View) []ui.ResumeFlow {
	flows := map[string]*ui.ResumeFlow{}
	spent := map[string]map[uint]*ui.ResumeCategory{}
	for _, view := range views {
		if transaction.Neutral(view.Type) {
			continue
		}
		currency := view.Account.Currency
		flow := flows[currency]
		if flow == nil {
			flow = &ui.ResumeFlow{Currency: currency}
			flows[currency], spent[currency] = flow, map[uint]*ui.ResumeCategory{}
		}
		if view.Type == transaction.Income {
			flow.Income += view.Amount
			continue
		}
		flow.Expenses -= view.Amount
		group := spent[currency][view.CategoryID]
		if group == nil {
			group = &ui.ResumeCategory{Category: view.Category}
			spent[currency][view.CategoryID] = group
		}
		group.Amount -= view.Amount
	}
	result := make([]ui.ResumeFlow, 0, len(flows))
	for currency, flow := range flows {
		for _, group := range spent[currency] {
			flow.Categories = append(flow.Categories, *group)
		}
		slices.SortFunc(flow.Categories, func(a, b ui.ResumeCategory) int {
			return cmp.Or(cmp.Compare(b.Amount, a.Amount), cmp.Compare(a.Category.Code, b.Category.Code))
		})
		result = append(result, *flow)
	}
	slices.SortFunc(result, func(a, b ui.ResumeFlow) int { return cmp.Compare(a.Currency, b.Currency) })
	return result
}

// gatherScheduled reads the occurrences of the schedules that wait, up
// to the last day of the month of today.
func gatherScheduled(db *gorm.DB, resume *ui.Resume) error {
	first, _ := time.Parse("2006-01-02", resume.Today[:len("2006-01")]+"-01")
	resume.ScheduledUntil = first.AddDate(0, 1, -1).Format("2006-01-02")
	schedules, err := schedule.NewStore(db).List(false)
	if err != nil {
		return err
	}
	views, err := scheduleViews(db, schedules)
	if err != nil {
		return err
	}
	for _, view := range views {
		for _, date := range view.Pending(resume.ScheduledUntil) {
			resume.Scheduled = append(resume.Scheduled, ui.ResumeOccurrence{View: view, Date: date})
		}
	}
	slices.SortStableFunc(resume.Scheduled, func(a, b ui.ResumeOccurrence) int {
		return cmp.Or(cmp.Compare(a.Date, b.Date), cmp.Compare(a.View.Code, b.View.Code))
	})
	return nil
}

// gatherResume reads what the resume shows. The resume has its month
// and the date of today.
func gatherResume(db *gorm.DB, resume *ui.Resume) error {
	resume.Days = monthDays(resume.Month, resume.Today)
	views, err := transaction.NewStore(db).List(transaction.Filter{Period: resume.Month})
	if err != nil {
		return err
	}
	resume.Flows = monthFlows(views)
	if resume.Accounts, err = account.NewStore(db).List(false, resume.Today); err != nil {
		return err
	}
	if err := gatherScheduled(db, resume); err != nil {
		return err
	}
	cards := card.NewStore(db)
	owners, err := cards.List(false)
	if err != nil {
		return err
	}
	for _, owner := range owners {
		bills, err := cards.Bills(owner.ID)
		if err != nil {
			return err
		}
		// A bill that closes after the current one is not to pay yet: a
		// transaction in installments has a bill for each part.
		current, err := cards.Current(owner, resume.Today)
		if err != nil {
			return err
		}
		for _, bill := range bills {
			if !bill.IsPaid() && bill.Total() > 0 && bill.ClosesOn <= current.ClosesOn {
				resume.Bills = append(resume.Bills, ui.ResumeBill{Card: owner, Bill: bill})
			}
		}
	}
	slices.SortStableFunc(resume.Bills, func(a, b ui.ResumeBill) int { return cmp.Compare(a.Bill.DueOn, b.Bill.DueOn) })
	budgets, err := budget.NewStore(db).List(false, resume.Today)
	if err != nil {
		return err
	}
	for _, current := range budgets {
		if _, ok := current.Current(resume.Today); ok {
			resume.Budgets = append(resume.Budgets, current)
		}
	}
	resume.Goals, err = goal.NewStore(db).List(false)
	return err
}
