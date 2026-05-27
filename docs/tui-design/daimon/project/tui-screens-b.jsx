// Daimon TUI — screens (part B)
// Slash palette, Tools & MCPs, Sessions + Model picker, Error view

// ─────────────────────────────────────────────────────────────────
// SCREEN 4 — Slash palette
// ─────────────────────────────────────────────────────────────────
function TUI_Slash({ width = 1400, height = 860 }) {
  // Background = a dimmed chat behind the palette overlay
  const dimmedChat = (
    <div style={{
      filter: 'blur(0.5px)', opacity: 0.32,
      pointerEvents: 'none', userSelect: 'none',
    }}>
      <MsgUser time="14:32" name="arnau">
        analyze yesterday's payment logs, find the anomaly, and propose a conservative patch.
      </MsgUser>
      <MsgDaimon time="14:32">
        <Reasoning duration="6s"/>
        <div style={{ marginBottom: 6 }}>found the window. pulling sources now —</div>
        <ToolLine status="done" name="read_file" input="/var/log/payments/2026-04-18.log" stat="500 lines" tokens={1842} duration="142ms"/>
        <ToolLine status="done" name="grep" input="'webhook timeout|retry exhausted'" stat="21 matches" tokens={612} duration="89ms"/>
        <ToolLine status="done" name="git_log" input="services/payments/ since=2026-04-17" stat="2 commits" tokens={284} duration="54ms"/>
        <ToolLine status="running" name="read_file" input="./services/payments/webhook.ts" tokens={420}/>
      </MsgDaimon>
    </div>
  );

  const groups = [
    {
      label: 'session',
      items: [
        { c: 'new',       d: 'start a fresh thread',                 k: '⌃N', active: false },
        { c: 'resume',    d: 'pick up a recent conversation',        k: '⌃R', hilite: true },
        { c: 'fork',      d: 'branch from current turn',             k: '⌃F' },
        { c: 'save',      d: 'snapshot to ~/.daimon/sessions',       k: '⌃S' },
        { c: 'export',    d: 'export as markdown · json',            k: '⌃E' },
      ],
    },
    {
      label: 'agent',
      items: [
        { c: 'model',     d: 'switch model (sonnet · opus · haiku)', k: '⌃M' },
        { c: 'mode',      d: 'plan · build · review',                k: '⇧⇥' },
        { c: 'memory',    d: 'inspect long-term facts',              k: '⌃⌥M' },
        { c: 'tools',     d: 'enable/disable tools & MCPs',          k: '⌃T' },
        { c: 'spawn',     d: 'launch a sub-agent',                   k: '⌃⇧A' },
      ],
    },
    {
      label: 'workspace',
      items: [
        { c: 'cd',        d: 'change working directory',             k: '' },
        { c: 'git',       d: 'inline git operations (log · diff)',   k: '⌃G' },
        { c: 'shell',     d: 'one-shot shell command',               k: '!' },
        { c: 'open',      d: 'open a file in $EDITOR',               k: '⌃O' },
      ],
    },
  ];

  return (
    <TUIScreen width={width} height={height}>
      <TUITopBar status="paused" mode="plan" cost="$0.142"/>

      <div style={{ position: 'relative', flex: 1, minHeight: 0 }}>
        {/* Dimmed background */}
        <div style={{
          position: 'absolute', inset: 0, overflow: 'hidden',
          padding: '4px 4px 0',
        }}>{dimmedChat}</div>

        {/* Dim overlay */}
        <div style={{
          position: 'absolute', inset: 0,
          background: 'linear-gradient(to bottom, rgba(14,15,19,0.55), rgba(14,15,19,0.85))',
          backdropFilter: 'blur(2px)',
        }}/>

        {/* Palette */}
        <div style={{
          position: 'absolute', top: 30, left: '50%', transform: 'translateX(-50%)',
          width: 680, background: TUI.bgPanel,
          border: `1px solid ${TUI.accent}`,
          borderRadius: 3, overflow: 'hidden',
          boxShadow: `0 0 30px ${TUI.accent}22, 0 18px 60px rgba(0,0,0,0.7)`,
        }}>
          {/* Search */}
          <div style={{
            padding: '12px 16px',
            borderBottom: `1px solid ${TUI.line}`,
            display: 'flex', alignItems: 'center', gap: 10,
            background: 'rgba(93,191,167,0.04)',
          }}>
            <T c={TUI.accent} bold style={{ fontSize: 14 }}>/</T>
            <span style={{ flex: 1, fontSize: 13, color: TUI.ink }}>
              re<Caret w={6} h={14}/>
            </span>
            <T c={TUI.inkMuted} style={{ fontSize: 11 }}>3 matches · 14 total</T>
          </div>

          {/* Results */}
          <div style={{ maxHeight: 480, overflow: 'hidden' }}>
            {groups.map((g, gi) => (
              <div key={g.label}>
                <div style={{
                  padding: '6px 16px 3px', fontSize: 10.5,
                  color: TUI.inkMuted, textTransform: 'uppercase', letterSpacing: 1.4,
                  background: TUI.bgDeep,
                }}>── {g.label}</div>
                {g.items.map((it, ii) => {
                  const isHi = it.hilite;
                  // highlight matched chars "re" at start
                  const match = it.c.match(/^(re)(.*)/i);
                  const rendered = match
                    ? <><T c={TUI.accent} bold>{match[1]}</T><T c={TUI.ink}>{match[2]}</T></>
                    : <T c={TUI.ink}>{it.c}</T>;
                  return (
                    <div key={it.c} style={{
                      padding: '5px 16px',
                      display: 'flex', alignItems: 'center', gap: 12,
                      background: isHi ? TUI.accentBg : 'transparent',
                      borderLeft: isHi ? `2px solid ${TUI.accent}` : '2px solid transparent',
                      paddingLeft: isHi ? 14 : 16,
                    }}>
                      <span style={{ width: 88, fontWeight: 500 }}>/{rendered}</span>
                      <T c={isHi ? TUI.inkSoft : TUI.inkMuted} style={{ flex: 1, fontSize: 12.5 }}>
                        {it.d}
                      </T>
                      {it.k && <T c={TUI.inkFaint} style={{ fontSize: 11 }}>{it.k}</T>}
                      {isHi && <T c={TUI.accent}>↵</T>}
                    </div>
                  );
                })}
              </div>
            ))}
          </div>

          {/* Footer */}
          <div style={{
            padding: '8px 16px', borderTop: `1px solid ${TUI.line}`,
            display: 'flex', gap: 16, fontSize: 11, color: TUI.inkMuted,
            background: TUI.bgDeep,
          }}>
            <span><T c={TUI.accent}>↑↓</T> navigate</span>
            <span><T c={TUI.accent}>↵</T> run</span>
            <span><T c={TUI.accent}>⇥</T> autocomplete</span>
            <span><T c={TUI.accent}>esc</T> dismiss</span>
            <span style={{ flex: 1 }}/>
            <T italic c={TUI.inkFaint}>summon a command.</T>
          </div>
        </div>
      </div>

      <TUIFooter hints={[
        { k: 'esc', l: 'close palette' },
        { k: '/', l: 'search prefix' },
        { k: '?', l: 'help' },
      ]}/>
    </TUIScreen>
  );
}

