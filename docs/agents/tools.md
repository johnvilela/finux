# Finux MCP tools

<!-- Generated from the tool schemas by 'make docs'. Do not edit. -->

Each tool returns JSON; see [concepts.md](concepts.md) for the objects in a
result. An agent can show a tool with a prefix, for example
`mcp__finux__account_list`. *Read* tools change nothing. *Delete* tools
need `confirm: true`; without it they return `confirm_required` and a
message, and change nothing.

## Database

### `database_init`

*Change.* Create the Finux database or update it to the current version. The first setup adds the base categories. It is safe to call again: it keeps the data.

No arguments.

### `database_doctor`

*Read.* Check that the Finux database exists and is up to date. It changes nothing. When healthy is false, call database_init.

No arguments.

### `database_reseed`

*Change.* Add the base categories that are missing, for example after one was deleted. It finds them by code and does not change the categories that exist.

No arguments.

## Account

### `account_list`

*Read.* List the accounts with their balances. The balance is the opening balance plus the transactions of the account that have a date up to today.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `include_archived` | boolean | no | Also return the archived ones |

### `account_show`

*Read.* Show one account with its balance. It also finds an archived account.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `account` | string | yes | Code or id of the account |

### `account_add`

*Change.* Create an account, for example a bank account or a wallet. For a credit card, use credit_card_add.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | yes | Name of the account |
| `currency` | string | yes | Currency of the account; it cannot change later. One of: `USD`, `EUR`, `BRL`, `BTC` |
| `balance` | string | no | Opening balance as a decimal string, for example "1234.56". It can be negative. Default: 0 |
| `code` | string | no | Short code, 1 to 5 letters and digits. Default: made from the name |
| `color` | string | no | Color. Default: the first color no account uses. One of: `red`, `orange`, `amber`, `yellow`, `lime`, `green`, `emerald`, `teal`, `cyan`, `sky`, `blue`, `indigo`, `violet`, `purple`, `pink`, `slate` |
| `description` | string | no | Description |

### `account_edit`

*Change.* Change the name, the code, the description or the color of an account. Give only the arguments to change. The currency and the opening balance cannot change.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `account` | string | yes | Code or id of the account |
| `code` | string | no | New code, 1 to 5 letters and digits |
| `color` | string | no | New color. One of: `red`, `orange`, `amber`, `yellow`, `lime`, `green`, `emerald`, `teal`, `cyan`, `sky`, `blue`, `indigo`, `violet`, `purple`, `pink`, `slate` |
| `description` | string | no | New description |
| `name` | string | no | New name |

### `account_archive`

*Change.* Archive an account: it leaves the lists and accepts no new transactions, but it keeps its transactions. account_restore brings it back.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `account` | string | yes | Code or id of the account |

### `account_restore`

*Change.* Restore an archived account.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `account` | string | yes | Code or id of the account |

### `account_delete`

*Delete.* Delete an account permanently, with its transactions. This cannot be undone. The other side of each transfer stays in its account as an expense or an income of the category OTHER. To keep the data, use account_archive.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `account` | string | yes | Code or id of the account |
| `confirm` | boolean | no | Set to true to delete. Without it the tool only tells what it will delete. |

## Category

### `category_list`

*Read.* List the categories. The field accepts tells the type of transaction a category can have: expense, income or all.

No arguments.

### `category_show`

*Read.* Show one category by its code or id.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `category` | string | yes | Code or id of the category |

### `category_add`

*Change.* Create a category for transactions.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | yes | Name of the category |
| `color` | string | yes | The color gives the transactions the category can have: red = expenses, green = incomes, grey = all types. One of: `red`, `green`, `grey` |
| `code` | string | no | Short code, 1 to 5 letters and digits. Default: made from the name |
| `description` | string | no | Description |

### `category_edit`

*Change.* Change the name, the code, the description or the color of a category. Give only the arguments to change.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `category` | string | yes | Code or id of the category |
| `code` | string | no | New code, 1 to 5 letters and digits |
| `color` | string | no | New color: red = expenses, green = incomes, grey = all types. Refused when the transactions of the category do not accept it. One of: `red`, `green`, `grey` |
| `description` | string | no | New description |
| `name` | string | no | New name |

### `category_delete`

