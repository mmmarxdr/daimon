---
name: noallowlist
description: Skill with empty tools_allowlist to verify parent MCP tools are not leaked
version: 1
executable: true
provider: mock
model: mock-model
budget: defaults
tools_allowlist: []
---

You are a test agent with no tools. This skill verifies that an explicit
empty tools_allowlist does not inherit any parent tools. The child agent
should have zero tools available.
