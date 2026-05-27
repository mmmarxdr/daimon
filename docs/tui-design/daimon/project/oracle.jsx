// Direction A — "Oracle" / Mystic editorial
// Warm sepia + ink. Serif display for moments, sans for UI, mono for code.
// Accent: muted copper/amber. Wordmark: geometric D with radial mark.

const oracleStyles = {
  // Palette
  bg: '#f4efe6',              // warm paper
  bgElev: '#faf6ee',
  bgDeep: '#ebe4d5',
  ink: '#1c1814',             // near-black warm
  inkSoft: '#4a4136',
  inkMuted: '#8a7f6f',
  inkFaint: '#b8ad9a',
  line: 'rgba(28,24,20,0.08)',
  lineStrong: 'rgba(28,24,20,0.14)',
  accent: '#b85c2e',          // copper
  accentSoft: 'rgba(184,92,46,0.10)',
  sage: '#6b7a5a',            // success
  crimson: '#a23a2e',
  // Type
  display: '"Fraunces", Georgia, serif',
  sans: '"Inter", -apple-system, system-ui, sans-serif',
  mono: '"JetBrains Mono", ui-monospace, monospace',
};

function OracleWordmark({ size = 18 }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <svg width={size} height={size} viewBox="0 0 24 24" fill="none">
        <circle cx="12" cy="12" r="10.5" stroke={oracleStyles.ink} strokeWidth="1"/>
        <path d="M 7.5 7 L 7.5 17 L 12 17 C 15 17 16.5 14.8 16.5 12 C 16.5 9.2 15 7 12 7 Z"
              fill={oracleStyles.ink}/>
        <circle cx="18.5" cy="5.5" r="1.2" fill={oracleStyles.accent}/>
      </svg>
      <span style={{
        fontFamily: oracleStyles.display,
        fontSize: size * 0.95,
        fontWeight: 500,
        letterSpacing: -0.3,
        color: oracleStyles.ink,
        fontStyle: 'italic',
      }}>Daimon</span>
    </div>
  );
}

function OracleSidebar() {
  const items = [
    { icon: '◐', label: 'Chat', active: true },
    { icon: '◇', label: 'Overview' },
    { icon: '≡', label: 'Conversations' },
    { icon: '✦', label: 'Memory' },
    { icon: '⚒', label: 'Tools' },
    { icon: '⌘', label: 'Integrations' },
    { icon: '∷', label: 'Logs' },
    { icon: '○', label: 'Settings' },
  ];
  return (
    <div style={{
      width: 196,
      background: oracleStyles.bgDeep,
      borderRight: `1px solid ${oracleStyles.line}`,
      display: 'flex', flexDirection: 'column',
      padding: '18px 12px',
      fontFamily: oracleStyles.sans,
    }}>
      <div style={{ padding: '4px 8px 20px' }}><OracleWordmark /></div>

      <div style={{
        fontFamily: oracleStyles.display, fontSize: 11, fontStyle: 'italic',
        color: oracleStyles.inkMuted, padding: '0 8px 8px',
        letterSpacing: 0.2,
      }}>Invocation</div>

      {items.slice(0, 1).map(it => (
        <div key={it.label} style={{
          display: 'flex', alignItems: 'center', gap: 10,
          padding: '7px 9px', borderRadius: 4,
          background: it.active ? oracleStyles.bgElev : 'transparent',
          color: it.active ? oracleStyles.ink : oracleStyles.inkSoft,
          fontSize: 13, fontWeight: it.active ? 500 : 400,
          boxShadow: it.active ? `inset 2px 0 0 ${oracleStyles.accent}` : 'none',
        }}>
          <span style={{ width: 14, color: it.active ? oracleStyles.accent : oracleStyles.inkMuted, fontSize: 12 }}>{it.icon}</span>
          <span>{it.label}</span>
        </div>
      ))}

      <div style={{
        fontFamily: oracleStyles.display, fontSize: 11, fontStyle: 'italic',
        color: oracleStyles.inkMuted, padding: '14px 8px 8px',
        letterSpacing: 0.2,
      }}>Observatory</div>

      {items.slice(1).map(it => (
        <div key={it.label} style={{
          display: 'flex', alignItems: 'center', gap: 10,
          padding: '7px 9px', borderRadius: 4,
          color: oracleStyles.inkSoft,
          fontSize: 13,
        }}>
          <span style={{ width: 14, color: oracleStyles.inkMuted, fontSize: 12 }}>{it.icon}</span>
          <span>{it.label}</span>
        </div>
      ))}

      <div style={{ flex: 1 }}/>
      <div style={{
        fontSize: 11, color: oracleStyles.inkMuted, padding: '10px 9px',
        display: 'flex', alignItems: 'center', gap: 8,
        borderTop: `1px solid ${oracleStyles.line}`,
      }}>
        <span style={{
          width: 6, height: 6, borderRadius: 99,
          background: oracleStyles.sage,
          boxShadow: `0 0 0 3px ${oracleStyles.sage}22`,
        }}/>
        <span style={{ fontFamily: oracleStyles.mono, fontSize: 10 }}>claude-sonnet-4</span>
      </div>
    </div>
  );
}

