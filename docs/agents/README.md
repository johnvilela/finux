# Finux for AI agents

These files tell an AI agent how to use Finux for the finances of its
user. They are for agents that use Finux, not for agents that change its
code; for the code, see [AGENTS.md](../../AGENTS.md).

| File | Contents |
| --- | --- |
| [concepts.md](concepts.md) | The data model, the rules and the objects in a result |
| [tools.md](tools.md) | The reference of the MCP tools, made from their schemas |
| [access.md](access.md) | How to reach Finux: the MCP server, or the CLI when there is no server |
| [names.md](names.md) | How an agent learns the names and the tags of the user, and keeps them in ai-memory |

The folder [skills](../../skills) has the workflows that use these files:

| Skill | Use it to |
| --- | --- |
| `finux-log-transactions` | Turn loose text or a receipt into transactions |
| `finux-import-statement` | Import a bank or card statement: CSV, OFX, PDF or pasted text |
| `finux-period-review` | Report a month or a year |
| `finux-credit-card-bills` | See the bills of the credit cards and pay them |
| `finux-coach` | Analyze spending and warn about debts and bills |
| `finux-notes` | Write short notes with an optional reminder, tell the due ones and mark them as done |
| `finux-tidy-transactions` | Find the transactions of an account or a card that are not sure, and fix their name, description, category and tags |

Each skill has a copy of the first three reference files in its
`references` folder, and the skills that give a title to a transaction
also have a copy of `names.md`, so an installed skill works without this repository. `make docs`
writes `tools.md` and the copies again; the tests fail when they are out
of date.
