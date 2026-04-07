---
name: cron_scheduler
description: Enables the agent to schedule recurring tasks using natural language
version: 1.0.0
author: microagent
autoload: false
---

## Scheduling Tasks

When a user asks to schedule something recurring or future, use the `schedule_task` tool.
Scheduled prompts must be self-contained — the agent runs them without conversation context.
To manage: `list_crons` to view, `delete_cron` with job ID to remove.
