# Finux

A personal finance manager written in Go, starting with a CLI.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/johnvilela/finux/main/install.sh | sh
```

The script downloads the newest release for Linux x86-64, checks its SHA-256,
puts `finux` in `~/.local/bin` and runs `finux init`, which creates the
database `~/.finux/finux.db`. It needs `curl` or `wget`. When `~/.local/bin`
is not in your `PATH`, the script tells you the line to add to the file of
your shell. Then run `finux`.

| Variable | Meaning | Default |
| --- | --- | --- |
| `FINUX_VERSION` | Release to install, for example `v0.10.0` | the newest |
| `FINUX_INSTALL_DIR` | Folder of the binary | `~/.local/bin` |
| `FINUX_DB` | Database file | `~/.finux/finux.db` |

```sh
curl -fsSL https://raw.githubusercontent.com/johnvilela/finux/main/install.sh | FINUX_VERSION=v0.10.0 sh
```

For another system, [build from source](#build-and-run).

### Update

```sh
finux version          # the version you have
finux update --check   # tell if a newer version exists
finux update           # install it
```

`finux update` asks GitHub for the newest release. When it is newer, Finux
runs the install script of that release for the folder of your binary. The
script makes a copy of your database beside it, as
`finux.db.backup-<date>-<time>`, and the new version then applies its
database migrations. When that works, the new binary replaces the old one.
When it fails, the script puts the database back and your version stays.
Delete the copy when the new version works for you.

To uninstall, delete `~/.local/bin/finux`. Your data is in `~/.finux`.

### Release a version

A version is a Git tag such as `v0.10.0`. When the tag goes to GitHub, the
workflow `.github/workflows/release.yml` runs the tests, builds the binary
with that version and makes a release with `finux-linux-amd64` and
`checksums.txt`:

```sh
git tag v0.10.1
git push origin v0.10.1
```

## Development

Requires Go 1.27.3 or newer and Make. SQLite builds require CGO enabled and a C
compiler, such as GCC, when the database driver is used.

## Dependencies

- Lip Gloss v2 (`charm.land/lipgloss/v2`) for terminal styling.
- Bubbles v2 (`charm.land/bubbles/v2`) for terminal UI components.
- Bubble Tea v2 (`charm.land/bubbletea/v2`), required by Bubbles, for the terminal UI event loop.
- GORM (`gorm.io/gorm`) for mapping Go structs to database tables.
- GORM's official SQLite driver (`gorm.io/driver/sqlite`) for local storage, using CGO.

Lip Gloss styles all output. Bubble Tea and Bubbles run the forms, the
selectors and the confirmation prompt. GORM stores the data in SQLite.
`github.com/charmbracelet/x/term` (already required by Bubble Tea) detects if
input and output are terminals.

## Test

```sh
go test ./...
go vet ./...
```

## Build and run

```sh
make prod
./bin/finux
```

Output:

```text
███████╗██╗███╗   ██╗██╗   ██╗██╗  ██╗
██╔════╝██║████╗  ██║██║   ██║╚██╗██╔╝
█████╗  ██║██╔██╗ ██║██║   ██║ ╚███╔╝
██╔══╝  ██║██║╚██╗██║██║   ██║ ██╔██╗
██║     ██║██║ ╚████║╚██████╔╝██╔╝ ██╗
╚═╝     ╚═╝╚═╝  ╚═══╝ ╚═════╝ ╚═╝  ╚═╝

Your personal finance manager

Usage: finux [command]

Commands:
  init              Set up the database
  config            Change the settings and set up AI agents
  accounts, ac      Manage accounts
  categories, ct    Manage categories
  transactions, tx  Manage transactions
  credit-card, cc   Manage credit cards
  goals, gl         Manage goals
  budgets, bg       Manage budgets
  scheduled, sc     Manage scheduled transactions
  resume, rs        Show your financial resume
  update            Update Finux to the newest version
  version           Show the version
  help              Show this help

Options:
  -h, --help        Show this help
      --version     Show the version
