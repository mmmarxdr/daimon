// Daimon v2 — "Liminal" identity
// Own identity: between terminal and editorial. Daimon = inner voice.
// Signature: the conversation is a single thread you walk down, with
// speaker headers that read like stage directions. Warm paper + teal.

// ─────────────────────────────────────────────────────────────────
// Platform detection — shortcuts adapt (Ctrl on Win/Linux, ⌘ on mac)
// ─────────────────────────────────────────────────────────────────
const IS_MAC = typeof navigator !== 'undefined' &&
  /Mac|iPhone|iPad|iPod/i.test(navigator.platform || navigator.userAgent || '');
const MOD = IS_MAC ? '⌘' : 'Ctrl';
const ENTER = '↵';

// ─────────────────────────────────────────────────────────────────
// Tokens
// ─────────────────────────────────────────────────────────────────
const LIMINAL_TOKENS = {
  light: {
    bg:         '#f8f6f1',
    bgElev:     '#ffffff',
    bgDeep:     '#efece5',
    bgSidebar:  '#f3f0e9',
    bgCode:     '#1a1813',
    ink:        '#1a1813',
    inkSoft:    '#3d3a32',
    inkMuted:   '#8a8275',
    inkFaint:   '#b5ad9d',
    line:       'rgba(26,24,19,0.08)',
    lineStrong: 'rgba(26,24,19,0.15)',
    accent:     '#2d8573',
    accentSoft: 'rgba(45,133,115,0.08)',
    accentStrong:'#1f6357',
    green:      '#4a8a5a',
    amber:      '#b87a2e',
    red:        '#b0432e',
    user:       '#3d3a32',
  },
  dark: {
    bg:         '#141210',
    bgElev:     '#1c1a17',
    bgDeep:     '#0e0d0b',
    bgSidebar:  '#100f0d',
    bgCode:     '#0a0908',
    ink:        '#eae5d8',
    inkSoft:    '#c2bca9',
    inkMuted:   '#7a7465',
    inkFaint:   '#443e33',
    line:       'rgba(234,229,216,0.08)',
    lineStrong: 'rgba(234,229,216,0.16)',
    accent:     '#5dbfa7',
    accentSoft: 'rgba(93,191,167,0.10)',
    accentStrong:'#8ed9c3',
    green:      '#7aba8a',
    amber:      '#e3b67a',
    red:        '#e38775',
    user:       '#c2bca9',
  },
};

// ─────────────────────────────────────────────────────────────────
// Syntax highlighting
// ─────────────────────────────────────────────────────────────────
function highlightTS(code) {
  const tokens = [];
  let i = 0;
  const KW = /^(const|let|var|function|async|await|return|if|else|for|while|do|switch|case|break|continue|class|extends|new|this|super|import|export|from|default|try|catch|throw|type|interface|enum|void|boolean|number|string|any|true|false|null|undefined)$/;
  while (i < code.length) {
    if (code[i] === '/' && code[i+1] === '/') {
      const e = code.indexOf('\n', i); const end = e === -1 ? code.length : e;
      tokens.push({ t: 'comment', v: code.slice(i, end) }); i = end; continue;
    }
    if (code[i] === '"' || code[i] === "'" || code[i] === '`') {
      const q = code[i]; let j = i + 1;
      while (j < code.length && code[j] !== q) { if (code[j] === '\\') j++; j++; }
      tokens.push({ t: 'string', v: code.slice(i, j + 1) }); i = j + 1; continue;
    }
    if (/[0-9]/.test(code[i])) {
      let j = i; while (j < code.length && /[0-9_]/.test(code[j])) j++;
      tokens.push({ t: 'number', v: code.slice(i, j) }); i = j; continue;
    }
    if (/[A-Za-z_$]/.test(code[i])) {
      let j = i; while (j < code.length && /[A-Za-z0-9_$]/.test(code[j])) j++;
      const v = code.slice(i, j);
      if (KW.test(v)) tokens.push({ t: 'keyword', v });
      else if (/^[A-Z]/.test(v)) tokens.push({ t: 'type', v });
      else if (code[j] === '(') tokens.push({ t: 'fn', v });
      else tokens.push({ t: 'ident', v });
      i = j; continue;
    }
    tokens.push({ t: 'punct', v: code[i] }); i++;
  }
  return tokens;
}