// ─────────────────────────────────────────────────────────────────
// SCREEN 5 — Tools & MCPs
// ─────────────────────────────────────────────────────────────────
function TUI_Tools({ width = 1400, height = 860 }) {
  const builtin = [
    { n: 'read_file',   on: true,  cat: 'fs',    risk: 'safe',   calls: 84, lat: '32ms' },
    { n: 'write_file',  on: true,  cat: 'fs',    risk: 'edit',   calls: 12, lat: '48ms' },
    { n: 'edit_file',   on: true,  cat: 'fs',    risk: 'edit',   calls: 19, lat: '54ms' },
    { n: 'grep',        on: true,  cat: 'fs',    risk: 'safe',   calls: 62, lat: '88ms' },
    { n: 'ls',          on: true,  cat: 'fs',    risk: 'safe',   calls: 31, lat: '12ms' },
    { n: 'shell',       on: true,  cat: 'sys',   risk: 'exec',   calls: 27, lat: '410ms' },
    { n: 'git',         on: true,  cat: 'sys',   risk: 'edit',   calls: 14, lat: '92ms' },
    { n: 'web_fetch',   on: false, cat: 'net',   risk: 'net',    calls: 0,  lat: '—' },
    { n: 'web_search',  on: false, cat: 'net',   risk: 'net',    calls: 0,  lat: '—' },
    { n: 'spawn',       on: true,  cat: 'agent', risk: 'meta',   calls: 4,  lat: '6.2s' },
    { n: 'memory_add',  on: true,  cat: 'agent', risk: 'safe',   calls: 9,  lat: '14ms' },
    { n: 'memory_recall',on:true,  cat: 'agent', risk: 'safe',   calls: 18, lat: '22ms' },
  ];
  const mcps = [
    { n: 'postgres-mcp',     v: 'v0.6.1', st: 'connected', tools: 8, h: 'localhost:5432' },
    { n: 'github-mcp',       v: 'v1.2.0', st: 'connected', tools: 12, h: 'api.github.com' },
    { n: 'linear-mcp',       v: 'v0.4.3', st: 'connected', tools: 6, h: 'api.linear.app' },
    { n: 'figma-mcp',        v: 'v0.3.0', st: 'disabled',  tools: 4, h: 'api.figma.com' },
    { n: 'localhost-shell',  v: 'v0.1.0', st: 'error',     tools: 1, h: 'sandbox · denied' },
  ];

  const riskColor = r => ({ safe: TUI.green, edit: TUI.amber, exec: TUI.red, net: TUI.pink, meta: TUI.accent }[r]);

  return (
    <TUIScreen width={width} height={height}>
      <TUITopBar status="ready" mode="build" cost="$0.142"/>

      <div style={{
        display: 'flex', gap: 14, padding: '4px 4px 8px',
        fontSize: 12.5, color: TUI.inkMuted,
      }}>
        <T c={TUI.accent}>⚒</T>
        <T c={TUI.ink} bold>tools & integrations</T>
        <T c={TUI.inkFaint}>·</T>
        <span>12 builtin · 5 mcp servers · 31 tools available</span>
        <span style={{ flex: 1 }}/>
        <T italic c={TUI.inkMuted}>extend with daimon mcp install &lt;name&gt;</T>
      </div>

      <div style={{ flex: 1, display: 'flex', gap: 10, minHeight: 0 }}>
        {/* Builtin tools */}
        <TUIPanel
          title="builtin tools"
          badge={<T c={TUI.inkMuted} style={{ fontSize: 10.5 }}>10 enabled · 2 off</T>}
          flex={1}>
          <div style={{ padding: '0 0 6px' }}>
            {/* header */}
            <div style={{
              display: 'grid',
              gridTemplateColumns: '24px 1fr 60px 60px 60px 60px',
              padding: '5px 12px',
              fontSize: 10.5, color: TUI.inkMuted, letterSpacing: 1, textTransform: 'uppercase',
              borderBottom: `1px solid ${TUI.line}`,
            }}>
              <span></span><span>tool</span><span>cat</span><span style={{ textAlign: 'right' }}>risk</span>
              <span style={{ textAlign: 'right' }}>calls</span><span style={{ textAlign: 'right' }}>p50</span>
            </div>
            {builtin.map((t, i) => {
              const active = i === 5; // shell row hilited
              return (
                <div key={t.n} style={{
                  display: 'grid',
                  gridTemplateColumns: '24px 1fr 60px 60px 60px 60px',
                  padding: '3px 12px', fontSize: 12.5, alignItems: 'center',
                  background: active ? TUI.accentBg : 'transparent',
                  borderLeft: active ? `2px solid ${TUI.accent}` : '2px solid transparent',
                  paddingLeft: active ? 10 : 12,
                  color: t.on ? TUI.ink : TUI.inkMuted,
                }}>
                  <span>{t.on
                    ? <T c={TUI.accent}>●</T>
                    : <T c={TUI.inkFaint}>○</T>}</span>
                  <span>{t.n}</span>
                  <T c={TUI.inkMuted} style={{ fontSize: 11.5 }}>{t.cat}</T>
                  <T c={riskColor(t.risk)} style={{ fontSize: 11, textAlign: 'right' }}>{t.risk}</T>
                  <T c={t.on ? TUI.inkSoft : TUI.inkFaint} style={{ textAlign: 'right' }}>{t.calls || '—'}</T>
                  <T c={t.on ? TUI.inkSoft : TUI.inkFaint} style={{ textAlign: 'right', fontSize: 11.5 }}>{t.lat}</T>
                </div>
              );
            })}
          </div>
        </TUIPanel>

        {/* MCP servers + detail */}
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 8 }}>
          <TUIPanel title="mcp servers"
            badge={<T c={TUI.inkMuted} style={{ fontSize: 10.5 }}>3 up · 1 off · 1 error</T>}>
            <div style={{ padding: '0 0 4px' }}>
              {mcps.map((m, i) => {
                const stColor = m.st === 'connected' ? TUI.green
                  : m.st === 'error' ? TUI.red
                  : TUI.inkMuted;
                return (
                  <div key={m.n} style={{
                    display: 'flex', alignItems: 'center', gap: 12,
                    padding: '6px 12px', fontSize: 12.5,
                    borderBottom: i < mcps.length - 1 ? `1px dashed ${TUI.lineSoft}` : 'none',
                  }}>
                    <PulseDot c={stColor} size={6}/>
                    <T c={TUI.ink} bold style={{ width: 140 }}>{m.n}</T>
                    <T c={TUI.inkFaint} style={{ width: 50, fontSize: 11 }}>{m.v}</T>
                    <T c={TUI.inkMuted} style={{ flex: 1, fontSize: 11.5 }}>{m.h}</T>
                    <T c={TUI.accent} style={{ fontSize: 11 }}>{m.tools} tools</T>
                    <T c={stColor} style={{ width: 80, textAlign: 'right', fontSize: 11.5 }}>
                      {m.st === 'connected' ? '✓ up' : m.st === 'error' ? '✗ failed' : '○ off'}
                    </T>
                  </div>
                );
              })}
            </div>
          </TUIPanel>

          {/* Selected tool detail */}
          <TUIPanel title="detail · shell" badge={<T c={TUI.red} style={{ fontSize: 10.5 }}>exec risk</T>} flex={1}>
            <div style={{ padding: '8px 12px', fontSize: 12, color: TUI.inkSoft, lineHeight: '17px' }}>
              <T italic c={TUI.inkMuted}>description —</T>
              <div style={{ marginTop: 3, marginBottom: 8 }}>
                runs a single shell command in a sandboxed subprocess. stdout/stderr stream back as the
                tool result. cwd defaults to <Code>$DAIMON_CWD</Code>.
              </div>
              <div style={{
                background: TUI.bgDeep, border: `1px solid ${TUI.line}`,
                borderRadius: 2, padding: 8, fontSize: 11.5, color: TUI.ink,
              }}>
                <T c={TUI.inkMuted}>signature:</T>{'\n'}
                <span style={{ color: TUI.pink }}>shell</span>
                <span style={{ color: TUI.inkSoft }}>{`(`}</span>
                <span style={{ color: TUI.accent }}>cmd</span>
                <span style={{ color: TUI.inkSoft }}>{`: `}</span>
                <span style={{ color: TUI.amber }}>string</span>
                <span style={{ color: TUI.inkSoft }}>{`, `}</span>
                <span style={{ color: TUI.accent }}>timeout</span>
                <span style={{ color: TUI.inkSoft }}>{`?: `}</span>
                <span style={{ color: TUI.amber }}>number</span>
                <span style={{ color: TUI.inkSoft }}>{`)`}</span>
              </div>
              <div style={{
                marginTop: 8, display: 'grid',
                gridTemplateColumns: '110px 1fr', gap: '3px 12px',
                fontSize: 11.5,
              }}>
                <T c={TUI.inkMuted}>permission</T><T c={TUI.amber}>ask (per-command)</T>
                <T c={TUI.inkMuted}>allowlist</T><T c={TUI.inkSoft}>bun, npm, jest, tsc, git, ls, cat</T>
                <T c={TUI.inkMuted}>denylist</T><T c={TUI.red}>rm -rf, curl | sh, sudo</T>
                <T c={TUI.inkMuted}>timeout</T><T c={TUI.inkSoft}>30s default · 120s max</T>
                <T c={TUI.inkMuted}>sandbox</T><T c={TUI.inkSoft}>bwrap (linux) · sandbox-exec (darwin)</T>
              </div>
            </div>
          </TUIPanel>
        </div>
      </div>

      <TUIFooter hints={[
        { k: 'space', l: 'toggle enabled' },
        { k: '↵', l: 'open detail' },
        { k: 'a', l: 'add MCP server' },
        { k: 'd', l: 'remove' },
        { k: '/', l: 'filter' },
      ]}/>
    </TUIScreen>
  );
}

