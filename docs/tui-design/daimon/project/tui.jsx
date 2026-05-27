// Daimon TUI — terminal interface for the code-focused agent
// Pure ASCII/Unicode, monospace, opencode-feeling, adapted to Daimon's identity.
// All borders are box-drawing characters. Stage directions in Fraunces italic.

const TUI = {
  bg:         '#0e0f13',
  bgElev:     '#15171d',
  bgDeep:     '#0a0b0f',
  bgPanel:    '#11131a',
  ink:        '#eae5d8',
  inkSoft:    '#c2bca9',
  inkMuted:   '#7a7465',
  inkFaint:   '#4a4438',
  inkGhost:   '#2c2a25',
  line:       '#22242c',
  lineSoft:   '#1a1c22',
  lineStrong: '#2e3038',
  accent:     '#5dbfa7',
  accentDim:  'rgba(93,191,167,0.14)',
  accentBg:   'rgba(93,191,167,0.07)',
  amber:      '#e3b67a',
  amberBg:    'rgba(227,182,122,0.10)',
  red:        '#e38775',
  redBg:      'rgba(227,135,117,0.10)',
  green:      '#7aba8a',
  user:       '#c2bca9',
  pink:       '#d67b9e',
  mono:       '"JetBrains Mono", "SF Mono", ui-monospace, monospace',
  serif:      '"Fraunces", Georgia, serif',
};

// ─────────────────────────────────────────────────────────────────
// Animation primitives
// ─────────────────────────────────────────────────────────────────
const SPINNER_FRAMES = ['⠋','⠙','⠹','⠸','⠼','⠴','⠦','⠧','⠇','⠏'];
const BAR_FRAMES = ['▏','▎','▍','▌','▋','▊','▉','█','▉','▊','▋','▌','▍','▎'];

function useTick(period, frames) {
  const [i, setI] = React.useState(0);
  React.useEffect(() => {
    const id = setInterval(() => setI(x => (x + 1) % frames.length), period);
    return () => clearInterval(id);
  }, [period, frames.length]);
  return frames[i];
}

function Spinner({ c = TUI.amber }) {
  const f = useTick(80, SPINNER_FRAMES);
  return <span style={{ color: c }}>{f}</span>;
}

function PulseDot({ c = TUI.accent, size = 6 }) {
  return <span style={{
    display: 'inline-block', width: size, height: size, borderRadius: 99,
    background: c, boxShadow: `0 0 6px ${c}`,
    animation: 'tuiBreathe 1.8s ease-in-out infinite',
    verticalAlign: 'middle', marginRight: 2, marginLeft: 2,
  }}/>;
}

function Caret({ c = TUI.accent, w = 7, h = 14 }) {
  return <span style={{
    display: 'inline-block', width: w, height: h, background: c,
    verticalAlign: 'text-bottom', marginLeft: 1,
    animation: 'tuiCaret 1.06s steps(2) infinite',
  }}/>;
}

function TokenTicker({ target, c = TUI.inkSoft, start = 0 }) {
  const [v, setV] = React.useState(start);
  React.useEffect(() => {
    let cur = start;
    const id = setInterval(() => {
      const next = cur + Math.ceil(Math.max(1, (target - cur) / 18));
      cur = Math.min(target, next);
      setV(cur);
      if (cur >= target) clearInterval(id);
    }, 70);
    return () => clearInterval(id);
  }, [target, start]);
  return <span style={{ color: c }}>{v.toLocaleString()}</span>;
}

// A progress bar built from block chars (no animation)
function BlockBar({ pct, width = 14, c = TUI.accent, dimC = TUI.inkGhost }) {
  const filled = Math.round((pct / 100) * width);
  return (
    <>
      <span style={{ color: c }}>{'█'.repeat(filled)}</span>
      <span style={{ color: dimC }}>{'░'.repeat(width - filled)}</span>
    </>
  );
}

// ─────────────────────────────────────────────────────────────────
// TUI primitives
// ─────────────────────────────────────────────────────────────────
// A row of text — monospace, no wrap, pre whitespace
function L({ children, c = TUI.ink, style }) {
  return <div style={{
    color: c, whiteSpace: 'pre', overflow: 'hidden',
    fontFamily: TUI.mono, ...style,
  }}>{children}</div>;
}

// Inline colored span
function T({ c = TUI.ink, dim, italic, bold, bg, children, style }) {
  return <span style={{
    color: c,
    opacity: dim ? 0.55 : 1,
    fontFamily: italic ? TUI.serif : 'inherit',
    fontStyle: italic ? 'italic' : 'normal',
    fontWeight: bold ? 600 : 400,
    background: bg, padding: bg ? '0 4px' : 0, borderRadius: bg ? 2 : 0,
    ...style,
  }}>{children}</span>;
}