function OracleToolCard({ tool, onOpen }) {
  const [expanded, setExpanded] = React.useState(false);
  return (
    <div style={{
      margin: '6px 0',
      border: `1px solid ${oracleStyles.line}`,
      background: oracleStyles.bgElev,
      borderRadius: 6,
      overflow: 'hidden',
      fontFamily: oracleStyles.sans,
    }}>
      <div
        onClick={() => setExpanded(e => !e)}
        style={{
          display: 'flex', alignItems: 'center', gap: 10,
          padding: '8px 12px',
          fontSize: 12,
          cursor: 'pointer',
        }}>
        <span style={{
          color: tool.status === 'done' ? oracleStyles.sage : oracleStyles.accent,
          fontSize: 11, width: 12,
        }}>{tool.status === 'done' ? '●' : '◐'}</span>
        <span style={{
          fontFamily: oracleStyles.mono, fontSize: 11.5,
          color: oracleStyles.ink, fontWeight: 500,
        }}>{tool.name}</span>
        <span style={{
          fontFamily: oracleStyles.mono, fontSize: 11,
          color: oracleStyles.inkMuted, flex: 1,
          whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
        }}>{tool.preview}</span>
        {tool.stats?.matches != null && (
          <span style={{
            fontSize: 10.5, color: oracleStyles.inkMuted,
            fontFamily: oracleStyles.mono,
            background: oracleStyles.bgDeep, padding: '1px 6px', borderRadius: 99,
          }}>{tool.stats.matches} matches</span>
        )}
        <span style={{
          fontFamily: oracleStyles.mono, fontSize: 10.5,
          color: oracleStyles.inkFaint,
        }}>{tool.duration}</span>
      </div>
      {expanded && (
        <div style={{
          borderTop: `1px solid ${oracleStyles.line}`,
          padding: '10px 14px',
          background: oracleStyles.bg,
        }}>
          <div style={{ fontSize: 10.5, color: oracleStyles.inkMuted, marginBottom: 4, fontStyle: 'italic' }}>input</div>
          <pre style={{
            fontFamily: oracleStyles.mono, fontSize: 11,
            color: oracleStyles.ink, margin: 0, marginBottom: 10,
            whiteSpace: 'pre-wrap',
          }}>{JSON.stringify(tool.input, null, 2)}</pre>
          <div style={{
            display: 'flex', justifyContent: 'space-between', alignItems: 'center',
            marginBottom: 4,
          }}>
            <div style={{ fontSize: 10.5, color: oracleStyles.inkMuted, fontStyle: 'italic' }}>output</div>
            {onOpen && (
              <button onClick={(e) => { e.stopPropagation(); onOpen(); }} style={{
                fontSize: 10.5, color: oracleStyles.accent,
                background: 'none', border: 'none', cursor: 'pointer',
                fontFamily: oracleStyles.sans, fontStyle: 'italic',
              }}>open in workspace →</button>
            )}
          </div>
          <pre style={{
            fontFamily: oracleStyles.mono, fontSize: 10.5,
            color: oracleStyles.inkSoft, margin: 0,
            background: oracleStyles.bgDeep, padding: 8, borderRadius: 3,
            maxHeight: 120, overflow: 'auto',
            whiteSpace: 'pre-wrap',
          }}>{tool.output}</pre>
        </div>
      )}
    </div>
  );
}

