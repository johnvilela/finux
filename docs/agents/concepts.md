# Finux concepts

Finux is a personal finance manager for one user. It has accounts,
categories, transactions, credit cards with bills, and goals. This file gives the
rules an agent needs to read and change that data correctly.

## Codes and references

An account, a category and a credit card each have a short code: 1 to 5
letters and digits, in capitals, for example `NU`, `FOOD`, `VISA`. An
argument that names one of them accepts the code or the numeric id. Prefer
the code.

Accounts and credit cards share one set of codes: a code is an account or a
credit card, not the two. Categories have their own set, and goals have
their own set.

## Money

- Each account and each credit card has one currency: `USD`, `EUR`, `BRL`
  or `BTC`. The currency cannot change.
- Send money as a decimal string in that currency: `"1234.56"`. A comma as
  the decimal mark also works: `"1234,56"`. Do not send a thousands
  separator, a currency symbol or a number in place of the string.
- A result gives money as an object:
  `{"minor": 123456, "decimal": "1234.56", "formatted": "R$ 1.234,56"}`.
  `minor` is an integer in the smallest unit of the currency (cents; BTC
  has 8 decimal places). Calculate with `minor`. Show `formatted` to the
  user.
- Never add or compare amounts of different currencies. Finux has no
  exchange rates.

## Dates

- Send a date as `YYYY-MM-DD` or `DD/MM/YYYY`. `10/11/2026` is 10 November.
- A result gives a date as `YYYY-MM-DD`.
- A date filter also accepts a year (`2026`) or a month (`2026-11`).
- Without a date, a new transaction has the date of today.

## Accounts

An account holds money: a bank account, a wallet, an investment. Its
balance is its opening balance plus the amounts of its transactions that
have a date up to today. A transaction with a future date changes the
balance when its day comes. The balance can be negative.

An archived account leaves the lists and takes no new transactions, but it
keeps its transactions; it can be restored. A permanent delete also deletes
its transactions and cannot be undone.

### Accounts and credit cards of other persons

An account or a credit card can have an `owner`: the name of the person it
belongs to, for example the partner of the user. An empty `owner` means it
belongs to the user. The user records these to follow the money of that
person, not to mix it with their own.

- `account_list`, `credit_card_list`, `transaction_list` and its `totals`
  leave out the accounts, the credit cards and the transactions of other
  persons. Send `include_others: true` to get them. Do this only when the
  user asks for all persons or for that person.
- `transaction_list` with `account` returns the transactions of that
  account or credit card in any case.
- A tool that takes one account or one credit card (`account_show`,
  `credit_card_show`, `credit_card_bills`, `credit_card_pay`,
  `transaction_add`) works for any owner.
- A budget and a goal never count a transaction in an account or a credit
  card of another person.
- A transfer from an account of the user to an account of another person
  lowers the balance of the user, and it is not an expense.
- When you report a sum of balances or of bills, do not add the ones of
  other persons unless the user asks. When you add them, say so and give
  the name of the person.

## Categories

Each transaction has one category. The color of a category gives the types
of transaction it accepts; a result shows this in `accepts`:

| Color | `accepts` | Types |
| --- | --- | --- |
| red | `expense` | expense |
| green | `income` | income |
| grey | `all` | expense, income, transfer, payment |

Two categories have a function: `XFER` is the default category of a
transfer and the category of a payment; `OTHER` receives the transactions
of a deleted category. Do not delete them or change their color.

## Transactions

A transaction has a title (at most 60 characters), an optional description
(at most 200), a type, an amount, a date, a category, tags, and one place:
an account or a credit card, never the two.

| Type | Amount | Meaning |
| --- | --- | --- |
| `expense` | negative | Money that leaves, or a purchase on a credit card |
| `income` | positive | Money that comes in, or a refund on a credit card |
| `transfer` | one of each | Money moved between two accounts of the user |
| `payment` | negative | The payment of a credit card bill, from an account |

