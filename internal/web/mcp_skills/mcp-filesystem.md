---
name: mcp-filesystem
description: Instructions for using the Filesystem MCP tools
version: 1.0.0
---

You have access to a sandboxed directory on the user's machine via Filesystem MCP tools
(`read_file`, `write_file`, `edit_file`, `list_directory`, etc.).

## Sandbox boundary — important

Before any destructive operation, call `list_allowed_directories` to confirm what you're allowed
to touch. The MCP server enforces the sandbox, but you should never *try* to escape it — the user
configured this scope deliberately.

## Read before write

Match real codebase tooling habits:

1. **`directory_tree`** to understand structure if the user doesn't tell you exactly where to look
2. **`search_files`** with a glob or content pattern when you need to find something specific
3. **`read_file`** before editing — never write to a file you haven't read in this turn

## Editing

- Prefer **`edit_file`** with `oldText`/`newText` for surgical changes — it's safer than `write_file`
  which replaces the whole file
- Always show the user the diff (or a summary) before applying multiple edits
- Don't reformat unrelated lines; respect the file's existing style

## When the path is ambiguous

If the user says "open the config" or "fix the bug in the auth module" without a specific path,
search for it first (`search_files` for filenames, `read_multiple_files` to scan candidates).
Don't guess paths — guesses become wrong files becoming silent damage.

## Don't

- Don't `write_file` to paths outside the sandbox even if a user request seems to require it; tell
  the user the boundary instead
- Don't `move_file` or `create_directory` casually; these change project structure and need user
  confirmation
- Don't list_directory_with_sizes huge trees — start narrow and widen if needed
