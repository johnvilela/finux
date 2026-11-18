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

## Steps

### 1. Read the context, one time for each conversation

Call `account_list`, `credit_card_list`, `category_list` and
`transaction_suggestions`. Keep the results: they give the codes, the
currencies, and the titles and tags the user already uses.

### 2. Make a draft for each transaction

| Field | How to find it |
| --- | --- |
| `title` | Short name of the merchant or the item, at most 60 characters. When `transaction_suggestions` has a title for the same thing, use that exact title |
| `value` | The amount, positive, as a decimal string in the currency of the account |
| `date` | What the user says ("yesterday", "on the 5th"). Default: today. Send `YYYY-MM-DD` |
| `account` | The account or the credit card that paid. See below |
| `category` | The category that fits. See below |
| `type` | Only for a grey category, or to make the direction clear |
| `tags` | Only tags the user gives, or tags the same title had before |
| `description` | Details that do not fit the title. Usually empty |

**Account or credit card.** Use what the user names ("on the Visa", "from
Nubank", "cash"). Without a name, look at the last transactions with the
same title (`transaction_list` with `search`): when all of them use one
place, propose it. In all other cases, ask. Do not guess between two
plausible accounts: a wrong account makes two balances wrong.

**Category.** Match the meaning of the purchase to the name and the
description of a category. Use the category that the same title had
before. When no category fits, propose `OTHER` and tell the user; do not
create a category unless the user asks.

**Special cases:**

- Money between two accounts of the user is one transfer: `account` is the
  source and `to` is the destination. Do not make an expense and an
  income. For different currencies also ask for the amount that arrived
  and send it as `received`.
- A refund on a credit card is an `income` on the card.
- A purchase in installments: Finux has no installments. Record the
  installment of this month only, with a description such as
  `Installment 2 of 10`, and tell the user.
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
type, account or credit card, category, tags. Add the notes: duplicates,
guesses, questions. Ask the user to approve or to correct.

Create nothing before the user approves. One clear exception: the user
said to record it without a question ("just log it"), all fields are
certain, and there is no duplicate.

### 5. Create and report

Call `transaction_add` for each approved draft, the oldest date first. Stop
at the first error and report it.

Then report in a few lines: the id of each transaction, and what changed:
the new balance of the account (`account_show`), or for a credit card the
bill it went to (`bill` in the result) and the new total of that bill.

## Corrections

To correct a transaction, use `transaction_edit` with only the fields that
change. To remove one, call `transaction_delete`, show its message to the
user, and call it again with `confirm: true` only after a yes.

## Errors you can expect

- `bill MM/YYYY of CODE is paid: delete its payment first`: the date puts
  the purchase in a bill that is already paid. Tell the user. Do not delete
  the payment unless the user asks.
- `type is necessary: the category X is for all types`: send `type`.
- `account not found`: the account can be archived. `account_list` with
  `include_archived` shows it.