*Delete.* Delete a category permanently. Its transactions go to the category OTHER.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `category` | string | yes | Code or id of the category |
| `confirm` | boolean | no | Set to true to delete. Without it the tool only tells what it will delete. |

## Transaction

### `transaction_list`

*Read.* List transactions, the newest first, with the totals of all transactions that match the filters. All filters are optional and combine. Read more pages with limit and offset.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `account` | string | no | Only the transactions of this account or credit card (code or id) |
| `category` | string | no | Only the transactions of this category (code or id) |
| `date` | string | no | A year (2026), a month (2026-11 or 11/2026) or a date (2026-11-17) |
| `goal` | string | no | Only the transactions linked to this goal (code or id) |
| `limit` | integer | no | Most transactions to return. Default: 100 |
| `offset` | integer | no | Number of transactions to skip, to read the next page. Default: 0 |
| `search` | string | no | Only the transactions whose title contains this text |
| `tags` | string | no | Only the transactions that have all these tags, for example "market weekly" |
| `type` | string | no | Only the transactions of this type. One of: `expense`, `income`, `transfer`, `payment` |
| `virtual` | boolean | no | true: only the transactions made with a virtual card of a credit card. false: only the other ones |

### `transaction_show`

*Read.* Show one transaction by its id.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | integer | yes | Id of the transaction |

### `transaction_add`

*Change.* Create a transaction: an expense, an income or a transfer between two accounts. A transaction of a credit card goes to the bill of its date. To pay a bill of a credit card, use credit_card_pay. Give goal to count the transaction toward a goal. Give virtual for a purchase made with a virtual card of a credit card. Give installments for an expense or an income in monthly parts: the result then has all the parts in transactions.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `title` | string | yes | Title, at most 60 characters |
| `account` | string | yes | Code or id of the account or the credit card of the transaction |
| `category` | string | no | Code or id of the category. Necessary, but for a transfer, which has the default XFER |
| `date` | string | no | YYYY-MM-DD or DD/MM/YYYY. Default: today |
| `description` | string | no | Description, at most 200 characters |
| `first_installment` | integer | no | With installments, for a purchase that is already in progress: the first part to create. date is then the date of that part. Default: 1 |
| `goal` | string | no | Code or id of the goal the transaction counts toward. The goal must have the currency of the account |
| `installment_value` | string | no | With installments: the amount of one part, in place of value |
| `installments` | integer | no | For an expense or an income in installments: the number of monthly parts, from 2 to 120. The tool creates one transaction for each part |
| `received` | string | no | For a transfer between accounts of different currencies: the amount the other account receives |
| `tags` | string | no | Tags divided by spaces, for example "market weekly". Use transaction_suggestions to find the tags in use |
| `to` | string | no | For a transfer: code or id of the account that receives the money. A credit card cannot have a transfer |
| `type` | string | no | Default: expense for a red category, income for a green one, transfer when 'to' is given. Necessary for a grey category. One of: `expense`, `income`, `transfer` |
| `value` | string | no | Positive amount in the currency of the account, as a decimal string, for example "1234.56". The type gives the sign. With installments it is the total of all the parts. Necessary, but when installment_value is given |
| `virtual` | boolean | no | true when the person paid with a virtual card of the credit card. Only for a transaction of a credit card. Default: false |

### `transaction_edit`

*Change.* Change a transaction. Give only the arguments to change. A change of one side of a transfer also changes the other side. A payment of a bill cannot change: delete it. For a transaction in installments, scope tells which parts change: the result then has them in transactions.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | integer | yes | Id of the transaction |
| `account` | string | no | Code or id of the new account or credit card. Give value also when the currency is different |
| `category` | string | no | Code or id of the new category |
| `date` | string | no | New date, YYYY-MM-DD or DD/MM/YYYY |
| `description` | string | no | New description, at most 200 characters |
| `goal` | string | no | Code or id of the new goal. An empty string takes the goal away |
| `installment_value` | string | no | With the scope next or all: the new amount of each part, in place of value |
| `scope` | string | no | For a transaction in installments: one changes this part only, next changes this part and the parts after it, all changes all the parts. With next or all, value is the new total of these parts and date is the new date of the first of them. Default: one. One of: `one`, `next`, `all` |
| `tags` | string | no | New tags; they replace the tags of the transaction. An empty string removes all tags |
| `title` | string | no | New title, at most 60 characters |
| `type` | string | no | New type. A transfer keeps its type. One of: `expense`, `income`, `transfer` |
| `value` | string | no | New positive amount as a decimal string |
| `virtual` | boolean | no | true when the person paid with a virtual card of the credit card; false takes the mark away. Only for a transaction of a credit card |

