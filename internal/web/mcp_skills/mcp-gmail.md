---
name: mcp-gmail
description: Instructions for efficiently using Gmail MCP tools
version: 1.0.0
---

You have access to Gmail via MCP tools (prefixed with `gmail_`). Follow these rules:

## Important: Minimize data fetched

Email responses can be very large (100KB+ per email with HTML bodies). Always:

1. **Start with counts**: Use `gmail_get_message_count` to check how many messages exist.
2. **Use small limits**: When fetching messages, always set `limit` to 5 or less.
3. **Search first**: Prefer `gmail_search_by_sender`, `gmail_search_by_subject`, or `gmail_search_since_date` over `gmail_get_recent_messages` to get targeted results.
4. **Summarize, don't dump**: When presenting emails to the user, show only: sender, subject, date, and a 1-2 line summary of the content. Never paste the full email body unless explicitly asked.

## Common tasks

- **"Show my unread emails"** → Use `gmail_get_unseen_messages` with `limit: 5`
- **"How many emails do I have?"** → Use `gmail_get_message_count`
- **"Search emails from X"** → Use `gmail_search_by_sender` with the email address
- **"Search emails about X"** → Use `gmail_search_by_subject` with keywords
- **"Send an email"** → Use `gmail_send_email` with `to`, `subject`, and `text` fields
- **"Reply to email"** → First get the message UID, then use `gmail_reply_to_email`

## Connection

The Gmail tools auto-connect to IMAP/SMTP on first use. The first call may take a few extra seconds.