// Wraps a screen — fixed size, dark bg, monospace, padding
function TUIScreen({ width, height, children, vignette = true }) {
  return (
    <div style={{
      width, height, background: TUI.bg, color: TUI.ink,
      fontFamily: TUI.mono, fontSize: 13, lineHeight: '20px',
      letterSpacing: 0.1, padding: '12px 16px',
      boxSizing: 'border-box', position: 'relative',
      display: 'flex', flexDirection: 'column', overflow: 'hidden',
    }}>
      {vignette && <div style={{
        position: 'absolute', inset: 0, pointerEvents: 'none',
        background: 'radial-gradient(ellipse at center, transparent 55%, rgba(0,0,0,0.35) 100%)',
      }}/>}
      <style>{`
        @keyframes tuiCaret { 0%,50% { opacity: 1; } 51%,100% { opacity: 0; } }
        @keyframes tuiBreathe { 0%,100% { opacity: 1; transform: scale(1); } 50% { opacity: 0.45; transform: scale(0.7); } }
        @keyframes tuiPulse { 0%,100% { opacity: 0.4; } 50% { opacity: 1; } }
        @keyframes tuiShimmer { 0% { background-position: -200px 0; } 100% { background-position: 200px 0; } }
        @keyframes tuiFadeIn { from { opacity: 0; transform: translateY(2px); } to { opacity: 1; transform: translateY(0); } }
        @keyframes tuiScan { 0% { transform: translateY(-100%); } 100% { transform: translateY(2000%); } }
      `}</style>
      {children}
    </div>
  );
}

// Top status bar — one line with project/branch/model/status
function TUITopBar({ project = 'helix', branch = 'main', model = 'sonnet-4.5', mode = 'plan', status = 'ready', cost = '$0.042' }) {
  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 18,
      padding: '0 4px 6px', borderBottom: `1px solid ${TUI.line}`,
      marginBottom: 8, fontSize: 12.5,
    }}>
      <span style={{ color: TUI.accent }}>⫶</span>
      <span style={{ color: TUI.ink, fontWeight: 500 }}>daimon</span>
      <span style={{ color: TUI.inkFaint }}>│</span>
      <span style={{ color: TUI.inkMuted }}>~/projects/<span style={{ color: TUI.inkSoft }}>{project}</span></span>
      <span style={{ color: TUI.inkFaint }}>·</span>
      <span style={{ color: TUI.pink }}>{branch}</span>
      <span style={{ color: TUI.inkFaint }}>│</span>
      <span style={{ color: TUI.inkMuted }}>model </span>
      <span style={{ color: TUI.ink }}>{model}</span>
      <span style={{ color: TUI.inkFaint }}>·</span>
      <span style={{ color: TUI.inkMuted }}>mode </span>
      <span style={{ color: TUI.amber }}>{mode}</span>
      <span style={{ flex: 1 }}/>
      <span style={{ color: TUI.inkMuted }}>{cost}</span>
      <span style={{ color: TUI.inkFaint }}>·</span>
      <span style={{
        color: TUI.accent, display: 'inline-flex', alignItems: 'center', gap: 6,
      }}>
        <PulseDot c={TUI.accent} size={6}/>
        {status}
      </span>
    </div>
  );
}

// Bottom status / hint bar
function TUIFooter({ hints }) {
  return (
    <div style={{
      display: 'flex', gap: 16, padding: '6px 4px 0',
      borderTop: `1px solid ${TUI.line}`,
      marginTop: 8, fontSize: 11.5, color: TUI.inkMuted,
    }}>
      {hints.map((h, i) => (
        <span key={i}>
          <span style={{ color: TUI.accent }}>{h.k}</span>{' '}
          <span style={{ color: TUI.inkMuted }}>{h.l}</span>
        </span>
      ))}
      <span style={{ flex: 1 }}/>
      <span style={{ color: TUI.inkFaint, fontFamily: TUI.serif, fontStyle: 'italic' }}>
        daimon listens.
      </span>
    </div>
  );
}

