---
name: finux-tidy-transactions
description: Finds the transactions of one account or one credit card in Finux, the personal finance manager of the user, that are unknown or not sure, so the user can look at each one and give it a description, a better category and tags. Use it for "tidy my Nubank transactions", "review the transactions of my Visa", "which transactions are not categorized", "clean up last month", "what is this charge", "fix the categories of my card". It takes an account or a credit card and a period (default the last 30 days), shows a numbered list with the reason and a suggestion of name, category and tags for each row, changes a transaction only with what the user gives or approves, and remembers the names and the tags for the next time.
---

# Tidy the transactions of an account in Finux

Find the transactions that the user must look at, and write what the user
decides. The user decides each row: you suggest, you do not guess and
write.

## Before you start

- Reach Finux as [references/access.md](references/access.md) tells: MCP
  tools first, the CLI when there are none.
- The rules of money, dates, types and bills are in
  [references/concepts.md](references/concepts.md). The arguments of each
  tool are in [references/tools.md](references/tools.md).

- The names and the tags that the user gave before are in ai-memory.
  [references/names.md](references/names.md) tells how to read them, how
  to ask about a title you do not know, and how to save the answer.

## Inputs

| Input | Rule |
| --- | --- |
| Target | One account or one credit card: a code or a name. Necessary. When the user gives none, show the codes of `account_list` and `credit_card_list` and ask. Do not do all the accounts in one run |
| Period | Default: the last 30 days, from 30 days before today to today. The user can give a month ("November"), a range, or a number of days |

Tell the user the target and the two dates of the period before the list.

## Steps

### 1. Read the context

Call `category_list` and `transaction_suggestions`: they give the
categories, and the titles and tags the user already uses. Find the
target with `account_show` or `credit_card_show`. Read the rules of
names, as names.md tells.

### 2. Read the transactions of the period

`transaction_list` has no range of dates: `date` is a year, a month or a
day. Call it with `account` (the code of the target) and `date` for each
month that the period touches, then keep the transactions with a `date`
in the period. Read all the pages: when `total` is more than `count`,
call again with `offset`.

Leave out:

- a transaction with the type `transfer` or `payment`;
- a transaction with a `date` after today;
- a transaction that a schedule created (`schedule`): the schedule gave
  its title and its category.

A purchase in installments has one transaction for each part. Keep one
row for each `installment_id`: the part in the period.

### 3. Read the history

For each title in the period, look at how the user used it before:
`transaction_list` with `search` and no `date`. One call for each
different title is enough. The history gives the usual category, the
usual tags and a description.

### 4. Find the rows to show

A transaction goes to the list when one or more of these is true. Keep
the reason: the user sees it.

| Reason | Test |
| --- | --- |
| `no category` | The category is `OTHER` |
| `raw title` | The title looks as a line of a statement and not as a name: a code of a payment company (`PAG*`, `PAY`, `MP *`, `PIX`, `TED`, `DOC`), a long number, a date or a code at the end, a city or a country at the end, a word that is cut, all in capitals with no meaning |
| `rule` | A rule of names matches the title, and the title, the category or the tags are not those of the rule |
| `new title` | No other transaction in the history has this title, and the transaction has no description |
| `other category` | The history of the title has a different category in most of its transactions |
| `no tags` | The history of the title has tags, and this transaction has none |
| `large` | The value is 3 times the usual value of this title or more, or it is one of the 3 largest expenses of the period and has no description |
| `same day` | One more transaction of the target has the same value and the same date: a possible duplicate |

A transaction with a clear title, a category that is not `OTHER` and the
same category as its history does not go to the list. Do not show a row
only because it has no description or no tags.

When no row is in the list, tell the user that the period is tidy, with
the number of transactions you read, and stop.

### 5. Suggest, but only from evidence

For each row, make a suggestion when the history or the title supports
it:

- A rule of names that matches the title gives the title, the category
  and the tags. It is the best evidence: the user gave them before.
- Category: the usual category of the title, or the best match by
  meaning from `category_list`.
- Tags: the tags the same title had before. Without them, one or two
  tags the user already has that fit, marked `tag?`: a guess that needs
  a yes.
- Title: a clean name for a `raw title` (`PAG*MERCADOLIVRE 0423 SAO
  PAULO` becomes `Mercado Livre`). When `transaction_suggestions` has a
  title for the same merchant, use that exact title. For a title that is
  cut or is a code (`PAY MINUT 19/11`), suggest a name as a guess with
  `?` after it (`Minuto Pão de Açúcar?`), from the letters, the value and
  the category.
- Description: do not invent one. Only the user knows what the purchase
  was.

A row with no evidence has no suggestion: show `?` and ask the user what
it was. Put the rows with the same raw title together, so one answer
names all of them.

### 6. Show the list and wait

Show the rows with a number, the oldest date first, 20 rows at most for
each message. Each row has: the number, the date, the title, the value
with its currency, the category, the tags, the reason, and the
suggestion. For a part of a purchase in installments, show `(3/10)` after
the title. For a purchase with `virtual`, show `virtual`.

After the rows, tell the user how to answer, with an example:

```text
1: ok                      accept the suggestion of row 1
2: gift for Ana, cat SHOP, tags gift family
5: name Lamen House, cat FOOD, tags lunch
3: skip                    leave row 3 as it is
4: delete                  it is a duplicate
all ok                     accept all the suggestions that remain
```

The text with no label is the description. `name` is the new title; it
also goes to the other rows with the same raw title. Change nothing before the
user answers. When there are more rows than one message shows, do the
rows the user answered, then show the next rows.

### 7. Write what the user decided

For each row with an answer, call `transaction_edit` with the `id` and
only the fields that change:

- `description`, `category`, `tags`, `title`, as the user gave them or as
  the suggestion the user accepted.
- `tags` replaces all the tags of the transaction. To add a tag, send the
  tags the transaction has and the new one.
- Do not change `value`, `date`, `account` or `type` in this skill. When
  the user tells that one of them is wrong, ask and then change it as the
  skill finux-log-transactions tells.
- For a part of a purchase in installments, send `scope: all`: the
  description, the category and the tags are of the whole purchase. When
  a part is in a paid bill, nothing changes and the error tells it; use
  `scope: next` from the first part in a bill that is not paid, and tell
  the user.
- `delete`: call `transaction_delete`, show its message, and call it
  again with `confirm: true` only after a yes for that row.
- A category that does not exist: tell the user and ask. Create it with
  `category_add` only when the user asks.

Stop at the first error and report it.

### 8. Report

In a few lines: the number of transactions you read, the number in the
list, the number changed, skipped and deleted, and the rows that still
wait for an answer. When the result of an edit has `budget_warnings`,
report the `message` of each one: a new category can put an expense in a
budget.

### 9. Remember the names

Save as rules, as names.md tells, each name, category and tags that the
user gave or approved for a `raw title`. The pattern comes from the old
title, not from the new one. Show the rules you save in one line each,
and tell that the skills finux-log-transactions and
finux-import-statement use them from now on. Then offer one time to
rename the older transactions, out of the period, that match a new rule.

## Rules

- One target in each run. The user evaluates the rows by hand, so a
  short list is better than a full one.
- Keep each currency apart. Do not add two currencies.
- A suggestion is not a decision. Without `ok` or a value from the user,
  a row does not change.
- A transaction of a credit card in a bill that is paid cannot change:
  the error is `bill MM/YYYY of CODE is paid: delete its payment first`.
  In the list, mark these rows as `paid bill` (the `bill` of the
  transaction is in `credit_card_bills` with the status `paid`). Tell the
  user what to change and why it did not change. Do not delete the
  payment unless the user asks.
