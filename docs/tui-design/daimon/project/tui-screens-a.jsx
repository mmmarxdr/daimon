// Daimon TUI — screens
// Each screen is composed of TUI primitives from tui.jsx.

// ─────────────────────────────────────────────────────────────────
// SCREEN 1 — Boot / welcome
// ─────────────────────────────────────────────────────────────────
function TUI_Welcome({ width = 1200, height = 760 }) {
  const ascii = [
    '       ▄▄▄▄▄                                                        ',
    '     ▄█▀   ▀█▄    ▐█▌                                               ',
    '    █▀       ▀█   ▐█▌  ┌───────────────────────────────────┐        ',
    '   █▀  ▄▄▄    █   ▐█▌  │  daimon · v0.4.2 · MIT             │        ',
    '   █  ▐█ █▌   █   ▐█▌  │  agent runtime, on your hardware   │        ',
    '   █▄  ▀▀▀    █   ▐█▌  └───────────────────────────────────┘        ',
    '    █▄       ▄█   ▐█▌                                               ',
    '     ▀█▄▄▄▄▄█▀     ▀                                                ',
  ];
  return (
    <TUIScreen width={width} height={height}>
      <TUITopBar status="awake" cost="$0.00" mode="plan"/>
      <div style={{
        flex: 1, display: 'flex', flexDirection: 'column',
        alignItems: 'center', justifyContent: 'center', gap: 28,
        padding: '0 40px',
      }}>
        {/* ASCII δ + wordmark */}
        <div style={{ position: 'relative' }}>
          <div style={{
            fontFamily: TUI.mono, fontSize: 13, lineHeight: '17px',
            color: TUI.accent, whiteSpace: 'pre', letterSpacing: 0,
            textShadow: `0 0 12px ${TUI.accent}55`,
          }}>{ascii.join('\n')}</div>
        </div>

        <div style={{ textAlign: 'center', maxWidth: 600 }}>
          <T italic c={TUI.inkSoft} style={{ fontSize: 17 }}>
            speak, and daimon listens.
          </T>
          <div style={{
            marginTop: 8, fontSize: 12, color: TUI.inkMuted,
            display: 'flex', justifyContent: 'center', gap: 18,
          }}>
            <span><T c={TUI.accent}>⇥</T> /commands</span>
            <span><T c={TUI.accent}>⌃R</T> resume last</span>
            <span><T c={TUI.accent}>⌃C</T> exit</span>
          </div>
        </div>

        {/* Recent sessions + boot diagnostics, side by side */}
        <div style={{ display: 'flex', gap: 14, width: '100%', maxWidth: 940 }}>
          <TUIPanel title="resume" flex={1.2}>
            <div style={{ padding: '6px 10px', fontSize: 12 }}>
              {[
                { id: 'a8f3', name: 'payment-anomalies', meta: '14:32 · sonnet-4.5 · 47 turns · $0.42', active: true },
                { id: 'b2c1', name: 'webhook async refactor', meta: 'yesterday · sonnet-4.5 · 18 turns · $0.14' },
                { id: 'c9d0', name: 'helix → migrate to bun', meta: '2d ago · opus · 91 turns · $1.20' },
                { id: 'd4e2', name: 'sketch retry semantics', meta: '3d ago · haiku · 6 turns · $0.01' },
              ].map(s => (
                <div key={s.id} style={{
                  display: 'flex', alignItems: 'center', gap: 12,
                  padding: '3px 0',
                  color: s.active ? TUI.accent : TUI.inkSoft,
                }}>
                  <T c={TUI.inkFaint} style={{ width: 36 }}>{s.id}</T>
                  <span style={{ flex: 1 }}>{s.name}</span>
                  <T c={TUI.inkMuted} style={{ fontSize: 11 }}>{s.meta}</T>
                  {s.active && <T c={TUI.accent}>◀</T>}
                </div>
              ))}
            </div>
          </TUIPanel>

          <TUIPanel title="environment" flex={1}>
            <div style={{ padding: '6px 10px', fontSize: 12, color: TUI.inkSoft }}>
              {[
                ['runtime',  'daimon 0.4.2', TUI.ink],
                ['model',    'anthropic / sonnet-4.5', TUI.ink],
                ['cwd',      '~/projects/helix', TUI.ink],
                ['branch',   'main (clean)', TUI.green],
                ['tools',    '12 builtin · 3 mcp', TUI.ink],
                ['memory',   '1,284 facts loaded', TUI.ink],
                ['context',  '0 / 200k', TUI.inkMuted],
              ].map(([k, v, c]) => (
                <div key={k} style={{ display: 'flex', padding: '2px 0' }}>
                  <T c={TUI.inkMuted} style={{ width: 88 }}>{k}</T>
                  <T c={c}>{v}</T>
                </div>
              ))}
              <div style={{ marginTop: 8, color: TUI.green, display: 'flex', alignItems: 'center', gap: 6 }}>
                <PulseDot c={TUI.green} size={5}/>
                <span>all systems green</span>
              </div>
            </div>
          </TUIPanel>
        </div>
      </div>

      {/* Input bar */}
      <div style={{ maxWidth: 940, alignSelf: 'center', width: '100%' }}>
        <TUIInput placeholder="what shall we build today?" mode="plan"/>
      </div>

      <TUIFooter hints={[
        { k: '/', l: 'commands' },
        { k: '⇥', l: 'switch agent' },
        { k: '⌃P', l: 'palette' },
        { k: '?', l: 'help' },
      ]}/>
    </TUIScreen>
  );
}