- You send `value` as a positive amount. The type gives the sign.
- Without a type, a red category gives `expense` and a green one `income`.
  A grey category needs the type.
- A transfer is two transactions, one in each account, joined by
  `pair_id`. Create it with one call: `account` is the source and `to` is
  the destination. A change or a delete of one side also changes or deletes
  the other. When the two accounts have different currencies, also send
  `received`: the amount the destination gets.
- Transfers and payments are not income and not expense. They are not in
  the totals: the money stays with the user.
- Tags are lower-case words of letters, digits and `_`, shown with `#`:
  `#market`. Send them divided by spaces, with or without `#`. In an edit,
  the tags you send replace the tags of the transaction.

## Corrections

Finux learns from each change of a transaction. When the title, the
category, the tags or the description of a transaction changes, in the
CLI, in Telegram or through `transaction_edit`, Finux keeps a correction:
"a transaction with this title must have this title, this category, these
tags, this description". `transaction_corrections` returns them.

```json
{"pattern": "PAY LAMEI", "title": "Lamen House", "category": "FOOD",
 "tags": ["#lunch"], "source": "user", "count": 3, "updated": "2026-11-24"}
```

- `pattern` is the title the transaction had, in capitals, without
  accents, without dates and without the words that have a digit:
  `PAY LAMEI 21/11` and `PAY LAMEI 23/11` have the pattern `PAY LAMEI`.
  There is one correction for each pattern, and a new change replaces
  the values of the old one.
- A field that is absent was not taught: use what you know for it. Empty
  `tags` or an empty `description` mean that the user wants none.
- `source` tells who made the last change. `user` is a change by hand:
  the user did not want what the transaction had. `agent` is a change
  through the tools.
- A change follows the names: when `PAY LAMEI` became `Lamen` and the
  user then changes `Lamen` to `Lamen House`, the corrections of
  `PAY LAMEI` and of `LAMEN` give `Lamen House`.
- A new transaction teaches nothing: a title that an agent gave is not a
  title that the user chose. Transfers and payments teach nothing.
- Send `titles` to get only the corrections of the transactions at hand;
  a title can be a raw line of a statement. Do not read all the
  corrections for an import.

## Installments

An expense or an income can be in installments: N monthly parts. Finux
keeps one transaction for each part. Transfers and payments have no
installments.

- Create all the parts with one call of `transaction_add`: give
  `installments` (2 to 120) and `value`, the total of all the parts. Finux
  divides the total; the cents that remain go to the first part. When you
  know the amount of one part and not the total, give `installment_value`
  in place of `value`. Do not give the two.
- `date` is the date of the first part. Each part after it is one month
  later, on the same day (a short month uses its last day).
- For a purchase that is already in progress, give `first_installment`:
  with `installments: 10` and `first_installment: 4`, Finux creates the
  parts 4 to 10, and `date` is the date of part 4. `value` is still the
  total of the 10 parts.
- Each part has `installment` (its number), `installments` (the number of
  parts) and `installment_id` (the same in all the parts of one purchase).
  To read all the parts, filter `transaction_list` by the title and keep
  the transactions with that `installment_id`.
- On a credit card, each part goes to the bill after the one before: the
  first part to the bill of its date, the second to the next bill. All the
  parts count in `used` from the first day, so the limit goes down by the
  total.
- On an account, a part changes the balance on its date.
- Each part counts by itself in the totals and in the budgets, in the
  month of its date. So a purchase of 1200 in 10 parts is an expense of 120
  in each of 10 months, not 1200 in the first month.
- A goal counts all the parts of a linked transaction from the first day.

### Change and delete the parts

`transaction_edit` and `transaction_delete` have the argument `scope`:

| Scope | Parts |
| --- | --- |
| `one` (default) | Only the transaction with the id |
| `next` | That part and the parts with a higher number |
| `all` | All the parts |

- With `next` or `all`, a title, a description, a type, a category, tags,
  a goal, an account and `virtual` go to each part of the scope.