function OracleReasoning({ text, duration }) {
  const [open, setOpen] = React.useState(false);
  return (
    <div style={{
      margin: '10px 0 14px',
      borderLeft: `1px solid ${oracleStyles.lineStrong}`,
      paddingLeft: 14,
    }}>
      <div
        onClick={() => setOpen(o => !o)}
        style={{
          fontFamily: oracleStyles.display, fontStyle: 'italic',
          fontSize: 13, color: oracleStyles.inkMuted,
          cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 8,
        }}>
        <span style={{ fontSize: 10 }}>{open ? '▼' : '▸'}</span>
        Contemplated for {duration}
      </div>
      {open && (
        <div style={{
          marginTop: 8,
          fontFamily: oracleStyles.sans, fontSize: 12,
          color: oracleStyles.inkSoft, lineHeight: 1.65,
          whiteSpace: 'pre-wrap',
        }}>{text}</div>
      )}
    </div>
  );
}

function OracleMessageText({ children }) {
  return (
    <div style={{
      fontFamily: oracleStyles.sans, fontSize: 14,
      color: oracleStyles.ink, lineHeight: 1.65,
    }}>{children}</div>
  );
}

// Very tiny markdown renderer for mock
function OracleMd({ content }) {
  const parts = content.split(/```(\w+)?\n([\s\S]*?)```/g);
  const out = [];
  for (let i = 0; i < parts.length; i++) {
    if (i % 3 === 0) {
      // text
      const t = parts[i];
      if (!t) continue;
      t.split('\n').forEach((line, idx) => {
        if (line.startsWith('## ')) {
          out.push(<h3 key={`${i}-${idx}`} style={{
            fontFamily: oracleStyles.display, fontSize: 17, fontWeight: 500,
            color: oracleStyles.ink, margin: '14px 0 6px', letterSpacing: -0.3,
          }}>{line.slice(3)}</h3>);
        } else if (line.startsWith('### ')) {
          out.push(<h4 key={`${i}-${idx}`} style={{
            fontFamily: oracleStyles.sans, fontSize: 13, fontWeight: 600,
            color: oracleStyles.ink, margin: '10px 0 4px',
            textTransform: 'uppercase', letterSpacing: 0.5,
          }}>{line.slice(4)}</h4>);
        } else if (line.match(/^\d+\. /)) {
          out.push(<div key={`${i}-${idx}`} style={{ margin: '3px 0', paddingLeft: 4 }}>{renderInline(line)}</div>);
        } else if (line.trim() === '') {
          out.push(<div key={`${i}-${idx}`} style={{ height: 6 }}/>);
        } else {
          out.push(<div key={`${i}-${idx}`} style={{ margin: '2px 0' }}>{renderInline(line)}</div>);
        }
      });
    } else if (i % 3 === 2) {
      // code
      out.push(<OracleCodeBlock key={`c-${i}`} lang={parts[i-1]} code={parts[i].trim()} />);
    }
  }
  return <OracleMessageText>{out}</OracleMessageText>;

  function renderInline(line) {
    const segs = line.split(/(\*\*[^*]+\*\*|`[^`]+`)/g);
    return segs.map((s, k) => {
      if (s.startsWith('**')) return <strong key={k} style={{ color: oracleStyles.ink }}>{s.slice(2, -2)}</strong>;
      if (s.startsWith('`')) return <code key={k} style={{
        fontFamily: oracleStyles.mono, fontSize: 12,
        background: oracleStyles.bgDeep, padding: '1px 5px', borderRadius: 3,
        color: oracleStyles.accent,
      }}>{s.slice(1, -1)}</code>;
      return <span key={k}>{s}</span>;
    });
  }
}

function OracleCodeBlock({ lang, code }) {
  return (
    <div style={{
      margin: '10px 0',
      background: '#211d18',
      border: `1px solid ${oracleStyles.lineStrong}`,
      borderRadius: 4,
      overflow: 'hidden',
    }}>
      <div style={{
        padding: '6px 12px',
        borderBottom: '1px solid rgba(255,255,255,0.05)',
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
      }}>
        <span style={{
          fontFamily: oracleStyles.mono, fontSize: 10.5,
          color: '#8a7f6f', letterSpacing: 0.3,
        }}>{lang}</span>
        <span style={{ fontSize: 10, color: '#6a5f4f', fontFamily: oracleStyles.sans, fontStyle: 'italic', cursor: 'pointer' }}>copy</span>
      </div>
      <pre style={{
        margin: 0, padding: '10px 14px',
        fontFamily: oracleStyles.mono, fontSize: 11.5,
        color: '#e8dcc7', lineHeight: 1.55,
        whiteSpace: 'pre', overflow: 'auto',
      }}>{code}</pre>
    </div>
  );
}