```

The executable stays in the project; it is not installed on your PATH.
`make prod` gives the binary the version of the Git tag, or the last tag
with the distance to it (`v0.10.0-3-gabc1234`).

Run `./bin/finux help`, `./bin/finux --help`, or `./bin/finux -h` for the same
screen. Finux prints help and returns to the shell. Unsupported arguments exit
with status 2 and a diagnostic on stderr.

The logo and headings use green when the terminal supports colors. Redirected
output is plain text unless color is explicitly forced with `CLICOLOR_FORCE=1`.
Set `NO_COLOR=1` to disable colors, including forced colors.

For a debuggable build with optimizations and inlining disabled:

```sh
make dev
./bin/finux-dev
```

`make dev` also gives the development build its own database,
`finux-dev.db` in the root of the project. Git ignores the file. After each
build, `make dev` applies the migrations to it and adds sample data:

- If the database does not exist, it is created with the base categories, a
  base set of 8 accounts, 2 credit cards, 3 goals, 3 budgets, 4 schedules
  and 30 transactions. The credit card `VISA` has an open bill, a late bill and a
  paid bill. The credit card `AMEX` has a purchase in 6 installments.
- If the database exists, it keeps its data and gets 3 more random accounts and
  5 more random transactions.

To delete the database and start again from the base data:

```sh
make dev NEW_DB=1
```

`./bin/finux-dev seed` and `./bin/finux-dev seed --new-db` do the same without a
new build. Make does not accept `--new-db` after a target, so the Makefile uses
`NEW_DB=1`.

`./bin/finux-dev` uses `finux-dev.db` and never your real database, unless you
set `FINUX_DB`. The `seed` command always uses `finux-dev.db`. The production
build does not have the `seed` command. The path of `finux-dev.db` is fixed in
the binary at build time: build again if you move the project.

`make prod` uses Go's default optimizations, strips debug symbols, and omits
local source paths. Both commands only build and keep separate binaries.
Running `make` without a target builds development mode.

## Database

Run `finux init` one time before the other commands:

```sh
./bin/finux init
```

`init` creates the database file if it does not exist, then applies the database
migrations. The first setup also adds the base categories. Run it again after an
update of Finux; it keeps your data. Other
commands do not create or migrate the database. Without a database, they stop
and tell you to run `finux init`.

The database is a SQLite file. Its path is the value of the `FINUX_DB`
environment variable. Without `FINUX_DB`, the path is `~/.finux/finux.db`.
The development build uses `finux-dev.db` in the project root instead; see
[Build and run](#build-and-run).

`finux config doctor` reports if the database exists and if it has all the migrations.
It does not change the database. Its exit status is 1 when a check fails:

```text
✓ Database found: /home/you/.finux/finux.db
✓ Migrations up to date
```

`init` adds the base categories one time only. A base category that you delete
does not come back on the next `init`. To get one back, add it again:

```sh
./bin/finux ct add --name Other --code OTHER --color grey
```

The development build also has `reseed`, which adds all the base categories
that are missing. It finds them by code and does not change the categories you
have:

```sh
./bin/finux-dev reseed
```

## Settings

`finux config` opens a screen where you change the settings of Finux: the up
and down arrows move, the left and right arrows or the space bar change a
value, Enter saves and Esc leaves without a change. A setting that is text,
for example the Telegram token, has a field: type or paste its value, Ctrl+U
clears it. The screen shows `(set)` for a saved token, never the token. In
`AI model`, the left and right arrows go through the usual models of the AI
provider on the screen; you can also type the name of another model. The
screen does not save a value that is not correct. The settings are in the
database.

| Setting | Values | Meaning |
| --- | --- | --- |
| `automatic-schedules` | `off` (default), `on` | On: a schedule with `--auto` creates its transactions on their dates, without your confirmation. Off: no schedule creates a transaction by itself |
| `ai-provider` | `none` (default), `claude`, `codex`, `opencode` | The AI agent program that answers `/ai` in the Telegram bot. `finux config ai <agent>` sets it. None: `/ai` is off |
| `ai-model` | text (default empty) | The model that the agent uses for `/ai`, for example `opus`. Empty: the default model of the agent, which is `sonnet` for `claude`, `gpt-6.2-sol` for `codex` and `opencode/claude-sonnet-4-6` for `opencode` |
| `telegram-token` | text | The token of your Telegram bot. Finux never shows it: `config get` prints `(set)` or `(not set)` |
| `telegram-reminders` | `off` (default), `on` | On: the daemon sends reminders to the allowed people in Telegram |
| `telegram-resume` | `off` (default), `daily`, `monthly` | The daemon sends the resume in Telegram each day, or on day 1 of each month |
| `telegram-resume-time` | text, `HH:MM` (default `08:00`) | The time when the daemon sends the resume |
| `daemon-mcp` | text, `host:port` (default empty) | The daemon serves the MCP server on this address. Empty: it does not |

```sh
./bin/finux config                              # the settings screen
./bin/finux config get                          # all settings
./bin/finux config get ai-provider              # one value
./bin/finux config set ai-provider claude
./bin/finux config set automatic-schedules on
./bin/finux config set telegram-resume-time 07:30
./bin/finux config set daemon-mcp ""            # an empty text clears a text setting
```

The settings screen shows all the settings.

Without a terminal, `finux config` shows its help. `finux config` also has
the commands that check Finux and connect it to AI agents:

```text
finux config doctor     Check the database
finux config ai         Connect Finux to an AI agent: its MCP server and its skills
finux config telegram   Set up the Telegram bot and its allowed people
finux config mcp        Run the MCP server for AI agents
finux config skills     List, install or remove the skills for AI agents
```

## Accounts

`finux accounts` (short form: `finux ac`) manages the accounts that hold your
money.

```text
finux ac                             List accounts; select one to see its details
finux ac list --all                  The same, with archived accounts
finux ac <code|id>                   Show the details of an account
finux ac add [options]               Create an account
finux ac edit [code|id] [options]    Change the name, code, description or color
finux ac delete [code|id]            Archive an account
finux ac delete [code|id] --hard     Delete an account permanently
finux ac restore [code|id]           Restore an archived account
finux ac help                        Show all commands and options
```

Short forms: `l` for `list`, `a` for `add`, `e` for `edit`, `d` for `delete` and
`r` for `restore`. All modules show their help with `help`, `--help` or `-h`.

`finux ac` shows 5 accounts at a time. Type in the search input to find an
account by name or code, move with the arrow keys and press Enter to see its
details. A command name wins over an account code: use `finux ac show L` for an
account with the code `L`.

An account has an id, a name, an optional description, a code, a color, a
currency and an opening balance.

- **Code**: 1 to 5 letters and digits, with at least one letter, for example
  `INTER`. Finux suggests a code from the name (`Nubank` gives `NUBAN`,
  `Banco Inter` gives `BINTE`) and you can change it. Each code is unique
  across accounts and credit cards. An archived account keeps its code.
- **Currency**: `USD`, `EUR`, `BRL` or `BTC`. It does not change after creation.
- **Opening balance**: set on creation only. Use `1234.56` or `1234,56`; negative
  values are permitted. The balance of an account is its opening balance plus
  its transactions that have a date up to today. A transaction with a future
  date changes the balance when its day comes.
- **Color**: one of 16 colors: red, orange, amber, yellow, lime, green, emerald,
  teal, cyan, sky, blue, indigo, violet, purple, pink, slate.

`finux ac add` and `finux ac edit` open a form. Options such as `--name`,
`--code`, `--description`, `--currency`, `--balance` and `--color` give the
values on the command line. With `--name` and `--currency`, `add` does not open
the form:

```sh
./bin/finux ac add --name "Banco Inter" --currency BRL --balance 1234,56 --color orange
./bin/finux ac BINTE
./bin/finux ac e BINTE --code INTER
./bin/finux ac d INTER --yes
```

`edit`, `delete` and `restore` accept a code or an id. Without one, they open
the same selector as `finux ac`. `delete` asks for confirmation unless you give
`--yes`.

An archived account keeps its transactions, but takes no new ones.
`delete --hard` also deletes the transactions of the account. For each transfer
of the account, the transaction in the other account stays: it becomes an
expense or an income of the category `OTHER`, with a note in its description.
A credit card bill that the account paid stays paid.

When input or output is not a terminal, Finux does not open forms, selectors or
prompts: give the values as options. `finux ac` then prints a plain table, and
details are plain text without borders.

## Categories

`finux categories` (short form: `finux ct`) manages the categories of your
money movements.

```text
finux ct                             List categories; select one to see its details
finux ct <code|id>                   Show the details of a category
finux ct add [options]               Create a category
finux ct edit [code|id] [options]    Change the name, code, description or color
finux ct delete [code|id]            Delete a category permanently
finux ct help                        Show all commands and options
```

Short forms: `l` for `list`, `a` for `add`, `e` for `edit` and `d` for `delete`.

A category has an id, a name, an optional description, a code and a color. The
code follows the same rules as an account code. The color gives the type of the
category:

- `red`: expense
- `green`: income
- `grey`: neutral, for example a transfer between your accounts

The base categories are Food, Housing, Transport, Health, Education, Leisure,
Shopping, Bills and Taxes (red); Salary, Freelance and Investments (green);
Transfer and Other (grey).

The screens are the same as in accounts: a list with a search input, and a form
for `add` and `edit`. The options are `--name`, `--code`, `--description` and
`--color`. With `--name` and `--color`, `add` does not open the form:

```sh
./bin/finux ct add --name "Pet Care" --color red
./bin/finux ct PCARE
./bin/finux ct e PCARE --code PETS
./bin/finux ct d PETS --yes
```

Delete is permanent; categories have no archive. `delete` asks for confirmation
unless you give `--yes`. The transactions of a deleted category go to the
category `OTHER`. `OTHER` cannot be deleted while it has transactions.

The color limits the transactions of a category: red is for expenses, green for
incomes and grey for all types. Finux refuses a new color that the transactions
of the category do not accept.

## Transactions

`finux transactions` (short form: `finux tx`) manages the movements of money in
your accounts.

```text
finux tx                         List transactions; select one to see its details
finux tx [filters]               The same, for the transactions the filters select
finux tx <id>                    Show the details of a transaction
finux tx add [options]           Create a transaction
finux tx edit [id] [options]     Change a transaction
finux tx delete [id]             Delete a transaction permanently
finux tx help                    Show all commands and options
```

A transaction has a status. It is `pending` while its date is after today:
the transaction is not in the balance of its account yet. On its date it
becomes `cleared`. Finux keeps the status in the database and changes it when
the date comes. `finux tx <id>` shows it.

Short forms: `l` for `list`, `a` for `add`, `e` for `edit` and `d` for `delete`.

A transaction has an id, a title (at most 60 characters), an optional
description (at most 200 characters), a value, a type, a category, tags, an
account and a date.

- **Type**: `expense`, `income` or `transfer`. A red category makes an expense
  and a green category makes an income, so `--type` is necessary only with a
  grey category. A fourth type, `payment`, comes only from the payment of a
  credit card bill.
- **Account**: an account or a credit card, never the two. `--account` accepts
  the code of a credit card; see [Credit cards](#credit-cards).
- **Value**: a positive amount in the currency of the account. The type gives
  the direction of the money.
- **Category**: required. An expense uses a red or grey category, an income a
  green or grey one, and a transfer a grey one.
- **Tags**: words such as `#market #weekly`: letters, digits and `_`, in
  lowercase. The `#` is optional on the command line; a shell reads a bare `#`
  as a comment, so write `--tags "market weekly"`.
