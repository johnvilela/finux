---
name: finux-notes
description: Writes and reads the notes in Finux, the personal finance manager of the user: short texts about the finances, with an optional reminder date, tags and links to a goal, a budget, an account or a credit card. Use it for "note this", "remember that", "remind me to", "write down that I decided", "what are my notes", "what must I not forget", "what did I decide about", "mark that note as done", "I already did that". It writes each note short and plain, tells the user which notes are due, and marks a note as done when it is handled.
---

# Notes in Finux

Keep the facts, the decisions and the things to do that the user wants to
remember about money. A note is short, and the user reads it later in the
terminal or in Telegram.

## Before you start

- Reach Finux as [references/access.md](references/access.md) tells: MCP
  tools first, the CLI when there are none.
- What a note is, its statuses and its links are in "Notes" of
  [references/concepts.md](references/concepts.md). The arguments of each
  tool are in [references/tools.md](references/tools.md).

## When to write a note

Write a note when the user:

- asks to remember or to note something about money ("note that Ana pays
  half of the energy bill");
- takes a decision ("no new installments until March");
- must do or check something on a date ("ask the bank about the fee next
  week").

Do not write a note for these: Finux has a better place.

| The user tells | Use |
| --- | --- |
| Money that moved | `transaction_add` (skill finux-log-transactions) |
| A bill or an income that will happen | `schedule_add` |
| An amount to save or to pay off | `goal_add` |
| A limit to spend | `budget_add` |
| The name or the tags of a raw line of a bank | The corrections and the page of rules |

Do not write a note on your own for each thing you see. Offer one note
when it clearly helps, and write it after a yes.

## How to write a note

Write it simple and small. The user must read it in 5 seconds.

| Field | Rule |
| --- | --- |
| `title` | 6 words at most, in the words of the user. It tells the subject: `Ask the bank about the fee` |
| `text` | 1 to 3 short sentences, 500 characters at most. Facts and numbers only: who, how much, when. No advice, no greeting, no "the user said". Leave it out when the title tells all |
| `remind_on` | Only when there is something to do or to check on a date. "Next week" is a date: calculate it and tell it to the user |
| `tags` | 1 or 2 tags, from the tags the user already has (`transaction_suggestions`) when one fits |
| `goal`, `budget`, `account` | The record the note is about, when there is one. `account` also takes a credit card |

More rules:

- One subject for each note. Two subjects are two notes.
- Write in the language of the user.
- Do not put account numbers, card numbers or document numbers in a note.
- Before a new note, look for one with the same subject (`note_list` with
  `search`). Change that note with `note_edit` in place of a second one.

```text
Good   title: Energy bill is shared
       text:  Ana pays half. She sends a Pix after the bill comes.

Bad    title: Note about the energy bill situation with my roommate
       text:  The user told me today that the energy bill is something
              they share. I think it would be a good idea to track it.
```

## Draft, then write

Show the note in one or two lines and wait for a yes. One clear
exception: the user dictated the note ("note that ...") and nothing is
uncertain. Then write it and show what you wrote.

After `note_add`, report the id, the title and the reminder date.

## Read the notes

- `note_list` returns the open notes, the due ones first. `due` in the
  result is the number of open notes with a reminder date up to today.
- At the start of a conversation about the finances, call `note_list`
  with `status: due`. Tell the user each due note in one line, before the
  rest of your answer. Do not repeat a note in the same conversation.
- Use the notes as context: a note such as "no new installments until
  March" changes your advice.
- `account_show`, `credit_card_show`, `goal_show` and `budget_show` return
  the open notes of the record in `notes`.
- A note with `source: agent` came from an agent, and one with `user`
  from the user. The words of the user win when two notes do not agree.

## Mark a note as done

Call `note_done` when:

- the user says that it is handled ("I already called the bank");
- the data shows it: the note tells to pay a bill, and the payment is in
  Finux. When you are not sure, ask first.

A done note stays in Finux (`note_list` with `status: done`) and reminds
no more. `note_done` with `done: false` opens it again. To move a
reminder to another date, use `note_edit` with a new `remind_on`: the
note then reminds again.

Delete a note with `note_delete` only when the user asks: show its
message, and call it again with `confirm: true` after a yes.

## Errors you can expect

- `note not found: N`: the id is wrong or the note was deleted. Call
  `note_list` with `status: all`.
- `text is too long: the limit is 500 characters`: make the text shorter;
  do not split one subject into two notes.
- `note N is already done`: nothing to do.
- `goal not found`, `budget not found`, `account not found`: the code is
  wrong. Write the note without the link, or ask.
