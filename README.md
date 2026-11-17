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

Lip Gloss styles the startup screen. Other dependencies are pinned for upcoming
UI and database work. The CLI does not create a database yet.

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
  help        Show this help

Options:
  -h, --help  Show this help
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

## Development

Follow the TDD and dependency rules in [AGENTS.md](AGENTS.md).
Format Go code with `gofmt`; EditorConfig displays Go tabs at width 2.

ai-memory uses workspace `default`, project `finux`, configured in
`.ai-memory.toml`. Managed routing and skills provide cross-session continuity.
