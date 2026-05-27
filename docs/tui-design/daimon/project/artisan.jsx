// Direction B — "Artisan" / Linear-Raycast quality
// Cool neutral, refined, tight spacing, monochromatic with indigo accent.
// Premium, tool-focused, very clean.

const artisanStyles = {
  bg: '#fbfbfc',
  bgElev: '#ffffff',
  bgDeep: '#f4f4f6',
  bgSidebar: '#f7f7f9',
  ink: '#0a0a0f',
  inkSoft: '#3a3a42',
  inkMuted: '#7b7b85',
  inkFaint: '#b8b8c0',
  line: '#ececef',
  lineStrong: '#dcdce1',
  accent: '#5b5bd6',        // indigo
  accentSoft: '#eeeefc',
  accentBorder: '#d8d8f5',
  green: '#30a46c',
  red: '#e5484d',
  amber: '#ffb224',
  sans: '"Inter", -apple-system, system-ui, sans-serif',
  mono: '"JetBrains Mono", ui-monospace, SF Mono, monospace',
};

function ArtisanWordmark({ size = 18 }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <div style={{
        width: size + 2, height: size + 2, borderRadius: 5,
        background: `linear-gradient(135deg, ${artisanStyles.ink}, ${artisanStyles.accent})`,
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        boxShadow: '0 1px 2px rgba(91,91,214,0.25), inset 0 0 0 1px rgba(255,255,255,0.1)',
      }}>
        <div style={{
          color: '#fff', fontSize: size * 0.6, fontWeight: 700,
          fontFamily: artisanStyles.sans, letterSpacing: -0.5, marginTop: -1,
        }}>D</div>
      </div>
      <span style={{
        fontFamily: artisanStyles.sans, fontSize: size * 0.85, fontWeight: 600,
        letterSpacing: -0.3, color: artisanStyles.ink,
      }}>Daimon</span>
    </div>
  );
}

function ArtisanSidebar() {
  const items = [
    { icon: 'chat', label: 'Chat', active: true, badge: '3' },
    { icon: 'home', label: 'Overview' },
    { icon: 'chart', label: 'Metrics' },
    { icon: 'list', label: 'Conversations' },
    { icon: 'brain', label: 'Memory' },
    { icon: 'wrench', label: 'Tools', badge: '12' },
    { icon: 'plug', label: 'Integrations' },
    { icon: 'gear', label: 'Settings' },
  ];
  return (
    <div style={{
      width: 210, background: artisanStyles.bgSidebar,
      borderRight: `1px solid ${artisanStyles.line}`,
      display: 'flex', flexDirection: 'column',
      padding: '14px 10px', fontFamily: artisanStyles.sans,
    }}>
      <div style={{ padding: '4px 8px 12px' }}><ArtisanWordmark /></div>

      <div style={{
        display: 'flex', alignItems: 'center', gap: 8,
        padding: '7px 10px', margin: '2px 0 10px',
        border: `1px solid ${artisanStyles.line}`, borderRadius: 6,
        background: artisanStyles.bgElev,
        fontSize: 12, color: artisanStyles.inkMuted,
      }}>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <circle cx="11" cy="11" r="7"/><path d="m21 21-5-5"/>
        </svg>
        <span style={{ flex: 1 }}>Search…</span>
        <span style={{
          fontFamily: artisanStyles.mono, fontSize: 10,
          color: artisanStyles.inkMuted,
          background: artisanStyles.bgDeep, border: `1px solid ${artisanStyles.line}`,
          padding: '1px 5px', borderRadius: 3,
        }}>⌘K</span>
      </div>

      {items.map(it => (
        <div key={it.label} style={{
          display: 'flex', alignItems: 'center', gap: 9,
          padding: '6px 10px', borderRadius: 5, margin: '1px 0',
          background: it.active ? artisanStyles.bgElev : 'transparent',
          color: it.active ? artisanStyles.ink : artisanStyles.inkSoft,
          fontSize: 13, fontWeight: it.active ? 500 : 400,
          boxShadow: it.active ? `0 0 0 1px ${artisanStyles.line}, 0 1px 2px rgba(10,10,15,0.04)` : 'none',
        }}>
          <ArtisanIcon name={it.icon} size={13} color={it.active ? artisanStyles.accent : artisanStyles.inkMuted} />
          <span style={{ flex: 1 }}>{it.label}</span>
          {it.badge && (
            <span style={{
              fontSize: 10, color: artisanStyles.inkMuted,
              fontFamily: artisanStyles.mono,
            }}>{it.badge}</span>
          )}
        </div>
      ))}

      <div style={{ flex: 1 }}/>

      <div style={{
        padding: '8px 10px', borderTop: `1px solid ${artisanStyles.line}`,
        display: 'flex', alignItems: 'center', gap: 8,
      }}>
        <div style={{
          width: 22, height: 22, borderRadius: 99, background: artisanStyles.accent,
          color: '#fff', fontSize: 10, fontWeight: 600,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}>AR</div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: 11.5, color: artisanStyles.ink, fontWeight: 500 }}>Workspace</div>
          <div style={{
            fontSize: 10, color: artisanStyles.inkMuted, fontFamily: artisanStyles.mono,
            display: 'flex', alignItems: 'center', gap: 4,
          }}>
            <span style={{
              width: 5, height: 5, borderRadius: 99, background: artisanStyles.green,
            }}/>
            sonnet-4 · online
          </div>
        </div>
      </div>
    </div>
  );
}