### `transaction_delete`

*Delete.* Delete a transaction permanently. The delete of one side of a transfer also deletes the other side. The delete of a payment opens its bill again. For a transaction in installments, scope tells which parts to delete.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | integer | yes | Id of the transaction |
| `confirm` | boolean | no | Set to true to delete. Without it the tool only tells what it will delete. |
| `scope` | string | no | For a transaction in installments: one deletes this part only, next deletes this part and the parts after it, all deletes all the parts. Default: one. One of: `one`, `next`, `all` |

### `transaction_suggestions`

*Read.* List the titles and the tags that the transactions use, the last one used first. Use them to keep the titles and the tags of new transactions consistent.

No arguments.

### `transaction_corrections`

*Read.* List what the changes to transactions taught: for the title a transaction had (pattern), the title, the category, the tags and the description that the user wants. Call it with the titles of the transactions you create or tidy, and use the corrections in place of your own names. A correction with the source user is a change the user made by hand: it wins over what you know. A field that is absent was not taught.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `limit` | integer | no | Most corrections to return. Default: 50 |
| `offset` | integer | no | Number of corrections to skip, to read the next page. Default: 0 |
| `titles` | array | no | Titles or raw lines of a statement, for example ["PAY LAMEI 21/11"]. The result then has only the corrections of these titles; dates and codes in a title do not count. Without it, the result has all the corrections, the last one changed first |

## Credit card

### `credit_card_list`

*Read.* List the credit cards with the limit, the used value (the total of the bills that are not paid) and the available value.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `include_archived` | boolean | no | Also return the archived ones |

### `credit_card_show`

*Read.* Show one credit card, its open bill with the transactions of that bill, and the bills that wait for a payment (closed or late).

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `credit_card` | string | yes | Code or id of the credit card |

### `credit_card_bills`

*Read.* List the bills of a credit card, the newest first. A bill is open until its closing date, then closed until its due date, then late, until it is paid.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `credit_card` | string | yes | Code or id of the credit card |

### `credit_card_add`

*Change.* Create a credit card. Its purchases are transactions with the code of the card as the account; they go to a monthly bill and do not change the balance of an account.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | yes | Name of the credit card |
| `currency` | string | yes | Currency of the credit card; it cannot change later. One of: `USD`, `EUR`, `BRL`, `BTC` |
| `limit` | string | yes | Credit limit as a decimal string, for example "5000" |
| `closing_day` | integer | yes | Day of the month the bill closes, 1 to 31. A purchase after this day goes to the next bill |
| `due_day` | integer | yes | Day of the month the bill is due, 1 to 31 |
| `code` | string | no | Short code, 1 to 5 letters and digits, different from the codes of the accounts. Default: made from the name |
| `color` | string | no | Color. Default: the first color no credit card uses. One of: `red`, `orange`, `amber`, `yellow`, `lime`, `green`, `emerald`, `teal`, `cyan`, `sky`, `blue`, `indigo`, `violet`, `purple`, `pink`, `slate` |
| `description` | string | no | Description |

### `credit_card_edit`

*Change.* Change a credit card. Give only the arguments to change. The currency cannot change.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `credit_card` | string | yes | Code or id of the credit card |
| `closing_day` | integer | no | New closing day, 1 to 31. It applies to the bills that do not exist yet |
| `code` | string | no | New code, 1 to 5 letters and digits |
| `color` | string | no | New color. One of: `red`, `orange`, `amber`, `yellow`, `lime`, `green`, `emerald`, `teal`, `cyan`, `sky`, `blue`, `indigo`, `violet`, `purple`, `pink`, `slate` |
| `description` | string | no | New description |
| `due_day` | integer | no | New due day, 1 to 31. It applies to the bills that do not exist yet |
| `limit` | string | no | New credit limit as a decimal string |
| `name` | string | no | New name |