function LiminalCode({ code, lang = 'typescript', theme, showDiff = false, compact = false, highlightLine = null }) {
  const palette = {
    keyword: '#d67b9e', type: '#d4a85a', fn: '#5dbfa7',
    string: '#b8c97a', number: '#e3a96b', comment: '#6b6457',
    ident: '#e5dfd0', punct: '#a8a092',
  };
  const tokens = highlightTS(code);
  const lines = [];
  let cur = [];
  tokens.forEach((tk, ti) => {
    const parts = tk.v.split('\n');
    parts.forEach((p, pi) => {
      if (p.length) cur.push(<span key={`${ti}-${pi}`} style={{ color: palette[tk.t] }}>{p}</span>);
      if (pi < parts.length - 1) { lines.push(cur); cur = []; }
    });
  });
  if (cur.length) lines.push(cur);

  const rawLines = code.split('\n');

  return (
    <div style={{
      background: '#1a1813',
      border: `1px solid rgba(234,229,216,0.12)`,
      borderRadius: 6, overflow: 'hidden',
      fontFamily: '"JetBrains Mono", ui-monospace, monospace',
      margin: compact ? '4px 0' : '10px 0',
    }}>
      {/* Chrome — no mac traffic lights. A single accent rule + lang label. */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10,
        padding: '7px 12px',
        borderBottom: '1px solid rgba(234,229,216,0.08)',
        background: 'linear-gradient(to bottom, rgba(255,255,255,0.025), transparent)',
      }}>
        <span style={{
          width: 6, height: 6, borderRadius: 2, background: '#5dbfa7',
          boxShadow: '0 0 6px rgba(93,191,167,0.5)',
        }}/>
        <span style={{
          fontSize: 10.5, color: '#8a8275', letterSpacing: 0.5,
          fontFamily: '"JetBrains Mono", monospace',
        }}>{lang}</span>
        {showDiff && <span style={{
          fontSize: 10, padding: '1px 6px', borderRadius: 2,
          background: 'rgba(93,191,167,0.12)', color: '#5dbfa7',
          border: '1px solid rgba(93,191,167,0.25)',
          fontFamily: '"JetBrains Mono", monospace',
        }}>diff</span>}
        <span style={{ flex: 1 }}/>
        <button style={{
          fontSize: 10.5, color: '#8a8275', cursor: 'pointer', background: 'transparent',
          padding: '2px 8px', border: '1px solid rgba(234,229,216,0.12)', borderRadius: 3,
          fontFamily: '"JetBrains Mono", monospace',
        }}>copy</button>
        <button style={{
          fontSize: 10.5, color: '#5dbfa7', cursor: 'pointer',
          padding: '2px 8px', border: '1px solid rgba(93,191,167,0.3)', borderRadius: 3,
          background: 'rgba(93,191,167,0.08)',
          fontFamily: '"JetBrains Mono", monospace',
        }}>apply</button>
      </div>
      <div style={{ display: 'flex', fontSize: 11.5, lineHeight: 1.7 }}>
        <div style={{
          padding: '10px 6px 10px 14px', color: '#4a4438', textAlign: 'right',
          userSelect: 'none', borderRight: '1px solid rgba(234,229,216,0.05)',
          fontFamily: '"JetBrains Mono", monospace', fontSize: 10.5,
        }}>
          {lines.map((_, i) => <div key={i} style={{ lineHeight: 1.7 }}>{i + 1}</div>)}
        </div>
        <pre style={{
          margin: 0, padding: '10px 14px', flex: 1, overflow: 'auto',
          whiteSpace: 'pre', color: '#e5dfd0',
        }}>
          {lines.map((spans, i) => {
            const raw = rawLines[i] || '';
            const isPlus = showDiff && raw.trimStart().startsWith('+');
            const isMinus = showDiff && raw.trimStart().startsWith('-');
            const isHi = highlightLine != null && i + 1 === highlightLine;
            return (
              <div key={i} style={{
                background: isPlus ? 'rgba(122,186,138,0.10)' : isMinus ? 'rgba(227,135,117,0.10)' : isHi ? 'rgba(93,191,167,0.06)' : 'transparent',
                marginLeft: -14, paddingLeft: 12, marginRight: -14, paddingRight: 14,
                borderLeft: isPlus ? '2px solid rgba(122,186,138,0.6)' : isMinus ? '2px solid rgba(227,135,117,0.6)' : isHi ? '2px solid rgba(93,191,167,0.5)' : '2px solid transparent',
              }}>{spans.length ? spans : '\u00a0'}</div>
            );
          })}
        </pre>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Identity glyph — the Daimon ⫶
// ─────────────────────────────────────────────────────────────────
function LiminalGlyph({ size = 16, theme, animate = true }) {
  return (
    <div style={{
      position: 'relative', width: size, height: size,
      display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
      flexShrink: 0,
    }}>
      <div style={{
        width: size * 0.22, height: size * 0.22, borderRadius: 99,
        background: theme.accent, position: 'absolute',
        top: size * 0.08, left: '50%', transform: 'translateX(-50%)',
        boxShadow: `0 0 ${size * 0.4}px ${theme.accent}`,
        animation: animate ? 'liminalDot 3.2s ease-in-out infinite' : 'none',
      }}/>
      <div style={{
        width: size * 0.14, height: size * 0.58, borderRadius: size * 0.07,
        background: theme.ink,
        position: 'absolute', top: size * 0.38, left: '50%', transform: 'translateX(-50%)',
      }}/>
    </div>
  );
}

function LiminalMark({ size = 20, theme, animate = true, showLabel = true }) {
  if (!showLabel) return <LiminalGlyph size={size} theme={theme} animate={animate} />;
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
      <LiminalGlyph size={size} theme={theme} animate={animate} />
      <span style={{
        fontFamily: '"Fraunces", Georgia, serif', fontSize: size * 0.82,
        color: theme.ink, letterSpacing: -0.2, fontWeight: 500,
      }}>Daimon</span>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Kbd — platform-agnostic keyboard shortcut
// ─────────────────────────────────────────────────────────────────
function Kbd({ children, theme }) {
  return (
    <span style={{
      fontFamily: '"JetBrains Mono", monospace', fontSize: 10,
      color: theme.inkMuted, background: theme.bgDeep,
      border: `1px solid ${theme.line}`,
      padding: '1px 6px', borderRadius: 3, letterSpacing: 0.3,
    }}>{children}</span>
  );
}

// ─────────────────────────────────────────────────────────────────
// Sidebar
// ─────────────────────────────────────────────────────────────────
function LiminalSidebar({ theme, onToggleTheme, isDark }) {
  const items = [
    { glyph: '⫶', label: 'Chat', active: true },
    { glyph: '◇', label: 'Overview' },
    { glyph: '▤', label: 'Conversations', badge: 12 },
    { glyph: '❋', label: 'Memory' },
    { glyph: '⚒', label: 'Tools', badge: 24 },
    { glyph: '⌁', label: 'Integrations' },
    { glyph: '≈', label: 'Metrics' },
    { glyph: '∷', label: 'Logs' },
    { glyph: '⚙', label: 'Settings' },
  ];
  return (
    <div style={{
      width: 208, background: theme.bgSidebar,
      borderRight: `1px solid ${theme.line}`,
      display: 'flex', flexDirection: 'column',
      padding: '16px 10px',
      fontFamily: '"Inter", -apple-system, system-ui, sans-serif',
    }}>
      <div style={{ padding: '2px 8px 14px' }}>
        <LiminalMark size={22} theme={theme} />
      </div>

      <div style={{
        display: 'flex', alignItems: 'center', gap: 8,
        padding: '6px 10px', margin: '2px 0 12px',
        border: `1px solid ${theme.line}`, borderRadius: 5,
        background: theme.bgElev, fontSize: 12, color: theme.inkMuted,
      }}>
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <circle cx="11" cy="11" r="7"/><path d="m21 21-5-5"/>
        </svg>
        <span style={{ flex: 1, fontFamily: 'Fraunces, Georgia, serif', fontStyle: 'italic' }}>summon…</span>
        <Kbd theme={theme}>{MOD} K</Kbd>
      </div>

      {items.map(it => (
        <div key={it.label} style={{
          display: 'flex', alignItems: 'center', gap: 10,
          padding: '6px 10px', borderRadius: 4, margin: '1px 0',
          background: it.active ? theme.bgElev : 'transparent',
          color: it.active ? theme.ink : theme.inkSoft,
          fontSize: 13, fontWeight: it.active ? 500 : 400,
          borderLeft: it.active ? `2px solid ${theme.accent}` : '2px solid transparent',
          paddingLeft: it.active ? 8 : 10,
          cursor: 'pointer',
        }}>
          <span style={{
            width: 14, textAlign: 'center', fontSize: 13,
            color: it.active ? theme.accent : theme.inkMuted,
          }}>{it.glyph}</span>
          <span style={{ flex: 1 }}>{it.label}</span>
          {it.badge && (
            <span style={{
              fontSize: 10, color: theme.inkMuted,
              fontFamily: 'JetBrains Mono, monospace',
            }}>{it.badge}</span>
          )}
        </div>
      ))}

      <div style={{ flex: 1 }}/>

      <div style={{
        padding: 10, borderTop: `1px solid ${theme.line}`,
        fontSize: 10.5, fontFamily: '"JetBrains Mono", monospace',
        color: theme.inkMuted,
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', padding: '2px 0' }}>
          <span>model</span><span style={{ color: theme.ink }}>sonnet 4.5</span>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', padding: '2px 0' }}>
          <span>ctx</span>
          <span style={{ color: theme.ink }}>
            <span style={{
              display: 'inline-block', width: 30, height: 3,
              background: theme.line, borderRadius: 99, marginRight: 6,
              position: 'relative', top: -2,
            }}>
              <span style={{
                position: 'absolute', left: 0, top: 0, height: '100%',
                width: '6.2%', background: theme.accent, borderRadius: 99,
              }}/>
            </span>
            12.4k
          </span>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', padding: '2px 0' }}>
          <span>today</span><span style={{ color: theme.ink }}>$0.42</span>
        </div>
        <div style={{
          marginTop: 8, paddingTop: 8, borderTop: `1px solid ${theme.line}`,
          display: 'flex', alignItems: 'center', gap: 6,
        }}>
          <span style={{
            width: 6, height: 6, borderRadius: 99, background: theme.accent,
            boxShadow: `0 0 6px ${theme.accent}`,
            animation: 'liminalBreathe 2.4s ease-in-out infinite',
          }}/>
          <span style={{ flex: 1, color: theme.ink }}>listening</span>
          <span onClick={onToggleTheme} style={{
            cursor: 'pointer', color: theme.inkMuted, padding: 2, fontSize: 12,
          }}>{isDark ? '☾' : '☀'}</span>
        </div>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Tool card — richer, status bar on left, expandable
// ─────────────────────────────────────────────────────────────────
function LiminalTool({ tool, theme, onOpen }) {
  const [expanded, setExpanded] = React.useState(false);
  const statusColor = tool.status === 'error' ? theme.red
    : tool.status === 'running' ? theme.amber
    : theme.accent;
  const isRunning = tool.status === 'running';

  return (
    <div style={{ margin: '5px 0', display: 'flex' }}>
      <div style={{
        width: 2, background: statusColor, borderRadius: 99,
        marginRight: 12, opacity: isRunning ? 1 : 0.55,
        animation: isRunning ? 'liminalPulse 1.4s ease-in-out infinite' : 'none',
        alignSelf: 'stretch',
      }}/>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div onClick={() => setExpanded(e => !e)} style={{
          display: 'flex', alignItems: 'center', gap: 10,
          padding: '3px 0', fontSize: 12, cursor: 'pointer',
          fontFamily: '"JetBrains Mono", monospace',
        }}>
          <LiminalToolGlyph name={tool.name} color={statusColor} />
          <span style={{ color: theme.ink, fontWeight: 500 }}>{tool.name}</span>
          <span style={{
            color: theme.inkMuted, flex: 1, minWidth: 0,
            whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
          }}>{tool.preview}</span>
          {tool.status === 'error' && (
            <LiminalChip color={theme.red} theme={theme} soft>failed</LiminalChip>
          )}
          {isRunning && (
            <span style={{
              fontSize: 10.5, color: theme.amber,
              display: 'flex', alignItems: 'center', gap: 5,
            }}>
              <span style={{
                width: 6, height: 6, borderRadius: 99, background: theme.amber,
                animation: 'liminalBreathe 1s ease-in-out infinite',
              }}/>
              running
            </span>
          )}
          {tool.stats?.lines != null && (
            <LiminalChip color={theme.inkMuted} theme={theme}>{tool.stats.lines} lines</LiminalChip>
          )}
          {tool.stats?.matches != null && (
            <LiminalChip color={theme.accent} theme={theme} soft>{tool.stats.matches} matches</LiminalChip>
          )}
          {tool.stats?.size != null && (
            <LiminalChip color={theme.inkMuted} theme={theme}>{tool.stats.size}</LiminalChip>
          )}
          <span style={{ fontSize: 10.5, color: theme.inkFaint, minWidth: 38, textAlign: 'right' }}>
            {tool.duration}
          </span>
          <span style={{ fontSize: 10, color: theme.inkMuted, width: 10 }}>{expanded ? '▼' : '▸'}</span>
        </div>
        {expanded && (
          <div style={{
            marginTop: 6, padding: 10, borderRadius: 5,
            background: theme.bgElev, border: `1px solid ${theme.line}`,
            fontFamily: '"JetBrains Mono", monospace', fontSize: 11,
          }}>
            <div style={{ display: 'grid', gridTemplateColumns: '70px 1fr', gap: '4px 12px', marginBottom: 10 }}>
              <span style={{ color: theme.inkMuted, fontSize: 10, textTransform: 'uppercase', letterSpacing: 0.5, paddingTop: 2 }}>input</span>
              <pre style={{
                margin: 0, whiteSpace: 'pre-wrap', color: theme.inkSoft,
                background: theme.bgDeep, padding: 8, borderRadius: 3, fontSize: 10.5,
              }}>{JSON.stringify(tool.input, null, 2)}</pre>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '70px 1fr', gap: '4px 12px' }}>
              <span style={{ color: theme.inkMuted, fontSize: 10, textTransform: 'uppercase', letterSpacing: 0.5, paddingTop: 2 }}>output</span>
              <div>
                <pre style={{
                  margin: 0, whiteSpace: 'pre-wrap',
                  color: tool.status === 'error' ? theme.red : theme.inkSoft,
                  background: tool.status === 'error' ? `${theme.red}0d` : theme.bgDeep,
                  border: tool.status === 'error' ? `1px solid ${theme.red}33` : 'none',
                  padding: 8, borderRadius: 3, fontSize: 10.5,
                  maxHeight: 120, overflow: 'auto',
                }}>{tool.output}</pre>
                {onOpen && (
                  <button onClick={(e) => { e.stopPropagation(); onOpen(); }} style={{
                    marginTop: 6, fontSize: 10.5, color: theme.accent,
                    background: theme.accentSoft, border: `1px solid ${theme.accent}44`, borderRadius: 3,
                    padding: '3px 8px', cursor: 'pointer', fontFamily: '"JetBrains Mono", monospace',
                  }}>open in workspace →</button>
                )}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function LiminalChip({ children, color, soft, theme }) {
  return (
    <span style={{
      fontSize: 10, color,
      background: soft ? `${color}18` : 'transparent',
      border: soft ? `1px solid ${color}30` : `1px solid ${theme.line}`,
      padding: '1px 6px', borderRadius: 99,
      fontFamily: '"JetBrains Mono", monospace',
    }}>{children}</span>
  );
}

function LiminalToolGlyph({ name, color }) {
  const s = { width: 11, height: 11, viewBox: '0 0 24 24', fill: 'none', stroke: color, strokeWidth: 2, strokeLinecap: 'round', strokeLinejoin: 'round' };
  if (name === 'read_file') return <svg {...s}><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>;
  if (name === 'grep') return <svg {...s}><circle cx="11" cy="11" r="7"/><path d="m21 21-5-5"/></svg>;
  if (name === 'git_log') return <svg {...s}><circle cx="5" cy="6" r="2"/><circle cx="5" cy="18" r="2"/><circle cx="19" cy="18" r="2"/><path d="M5 8v8M7 18h7a4 4 0 0 0 4-4v-2"/></svg>;
  if (name === 'web_fetch') return <svg {...s}><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/></svg>;
  if (name === 'shell') return <svg {...s}><polyline points="4 7 9 12 4 17"/><line x1="12" y1="17" x2="20" y2="17"/></svg>;
  return <svg {...s}><circle cx="12" cy="12" r="9"/></svg>;
}

// ─────────────────────────────────────────────────────────────────
// Reasoning
// ─────────────────────────────────────────────────────────────────
function LiminalReasoning({ text, duration, theme, streaming }) {
  const [open, setOpen] = React.useState(!!streaming);
  return (
    <div style={{ margin: '4px 0 10px' }}>
      <div onClick={() => setOpen(o => !o)} style={{
        display: 'inline-flex', alignItems: 'center', gap: 8,
        padding: '3px 10px 3px 8px', borderRadius: 99,
        background: theme.bgDeep, border: `1px solid ${theme.line}`,
        fontSize: 11, color: theme.inkMuted, cursor: 'pointer',
        fontFamily: '"Inter", system-ui, sans-serif',
      }}>
        {streaming ? (
          <span style={{
            width: 7, height: 7, borderRadius: 99, background: theme.accent,
            animation: 'liminalBreathe 1.2s ease-in-out infinite',
          }}/>
        ) : (
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M21 12a9 9 0 1 1-6.22-8.56"/>
          </svg>
        )}
        <span style={{ fontFamily: '"Fraunces", serif', fontStyle: 'italic' }}>
          {streaming ? 'pondering…' : `pondered for ${duration}`}
        </span>
        <span style={{ fontSize: 9, opacity: 0.6 }}>{open ? '▾' : '▸'}</span>
      </div>
      {open && (
        <div style={{
          marginTop: 6, padding: '10px 14px',
          background: theme.bgElev, border: `1px solid ${theme.line}`, borderRadius: 5,
          fontSize: 12, color: theme.inkSoft, lineHeight: 1.65,
          whiteSpace: 'pre-wrap',
          fontFamily: '"Inter", system-ui, sans-serif',
          borderLeft: `2px solid ${theme.accent}66`,
        }}>{text}{streaming && <span style={{
          display: 'inline-block', width: 7, height: 13,
          background: theme.accent, marginLeft: 2, verticalAlign: 'text-bottom',
          animation: 'liminalCursor 1s steps(2) infinite',
        }}/>}</div>
      )}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Markdown — incl. tables, headings, numbered lists, code
// ─────────────────────────────────────────────────────────────────
function LiminalMd({ content, theme, streaming }) {
  // Split around fenced code blocks
  const parts = content.split(/```(\w+)?\n([\s\S]*?)```/g);
  const out = [];

  for (let i = 0; i < parts.length; i++) {
    if (i % 3 === 0) {
      const text = parts[i]; if (!text) continue;
      // Extract tables first (group consecutive | ... | lines)
      const lines = text.split('\n');
      let j = 0;
      while (j < lines.length) {
        const line = lines[j];
        // Table start: header row starting with |
        if (/^\s*\|.+\|\s*$/.test(line) && j + 1 < lines.length && /^\s*\|[\s|:\-]+\|\s*$/.test(lines[j+1])) {
          const header = line.trim().slice(1,-1).split('|').map(s => s.trim());
          const align = lines[j+1].trim().slice(1,-1).split('|').map(s => {
            const t = s.trim();
            if (t.startsWith(':') && t.endsWith(':')) return 'center';
            if (t.endsWith(':')) return 'right';
            return 'left';
          });
          let k = j + 2;
          const rows = [];
          while (k < lines.length && /^\s*\|.+\|\s*$/.test(lines[k])) {
            rows.push(lines[k].trim().slice(1,-1).split('|').map(s => s.trim()));
            k++;
          }
          out.push(<LiminalTable key={`t-${i}-${j}`} header={header} rows={rows} align={align} theme={theme} />);
          j = k; continue;
        }
        // Headings / lists / paragraphs
        const key = `${i}-${j}`;
        if (line.startsWith('## ')) out.push(<h3 key={key} style={{
          fontFamily: '"Fraunces", serif', fontSize: 17, fontWeight: 500,
          color: theme.ink, margin: '14px 0 6px', letterSpacing: -0.3,
        }}>{line.slice(3)}</h3>);
        else if (line.startsWith('### ')) out.push(<h4 key={key} style={{
          fontFamily: '"Inter", system-ui, sans-serif', fontSize: 11, fontWeight: 600,
          color: theme.accent, margin: '10px 0 3px',
          textTransform: 'uppercase', letterSpacing: 0.8,
        }}>{line.slice(4)}</h4>);
        else if (line.match(/^\d+\. /)) out.push(
          <div key={key} style={{ margin: '3px 0', display: 'flex', gap: 8 }}>
            <span style={{
              color: theme.accent, fontFamily: '"JetBrains Mono", monospace',
              fontSize: 12, minWidth: 14,
            }}>{line.match(/^(\d+)\./)[1]}</span>
            <span style={{ flex: 1 }}>{renderInline(line.replace(/^\d+\.\s*/, ''), theme)}</span>
          </div>
        );
        else if (line.trim() === '') out.push(<div key={key} style={{ height: 5 }}/>);
        else out.push(<div key={key} style={{ margin: '2px 0' }}>{renderInline(line, theme)}</div>);
        j++;
      }
    } else if (i % 3 === 2) {
      out.push(<LiminalCode key={`c-${i}`} lang={parts[i-1] || 'text'} code={parts[i].trim()}
        theme={theme} showDiff={/^[+\- ]/m.test(parts[i]) && /\n[+\-]/.test(parts[i])} />);
    }
  }

  return <div style={{
    fontFamily: '"Inter", system-ui, sans-serif', fontSize: 13.5,
    color: theme.ink, lineHeight: 1.65,
  }}>
    {out}
    {streaming && <span style={{
      display: 'inline-block', width: 8, height: 14,
      background: theme.accent, marginLeft: 2, verticalAlign: 'text-bottom',
      animation: 'liminalCursor 1s steps(2) infinite',
    }}/>}
  </div>;
}

function renderInline(line, theme) {
  const segs = line.split(/(\*\*[^*]+\*\*|`[^`]+`)/g);
  return segs.map((s, k) => {
    if (s.startsWith('**')) return <strong key={k} style={{ color: theme.ink, fontWeight: 600 }}>{s.slice(2, -2)}</strong>;
    if (s.startsWith('`')) return <code key={k} style={{
      fontFamily: '"JetBrains Mono", monospace', fontSize: 11.5,
      background: theme.bgDeep, padding: '1px 5px', borderRadius: 3,
      color: theme.accent, border: `1px solid ${theme.line}`,
    }}>{s.slice(1, -1)}</code>;
    return <span key={k}>{s}</span>;
  });
}

function LiminalTable({ header, rows, align, theme }) {
  return (
    <div style={{
      margin: '10px 0', border: `1px solid ${theme.line}`, borderRadius: 5,
      overflow: 'hidden', background: theme.bgElev,
    }}>
      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
        <thead>
          <tr>{header.map((h, i) => (
            <th key={i} style={{
              padding: '7px 12px', textAlign: align[i] || 'left',
              fontSize: 10.5, color: theme.inkMuted,
              textTransform: 'uppercase', letterSpacing: 0.6, fontWeight: 600,
              background: theme.bgDeep, borderBottom: `1px solid ${theme.line}`,
              fontFamily: '"Inter", system-ui, sans-serif',
            }}>{h}</th>
          ))}</tr>
        </thead>
        <tbody>
          {rows.map((row, r) => (
            <tr key={r}>{row.map((cell, i) => {
              const isNum = align[i] === 'right';
              const isBold = /\*\*.*\*\*/.test(cell);
              return <td key={i} style={{
                padding: '6px 12px', textAlign: align[i] || 'left',
                color: isBold ? theme.red : theme.ink,
                fontFamily: isNum ? '"JetBrains Mono", monospace' : '"Inter", sans-serif',
                fontSize: isNum ? 11.5 : 12.5,
                fontWeight: isBold ? 600 : 400,
                borderTop: r > 0 ? `1px solid ${theme.line}` : 'none',
              }}>{renderInline(cell, theme)}</td>;
            })}</tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Timeline block
// ─────────────────────────────────────────────────────────────────
function LiminalTimeline({ events, theme }) {
  const lvlColor = { info: theme.accent, warn: theme.amber, error: theme.red };
  return (
    <div style={{
      margin: '10px 0', padding: '10px 0',
      border: `1px solid ${theme.line}`, borderRadius: 5,
      background: theme.bgElev,
    }}>
      {events.map((e, i) => (
        <div key={i} style={{
          display: 'flex', alignItems: 'center', gap: 10,
          padding: '5px 14px', position: 'relative',
        }}>
          <span style={{
            fontFamily: '"JetBrains Mono", monospace', fontSize: 11,
            color: theme.inkMuted, width: 42,
          }}>{e.t}</span>
          <span style={{
            width: 7, height: 7, borderRadius: 99,
            background: lvlColor[e.lvl], flexShrink: 0,
            boxShadow: `0 0 4px ${lvlColor[e.lvl]}80`,
          }}/>
          <span style={{ flex: 1, fontSize: 12.5, color: theme.ink }}>{e.msg}</span>
          <LiminalChip color={lvlColor[e.lvl]} theme={theme} soft>{e.lvl}</LiminalChip>
        </div>
      ))}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Speaker rows
// ─────────────────────────────────────────────────────────────────
// Speaker row — the glyph sits in the -42px gutter, ON the thread line.
// The label + time bar sits in the content column.
function LiminalSpeaker({ label, time, glyph, color, theme, italic }) {
  return (
    <div style={{ position: 'relative', marginBottom: 10 }}>
      {/* Glyph — floats into the thread gutter */}
      <div style={{
        position: 'absolute', left: -42, top: -1,
        width: 24, height: 24, borderRadius: 99,
        background: theme.bg,
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        // halo that occludes the thread line behind the glyph
        boxShadow: `0 0 0 4px ${theme.bg}`,
      }}>{glyph}</div>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 10 }}>
        <span style={{
          fontSize: 12, fontWeight: 600, color,
          fontFamily: '"Inter", system-ui, sans-serif',
          letterSpacing: 0.1,
        }}>{label}</span>
        {italic && (
          <span style={{
            fontFamily: '"Fraunces", serif', fontStyle: 'italic',
            fontSize: 12, color: theme.inkMuted,
          }}>{italic}</span>
        )}
        <span style={{ flex: 1, height: 1, background: theme.line, alignSelf: 'center' }}/>
        <span style={{
          fontSize: 10.5, color: theme.inkFaint,
          fontFamily: '"JetBrains Mono", monospace',
        }}>{time}</span>
      </div>
    </div>
  );
}

function LiminalUserMsg({ msg, theme }) {
  return (
    <div style={{ padding: '18px 0 8px' }}>
      <LiminalSpeaker
        label="You" time={msg.time} theme={theme} color={theme.ink}
        glyph={
          <div style={{
            width: 18, height: 18, borderRadius: 4,
            background: theme.ink, color: theme.bg,
            fontSize: 9, fontWeight: 700,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            fontFamily: 'Inter, sans-serif', letterSpacing: 0.3,
          }}>AR</div>
        }
      />
      <div style={{
        fontSize: 13.5, color: theme.ink, lineHeight: 1.6,
        fontFamily: '"Inter", system-ui, sans-serif',
      }}>{msg.content}</div>
    </div>
  );
}

function LiminalAssistantMsg({ msg, theme, onOpenArtifact }) {
  return (
    <div style={{ padding: '16px 0 24px' }}>
      <LiminalSpeaker
        label="Daimon" italic="speaks" time={msg.time}
        theme={theme} color={theme.accent}
        glyph={
          <div style={{
            width: 18, height: 18, display: 'flex', alignItems: 'center', justifyContent: 'center',
          }}><LiminalGlyph size={16} theme={theme} animate={!!msg.streaming} /></div>
        }
      />
      {msg.reasoning && (
        <LiminalReasoning text={msg.reasoning} duration={msg.reasoningDuration}
          theme={theme} streaming={msg.streaming && msg.reasoningDuration === 'now'} />
      )}
      {msg.blocks.map((b, i) => {
        if (b.kind === 'tool') return <LiminalTool key={i} tool={b} theme={theme}
          onOpen={b.name === 'read_file' && !b.error ? () => onOpenArtifact('log') : null} />;
        if (b.kind === 'text') return (
          <div key={i} style={{ margin: '8px 0' }}>
            <LiminalMd content={b.content} theme={theme} streaming={b.streaming} />
          </div>
        );
        if (b.kind === 'timeline') return <LiminalTimeline key={i} events={b.events} theme={theme} />;
        return null;
      })}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Workspace — shared shell + artifact renderers
// ─────────────────────────────────────────────────────────────────
function WSShell({ title, icon, actions, onClose, theme, children, subtitle }) {
  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', background: theme.bgElev }}>
      <div style={{
        padding: '10px 16px', borderBottom: `1px solid ${theme.line}`,
        display: 'flex', alignItems: 'center', gap: 10, background: theme.bg,
      }}>
        {icon}
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{
            fontFamily: '"JetBrains Mono", monospace', fontSize: 12, color: theme.ink,
            whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
          }}>{title}</div>
          {subtitle && <div style={{
            fontFamily: '"Fraunces", serif', fontStyle: 'italic',
            fontSize: 11, color: theme.inkMuted, marginTop: 1,
          }}>{subtitle}</div>}
        </div>
        {actions}
        <span onClick={onClose} style={{ fontSize: 16, color: theme.inkMuted, cursor: 'pointer', marginLeft: 4 }}>×</span>
      </div>
      {children}
    </div>
  );
}

function WSEmpty({ theme }) {
  return (
    <div style={{
      height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center',
      flexDirection: 'column', gap: 14, padding: 40, background: theme.bgElev,
      color: theme.inkMuted, fontFamily: '"Inter", system-ui, sans-serif',
      fontSize: 12.5, textAlign: 'center',
    }}>
      <LiminalMark size={36} theme={theme} />
      <div style={{ fontFamily: '"Fraunces", serif', fontStyle: 'italic', fontSize: 14, color: theme.inkMuted }}>
        the workspace listens.
      </div>
      <div style={{ fontSize: 11, color: theme.inkFaint, maxWidth: 240, lineHeight: 1.5 }}>
        Open an artifact from the conversation — code, logs, tables, images. Artifacts stay here while you keep chatting.
      </div>
      <div style={{
        marginTop: 8, display: 'flex', gap: 4, fontSize: 10,
        fontFamily: '"JetBrains Mono", monospace', color: theme.inkFaint,
      }}>
        <Kbd theme={theme}>{MOD} ⇧ O</Kbd>
        <span style={{ alignSelf: 'center' }}>opens last artifact</span>
      </div>
    </div>
  );
}

function WSIcon({ kind, theme }) {
  const s = { width: 13, height: 13, viewBox: '0 0 24 24', fill: 'none', stroke: theme.accent, strokeWidth: 1.8, strokeLinecap: 'round', strokeLinejoin: 'round' };
  if (kind === 'code') return <svg {...s}><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>;
  if (kind === 'log') return <svg {...s}><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/></svg>;
  if (kind === 'table') return <svg {...s}><rect x="3" y="5" width="18" height="14" rx="1"/><line x1="3" y1="10" x2="21" y2="10"/><line x1="9" y1="5" x2="9" y2="19"/></svg>;
  if (kind === 'image') return <svg {...s}><rect x="3" y="5" width="18" height="14" rx="1"/><circle cx="9" cy="10" r="1.5"/><path d="m3 17 5-5 4 4 3-3 6 6"/></svg>;
  return null;
}

// Diff parser — splits into hunks for navigation
function parseDiff(code) {
  const lines = code.split('\n');
  const hunks = [];
  let cur = null;
  lines.forEach((l, idx) => {
    const isChange = /^[+\-]/.test(l.trimStart());
    if (isChange && !cur) {
      cur = { start: idx, end: idx, plus: 0, minus: 0 };
      hunks.push(cur);
    }
    if (isChange) {
      cur.end = idx;
      if (l.trimStart().startsWith('+')) cur.plus++;
      else cur.minus++;
    } else if (cur && idx - cur.end > 2) {
      cur = null;
    }
  });
  return hunks;
}

function LiminalWorkspace({ artifact, onClose, theme }) {
  const [hunkIdx, setHunkIdx] = React.useState(0);
  if (!artifact) return <WSEmpty theme={theme} />;

  if (artifact.type === 'code') {
    const hunks = artifact.diff ? parseDiff(artifact.content) : [];
    const activeHunk = hunks[hunkIdx];
    return (
      <WSShell theme={theme} onClose={onClose}
        icon={<WSIcon kind="code" theme={theme} />}
        title={artifact.title}
        subtitle={artifact.diff ? `${hunks.length} hunk${hunks.length !== 1 ? 's' : ''} · proposed change` : undefined}
        actions={<>
          {artifact.diff && hunks.length > 0 && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginRight: 6 }}>
              <button onClick={() => setHunkIdx(i => Math.max(0, i - 1))}
                disabled={hunkIdx === 0}
                style={{
                  fontSize: 10, padding: '3px 7px', background: 'transparent',
                  border: `1px solid ${theme.line}`, borderRadius: 3,
                  color: hunkIdx === 0 ? theme.inkFaint : theme.inkSoft,
                  cursor: hunkIdx === 0 ? 'default' : 'pointer',
                  fontFamily: '"JetBrains Mono", monospace',
                }}>↑</button>
              <span style={{ fontSize: 10.5, color: theme.inkMuted, fontFamily: 'JetBrains Mono, monospace' }}>
                hunk {hunkIdx + 1}/{hunks.length}
              </span>
              <button onClick={() => setHunkIdx(i => Math.min(hunks.length - 1, i + 1))}
                disabled={hunkIdx >= hunks.length - 1}
                style={{
                  fontSize: 10, padding: '3px 7px', background: 'transparent',
                  border: `1px solid ${theme.line}`, borderRadius: 3,
                  color: hunkIdx >= hunks.length - 1 ? theme.inkFaint : theme.inkSoft,
                  cursor: hunkIdx >= hunks.length - 1 ? 'default' : 'pointer',
                  fontFamily: '"JetBrains Mono", monospace',
                }}>↓</button>
            </div>
          )}
          <button style={{
            fontSize: 11, padding: '4px 10px', background: 'transparent',
            border: `1px solid ${theme.line}`, borderRadius: 4,
            color: theme.inkSoft, cursor: 'pointer', fontFamily: '"Inter", sans-serif',
          }}>Copy</button>
          <button style={{
            fontSize: 11, padding: '4px 10px', background: theme.accent,
            border: 'none', borderRadius: 4, color: theme.bgElev, cursor: 'pointer',
            fontWeight: 500, fontFamily: '"Inter", sans-serif',
          }}>{artifact.diff ? 'Apply patch' : 'Save'}</button>
        </>}
      >
        <div style={{ flex: 1, overflow: 'auto', padding: 14, background: theme.bgDeep }}>
          <LiminalCode code={artifact.content} lang={artifact.language} theme={theme} showDiff={artifact.diff}
            highlightLine={activeHunk ? activeHunk.start + 1 : null} />
        </div>
      </WSShell>
    );
  }

  if (artifact.type === 'table') {
    return (
      <WSShell theme={theme} onClose={onClose}
        icon={<WSIcon kind="table" theme={theme} />}
        title={artifact.title}
        subtitle={`${artifact.rows.length} rows`}
        actions={<button style={{
          fontSize: 11, padding: '4px 10px', background: 'transparent',
          border: `1px solid ${theme.line}`, borderRadius: 4,
          color: theme.inkSoft, cursor: 'pointer', fontFamily: '"Inter", sans-serif',
        }}>Export CSV</button>}
      >
        <div style={{ flex: 1, overflow: 'auto', padding: 16 }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr>{artifact.header.map((h, i) => (
                <th key={i} style={{
                  padding: '9px 12px', textAlign: artifact.align[i] || 'left',
                  fontSize: 10.5, color: theme.inkMuted,
                  textTransform: 'uppercase', letterSpacing: 0.6, fontWeight: 600,
                  background: theme.bgDeep, borderBottom: `1px solid ${theme.lineStrong}`,
                  fontFamily: '"Inter", sans-serif', position: 'sticky', top: 0,
                }}>{h}</th>
              ))}</tr>
            </thead>
            <tbody>
              {artifact.rows.map((row, r) => {
                const bad = row[row.length - 1] === 'bad';
                const cells = bad ? row.slice(0, -1) : row;
                return (
                  <tr key={r} style={{ background: bad ? `${theme.red}08` : 'transparent' }}>
                    {cells.map((cell, i) => {
                      const align = artifact.align[i] || 'left';
                      const isNum = align === 'right';
                      const isLastCol = i === cells.length - 1;
                      return <td key={i} style={{
                        padding: '8px 12px', textAlign: align,
                        color: isLastCol && bad ? theme.red : theme.ink,
                        fontFamily: isNum ? '"JetBrains Mono", monospace' : '"Inter", sans-serif',
                        fontSize: isNum ? 11.5 : 12.5,
                        fontWeight: isLastCol && bad ? 600 : 400,
                        borderTop: r > 0 ? `1px solid ${theme.line}` : 'none',
                      }}>{cell}</td>;
                    })}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </WSShell>
    );
  }

  if (artifact.type === 'image') {
    return (
      <WSShell theme={theme} onClose={onClose}
        icon={<WSIcon kind="image" theme={theme} />}
        title={artifact.title}
        actions={<button style={{
          fontSize: 11, padding: '4px 10px', background: 'transparent',
          border: `1px solid ${theme.line}`, borderRadius: 4,
          color: theme.inkSoft, cursor: 'pointer', fontFamily: '"Inter", sans-serif',
        }}>Save</button>}
      >
        <div style={{
          flex: 1, display: 'flex', flexDirection: 'column',
          alignItems: 'center', justifyContent: 'center',
          padding: 30, background: theme.bgDeep, gap: 18,
        }}>
          <FakeChart theme={theme} />
          {artifact.caption && (
            <div style={{
              fontFamily: '"Fraunces", serif', fontStyle: 'italic',
              fontSize: 12, color: theme.inkMuted, textAlign: 'center',
              maxWidth: 380, lineHeight: 1.5,
            }}>{artifact.caption}</div>
          )}
        </div>
      </WSShell>
    );
  }

  if (artifact.type === 'log') {
    return (
      <WSShell theme={theme} onClose={onClose}
        icon={<WSIcon kind="log" theme={theme} />}
        title={artifact.title}
        actions={<div style={{ display: 'flex', gap: 4, fontSize: 10, fontFamily: '"JetBrains Mono", monospace' }}>
          <span style={{ padding: '2px 6px', border: `1px solid ${theme.red}55`, color: theme.red, borderRadius: 3, background: `${theme.red}12` }}>ERROR 4</span>
          <span style={{ padding: '2px 6px', border: `1px solid ${theme.amber}55`, color: theme.amber, borderRadius: 3, background: `${theme.amber}12` }}>WARN 2</span>
          <span style={{ padding: '2px 6px', border: `1px solid ${theme.green}55`, color: theme.green, borderRadius: 3, background: `${theme.green}12` }}>INFO 2</span>
        </div>}
      >
        <div style={{ flex: 1, overflow: 'auto' }}>
          {artifact.entries.map((e, i) => (
            <div key={i} style={{
              display: 'flex', gap: 12, padding: '4px 16px',
              fontFamily: '"JetBrains Mono", monospace', fontSize: 11,
              borderBottom: `1px solid ${theme.line}`,
              background: e.lvl === 'ERROR' ? `${theme.red}08` : 'transparent',
            }}>
              <span style={{ color: theme.inkMuted, width: 64 }}>{e.t}</span>
              <span style={{
                width: 44, fontWeight: 600, fontSize: 10,
                color: e.lvl === 'ERROR' ? theme.red :
                       e.lvl === 'WARN' ? theme.amber : theme.green,
              }}>{e.lvl}</span>
              <span style={{ color: theme.ink, flex: 1 }}>{e.msg}</span>
            </div>
          ))}
        </div>
      </WSShell>
    );
  }
  return null;
}

// A hand-drawn chart that looks native to Liminal (no external deps)
function FakeChart({ theme }) {
  const W = 420, H = 220, pad = 28;
  // Points representing a 24h p99 latency curve with 2 spikes
  const pts = [
    [0,50],[1,48],[2,52],[3,180],[3.2,150],[4,70],[5,65],[6,70],[7,75],
    [7.3,160],[7.5,140],[8,80],[9,70],[10,65],[12,60],[14,68],[16,72],[18,78],[20,70],[22,60],[24,55]
  ];
  const xs = x => pad + (x / 24) * (W - pad * 2);
  const ys = y => H - pad - (y / 220) * (H - pad * 2);
  const path = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${xs(p[0])},${ys(p[1])}`).join(' ');
  return (
    <svg width={W} height={H} style={{
      background: theme.bgElev, borderRadius: 6,
      border: `1px solid ${theme.line}`,
    }}>
      {/* grid */}
      {[0, 1, 2, 3, 4].map(i => (
        <line key={i} x1={pad} x2={W - pad}
          y1={pad + i * (H - pad * 2) / 4} y2={pad + i * (H - pad * 2) / 4}
          stroke={theme.line} strokeDasharray="2 3" />
      ))}
      {/* axes */}
      <line x1={pad} y1={H - pad} x2={W - pad} y2={H - pad} stroke={theme.lineStrong} strokeWidth={1} />
      {/* threshold */}
      <line x1={pad} x2={W - pad} y1={ys(100)} y2={ys(100)}
        stroke={theme.amber} strokeWidth={1} strokeDasharray="3 3" opacity={0.6} />
      <text x={W - pad - 4} y={ys(100) - 4} fontSize={9} fill={theme.amber}
        textAnchor="end" fontFamily="JetBrains Mono, monospace">SLO 100ms</text>
      {/* spike highlights */}
      <rect x={xs(3)} y={pad} width={xs(3.5) - xs(3)} height={H - pad * 2} fill={theme.red} opacity={0.08}/>
      <rect x={xs(7.2)} y={pad} width={xs(7.6) - xs(7.2)} height={H - pad * 2} fill={theme.red} opacity={0.08}/>
      {/* line */}
      <path d={path} stroke={theme.accent} strokeWidth={1.8} fill="none" strokeLinejoin="round" />
      {/* spike dots */}
      <circle cx={xs(3)} cy={ys(180)} r={3.5} fill={theme.red} />
      <circle cx={xs(7.3)} cy={ys(160)} r={3.5} fill={theme.red} />
      {/* x labels */}
      {[0, 6, 12, 18, 24].map(h => (
        <text key={h} x={xs(h)} y={H - pad + 14} fontSize={9} fill={theme.inkMuted}
          textAnchor="middle" fontFamily="JetBrains Mono, monospace">{String(h).padStart(2,'0')}:00</text>
      ))}
    </svg>
  );
}

// ─────────────────────────────────────────────────────────────────
// Input
// ─────────────────────────────────────────────────────────────────
function LiminalInput({ theme, showCmd }) {
  return (
    <div style={{ padding: 16, borderTop: `1px solid ${theme.line}`, background: theme.bg }}>
      <div style={{
        border: `1px solid ${theme.lineStrong}`, borderRadius: 8,
        background: theme.bgElev, padding: '10px 12px',
        boxShadow: `0 1px 2px rgba(0,0,0,0.03)`,
      }}>
        <div style={{
          fontSize: 13.5, minHeight: 36, marginBottom: 8,
          display: 'flex', alignItems: 'center', gap: 6,
        }}>
          <span style={{ color: theme.accent, fontWeight: 600, fontFamily: '"JetBrains Mono", monospace' }}>⫶</span>
          <span style={{
            color: theme.inkMuted,
            fontFamily: '"Fraunces", serif', fontStyle: 'italic', fontSize: 14,
          }}>speak, and Daimon listens…</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11 }}>
          <LiminalPill theme={theme}>Attach</LiminalPill>
          <LiminalPill theme={theme}>/ tools</LiminalPill>
          <LiminalPill theme={theme}>@ mention</LiminalPill>
          <span style={{ flex: 1 }}/>
          <span onClick={showCmd} style={{ cursor: 'pointer' }}><Kbd theme={theme}>{MOD} K</Kbd></span>
          <button style={{
            background: theme.accent, color: theme.bgElev,
            fontSize: 12, fontWeight: 600, padding: '5px 14px', borderRadius: 5,
            border: 'none', cursor: 'pointer', fontFamily: '"Inter", sans-serif',
            display: 'flex', alignItems: 'center', gap: 6,
          }}>Invoke
            <span style={{
              fontFamily: '"JetBrains Mono", monospace', fontSize: 10,
              background: 'rgba(255,255,255,0.2)', padding: '1px 5px', borderRadius: 3,
            }}>{ENTER}</span>
          </button>
        </div>
      </div>
    </div>
  );
}

function LiminalPill({ theme, children }) {
  return (
    <button style={{
      background: 'transparent', border: `1px solid ${theme.line}`,
      padding: '3px 9px', borderRadius: 99, color: theme.inkSoft,
      cursor: 'pointer', fontFamily: '"Inter", sans-serif', fontSize: 11,
      display: 'flex', alignItems: 'center', gap: 5,
    }}>{children}</button>
  );
}

// ─────────────────────────────────────────────────────────────────
// Command palette
// ─────────────────────────────────────────────────────────────────
function LiminalCmd({ onClose, theme }) {
  return (
    <div onClick={onClose} style={{
      position: 'absolute', inset: 0, background: 'rgba(10,8,5,0.5)',
      display: 'flex', alignItems: 'flex-start', justifyContent: 'center',
      paddingTop: 90, zIndex: 50, backdropFilter: 'blur(4px)',
    }}>
      <div onClick={(e) => e.stopPropagation()} style={{
        width: 520, background: theme.bgElev,
        border: `1px solid ${theme.lineStrong}`,
        borderRadius: 10, overflow: 'hidden',
        boxShadow: `0 0 0 1px ${theme.accent}22, 0 20px 60px rgba(0,0,0,0.3)`,
      }}>
        <div style={{
          padding: '14px 18px', borderBottom: `1px solid ${theme.line}`,
          display: 'flex', alignItems: 'center', gap: 10,
        }}>
          <span style={{ color: theme.accent, fontSize: 14, fontFamily: 'Fraunces, serif', fontStyle: 'italic' }}>summon</span>
          <span style={{ fontSize: 14, color: theme.inkMuted, flex: 1, fontFamily: '"Fraunces", serif', fontStyle: 'italic' }}>
            a command, a tool, a memory…
          </span>
          <Kbd theme={theme}>esc</Kbd>
        </div>
        <div style={{ padding: '6px 0', fontFamily: '"Inter", system-ui, sans-serif' }}>
          <div style={{
            padding: '4px 18px', fontSize: 10, color: theme.inkMuted,
            textTransform: 'uppercase', letterSpacing: 0.8,
            fontFamily: '"JetBrains Mono", monospace',
          }}>· actions</div>
          {[
            { glyph: '⫶', l: 'New thread', s: `${MOD} N`, hi: true },
            { glyph: '⎇', l: 'Fork conversation', s: `${MOD} ⇧ F` },
            { glyph: '❋', l: 'Save to memory', s: `${MOD} M` },
            { glyph: '⚒', l: 'Run tool…', s: `${MOD} T` },
            { glyph: '☾', l: 'Toggle theme', s: `${MOD} .` },
          ].map(i => (
            <div key={i.l} style={{
              padding: '7px 18px', display: 'flex', alignItems: 'center', gap: 12,
              fontSize: 13, color: theme.ink,
              background: i.hi ? theme.accentSoft : 'transparent',
              borderLeft: i.hi ? `2px solid ${theme.accent}` : '2px solid transparent',
              paddingLeft: i.hi ? 16 : 18, cursor: 'pointer',
            }}>
              <span style={{
                width: 22, height: 22, borderRadius: 4, background: theme.bgDeep,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 12, color: i.hi ? theme.accent : theme.inkMuted,
              }}>{i.glyph}</span>
              <span style={{ flex: 1 }}>{i.l}</span>
              <Kbd theme={theme}>{i.s}</Kbd>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Main — chat + workspace
// ─────────────────────────────────────────────────────────────────
function LiminalChat({ theme, isDark, onToggleTheme, showCmd, cmdOpen, closeCmd }) {
  const [artifact, setArtifact] = React.useState(WORKSPACE_ARTIFACTS.patch);
  const scrollRef = React.useRef(null);

  return (
    <div style={{
      height: '100%', display: 'flex', flexDirection: 'row',
      background: theme.bg, color: theme.ink, position: 'relative',
      fontFamily: '"Inter", system-ui, sans-serif',
    }}>
      <LiminalSidebar theme={theme} onToggleTheme={onToggleTheme} isDark={isDark} />

      <div style={{ flex: 1.3, display: 'flex', flexDirection: 'column', minWidth: 0 }}>
        <div style={{
          padding: '11px 24px', borderBottom: `1px solid ${theme.line}`,
          display: 'flex', alignItems: 'center', gap: 12, background: theme.bg,
        }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{
              fontSize: 14, fontFamily: '"Fraunces", serif', fontWeight: 500,
              color: theme.ink, letterSpacing: -0.2,
              whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
            }}>Payment service anomalies</div>
            <div style={{
              fontSize: 11, color: theme.inkMuted, marginTop: 2,
              display: 'flex', gap: 10, fontFamily: '"JetBrains Mono", monospace',
            }}>
              <span>started 14:32</span><span>·</span>
              <span>5 tools</span><span>·</span>
              <span>$0.042</span><span>·</span>
              <span>4 iter</span>
            </div>
          </div>
          <span style={{
            fontSize: 10.5, color: theme.accent,
            padding: '3px 9px', border: `1px solid ${theme.accent}44`, borderRadius: 99,
            display: 'flex', alignItems: 'center', gap: 6, fontFamily: '"JetBrains Mono", monospace',
            background: theme.accentSoft,
          }}>
            <span style={{
              width: 5, height: 5, borderRadius: 99, background: theme.accent,
              boxShadow: `0 0 4px ${theme.accent}`,
              animation: 'liminalBreathe 2.4s ease-in-out infinite',
            }}/>
            listening
          </span>
        </div>

        <div ref={scrollRef} style={{ flex: 1, overflow: 'auto', position: 'relative' }}>
          <div style={{
            position: 'relative',
            padding: '8px 40px 8px 88px',  // 88px left = 40px (thread) + 48px (gutter)
            maxWidth: 880, margin: '0 auto',
          }}>
            {/* The thread — a single line running the height of the conversation.
                Fades at top and bottom so it feels liminal, not mechanical. */}
            <div aria-hidden style={{
              position: 'absolute', left: 46, top: 0, bottom: 0, width: 1,
              background: `linear-gradient(to bottom, transparent 0, ${theme.accent}66 24px, ${theme.accent}33 40%, ${theme.line} 70%, transparent 100%)`,
              pointerEvents: 'none',
            }}/>
            {MOCK_CONVO.map(m => m.role === 'user'
              ? <LiminalUserMsg key={m.id} msg={m} theme={theme} />
              : <LiminalAssistantMsg key={m.id} msg={m} theme={theme}
                  onOpenArtifact={(k) => setArtifact(WORKSPACE_ARTIFACTS[k])} />)}
          </div>
        </div>

        <LiminalInput theme={theme} showCmd={showCmd} />
      </div>

      <div style={{ flex: 1, borderLeft: `1px solid ${theme.line}`, minWidth: 0 }}>
        <LiminalWorkspace artifact={artifact} onClose={() => setArtifact(null)} theme={theme} />
      </div>

      {cmdOpen && <LiminalCmd onClose={closeCmd} theme={theme} />}

      <style>{`
        @keyframes liminalBreathe { 0%,100% { opacity: 1; transform: scale(1); } 50% { opacity: 0.5; transform: scale(0.75); } }
        @keyframes liminalDot { 0%,100% { opacity: 1; } 50% { opacity: 0.55; } }
        @keyframes liminalPulse { 0%,100% { opacity: 0.4; } 50% { opacity: 1; } }
        @keyframes liminalCursor { 0%,50% { opacity: 1; } 51%,100% { opacity: 0; } }
      `}</style>
    </div>
  );
}

Object.assign(window, { LiminalChat, LiminalMark, LiminalGlyph, LIMINAL_TOKENS });
