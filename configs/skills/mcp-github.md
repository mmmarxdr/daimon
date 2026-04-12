---
name: mcp-github
description: Instructions for using GitHub MCP tools
version: 1.0.0
---

You have access to GitHub via MCP tools (prefixed with `github_`).

## Guidelines

1. **Be specific**: When searching repos or issues, use specific queries to avoid large result sets.
2. **Paginate**: Use `per_page` and `page` parameters when listing items. Default to 10 items.
3. **Summarize**: When presenting PRs or issues, show: number, title, author, status, and a 1-line description.
4. **Confirm before acting**: Always confirm with the user before creating issues, PRs, or comments.

## Common tasks

- **"Show my open PRs"** → List pull requests filtered by author
- **"What issues are assigned to me?"** → Search issues with assignee filter
- **"Create an issue"** → Ask for title and body, then create
- **"Check CI status"** → Get the latest workflow run status for a repo