// ─────────────────────────────────────────────────────────────────
// SCREEN 6 — Sessions browser + model picker (split)
// ─────────────────────────────────────────────────────────────────
function TUI_Sessions({ width = 1400, height = 860 }) {
  const sessions = [
    { id: 'a8f3', name: 'payment-anomalies',          when: '14:32 today',     model: 'sonnet-4.5', turns: 47, cost: '$0.42', tokens: '43.2k', branch: 'main' },
    { id: 'b2c1', name: 'webhook async refactor',     when: 'yesterday 18:04', model: 'sonnet-4.5', turns: 18, cost: '$0.14', tokens: '12.1k', branch: 'fix/webhook' },
    { id: 'c9d0', name: 'helix → migrate to bun',     when: '2d ago',          model: 'opus',       turns: 91, cost: '$1.20', tokens: '180k',  branch: 'migrate/bun' },
    { id: 'd4e2', name: 'sketch retry semantics',     when: '3d ago',          model: 'haiku',      turns: 6,  cost: '$0.01', tokens: '2.4k',  branch: 'spike/retry' },
    { id: 'e8a1', name: 'rate-limit middleware',      when: 'mon',             model: 'sonnet-4.5', turns: 22, cost: '$0.18', tokens: '14.8k', branch: 'feat/rl' },
    { id: 'f3b9', name: 'sql planner deep-dive',      when: 'sun',             model: 'opus',       turns: 64, cost: '$0.92', tokens: '120k',  branch: 'study' },
    { id: 'g1c4', name: 'tui keymap exploration',     when: 'last week',       model: 'sonnet-4.5', turns: 11, cost: '$0.06', tokens: '5.2k',  branch: 'design/tui' },
    { id: 'h7d2', name: 'CI cache investigation',     when: '2w ago',          model: 'sonnet-4.5', turns: 33, cost: '$0.31', tokens: '22.1k', branch: 'fix/ci' },
  ];

  const models = [
    { n: 'sonnet-4.5',   v: 'anthropic', ctx: '200k', cost: '$3 / $15', sp: 'fast',  on: true },
    { n: 'opus-4.1',     v: 'anthropic', ctx: '200k', cost: '$15 / $75', sp: 'deep' },
    { n: 'haiku-4.5',    v: 'anthropic', ctx: '200k', cost: '$1 / $5',   sp: 'snappy' },
    { n: 'gpt-5-2025',   v: 'openai',    ctx: '256k', cost: '$5 / $20',  sp: 'fast' },
    { n: 'llama-4-405b', v: 'meta · local', ctx: '128k', cost: 'free',   sp: 'slow' },
    { n: 'qwen3-72b',    v: 'ollama · local', ctx: '128k', cost: 'free', sp: 'medium' },
  ];

  return (
    <TUIScreen width={width} height={height}>
      <TUITopBar status="resuming…" mode="plan" cost="$0.00"/>

      <div style={{ flex: 1, display: 'flex', gap: 10, minHeight: 0 }}>
        {/* Sessions */}
        <TUIPanel
          title="sessions"
          badge={<span style={{ display: 'flex', gap: 10 }}>
            <T c={TUI.inkMuted} style={{ fontSize: 10.5 }}>8 of 124</T>
            <T c={TUI.inkMuted} style={{ fontSize: 10.5 }}><T c={TUI.accent}>/</T> filter</T>
          </span>}
          flex={1.6}>
          <div style={{ padding: '0 0 4px' }}>
            {/* Search line at top */}
            <div style={{
              padding: '8px 14px', borderBottom: `1px solid ${TUI.line}`,
              display: 'flex', alignItems: 'center', gap: 10, background: TUI.bgDeep,
            }}>
              <T c={TUI.accent} bold>/</T>
              <span style={{ flex: 1, color: TUI.ink, fontSize: 13 }}>web<Caret w={6} h={14}/></span>
              <T c={TUI.inkMuted} style={{ fontSize: 11 }}>2 matches</T>
            </div>

            {/* Header */}
            <div style={{
              display: 'grid',
              gridTemplateColumns: '46px 1fr 130px 80px 60px 70px 110px',
              padding: '5px 14px',
              fontSize: 10.5, color: TUI.inkMuted, letterSpacing: 1, textTransform: 'uppercase',
              borderBottom: `1px solid ${TUI.line}`,
            }}>
              <span>id</span><span>thread</span><span>when</span>
              <span style={{ textAlign: 'right' }}>turns</span>
              <span style={{ textAlign: 'right' }}>cost</span>
              <span style={{ textAlign: 'right' }}>tokens</span>
              <span style={{ textAlign: 'right' }}>branch</span>
            </div>

            {sessions.map((s, i) => {
              const active = i === 1; // "webhook async refactor" hilited (matches /web)
              const matchName = s.name.replace(/(web)/i, '⟨$1⟩');
              return (
                <div key={s.id} style={{
                  display: 'grid',
                  gridTemplateColumns: '46px 1fr 130px 80px 60px 70px 110px',
                  padding: '4px 14px', fontSize: 12.5, alignItems: 'center',
                  background: active ? TUI.accentBg : 'transparent',
                  borderLeft: active ? `2px solid ${TUI.accent}` : '2px solid transparent',
                  paddingLeft: active ? 12 : 14,
                  color: active ? TUI.ink : TUI.inkSoft,
                }}>
                  <T c={TUI.inkFaint} style={{ fontSize: 11 }}>{s.id}</T>
                  <span>
                    {s.name.split(/(web)/i).map((part, k) =>
                      /^web$/i.test(part)
                        ? <T key={k} c={TUI.accent} bold>{part}</T>
                        : <span key={k}>{part}</span>
                    )}
                  </span>
                  <T c={TUI.inkMuted} style={{ fontSize: 11.5 }}>{s.when}</T>
                  <T c={TUI.inkSoft} style={{ textAlign: 'right', fontSize: 11.5 }}>{s.turns}</T>
                  <T c={TUI.inkSoft} style={{ textAlign: 'right', fontSize: 11.5 }}>{s.cost}</T>
                  <T c={TUI.inkSoft} style={{ textAlign: 'right', fontSize: 11.5 }}>{s.tokens}</T>
                  <T c={TUI.pink} style={{ textAlign: 'right', fontSize: 11.5 }}>{s.branch}</T>
                </div>
              );
            })}
          </div>

          {/* Preview of selected */}
          <div style={{
            margin: '8px 14px 0', padding: 10,
            border: `1px dashed ${TUI.line}`, borderRadius: 2,
            background: TUI.bgDeep, fontSize: 12, color: TUI.inkSoft, lineHeight: '17px',
          }}>
            <div style={{
              fontSize: 10.5, color: TUI.inkMuted, textTransform: 'uppercase',
              letterSpacing: 1, marginBottom: 6,
            }}>preview · b2c1 · webhook async refactor</div>
            <T c={TUI.inkSoft}>▌ </T><T c={TUI.ink}>arnau</T>
            <T c={TUI.inkMuted}>: refactor the stripe webhook handler to async, keep retries, watch the timeout.</T>
            {'\n'}<T c={TUI.accent}>⫶ </T><T c={TUI.accent}>daimon</T>
            <T c={TUI.inkSoft}>: drafted three options — sketching the safest one first…</T>
            {'\n'}<T c={TUI.inkFaint}>—— last turn 21h ago · context restored from snapshot</T>
          </div>
        </TUIPanel>

        {/* Model picker */}
        <TUIPanel
          title="model"
          badge={<T c={TUI.inkMuted} style={{ fontSize: 10.5 }}>⌃M to summon</T>}
          width={420}>
          <div style={{ padding: '0 0 6px' }}>
            {models.map((m, i) => (
              <div key={m.n} style={{
                display: 'flex', alignItems: 'center', gap: 10,
                padding: '8px 14px', fontSize: 12.5,
                background: m.on ? TUI.accentBg : 'transparent',
                borderLeft: m.on ? `2px solid ${TUI.accent}` : '2px solid transparent',
                paddingLeft: m.on ? 12 : 14,
                borderBottom: i < models.length - 1 ? `1px dashed ${TUI.lineSoft}` : 'none',
              }}>
                <span style={{ width: 12 }}>{m.on
                  ? <T c={TUI.accent}>●</T>
                  : <T c={TUI.inkFaint}>○</T>}</span>
                <div style={{ flex: 1 }}>
                  <div style={{ color: m.on ? TUI.ink : TUI.inkSoft, fontWeight: m.on ? 500 : 400 }}>
                    {m.n}
                  </div>
                  <div style={{ fontSize: 10.5, color: TUI.inkMuted, marginTop: 1 }}>
                    {m.v} · ctx {m.ctx} · {m.cost}
                  </div>
                </div>
                <T c={m.sp === 'deep' ? TUI.amber : m.sp === 'fast' ? TUI.green : TUI.inkMuted}
                   style={{ fontSize: 11 }}>{m.sp}</T>
              </div>
            ))}
          </div>

          <div style={{
            margin: '8px 14px',
            padding: 10, background: TUI.bgDeep, border: `1px solid ${TUI.line}`,
            borderRadius: 2, fontSize: 11.5, color: TUI.inkSoft, lineHeight: '17px',
          }}>
            <T italic c={TUI.inkMuted}>active —</T>
            <div style={{ marginTop: 4 }}>
              <T c={TUI.ink} bold>sonnet-4.5</T> · <T c={TUI.inkMuted}>anthropic</T>
            </div>
            <div style={{ marginTop: 4, display: 'flex', gap: 14, fontSize: 11 }}>
              <span><T c={TUI.inkMuted}>thinking</T> <T c={TUI.accent}>on</T></span>
              <span><T c={TUI.inkMuted}>temp</T> <T c={TUI.ink}>0.4</T></span>
              <span><T c={TUI.inkMuted}>top-p</T> <T c={TUI.ink}>0.95</T></span>
            </div>
          </div>
        </TUIPanel>
      </div>

      <TUIFooter hints={[
        { k: '↵', l: 'resume thread' },
        { k: 'n', l: 'new from this' },
        { k: 'd', l: 'delete' },
        { k: 'm', l: 'change model' },
        { k: '/', l: 'filter' },
      ]}/>
    </TUIScreen>
  );
}

