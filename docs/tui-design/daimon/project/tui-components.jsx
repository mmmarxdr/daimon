// Daimon TUI — Component System
// Anatomy + variants for each component. Shows what stays fixed vs what swaps.

// ─────────────────────────────────────────────────────────────────
// Spec helpers — labels, callouts, swatch frames
// ─────────────────────────────────────────────────────────────────
function SpecFrame({ label, w, h, children, bg = TUI.bg, note }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <div style={{
        display: 'flex', alignItems: 'baseline', gap: 10,
        fontFamily: TUI.mono, fontSize: 11,
      }}>
        <span style={{
          color: TUI.accent, padding: '1px 7px',
          background: TUI.accentBg, border: `1px solid ${TUI.accent}44`,
          borderRadius: 2,
        }}>{label}</span>
        {note && <span style={{
          color: TUI.inkMuted, fontFamily: TUI.serif, fontStyle: 'italic',
          fontSize: 12,
        }}>{note}</span>}
      </div>
      <div style={{
        width: w, height: h, background: bg,
        border: `1px dashed ${TUI.line}`,
        borderRadius: 2, padding: '10px 14px',
        boxSizing: 'border-box', overflow: 'hidden',
        fontFamily: TUI.mono, fontSize: 13, lineHeight: '20px',
        color: TUI.ink, position: 'relative',
      }}>{children}</div>
    </div>
  );
}

function SpecGroup({ title, kicker, children }) {
  return (
    <div style={{ marginBottom: 36 }}>
      <div style={{
        display: 'flex', alignItems: 'baseline', gap: 14, marginBottom: 14,
        borderBottom: `1px solid ${TUI.line}`, paddingBottom: 8,
      }}>
        <span style={{
          fontFamily: TUI.mono, fontSize: 10.5,
          color: TUI.accent, letterSpacing: 1.5, textTransform: 'uppercase',
        }}>{kicker}</span>
        <span style={{
          fontFamily: TUI.serif, fontStyle: 'italic',
          fontSize: 20, color: TUI.ink, letterSpacing: -0.3,
        }}>{title}</span>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 28 }}>{children}</div>
    </div>
  );
}

function Row({ children, gap = 18 }) {
  return <div style={{ display: 'flex', gap, flexWrap: 'wrap', alignItems: 'flex-start' }}>{children}</div>;
}

function Para({ children }) {
  return <div style={{
    fontFamily: 'Inter, system-ui, sans-serif', fontSize: 13.5,
    color: TUI.inkSoft, lineHeight: 1.65, maxWidth: 720,
  }}>{children}</div>;
}

// ─────────────────────────────────────────────────────────────────
// 1 · SHELL PERSISTENTE
// ─────────────────────────────────────────────────────────────────
function ComponentsShell() {
  return (
    <SpecGroup kicker="01 · persistent shell" title="Lo que nunca se mueve.">
      <Para>
        El shell es la promesa visual: el usuario sabe siempre <em>quién, dónde, con qué, cuánto</em>.
        Aparece en las 7 pantallas en exactamente la misma posición y con los mismos slots.
        El contenido cambia, la estructura no.
      </Para>

      {/* Top bar */}
      <div>
        <SpecFrame label="TopBar" w={1180} h={50} note="6 slots de izquierda a derecha. Status dot a la derecha, siempre vivo.">
          <TUITopBar/>
        </SpecFrame>
        <div style={{ marginTop: 8, display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 14, fontFamily: TUI.mono, fontSize: 11.5, color: TUI.inkMuted }}>
          {[
            ['identity', '⫶ daimon — nunca falta'],
            ['cwd · branch', 'workspace actual; branch en pink'],
            ['model · mode', 'sonnet · plan/build/review'],
            ['cost', 'acumulado de la sesión'],
            ['status', 'pulse dot + verb (ready/working/held/awake)'],
            ['mode pill', 'amber chip — clickable para cambiar'],
          ].map(([k, v]) => (
            <div key={k} style={{ display: 'flex', gap: 8 }}>
              <span style={{ color: TUI.accent, width: 90 }}>{k}</span>
              <span style={{ color: TUI.inkSoft }}>{v}</span>
            </div>
          ))}
        </div>
      </div>

      {/* Input */}
      <div>
        <SpecFrame label="InputBar" w={1180} h={92} note="Pegado abajo del thread. Mode badge a la derecha. Slash/at/hash siempre visibles.">
          <TUIInput placeholder="speak, daimon listens…" mode="build"/>
        </SpecFrame>
      </div>

      {/* Footer */}
      <div>
        <SpecFrame label="FooterHints" w={1180} h={44} note="Keymap CONTEXTUAL — varía por pantalla. El italic 'daimon listens.' se mantiene.">
          <TUIFooter hints={[
            { k: '⇥', l: '/commands' },
            { k: '⌃C', l: 'interrupt' },
            { k: '⌃R', l: 'retry turn' },
            { k: '⌃E', l: 'edit last' },
          ]}/>
        </SpecFrame>
      </div>
    </SpecGroup>
  );
}