### `credit_card_pay`

*Change.* Pay a bill of a credit card from an account. It creates a transaction of the type payment in the account and marks the bill as paid. A bill that is still open cannot be paid. A paid bill is locked: its transactions cannot change until credit_card_unpay.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `credit_card` | string | yes | Code or id of the credit card |
| `from` | string | yes | Code or id of the account that pays. It must have the currency of the credit card |
| `bill` | string | no | Bill to pay, as YYYY-MM or MM/YYYY (the month of its due date). Default: the oldest bill that is closed or late |
| `date` | string | no | Date of the payment, YYYY-MM-DD or DD/MM/YYYY. Default: today |
| `value` | string | no | Amount to pay as a decimal string. Default: the total of the bill. With a smaller value, the bill is paid and the rest goes to the next bill |

### `credit_card_unpay`

*Delete.* Delete the payment of a bill of a credit card. The payment transaction is deleted, the money goes back to the account and the bill opens again.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `credit_card` | string | yes | Code or id of the credit card |
| `bill` | string | no | Bill, as YYYY-MM or MM/YYYY. Default: the last paid bill |
| `confirm` | boolean | no | Set to true to delete the payment. Without it the tool only tells what it will delete. |

### `credit_card_archive`

*Change.* Archive a credit card: it leaves the lists and accepts no new transactions, but it keeps its bills and transactions. credit_card_restore brings it back.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `credit_card` | string | yes | Code or id of the credit card |

### `credit_card_restore`

*Change.* Restore an archived credit card.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `credit_card` | string | yes | Code or id of the credit card |

### `credit_card_delete`

*Delete.* Delete a credit card permanently, with its bills and its transactions. This cannot be undone. Each payment of its bills stays in its account as an expense of the category OTHER. To keep the data, use credit_card_archive.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `credit_card` | string | yes | Code or id of the credit card |
| `confirm` | boolean | no | Set to true to delete. Without it the tool only tells what it will delete. |

## Goal

### `goal_list`

*Read.* List the goals with their target, progress and status. The progress comes from the transactions linked to each goal. Status is active, reached or overdue.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `include_archived` | boolean | no | Also return the archived ones |

### `goal_show`

*Read.* Show one goal and the transactions linked to it, the newest first. Each transaction has effect: what it adds to the goal, below zero when it removes. A transfer shows one time.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `goal` | string | yes | Code or id of the goal |

### `goal_add`

*Change.* Create a goal: an amount to save, a debt to pay off or a purchase to save for. Link transactions to it with the argument goal of transaction_add and transaction_edit.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `title` | string | yes | Title of the goal, at most 60 characters |
| `type` | string | yes | How a linked transaction counts. save and purchase: an income adds and an expense removes. payoff: an expense adds and an income removes. One of: `save`, `payoff`, `purchase` |
| `target` | string | yes | Amount to reach, as a decimal string, for example "10000" |
| `currency` | string | yes | Currency of the goal; it cannot change later. Only transactions in this currency can link. One of: `USD`, `EUR`, `BRL`, `BTC` |
| `code` | string | no | Short code, 1 to 5 letters and digits. Default: made from the title |
| `description` | string | no | Description, at most 200 characters |
| `due_date` | string | no | Final date, YYYY-MM-DD or DD/MM/YYYY. Optional |

### `goal_edit`

*Change.* Change a goal. Give only the arguments to change. The currency cannot change.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `goal` | string | yes | Code or id of the goal |
| `code` | string | no | New code, 1 to 5 letters and digits |
| `description` | string | no | New description |
| `due_date` | string | no | New final date, YYYY-MM-DD or DD/MM/YYYY. An empty string removes it |
| `target` | string | no | New amount to reach, as a decimal string |
| `title` | string | no | New title |
| `type` | string | no | New type; the progress is counted again. One of: `save`, `payoff`, `purchase` |

### `goal_archive`

*Change.* Archive a goal: it leaves the lists and takes no new transactions, but its transactions keep the link. goal_restore brings it back.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `goal` | string | yes | Code or id of the goal |

### `goal_restore`

*Change.* Restore an archived goal.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `goal` | string | yes | Code or id of the goal |

### `goal_delete`