function ArtisanIcon({ name, size = 14, color = 'currentColor' }) {
  const p = { width: size, height: size, viewBox: '0 0 24 24', fill: 'none', stroke: color, strokeWidth: 1.8, strokeLinecap: 'round', strokeLinejoin: 'round' };
  switch (name) {
    case 'chat': return <svg {...p}><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>;
    case 'home': return <svg {...p}><path d="m3 9 9-7 9 7v11a2 2 0 0 1-2 2h-4a2 2 0 0 1-2-2v-4h-2v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>;
    case 'chart': return <svg {...p}><path d="M3 3v18h18"/><path d="M18 17V9M13 17V5M8 17v-3"/></svg>;
    case 'list': return <svg {...p}><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/></svg>;
    case 'brain': return <svg {...p}><path d="M12 3a3 3 0 0 0-3 3v12a3 3 0 1 0 6 0V6a3 3 0 0 0-3-3"/><path d="M9 6a3 3 0 1 0-6 3 3 3 0 0 0 6 0"/><path d="M21 9a3 3 0 1 0-6-3 3 3 0 0 0 6 0"/></svg>;
    case 'wrench': return <svg {...p}><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>;
    case 'plug': return <svg {...p}><path d="M9 2v6M15 2v6M5 10h14l-1 12H6z"/></svg>;
    case 'gear': return <svg {...p}><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3h0a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5h0a1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8v0a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/></svg>;
    default: return null;
  }
}

function ArtisanToolCall({ tool, onOpen }) {
  const [expanded, setExpanded] = React.useState(false);
  const toolIcons = {
    'read_file': 'file', 'grep': 'search', 'git_log': 'git',
  };
  return (
    <div style={{ margin: '4px 0' }}>
      <div
        onClick={() => setExpanded(e => !e)}
        style={{
          display: 'flex', alignItems: 'center', gap: 10,
          padding: '5px 10px',
          background: artisanStyles.bgElev,
          border: `1px solid ${artisanStyles.line}`,
          borderRadius: 6, cursor: 'pointer',
          fontSize: 12,
        }}>
        <ArtisanCheckIcon status={tool.status} />
        <span style={{
          fontFamily: artisanStyles.mono, fontSize: 11.5,
          color: artisanStyles.ink, fontWeight: 500,
        }}>{tool.name}</span>
        <span style={{
          fontFamily: artisanStyles.mono, fontSize: 11,
          color: artisanStyles.inkMuted, flex: 1,
          whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
        }}>{tool.preview}</span>
        {tool.stats?.lines != null && (
          <span style={{
            fontSize: 10.5, color: artisanStyles.inkMuted, fontFamily: artisanStyles.mono,
          }}>{tool.stats.lines} lines</span>
        )}
        {tool.stats?.matches != null && (
          <span style={{
            fontSize: 10.5, color: artisanStyles.accent, fontFamily: artisanStyles.mono,
            background: artisanStyles.accentSoft, padding: '1px 6px', borderRadius: 99,
          }}>{tool.stats.matches}</span>
        )}
        <span style={{
          fontFamily: artisanStyles.mono, fontSize: 10.5, color: artisanStyles.inkFaint,
        }}>{tool.duration}</span>
      </div>
      {expanded && (
        <div style={{
          margin: '4px 0 4px 12px',
          borderLeft: `1px solid ${artisanStyles.line}`,
          paddingLeft: 12, paddingTop: 4,
        }}>
          <div style={{ fontSize: 10.5, color: artisanStyles.inkMuted, marginBottom: 4, textTransform: 'uppercase', letterSpacing: 0.5 }}>input</div>
          <pre style={{
            fontFamily: artisanStyles.mono, fontSize: 11, margin: 0, marginBottom: 10,
            color: artisanStyles.ink, background: artisanStyles.bgDeep,
            padding: 8, borderRadius: 4, whiteSpace: 'pre-wrap',
          }}>{JSON.stringify(tool.input, null, 2)}</pre>
          <div style={{ display: 'flex', alignItems: 'center', marginBottom: 4 }}>
            <div style={{ fontSize: 10.5, color: artisanStyles.inkMuted, textTransform: 'uppercase', letterSpacing: 0.5, flex: 1 }}>output</div>
            {onOpen && (
              <button onClick={(e) => { e.stopPropagation(); onOpen(); }} style={{
                fontSize: 10.5, color: artisanStyles.accent,
                background: 'none', border: 'none', cursor: 'pointer',
                fontFamily: artisanStyles.sans, fontWeight: 500,
              }}>Open →</button>
            )}
          </div>
          <pre style={{
            fontFamily: artisanStyles.mono, fontSize: 10.5, margin: 0,
            color: artisanStyles.inkSoft, background: artisanStyles.bgDeep,
            padding: 8, borderRadius: 4,
            whiteSpace: 'pre-wrap', maxHeight: 120, overflow: 'auto',
          }}>{tool.output}</pre>
        </div>
      )}
    </div>
  );
}