function OracleUserMsg({ msg }) {
  return (
    <div style={{
      display: 'flex', gap: 12, padding: '14px 24px',
      background: oracleStyles.bgElev,
      borderBottom: `1px solid ${oracleStyles.line}`,
    }}>
      <div style={{
        width: 24, height: 24, borderRadius: 99,
        background: oracleStyles.accent,
        color: '#fff', fontSize: 11, fontWeight: 600,
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        fontFamily: oracleStyles.sans, flexShrink: 0, marginTop: 1,
      }}>A</div>
      <div style={{
        fontFamily: oracleStyles.sans, fontSize: 14,
        color: oracleStyles.ink, lineHeight: 1.55, flex: 1,
      }}>{msg.content}</div>
      <div style={{ fontFamily: oracleStyles.mono, fontSize: 10.5, color: oracleStyles.inkFaint }}>{msg.time}</div>
    </div>
  );
}

function OracleAssistantMsg({ msg, onOpenArtifact }) {
  return (
    <div style={{ padding: '16px 24px 10px', borderBottom: `1px solid ${oracleStyles.line}` }}>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10, marginBottom: 8,
      }}>
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none">
          <circle cx="12" cy="12" r="10.5" stroke={oracleStyles.ink} strokeWidth="1"/>
          <path d="M 7.5 7 L 7.5 17 L 12 17 C 15 17 16.5 14.8 16.5 12 C 16.5 9.2 15 7 12 7 Z"
                fill={oracleStyles.ink}/>
        </svg>
        <span style={{
          fontFamily: oracleStyles.display, fontSize: 13, fontStyle: 'italic',
          color: oracleStyles.inkMuted,
        }}>Daimon answers</span>
        <span style={{ flex: 1 }}/>
        <span style={{ fontFamily: oracleStyles.mono, fontSize: 10.5, color: oracleStyles.inkFaint }}>{msg.time}</span>
      </div>

      {msg.reasoning && <OracleReasoning text={msg.reasoning} duration={msg.reasoningDuration} />}

      {msg.blocks.map((b, i) => {
        if (b.kind === 'tool') return <OracleToolCard key={i} tool={b} onOpen={b.name === 'read_file' ? () => onOpenArtifact('log') : null} />;
        if (b.kind === 'text') return <OracleMd key={i} content={b.content} />;
        return null;
      })}
    </div>
  );
}

