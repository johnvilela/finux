---
name: finux-log-transactions
description: Records expenses, incomes and transfers in Finux, the personal finance manager of the user, from loose text, a chat message, a receipt or a photo. Use it for "I spent 45 on lunch", "log this expense", "add a transaction", "I got my salary", "I moved 500 to savings", "record this receipt", or a correction of a transaction that was just recorded. It picks the account, the category and the tags, looks for duplicates, shows a draft and creates the transactions only after the user approves. For a full bank or card statement use finux-import-statement.
---

# Log transactions in Finux

Turn what the user tells you into correct Finux transactions. The user
approves a draft before you write.

## Before you start

- Reach Finux as [references/access.md](references/access.md) tells: MCP
  tools first, the CLI when there are none.
- The rules of money, dates, types and bills are in
  [references/concepts.md](references/concepts.md). The arguments of each
  tool are in [references/tools.md](references/tools.md).
- The names and the tags that the user gave before are in ai-memory.
  [references/names.md](references/names.md) tells how to read them, how
  to ask about a name you do not know, and how to save the answer.
  Call `transaction_corrections` with the titles of the rows too: it has
  the changes the user made by hand.

## Steps

### 1. Read the context, one time for each conversation

Call `account_list`, `credit_card_list`, `category_list`, `goal_list`,
`schedule_list` and `transaction_suggestions`. Keep the results: they give the codes, the
currencies, the goals, and the titles and tags the user already uses.
Read the rules of names too, as names.md tells. For a raw line of a bank,
call `transaction_corrections` with it.

### 2. Make a draft for each transaction

| Field | How to find it |
| --- | --- |
| `title` | Short name of the merchant or the item, at most 60 characters. See below |
| `value` | The amount, positive, as a decimal string in the currency of the account |
| `date` | What the user says ("yesterday", "on the 5th"). Default: today. Send `YYYY-MM-DD` |
| `account` | The account or the credit card that paid. See below |
| `category` | The category that fits. See below |
| `type` | Only for a grey category, or to make the direction clear |
| `tags` | Tags the user gives, the tags of the rule, or tags the same title had before. See below |
| `description` | Details that do not fit the title. Usually empty |
| `goal` | Only when the user says the money is for a goal. See below |
| `virtual` | `true` only for a purchase on a credit card that the user says was made with a virtual card |
| `installments` | Only for a purchase or an income in monthly parts. See below |

**Title.** The title must be a name the user can read later. When the
text is a raw line of a bank (a notification, a line the user pasted,
`PAY LAMEI 21/11`), look for a rule that matches and use its title, its
category and its tags. Without a rule, use the exact title that
`transaction_suggestions` has for the same thing. When you cannot tell
what the line is, do not invent a title: ask, and suggest a name. When
the user answers, save the rule, as names.md tells.

**Tags.** Suggest tags for each draft: the tags of the rule, then the
tags the same title had before, then one or two tags the user already
has that fit. Mark a tag that is only a suggestion; write it only after
a yes. When the user gives the same tags to a name two times, or says
"always", save them in the rule.

