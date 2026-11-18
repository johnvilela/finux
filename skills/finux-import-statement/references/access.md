# How to reach Finux

Use the MCP tools when the agent has them. Use the CLI when it does not.
The rules are the same: the tools run the code of the CLI.

## 1. MCP tools

Look for tools such as `account_list` in your tool list. An agent can show
them with a prefix, for example `mcp__finux__account_list`. The reference
is [tools.md](tools.md).

When a tool reports that the database is not found or is out of date, call
`database_init`, then try again. It keeps the data.

## 2. CLI

Without the MCP tools, run the `finux` binary in a shell. First run
`finux doctor`; when it fails, `finux init` creates or updates the
database.

Tell the user one time that the MCP server gives better results, and how
to install it: `finux mcp config claude`, `finux mcp config codex` or
`finux mcp config opencode` prints the steps.

Rules for the CLI:

- Give all values as options. Without a terminal, Finux opens no form and
  no prompt; a command that lacks a value fails with a message.
- The output is text, not JSON: tables without borders, and a line that
  starts with `✓` after a change. Amounts are formatted for the currency
  (`R$ 1.234,56`); in a calculation, read them with care.
- A delete needs `--yes`. The CLI then asks nothing, so get a clear yes
  from the user before you run it.
- A failed command writes `finux: ...` to stderr and has an exit status
  that is not 0.
- Write a tag without `#`, or put the tags in quotes: the shell reads `#`
  as a comment.
- `finux <module> --help` shows all options.

| MCP tool | CLI command |
| --- | --- |
| `database_init` | `finux init` |
| `database_doctor` | `finux doctor` |
| `database_reseed` | `finux reseed` |
| `account_list` | `finux ac list [--all]` |
| `account_show` | `finux ac show <code>` |
| `account_add` | `finux ac add --name <text> --currency <code> [--code <code>] [--description <text>] [--balance <amount>] [--color <name>]` |
| `account_edit` | `finux ac edit <code> [--name] [--code] [--description] [--color]` |
| `account_archive` | `finux ac delete <code> --yes` |
| `account_restore` | `finux ac restore <code>` |
| `account_delete` | `finux ac delete <code> --hard --yes` |
| `category_list` | `finux ct list` |
| `category_show` | `finux ct show <code>` |
| `category_add` | `finux ct add --name <text> --color <red\|green\|grey> [--code <code>] [--description <text>]` |
| `category_edit` | `finux ct edit <code> [--name] [--code] [--description] [--color]` |
| `category_delete` | `finux ct delete <code> --yes` |
| `transaction_list` | `finux tx list [--category <code>] [--tag <tags>] [--search <text>] [--date <date>] [--account <code>] [--type <type>]` |
| `transaction_show` | `finux tx show <id>` |
| `transaction_add` | `finux tx add --title <text> --value <amount> --account <code> --category <code> [--type <type>] [--description <text>] [--tags <tags>] [--date <date>] [--to <code>] [--received <amount>]` |
| `transaction_edit` | `finux tx edit <id> [--title] [--description] [--value] [--type] [--category] [--tags] [--account] [--date]` |
| `transaction_delete` | `finux tx delete <id> --yes` |
| `transaction_suggestions` | No command. Read the titles and the tags in the output of `finux tx list` |
| `credit_card_list` | `finux cc list [--all]` |
| `credit_card_show` | `finux cc show <code>` |
| `credit_card_bills` | `finux cc bills <code>` |
| `credit_card_add` | `finux cc add --name <text> --currency <code> --limit <amount> --closing <day> --due <day> [--code <code>] [--description <text>] [--color <name>]` |
| `credit_card_edit` | `finux cc edit <code> [--name] [--code] [--description] [--color] [--limit] [--closing] [--due]` |
| `credit_card_pay` | `finux cc pay <code> --from <account> [--value <amount>] [--date <date>] [--bill <MM/YYYY>]` |
| `credit_card_unpay` | `finux cc unpay <code> [--bill <MM/YYYY>] --yes` |
| `credit_card_archive` | `finux cc delete <code> --yes` |
| `credit_card_restore` | `finux cc restore <code>` |
| `credit_card_delete` | `finux cc delete <code> --hard --yes` |

Differences from the MCP tools:

- `finux tx list` prints all transactions that match; it has no pages and
  no totals. Add the amounts with a script, not by eye.
- The CLI has no step that only tells what a delete will do. Use the list
  and show commands to tell the user what will go, then ask.