// ─────────────────────────────────────────────────────────────────
// 2 · THREAD — varía solo en contenido
// ─────────────────────────────────────────────────────────────────
function ComponentsThread() {
  return (
    <SpecGroup kicker="02 · thread" title="Estructura del hilo principal.">
      <Para>
        El hilo es una secuencia plana de bloques. Cada bloque es uno de los componentes de abajo —
        no hay anidamientos extraños salvo <code style={{ color: TUI.pink }}>Subagent</code>, que es un sub-thread con su propia caja.
      </Para>

      <Row>
        <SpecFrame label="MsgUser" w={580} h={90} note="▌ prefix · name · time">
          <MsgUser time="14:32" name="arnau">
            analyze yesterday's payment logs, find the anomaly.
          </MsgUser>
        </SpecFrame>

        <SpecFrame label="MsgDaimon" w={580} h={90} note="⫶ prefix · 'speaks' italic · time">
          <MsgDaimon time="14:32">
            <div>found the window. pulling sources now —</div>
          </MsgDaimon>
        </SpecFrame>
      </Row>

      <SpecFrame label="Reasoning" w={1180} h={120} note="Collapsed por default · italic Fraunces al expandir">
        <Reasoning duration="6s" open>
          two recent deploys touched payments. likely culprit: <Code>2c1d9e7</Code> — the async
          refactor kept the old 2s timeout.
        </Reasoning>
      </SpecFrame>

      {/* Tool line — all 4 states */}
      <div>
        <SpecFrame label="ToolLine · 4 estados" w={1180} h={170}
          note="Una línea, truncable. Glyph · name · input · stat · tokens · cost · duration · expand">
          <ToolLine status="done" name="read_file" input="./services/payments/webhook.ts" stat="48 lines" tokens={920} cost="$0.003" duration="38ms"/>
          <ToolLine status="running" name="grep" input="'webhook timeout|retry exhausted'" tokens={612}/>
          <ToolLine status="error" name="read_file" input="/etc/daimon/secrets.env" stat="denied" tokens={48} duration="12ms"/>
          <ToolLine status="queued" name="shell" input="bun test integration/payments" duration="—"/>
        </SpecFrame>
        <div style={{ marginTop: 10, display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12, fontFamily: TUI.mono, fontSize: 11.5 }}>
          {[
            ['✓ done',    TUI.accent, 'tokens fijos · stat final · duration'],
            ['⠋ running', TUI.amber,  'spinner braille · token ticker animado · sin duration'],
            ['✗ error',   TUI.red,    'stat = error code · expand → output rojo'],
            ['○ queued',  TUI.inkMuted, 'esperando turno · dim'],
          ].map(([k, c, v]) => (
            <div key={k}>
              <div style={{ color: c, fontWeight: 600 }}>{k}</div>
              <div style={{ color: TUI.inkMuted, marginTop: 2 }}>{v}</div>
            </div>
          ))}
        </div>
      </div>

      <SpecFrame label="Subagent" w={1180} h={230}
        note="Pink border-left · header con tarea · footer con telemetría propia · cierra como un acordeón">
        <Subagent name="test-runner" task="verify proposed patch against suite"
          status="running" tokens={2148} cost="$0.008" duration="4.8s">
          <ToolLine status="done" name="shell" input="bun test webhook.test.ts" stat="exit 0" tokens={420} duration="2.1s"/>
          <ToolLine status="done" name="shell" input="bun test --coverage" stat="12/12 pass" tokens={812} duration="2.4s"/>
          <ToolLine status="running" name="shell" input="bun test integration/payments" tokens={920}/>
        </Subagent>
      </SpecFrame>
    </SpecGroup>
  );
}