- **Date**: `DD/MM/YYYY` or `YYYY/MM/DD`. The default is today. Finux shows
  dates as `DD/MM/YYYY`.

```sh
./bin/finux tx add --title "Weekly market" --value 50,25 --account INTER --category FOOD --tags "market weekly"
./bin/finux tx add --title Salary --value 3000 --account INTER --category SAL --date 05/11/2026
./bin/finux tx 1
./bin/finux tx e 1 --value 60 --date 2026/11/18
./bin/finux tx d 1 --yes
```

With `--title`, `--value`, `--account` and `--category`, `add` does not open the
form. The form suggests the titles and the tags that other transactions have:
type `wee` and the title shows `Weekly market`; type `#mar` and the tags show
`#market`. Press Tab to accept the suggestion, the up and down arrows to see
other suggestions, or continue to type a different value.

### Installments

An expense or an income can be in installments: monthly parts of one purchase.
Finux creates one transaction for each part.

```sh
./bin/finux tx add --title Notebook --value 1200 --installments 10 --account VISA --category SHOP
./bin/finux tx add --title Sofa --installment-value 150 --installments 6 --account INTER --category HOME
./bin/finux tx add --title Phone --installment-value 99,90 --installments 12 --from 4 --account VISA --category SHOP
./bin/finux tx edit 14 --value 125               # one part
./bin/finux tx edit 14 --all --category EDU      # all the parts
./bin/finux tx delete 14 --next --yes            # part 14 and the parts after it
```

- `--value` is the total. The parts are equal, and the cents that remain go to
  the first part. `--installment-value` gives the value of one part instead.
- `--date` is the date of the first part. Each part after it is one month
  later, on the same day; a short month uses its last day.
- `--from 4` is for a purchase that is already in progress: Finux creates the
  parts 4 to the last one, and `--date` is the date of part 4.
- On a credit card, each part goes to the bill after the one before. All the
  parts use the limit of the card from the first day.
- On an account, a part changes the balance on its date.
- Each part counts in the budgets and in `finux resume` in the month of its
  date.
- The lists show the part after the title: `Notebook 3/10`.

`edit` and `delete` change one part. `--all` changes all the parts and `--next`
changes the part and the parts after it. With `--all` or `--next`, `--value`
is the new total of those parts, `--installment-value` is the new value of each
one, and `--date` is the new date of the first of them. The number of parts
does not change, and the parts that stay keep their numbers. A part in a paid
bill stops a change with `--all` or `--next`: use `--next` from a part in a
bill that is not paid.

An installment is from 2 to 120 parts. A transfer has no installments.

### Transfers

A transfer between accounts is two transactions that point to each other: one
takes the money out of an account and one puts it in the other account.

