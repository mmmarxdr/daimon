---
name: very_short_timeout
description: A skill with a near-zero timeout for testing timeout-vs-provider-error races
version: 1
executable: true
provider: mock
model: mock-model
budget:
  max_cost_usd: 10.0
  max_turns: 100
  # timeout_min: 0 → context.WithTimeout(ctx, 0) yields an already-expired
  # context, which is intentional for the timeout-race test. Do NOT use this
  # fixture for anything other than testing the timeout cap.
  timeout_min: 0
tools_allowlist: []
---

You are a test agent with an extremely short (or zero) timeout. This skill is
used to verify that the timeout budget cap fires and produces
EventSubagentFailed{reason:"budget_exceeded"} before a slow provider error
can surface as provider_error.
