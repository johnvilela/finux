# Finux for AI agents

These files tell an AI agent how to use Finux for the finances of its
user. They are for agents that use Finux, not for agents that change its
code; for the code, see [AGENTS.md](../../AGENTS.md).

| File | Contents |
| --- | --- |
| [concepts.md](concepts.md) | The data model, the rules and the objects in a result |
| [tools.md](tools.md) | The reference of the MCP tools, made from their schemas |
| [access.md](access.md) | How to reach Finux: the MCP server, or the CLI when there is no server |

The folder [skills](../../skills) has the workflows that use these files:

| Skill | Use it to |
| --- | --- |
| `finux-log-transactions` | Turn loose text or a receipt into transactions |
| `finux-import-statement` | Import a bank or card statement: CSV, OFX, PDF or pasted text |
| `finux-period-review` | Report a month or a year |
| `finux-credit-card-bills` | See the bills of the credit cards and pay them |
| `finux-coach` | Analyze spending and warn about debts and bills |

Each skill has a copy of the three reference files in its `references`
folder, so an installed skill works without this repository. `make docs`
writes `tools.md` and the copies again; the tests fail when they are out
of date.