```sh
./bin/finux tx add --title "Card payment" --value 200 --account INTER --to NU
./bin/finux tx add --title "Travel money" --value 550 --account INTER --to WISE --received 100
```

`--to` makes a transfer. Its category is `XFER` unless you give one. When the
two accounts have different currencies, `--received` gives the amount that the
other account receives.

A change or a delete of one of the two transactions also changes or deletes the
other. Between two currencies, a new value changes only its transaction. A
transfer keeps its type and its accounts: to change them, delete the transfer
and create a new one.

### Filters

```sh
./bin/finux tx --category FOOD
./bin/finux tx --tag market
./bin/finux tx --search market
./bin/finux tx --date 2026            # a year
./bin/finux tx --date 11/2026         # a month; 2026/11 is the same
./bin/finux tx --date 17/11/2026      # a date
./bin/finux tx --account INTER
./bin/finux tx --type expense
./bin/finux tx --virtual              # made with a virtual card; --virtual=false gives the others
```

The interactive list reads like a bank statement: one line for each transaction
under a heading for its day, the amounts in one column, and at the top the
totals of the list for each currency (what came in, what went out and the
difference; transfers and payments of credit card bills do not count).

You can use many filters in one command. In the interactive list, the search
input finds transactions by title and also reads filters: `#tag`, `@ACCOUNT`,
`cat:CODE`, `type:TYPE` and a date such as `2026`, `2026-11`, `11/2026` or
`17/11/2026`. For example: `market #weekly @inter 2026-11`.

## Credit cards

`finux credit-card` (short form: `finux cc`) manages your credit cards and
their bills.

```text
finux cc                             List credit cards; select one to see its details
finux cc list --all                  The same, with archived credit cards
finux cc <code|id>                   Show a credit card and the transactions of its open bill
finux cc add [options]               Create a credit card
finux cc edit [code|id] [options]    Change a credit card
finux cc bills [code|id]             List the bills of a credit card
finux cc pay [code|id] [options]     Pay a bill from an account
finux cc unpay [code|id]             Delete the payment of a bill
finux cc delete [code|id]            Archive a credit card
finux cc delete [code|id] --hard     Delete a credit card permanently
finux cc restore [code|id]           Restore an archived credit card
finux cc help                        Show all commands and options
```

Short forms: `l` for `list`, `a` for `add`, `e` for `edit`, `b` for `bills`,
`p` for `pay`, `d` for `delete` and `r` for `restore`.

A credit card has an id, a name, an optional description, a code, a color, a
currency, a limit, a closing day and a due day. It has no opening balance: it
starts empty.

- **Code**: same rules as an account code. An account and a credit card cannot
  have the same code, because transactions use the code for the two.
- **Currency**: it does not change after creation.
- **Limit**: the credit limit. The details show the used part (the total of the
  bills that are not paid) and the available part.
- **Closing day** and **due day**: days of the month, 1 to 31. A month with
  fewer days uses its last day. New days apply to the bills that do not exist
  yet.

```sh
./bin/finux cc add --name "Visa Gold" --code VISA --currency BRL --limit 5000 --closing 25 --due 5
./bin/finux cc VISA
./bin/finux cc e VISA --limit 8000
```

With `--name`, `--currency`, `--limit`, `--closing` and `--due`, `add` does not
open the form.

### Transactions of a credit card

A transaction has an account or a credit card, never the two. Give the code of
the card to `--account`:

```sh
./bin/finux tx add --title Market --value 100 --account VISA --category FOOD
./bin/finux tx --account VISA
```

A credit card has expenses (purchases) and incomes (refunds). It has no
transfers. Its transactions do not change the balance of an account.

When you pay with a virtual card of the credit card, add `--virtual`:

```sh
./bin/finux tx add --title Streaming --value 40 --account VISA --category FUN --virtual
./bin/finux tx edit 7 --virtual=false     # take the mark away
./bin/finux tx --virtual                  # only these transactions
```

The lists show `[virtual]` after the title, and `finux cc VISA` shows the
total of these transactions in the open bill. The form of `tx add` has the
field `Virtual card` when the account is a credit card, and the search of the
interactive list reads `virtual:yes` and `virtual:no`. Only a transaction of
a credit card can have the mark: one that goes to an account loses it. With
installments, all the parts have the mark.

### Bills

Each transaction of a credit card goes to a bill. Finux finds the bill from the
date of the transaction and creates it when necessary: a transaction on or
before the closing day goes to the bill that closes in that month, and a later
transaction goes to the next bill. A bill has the name of the month it is due.
With closing day 25 and due day 5, a purchase on 17/11/2026 is in the bill
`12/2026`, which closes on 25/11/2026 and is due on 05/12/2026.

A bill has one of four statuses:

- `open`: the closing date did not pass; the bill receives transactions.
- `closed`: the closing date passed; the bill waits for its payment.
- `late`: the due date passed without a payment.
- `paid`: the bill has a payment.

