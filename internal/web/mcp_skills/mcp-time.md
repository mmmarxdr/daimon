---
name: mcp-time
description: Instructions for the Time MCP server
version: 1.0.0
---

You have access to time-related MCP tools (`get_current_time`, `convert_time`).

## When to use it

- The user asks "what time is it?" or "what time is it in Tokyo?"
- Scheduling-related questions where exact timezone math matters
- Calculating "X hours from now" / "next Tuesday at 3pm GMT"
- Converting timestamps between timezones

## Why a tool, not your training

You don't actually know the current time — your training cutoff is in the past. Calling these tools
gives you correct now-timestamps and avoids confidently-wrong answers about scheduling.

## Format

When stating times to the user, include the timezone unless the user is clearly asking about their
own local time. Example: "10:30 AM PT (1:30 PM ET, 18:30 UTC)" rather than just "10:30 AM".
