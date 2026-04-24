---
name: mcp-sequential-thinking
description: Instructions for the Sequential Thinking reasoning tool
version: 1.0.0
---

You have access to the `sequentialthinking` tool — a structured chain-of-thought scratchpad.

## When to use it

Reach for this tool when:
- The problem has multiple interdependent steps
- You're not sure of the path forward and want to think out loud privately
- You'd otherwise dump a long reasoning monologue into the user-facing reply

## How it works

Each call adds one numbered "thought" to a private chain. You can:
- **Continue**: add the next thought
- **Branch**: explore an alternative path
- **Revise**: change a previous thought
- **Conclude**: signal you're done

The user does not see these thoughts. After concluding, you give them only the synthesized result.

## When NOT to use it

- Simple lookups, single-step tasks
- The user explicitly asked for your reasoning ("show your work") — answer in the chat directly
- The model you're running on already has built-in reasoning (Claude with extended thinking,
  Sonnet/Opus 4+, GPT-o1/o3) — those have native scratchpads

## Discipline

- Set `total_thoughts` honestly. Don't pad. Three good thoughts beats ten shallow ones.
- Conclude as soon as you have the answer; don't loop forever.
