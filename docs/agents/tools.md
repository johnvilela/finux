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

*Read.* List the accounts with their balances. The balance is the opening balance plus the transactions of the account.

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
| `limit` | integer | no | Most transactions to return. Default: 100 |
| `offset` | integer | no | Number of transactions to skip, to read the next page. Default: 0 |
| `search` | string | no | Only the transactions whose title contains this text |
| `tags` | string | no | Only the transactions that have all these tags, for example "market weekly" |
| `type` | string | no | Only the transactions of this type. One of: `expense`, `income`, `transfer`, `payment` |

### `transaction_show`

*Read.* Show one transaction by its id.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | integer | yes | Id of the transaction |

### `transaction_add`

*Change.* Create a transaction: an expense, an income or a transfer between two accounts. A transaction of a credit card goes to the bill of its date. To pay a bill of a credit card, use credit_card_pay.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `title` | string | yes | Title, at most 60 characters |
| `value` | string | yes | Positive amount in the currency of the account, as a decimal string, for example "1234.56". The type gives the sign |
| `account` | string | yes | Code or id of the account or the credit card of the transaction |
| `category` | string | no | Code or id of the category. Necessary, but for a transfer, which has the default XFER |
| `date` | string | no | YYYY-MM-DD or DD/MM/YYYY. Default: today |
| `description` | string | no | Description, at most 200 characters |
| `received` | string | no | For a transfer between accounts of different currencies: the amount the other account receives |
| `tags` | string | no | Tags divided by spaces, for example "market weekly". Use transaction_suggestions to find the tags in use |
| `to` | string | no | For a transfer: code or id of the account that receives the money. A credit card cannot have a transfer |
| `type` | string | no | Default: expense for a red category, income for a green one, transfer when 'to' is given. Necessary for a grey category. One of: `expense`, `income`, `transfer` |

### `transaction_edit`

*Change.* Change a transaction. Give only the arguments to change. A change of one side of a transfer also changes the other side. A payment of a bill cannot change: delete it.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | integer | yes | Id of the transaction |
| `account` | string | no | Code or id of the new account or credit card. Give value also when the currency is different |
| `category` | string | no | Code or id of the new category |
| `date` | string | no | New date, YYYY-MM-DD or DD/MM/YYYY |
| `description` | string | no | New description, at most 200 characters |
| `tags` | string | no | New tags; they replace the tags of the transaction. An empty string removes all tags |
| `title` | string | no | New title, at most 60 characters |
| `type` | string | no | New type. A transfer keeps its type. One of: `expense`, `income`, `transfer` |
| `value` | string | no | New positive amount as a decimal string |

### `transaction_delete`

*Delete.* Delete a transaction permanently. The delete of one side of a transfer also deletes the other side. The delete of a payment opens its bill again.

| Argument | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | integer | yes | Id of the transaction |
| `confirm` | boolean | no | Set to true to delete. Without it the tool only tells what it will delete. |

### `transaction_suggestions`

*Read.* List the titles and the tags that the transactions use, the last one used first. Use them to keep the titles and the tags of new transactions consistent.

No arguments.

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