function ArtisanCheckIcon({ status }) {
  if (status === 'done') {
    return (
      <div style={{
        width: 13, height: 13, borderRadius: 99, flexShrink: 0,
        background: artisanStyles.green,
        display: 'flex', alignItems: 'center', justifyContent: 'center',
      }}>
        <svg width="8" height="8" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round">
          <polyline points="5 12 10 17 20 6"/>
        </svg>
      </div>
    );
  }
  return (
    <div style={{
      width: 11, height: 11, borderRadius: 99, border: `1.5px solid ${artisanStyles.accent}`,
      borderTopColor: 'transparent', animation: 'artisanSpin 0.9s linear infinite',
    }}/>
  );
}

function ArtisanReasoning({ text, duration }) {
  const [open, setOpen] = React.useState(false);
  return (
    <div
      onClick={() => setOpen(o => !o)}
      style={{
        margin: '4px 0 10px', padding: '5px 10px',
        background: artisanStyles.bgDeep, borderRadius: 5,
        fontSize: 11.5, color: artisanStyles.inkMuted,
        display: 'flex', alignItems: 'center', gap: 8,
        cursor: 'pointer', border: `1px solid ${artisanStyles.line}`,
      }}>
      <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
        <path d="M9 18h6M10 22h4M12 2a7 7 0 0 0-4 13c1 1 1 1 1 3h6c0-2 0-2 1-3a7 7 0 0 0-4-13z"/>
      </svg>
      <span style={{ flex: 1 }}>Thought for {duration}</span>
      <span style={{ fontSize: 10 }}>{open ? '−' : '+'}</span>
      {open && (
        <div style={{
          position: 'absolute', left: 24, right: 24, marginTop: 28,
          background: artisanStyles.bgElev, border: `1px solid ${artisanStyles.line}`,
          padding: 12, borderRadius: 6, whiteSpace: 'pre-wrap',
          lineHeight: 1.6, fontSize: 12, color: artisanStyles.inkSoft,
          boxShadow: '0 4px 16px rgba(10,10,15,0.06)',
          zIndex: 3,
        }}>{text}</div>
      )}
    </div>
  );
}

