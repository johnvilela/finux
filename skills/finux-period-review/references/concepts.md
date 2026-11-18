# Finux concepts

Finux is a personal finance manager for one user. It has accounts,
categories, transactions and credit cards with bills. This file gives the
rules an agent needs to read and change that data correctly.

## Codes and references

An account, a category and a credit card each have a short code: 1 to 5
letters and digits, in capitals, for example `NU`, `FOOD`, `VISA`. An
argument that names one of them accepts the code or the numeric id. Prefer
the code.

Accounts and credit cards share one set of codes: a code is an account or a
credit card, not the two. Categories have their own set.

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
balance is its opening balance plus the amounts of its transactions. The
balance can be negative.

An archived account leaves the lists and takes no new transactions, but it
keeps its transactions; it can be restored. A permanent delete also deletes
its transactions and cannot be undone.

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
  transaction needs it.
- The name of a bill is the month of its due date, as `YYYY-MM`. Example
  for a card that closes on day 25 and is due on day 5: a purchase on
  2026-11-10 goes to the bill `2026-12` (closes 2026-11-25, due
  2026-12-05); a purchase on 2026-11-26 goes to the bill `2027-01`.
- New closing and due days apply only to the bills that do not exist yet.
- `used` is the total of the bills that are not paid. `available` is the
  limit less `used`.

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

## Deletes need confirmation

`account_delete`, `category_delete`, `transaction_delete`,
`credit_card_delete` and `credit_card_unpay` do nothing without
`confirm: true`. The first call returns
`{"confirm_required": true, "message": "..."}`, where the message tells
what the tool will delete. Show that message to the user and wait for a
clear yes before the call with `confirm: true`. Prefer `account_archive`
and `credit_card_archive` when the user only wants an item out of the way.

## Objects in a result

Money fields are money objects, as in [Money](#money).

**Account:** `id`, `code`, `name`, `description`, `color`, `currency`,
`archived`, `opening_balance`, `balance`.

**Category:** `id`, `code`, `name`, `description`, `color`, `accepts`.

**Transaction:** `id`, `title`, `description`, `type`, `amount` (signed),
`currency`, `date`, `tags` (list, each with `#`), `category` (code), and
its place:

- in an account: `account` (code);
- a purchase on a credit card: `credit_card` (code) and `bill` (`YYYY-MM`),
  and no `account`;
- a payment: `account`, and the `credit_card` and `bill` it paid;
- a transfer: `account`, `pair_id` and `other_account`.

**Credit card:** `id`, `code`, `name`, `description`, `color`, `currency`,
`archived`, `limit`, `used`, `available`, `closing_day`, `due_day`.

**Bill:** `period` (`YYYY-MM`), `closes_on`, `due_on`, `status`, `carried`,
`spent`, `total`. A paid bill also has `paid_on`, `paid`, `remaining` (the
value that went to the next bill) and `payment_id`.

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