// Panel with title — box-drawing border
function TUIPanel({ title, badge, children, style, flex, width, dim }) {
  return (
    <div style={{
      border: `1px solid ${TUI.line}`,
      background: dim ? TUI.bgDeep : TUI.bgPanel,
      borderRadius: 2,
      display: 'flex', flexDirection: 'column',
      minHeight: 0, minWidth: 0, flex, width,
      ...style,
    }}>
      {title && (
        <div style={{
          padding: '4px 10px', borderBottom: `1px solid ${TUI.line}`,
          display: 'flex', alignItems: 'center', gap: 8,
          fontSize: 11, color: TUI.inkMuted,
          textTransform: 'uppercase', letterSpacing: 1,
        }}>
          <span>── {title}</span>
          <span style={{ flex: 1 }}/>
          {badge}
        </div>
      )}
      <div style={{ flex: 1, minHeight: 0, overflow: 'hidden' }}>{children}</div>
    </div>
  );
}

// Input line at bottom of chat
function TUIInput({ value = '', placeholder = 'speak…', mode = 'plan' }) {
  return (
    <div style={{
      border: `1px solid ${TUI.lineStrong}`,
      background: TUI.bgDeep, borderRadius: 2,
      padding: '8px 10px', marginTop: 8,
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, minHeight: 22 }}>
        <span style={{ color: TUI.accent, fontWeight: 600 }}>›</span>
        {value
          ? <span style={{ color: TUI.ink }}>{value}<Caret/></span>
          : <span style={{ color: TUI.inkMuted, fontFamily: TUI.serif, fontStyle: 'italic', fontSize: 14 }}>
              {placeholder}<Caret/>
            </span>
        }
      </div>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 14, marginTop: 6,
        fontSize: 11, color: TUI.inkMuted,
      }}>
        <span><T c={TUI.accent}>⇥</T> /commands</span>
        <span><T c={TUI.accent}>@</T> mention file</span>
        <span><T c={TUI.accent}>#</T> add to memory</span>
        <span><T c={TUI.accent}>⌃R</T> retry</span>
        <span style={{ flex: 1 }}/>
        <span style={{
          padding: '1px 7px', borderRadius: 2, border: `1px solid ${TUI.amber}55`,
          color: TUI.amber, background: TUI.amberBg, fontSize: 10.5,
        }}>{mode.toUpperCase()} MODE</span>
        <span><T c={TUI.accent}>⇧⇥</T> switch</span>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Chat blocks — used in the main thread
// ─────────────────────────────────────────────────────────────────
function MsgUser({ time, name = 'arnau', children }) {
  return (
    <div style={{ marginBottom: 14 }}>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 8,
        fontSize: 11.5, color: TUI.inkMuted, marginBottom: 3,
      }}>
        <span style={{ color: TUI.inkSoft }}>▌</span>
        <span style={{ color: TUI.ink, fontWeight: 500 }}>{name}</span>
        <span style={{ color: TUI.inkFaint }}>·</span>
        <span>{time}</span>
      </div>
      <div style={{ paddingLeft: 14, color: TUI.ink, fontSize: 13, lineHeight: '20px' }}>
        {children}
      </div>
    </div>
  );
}

function MsgDaimon({ time, children }) {
  return (
    <div style={{ marginBottom: 14 }}>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 8,
        fontSize: 11.5, marginBottom: 3,
      }}>
        <span style={{ color: TUI.accent }}>⫶</span>
        <span style={{ color: TUI.accent, fontWeight: 500 }}>daimon</span>
        <T italic c={TUI.inkMuted} style={{ fontSize: 12 }}>speaks</T>
        <span style={{ color: TUI.inkFaint }}>·</span>
        <span style={{ color: TUI.inkMuted }}>{time}</span>
      </div>
      <div style={{ paddingLeft: 14, color: TUI.ink, fontSize: 13, lineHeight: '20px' }}>
        {children}
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Tool entry — single-line with truncation hint, optional expand
// ─────────────────────────────────────────────────────────────────
function ToolLine({ status, name, input, stat, tokens, cost, duration, expanded, output, error }) {
  const statusGlyph = status === 'done' ? <T c={TUI.accent}>✓</T>
    : status === 'error' ? <T c={TUI.red}>✗</T>
    : status === 'running' ? <Spinner c={TUI.amber}/>
    : <T c={TUI.inkMuted}>○</T>;
  return (
    <div style={{ marginBottom: 3, fontSize: 12.5 }}>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10,
        whiteSpace: 'nowrap', overflow: 'hidden',
      }}>
        <span style={{ width: 12, textAlign: 'center' }}>{statusGlyph}</span>
        <span style={{ color: TUI.ink, fontWeight: 500, width: 96, flexShrink: 0 }}>{name}</span>
        <span style={{
          color: TUI.inkSoft, flex: 1,
          overflow: 'hidden', textOverflow: 'ellipsis',
        }}>{input}</span>
        {stat && <T c={TUI.accent} style={{ fontSize: 11.5 }}>{stat}</T>}
        {tokens != null && (
          <span style={{ fontSize: 11, color: TUI.inkMuted }}>
            {status === 'running'
              ? <><TokenTicker target={tokens} c={TUI.amber}/> tok</>
              : <>{tokens.toLocaleString()} tok</>}
          </span>
        )}
        {cost && <T c={TUI.inkMuted} style={{ fontSize: 11 }}>{cost}</T>}
        {duration && <T c={TUI.inkFaint} style={{ fontSize: 11, width: 50, textAlign: 'right' }}>{duration}</T>}
        {(output || error) && <T c={TUI.inkMuted} style={{ fontSize: 11 }}>{expanded ? '▾' : '▸'} view</T>}
      </div>
      {expanded && output && (
        <div style={{
          marginLeft: 22, marginTop: 4, marginBottom: 6,
          paddingLeft: 12, borderLeft: `1px solid ${TUI.line}`,
          fontSize: 11.5, color: TUI.inkSoft, lineHeight: '18px',
          whiteSpace: 'pre', overflow: 'hidden',
        }}>{output}</div>
      )}
    </div>
  );
}

