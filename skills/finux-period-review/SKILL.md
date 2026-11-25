---
name: finux-period-review
description: Makes a report of a month, a year or a date range from the data in Finux, the personal finance manager of the user, covering income, expenses, net result, categories, the largest transactions, a comparison with the period before, account balances and credit card bills. Use it for "how was my month", "review November", "monthly report", "how much did I spend this year", "where did my money go", "show my finances", "summary of my spending". It only reads data.
---

# Review a period in Finux

Give the user a correct and short picture of a period. This skill reads
data; it changes nothing.

## Before you start

- Reach Finux as [references/access.md](references/access.md) tells: MCP
  tools first, the CLI when there are none.
- The meaning of types, totals and bills is in
  [references/concepts.md](references/concepts.md). The arguments of each
  tool are in [references/tools.md](references/tools.md).

## Rules for the numbers

- Take income, expense and net from `totals` of `transaction_list`. Finux
  calculates them for all transactions that match the filters.
- For all other sums, add the `minor` values with a script or a
  calculator tool. Do not add a long list in your head.
- Keep each currency apart. Never add two currencies, and do not convert:
  Finux has no exchange rates.
- Transfers and payments are not income and not expense. Do not count
  them in the spending.
- A purchase on a credit card is an expense on the date of the purchase,
  not on the date the bill is paid.
- A purchase in installments is one transaction for each monthly part,
  with `installment` and `installments`. Each part is an expense in the
  month of its date: the period has only its own parts, not the total of
  the purchase. In the list of the largest expenses, show a part as
  `Notebook (3/10)`.
- A period in the future has data: the parts of the purchases in
  installments. Report it as "already committed", not as spending.
- The balance of an account counts the transactions up to today.
- A scheduled transaction is not a transaction and is not in the totals.
  It counts in a period only after the user paid it (`schedule` in the
  transaction).
- Give each number one time and say what it is. Do not invent a number
  that the data does not give.

## Steps

### 1. Fix the period

"This month" is the calendar month of today; "last month" is the one
before. Without a period, use the current month, and say that it is not
finished. Send a month as `YYYY-MM` and a year as `YYYY` in `date`. For a
range that is not a month or a year, read the months it touches and keep
the transactions inside the range.

### 2. Read the data

- `transaction_list` with `date` of the period. Read all pages: add 100
  to `offset` until you have `total` transactions.
- `transaction_list` with `date` of the period before, of the same
  length, for the comparison. The `totals` and one read of the pages are
  enough.
- `account_list` for the balances of today.
- The lists and the totals leave out the accounts, the credit cards and
  the transactions of other persons (the ones with an `owner`). Send
  `include_others: true` only when the user asks for all persons, and
  then report that person apart, with the name.
- `credit_card_list`, and `credit_card_show` for each card, for the open
  bill and the bills that wait for a payment.
- `goal_list` for the goals and their progress.
- `schedule_list` for the scheduled transactions that wait.
- `note_list` with `status: due` for the notes that are due: list them at
  the end of the report.
- `budget_list` for the budgets, and `budget_show` with a `date` of the
  period for each budget that had a period in it.

### 3. Calculate, for each currency

- Income, expense, net, and the savings rate (net divided by income)
  when the income is not zero.
- Expenses by category, the largest first, each with its share of the
  expenses. Group the small ones as "other categories" after the first
  eight.
- Income by category.
- The five largest expenses: date, title, value, category.
- Change against the period before: income, expense, and the categories
  that changed the most in value. Compare an unfinished month with care:
  say how many days it has.
- For a year: the totals of each month.

### 4. Write the report

Lead with the result in one sentence: what came in, what went out, what
remained. Then use this order and leave out an empty part:

1. **Summary** for each currency: income, expense, net, savings rate.
2. **Where the money went:** the table of categories.
3. **Largest expenses.**
4. **Against the period before:** what rose and what fell, in value and
   in percent.
5. **Accounts now:** each balance. Say that these are the balances of
   today, not of the end of the period.
6. **Credit cards:** used and available, the open bill, and each bill
   that is closed or late with its due date.
7. **Goals:** each active or overdue goal with its progress, percent,
   what remains and its final date. Say what the period added: the sum
   of `effect` of the transactions of `goal_show` that have a date in
   the period.
8. **Budgets:** each budget that had a period in the period of the
   review, with `spent`, `available`, `percent` and `level`. A budget
   period can start or end on other days than the review: give its
   dates. Put the budgets that are `over` first.
9. **Scheduled:** for a period that has today or is in the future, the
   schedules with a date in `due` (they wait for a confirmation) and the
   ones whose `next` is in the period, with the total of each currency.
   Call it "to pay" or "to receive", not spending. Leave it out for a
   period in the past.
10. **To look at:** at most three facts that need attention, for example
    a late bill, a category that doubled, a scheduled transaction that
    waits, or many transactions in `OTHER`.

Show amounts with `formatted`. Keep the report short enough to read on
one screen; give more detail when the user asks. Give facts, not
judgments about how the user spends.

## Related skills

For an analysis of several months, recurring charges and alerts, use the
skill finux-coach. To pay a bill, use finux-credit-card-bills.