- With `next` or `all`, `value` is the new total of the parts of the
  scope, divided again, and `installment_value` is the new amount of each
  one. `date` is the new date of the first part of the scope; the others
  follow by month.
- The number of parts does not change. When you delete a part, the others
  keep their numbers: `3/10` stays `3/10`.
- A part in a paid bill is locked. With `next` or `all`, one locked part
  stops the change of all the parts, and nothing changes. Use `next` from
  the first part in a bill that is not paid, or delete the payment first.
- The user paid the rest of a purchase early: delete the parts that will
  not come with the scope `next`, then record what the user paid as one
  new expense.

## Credit cards and bills

A credit card has a limit, a closing day and a due day (days of the month,
1 to 31; a short month uses its last day). It has no balance. Its
transactions go to bills.

- A purchase is an `expense` with the code of the card as its `account`. A
  refund is an `income`. A credit card cannot have a transfer.
- A purchase does not change the balance of an account. It counts as an
  expense on the date of the purchase.
- Finux puts each transaction in a bill by its date. A transaction on or
  before the closing day goes to the bill that closes in that month; a
  later one goes to the next bill. Finux creates a bill when the first
  transaction needs it. The parts of a purchase in installments are the
  exception: each one goes to the bill after the one before. See
  [Installments](#installments).
- The name of a bill is the month of its due date, as `YYYY-MM`. Example
  for a card that closes on day 25 and is due on day 5: a purchase on
  2026-11-10 goes to the bill `2026-12` (closes 2026-11-25, due
  2026-12-05); a purchase on 2026-11-26 goes to the bill `2027-01`.
- New closing and due days apply only to the bills that do not exist yet.
- `used` is the total of the bills that are not paid, also the future
  bills that hold the parts of a purchase in installments. `available` is
  the limit less `used`.

A bill has one status:

| Status | Meaning |
| --- | --- |
| `open` | Today is on or before the closing date. It still gets transactions. It cannot be paid yet |
| `closed` | The closing date passed and the due date did not. It waits for a payment |
| `late` | The due date passed and the bill is not paid |
| `paid` | It has a payment. It is locked |

The total of a bill is `carried` (what the bill before left) plus `spent`
(its transactions). A bill with a total of zero or less is never late.

### Payments

- `credit_card_pay` pays one bill from an account that has the currency of
  the card. It creates one transaction of the type `payment` in that
  account and marks the bill as paid.
- Without a value, it pays the total. With a smaller value, the bill is
  still paid, and the rest goes to the next bill that is not paid, as its
  `carried` value. Finux calculates no interest: when the bank charges
  interest, that is a new expense on the card.
- A paid bill is locked: Finux refuses a new transaction in it, and a
  change or a delete of one of its transactions. To change them, delete the
  payment first (`credit_card_unpay`, or `transaction_delete` of the
  payment). The bill opens again and the carried value comes back.
- A payment cannot be edited: delete it and pay again.
- A permanent delete of a credit card deletes its bills and its purchases.
  Each payment stays in its account as an expense of the category `OTHER`.

## Goals

A goal is an amount of money the user wants to reach: a title, a type, a
target, one currency and an optional final date. Its progress comes only
from the transactions linked to it.

- Link a transaction with the argument `goal` of `transaction_add` or
  `transaction_edit`. In an edit, an empty `goal` takes the link away. A
  transaction has at most one goal.
- A goal takes only transactions in its currency: the currency of the
  account or the credit card of the transaction.
- The payment of a credit card bill cannot have a goal. To follow a debt
  on a card, link the purchases or other expenses.
- The type tells how a linked transaction counts:

| Type | An income | An expense | A transfer |
| --- | --- | --- | --- |
| `save` | adds | removes | adds one time |
| `purchase` | adds | removes | adds one time |
| `payoff` | removes | adds | adds one time |

- `purchase` counts as `save`; it only tells that the money is for one
  thing. Money put aside is often a transfer to a savings account: link
  that transfer.
- `progress` is never below zero. `remaining` is the target less the
  progress. `percent` is a whole number from 0 to 100.
- Link a transaction only when the user wants it to count. Do not link
  the usual expenses of a month to a `save` goal: each one removes
  progress.

A goal has one status:

| Status | Meaning |
| --- | --- |
| `active` | Not reached, and no final date or a final date that did not pass |
| `reached` | The progress is the target or more |
| `overdue` | The final date passed and the goal is not reached |

An archived goal leaves the lists and takes no new transactions; its
transactions keep the link, and it can be restored. `goal_delete` deletes
the goal only: no transaction is deleted, the linked ones lose the link.

## Budgets

A budget is a limit of money for some categories in a period: a title, a
limit, one currency, a list of categories and a period. A transaction has
no link to a budget. Each expense and income counts by itself when it is in
a category of the budget, in the currency of the budget, on a date in a
period.

- An expense adds to `spent`. An income gives money back: it removes from
  `spent`. `spent` is never below zero.
- A transfer and the payment of a credit card bill do not count. A purchase
  with a credit card counts on the day of the purchase, not on the day the
  bill is paid.
- `categories` is a list of codes. An empty list means all categories but
  the income (green) ones, so a salary does not pay for the expenses.
- A budget takes only transactions in its currency: the currency of the
  account or the credit card of the transaction.
- A budget is recurring (`every`: `month`, `2 weeks`, `15 days`, ...) or
  for one time (no `every`, and an `end_date`). A recurring budget can also
  have an `end_date`: its last period stops there.
- The periods count from `start_date`. A monthly budget that starts on
  2026-11-05 has the periods 2026-11-05 to 2026-12-04, 2026-12-05 to
  2027-01-04 and so on. A month too short for the day uses its last day.
- With `rollover`, a period gets what the period before did not spend and
  loses what it spent too much: `carried`, which can be below zero.
  `available` is the limit and `carried`. Without rollover, `carried` is
  zero and each period has the limit.
- A change of the limit, the categories or the dates of a budget counts
  all its periods again. Finux does not keep the old limit of a past
  period.

A period has one level, and a budget has the level of its current period
as its status:

| Status | Meaning |
| --- | --- |
| `ok` | The period spent less than 80% of `available` |
| `near` | The period spent 80% of `available` or more |
| `over` | The period spent more than `available` |
| `upcoming` | The start date is in the future; the budget has no period yet |
| `ended` | The end date passed; `current` is the last period |

`transaction_add` and `transaction_edit` return `budget_warnings` when the
transaction leaves an active budget `near` or `over`: a list with `budget`
(code), `level` and `message`. Tell the user about each one. The result
has no `budget_warnings` when no budget needs attention.

An archived budget leaves the lists and gives no warnings; it keeps its
history and can be restored. `budget_delete` deletes the budget only: no
transaction changes. A deleted category leaves the budgets that have it;
Finux refuses to delete a category that is the only category of a budget.

## Scheduled transactions

A schedule is a transaction that will happen: a bill to pay or an income
to receive. It is for one time (a dentist next month) or it repeats (rent
each month, `every`: `month`, `2 weeks`, `15 days`, `year`). A schedule is
an expense or an income, in an account or on a credit card. It has no
transfers.

A schedule can also have **no account**: the user did not decide yet
which account or card will pay the bill. Leave `account` out of
`schedule_add`; do not ask the user to choose one, and do not guess. The
schedule then has only `currency`: the currency of the accounts of the
user when they have only one, or the `currency` you send. `schedule_pay`
then needs `account`, an account or a credit card, and the schedule stays
without one for its next occurrences. Such a schedule cannot be
`automatic`. `schedule_edit` gives it an account, and `account: "none"`
takes the account away again.

- A schedule is **not** a transaction. It changes no balance, no bill and
  no total. `transaction_list` does not return it.
- Each date of a schedule is an occurrence. An occurrence waits until
  `schedule_pay` creates its transaction, or `schedule_skip` drops it.
  The occurrences go in order: the oldest one first.
- An occurrence with a date up to today is due. It stays due until the
  user pays or skips it, so one schedule can have many due occurrences.
  `due` in a schedule lists their dates and `next` is the first date
  after today.
- `schedule_pay` uses the title, the category, the tags and the goal of
  the schedule. Its value, its date and its account are the ones of the
  schedule unless you give the real ones. `account` can be a credit card:
  the transaction then goes to a bill of the card. Without `date`, a due occurrence
  gets its own date and an occurrence that is not due yet gets the date
  of today. The transaction has `schedule` with the code of the schedule.
- The status of a schedule is `due`, `upcoming` or `done` (no occurrence
  is left). A schedule ends after `times` occurrences or after
  `end_date`; without them it repeats until it is archived.
- `automatic` is false unless the user asked for it. An automatic schedule
  creates the transaction of each due occurrence by itself, with the date
  of the occurrence, the next time Finux runs. Never set it without a
  clear request of the user: money must not move in Finux without a
  confirmation. It also needs the setting `automatic-schedules`, which
  only the user turns on (`finux config set automatic-schedules on`; no
  tool changes it). While the setting is off, an automatic schedule waits
  as the others: its occurrences are in `due` and `schedule_pay` confirms
  them.
- A change with `schedule_edit` is for the occurrences that wait. A new
  `date` is the date of the next occurrence, and the ones after it count
  from that date.
- A budget counts a schedule only after `schedule_pay`. `budget_show`
  returns `scheduled`: what the schedules of the categories of the budget
  still take from the current period.
- The delete of a transaction that a schedule created does not bring the
  occurrence back.

When the user says that a bill of a schedule is paid, use `schedule_pay`,
not `transaction_add`: a second transaction would count the bill two times
and the occurrence would stay due.

## Notes

A note is a short text about the finances of the user: a fact to
remember, a decision, or a thing to do or to check. The user writes notes
in the CLI and in Telegram, and an agent writes them with `note_add`.

```json
{"id": 12, "title": "Ask the bank about the fee", "text": "R$ 12 in November.",
 "status": "due", "remind_on": "2026-11-17", "tags": ["#bank"],
 "account": "NU", "source": "user", "created": "2026-11-10"}
```

- A note has a `title` (60 characters at most) and an optional `text`
  (500 characters at most). It has an id and no code.
- All the rest is optional: `remind_on`, `tags`, and the records the note
  is about: `goal`, `budget`, and `account` or `credit_card`.
- `status` is `open`, `due` or `done`. A note is `due` when it is open
  and its reminder date is today or before. It stays due until
  `note_done`, or until `note_edit` gives it a later date.
- Finux shows the due notes in `finux resume` and `finux tx`, and the
  Telegram bot sends one message for each reminder date.
- `source` tells who wrote the note: `user` or `agent`. A note that a
  tool creates has `agent`.
- The results of `account_show`, `credit_card_show`, `goal_show` and
  `budget_show` have `notes`: the open notes of the record.
- A note changes no balance, no bill and no total. It is not a schedule:
  a bill that will happen is `schedule_add`.
- A record that is deleted permanently leaves its notes; they lose the
  link. `note_delete` deletes one note and needs `confirm: true`.

## Deletes need confirmation

`account_delete`, `category_delete`, `transaction_delete`, `goal_delete`,
`budget_delete`, `credit_card_delete` and `credit_card_unpay` do nothing without
`confirm: true`. The first call returns
`{"confirm_required": true, "message": "..."}`, where the message tells
what the tool will delete. Show that message to the user and wait for a
clear yes before the call with `confirm: true`. Prefer `account_archive`,
`credit_card_archive`, `goal_archive` and `budget_archive` when the user
only wants an item out of the way.

## Objects in a result

Money fields are money objects, as in [Money](#money).

**Account:** `id`, `code`, `name`, `description`, `color`, `currency`,
`archived`, `owner` (empty for an account of the user), `opening_balance`,
`balance`.

**Category:** `id`, `code`, `name`, `description`, `color`, `accepts`.

**Transaction:** `id`, `title`, `description`, `type`, `amount` (signed),
`currency`, `date`, `status` (`pending` for a date after today: the
transaction is not in the balance yet; `cleared` for the others), `tags`
(list, each with `#`), `category` (code), and its place:

- in an account: `account` (code);
- a purchase on a credit card: `credit_card` (code) and `bill` (`YYYY-MM`),
  and no `account`;
- a payment: `account`, and the `credit_card` and `bill` it paid;
- a transfer: `account`, `pair_id` and `other_account`.

A purchase made with a virtual card of a credit card also has `virtual`
(`true`); the other transactions do not have the field. A transaction
linked to a goal also has `goal` (code). A part of a
transaction in installments also has `installment`, `installments` and
`installment_id`. A transaction that a schedule created also has
`schedule` (code).

`transaction_add` and `transaction_edit` return `transaction`, and
`budget_warnings` when a budget is near or over its limit. For a
transaction in installments they also return `transactions`: all the parts
that the call created or changed.

**Credit card:** `id`, `code`, `name`, `description`, `color`, `currency`,
`archived`, `owner` (empty for a credit card of the user), `limit`, `used`,
`available`, `closing_day`, `due_day`.

**Bill:** `period` (`YYYY-MM`), `closes_on`, `due_on`, `status`, `carried`,
`spent`, `total`. A paid bill also has `paid_on`, `paid`, `remaining` (the
value that went to the next bill) and `payment_id`. The open bill of
`credit_card_show` also has `transactions`, and `virtual_total` when it has
purchases made with a virtual card: the sum of these purchases, positive
as `spent` is.

**Goal:** `id`, `code`, `title`, `description`, `type`, `currency`,
`archived`, `target`, `progress`, `remaining`, `percent`, `status`,
`transactions` (the number of linked transactions; a transfer is one), and
`due_date` when the goal has a final date. `goal_show` also returns the
linked transactions, each with `effect`: what it adds to the goal, below
zero when it removes.

**Budget:** `id`, `code`, `title`, `description`, `currency`, `archived`,
`limit`, `categories` (list of codes; empty for all), `period` (in words:
`every 2 weeks` or `one time`), `every` and `unit` (only in a recurring
budget), `start_date`, `end_date` (when the budget has one), `rollover`,
`status`, and `current`: the period of today, or the last period of a
budget that ended. A budget that did not start has no `current`.

**Schedule:** `id`, `code`, `title`, `description`, `type`, `value`,
`currency`, `account` or `credit_card`, `category`, `tags`, `goal` (when
it has one), `repeats` (in words: `every month` or `one time`),
`end_date` (when it has one), `times_left` (not in a schedule without an
end), `automatic`, `archived`, `status`, `due` (list of dates) and `next`
(when an occurrence comes after today). `schedule_list` also returns
`due`: the number of occurrences that wait in all the schedules.

**Period of a budget:** `from`, `to`, `carried`, `available`, `spent`,
`remaining`, `over` (what it spent above `available`), `percent` (can be
more than 100) and `level`. `budget_show` returns `period` (the period it
shows), `transactions` (the ones that count in it, each with `counts`: what
it adds to `spent`, below zero for an income) and `history` (the periods
before it, the newest first).

**List of transactions** (`transaction_list`): `total` (all that match the
filters), `offset`, `count` (in this page), `transactions`, and `totals`:
one entry for each currency with `income`, `expense` and `net`, for all
transactions that match, not only this page. The default page has 100
transactions; read the next pages with `offset` until you have `total`.

## Errors

A tool that fails returns a text that tells what is wrong, for example
`account not found: NUU` or `bill 11/2026 of VISA is paid: delete its
payment first`. Read it, correct the call or tell the user. Do not try
the same call again without a change. In an error text, a bill shows as
`MM/YYYY`.
