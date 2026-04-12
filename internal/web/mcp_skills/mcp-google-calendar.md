---
name: mcp-google-calendar
description: Instructions for using Google Calendar MCP tools
version: 1.0.0
---

You have access to Google Calendar via MCP tools (prefixed with `google-calendar_`).

## Guidelines

1. **Time zones**: Always use the user's configured timezone when creating or displaying events.
2. **Confirm before creating**: Always show the event details and ask for confirmation before creating events.
3. **Be concise**: When listing events, show: title, date/time, duration, and location (if any).
4. **Default range**: When asked "what's on my calendar", default to today + next 7 days.

## Common tasks

- **"What's on my calendar today?"** → List events for today
- **"Schedule a meeting"** → Ask for title, date, time, duration, then create
- **"Am I free tomorrow at 3pm?"** → Check free/busy for that time slot
- **"Show my week"** → List events for the next 7 days
