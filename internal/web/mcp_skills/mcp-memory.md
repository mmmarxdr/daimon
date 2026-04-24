---
name: mcp-memory
description: Instructions for using the Memory MCP knowledge graph
version: 1.0.0
---

You have access to a persistent knowledge graph via Memory MCP tools (prefixed with `memory_`).
This is a SEPARATE store from your built-in long-term memory — use it for **structured entities and
relationships** that survive across conversations.

## When to use the knowledge graph

The graph stores three kinds of items:

- **Entities** — recurring people, organizations, projects, places, significant events
- **Relations** — directional links between entities ("Alice works_at Acme", "Project X depends_on Library Y")
- **Observations** — factual statements attached to an entity ("Alice prefers Python over Go")

Use it when the information is:
- About a *thing the user keeps referring back to* (a project, a person, a recurring topic)
- Worth connecting to other things (relationships matter)
- Likely to be queried again later

Do NOT use it for:
- Single-turn facts you can hold in context
- Things daimon's built-in `save_memory` tool already covers (user preferences, simple atomic facts)

## Standard interaction pattern

**At the start of each session involving the user's recurring topics:**
1. Call `memory_open_nodes` or `memory_search_nodes` to retrieve what's already known about the
   relevant entities. Don't announce this — just use it.
2. Use that context to inform your response.

**During the conversation:**
3. Listen for new information that fits the entity/relation/observation model:
   - Identity (name, role, location, expertise)
   - Behaviors (recurring patterns, work habits)
   - Preferences (tools, languages, communication style)
   - Goals (current projects, aspirations)
   - Relationships (people, orgs, projects connected to them)

**At the end of meaningful exchanges:**
4. Update the graph:
   - `memory_create_entities` for new things
   - `memory_create_relations` to link them
   - `memory_add_observations` for facts about existing entities
5. Be quiet about it. The user does not need a "saved to memory" notification.

## Boundaries

- **Don't duplicate**: search before creating. If an entity already exists, add observations to it.
- **Don't infer relationships**: only create edges the user actually stated.
- **Don't hoard**: a fact that won't be referenced again next month is noise, not knowledge.
- **Forget on request**: if the user says "forget about X", call `memory_delete_entities` immediately.