function ArtisanMd({ content }) {
  const parts = content.split(/```(\w+)?\n([\s\S]*?)```/g);
  const out = [];
  for (let i = 0; i < parts.length; i++) {
    if (i % 3 === 0) {
      const t = parts[i]; if (!t) continue;
      t.split('\n').forEach((line, idx) => {
        if (line.startsWith('## ')) out.push(<h3 key={`${i}-${idx}`} style={{
          fontSize: 15, fontWeight: 600, color: artisanStyles.ink, margin: '14px 0 6px', letterSpacing: -0.3,
        }}>{line.slice(3)}</h3>);
        else if (line.startsWith('### ')) out.push(<h4 key={`${i}-${idx}`} style={{
          fontSize: 12, fontWeight: 600, color: artisanStyles.ink, margin: '10px 0 4px',
          textTransform: 'uppercase', letterSpacing: 0.6,
        }}>{line.slice(4)}</h4>);
        else if (line.match(/^\d+\. /)) out.push(<div key={`${i}-${idx}`} style={{ margin: '3px 0' }}>{renderInline(line)}</div>);
        else if (line.trim() === '') out.push(<div key={`${i}-${idx}`} style={{ height: 5 }}/>);
        else out.push(<div key={`${i}-${idx}`} style={{ margin: '2px 0' }}>{renderInline(line)}</div>);
      });
    } else if (i % 3 === 2) {
      out.push(<ArtisanCodeBlock key={`c-${i}`} lang={parts[i-1]} code={parts[i].trim()} />);
    }
  }
  return <div style={{ fontFamily: artisanStyles.sans, fontSize: 13.5, color: artisanStyles.ink, lineHeight: 1.6 }}>{out}</div>;

  function renderInline(line) {
    const segs = line.split(/(\*\*[^*]+\*\*|`[^`]+`)/g);
    return segs.map((s, k) => {
      if (s.startsWith('**')) return <strong key={k}>{s.slice(2, -2)}</strong>;
      if (s.startsWith('`')) return <code key={k} style={{
        fontFamily: artisanStyles.mono, fontSize: 11.5,
        background: artisanStyles.bgDeep, padding: '1px 5px', borderRadius: 3,
        color: artisanStyles.accent,
      }}>{s.slice(1, -1)}</code>;
      return <span key={k}>{s}</span>;
    });
  }
}

function ArtisanCodeBlock({ lang, code }) {
  return (
    <div style={{
      margin: '10px 0', background: artisanStyles.bgElev,
      border: `1px solid ${artisanStyles.line}`, borderRadius: 6, overflow: 'hidden',
    }}>
      <div style={{
        padding: '5px 12px', borderBottom: `1px solid ${artisanStyles.line}`,
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        background: artisanStyles.bgDeep,
      }}>
        <span style={{ fontFamily: artisanStyles.mono, fontSize: 10.5, color: artisanStyles.inkMuted }}>{lang}</span>
        <span style={{ fontSize: 10.5, color: artisanStyles.inkMuted, cursor: 'pointer' }}>Copy</span>
      </div>
      <pre style={{
        margin: 0, padding: 12, fontFamily: artisanStyles.mono, fontSize: 11.5,
        color: artisanStyles.ink, lineHeight: 1.6, whiteSpace: 'pre', overflow: 'auto',
      }}>{code}</pre>
    </div>
  );
}