**Account or credit card.** Use what the user names ("on the Visa", "from
Nubank", "cash"). Without a name, look at the last transactions with the
same title (`transaction_list` with `search`): when all of them use one
place, propose it. In all other cases, ask. Do not guess between two
plausible accounts: a wrong account makes two balances wrong.

**Category.** Match the meaning of the purchase to the name and the
description of a category. Use the category that the same title had
before. When no category fits, propose `OTHER` and tell the user; do not
create a category unless the user asks.

**Goal.** Link a transaction to a goal only when the user says so ("for
the trip", "this goes to the car fund", "toward my loan") or when the
title had that goal each time before. Do not link the usual expenses of
a month: in a `save` goal an expense removes progress. The goal must
have the currency of the account. Money put aside is usually a transfer
to a savings account; link the transfer. When a goal does not exist yet,
offer to create it with `goal_add`; do not create it without a yes.

**Installments.** Words such as "in 10x", "10 times", "parcelado", "em 10
vezes", "12 installments" or "3 of 10" mean a transaction in installments.
Finux creates one transaction for each monthly part, with one call.

- Send `installments` (the number of parts) and `value`, the **total** of
  the purchase. "A notebook of 1200 in 10x" is `value: "1200"`,
  `installments: 10`.
- When the user gives the amount of one part ("10x of 120"), send
  `installment_value: "120"` in place of `value`. Do not send the two.
  When you cannot tell if an amount is the total or one part, ask.
- `date` is the date of the first part: the date of the purchase.
- A purchase that is already in progress ("I am at part 4 of 10"): send
  `first_installment: 4` and the date of that part. Finux creates the parts
  4 to 10. Prefer `installment_value` here: the user usually knows the
  amount of one part. Do not create the parts that are in the past: they
  can be in a bill that is paid.
- Interest: when the total with interest is known, use it as `value`.
- Do not create the parts one by one with many calls: they would not be
  one purchase.

**Scheduled transactions.** Before you draft an expense or an income,
look at `schedule_list`. A schedule is a bill or an income that the user
planned: rent, internet, a dentist next month.

- The user reports something that a schedule has ("I paid the rent", "the
  electric bill was 187", "salary came in"): do not use `transaction_add`.
  Use `schedule_pay` with the code of the schedule. Send `value` when the
  real amount is different, `date` when the user paid on another day, and
  `account` when another account paid. A `transaction_add` here would
  count the bill two times and leave the occurrence due.
- A schedule without an account (no `account` and no `credit_card` in
  the result) needs `account` in `schedule_pay`: the account or the
  credit card that paid. Ask when the user did not tell it.
- For a new schedule, the account is optional. When the user did not
  tell which account will pay a future bill, create the schedule without
  `account`. Do not ask for a default account.
- Match by meaning, not only by title, and use the category and the
  account to confirm. When two schedules can match, or the schedule has
  more than one date in `due`, ask which one. `schedule_pay` takes the
  oldest occurrence first.
- The user will not pay an occurrence, or the bill did not come:
  `schedule_skip`.
- The user wants Finux to remember a future or a repeating transaction
  ("remind me", "every month I pay", "schedule", "on the 10th of next
  month"): use `schedule_add`. Send `every` for one that repeats (`month`,
  `2 weeks`, `year`) and nothing for one time. `date` is the first date.
  Send `times` or `end_date` only when the user gives an end. It creates
  no transaction: tell the user that it waits for a confirmation.
- Do not send `automatic` unless the user asks, in clear words, that
  Finux creates the transactions without a confirmation.
- A transaction for a future date that the user did not make yet is a
  schedule, not a transaction.

**Special cases:**

- Money between two accounts of the user is one transfer: `account` is the
  source and `to` is the destination. Do not make an expense and an
  income. For different currencies also ask for the amount that arrived
  and send it as `received`.
- A refund on a credit card is an `income` on the card.
- The payment of a credit card bill is not a transaction you add here. Use
  `credit_card_pay`; the skill finux-credit-card-bills has the steps.
- A receipt or a photo: read the merchant, the total and the date. Record
  one transaction for the total, not one for each item, unless the user
  asks for a division.

### 3. Look for duplicates

For each draft, call `transaction_list` with the `date` and the `account`.
A transaction with the same date, the same value and the same place is a
probable duplicate. Mark it in the draft and do not create it unless the
user tells you to.

### 4. Show the draft and wait

Show one row for each transaction: date, title, value with its currency,
type, account or credit card, category, tags. Mark a row that pays a
schedule ("pays RENT") and a row that creates a schedule ("new schedule,
each month from 05/12"). For installments show one
row with the total, the number of parts, the amount of one part and the
dates of the first and the last part. Add the notes: duplicates,
guesses, questions. Ask the user to approve or to correct.

Create nothing before the user approves. One clear exception: the user
said to record it without a question ("just log it"), all fields are
certain, and there is no duplicate.

### 5. Create and report

Call `transaction_add` for each approved draft, the oldest date first, or
`schedule_pay`, `schedule_skip` or `schedule_add` for a row of a schedule.
Stop at the first error and report it. After `schedule_pay`, also report
`next` of the schedule, and the dates that are still in `due`.

Then report in a few lines: the id of each transaction, and what changed:
the new balance of the account (`account_show`), or for a credit card the
bill it went to (`bill` in the result) and the new total of that bill.
For installments, the result has all the parts in `transactions`: report
the ids of the first and the last part, the amount of each part, the bill
of the first part and the month of the last one. On a credit card, tell
the user that the limit in use went up by the total. For a transaction
with a goal, also report the new progress of the goal (`goal_show`). When the result has `budget_warnings`, report the
`message` of each one: the transaction left a budget near or over its
limit. Do not link a transaction to a budget: it counts by its category.

## Corrections

To correct a transaction, use `transaction_edit` with only the fields that
change. To remove one, call `transaction_delete`, show its message to the
user, and call it again with `confirm: true` only after a yes.

A transaction with `installment_id` is one part of a purchase. Choose the
`scope` from what the user means:

| The user means | `scope` |
| --- | --- |
| One part is wrong ("the March part was 125") | `one` (default) |
| The whole purchase is wrong (title, category, card, total) | `all` |
| A change from now on, or the rest is cancelled or paid early | `next`, with the id of the first part to change |

With `next` or `all`, `value` is the new total of those parts and
`installment_value` the new amount of each one. The number of parts does
not change: to change it, delete the parts with `all` and create the
purchase again. When the scope is not clear, ask before you change more
than one part.

## Errors you can expect

- `bill MM/YYYY of CODE is paid: delete its payment first`: the date puts
  the purchase in a bill that is already paid. Tell the user. Do not delete
  the payment unless the user asks.
- `installment 2/10 (transaction 13): bill MM/YYYY of CODE is paid`: with
  the scope `all` or `next`, one part is in a paid bill, so nothing
  changed. Use `next` with the id of the first part in a bill that is not
  paid, or tell the user.
- `schedule CODE has no occurrence left`: the schedule is done. Record the
  transaction with `transaction_add`, and ask if the schedule must
  continue (`schedule_edit` with a new `date`).
- `schedule not found: CODE (it is archived; restore it first)`: ask
  before `schedule_restore`.
- `transaction N is not an installment`: `scope` is only for a part of a
  purchase in installments. Call again without it.
- `type is necessary: the category X is for all types`: send `type`.
- `goal CODE is in BRL, not in USD`: the goal takes transactions of its
  currency only. Record the transaction without the goal and tell the
  user.
- `goal CODE is archived`: the goal takes no new transaction. Ask the
  user before `goal_restore`.
- `account not found`: the account can be archived. `account_list` with
  `include_archived` shows it.