*Delete.* Delete a goal permanently. No transaction is deleted: the linked transactions stay and only lose the link. To keep the goal and its links, use goal_archive.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `goal` | string | yes | Code or id of the goal |
| `confirm` | boolean | no | Set to true to delete. Without it the tool only tells what it will delete. |

## Budget

### `budget_list`

*Read.* List the budgets with their limit, categories, period and status. current is the period of today: what it has available, what it spent and what remains. Status is ok, near (80% spent or more), over, upcoming or ended.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `include_archived` | boolean | no | Also return the archived ones |

### `budget_show`

*Read.* Show one budget with one period, the transactions that count in it, the newest first, and the periods before it. Each transaction has counts: what it adds to the spent amount, below zero for an income. Give date to see the period of a date in the past.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `budget` | string | yes | Code or id of the budget |
| `date` | string | no | A date in the period to show, YYYY-MM-DD or DD/MM/YYYY. Default: the current period |
| `history` | integer | no | Number of periods before it to return, the newest first. Default: 6 |

### `budget_add`

*Change.* Create a budget: a limit of money for some categories in a period. Every expense and income of those categories counts on it without a link: an expense adds to the spent amount and an income gives money back. Transfers and payments of credit card bills do not count. Give every for a recurring budget, or end_date for a one-time budget.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `title` | string | yes | Title of the budget, at most 60 characters |
| `limit` | string | yes | Amount of one period, as a decimal string, for example "800" |
| `currency` | string | yes | Currency of the budget; it cannot change later. Only transactions in this currency count. One of: `USD`, `EUR`, `BRL`, `BTC` |
| `categories` | string | no | Codes of the categories that count, for example "FOOD,FUN". Default, or "all": all categories but the income ones |
| `code` | string | no | Short code, 1 to 5 letters and digits. Default: made from the title |
| `description` | string | no | Description, at most 200 characters |
| `end_date` | string | no | Last day of the budget. Optional with every; necessary without it |
| `every` | string | no | Length of a period of a recurring budget: "day", "week", "month", "year" or a number of them, for example "2 weeks". Without it the budget is for one time and needs end_date |
| `rollover` | boolean | no | Move what a period did not spend, or spent too much, to the next period. Default: false |
| `start_date` | string | no | First day of the first period. The periods count from it: a monthly budget that starts on day 5 has periods from day 5 to day 4. Default: the first day of this week, month or year for a period of one of them; today for the others |

### `budget_edit`

*Change.* Change a budget. Give only the arguments to change. The currency cannot change. A change of the limit, the categories or the dates counts all the periods again.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `budget` | string | yes | Code or id of the budget |
| `categories` | string | no | New codes of the categories that count; they replace the list. "all" counts all categories but the income ones |
| `code` | string | no | New code, 1 to 5 letters and digits |
| `description` | string | no | New description |
| `end_date` | string | no | New last day of the budget. An empty string removes it |
| `every` | string | no | New length of a period, for example "month" or "2 weeks". "none" makes a one-time budget, which needs an end date |
| `limit` | string | no | New amount of one period, as a decimal string. It also changes the periods in the past |
| `rollover` | boolean | no | Start or stop the move of what a period left to the next period |
| `start_date` | string | no | New first day of the first period |
| `title` | string | no | New title |

### `budget_archive`

*Change.* Archive a budget: it leaves the lists and gives no warnings, but it keeps its history. budget_restore brings it back.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `budget` | string | yes | Code or id of the budget |

### `budget_restore`

*Change.* Restore an archived budget.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `budget` | string | yes | Code or id of the budget |

### `budget_delete`

*Delete.* Delete a budget permanently. No transaction changes: a budget has no link to its transactions. To keep the budget and its history, use budget_archive.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `budget` | string | yes | Code or id of the budget |
| `confirm` | boolean | no | Set to true to delete. Without it the tool only tells what it will delete. |

## Schedule

### `schedule_list`

*Read.* List the scheduled transactions: bills to pay and incomes to receive, one time or again and again. A schedule is not a transaction: it changes no balance until schedule_pay. Each schedule has due, the dates of the occurrences that wait for a confirmation, and next. The result has due: the number of occurrences that wait in all schedules. Without include_archived, the schedules that are done or archived stay out.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `include_archived` | boolean | no | Also return the archived ones |