function OracleWorkspace({ artifact, onClose }) {
  if (!artifact) {
    return (
      <div style={{
        height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center',
        flexDirection: 'column', gap: 10, padding: 40,
        color: oracleStyles.inkFaint, fontFamily: oracleStyles.display, fontStyle: 'italic',
        fontSize: 14, textAlign: 'center',
      }}>
        <svg width="42" height="42" viewBox="0 0 24 24" fill="none">
          <circle cx="12" cy="12" r="10" stroke={oracleStyles.inkFaint} strokeWidth="0.8"/>
          <circle cx="12" cy="12" r="4" stroke={oracleStyles.inkFaint} strokeWidth="0.8"/>
          <circle cx="12" cy="12" r="0.8" fill={oracleStyles.inkFaint}/>
        </svg>
        <div>The workspace awaits.<br/>Open an artifact from the conversation.</div>
      </div>
    );
  }

  if (artifact.type === 'code') {
    return (
      <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        <div style={{
          padding: '10px 16px', borderBottom: `1px solid ${oracleStyles.line}`,
          display: 'flex', alignItems: 'center', gap: 10,
          background: oracleStyles.bgElev,
        }}>
          <span style={{ color: oracleStyles.accent, fontSize: 12 }}>◆</span>
          <span style={{
            fontFamily: oracleStyles.mono, fontSize: 12, color: oracleStyles.ink,
            flex: 1,
          }}>{artifact.title}</span>
          <span style={{ fontFamily: oracleStyles.sans, fontSize: 11, color: oracleStyles.inkMuted, fontStyle: 'italic', cursor: 'pointer' }}>copy</span>
          <span style={{ fontFamily: oracleStyles.sans, fontSize: 11, color: oracleStyles.inkMuted, fontStyle: 'italic', cursor: 'pointer' }}>apply</span>
          <span onClick={onClose} style={{ fontSize: 14, color: oracleStyles.inkMuted, cursor: 'pointer' }}>×</span>
        </div>
        <pre style={{
          margin: 0, padding: 18, flex: 1, overflow: 'auto',
          fontFamily: oracleStyles.mono, fontSize: 12,
          color: oracleStyles.ink, background: oracleStyles.bg,
          lineHeight: 1.65,
        }}>{artifact.content}</pre>
      </div>
    );
  }
  if (artifact.type === 'log') {
    return (
      <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        <div style={{
          padding: '10px 16px', borderBottom: `1px solid ${oracleStyles.line}`,
          display: 'flex', alignItems: 'center', gap: 10, background: oracleStyles.bgElev,
        }}>
          <span style={{ color: oracleStyles.accent, fontSize: 12 }}>∷</span>
          <span style={{ fontFamily: oracleStyles.mono, fontSize: 12, color: oracleStyles.ink, flex: 1 }}>{artifact.title}</span>
          <span onClick={onClose} style={{ fontSize: 14, color: oracleStyles.inkMuted, cursor: 'pointer' }}>×</span>
        </div>
        <div style={{ flex: 1, overflow: 'auto', background: oracleStyles.bg }}>
          {artifact.entries.map((e, i) => (
            <div key={i} style={{
              display: 'flex', gap: 12, padding: '4px 14px',
              fontFamily: oracleStyles.mono, fontSize: 11,
              borderBottom: `1px solid ${oracleStyles.line}`,
              background: i % 2 === 0 ? 'transparent' : oracleStyles.bgDeep + '55',
            }}>
              <span style={{ color: oracleStyles.inkMuted, width: 60 }}>{e.t}</span>
              <span style={{
                width: 42, fontWeight: 600,
                color: e.lvl === 'ERROR' ? oracleStyles.crimson :
                       e.lvl === 'WARN' ? oracleStyles.accent : oracleStyles.sage,
              }}>{e.lvl}</span>
              <span style={{ color: oracleStyles.ink, flex: 1 }}>{e.msg}</span>
            </div>
          ))}
        </div>
      </div>
    );
  }
  return null;
}

function OracleInput({ showCmd }) {
  return (
    <div style={{
      padding: 18, borderTop: `1px solid ${oracleStyles.line}`,
      background: oracleStyles.bgElev,
    }}>
      <div style={{
        border: `1px solid ${oracleStyles.lineStrong}`,
        borderRadius: 6,
        background: oracleStyles.bg,
        padding: '10px 14px',
        display: 'flex', flexDirection: 'column', gap: 10,
      }}>
        <div style={{
          fontFamily: oracleStyles.sans, fontSize: 13,
          color: oracleStyles.inkMuted,
          minHeight: 40, display: 'flex', alignItems: 'center',
        }}>
          <span style={{
            fontFamily: oracleStyles.display, fontStyle: 'italic',
          }}>Speak your question…</span>
        </div>
        <div style={{
          display: 'flex', alignItems: 'center', gap: 8,
          fontSize: 11, color: oracleStyles.inkMuted, fontFamily: oracleStyles.sans,
        }}>
          <span style={{ cursor: 'pointer' }}>+ attach</span>
          <span style={{ cursor: 'pointer' }}>⌘ tools</span>
          <span style={{ flex: 1 }}/>
          <span onClick={showCmd} style={{
            fontFamily: oracleStyles.mono, fontSize: 10,
            border: `1px solid ${oracleStyles.line}`, padding: '2px 6px', borderRadius: 3,
            cursor: 'pointer',
          }}>⌘K</span>
          <button style={{
            background: oracleStyles.ink, color: oracleStyles.bg,
            fontFamily: oracleStyles.display, fontStyle: 'italic',
            fontSize: 12, padding: '5px 14px', borderRadius: 3,
            border: 'none', cursor: 'pointer',
          }}>Invoke ↵</button>
        </div>
      </div>
    </div>
  );
}

