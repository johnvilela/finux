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

Lip Gloss styles all output. Bubble Tea and Bubbles run the account form, the
account selector and the confirmation prompt. GORM stores accounts in SQLite.
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
  accounts, ac  Manage accounts
  help          Show this help

Options:
  -h, --help    Show this help
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

`make prod` uses Go's default optimizations, strips debug symbols, and omits
local source paths. Both commands only build and keep separate binaries.
Running `make` without a target builds development mode.

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
  values are permitted. The balance of an account is its opening balance until
  transactions exist.
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

When input or output is not a terminal, Finux does not open forms, selectors or
prompts: give the values as options. `finux ac` then prints a plain table, and
details are plain text without borders.

Finux keeps its data in `~/.finux/finux.db`. Set `FINUX_HOME` to use a different
directory.

## Development

Follow the TDD and dependency rules in [AGENTS.md](AGENTS.md).
Format Go code with `gofmt`; EditorConfig displays Go tabs at width 2.

ai-memory uses workspace `default`, project `finux`, configured in
`.ai-memory.toml`. Managed routing and skills provide cross-session continuity.