// ─────────────────────────────────────────────────────────────────
// SCREEN 2 — Chat activo (hero) — tools, subagente, todolist, telemetría
// ─────────────────────────────────────────────────────────────────
function TUI_Chat({ width = 1400, height = 860 }) {
  return (
    <TUIScreen width={width} height={height}>
      <TUITopBar status="working" cost="$0.142" mode="build"/>

      {/* Two-column body */}
      <div style={{ flex: 1, display: 'flex', gap: 10, minHeight: 0 }}>
        {/* Main thread */}
        <div style={{
          flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column',
          overflow: 'hidden',
        }}>
          {/* Session header / breadcrumb */}
          <div style={{
            fontSize: 12, color: TUI.inkMuted,
            padding: '0 4px 6px',
            display: 'flex', gap: 14, alignItems: 'center',
          }}>
            <T c={TUI.accent}>~/chat/</T>
            <T c={TUI.ink} bold>payment-anomalies</T>
            <T c={TUI.inkFaint}>·</T>
            <span>iter 4 · 47 turns</span>
            <T c={TUI.inkFaint}>·</T>
            <span>tokens <T c={TUI.inkSoft}>34.2k in · 8.9k out</T></span>
            <span style={{ flex: 1 }}/>
            <T italic c={TUI.inkMuted}>autosave · 12s ago</T>
          </div>

          {/* Thread */}
          <div style={{ flex: 1, overflow: 'hidden', paddingRight: 4 }}>
            <MsgUser time="14:32" name="arnau">
              analyze yesterday's payment logs, find the anomaly, and propose a conservative patch.
              run the test suite after.
            </MsgUser>

            <MsgDaimon time="14:32">
              <Reasoning duration="6s" open>
                two recent deploys touched payments. likely culprit: <Code>2c1d9e7</Code> — the async
                refactor kept the old 2s timeout. i'll verify via logs + git, draft a minimal patch,
                then spawn a test-runner.
              </Reasoning>

              <div style={{ marginBottom: 6 }}>found the window. pulling sources now —</div>

              <ToolLine status="done" name="read_file" input="/var/log/payments/2026-04-18.log"
                stat="500 lines" tokens={1842} cost="$0.006" duration="142ms"/>
              <ToolLine status="done" name="grep" input="'webhook timeout|retry exhausted'"
                stat="21 matches" tokens={612} cost="$0.002" duration="89ms"/>
              <ToolLine status="done" name="git_log" input="services/payments/ since=2026-04-17"
                stat="2 commits" tokens={284} cost="$0.001" duration="54ms"/>
              <ToolLine status="error" name="read_file" input="services/payments/webhook.ts"
                stat="denied" tokens={48} duration="12ms"/>
              <ToolLine status="done" name="read_file" input="./services/payments/webhook.ts"
                stat="48 lines" tokens={920} cost="$0.003" duration="38ms"
                expanded
                output={`  20  export async function handle(req: Request): Promise<Response> {
  21    const ev = await stripe.webhooks.constructEvent(
  22      await req.text(), req.headers.get('stripe-signature'),
  23      WEBHOOK_SECRET, undefined, { timeoutMs: 2000 }   // ← suspect
  24    );
  25    return await withRetry(() => process(ev), { max: 3 });
  26  }`}/>

              <Subagent name="test-runner" task="verify proposed patch against suite"
                status="running" tokens={2148} cost="$0.008" duration="4.8s">
                <ToolLine status="done" name="shell" input="bun test webhook.test.ts" stat="exit 0"
                  tokens={420} duration="2.1s"/>
                <ToolLine status="done" name="shell" input="bun test --coverage" stat="12/12 pass"
                  tokens={812} duration="2.4s"/>
                <ToolLine status="running" name="shell" input="bun test integration/payments"
                  tokens={920}/>
              </Subagent>

              <div style={{ marginTop: 6, marginBottom: 4 }}>
                <Spinner c={TUI.amber}/> drafting the patch
                — <T italic c={TUI.inkMuted}>holding off until the integration run lands.</T>
                <Caret/>
              </div>
            </MsgDaimon>
          </div>

          <TUIInput placeholder="add a follow-up, or ⌃C to interrupt…" mode="build"/>
        </div>

        {/* Right sidebar */}
        <div style={{
          width: 320, display: 'flex', flexDirection: 'column', gap: 8,
          minHeight: 0,
        }}>
          {/* Todolist */}
          <TUIPanel title="todolist" badge={<T c={TUI.inkMuted} style={{ fontSize: 10.5 }}>2/5 · auto</T>}>
            <div style={{ padding: '4px 10px', fontSize: 12 }}>
              {[
                { st: 'done',     l: 'pull yesterday\'s payment logs' },
                { st: 'done',     l: 'isolate the error cluster window' },
                { st: 'running',  l: 'identify the root commit' },
                { st: 'pending',  l: 'draft a minimal patch (2 hunks)' },
                { st: 'pending',  l: 'verify with full test run' },
              ].map((t, i) => {
                const g = t.st === 'done' ? <T c={TUI.accent}>✓</T>
                  : t.st === 'running' ? <Spinner c={TUI.amber}/>
                  : <T c={TUI.inkFaint}>○</T>;
                return (
                  <div key={i} style={{
                    display: 'flex', gap: 10, padding: '3px 0',
                    color: t.st === 'done' ? TUI.inkMuted : TUI.inkSoft,
                    textDecoration: t.st === 'done' ? 'line-through' : 'none',
                    textDecorationColor: TUI.inkFaint,
                  }}>
                    <span style={{ width: 12, textAlign: 'center', textDecoration: 'none' }}>{g}</span>
                    <span style={{ flex: 1 }}>{t.l}</span>
                    {t.st === 'running' && <T c={TUI.amber} style={{ fontSize: 10.5 }}>now</T>}
                  </div>
                );
              })}
              <div style={{
                marginTop: 6, paddingTop: 6, borderTop: `1px dashed ${TUI.line}`,
                fontSize: 11, color: TUI.inkMuted,
                display: 'flex', alignItems: 'center', gap: 8,
              }}>
                <T c={TUI.accent}>+</T>
                <T italic>add a step…</T>
              </div>
            </div>
          </TUIPanel>

          {/* Context meter */}
          <TUIPanel title="context window">
            <div style={{ padding: '6px 10px', fontSize: 12 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                <T c={TUI.inkSoft}>43,182 / 200,000</T>
                <T c={TUI.accent}>21.6%</T>
              </div>
              <div style={{ marginBottom: 8 }}>
                <BlockBar pct={21.6} width={28}/>
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '2px 12px', color: TUI.inkMuted, fontSize: 11.5 }}>
                <span>system</span>      <T c={TUI.inkSoft} style={{ textAlign: 'right' }}>2.1k</T>
                <span>memory</span>      <T c={TUI.inkSoft} style={{ textAlign: 'right' }}>4.8k</T>
                <span>conversation</span><T c={TUI.inkSoft} style={{ textAlign: 'right' }}>21.0k</T>
                <span>tool output</span> <T c={TUI.inkSoft} style={{ textAlign: 'right' }}>14.2k</T>
                <span>workspace</span>   <T c={TUI.inkSoft} style={{ textAlign: 'right' }}>1.1k</T>
              </div>
            </div>
          </TUIPanel>

          {/* Telemetry */}
          <TUIPanel title="telemetry" badge={<T c={TUI.green} style={{ fontSize: 10.5 }}>live</T>} flex={1}>
            <div style={{ padding: '6px 10px', fontSize: 12 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', padding: '2px 0' }}>
                <T c={TUI.inkMuted}>tokens in</T><T c={TUI.ink}>34,210</T>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', padding: '2px 0' }}>
                <T c={TUI.inkMuted}>tokens out</T><T c={TUI.ink}>8,942</T>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', padding: '2px 0' }}>
                <T c={TUI.inkMuted}>cost</T><T c={TUI.ink}>$0.142</T>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', padding: '2px 0' }}>
                <T c={TUI.inkMuted}>elapsed</T><T c={TUI.ink}>2m 14s</T>
              </div>

              <div style={{ marginTop: 8, fontSize: 10.5, color: TUI.inkMuted, textTransform: 'uppercase', letterSpacing: 1 }}>── by tool</div>
              {[
                { n: 'read_file', c: 3, t: '2.8k' },
                { n: 'grep',      c: 2, t: '0.9k' },
                { n: 'git_log',   c: 1, t: '0.3k' },
                { n: 'shell',     c: 3, t: '2.1k' },
              ].map(t => (
                <div key={t.n} style={{ display: 'flex', padding: '1.5px 0', fontSize: 11.5 }}>
                  <T c={TUI.inkSoft} style={{ flex: 1 }}>{t.n}</T>
                  <T c={TUI.inkMuted} style={{ width: 36, textAlign: 'right' }}>×{t.c}</T>
                  <T c={TUI.ink} style={{ width: 56, textAlign: 'right' }}>{t.t}</T>
                </div>
              ))}

              <div style={{ marginTop: 8, fontSize: 10.5, color: TUI.inkMuted, textTransform: 'uppercase', letterSpacing: 1 }}>── subagents</div>
              <div style={{ display: 'flex', padding: '1.5px 0', fontSize: 11.5, color: TUI.inkSoft }}>
                <T c={TUI.pink}>↳</T>&nbsp;<span style={{ flex: 1 }}>test-runner</span>
                <T c={TUI.amber}>2.1k</T>
              </div>
            </div>
          </TUIPanel>
        </div>
      </div>

      <TUIFooter hints={[
        { k: '⇥', l: '/commands' },
        { k: '⌃C', l: 'interrupt' },
        { k: '⌃R', l: 'retry turn' },
        { k: '⌃E', l: 'edit last' },
        { k: '⌃S', l: 'save session' },
      ]}/>
    </TUIScreen>
  );
}

// ─────────────────────────────────────────────────────────────────
// SCREEN 3 — Diff approval
// ─────────────────────────────────────────────────────────────────
function TUI_Diff({ width = 1400, height = 860 }) {
  const diff = [
    { n:  19, k: ' ', t: ' export async function handle(req: Request): Promise<Response> {' },
    { n:  20, k: '-', t: '   const ev = await stripe.webhooks.constructEvent(' },
    { n:  21, k: '-', t: "     await req.text(), req.headers.get('stripe-signature')," },
    { n:  22, k: '-', t: '     WEBHOOK_SECRET, undefined, { timeoutMs: 2000 }' },
    { n:  23, k: '-', t: '   );' },
    { n:  20, k: '+', t: '   const body = await req.text();' },
    { n:  21, k: '+', t: "   const sig  = req.headers.get('stripe-signature');" },
    { n:  22, k: '+', t: '   const ev = await stripe.webhooks.constructEvent(' },
    { n:  23, k: '+', t: '     body, sig, WEBHOOK_SECRET, undefined,' },
    { n:  24, k: '+', t: '     { timeoutMs: ASYNC_TIMEOUT_MS }   // 5000, see config.ts' },
    { n:  25, k: '+', t: '   );' },
    { n:  26, k: ' ', t: '   return await withRetry(' },
    { n:  27, k: '-', t: '     () => process(ev),' },
    { n:  28, k: '+', t: '     () => process(ev, { traceId: ev.id }),' },
    { n:  29, k: ' ', t: '     { max: 3 }' },
    { n:  30, k: ' ', t: '   );' },
    { n:  31, k: ' ', t: ' }' },
  ];
  const colorOf = k => k === '+' ? TUI.green : k === '-' ? TUI.red : TUI.inkSoft;
  const bgOf = k => k === '+' ? 'rgba(122,186,138,0.07)' : k === '-' ? 'rgba(227,135,117,0.07)' : 'transparent';

  return (
    <TUIScreen width={width} height={height}>
      <TUITopBar status="awaiting approval" cost="$0.142" mode="build"/>

      <div style={{
        display: 'flex', alignItems: 'center', gap: 14, padding: '4px 6px 8px',
        fontSize: 12.5, color: TUI.inkMuted,
      }}>
        <T c={TUI.amber}>◆</T>
        <T c={TUI.ink} bold>proposed patch</T>
        <T c={TUI.inkFaint}>·</T>
        <T c={TUI.inkSoft}>services/payments/webhook.ts</T>
        <T c={TUI.inkFaint}>·</T>
        <T c={TUI.green}>+7</T>
        <T c={TUI.red}>−4</T>
        <T c={TUI.inkFaint}>·</T>
        <span>hunk <T c={TUI.ink}>1</T> / 2</span>
        <span style={{ flex: 1 }}/>
        <T italic c={TUI.inkMuted}>daimon awaits your nod.</T>
      </div>

      <div style={{ flex: 1, display: 'flex', gap: 10, minHeight: 0 }}>
        {/* Diff body */}
        <TUIPanel title="webhook.ts · hunk 1/2" flex={1.8}
          badge={<span style={{ display: 'flex', gap: 8 }}>
            <T c={TUI.inkMuted} style={{ fontSize: 10.5 }}><T c={TUI.accent}>j</T>/<T c={TUI.accent}>k</T> line</T>
            <T c={TUI.inkMuted} style={{ fontSize: 10.5 }}><T c={TUI.accent}>n</T>/<T c={TUI.accent}>p</T> hunk</T>
          </span>}>
          <div style={{ padding: '6px 0', fontSize: 12.5, fontFamily: TUI.mono }}>
            {diff.map((row, i) => (
              <div key={i} style={{
                display: 'flex', padding: '0 10px',
                background: bgOf(row.k),
                borderLeft: `2px solid ${row.k === '+' ? TUI.green : row.k === '-' ? TUI.red : 'transparent'}`,
              }}>
                <span style={{ width: 36, color: TUI.inkFaint, textAlign: 'right', paddingRight: 10, userSelect: 'none' }}>
                  {row.k === '+' ? '' : row.n}
                </span>
                <span style={{ width: 36, color: TUI.inkFaint, textAlign: 'right', paddingRight: 10, userSelect: 'none' }}>
                  {row.k === '-' ? '' : row.n}
                </span>
                <span style={{ width: 14, color: colorOf(row.k), fontWeight: 600 }}>{row.k}</span>
                <span style={{ color: colorOf(row.k), whiteSpace: 'pre', flex: 1 }}>{row.t}</span>
              </div>
            ))}
            <div style={{
              padding: '8px 10px', marginTop: 6,
              fontSize: 11.5, color: TUI.inkMuted,
              borderTop: `1px dashed ${TUI.line}`,
              display: 'flex', gap: 14,
            }}>
              <T italic>summary —</T>
              <span>relax timeout to <T c={TUI.ink}>ASYNC_TIMEOUT_MS</T> (5s, env-controlled), and thread the
              <T c={TUI.ink}> traceId</T> through retries so duplicates collapse.</span>
            </div>
          </div>
        </TUIPanel>

        {/* Right sidebar: hunk navigator + rationale */}
        <div style={{ width: 360, display: 'flex', flexDirection: 'column', gap: 8 }}>
          <TUIPanel title="hunks">
            <div style={{ padding: '6px 10px', fontSize: 12 }}>
              {[
                { i: 1, file: 'webhook.ts',     lines: '+7 −4', active: true },
                { i: 2, file: 'config.ts',      lines: '+2 −0' },
              ].map(h => (
                <div key={h.i} style={{
                  display: 'flex', gap: 10, padding: '3px 0',
                  color: h.active ? TUI.accent : TUI.inkSoft,
                  background: h.active ? TUI.accentBg : 'transparent',
                  margin: '0 -4px', padding: '3px 6px', borderRadius: 2,
                  borderLeft: h.active ? `2px solid ${TUI.accent}` : '2px solid transparent',
                }}>
                  <T c={h.active ? TUI.accent : TUI.inkFaint}>{h.i}</T>
                  <span style={{ flex: 1 }}>{h.file}</span>
                  <T c={TUI.green}>{h.lines.split(' ')[0]}</T>
                  <T c={TUI.red}>{h.lines.split(' ')[1]}</T>
                </div>
              ))}
            </div>
          </TUIPanel>

          <TUIPanel title="why this change">
            <div style={{ padding: '8px 10px', fontSize: 12, color: TUI.inkSoft, lineHeight: '17px' }}>
              <T italic c={TUI.inkMuted}>rationale —</T>
              <div style={{ marginTop: 4 }}>
                the async refactor in <Code>2c1d9e7</Code> kept the legacy 2s timeout while moving stripe
                verification to the event loop. under load, p99 latency jumps to <T c={TUI.red}>4.2s</T>.
                threading <Code>traceId</Code> prevents retry duplicates from showing as fresh tx.
              </div>
              <div style={{
                marginTop: 8, paddingTop: 8, borderTop: `1px dashed ${TUI.line}`,
                fontSize: 11.5, color: TUI.inkMuted,
              }}>
                refs: <T c={TUI.accent}>logs/2026-04-18.log:314</T>, <T c={TUI.accent}>git 2c1d9e7</T>
              </div>
            </div>
          </TUIPanel>

          <TUIPanel title="impact" flex={1}>
            <div style={{ padding: '8px 10px', fontSize: 12 }}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr auto auto', gap: '4px 10px' }}>
                <T c={TUI.inkMuted}></T><T c={TUI.inkMuted} style={{ fontSize: 10.5 }}>before</T><T c={TUI.inkMuted} style={{ fontSize: 10.5 }}>after</T>
                <T c={TUI.inkSoft}>p99 latency</T> <T c={TUI.red}>4.2s</T>  <T c={TUI.green}>~0.9s</T>
                <T c={TUI.inkSoft}>timeout rate</T><T c={TUI.red}>2.1%</T>  <T c={TUI.green}>0.04%</T>
                <T c={TUI.inkSoft}>retry duplicates</T><T c={TUI.red}>18</T>  <T c={TUI.green}>0</T>
              </div>
              <div style={{
                marginTop: 10, padding: 8, background: TUI.bgDeep, borderRadius: 2,
                border: `1px solid ${TUI.line}`, fontSize: 11.5, color: TUI.inkMuted,
              }}>
                <T italic>tests —</T> <T c={TUI.green}>12/12 unit</T>, <Spinner c={TUI.amber}/>
                <span> integration running…</span>
              </div>
            </div>
          </TUIPanel>
        </div>
      </div>

      {/* Action bar — opencode-feeling prompt */}
      <div style={{
        marginTop: 8, padding: '10px 14px',
        border: `1px solid ${TUI.lineStrong}`, borderRadius: 2,
        background: TUI.bgDeep,
        display: 'flex', alignItems: 'center', gap: 18, fontSize: 12.5,
      }}>
        <T c={TUI.amber}>?</T>
        <T italic c={TUI.inkSoft} style={{ fontSize: 14 }}>apply this hunk?</T>
        <span style={{ flex: 1 }}/>
        {[
          { k: 'a', l: 'apply',     c: TUI.green },
          { k: 'A', l: 'apply all', c: TUI.green, dim: true },
          { k: 'r', l: 'reject',    c: TUI.red },
          { k: 'e', l: 'edit',      c: TUI.amber },
          { k: 's', l: 'skip',      c: TUI.inkMuted },
          { k: 'q', l: 'quit',      c: TUI.inkMuted },
        ].map(b => (
          <span key={b.k} style={{
            display: 'inline-flex', gap: 6, alignItems: 'center',
            padding: '3px 9px', borderRadius: 2,
            border: `1px solid ${b.c}33`,
            background: b.dim ? 'transparent' : `${b.c}10`,
            opacity: b.dim ? 0.6 : 1,
          }}>
            <T c={b.c} bold>{b.k}</T>
            <T c={TUI.inkSoft}>{b.l}</T>
          </span>
        ))}
      </div>

      <TUIFooter hints={[
        { k: 'a/A', l: 'apply / apply-all' },
        { k: 'r', l: 'reject hunk' },
        { k: 'e', l: 'open in $EDITOR' },
        { k: 'n/p', l: 'next/prev hunk' },
        { k: 'q', l: 'cancel patch' },
      ]}/>
    </TUIScreen>
  );
}

Object.assign(window, { TUI_Welcome, TUI_Chat, TUI_Diff });
