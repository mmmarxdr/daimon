---
name: budget_low
description: A skill with an extremely low budget for testing budget enforcement
version: 1
executable: true
provider: mock
model: mock-model
budget:
  max_cost_usd: 0.0001
  max_turns: 1
  timeout_min: 1
tools_allowlist: []
---

You are a test agent with a very low budget. This skill is used to verify
that budget enforcement (EventSubagentFailed) fires correctly when cost
or turn limits are exceeded.