function ArtisanUserMsg({ msg }) {
  return (
    <div style={{ padding: '14px 28px 4px' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
        <div style={{
          width: 18, height: 18, borderRadius: 4, background: artisanStyles.ink,
          color: '#fff', fontSize: 9, fontWeight: 700,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}>AR</div>
        <span style={{ fontSize: 12, fontWeight: 600, color: artisanStyles.ink }}>You</span>
        <span style={{ fontSize: 11, color: artisanStyles.inkFaint, fontFamily: artisanStyles.mono }}>{msg.time}</span>
      </div>
      <div style={{
        fontSize: 13.5, color: artisanStyles.ink, lineHeight: 1.55, paddingLeft: 26,
      }}>{msg.content}</div>
    </div>
  );
}

function ArtisanAssistantMsg({ msg, onOpenArtifact }) {
  return (
    <div style={{ padding: '10px 28px 16px' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
        <div style={{
          width: 18, height: 18, borderRadius: 4,
          background: `linear-gradient(135deg, ${artisanStyles.ink}, ${artisanStyles.accent})`,
          color: '#fff', fontSize: 10, fontWeight: 700,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}>D</div>
        <span style={{ fontSize: 12, fontWeight: 600, color: artisanStyles.ink }}>Daimon</span>
        <span style={{ fontSize: 11, color: artisanStyles.inkFaint, fontFamily: artisanStyles.mono }}>{msg.time}</span>
      </div>
      <div style={{ paddingLeft: 26, position: 'relative' }}>
        {msg.reasoning && <ArtisanReasoning text={msg.reasoning} duration={msg.reasoningDuration} />}
        {msg.blocks.map((b, i) => {
          if (b.kind === 'tool') return <ArtisanToolCall key={i} tool={b} onOpen={b.name === 'read_file' ? () => onOpenArtifact('log') : null} />;
          if (b.kind === 'text') return <div key={i} style={{ margin: '8px 0' }}><ArtisanMd content={b.content} /></div>;
          return null;
        })}
      </div>
    </div>
  );
}

function ArtisanWorkspace({ artifact, onClose }) {
  if (!artifact) {
    return (
      <div style={{
        height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center',
        flexDirection: 'column', gap: 8, padding: 40,
        color: artisanStyles.inkFaint, fontFamily: artisanStyles.sans,
        fontSize: 12, textAlign: 'center',
      }}>
        <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke={artisanStyles.inkFaint} strokeWidth="1.2">
          <rect x="3" y="3" width="18" height="18" rx="2"/>
          <path d="M9 3v18M3 9h6"/>
        </svg>
        Workspace empty. Open an artifact from the chat.
      </div>
    );
  }
  if (artifact.type === 'code') {
    return (
      <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        <div style={{
          padding: '8px 14px', borderBottom: `1px solid ${artisanStyles.line}`,
          display: 'flex', alignItems: 'center', gap: 10, background: artisanStyles.bgElev,
        }}>
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke={artisanStyles.accent} strokeWidth="1.8">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
            <polyline points="14 2 14 8 20 8"/>
          </svg>
          <span style={{ fontFamily: artisanStyles.mono, fontSize: 12, color: artisanStyles.ink, flex: 1 }}>{artifact.title}</span>
          <button style={{
            fontSize: 11, padding: '3px 8px', background: artisanStyles.bgElev,
            border: `1px solid ${artisanStyles.line}`, borderRadius: 4,
            color: artisanStyles.inkSoft, cursor: 'pointer', fontFamily: artisanStyles.sans,
          }}>Copy</button>
          <button style={{
            fontSize: 11, padding: '3px 8px', background: artisanStyles.accent,
            border: 'none', borderRadius: 4,
            color: '#fff', cursor: 'pointer', fontFamily: artisanStyles.sans, fontWeight: 500,
          }}>Apply patch</button>
          <span onClick={onClose} style={{ fontSize: 16, color: artisanStyles.inkMuted, cursor: 'pointer', marginLeft: 4 }}>×</span>
        </div>
        <div style={{ display: 'flex', flex: 1, overflow: 'hidden', background: artisanStyles.bgElev }}>
          <div style={{
            padding: '12px 6px 12px 14px', fontFamily: artisanStyles.mono, fontSize: 11.5,
            color: artisanStyles.inkFaint, textAlign: 'right',
            userSelect: 'none', borderRight: `1px solid ${artisanStyles.line}`, lineHeight: 1.7,
          }}>
            {artifact.content.split('\n').map((_, i) => <div key={i}>{i + 1}</div>)}
          </div>
          <pre style={{
            margin: 0, padding: '12px 14px', flex: 1, overflow: 'auto',
            fontFamily: artisanStyles.mono, fontSize: 11.5, color: artisanStyles.ink,
            lineHeight: 1.7, whiteSpace: 'pre',
          }}>{artifact.content}</pre>
        </div>
      </div>
    );
  }
  if (artifact.type === 'log') {
    return (
      <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        <div style={{
          padding: '8px 14px', borderBottom: `1px solid ${artisanStyles.line}`,
          display: 'flex', alignItems: 'center', gap: 10, background: artisanStyles.bgElev,
        }}>
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke={artisanStyles.accent} strokeWidth="1.8">
            <line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/>
          </svg>
          <span style={{ fontFamily: artisanStyles.mono, fontSize: 12, color: artisanStyles.ink, flex: 1 }}>{artifact.title}</span>
          <span onClick={onClose} style={{ fontSize: 16, color: artisanStyles.inkMuted, cursor: 'pointer' }}>×</span>
        </div>
        <div style={{ flex: 1, overflow: 'auto', background: artisanStyles.bgElev }}>
          {artifact.entries.map((e, i) => (
            <div key={i} style={{
              display: 'flex', gap: 12, padding: '4px 14px',
              fontFamily: artisanStyles.mono, fontSize: 11,
              borderBottom: `1px solid ${artisanStyles.line}`,
            }}>
              <span style={{ color: artisanStyles.inkFaint, width: 60 }}>{e.t}</span>
              <span style={{
                width: 40, fontWeight: 600, fontSize: 10,
                color: e.lvl === 'ERROR' ? artisanStyles.red : e.lvl === 'WARN' ? artisanStyles.amber : artisanStyles.green,
              }}>{e.lvl}</span>
              <span style={{ color: artisanStyles.ink, flex: 1 }}>{e.msg}</span>
            </div>
          ))}
        </div>
      </div>
    );
  }
  return null;
}

function ArtisanInput({ showCmd }) {
  return (
    <div style={{ padding: 16, borderTop: `1px solid ${artisanStyles.line}`, background: artisanStyles.bg }}>
      <div style={{
        border: `1px solid ${artisanStyles.lineStrong}`,
        borderRadius: 8, background: artisanStyles.bgElev,
        padding: '10px 12px 8px',
        boxShadow: '0 1px 2px rgba(10,10,15,0.04)',
      }}>
        <div style={{
          fontSize: 13, color: artisanStyles.inkMuted,
          minHeight: 32, marginBottom: 6,
        }}>Ask anything, @mention a file, or / to use a tool…</div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11 }}>
          <button style={{
            background: 'none', border: `1px solid ${artisanStyles.line}`,
            padding: '3px 8px', borderRadius: 4, color: artisanStyles.inkSoft,
            cursor: 'pointer', fontFamily: artisanStyles.sans, fontSize: 11,
            display: 'flex', alignItems: 'center', gap: 4,
          }}>
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.48-8.48l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48"/></svg>
            Attach
          </button>
          <button style={{
            background: 'none', border: `1px solid ${artisanStyles.line}`,
            padding: '3px 8px', borderRadius: 4, color: artisanStyles.inkSoft,
            cursor: 'pointer', fontFamily: artisanStyles.sans, fontSize: 11,
          }}>/ Tools</button>
          <button style={{
            background: 'none', border: `1px solid ${artisanStyles.line}`,
            padding: '3px 8px', borderRadius: 4, color: artisanStyles.inkSoft,
            cursor: 'pointer', fontFamily: artisanStyles.sans, fontSize: 11,
          }}>@ Mention</button>
          <span style={{ flex: 1 }}/>
          <span onClick={showCmd} style={{
            fontFamily: artisanStyles.mono, fontSize: 10,
            border: `1px solid ${artisanStyles.line}`, padding: '2px 6px', borderRadius: 3,
            color: artisanStyles.inkMuted, cursor: 'pointer',
          }}>⌘K</span>
          <button style={{
            background: artisanStyles.accent, color: '#fff',
            fontSize: 12, fontWeight: 500, padding: '5px 12px', borderRadius: 5,
            border: 'none', cursor: 'pointer', fontFamily: artisanStyles.sans,
            display: 'flex', alignItems: 'center', gap: 6,
          }}>Send
            <span style={{
              fontFamily: artisanStyles.mono, fontSize: 10,
              background: 'rgba(255,255,255,0.2)', padding: '1px 5px', borderRadius: 3,
            }}>↵</span>
          </button>
        </div>
      </div>
    </div>
  );
}

function ArtisanCmdPalette({ onClose }) {
  return (
    <div onClick={onClose} style={{
      position: 'absolute', inset: 0, background: 'rgba(10,10,15,0.4)',
      display: 'flex', alignItems: 'flex-start', justifyContent: 'center',
      paddingTop: 100, zIndex: 50, backdropFilter: 'blur(4px)',
    }}>
      <div onClick={(e) => e.stopPropagation()} style={{
        width: 500, background: artisanStyles.bgElev,
        border: `1px solid ${artisanStyles.lineStrong}`,
        borderRadius: 10, overflow: 'hidden',
        boxShadow: '0 20px 60px rgba(10,10,15,0.25)',
      }}>
        <div style={{
          padding: '12px 16px', borderBottom: `1px solid ${artisanStyles.line}`,
          display: 'flex', alignItems: 'center', gap: 10,
        }}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke={artisanStyles.inkMuted} strokeWidth="2">
            <circle cx="11" cy="11" r="7"/><path d="m21 21-5-5"/>
          </svg>
          <span style={{ fontSize: 13.5, color: artisanStyles.inkMuted, flex: 1 }}>Type a command or search…</span>
          <span style={{
            fontFamily: artisanStyles.mono, fontSize: 10,
            color: artisanStyles.inkMuted, background: artisanStyles.bgDeep,
            padding: '2px 6px', borderRadius: 3, border: `1px solid ${artisanStyles.line}`,
          }}>esc</span>
        </div>
        <div style={{ padding: '6px 0' }}>
          <div style={{ padding: '4px 16px', fontSize: 10.5, color: artisanStyles.inkMuted, textTransform: 'uppercase', letterSpacing: 0.6, fontWeight: 500 }}>Actions</div>
          {[
            { k: 'new', l: 'New chat', s: '⌘ N', hi: true },
            { k: 'fork', l: 'Fork conversation', s: '⌘ ⇧ F' },
            { k: 'mem', l: 'Add to memory', s: '⌘ M' },
            { k: 'theme', l: 'Toggle theme', s: '⌘ .' },
          ].map(i => (
            <div key={i.l} style={{
              padding: '7px 16px', display: 'flex', alignItems: 'center', gap: 10,
              fontSize: 13, color: artisanStyles.ink,
              background: i.hi ? artisanStyles.accentSoft : 'transparent',
              cursor: 'pointer',
            }}>
              <div style={{
                width: 18, height: 18, borderRadius: 4,
                background: artisanStyles.bgDeep,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 10, color: artisanStyles.inkMuted,
              }}>{i.k[0].toUpperCase()}</div>
              <span style={{ flex: 1 }}>{i.l}</span>
              <span style={{ fontFamily: artisanStyles.mono, fontSize: 10.5, color: artisanStyles.inkMuted }}>{i.s}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function ArtisanChat({ showCmd, cmdOpen, closeCmd }) {
  const [artifact, setArtifact] = React.useState(WORKSPACE_ARTIFACTS.patch);
  return (
    <div style={{
      height: '100%', display: 'flex', flexDirection: 'row',
      background: artisanStyles.bg, color: artisanStyles.ink,
      fontFamily: artisanStyles.sans, position: 'relative',
    }}>
      <ArtisanSidebar />
      <div style={{ flex: 1.3, display: 'flex', flexDirection: 'column', minWidth: 0 }}>
        <div style={{
          padding: '10px 24px', borderBottom: `1px solid ${artisanStyles.line}`,
          display: 'flex', alignItems: 'center', gap: 12, background: artisanStyles.bgElev,
        }}>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: 13, fontWeight: 600, color: artisanStyles.ink }}>Payment service anomalies</div>
            <div style={{
              fontSize: 11, color: artisanStyles.inkMuted, marginTop: 1,
              display: 'flex', gap: 10, fontFamily: artisanStyles.mono,
            }}>
              <span>14:32</span>
              <span>·</span>
              <span>3 tools</span>
              <span>·</span>
              <span>$0.042</span>
            </div>
          </div>
          <div style={{
            fontSize: 11, color: artisanStyles.inkMuted,
            display: 'flex', alignItems: 'center', gap: 5,
            padding: '3px 8px', border: `1px solid ${artisanStyles.line}`, borderRadius: 99,
          }}>
            <span style={{ width: 6, height: 6, borderRadius: 99, background: artisanStyles.green, animation: 'artisanPulse 2s infinite' }}/>
            Live
          </div>
        </div>
        <div style={{ flex: 1, overflow: 'auto' }}>
          {MOCK_CONVO.map(m => m.role === 'user'
            ? <ArtisanUserMsg key={m.id} msg={m} />
            : <ArtisanAssistantMsg key={m.id} msg={m} onOpenArtifact={(k) => setArtifact(WORKSPACE_ARTIFACTS[k])} />)}
        </div>
        <ArtisanInput showCmd={showCmd} />
      </div>
      <div style={{ flex: 1, borderLeft: `1px solid ${artisanStyles.line}`, minWidth: 0 }}>
        <ArtisanWorkspace artifact={artifact} onClose={() => setArtifact(null)} />
      </div>
      {cmdOpen && <ArtisanCmdPalette onClose={closeCmd} />}
      <style>{`
        @keyframes artisanSpin { to { transform: rotate(360deg); } }
        @keyframes artisanPulse { 0%,100% { opacity: 1; } 50% { opacity: 0.4; } }
      `}</style>
    </div>
  );
}

Object.assign(window, { ArtisanChat, ArtisanWordmark, artisanStyles });