function OracleCmdPalette({ onClose }) {
  return (
    <div onClick={onClose} style={{
      position: 'absolute', inset: 0, background: 'rgba(28,24,20,0.3)',
      display: 'flex', alignItems: 'flex-start', justifyContent: 'center',
      paddingTop: 100, zIndex: 50, backdropFilter: 'blur(2px)',
    }}>
      <div onClick={(e) => e.stopPropagation()} style={{
        width: 480, background: oracleStyles.bgElev,
        border: `1px solid ${oracleStyles.lineStrong}`,
        borderRadius: 8, overflow: 'hidden',
        boxShadow: '0 20px 60px rgba(28,24,20,0.25)',
      }}>
        <div style={{
          padding: '14px 18px', borderBottom: `1px solid ${oracleStyles.line}`,
          fontFamily: oracleStyles.display, fontStyle: 'italic',
          fontSize: 15, color: oracleStyles.inkMuted,
        }}>What would you like to do?</div>
        {[
          { k: '→', l: 'New chat', s: '⌘ N' },
          { k: '✦', l: 'Summon memory', s: '⌘ M' },
          { k: '⚒', l: 'Manage tools', s: '⌘ T' },
          { k: '○', l: 'Toggle theme', s: '⌘ .' },
        ].map(i => (
          <div key={i.l} style={{
            padding: '10px 18px', display: 'flex', alignItems: 'center', gap: 12,
            fontSize: 13, color: oracleStyles.ink, fontFamily: oracleStyles.sans,
            cursor: 'pointer',
            background: i.k === '→' ? oracleStyles.accentSoft : 'transparent',
          }}>
            <span style={{ color: oracleStyles.accent, width: 14 }}>{i.k}</span>
            <span style={{ flex: 1 }}>{i.l}</span>
            <span style={{ fontFamily: oracleStyles.mono, fontSize: 10.5, color: oracleStyles.inkMuted }}>{i.s}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function OracleChat({ showCmd, cmdOpen, closeCmd }) {
  const [artifact, setArtifact] = React.useState(WORKSPACE_ARTIFACTS.patch);
  return (
    <div style={{
      height: '100%', display: 'flex', flexDirection: 'row',
      background: oracleStyles.bg, color: oracleStyles.ink,
      fontFamily: oracleStyles.sans, position: 'relative',
    }}>
      <OracleSidebar />

      {/* Main chat column */}
      <div style={{ flex: 1.3, display: 'flex', flexDirection: 'column', minWidth: 0 }}>
        <div style={{
          padding: '12px 24px', borderBottom: `1px solid ${oracleStyles.line}`,
          display: 'flex', alignItems: 'center', gap: 12, background: oracleStyles.bgElev,
        }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{
              fontFamily: oracleStyles.display, fontSize: 16, fontStyle: 'italic',
              color: oracleStyles.ink,
              whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
            }}>Payment service anomalies</div>
            <div style={{ fontSize: 11, color: oracleStyles.inkMuted, marginTop: 2 }}>
              started 14:32 · 3 tool calls · $0.042
            </div>
          </div>
          <div style={{
            fontSize: 11, fontFamily: oracleStyles.mono, color: oracleStyles.inkMuted,
            display: 'flex', alignItems: 'center', gap: 6,
          }}>
            <span style={{ width: 6, height: 6, borderRadius: 99, background: oracleStyles.sage, animation: 'oraclePulse 2s infinite' }}/>
            connected
          </div>
        </div>

        <div style={{ flex: 1, overflow: 'auto' }}>
          {MOCK_CONVO.map(m => m.role === 'user'
            ? <OracleUserMsg key={m.id} msg={m} />
            : <OracleAssistantMsg key={m.id} msg={m} onOpenArtifact={(k) => setArtifact(WORKSPACE_ARTIFACTS[k])} />)}
        </div>

        <OracleInput showCmd={showCmd} />
      </div>

      {/* Workspace */}
      <div style={{
        flex: 1, borderLeft: `1px solid ${oracleStyles.line}`,
        background: oracleStyles.bgElev, minWidth: 0,
      }}>
        <OracleWorkspace artifact={artifact} onClose={() => setArtifact(null)} />
      </div>

      {cmdOpen && <OracleCmdPalette onClose={closeCmd} />}

      <style>{`
        @keyframes oraclePulse {
          0%,100% { opacity: 1; transform: scale(1); }
          50% { opacity: 0.5; transform: scale(0.85); }
        }
      `}</style>
    </div>
  );
}

Object.assign(window, { OracleChat, OracleWordmark, oracleStyles });
