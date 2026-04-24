---
name: mcp-obsidian
description: Instructions for using the Obsidian MCP tools
version: 1.0.0
---

You have access to the user's Obsidian vault via MCP tools (prefixed with `obsidian_`).
The vault is the user's primary thinking space — treat their notes with care.

## Read first, then write

When the user asks you to do something with their notes:

1. **Search before writing** — `obsidian_simple_search` or `obsidian_complex_search` to find related
   notes. The user almost certainly already has notes on the topic.
2. **Read the matched notes** — `obsidian_get_file_contents` to load full context before editing.
3. **Then act** — append, edit, or create.

Skipping step 1 leads to duplicate notes scattered across the vault. The vault is a graph; respect
existing structure.

## Writing conventions

- **Append, don't replace** by default. Use `obsidian_append_content` for adding to existing notes.
- **Preserve formatting** — Obsidian uses standard markdown plus `[[wikilinks]]`, `#tags`, and
  `![[embeds]]`. Honor all three.
- **Use wikilinks** — when mentioning another note that exists in the vault, use `[[Note Name]]`
  instead of plain text. This builds the graph the user relies on.
- **Add frontmatter to new notes** — at minimum `tags:` and `created:` (ISO date). The user may have
  templates; check existing notes in the same folder first.
- **Date stamps** — daily notes typically follow `YYYY-MM-DD.md`. Check `obsidian_list_files_in_dir`
  for the daily-notes folder before assuming a path.

## Common tasks

| User says | Do |
|-----------|----|
| "find notes about X" | `simple_search` or `complex_search` with the term, return ranked snippets |
| "add this to my daily note" | resolve today's daily note path, `append_content` |
| "create a note for project Y" | search first; if no match, `put_content` with frontmatter + initial structure |
| "what did I write about X last week" | `complex_search` with date filter, summarize matches |

## Don't

- Don't modify files outside the user's stated scope.
- Don't delete notes — `obsidian_delete_file` is for explicit "delete this" requests only.
- Don't bury wikilinks in long edits without telling the user; mention which links you added.
