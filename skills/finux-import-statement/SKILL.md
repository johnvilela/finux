---
name: finux-import-statement
description: Imports a bank statement or a credit card statement into Finux, the personal finance manager of the user, from a PDF, a CSV, an OFX file, a spreadsheet, a screenshot or pasted text. Use it for "import this statement", "load my bank extract", "add the transactions of this PDF", "import my card bill", "here is my statement". It extracts the rows, skips the transactions that Finux already has, proposes categories from the history, shows a draft, creates the transactions after the user approves and reconciles the result with the statement.
---

# Import a statement into Finux

Read a statement, turn its rows into Finux transactions without
duplicates, and prove the result with the totals of the statement.

## Before you start

- Reach Finux as [references/access.md](references/access.md) tells: MCP
  tools first, the CLI when there are none.
- The rules of money, dates, transfers and bills are in
  [references/concepts.md](references/concepts.md). The arguments of each
  tool are in [references/tools.md](references/tools.md).
- A statement has private data. Do not repeat account numbers, document
  numbers or addresses in your replies, and do not put them in a title or
  a description.

## Steps

### 1. Identify the statement

Find, from the file or from the user:

- the Finux account or credit card it belongs to (`account_list`,
  `credit_card_list`). When none matches, ask; offer to create it with
  `account_add` or `credit_card_add`;
- its currency, which must be the currency of that account or card;
- its period, and the control values it prints: opening and closing
  balance for a bank account; total of the bill, closing date and due date
  for a credit card.

### 2. Extract the rows

Get a table with one row for each movement: date, text, amount, direction.

- **CSV, OFX, spreadsheet:** parse the file with a script. Do not copy
  numbers by hand from a long file.
- **PDF:** extract the text first (the PDF reader of the agent, or
  `pdftotext -layout file.pdf -` in a shell). Keep the layout: columns
  give the direction of an amount. A PDF without text is a scan: read the
  pages as images. Read all pages; tables continue after page breaks and
  repeat their headers.
- **Screenshot or pasted text:** read it as it is, and tell the user when
  a part is cut or not clear.

Then normalize:

- Dates to `YYYY-MM-DD`. A statement often prints no year: take it from
  the period, and mind a period that goes from December to January. Find
  the order of day and month from a date with a day above 12 or from the
  language of the statement.
- Amounts to a decimal string with a point, without thousands separators:
  `1.234,56` and `1,234.56` are both `1234.56`. Find which mark is the
  decimal one from the currency and from an amount with cents.
- Direction from the sign, from a credit/debit column or from `D`/`C`
  marks. On a credit card statement a purchase is an expense, and a refund
  or a reversal is an income.

**Check the extraction before you go on.** The sum of the rows must
explain the control values: opening balance plus the rows equals the
closing balance, or the purchases less the refunds equal the total of the
bill. Calculate with a script. When the sum is wrong, find the missing or
the wrong row; do not go on with a difference you cannot explain. Report
a difference you cannot remove.

### 3. Drop the rows that are not transactions

- Balance lines, subtotals, totals and headers.
- In a credit card statement: the line of the payment of the bill before,
  and its carried balance. Finux has these from `credit_card_pay`.
- In a bank statement: the payment of a credit card that Finux has. Do
  not import it as an expense. Offer to pay the bill with
  `credit_card_pay`, as the skill finux-credit-card-bills tells.
- A movement between two accounts of the user is one transfer (`account`
  and `to`), not two transactions. When the other account also gets a
  statement, the same transfer is in the two files: create it one time.
- Interest, fees and taxes of the bank are real expenses: keep them.
- An installment line (`3/10`, `PARC 03/10`): keep the installment of
  this statement only, and put the installment number in the description.

### 4. Remove the duplicates

Call `transaction_list` with the `account` and the `date` of each month
the statement covers; read all pages. A row is already in Finux when a
transaction has the same place, the same value, the same direction and
the same date. A date one or two days apart with the same value and a
similar title is a probable duplicate: put it in a list of doubts for the
user. Two rows of the statement that are equal are two transactions:
match each one against a different Finux transaction.

### 5. Give each row a title, a category and a type

- Title: the merchant in clean words, at most 60 characters. Remove
  codes, card numbers and city names from the raw text
  (`PAG*MERCADOLIVRE 0423 SAO PAULO` becomes `Mercado Livre`). When
  `transaction_suggestions` has a title for the same merchant, use that
  exact title.
- Category: the category that the same title has in the history
  (`transaction_list` with `search`), then the best match by meaning from
  `category_list`. For a row you cannot place, use `OTHER`.
- Type: expense or income from the direction. A grey category needs it.
- Tags: only the tags the same title had before.

### 6. Show the draft and wait

Show a summary first: rows in the statement, rows dropped and why, rows
already in Finux, rows to create, and the totals in and out of the rows to
create. Then the table of the rows to create: date, title, value, type,
category. Then the doubts: probable duplicates, rows in `OTHER`, text you
could not read. For more than 40 rows, show the table in groups by
category or offer the full table as a file.

Wait for the user to approve or to correct. Create nothing before that.

### 7. Create

Call `transaction_add` for each approved row, the oldest date first. Stop
at the first error, report how many rows are done, and ask how to
continue.

For a credit card, a paid bill refuses new transactions
(`bill MM/YYYY of CODE is paid`). Tell the user: the payment must be
deleted and made again after the import, and that is a decision of the
user.

### 8. Reconcile and report

- **Bank account:** `account_show`. The balance must be the closing
  balance of the statement, when Finux has all the months before. A
  difference that existed before the import is a wrong opening balance or
  missing older months: report it, do not correct it with an invented
  transaction.
- **Credit card:** `credit_card_bills`. The total of the bill must be the
  total of the statement. Finux puts a purchase in a bill by its date and
  the closing day of the card; a bank can put a purchase near the closing
  date in the other bill. When the totals differ, compare the rows around
  the closing date, and check `closing_day` and `due_day` of the card
  against the statement.

Report: how many transactions you created, how many you skipped as
duplicates, how many you dropped, the result of the reconciliation, and
the doubts that remain.
