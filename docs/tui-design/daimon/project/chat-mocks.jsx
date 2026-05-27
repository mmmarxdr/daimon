// Extended mock conversation — richer demo across many moments
// Multiple turns covering: reasoning, tools variety, tables, images,
// code with diffs, streaming state, error recovery, inline answers.

const MOCK_CONVO = [
  // ── Turn 1: user asks broad question
  {
    id: 'u1', role: 'user', time: '14:32',
    content: 'Analyze yesterday\'s payment service logs, find the cause of the anomalies, and propose a fix.',
  },
  {
    id: 'a1', role: 'assistant', time: '14:32',
    reasoning: `The user wants a full root-cause analysis of yesterday's payment logs.

Plan:
1. Read the raw log file to get a sense of volume and error patterns.
2. Grep for explicit error markers: timeouts, retries, exhausted.
3. Cross-reference with recent deploys — something probably changed.
4. Propose a minimal, conservative patch.

The webhook timeouts feel suspicious — that pattern usually comes from an external provider under load, OR a timeout setting that's too tight for the new async code path. I should check both.`,
    reasoningDuration: '6s',
    blocks: [
      {
        kind: 'text',
        content: `Looking into this. Let me pull the log first and see what we're dealing with.`,
      },
      {
        kind: 'tool', name: 'read_file', status: 'done', duration: '142ms',
        input: { path: '/var/log/payments/2026-04-18.log', lines: 'last 500' },
        preview: '/var/log/payments/2026-04-18.log',
        output: `2026-04-18 03:14:22 INFO  svc=payments msg="tx accepted" id=tx_8f3a2
2026-04-18 03:14:23 ERROR svc=payments msg="stripe webhook timeout" id=tx_8f3a2
2026-04-18 03:14:24 WARN  svc=payments msg="retry 1/3" id=tx_8f3a2
2026-04-18 03:14:26 ERROR svc=payments msg="retry exhausted" id=tx_8f3a2
... (487 more lines)`,
        stats: { lines: 500, matches: 42 },
      },
      {
        kind: 'text',
        content: `**42 error lines** clustered in two time windows (03:14 and 07:22 UTC). Let me isolate the pattern:`,
      },
      {
        kind: 'tool', name: 'grep', status: 'done', duration: '89ms',
        input: { pattern: 'webhook timeout|retry exhausted', path: '/var/log/payments/' },
        preview: 'webhook timeout|retry exhausted',
        output: `2026-04-18.log:314: ERROR svc=payments msg="stripe webhook timeout"
2026-04-18.log:315: ERROR svc=payments msg="retry exhausted"
2026-04-18.log:412: ERROR svc=payments msg="stripe webhook timeout"
2026-04-18.log:417: ERROR svc=payments msg="retry exhausted"
... 19 more matches`,
        stats: { matches: 21 },
      },
      {
        kind: 'tool', name: 'git_log', status: 'done', duration: '54ms',
        input: { since: '2026-04-17', path: 'services/payments/' },
        preview: 'services/payments/ since 2026-04-17',
        output: `a8f21b3  chore: bump stripe SDK to 14.2.0   (2026-04-17 18:02)
2c1d9e7  feat: switch webhook handler to async  (2026-04-17 17:44)`,
      },
      {
        kind: 'text',
        content: `Found the window. Two recent deploys touched this path — the async refactor is the likely culprit. Let me inspect the handler:`,
      },
      {
        kind: 'tool', name: 'read_file', status: 'error', duration: '12ms',
        input: { path: 'services/payments/webhook.ts' },
        preview: 'services/payments/webhook.ts',
        output: `Error: permission denied — path outside workspace root`,
        error: true,
      },
      {
        kind: 'text',
        content: `The tool hit a permissions error — trying again with the correct root-relative path:`,
      },
      {
        kind: 'tool', name: 'read_file', status: 'done', duration: '38ms',
        input: { path: './services/payments/webhook.ts' },
        preview: './services/payments/webhook.ts',
        output: `// ... (file contents shown in workspace)`,
        stats: { lines: 48 },
      },
      {
        kind: 'text',
        content: `## What I found

Two correlated issues, both traceable to commit \`2c1d9e7\`:

### 1. Webhook timeout is too tight for the new async path
The handler was refactored to \`async\` but the 2s timeout was kept from the sync version. Under load, the event loop lag pushes Stripe's response past the ceiling.

### 2. Retry context is lost after exhaustion
The new \`withRetry\` wrapper doesn't carry the original request ID, so retries show up as new transactions in the retry queue — and are dropped.

| Metric | Before deploy | After deploy |
| :-- | --: | --: |
| p50 webhook latency | 340ms | 410ms |
| p99 webhook latency | 1.8s | **4.2s** |
| Timeout rate | 0.04% | **2.1%** |
| Tx success rate | 99.97% | 98.12% |

### Proposed patch

\`\`\`typescript
 // services/payments/webhook.ts
-const WEBHOOK_TIMEOUT_MS = 2000
+const WEBHOOK_TIMEOUT_MS = 8000
+const MAX_RETRIES = 5

 export async function handleStripeWebhook(event: StripeEvent) {
-  return await withTimeout(processEvent(event), WEBHOOK_TIMEOUT_MS)
+  return await withRetry(
+    () => withTimeout(processEvent(event), WEBHOOK_TIMEOUT_MS),
+    { maxAttempts: MAX_RETRIES, backoff: 'exponential', idKey: event.id }
+  )
 }
\`\`\`

Conservative — raises the ceiling without introducing new behavior. The \`idKey\` keeps retries deduplicated. Want me to open a PR against \`main\`?`,
      },
    ],
  },

  // ── Turn 2: user follow-up, different tool
  {
    id: 'u2', role: 'user', time: '14:41',
    content: 'Before the PR — check if Stripe\'s own status page had anything yesterday. I want to rule out their side.',
  },
  {
    id: 'a2', role: 'assistant', time: '14:41',
    reasoning: `Good instinct. Stripe occasionally has degraded webhook delivery during EU business hours. Let me check their status history API.`,
    reasoningDuration: '3s',
    blocks: [
      {
        kind: 'tool', name: 'web_fetch', status: 'done', duration: '1.2s',
        input: { url: 'https://status.stripe.com/history', range: '2026-04-18' },
        preview: 'status.stripe.com — 2026-04-18',
        output: `[{"date":"2026-04-18","incidents":[{"time":"14:00-15:30 UTC","component":"webhook delivery","severity":"degraded","region":"EU"}]}]`,
        stats: { size: '2.4kb' },
      },
      {
        kind: 'text',
        content: `Confirmed — Stripe **did** have a degraded webhook delivery incident in EU between 14:00–15:30 UTC. But our anomalies were at **03:14** and **07:22** UTC, outside that window. Stripe's side was healthy during our incidents. The issue is on our side.

So: go ahead with the patch. Here's a summary of the timeline:`,
      },
      {
        kind: 'timeline',
        events: [
          { t: '17:44', lvl: 'info',  msg: 'async webhook handler deployed (2c1d9e7)' },
          { t: '18:02', lvl: 'info',  msg: 'stripe SDK bump (a8f21b3)' },
          { t: '03:14', lvl: 'error', msg: 'first timeout cluster — 11 events' },
          { t: '07:22', lvl: 'error', msg: 'second timeout cluster — 10 events' },
          { t: '14:00', lvl: 'warn',  msg: 'stripe EU degraded (unrelated)' },
        ],
      },
    ],
  },

  // ── Turn 3: streaming right now
  {
    id: 'u3', role: 'user', time: '14:44',
    content: 'Open the PR.',
  },
  {
    id: 'a3', role: 'assistant', time: '14:44',
    reasoning: `Opening a PR against main with the patch. Title should reference the commit that introduced the regression.`,
    reasoningDuration: 'now',
    streaming: true,
    blocks: [
      {
        kind: 'tool', name: 'shell', status: 'running', duration: '—',
        input: { cmd: 'git checkout -b fix/webhook-timeout && git add -p services/payments/webhook.ts' },
        preview: 'git checkout -b fix/webhook-timeout…',
        output: `Switched to a new branch 'fix/webhook-timeout'
diff --git a/services/payments/webhook.ts b/services/payments/webhook.ts
Stage this hunk [y,n,q,a,d,e,?]? `,
        streaming: true,
      },
      {
        kind: 'text',
        content: `Staging the hunk. I'll push once the test suite passes locally…`,
        streaming: true,
      },
    ],
  },
];

