# Finux

A personal finance manager written in Go, starting with a CLI.

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
  doctor            Check the database
  reseed            Add missing base data
  accounts, ac      Manage accounts
  categories, ct    Manage categories
  transactions, tx  Manage transactions
  help              Show this help

Options:
  -h, --help        Show this help
```

The executable stays in the project; it is not installed on your PATH.

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
  base set of 8 accounts and 17 transactions.
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

`finux doctor` reports if the database exists and if it has all the migrations.
It does not change the database. Its exit status is 1 when a check fails:

```text
✓ Database found: /home/you/.finux/finux.db
✓ Migrations up to date
```

`init` adds the base categories one time only. A base category that you delete
does not come back on the next `init`. To add the base categories that are
missing, run:

```sh
./bin/finux reseed
```

`reseed` finds base categories by code and does not change the categories you
have.

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
  `Banco Inter` gives `BINTE`) and you can change it. Each code is unique. An
  archived account keeps its code.
- **Currency**: `USD`, `EUR`, `BRL` or `BTC`. It does not change after creation.
- **Opening balance**: set on creation only. Use `1234.56` or `1234,56`; negative
  values are permitted. The balance of an account is its opening balance plus
  its transactions.
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

Short forms: `l` for `list`, `a` for `add`, `e` for `edit` and `d` for `delete`.

A transaction has an id, a title (at most 60 characters), an optional
description (at most 200 characters), a value, a type, a category, tags, an
account and a date.

- **Type**: `expense`, `income` or `transfer`. A red category makes an expense
  and a green category makes an income, so `--type` is necessary only with a
  grey category.
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
```

The interactive list reads like a bank statement: one line for each transaction
under a heading for its day, the amounts in one column, and at the top the
totals of the list for each currency (what came in, what went out and the
difference; transfers do not count).

You can use many filters in one command. In the interactive list, the search
input finds transactions by title and also reads filters: `#tag`, `@ACCOUNT`,
`cat:CODE`, `type:TYPE` and a date such as `2026`, `2026-11`, `11/2026` or
`17/11/2026`. For example: `market #weekly @inter 2026-11`.

## Development

Follow the TDD and dependency rules in [AGENTS.md](AGENTS.md).
Format Go code with `gofmt`; EditorConfig displays Go tabs at width 2.

ai-memory uses workspace `default`, project `finux`, configured in
`.ai-memory.toml`. Managed routing and skills provide cross-session continuity.
