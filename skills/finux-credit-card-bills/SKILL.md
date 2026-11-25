---
name: finux-credit-card-bills
description: Shows and pays the credit card bills in Finux, the personal finance manager of the user. Use it for "what do I owe on my cards", "when is my card due", "show my Visa bill", "pay my card", "pay the bill", "I paid my credit card", "pay half of the bill", "undo the card payment", "why can I not change this purchase", or a question about the limit that is in use. It reports open, closed and late bills with their due dates, pays a bill from an account in full or in part after the user approves, and deletes a payment when a paid bill must change.
---

# Credit card bills in Finux

Tell the user what each card owes and when, and record the payments of
the bills.

## Before you start

- Reach Finux as [references/access.md](references/access.md) tells: MCP
  tools first, the CLI when there are none.
- How bills get their transactions, their statuses and the rules of a
  payment are in [references/concepts.md](references/concepts.md). The
  arguments of each tool are in
  [references/tools.md](references/tools.md).

## Show the bills

1. `credit_card_list` for the cards, with `used` and `available`.
2. `credit_card_show` for each card: the open bill with its transactions,
   and `pending_bills`, the bills that are closed or late.
3. `credit_card_bills` when the user asks for the history of a card.

Report in the order of urgency:

1. **Late** bills: total, due date, days late.
2. **Closed** bills: total, due date, days until it.
3. The **open** bill of each card: total until now and its closing date.
4. The limit: used, available, and the share in use.

Show the transactions of a bill only when the user asks for them or asks
about one bill.

A purchase made with a virtual card has `virtual: true`. The open bill of
`credit_card_show` has `virtual_total` when it has such purchases: report
it when the user asks what the virtual card spent. For another bill, call
`transaction_list` with `account` and `virtual: true`, and keep the
transactions whose `bill` is that period.

A purchase in installments has one transaction in each bill, with
`installment` and `installments` (part 3 of 10). Its future parts are in
future bills, which `credit_card_bills` lists with the status `open`.
These bills are not due: do not report them as debts to pay now. Their
totals are in `used`, so the limit already counts the whole purchase.

A purchase with `off_limit: true` does not use the limit while its bill
is a future one: a part counts in `used` only from the day its bill is
the current one. The card has `off_limit`, the total of these purchases
in the bills after the current one. When it is not
zero, report it apart from the limit in use, and say that the bills
have it.
When the user asks "how much do I still owe on installments", add the
parts with a `date` after today for each purchase
(`transaction_list` with `account`), with a script.

## Pay a bill

### 1. Choose the bill

The user names it, or it is the oldest bill that is closed or late:
`credit_card_pay` uses that one when you send no `bill`. An open bill
cannot be paid. When the user wants to pay before the closing date,
explain that Finux pays a bill after it closes.

### 2. Choose the account

The account must have the currency of the card. Use the account the user
names. Without a name, use the account that paid the last bills of this
card: `transaction_list` with `type` `payment`. When that does not give
one account, ask.

Look at the balance with `account_show`. When the payment makes it
negative, say so before you pay; the user decides.

### 3. Choose the value

Without `value`, the tool pays the total. The user can pay less: the bill
is then paid, and the rest goes to the next bill as `carried`. Tell the
user two facts before a partial payment: the rest shows in the next bill,
and Finux adds no interest, so the interest of the bank must be recorded
as an expense on the card when it comes.

The value cannot be more than the total.

### 4. Confirm and pay

Say what you will do in one sentence, for example: "Pay the bill 2026-11
of VISA, R$ 1.000,00, from NU on 2026-11-17?". Wait for a yes. When the
user already gave all of this in the request ("pay the Visa bill from
Nubank"), that request is the yes.

Call `credit_card_pay`. Send `date` when the user paid on a different
day.

### 5. Report

From the result: the bill that is now paid, the id of the payment, the
value that went to the next bill (`remaining` and `carried_to`), and the
new balance of the account (`account_show`).

## A bill that is paid must change

A paid bill is locked. An add, an edit or a delete of one of its
transactions fails with `bill MM/YYYY of CODE is paid: delete its payment
first`. To change it:

1. Call `credit_card_unpay` for the card, with `bill` when it is not the
   last paid bill. It returns a message that tells what it will delete.
2. Show the message. Wait for a yes. Call it again with `confirm: true`.
3. Make the change.
4. Pay the bill again with the same account, the same date and the same
   value as before, unless the user wants a different payment.

`credit_card_unpay` fails when the bill that got the rest of a partial
payment is also paid: delete the payment of that later bill first.

For a purchase in installments, the parts in paid bills are locked and
the others are not. A change with the scope `all` fails with
`installment K/N (transaction ID): bill MM/YYYY of CODE is paid` and
changes nothing. Usually the right change is the scope `next` with the id
of the first part in a bill that is not paid: the past stays as the bank
charged it. Delete a payment only when the user wants to correct a paid
part.

## The user paid the rest of a purchase early

The bank cancels the future parts and charges one amount, often with a
discount.

1. Find the parts: `transaction_list` with `search` of the title; keep the
   ones with the same `installment_id`.
2. Call `transaction_delete` with the id of the first part that will not
   come and `scope: "next"`. Show the message, wait for a yes, then send
   `confirm: true`.
3. Record what the user paid as one expense on the card, on the date the
   bank charged it, with a title such as `Loja X (early payoff)`.
4. Report the new `used` and `available` of the card.

## The user says "I paid my card"

That is a payment to record, not an expense to add. Use the steps of
"Pay a bill" with the date the user paid. Do not create an expense with
`transaction_add`: the purchases of the bill are already the expenses,
and a second one counts them two times.

## Errors you can expect

- `... is still open: it closes on DD/MM/YYYY`: the bill cannot be paid
  yet.
- `no bill of CODE waits for a payment`: all bills are paid or open.
- `value is more than the total of the bill`: lower the value.
- A message about a different currency: use an account in the currency of
  the card.