// ─────────────────────────────────────────────────────────────────
// SCREEN 7 — Error / permission denied
// ─────────────────────────────────────────────────────────────────
function TUI_Error({ width = 1400, height = 860 }) {
  return (
    <TUIScreen width={width} height={height}>
      <TUITopBar status="held" mode="build" cost="$0.142"/>

      <div style={{
        display: 'flex', alignItems: 'center', gap: 14, padding: '4px 4px 8px',
        fontSize: 12.5, color: TUI.inkMuted,
      }}>
        <T c={TUI.accent}>~/chat/</T>
        <T c={TUI.ink} bold>payment-anomalies</T>
        <T c={TUI.inkFaint}>·</T>
        <span>iter 4 · 47 turns</span>
        <span style={{ flex: 1 }}/>
        <T c={TUI.red} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <PulseDot c={TUI.red} size={6}/>
          tool denied — daimon paused
        </T>
      </div>

      <div style={{ flex: 1, display: 'flex', gap: 10, minHeight: 0 }}>
        {/* Main thread (truncated context) */}
        <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
          <div style={{ flex: 1, overflow: 'hidden', paddingRight: 4 }}>
            <MsgDaimon time="14:34">
              <div style={{ marginBottom: 6 }}>
                pulling the handler source —
              </div>
              <ToolLine status="done" name="grep" input="'webhook timeout|retry exhausted'"
                stat="21 matches" tokens={612} duration="89ms"/>
              <ToolLine status="done" name="git_log" input="services/payments/ since=2026-04-17"
                stat="2 commits" tokens={284} duration="54ms"/>
              <ToolLine status="error" name="read_file"
                input="/etc/daimon/secrets.env"
                stat="denied" tokens={48} duration="12ms"
                expanded
                output={`PermissionError: path outside workspace root
  requested  /etc/daimon/secrets.env
  workspace  ~/projects/helix
  policy     fs.read.allowlist  →  no rule matched
  hint       daimon does not auto-escalate. you may grant once, always, or deny.`}/>

              <div style={{
                marginTop: 10, padding: 12,
                border: `1px solid ${TUI.red}55`, borderLeft: `3px solid ${TUI.red}`,
                background: TUI.redBg, borderRadius: 2,
              }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 8 }}>
                  <T c={TUI.red} bold style={{ fontSize: 13 }}>⚠ permission required</T>
                  <T c={TUI.inkFaint}>·</T>
                  <T c={TUI.inkMuted} style={{ fontSize: 12 }}>
                    tool <Code c={TUI.red}>read_file</Code> wants to read outside the workspace
                  </T>
                </div>

                <div style={{ fontSize: 12.5, color: TUI.inkSoft, lineHeight: '18px' }}>
                  <T italic c={TUI.inkMuted}>daimon explains —</T>
                  {' '}i need the webhook secret to verify the signature path, but{' '}
                  <Code c={TUI.red}>/etc/daimon/secrets.env</Code> sits outside{' '}
                  <Code>~/projects/helix</Code>. you decide whether to expose it.
                </div>

                <div style={{
                  marginTop: 12, padding: 10, background: TUI.bgDeep, borderRadius: 2,
                  border: `1px solid ${TUI.line}`, fontFamily: TUI.mono, fontSize: 11.5,
                }}>
                  <div style={{ color: TUI.inkMuted, marginBottom: 4 }}>requested call:</div>
                  <span style={{ color: TUI.pink }}>read_file</span>
                  <span style={{ color: TUI.inkSoft }}>{`({ `}</span>
                  <span style={{ color: TUI.accent }}>path</span>
                  <span style={{ color: TUI.inkSoft }}>{`: `}</span>
                  <span style={{ color: TUI.amber }}>"/etc/daimon/secrets.env"</span>
                  <span style={{ color: TUI.inkSoft }}>{` })`}</span>
                </div>

                <div style={{
                  marginTop: 12, display: 'flex', flexWrap: 'wrap', gap: 8,
                }}>
                  {[
                    { k: 'a', l: 'allow once',          c: TUI.green },
                    { k: 'A', l: 'allow always · trust', c: TUI.green, dim: true },
                    { k: 'd', l: 'deny',                c: TUI.red },
                    { k: 'D', l: 'deny + never ask',    c: TUI.red, dim: true },
                    { k: 'e', l: 'edit path',           c: TUI.amber },
                    { k: 's', l: 'skip · let daimon adapt', c: TUI.accent },
                  ].map(b => (
                    <span key={b.k} style={{
                      display: 'inline-flex', gap: 6, alignItems: 'center',
                      padding: '3px 9px', borderRadius: 2,
                      border: `1px solid ${b.c}33`,
                      background: b.dim ? 'transparent' : `${b.c}10`,
                      opacity: b.dim ? 0.65 : 1, fontSize: 12,
                    }}>
                      <T c={b.c} bold>{b.k}</T>
                      <T c={TUI.inkSoft}>{b.l}</T>
                    </span>
                  ))}
                </div>
              </div>

              <div style={{ marginTop: 12, fontSize: 12.5, color: TUI.inkMuted }}>
                <T italic>i can also work around this —</T> use{' '}
                <Code>config.example.env</Code> in the repo, infer the signature flow, and flag the
                secret for you to inject manually.
                <span style={{ marginLeft: 6 }}><Caret/></span>
              </div>
            </MsgDaimon>
          </div>

          <TUIInput placeholder="answer the prompt above, or type to override…" mode="build"/>
        </div>

        {/* Sidebar — policy + recent denials */}
        <div style={{ width: 360, display: 'flex', flexDirection: 'column', gap: 8 }}>
          <TUIPanel title="active policy">
            <div style={{ padding: '8px 12px', fontSize: 12 }}>
              <div style={{ display: 'grid', gridTemplateColumns: '92px 1fr', gap: '3px 10px' }}>
                <T c={TUI.inkMuted}>fs.read</T>     <T c={TUI.green}>workspace · allowlist</T>
                <T c={TUI.inkMuted}>fs.write</T>    <T c={TUI.amber}>workspace · ask</T>
                <T c={TUI.inkMuted}>shell</T>       <T c={TUI.amber}>allowlist · ask</T>
                <T c={TUI.inkMuted}>net</T>         <T c={TUI.red}>disabled</T>
                <T c={TUI.inkMuted}>spawn</T>       <T c={TUI.green}>allowed · capped 3</T>
              </div>
              <div style={{
                marginTop: 8, paddingTop: 8, borderTop: `1px dashed ${TUI.line}`,
                fontSize: 11.5, color: TUI.inkMuted,
              }}>
                <T italic>policy file —</T>{' '}
                <Code>~/.daimon/policy.toml</Code>
              </div>
            </div>
          </TUIPanel>

          <TUIPanel title="recent denials" flex={1}>
            <div style={{ padding: '6px 12px', fontSize: 12 }}>
              {[
                { t: '14:34', p: '/etc/daimon/secrets.env', tool: 'read_file', why: 'outside workspace' },
                { t: '14:18', p: 'rm -rf node_modules',     tool: 'shell',     why: 'denylist · rm -rf' },
                { t: '13:51', p: 'https://api.stripe.com',  tool: 'web_fetch', why: 'net disabled' },
                { t: '13:42', p: 'sudo apt update',         tool: 'shell',     why: 'denylist · sudo' },
              ].map((d, i) => (
                <div key={i} style={{
                  padding: '4px 0',
                  borderBottom: i < 3 ? `1px dashed ${TUI.lineSoft}` : 'none',
                }}>
                  <div style={{ display: 'flex', gap: 8, fontSize: 11, color: TUI.inkMuted }}>
                    <T c={TUI.red}>✗</T>
                    <span>{d.t}</span>
                    <T c={TUI.pink}>{d.tool}</T>
                    <span style={{ flex: 1 }}/>
                    <T c={TUI.inkMuted} italic>{d.why}</T>
                  </div>
                  <div style={{ paddingLeft: 18, fontSize: 11.5, color: TUI.inkSoft }}>
                    {d.p}
                  </div>
                </div>
              ))}
            </div>
          </TUIPanel>

          <TUIPanel title="telemetry">
            <div style={{ padding: '6px 12px', fontSize: 12 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', padding: '2px 0' }}>
                <T c={TUI.inkMuted}>cost so far</T><T c={TUI.ink}>$0.142</T>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', padding: '2px 0' }}>
                <T c={TUI.inkMuted}>tokens</T><T c={TUI.ink}>43.2k</T>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', padding: '2px 0' }}>
                <T c={TUI.inkMuted}>denied calls</T><T c={TUI.red}>4 today</T>
              </div>
            </div>
          </TUIPanel>
        </div>
      </div>

      <TUIFooter hints={[
        { k: 'a/A', l: 'allow once / always' },
        { k: 'd/D', l: 'deny / never ask' },
        { k: 'e', l: 'edit path' },
        { k: 'p', l: 'open policy file' },
      ]}/>
    </TUIScreen>
  );
}

Object.assign(window, { TUI_Slash, TUI_Tools, TUI_Sessions, TUI_Error });
