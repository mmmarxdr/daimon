---
name: mcp-scrapling
description: Instructions for using the Scrapling MCP web scraper
version: 1.0.0
---

You have access to Scrapling, a full-featured web scraper that handles JavaScript-heavy pages,
captchas, and anti-bot protection. Tools are prefixed with `scrapling_`.

## When to use Scrapling vs Fetch vs web_fetch

Match the tool to the resistance level:

| Page type | Use |
|-----------|-----|
| Plain blog posts, docs sites, GitHub READMEs | daimon's built-in `web_fetch` (fastest) |
| Standard sites with light JS | `mcp-fetch` if installed, else `web_fetch` |
| Sites with cloudflare, captchas, or "Please enable JavaScript" walls | **Scrapling** |
| Single-page apps with content loaded dynamically | **Scrapling** |
| Anything with anti-scraping countermeasures | **Scrapling** |

Scrapling is the heaviest option (spawns a real browser). Don't reach for it first.

## Common patterns

- **`scrapling_StealthyFetcher`** — for high-resistance sites; uses a hardened browser fingerprint
- **`scrapling_PlayWrightFetcher`** — when you need full JavaScript execution
- **`scrapling_Fetcher`** — fast HTTP fetch for sites without JS requirements

Always specify the **CSS selector or XPath** you want — don't dump the entire page back to the
conversation. A page can be hundreds of KB; pick the data you need.

## Adapt when sites change

Scrapling's killer feature is **adaptive selectors**: if the site changes its DOM, Scrapling can
re-find the element using saved heuristics. Use `auto_match=True` on subsequent calls to the same
selector to enable this.

## Don't

- Don't scrape sites that explicitly forbid it in their robots.txt or ToS
- Don't loop scraping the same URL — once is enough, cache the result in conversation context
- Don't pass user credentials or session cookies through Scrapling unless the user explicitly
  authorized that specific use