// ─────────────────────────────────────────────────────────────────
// 3 · RIGHT RAIL — slots modulares
// ─────────────────────────────────────────────────────────────────
function PanelTodolist() {
  return (
    <TUIPanel title="todolist" badge={<T c={TUI.inkMuted} style={{ fontSize: 10.5 }}>2/5 · auto</T>}>
      <div style={{ padding: '4px 10px', fontSize: 12 }}>
        {[
          { st: 'done',     l: 'pull yesterday\'s logs' },
          { st: 'done',     l: 'isolate error cluster' },
          { st: 'running',  l: 'identify root commit' },
          { st: 'pending',  l: 'draft minimal patch' },
          { st: 'pending',  l: 'verify with test run' },
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
      </div>
    </TUIPanel>
  );
}

function PanelContext() {
  return (
    <TUIPanel title="context window">
      <div style={{ padding: '6px 10px', fontSize: 12 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
          <T c={TUI.inkSoft}>43,182 / 200,000</T>
          <T c={TUI.accent}>21.6%</T>
        </div>
        <div style={{ marginBottom: 8 }}><BlockBar pct={21.6} width={28}/></div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '2px 12px', color: TUI.inkMuted, fontSize: 11.5 }}>
          <span>system</span>      <T c={TUI.inkSoft} style={{ textAlign: 'right' }}>2.1k</T>
          <span>memory</span>      <T c={TUI.inkSoft} style={{ textAlign: 'right' }}>4.8k</T>
          <span>conversation</span><T c={TUI.inkSoft} style={{ textAlign: 'right' }}>21.0k</T>
          <span>tool output</span> <T c={TUI.inkSoft} style={{ textAlign: 'right' }}>14.2k</T>
          <span>workspace</span>   <T c={TUI.inkSoft} style={{ textAlign: 'right' }}>1.1k</T>
        </div>
      </div>
    </TUIPanel>
  );
}

function PanelTelemetry({ compact }) {
  return (
    <TUIPanel title="telemetry" badge={<T c={TUI.green} style={{ fontSize: 10.5 }}>live</T>}>
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
        {!compact && (<>
          <div style={{ marginTop: 6, fontSize: 10.5, color: TUI.inkMuted, textTransform: 'uppercase', letterSpacing: 1 }}>── by tool</div>
          {[['read_file', 3, '2.8k'], ['grep', 2, '0.9k'], ['shell', 3, '2.1k']].map(([n, c, t]) => (
            <div key={n} style={{ display: 'flex', padding: '1.5px 0', fontSize: 11.5 }}>
              <T c={TUI.inkSoft} style={{ flex: 1 }}>{n}</T>
              <T c={TUI.inkMuted} style={{ width: 36, textAlign: 'right' }}>×{c}</T>
              <T c={TUI.ink} style={{ width: 56, textAlign: 'right' }}>{t}</T>
            </div>
          ))}
        </>)}
      </div>
    </TUIPanel>
  );
}

function PanelHunks() {
  return (
    <TUIPanel title="hunks">
      <div style={{ padding: '6px 10px', fontSize: 12 }}>
        {[
          { i: 1, file: 'webhook.ts',     lines: '+7 −4', active: true },
          { i: 2, file: 'config.ts',      lines: '+2 −0' },
        ].map(h => (
          <div key={h.i} style={{
            display: 'flex', gap: 10, padding: '3px 6px',
            color: h.active ? TUI.accent : TUI.inkSoft,
            background: h.active ? TUI.accentBg : 'transparent',
            borderRadius: 2,
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
  );
}

function PanelPolicy() {
  return (
    <TUIPanel title="active policy">
      <div style={{ padding: '8px 12px', fontSize: 12 }}>
        <div style={{ display: 'grid', gridTemplateColumns: '92px 1fr', gap: '3px 10px' }}>
          <T c={TUI.inkMuted}>fs.read</T>     <T c={TUI.green}>workspace · allowlist</T>
          <T c={TUI.inkMuted}>fs.write</T>    <T c={TUI.amber}>workspace · ask</T>
          <T c={TUI.inkMuted}>shell</T>       <T c={TUI.amber}>allowlist · ask</T>
          <T c={TUI.inkMuted}>net</T>         <T c={TUI.red}>disabled</T>
          <T c={TUI.inkMuted}>spawn</T>       <T c={TUI.green}>allowed · capped 3</T>
        </div>
      </div>
    </TUIPanel>
  );
}

function PanelEnvironment() {
  return (
    <TUIPanel title="environment">
      <div style={{ padding: '6px 10px', fontSize: 12, color: TUI.inkSoft }}>
        {[
          ['runtime', 'daimon 0.4.2'],
          ['model',   'sonnet-4.5'],
          ['cwd',     '~/projects/helix'],
          ['branch',  'main (clean)'],
          ['tools',   '12 builtin · 3 mcp'],
          ['memory',  '1,284 facts'],
        ].map(([k, v]) => (
          <div key={k} style={{ display: 'flex', padding: '2px 0' }}>
            <T c={TUI.inkMuted} style={{ width: 70 }}>{k}</T>
            <T c={TUI.ink}>{v}</T>
          </div>
        ))}
      </div>
    </TUIPanel>
  );
}

function ComponentsRail() {
  // Panels arranged in a grid, all same width, easy to scan.
  const cards = [
    { id: 'environment', title: 'environment', node: <PanelEnvironment/>, screens: 'welcome' },
    { id: 'todolist',    title: 'todolist',    node: <PanelTodolist/>,    screens: 'chat' },
    { id: 'context',     title: 'context meter', node: <PanelContext/>, screens: 'chat · error (compact)' },
    { id: 'telemetry',   title: 'telemetry',   node: <PanelTelemetry/>,   screens: 'chat · diff · error' },
    { id: 'hunks',       title: 'hunks',       node: <PanelHunks/>,       screens: 'diff' },
    { id: 'policy',      title: 'active policy', node: <PanelPolicy/>,  screens: 'error' },
  ];
  return (
    <SpecGroup kicker="03 · right rail · modular" title="Slots, no panel fijo.">
      <Para>
        El sidebar derecho es una <strong>columna de slots</strong>. Cada pantalla decide qué paneles montar y en qué orden.
        Hay tres paneles que merecen estar en casi todas (telemetry, context, todolist) — el resto es contextual.
        Reglas: ancho fijo 320px, gap 8px entre paneles, scroll si excede, mismo header (── title · badge opcional).
      </Para>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 18 }}>
        {cards.map(c => (
          <div key={c.id} style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <div style={{
              display: 'flex', alignItems: 'baseline', gap: 10,
              fontFamily: TUI.mono, fontSize: 11,
            }}>
              <span style={{
                color: TUI.accent, padding: '1px 7px',
                background: TUI.accentBg, border: `1px solid ${TUI.accent}44`, borderRadius: 2,
              }}>{c.title}</span>
              <span style={{ color: TUI.inkMuted, fontFamily: TUI.serif, fontStyle: 'italic', fontSize: 12 }}>
                aparece en: {c.screens}
              </span>
            </div>
            <div style={{
              width: 320, background: TUI.bg, border: `1px dashed ${TUI.line}`,
              borderRadius: 2, padding: 10, fontFamily: TUI.mono,
            }}>
              {c.node}
            </div>
          </div>
        ))}
      </div>
    </SpecGroup>
  );
}

// ─────────────────────────────────────────────────────────────────
// 4 · OVERLAYS
// ─────────────────────────────────────────────────────────────────
function ComponentsOverlays() {
  return (
    <SpecGroup kicker="04 · overlays" title="Lo que se monta encima.">
      <Para>
        Overlays interrumpen el flujo: el chat detrás se dimea (blur + alpha). Mismo lenguaje de bordes —
        accent en outline para los críticos (palette · permiso), neutro para los informativos.
      </Para>

      <Row gap={24}>
        <SpecFrame label="ApprovalPrompt" w={720} h={64} note="Diff approval bar — letras como triggers">
          <div style={{
            padding: '8px 12px', border: `1px solid ${TUI.lineStrong}`, borderRadius: 2,
            background: TUI.bgDeep,
            display: 'flex', alignItems: 'center', gap: 14, fontSize: 12.5,
          }}>
            <T c={TUI.amber}>?</T>
            <T italic c={TUI.inkSoft} style={{ fontSize: 14 }}>apply this hunk?</T>
            <span style={{ flex: 1 }}/>
            {[['a','apply', TUI.green], ['r','reject', TUI.red], ['e','edit', TUI.amber], ['s','skip', TUI.inkMuted]].map(([k,l,c]) => (
              <span key={k} style={{
                display: 'inline-flex', gap: 6, alignItems: 'center',
                padding: '3px 9px', borderRadius: 2,
                border: `1px solid ${c}33`, background: `${c}10`,
              }}>
                <T c={c} bold>{k}</T><T c={TUI.inkSoft}>{l}</T>
              </span>
            ))}
          </div>
        </SpecFrame>

        <SpecFrame label="PermissionPrompt" w={420} h={64} note="Variante del prompt — rojo">
          <div style={{
            padding: '8px 12px', border: `1px solid ${TUI.red}55`, borderRadius: 2,
            background: TUI.redBg,
            display: 'flex', alignItems: 'center', gap: 12, fontSize: 12.5,
          }}>
            <T c={TUI.red} bold>⚠</T>
            <T italic c={TUI.inkSoft}>allow access?</T>
            <span style={{ flex: 1 }}/>
            <span style={{ padding: '3px 9px', border: `1px solid ${TUI.green}33`, background: `${TUI.green}10`, borderRadius: 2 }}>
              <T c={TUI.green} bold>a</T> <T c={TUI.inkSoft}>once</T>
            </span>
            <span style={{ padding: '3px 9px', border: `1px solid ${TUI.red}33`, background: `${TUI.red}10`, borderRadius: 2 }}>
              <T c={TUI.red} bold>d</T> <T c={TUI.inkSoft}>deny</T>
            </span>
          </div>
        </SpecFrame>
      </Row>

      <SpecFrame label="CommandPalette · header" w={680} h={62} note="Outline accent · siempre con / como prefix · placeholder italic">
        <div style={{
          width: '100%', height: '100%',
          background: TUI.bgPanel, border: `1px solid ${TUI.accent}`,
          borderRadius: 3, overflow: 'hidden',
          boxShadow: `0 0 30px ${TUI.accent}22`,
        }}>
          <div style={{
            padding: '12px 16px', borderBottom: `1px solid ${TUI.line}`,
            display: 'flex', alignItems: 'center', gap: 10,
            background: 'rgba(93,191,167,0.04)',
          }}>
            <T c={TUI.accent} bold style={{ fontSize: 14 }}>/</T>
            <span style={{ flex: 1, fontSize: 13, color: TUI.ink }}>
              re<Caret w={6} h={14}/>
            </span>
            <T c={TUI.inkMuted} style={{ fontSize: 11 }}>3 of 14</T>
          </div>
        </div>
      </SpecFrame>
    </SpecGroup>
  );
}

// ─────────────────────────────────────────────────────────────────
// 5 · MATRIZ — qué aparece en cada pantalla
// ─────────────────────────────────────────────────────────────────
function ComponentsMatrix() {
  const screens = ['welcome', 'chat', 'diff', 'slash', 'tools', 'sessions', 'error'];
  const components = [
    { group: 'shell', name: 'TopBar',       on: [1,1,1,1,1,1,1] },
    { group: 'shell', name: 'InputBar',     on: [1,1,0,0,0,0,1] },
    { group: 'shell', name: 'FooterHints',  on: [1,1,1,1,1,1,1] },

    { group: 'thread', name: 'MsgUser',     on: [0,1,0,0,0,0,0] },
    { group: 'thread', name: 'MsgDaimon',   on: [0,1,0,0,0,0,1] },
    { group: 'thread', name: 'Reasoning',   on: [0,1,0,0,0,0,0] },
    { group: 'thread', name: 'ToolLine',    on: [0,1,0,0,0,0,1] },
    { group: 'thread', name: 'Subagent',    on: [0,1,0,0,0,0,0] },

    { group: 'rail',   name: 'environment',   on: [1,0,0,0,0,0,0] },
    { group: 'rail',   name: 'resume list',   on: [1,0,0,0,0,1,0] },
    { group: 'rail',   name: 'todolist',      on: [0,1,0,0,0,0,0] },
    { group: 'rail',   name: 'context meter', on: [0,1,0,0,0,0,0] },
    { group: 'rail',   name: 'telemetry',     on: [0,1,1,0,0,0,1] },
    { group: 'rail',   name: 'hunks nav',     on: [0,0,1,0,0,0,0] },
    { group: 'rail',   name: 'rationale',     on: [0,0,1,0,0,0,0] },
    { group: 'rail',   name: 'impact',        on: [0,0,1,0,0,0,0] },
    { group: 'rail',   name: 'tool detail',   on: [0,0,0,0,1,0,0] },
    { group: 'rail',   name: 'model picker',  on: [0,0,0,0,0,1,0] },
    { group: 'rail',   name: 'active policy', on: [0,0,0,0,0,0,1] },
    { group: 'rail',   name: 'recent denials',on: [0,0,0,0,0,0,1] },

    { group: 'overlay', name: 'CommandPalette', on: [0,0,0,1,0,0,0] },
    { group: 'overlay', name: 'ApprovalPrompt', on: [0,0,1,0,0,0,0] },
    { group: 'overlay', name: 'PermissionPrompt', on: [0,0,0,0,0,0,1] },
  ];
  const groupColor = { shell: TUI.accent, thread: TUI.ink, rail: TUI.amber, overlay: TUI.pink };
  let lastGroup = null;
  return (
    <SpecGroup kicker="05 · matrix" title="¿Qué aparece dónde?">
      <Para>
        Lee la tabla de izquierda a derecha. Verás que el <strong>shell</strong> está siempre, el <strong>thread</strong>
        solo donde hay conversación, los <strong>rail panels</strong> son contextuales (telemetry está en 3 de 7,
        que es el más recurrente), y los <strong>overlays</strong> son únicos por pantalla.
      </Para>

      <div style={{
        border: `1px solid ${TUI.line}`, borderRadius: 2,
        background: TUI.bgPanel, overflow: 'hidden',
        fontFamily: TUI.mono, fontSize: 12.5,
      }}>
        {/* header */}
        <div style={{
          display: 'grid',
          gridTemplateColumns: '100px 200px repeat(7, 1fr)',
          padding: '8px 12px',
          fontSize: 10.5, color: TUI.inkMuted, letterSpacing: 1.5, textTransform: 'uppercase',
          borderBottom: `1px solid ${TUI.line}`, background: TUI.bgDeep,
        }}>
          <span>group</span>
          <span>component</span>
          {screens.map(s => <span key={s} style={{ textAlign: 'center' }}>{s}</span>)}
        </div>
        {components.map((c, i) => {
          const groupChanged = c.group !== lastGroup;
          lastGroup = c.group;
          return (
            <div key={i} style={{
              display: 'grid',
              gridTemplateColumns: '100px 200px repeat(7, 1fr)',
              padding: '6px 12px',
              borderTop: groupChanged ? `1px solid ${TUI.line}` : `1px solid ${TUI.lineSoft}`,
              alignItems: 'center',
            }}>
              <span>{groupChanged
                ? <T c={groupColor[c.group]} style={{ fontSize: 11 }}>{c.group}</T>
                : <T c={TUI.inkFaint}>›</T>}</span>
              <T c={TUI.ink}>{c.name}</T>
              {c.on.map((v, j) => (
                <span key={j} style={{ textAlign: 'center' }}>
                  {v
                    ? <T c={groupColor[c.group]} bold>●</T>
                    : <T c={TUI.inkGhost}>·</T>}
                </span>
              ))}
            </div>
          );
        })}
      </div>
    </SpecGroup>
  );
}

// ─────────────────────────────────────────────────────────────────
// 6 · TOKENS — quick palette + type ref
// ─────────────────────────────────────────────────────────────────
function ComponentsTokens() {
  const palette = [
    ['bg',         TUI.bg,         'canvas — el negro azulado'],
    ['bgPanel',    TUI.bgPanel,    'fondo de paneles laterales'],
    ['bgElev',     TUI.bgElev,     'elevación · seleccionado'],
    ['bgDeep',     TUI.bgDeep,     'inputs · code wells'],
    ['ink',        TUI.ink,        'texto principal'],
    ['inkSoft',    TUI.inkSoft,    'texto secundario'],
    ['inkMuted',   TUI.inkMuted,   'labels, metadata'],
    ['inkFaint',   TUI.inkFaint,   'separadores, line numbers'],
    ['accent',     TUI.accent,     'phosphor teal — Daimon, status ok'],
    ['amber',      TUI.amber,      'running, modo, warning'],
    ['red',        TUI.red,        'error, deny, breaking'],
    ['green',      TUI.green,      'success, added, healthy'],
    ['pink',       TUI.pink,       'branch, subagent, tool name'],
  ];
  return (
    <SpecGroup kicker="06 · tokens" title="Paleta + tipografía.">
      <Row gap={20}>
        <div style={{ flex: 1 }}>
          <div style={{ fontFamily: TUI.mono, fontSize: 10.5, color: TUI.inkMuted, letterSpacing: 1, textTransform: 'uppercase', marginBottom: 8 }}>── color</div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 6 }}>
            {palette.map(([n, v, d]) => (
              <div key={n} style={{
                display: 'flex', alignItems: 'center', gap: 10,
                fontFamily: TUI.mono, fontSize: 11.5,
                padding: '4px 8px', border: `1px solid ${TUI.line}`,
                borderRadius: 2, background: TUI.bgPanel,
              }}>
                <span style={{
                  width: 24, height: 24, borderRadius: 2,
                  background: v, border: `1px solid ${TUI.line}`,
                  flexShrink: 0,
                }}/>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ color: TUI.ink }}>{n}</div>
                  <div style={{ color: TUI.inkMuted, fontSize: 10.5 }}>{v}</div>
                </div>
                <div style={{ color: TUI.inkMuted, fontSize: 10.5, fontFamily: TUI.serif, fontStyle: 'italic', maxWidth: 130, textAlign: 'right' }}>
                  {d}
                </div>
              </div>
            ))}
          </div>
        </div>

        <div style={{ flex: 1 }}>
          <div style={{ fontFamily: TUI.mono, fontSize: 10.5, color: TUI.inkMuted, letterSpacing: 1, textTransform: 'uppercase', marginBottom: 8 }}>── type</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            <TypeRow family={TUI.mono} size={13} label="JetBrains Mono · 13/20 · body">
              the agent runs on your hardware.
            </TypeRow>
            <TypeRow family={TUI.mono} size={11.5} label="JetBrains Mono · 11.5 · meta">
              tokens 34.2k · cost $0.142 · 2m 14s
            </TypeRow>
            <TypeRow family={TUI.mono} size={10.5} label="JetBrains Mono · 10.5 uppercase · label">
              ── TELEMETRY
            </TypeRow>
            <TypeRow family={TUI.serif} size={14} italic label="Fraunces italic · 14 · stage direction">
              daimon awaits your nod.
            </TypeRow>
            <TypeRow family={TUI.serif} size={12} italic label="Fraunces italic · 12 · inline aside">
              pondered for 6s
            </TypeRow>
          </div>

          <div style={{ marginTop: 16, fontFamily: TUI.mono, fontSize: 10.5, color: TUI.inkMuted, letterSpacing: 1, textTransform: 'uppercase', marginBottom: 8 }}>── glyphs</div>
          <div style={{
            display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 6,
            fontFamily: TUI.mono, fontSize: 11.5,
          }}>
            {[
              ['⫶', 'daimon'],
              ['›', 'prompt'],
              ['▌', 'user'],
              ['↳', 'subagent'],
              ['✓', 'done'],
              ['✗', 'failed'],
              ['◐ ⠋', 'running'],
              ['○', 'queued'],
              ['◆', 'awaiting'],
              ['⚠', 'denied'],
              ['●', 'live dot'],
              ['── §', 'section'],
            ].map(([g, l]) => (
              <div key={l} style={{
                padding: '6px 8px', border: `1px solid ${TUI.line}`,
                background: TUI.bgPanel, borderRadius: 2,
                display: 'flex', alignItems: 'center', gap: 8,
              }}>
                <span style={{ color: TUI.accent, fontSize: 14, width: 36 }}>{g}</span>
                <span style={{ color: TUI.inkSoft }}>{l}</span>
              </div>
            ))}
          </div>
        </div>
      </Row>
    </SpecGroup>
  );
}

function TypeRow({ family, size, italic, label, children }) {
  return (
    <div>
      <div style={{ fontFamily: TUI.mono, fontSize: 10, color: TUI.inkMuted, marginBottom: 2 }}>{label}</div>
      <div style={{
        fontFamily: family, fontSize: size,
        fontStyle: italic ? 'italic' : 'normal',
        color: TUI.ink,
        padding: '6px 10px', border: `1px solid ${TUI.line}`,
        background: TUI.bgPanel, borderRadius: 2,
      }}>{children}</div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Main page
// ─────────────────────────────────────────────────────────────────
function ComponentSystem() {
  return (
    <div style={{
      background: TUI.bgDeep, minHeight: '100vh',
      color: TUI.ink, padding: '40px 48px 80px',
      fontFamily: 'Inter, system-ui, sans-serif',
    }}>
      <style>{`
        @keyframes tuiCaret { 0%,50% { opacity: 1; } 51%,100% { opacity: 0; } }
        @keyframes tuiBreathe { 0%,100% { opacity: 1; transform: scale(1); } 50% { opacity: 0.45; transform: scale(0.7); } }
      `}</style>

      {/* Header */}
      <div style={{ maxWidth: 1280, margin: '0 auto 48px' }}>
        <div style={{
          display: 'flex', alignItems: 'baseline', gap: 14, marginBottom: 10,
        }}>
          <span style={{ color: TUI.accent, fontFamily: TUI.mono, fontSize: 22 }}>⫶</span>
          <span style={{
            fontFamily: TUI.serif, fontSize: 42, color: TUI.ink,
            letterSpacing: -1, fontWeight: 400,
          }}>
            Daimon TUI — <em style={{ color: TUI.accent, fontWeight: 300 }}>component system</em>
          </span>
        </div>
        <div style={{
          fontFamily: TUI.serif, fontStyle: 'italic', fontSize: 16,
          color: TUI.inkMuted, maxWidth: 760,
        }}>
          Para iterar componente a componente. Lo que se queda quieto, lo que cambia, y dónde.
          Cada bloque está montado con los mismos primitives que la TUI real — no son ilustraciones.
        </div>
      </div>

      <div style={{ maxWidth: 1280, margin: '0 auto' }}>
        <ComponentsShell/>
        <ComponentsThread/>
        <ComponentsRail/>
        <ComponentsOverlays/>
        <ComponentsMatrix/>
        <ComponentsTokens/>
      </div>
    </div>
  );
}

Object.assign(window, { ComponentSystem });