### `schedule_show`

*Read.* Show one schedule and the transactions it created, the newest first.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `schedule` | string | yes | Code or id of the schedule |

### `schedule_add`

*Change.* Schedule a transaction that will happen: an expense or an income, one time (no every) or again and again (every). It creates no transaction. Each occurrence waits until schedule_pay creates its transaction or schedule_skip drops it.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `title` | string | yes | Title of the transactions, at most 60 characters |
| `value` | string | yes | Positive amount in the currency of the account, as a decimal string. The type gives the sign |
| `account` | string | yes | Code or id of the account or the credit card of the transactions |
| `category` | string | yes | Code or id of the category |
| `automatic` | boolean | no | True creates each transaction on its date without a confirmation, when the user also turned on the setting automatic-schedules. Default: false. Set it only when the user asks for it |
| `code` | string | no | Short code, 1 to 5 letters and digits. Default: made from the title |
| `date` | string | no | Date of the first occurrence, YYYY-MM-DD or DD/MM/YYYY. Default: today |
| `description` | string | no | Description, at most 200 characters |
| `end_date` | string | no | With every: the last day an occurrence can have |
| `every` | string | no | Time between two occurrences: day, week, month, year, or a number of them, for example "2 weeks". Without it the schedule is for one time |
| `goal` | string | no | Code or id of the goal the transactions count toward |
| `tags` | string | no | Tags of the transactions, divided by spaces |
| `times` | integer | no | With every: the number of occurrences, from 1 to 1000 |
| `type` | string | no | Default: expense for a red category, income for a green one. Necessary for a grey category. One of: `expense`, `income` |

### `schedule_edit`

*Change.* Change a schedule. Give only the arguments to change. The change is for the occurrences that wait; the transactions it already created stay as they are.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `schedule` | string | yes | Code or id of the schedule |
| `account` | string | no | Code or id of the new account or credit card. Give value also when the currency is different |
| `automatic` | boolean | no | True creates each transaction on its date without a confirmation; false stops it |
| `category` | string | no | Code or id of the new category |
| `code` | string | no | New code, 1 to 5 letters and digits |
| `date` | string | no | New date of the next occurrence. The occurrences after it count from this date |
| `description` | string | no | New description |
| `end_date` | string | no | New last day. An empty string removes it |
| `every` | string | no | New time between two occurrences. "none" makes the schedule for one time |
| `goal` | string | no | Code or id of the new goal. An empty string takes the goal away |
| `tags` | string | no | New tags. An empty string removes all tags |
| `times` | integer | no | New number of occurrences from now on. 0 removes it |
| `title` | string | no | New title |
| `type` | string | no | New type. One of: `expense`, `income` |
| `value` | string | no | New positive amount as a decimal string |

### `schedule_pay`

*Change.* Confirm the next occurrence of a schedule: create its transaction. Use it when the user paid the bill or got the income. The occurrences go in order, the oldest first. Give value, date or account when the real ones are different. The result has the transaction, the schedule as it is now, and budget_warnings.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `schedule` | string | yes | Code or id of the schedule |
| `account` | string | no | Code or id of the account or credit card that paid, when it is another one |
| `date` | string | no | Real date. Default: the date of the occurrence, or today for an occurrence that is not due yet |
| `value` | string | no | Real amount, when it is not the amount of the schedule |

### `schedule_skip`

*Change.* Skip the next occurrence of a schedule: it creates no transaction and the occurrence does not wait any more. Use it when the bill did not come or the user will not pay it.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `schedule` | string | yes | Code or id of the schedule |

### `schedule_archive`

*Change.* Archive a schedule: it stops and leaves the lists. Its transactions stay. schedule_restore brings it back.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `schedule` | string | yes | Code or id of the schedule |

### `schedule_restore`

*Change.* Restore an archived schedule. The occurrences whose dates passed are due again.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `schedule` | string | yes | Code or id of the schedule |

### `schedule_delete`

*Delete.* Delete a schedule permanently. No transaction is deleted: the transactions it created stay and only lose the link. To keep the schedule, use schedule_archive.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `schedule` | string | yes | Code or id of the schedule |
| `confirm` | boolean | no | Set to true to delete. Without it the tool only tells what it will delete. |
