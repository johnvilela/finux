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
`finux config doctor`; when it fails, `finux init` creates or updates the
database.

Tell the user one time that the MCP server gives better results, and how
to install it: `finux config ai claude`, `finux config ai codex` or
`finux config ai opencode` adds the server and the skills in one step.

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
| `database_doctor` | `finux config doctor` |
| `database_reseed` | No command. Add a base category that is missing with `finux ct add`, for example `finux ct add --name Other --code OTHER --color grey` |
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
| `transaction_list` | `finux tx list [--category <code>] [--tag <tags>] [--search <text>] [--date <date>] [--account <code>] [--type <type>] [--goal <code>] [--virtual[=false]]` |
| `transaction_show` | `finux tx show <id>` |
| `transaction_add` | `finux tx add --title <text> --value <amount> --account <code> --category <code> [--type <type>] [--description <text>] [--tags <tags>] [--date <date>] [--to <code>] [--received <amount>] [--goal <code>] [--installments <number>] [--installment-value <amount>] [--from <number>] [--virtual]` |
| `transaction_edit` | `finux tx edit <id> [--title] [--description] [--value] [--type] [--category] [--tags] [--account] [--date] [--goal <code\|none>] [--virtual[=false]] [--installment-value <amount>] [--all\|--next]` |
| `transaction_delete` | `finux tx delete <id> [--all\|--next] --yes` |
| `transaction_suggestions` | No command. Read the titles and the tags in the output of `finux tx list` |
| `transaction_corrections` | No command. The corrections are for the MCP tools only |
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
| `goal_list` | `finux gl list [--all]` |
| `goal_show` | `finux gl show <code>` |
| `goal_add` | `finux gl add --title <text> --type <save\|payoff\|purchase> --target <amount> --currency <code> [--due <date>] [--code <code>] [--description <text>]` |
| `goal_edit` | `finux gl edit <code> [--title] [--code] [--description] [--type] [--target] [--due <date\|none>]` |
| `goal_archive` | `finux gl delete <code> --yes` |
| `goal_restore` | `finux gl restore <code>` |
| `goal_delete` | `finux gl delete <code> --hard --yes` |
| `budget_list` | `finux bg list [--all]` |
| `budget_show` | `finux bg show <code> [--period <date>] [--history <number>]` |
| `budget_add` | `finux bg add --title <text> --limit <amount> --currency <code> [--categories <codes\|all>] [--every <period>] [--start <date>] [--until <date>] [--rollover] [--code <code>] [--description <text>]` |
| `budget_edit` | `finux bg edit <code> [--title] [--code] [--description] [--limit] [--categories <codes\|all>] [--every <period\|none>] [--start <date>] [--until <date\|none>] [--rollover[=false]]` |
| `budget_archive` | `finux bg delete <code> --yes` |
| `budget_restore` | `finux bg restore <code>` |
| `budget_delete` | `finux bg delete <code> --hard --yes` |
| `schedule_list` | `finux sc list [--all]` |
| `schedule_show` | `finux sc show <code>` |
| `schedule_add` | `finux sc add --title <text> --value <amount> --category <code> [--account <code>] [--currency <code>] [--type <type>] [--date <date>] [--every <period>] [--until <date>] [--times <number>] [--auto] [--tags <tags>] [--goal <code>] [--code <code>] [--description <text>]` |
| `schedule_edit` | `finux sc edit <code> [--title] [--code] [--description] [--value] [--account <code\|none>] [--currency] [--category] [--type] [--date] [--every <period\|none>] [--until <date\|none>] [--times <number\|none>] [--auto[=false]] [--tags] [--goal <code\|none>]` |
| `schedule_pay` | `finux sc pay <code> [--value <amount>] [--date <date>] [--account <code>]` |
| `schedule_skip` | `finux sc skip <code>` |
| `schedule_archive` | `finux sc delete <code> --yes` |
| `schedule_restore` | `finux sc restore <code>` |
| `schedule_delete` | `finux sc delete <code> --hard --yes` |

Differences from the MCP tools:

- `finux tx list` prints all transactions that match; it has no pages and
  no totals. Add the amounts with a script, not by eye.
- The CLI has no step that only tells what a delete will do. Use the list
  and show commands to tell the user what will go, then ask.
