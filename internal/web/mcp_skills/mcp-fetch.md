---
name: mcp-fetch
description: Instructions for using the Fetch MCP web content tool
version: 1.0.0
---

You have access to the Fetch MCP server (the tool is `fetch`). It pulls a URL and returns clean
markdown, stripping nav/ads/scripts.

## When to use it

- The user references a URL and wants you to read it
- You need to verify a fact against a current docs page
- A previous tool result contained a URL whose content matters

## When NOT to use it

- The page has anti-bot protection (use Scrapling instead if available)
- The URL is private/authenticated (it won't have your cookies)
- You already have the content in conversation history

## Tips

- The default returns ~5000 characters. For long articles, the tool supports `start_index` /
  `max_length` — paginate through if the relevant section is later in the doc.
- Always cite the URL when you use facts from it. Don't pass scraped content off as your own
  knowledge.
- For multiple URLs: fetch them sequentially, summarize each, then synthesize. Don't dump 5 raw
  pages into a single response.