// Subagent block — nested mini-thread with own telemetry
function Subagent({ name, task, status, tokens, cost, duration, children }) {
  const statusEl = status === 'done'
    ? <T c={TUI.accent}>✓ complete</T>
    : status === 'running'
    ? <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5, color: TUI.amber }}>
        <Spinner c={TUI.amber}/> working
      </span>
    : <T c={TUI.inkMuted}>queued</T>;
  return (
    <div style={{
      margin: '6px 0 8px', border: `1px solid ${TUI.line}`,
      borderLeft: `2px solid ${TUI.pink}`, borderRadius: 2,
      background: 'rgba(214,123,158,0.04)',
    }}>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10,
        padding: '4px 10px', borderBottom: `1px solid ${TUI.line}`,
        fontSize: 12,
      }}>
        <T c={TUI.pink}>↳</T>
        <T c={TUI.inkMuted}>subagent</T>
        <T c={TUI.ink} bold>{name}</T>
        <T c={TUI.inkFaint}>·</T>
        <T italic c={TUI.inkMuted}>{task}</T>
        <span style={{ flex: 1 }}/>
        {statusEl}
      </div>
      <div style={{ padding: '6px 10px' }}>
        {children}
      </div>
      <div style={{
        padding: '4px 10px', borderTop: `1px solid ${TUI.line}`,
        display: 'flex', gap: 14, fontSize: 11, color: TUI.inkMuted,
      }}>
        <span>tokens <T c={TUI.inkSoft}>{tokens.toLocaleString()}</T></span>
        <span>cost <T c={TUI.inkSoft}>{cost}</T></span>
        <span>time <T c={TUI.inkSoft}>{duration}</T></span>
        <span style={{ flex: 1 }}/>
        <T c={TUI.inkMuted}>▸ open thread</T>
      </div>
    </div>
  );
}

// Reasoning collapsed by default — one line with chevron
function Reasoning({ duration, open, children }) {
  return (
    <div style={{ margin: '4px 0 10px' }}>
      <div style={{
        display: 'inline-flex', alignItems: 'center', gap: 8,
        fontSize: 11.5, color: TUI.inkMuted,
      }}>
        <T c={TUI.inkFaint}>{open ? '▾' : '▸'}</T>
        <T italic c={TUI.inkMuted} style={{ fontSize: 12.5 }}>pondered for {duration}</T>
      </div>
      {open && children && (
        <div style={{
          marginLeft: 14, marginTop: 4, paddingLeft: 10,
          borderLeft: `1px solid ${TUI.line}`,
          fontSize: 12, color: TUI.inkSoft, lineHeight: '18px',
          fontFamily: TUI.serif, fontStyle: 'italic',
        }}>{children}</div>
      )}
    </div>
  );
}

// Inline code chip
function Code({ children, c = TUI.amber }) {
  return <span style={{
    fontFamily: TUI.mono, fontSize: 12,
    background: TUI.bgDeep, color: c,
    padding: '0 5px', borderRadius: 2,
    border: `1px solid ${TUI.line}`,
  }}>{children}</span>;
}

Object.assign(window, {
  TUI, L, T, Spinner, Caret, PulseDot, TokenTicker, BlockBar,
  TUIScreen, TUITopBar, TUIFooter, TUIPanel, TUIInput,
  MsgUser, MsgDaimon, ToolLine, Subagent, Reasoning, Code,
});