Finux keeps the status in the database and changes it when a date passes:
each command does this before it runs, and the [daemon](#daemon) does it also
while no command runs.

`finux cc <code>` shows the transactions of the open bill, and one line for
each bill that is closed or late. `finux cc bills <code>` lists all the bills.

### Payments

```sh
./bin/finux cc pay VISA --from INTER                 # the total of the oldest closed or late bill
./bin/finux cc pay VISA --from INTER --value 600     # a part of the total
./bin/finux cc pay VISA --from INTER --bill 11/2026 --date 03/11/2026
./bin/finux cc unpay VISA
```

`pay` creates a transaction of the type `payment` in the account, with the
category `XFER`. It takes the value out of the account, but it is not an
expense in the totals: the purchases already are. The account must have the
currency of the card. An open bill cannot be paid.

With a value smaller than the total, the bill is paid and the remaining value
goes to the next bill that is not paid, as "Carried from the bill before".
Finux does not calculate interest.

A paid bill is locked: Finux refuses a new transaction in it, and a change or
a delete of one of its transactions. To change them, delete the payment first
with `finux cc unpay` or with `finux tx delete` of the payment transaction.
The bill opens again and the carried value comes back. A payment cannot be
edited.

### Archive and delete

`delete` archives a credit card, as in accounts: it keeps its bills and its
transactions, but takes no new ones. `delete --hard` deletes the card, its
bills and its transactions permanently. Each payment stays in its account: it
becomes an expense of the category `OTHER`, with a note in its description.

## Goals

A goal is an amount of money to reach: to save, to pay off, or to put
aside for a purchase. Its progress comes from the transactions you link to
it.

```sh
./bin/finux goals                 # list goals; alias: gl
./bin/finux gl add --title "Trip to Japan" --type save --target 10000 \
  --currency BRL --due 30/06/2027 --code TRIP
./bin/finux gl TRIP               # progress and linked transactions
./bin/finux gl edit TRIP --target 12000 --due none
./bin/finux gl delete TRIP        # archive; --hard deletes permanently
./bin/finux gl restore TRIP
```

A goal has a title, a code, a description, a type, a target, one currency
and an optional final date. Without options, `add` and `edit` open a form.
The currency does not change after creation.

### Link transactions

```sh
./bin/finux tx add --title Bonus --value 400 --account NU --category SAL --goal TRIP
./bin/finux tx edit 12 --goal TRIP    # link a transaction that exists
./bin/finux tx edit 12 --goal none    # take the goal away
./bin/finux tx list --goal TRIP
```

The transaction form has a `Goal` field when you have goals. In the list,
the search input also reads `goal:CODE`.

- A transaction has at most one goal, and the goal must have the currency
  of its account or credit card.
- The type of the goal tells how a transaction counts:

  | Type | Income | Expense | Transfer |
  | --- | --- | --- | --- |
  | `save`, `purchase` | adds | removes | adds one time |
  | `payoff` | removes | adds | adds one time |

  `purchase` counts as `save`; it only tells what the money is for.
- The progress is never below zero.
- The payment of a credit card bill (`finux cc pay`) cannot have a goal.

### Status, archive and delete

A goal is `active`, then `reached` when the progress is the target or
more. A goal with a final date in the past that is not reached is
`overdue`.

`delete` archives a goal: it leaves the list and takes no new
transactions, but its transactions keep the link. `delete --hard` deletes
the goal permanently. It never deletes a transaction: the linked
transactions stay and only lose the goal.

## Budgets

A budget is a limit of money for some categories in a period. You do not
link a transaction to a budget: every expense and income of its categories
counts by itself, in the period that has its date.

```sh
./bin/finux budgets               # list budgets; alias: bg
./bin/finux bg add --title Food --limit 800 --currency BRL \
  --categories FOOD,FUN --every month --code FOOD
./bin/finux bg FOOD               # current period, its transactions, history
./bin/finux bg show FOOD --period 15/10/2026   # the period of a date
./bin/finux bg edit FOOD --limit 900 --rollover
./bin/finux bg delete FOOD        # archive; --hard deletes permanently
./bin/finux bg restore FOOD
```

A budget has a title, a code, a description, a limit, one currency, a list
of categories and a period. Without options, `add` and `edit` open a form.
The currency does not change after creation.

### Period

| Options | Budget |
| --- | --- |
| `--every month` | Recurring: a new period starts when one ends |
| `--every "2 weeks"`, `--every "15 days"` | Recurring, with a number of days, weeks, months or years |
| `--every month --until 31/12/2027` | Recurring until a date |
| `--until 31/12/2026` | One time: one period from the start date to that date |

The periods count from the start date (`--start`). A monthly budget that
starts on 05/11/2026 has periods from day 5 to day 4. Without `--start`, a
budget of one week, month or year starts on the first day of this week
(Monday), month or year; any other budget starts today.

### What counts

- An expense adds to what the period spent. An income gives money back.
  The spent amount is never below zero.
- Only the categories of the budget count. Without `--categories`, or with
  `--categories all`, all categories count but the income (green) ones.
- Only transactions in the currency of the budget count.
- A transfer and the payment of a credit card bill (`finux cc pay`) do not
  count. A purchase with a card counts on the day of the purchase.

A period is `ok`, then `near` when it spent 80% of its amount or more, then
`over`. A budget that did not start is `upcoming`; one whose end date
passed is `ended`. `finux tx add` and `finux tx edit` print a line when the
transaction leaves a budget near or over its limit:

```
✓ Transaction 42 created
! Budget FOOD: 85% spent, R$ 120,00 left until 30/11/2026
```

### Rollover

With `--rollover`, a period gets what the period before did not spend, and
loses what it spent too much. The amount carries from the start date of the
budget. `--rollover=false` in `edit` stops it. Without rollover, each period
has the limit.

A change of the limit, the categories or the dates counts all the periods
again: Finux does not keep the old limit of a past period.

### Archive and delete

`delete` archives a budget: it leaves the list and gives no warnings, but it
keeps its history. `delete --hard` deletes the budget permanently. A delete
never changes a transaction.

When you delete a category, it leaves the budgets that have it. Finux
refuses to delete a category that is the only category of a budget: change
or delete the budget first.


## Scheduled transactions

A schedule is a transaction that will happen: a bill to pay or an income to
receive, one time or again and again. It is not a transaction: it does not
change a balance, a bill or a budget until you confirm it.

```sh
./bin/finux scheduled             # list schedules; aliases: schedule, sc
./bin/finux sc add --title Rent --value 1500 --account INTER --category HOME \
  --every month --date 05/12/2026 --code RENT
./bin/finux sc add --title Dentist --value 300 --account VISA --category HEALT \
  --date 10/12/2026                # one time
./bin/finux sc add --title Course --value 200 --account INTER --category EDU \
  --every "2 weeks" --times 6
./bin/finux sc RENT               # what is due, the next date, its transactions
./bin/finux sc pay RENT           # confirm the next occurrence: creates the transaction
./bin/finux sc pay ELEC --value 187,40 --date 12/12/2026   # the real value and date
./bin/finux sc skip RENT          # drop the next occurrence
./bin/finux sc edit RENT --value 1600
./bin/finux sc delete RENT        # archive; --hard deletes permanently
./bin/finux sc restore RENT
```

A schedule has a title, a code, a value, a type (expense or income), an
account or a credit card, a category, and optional tags, description and
goal. `add` needs `--title`, `--value`, `--account` and `--category`.

- **Dates**: `--date` is the first occurrence (default: today). `--every`
  makes the schedule repeat: `day`, `week`, `month`, `year`, or a number of
  them such as `"2 weeks"`. Without `--every` the schedule is for one time.
  The occurrences count from the first date: a monthly schedule from day 31
  uses the last day of a shorter month.
- **End**: `--until <date>` or `--times <number>`. Without them a schedule
  repeats until you archive it.
- **Confirmation**: each occurrence waits until you run `sc pay`, which
  creates its transaction, or `sc skip`. An occurrence is due on its date and
  stays due until then, so a schedule can have many due occurrences; `pay`
  and `skip` take the oldest one first. `pay` can also confirm an occurrence
  before its date.
- **Real values**: `sc pay` uses the value, the date of the occurrence and the
  account of the schedule. `--value`, `--date` and `--account` give the real
  ones, for a bill that changes each month.
- **Automatic**: with `--auto`, Finux creates the transaction of each due
  occurrence the next time you run a `finux` command, with the date of the
  occurrence, and tells you. It needs two things: `--auto` on the schedule and
  the setting `automatic-schedules` on (`finux config set automatic-schedules
  on`). The two are off by default, so nothing becomes a transaction by
  itself. `sc edit CODE --auto=false` stops one schedule; the setting stops
  all of them.

`finux tx` tells you when occurrences are due. `finux resume` lists what is
due and what comes until the end of the month, and the balance after them.
A budget shows `Scheduled`: what the schedules of its categories still take
from the current period.

A change with `sc edit` is for the occurrences that wait: the transactions
that the schedule created stay as they are. A new `--date` is the date of the
next occurrence. An archived schedule stops. A delete of a schedule never
deletes a transaction, and a delete of a transaction does not bring its
occurrence back. A category that a schedule has cannot be deleted. A
permanent delete of an account or a credit card also deletes its schedules.

## Resume

The resume is the summary of your finances on the date of today.

```sh
./bin/finux resume                  # the month of today; alias: rs
./bin/finux rs --month 10/2026      # the incomes and expenses of another month
```

For each currency, it shows the incomes, the expenses, the net amount, the
expenses per day and the top expense categories of the month. A transfer and
the payment of a credit card bill do not count: a purchase with a card counts
on the day of the purchase. The expenses per day use the days of the month up
to today.

It also shows the accounts with their balances and a total for each currency,
the credit card bills to pay, the budgets with a period today and the goals.
These are always as they are today, also with `--month`.

## Telegram

Finux has a bot for Telegram, so you and the people you choose use Finux from
a phone. It is your own bot: you create it, and it runs on your machine in
the [daemon](#daemon). Telegram is the only service it talks to.

### Set up

1. In Telegram, talk to `@BotFather` and send `/newbot`. It gives a token.
2. Save the token and start the daemon:

   ```sh
   finux config telegram token 123456789:AAH...   # or paste it in the 'finux config' screen
   finux daemon install        # or 'finux daemon run' in a terminal
   ```

3. Let each person in. The easy way is an invite:

   ```sh
   finux config telegram invite My wife
   ```

   It prints a link to your bot. Send the link to the person, or open it
   yourself. The person presses Start in Telegram and has access.

   The other way starts in Telegram: the person sends a message to the bot,
   and the bot answers with a code. Type the code in the terminal:

   ```sh
   finux config telegram pair 482-177          # the name comes from Telegram
   finux config telegram pair 482-177 My wife  # or give the name
   ```

   A link and a code work one time and for 10 minutes. The bot tells its
   code one time while the code works. Each of the two ways needs a command
   in the terminal, so a person who finds your bot cannot get access alone.
   The bot does nothing more for a person who is not allowed.

4. See and change the allowed people:

   ```sh
   finux config telegram                  # the token state and the allowed people
   finux config telegram deny 654321
   finux config telegram allow 123456 Me  # with the user ID, which the bot also shows
   ```

The `FINUX_TELEGRAM_TOKEN` environment variable replaces the saved token. A
change of the allowed people needs no restart of the daemon; a new token
does. The bot answers only in a private chat, not in a group.

All the allowed people use the same database: yours. Allow only people who
can see and change all your data.

### Commands

The bot has the commands of Finux that read and change data, with the same
options as in the terminal and without the word `finux`:

```text
/rs                    Resume of the month
/tx                    Transactions
/ac                    Accounts
/cc                    Credit cards and bills
/sc                    Scheduled transactions
/bg                    Budgets
/gl                    Goals
/ct                    Categories
/ai                    Ask the AI; see below
/help                  Show the commands
/cancel                Stop the current question
```

```text
/tx add --title "Weekly market" --value 50,25 --account NU --category FOOD
/tx --date 11/2026
/cc bills VISA
/sc pay RENT
/tx --help
```

The long names work too: `/transactions`, `/accounts`, `/credit_card` (Telegram
permits no dash in a command). `config`, `init`, `update` and `daemon` are
only in the terminal. A phone often changes `--` to a long dash and quotes
to curly quotes: the bot reads them as the plain ones.

The bot helps when a command comes without its values:

- An `add` command with no options asks for each value, one question at a
  time, with buttons for the accounts, the categories and the other values
  that have a list. This works for `/tx add`, `/ac add`, `/ct add`,
  `/cc add`, `/gl add`, `/bg add` and `/sc add`. `/cc pay` asks for the
  card, the account and the value. Write an answer or press a button;
  `/cancel` stops the questions. For a transfer, installments or an option
  the questions do not have, give the options in the command.
- A command with no code or id, for example `/tx delete` or `/ac edit`, shows
  the items as buttons.
- A command that deletes data asks first, with the buttons Confirm and
  Cancel. With `--yes` it does not ask.
- `edit` takes its changes as options: `/tx edit 12 --value 60`.

A text that is not a command and not an answer gets: "Sorry, I did not
understand."

### AI

With an AI agent set up, `/ai` sends your message to the AI. The AI has the
Finux tools and skills, so it sees your data and can change it:

```text
/ai how was my month?
/ai should I buy a 3000 phone now or next month?
/ai new what do I owe on my cards?        Start a new conversation
```

Send a file, for example the PDF of a credit card bill or the photo of a
receipt, with `/ai` at the start of its caption: `/ai import this bill`. A
file with no caption waits: the bot keeps it for your next `/ai` message.
The AI asks before it changes data, so answer with one more `/ai` message:
`/ai yes, import it`.

Set it up in the terminal:

```sh
finux config ai claude            # the MCP server, the skills, ai-memory and ai-provider
finux config set ai-model opus    # optional: the default for claude is sonnet
finux daemon install              # again, so the service finds the agent and shows /ai
```

- Each person has one conversation: a message continues the one before.
  `/ai new` starts again. `/cancel` stops an answer in progress. An answer
  has 10 minutes.
- Finux starts the agent program for each message, with no screen, in the
  folder `~/.finux/ai`. The agent gets the Finux tools, the ai-memory tools,
  the skills and read access to the files you sent. It writes no file.
- The limits are different for each agent. Claude Code has no shell and
  reads only the files you sent. Codex has a shell in a sandbox with no
  write and no network, so it can read all the files of your user: think
  about this before you let other people use `/ai`. Codex runs without
  your `~/.codex/config.toml`: it gets only the `finux` and `ai-memory`
  servers, with no plugins, no hooks and no web search. With `opencode` as
  `ai-provider`, `/ai` tells that it cannot answer.
- The files are in `~/.finux/ai/inbox/<user id>`, and only your user can
  read them. The daemon deletes a file after 7 days. Telegram gives a bot
  files of 20 MB or less.
- With [ai-memory](https://github.com/akitaonrails/ai-memory) connected, the
  AI saves what you decide and reads it in later conversations. All the
  `/ai` conversations are one ai-memory project, `finux-finances`.
- A new conversation loads all that your agent has: its tools, its skills
  and its plugins. That can be many tokens for each new conversation. To pay
  less, keep a small model in `ai-model` and do not use `/ai new` without a
  reason. Without `ai-model`, Claude Code uses `sonnet`.
- `/ai` uses the login of the agent program on your machine. All the allowed
  people use it.

## Daemon

The daemon is a Finux process that stays alive. It is optional: all the
commands work without it.

```text
finux daemon run          Run the daemon in this terminal, until Ctrl+C
finux daemon install      Make the daemon a service: it starts now and at each login
finux daemon uninstall    Stop the service and remove it
finux daemon start        Start the service
finux daemon stop         Stop the service
finux daemon restart      Start the service again
finux daemon status       Show if the service runs
finux daemon logs         Show the last lines of the log; --follow keeps it open
```

While the daemon runs, it does this:

- It runs the [Telegram bot](#telegram), when a token is set.
- It keeps the status of the bills and the transactions current.
- It creates the transactions of the automatic schedules on their dates,
  when the setting `automatic-schedules` is on. Without the daemon, this
  happens the next time you run a command.
- With `telegram-reminders` on, it tells the allowed people in Telegram about
  a schedule that is due, a bill that closed, a bill that is due today or
  tomorrow, a late bill, a budget near or over its limit, and a transaction
  that an automatic schedule created. Each reminder comes one time.
- With `telegram-resume` on `daily` or `monthly`, it sends the output of
  `finux resume` at the time in `telegram-resume-time`. Monthly is on day 1.
- With an address in `daemon-mcp`, it serves the
  [MCP server](#http) there, with all its tools. For an address that other
  computers can reach, set the `FINUX_MCP_TOKEN` environment variable.

```sh
finux config set telegram-reminders on
finux config set telegram-resume daily
finux config set daemon-mcp 127.0.0.1:8765
finux daemon restart
```

The daemon reads the token of the bot and `daemon-mcp` when it starts, so
start it again after you change them. The other settings need no restart.

The service is a systemd user unit, `~/.config/systemd/user/finux.service`.
`finux daemon install` writes it for the binary that runs the command, and
for the database in `FINUX_DB` when that variable is set. Run it again after
you move the binary. `finux update` starts the service again by itself. The
service runs while you are logged in; to keep it running after you log out:

```sh
loginctl enable-linger
```

Without systemd, run `finux daemon run` with the tool that starts your
programs. A development build has no service: use `./bin/finux-dev daemon run`.

## AI agents

One command connects Finux to an AI agent:

```sh
finux config ai claude      # Claude Code
finux config ai codex       # Codex
finux config ai opencode    # opencode
finux config ai             # choose the agent from a list
```

It does the steps of the setup:

1. It adds the Finux [MCP server](#mcp-server-for-ai-agents) to the agent.
2. It installs the [skills](#skills-for-ai-agents) in the skills folder of
   the agent.
3. It connects [ai-memory](https://github.com/akitaonrails/ai-memory) to the
   agent with `ai-memory setup`, so the AI keeps what it learns and what you
   decide between sessions.
4. It saves the agent in the setting `ai-provider`: the agent then answers
   [`/ai` in the Telegram bot](#ai).

Then start a new session of the agent.

- For Claude Code and Codex it runs their own command (`claude mcp add`,
  `codex mcp add`), so the agent must be installed. For opencode it writes
  `~/.config/opencode/opencode.json`; it keeps the other values of the file
  and a copy of the file as it was, `opencode.json.backup`.
- `--read-only` gives the agent the tools that read your data, not the ones
  that change it.
- When the ai-memory program is not installed, the command asks before it
  runs the install script of the ai-memory project, which checks the release
  against its checksum. Without a terminal it installs only with `--yes`.
  `--no-memory` skips ai-memory.
- Run it again after you update Finux or move its binary: it replaces the
  server and the skills.

The next two sections tell what the first two parts are and how to set up
each one by hand.

## MCP server for AI agents

`finux config mcp` is a [Model Context Protocol](https://modelcontextprotocol.io)
server. It gives an AI agent, such as Claude Code, Codex or opencode, the
features of the CLI as tools. The server uses only the Go standard library,
and the same database as the CLI.

### Install

`finux config mcp config <agent>` shows the command or the configuration for an
agent, with the path of the binary you ran. It does not change the files of
the agent.

```sh
./bin/finux config mcp config claude    # prints a 'claude mcp add' command
./bin/finux config mcp config codex     # prints a block for ~/.codex/config.toml
./bin/finux config mcp config opencode  # prints a block for opencode.json
```

Add `--read-only` to give the agent only the tools that change nothing.
To use a different database, set `FINUX_DB` in the environment of the
server, in the configuration of the agent.

### HTTP

Without options, the agent starts the server and talks to it through
standard input and output. With `--http`, the server is one process that
you start, and agents connect to its URL:

```sh
./bin/finux config mcp --http 127.0.0.1:8765
# Finux MCP server listens on http://127.0.0.1:8765/mcp (no token)

./bin/finux config mcp config claude --url http://127.0.0.1:8765/mcp
```

This is the Streamable HTTP transport of the protocol: the agent sends each
message as a `POST` to `/mcp` and gets the response as JSON. The server has
no sessions and no event stream. Press `ctrl+c` to stop it.

To protect the server, set a token in the `FINUX_MCP_TOKEN` environment
variable before you start it. Each request then needs the header
`Authorization: Bearer <token>`. With the variable set, `config --url`
also shows how the agent sends the token; it shows the name of the
variable, not the token.

```sh
export FINUX_MCP_TOKEN="$(openssl rand -hex 32)"
./bin/finux config mcp --http 0.0.0.0:8765
```

- An address that other computers can reach, such as `0.0.0.0:8765`, is
  refused without a token.
- The server refuses a request from a web page of a different host, so a
  page in your browser cannot call it.
- The server does not encrypt. To use it from a different computer, put it
  behind a tunnel or a reverse proxy that has HTTPS.

### Tools

Each tool returns JSON. The tools with `*` need `confirm: true`.

| Module | Read | Change |
| --- | --- | --- |
| Database | `database_doctor` | `database_init`, `database_reseed` |
| Accounts | `account_list`, `account_show` | `account_add`, `account_edit`, `account_archive`, `account_restore`, `account_delete`* |
| Categories | `category_list`, `category_show` | `category_add`, `category_edit`, `category_delete`* |
| Transactions | `transaction_list`, `transaction_show`, `transaction_suggestions` | `transaction_add`, `transaction_edit`, `transaction_delete`* |
| Credit cards | `credit_card_list`, `credit_card_show`, `credit_card_bills` | `credit_card_add`, `credit_card_edit`, `credit_card_pay`, `credit_card_unpay`*, `credit_card_archive`, `credit_card_restore`, `credit_card_delete`* |
| Goals | `goal_list`, `goal_show` | `goal_add`, `goal_edit`, `goal_archive`, `goal_restore`, `goal_delete`* |
| Budgets | `budget_list`, `budget_show` | `budget_add`, `budget_edit`, `budget_archive`, `budget_restore`, `budget_delete`* |

The rules are the rules of the CLI, with these differences:

- A tool that deletes data does nothing without `confirm: true`. It returns
  `confirm_required` and a message that tells what it will delete.
- `delete` of the CLI is two tools: `*_archive` archives and `*_delete`
  deletes permanently (`delete --hard`).
- The agent sends money as a decimal string, for example `"1234.56"`. A
  result gives money as `minor` (integer minor units), `decimal` and
  `formatted`.
- The agent sends a date as `YYYY-MM-DD` or `DD/MM/YYYY`. A result gives a
  date as `YYYY-MM-DD` and a bill as `YYYY-MM`.
- `transaction_list` returns at most 100 transactions; `limit` and `offset`
  read the others. Its totals are for all transactions that match the
  filters.
- The development command `seed` is not a tool.

## Skills for AI agents

The folder [skills](skills) has workflows for an agent that manages your
finances with Finux. Each one is a folder with a `SKILL.md` file, in the
Agent Skills format that Claude Code, Codex and opencode read.

| Skill | Use it to |
| --- | --- |
| `finux-log-transactions` | Turn loose text or a receipt into transactions |
| `finux-import-statement` | Import a bank or card statement: CSV, OFX, PDF or pasted text |
| `finux-period-review` | Report a month or a year |
| `finux-credit-card-bills` | See the bills of the credit cards and pay them |
| `finux-coach` | Analyze spending and warn about debts and bills |
| `finux-tidy-transactions` | Find the transactions of an account or a card that are not sure, and fix their description, category and tags |

A skill that changes data shows a draft and waits for your approval. The
skills use the MCP tools; without the MCP server they use the `finux`
CLI. Install the [MCP server](#mcp-server-for-ai-agents) first.

To install the skills, run `finux config skills install` with your agent. The
skills are in the binary, so the command needs no other file:

```sh
./bin/finux config skills                    # list the skills
./bin/finux config skills install claude     # ~/.claude/skills
./bin/finux config skills install codex      # ~/.codex/skills
./bin/finux config skills install opencode   # ~/.config/opencode/skills
./bin/finux config skills install --dir .claude/skills   # a folder you choose
./bin/finux config skills remove claude      # remove them again
```

Run `install` again after you update Finux: it replaces the Finux skills
and keeps all other skills in the folder. When you work on the skills in
this repository, link them in place of the install, and `install` then
leaves the links as they are:

```sh
ln -s "$PWD"/skills/finux-* ~/.claude/skills/
```

Then start a new session of the agent and ask, for example, "I spent 45 on
lunch with Nubank" or "how was my month".

[docs/agents](docs/agents) has the reference the skills use: the data model
and the rules ([concepts.md](docs/agents/concepts.md)), the MCP tools
([tools.md](docs/agents/tools.md)) and the CLI commands for an agent
([access.md](docs/agents/access.md)). Each skill has a copy in its
`references` folder. `tools.md` and the copies are generated: after a
change to a tool or to `docs/agents`, run `make docs`. The tests fail when
they are out of date, or when a skill names a tool that does not exist.

## Development

Follow the TDD and dependency rules in [AGENTS.md](AGENTS.md).
Format Go code with `gofmt`; EditorConfig displays Go tabs at width 2.

ai-memory uses workspace `default`, project `finux`, configured in
`.ai-memory.toml`. Managed routing and skills provide cross-session continuity.