const WORKSPACE_ARTIFACTS = {
  table: {
    type: 'table',
    title: 'payment metrics — before vs. after deploy',
    header: ['Metric', 'Before', 'After', 'Δ'],
    align: ['left', 'right', 'right', 'right'],
    rows: [
      ['p50 webhook latency', '340ms', '410ms', '+70ms'],
      ['p99 webhook latency', '1.8s', '4.2s', '+2.4s', 'bad'],
      ['Timeout rate', '0.04%', '2.1%', '+52×', 'bad'],
      ['Tx success rate', '99.97%', '98.12%', '−1.85pt', 'bad'],
      ['Avg retries per event', '0.12', '1.9', '+15×', 'bad'],
      ['Queue depth (peak)', '12', '340', '+28×', 'bad'],
    ],
  },
  image: {
    type: 'image',
    title: 'webhook latency — 04-18',
    caption: 'p99 latency over 24h. Two spikes visible at 03:14 and 07:22 UTC.',
  },
  patch: {
    type: 'code',
    language: 'typescript',
    title: 'webhook.ts — proposed patch',
    diff: true,
    content: ` // services/payments/webhook.ts
-const WEBHOOK_TIMEOUT_MS = 2000
+const WEBHOOK_TIMEOUT_MS = 8000
+const MAX_RETRIES = 5

 export async function handleStripeWebhook(event: StripeEvent) {
-  return await withTimeout(processEvent(event), WEBHOOK_TIMEOUT_MS)
+  return await withRetry(
+    () => withTimeout(processEvent(event), WEBHOOK_TIMEOUT_MS),
+    { maxAttempts: MAX_RETRIES, backoff: 'exponential', idKey: event.id }
+  )
 }

 async function processEvent(event: StripeEvent) {
   switch (event.type) {
     case 'payment_intent.succeeded':
       return markTxPaid(event.data.object.id)
     case 'payment_intent.payment_failed':
       return markTxFailed(event.data.object.id)
     default:
       return ignore(event)
   }
 }`,
  },
  log: {
    type: 'log',
    title: '2026-04-18.log — filtered',
    entries: [
      { t: '03:14:22', lvl: 'INFO',  msg: 'tx accepted id=tx_8f3a2' },
      { t: '03:14:23', lvl: 'ERROR', msg: 'stripe webhook timeout id=tx_8f3a2' },
      { t: '03:14:24', lvl: 'WARN',  msg: 'retry 1/3 id=tx_8f3a2' },
      { t: '03:14:26', lvl: 'ERROR', msg: 'retry exhausted id=tx_8f3a2' },
      { t: '03:14:28', lvl: 'INFO',  msg: 'tx accepted id=tx_9e2b1' },
      { t: '07:22:01', lvl: 'ERROR', msg: 'stripe webhook timeout id=tx_c4f9d' },
      { t: '07:22:03', lvl: 'WARN',  msg: 'retry 1/3 id=tx_c4f9d' },
      { t: '07:22:05', lvl: 'ERROR', msg: 'retry exhausted id=tx_c4f9d' },
    ],
  },
};

Object.assign(window, { MOCK_CONVO, WORKSPACE_ARTIFACTS });
