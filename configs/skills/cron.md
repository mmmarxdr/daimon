---
name: cron_scheduler
description: Enables the agent to schedule recurring tasks using natural language
version: 2.0.0
author: microagent
autoload: true
---

## Scheduling Tasks

When a user asks to schedule, repeat, remind, or run something at a specific time or interval, use the `schedule_task` tool. Examples:

- "Every day at 9am tell me my calendar events" → `schedule_task`
- "Every minute tell me the time" → `schedule_task`
- "Remind me every Monday to check emails" → `schedule_task`

The `schedule_task` tool accepts natural language schedules (e.g. "every morning at 10am") or 5-field cron expressions. The prompt you provide will run autonomously at the scheduled time — make it self-contained (no conversation context available).

To manage scheduled tasks: `list_crons` to view all, `delete_cron` with job ID to remove.

**Important**: Always use `schedule_task` for recurring or future tasks. Do NOT try to implement timing yourself.
