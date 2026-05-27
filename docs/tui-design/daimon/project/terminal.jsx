// Direction C — "Terminal Elevated"
// High-craft monospace, CRT warmth, dark-first, neon accents but restrained.
// Evokes a terminal but feels premium.

const termStyles = {
  bg: '#0d0e12',
  bgElev: '#14161c',
  bgDeep: '#080910',
  bgSidebar: '#0a0b10',
  ink: '#e8e6dd',
  inkSoft: '#a8a69a',
  inkMuted: '#6a685d',
  inkFaint: '#3a382f',
  line: '#1f2028',
  lineStrong: '#2c2e38',
  accent: '#7dd3c0',         // phosphor teal
  accentSoft: 'rgba(125,211,192,0.08)',
  accentStrong: '#a9e6d4',
  warn: '#e6b858',
  error: '#e87870',
  mono: '"JetBrains Mono", "SF Mono", ui-monospace, monospace',
  sans: '"Inter", -apple-system, system-ui, sans-serif',
};

function TermWordmark({ size = 18 }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontFamily: termStyles.mono }}>
      <div style={{
        width: size + 2, height: size + 2, position: 'relative',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        border: `1px solid ${termStyles.accent}`, borderRadius: 3,
        boxShadow: `0 0 8px ${termStyles.accent}33, inset 0 0 8px ${termStyles.accent}22`,
      }}>
        <span style={{
          color: termStyles.accent, fontSize: size * 0.58,
          fontFamily: termStyles.mono, fontWeight: 700,
          textShadow: `0 0 6px ${termStyles.accent}`,
        }}>δ</span>
      </div>
      <span style={{
        fontFamily: termStyles.mono, fontSize: size * 0.78, fontWeight: 500,
        color: termStyles.ink, letterSpacing: 1,
        textTransform: 'uppercase',
      }}>daimon</span>
      <span style={{
        width: 6, height: 10, background: termStyles.accent,
        animation: 'termCaret 1s step-end infinite', marginLeft: -4,
      }}/>
    </div>
  );
}

function TermSidebar() {
  const items = [
    { code: '01', label: 'chat', active: true },
    { code: '02', label: 'overview' },
    { code: '03', label: 'metrics' },
    { code: '04', label: 'convos' },
    { code: '05', label: 'memory' },
    { code: '06', label: 'tools' },
    { code: '07', label: 'mcp' },
    { code: '08', label: 'logs' },
    { code: '09', label: 'settings' },
  ];
  return (
    <div style={{
      width: 200, background: termStyles.bgSidebar,
      borderRight: `1px solid ${termStyles.line}`,
      display: 'flex', flexDirection: 'column',
      padding: '16px 10px', fontFamily: termStyles.mono,
    }}>
      <div style={{ padding: '2px 6px 18px' }}><TermWordmark /></div>

      <div style={{
        fontSize: 10, color: termStyles.inkMuted, padding: '0 8px 6px',
        textTransform: 'uppercase', letterSpacing: 1.5,
      }}>── nav</div>

      {items.map(it => (
        <div key={it.label} style={{
          display: 'flex', alignItems: 'center', gap: 8,
          padding: '5px 8px', margin: '1px 0',
          color: it.active ? termStyles.accent : termStyles.inkSoft,
          fontSize: 12,
          background: it.active ? termStyles.accentSoft : 'transparent',
          borderLeft: it.active ? `2px solid ${termStyles.accent}` : `2px solid transparent`,
          paddingLeft: it.active ? 6 : 8,
        }}>
          <span style={{ color: termStyles.inkFaint, fontSize: 10 }}>{it.code}</span>
          <span style={{ flex: 1 }}>{it.label}</span>
          {it.active && <span style={{ color: termStyles.accent }}>▸</span>}
        </div>
      ))}

      <div style={{ flex: 1 }}/>
      <div style={{
        padding: '10px 8px', borderTop: `1px solid ${termStyles.line}`,
        fontSize: 10, color: termStyles.inkMuted, lineHeight: 1.7,
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>model</span><span style={{ color: termStyles.ink }}>sonnet-4</span>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>ctx</span><span style={{ color: termStyles.ink }}>12.4k/200k</span>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>status</span>
          <span style={{ color: termStyles.accent }}>● ready</span>
        </div>
      </div>
    </div>
  );
}

function TermToolCall({ tool, onOpen }) {
  const [expanded, setExpanded] = React.useState(false);
  const glyph = tool.status === 'done' ? '✓' : '◐';
  return (
    <div style={{ margin: '3px 0', fontFamily: termStyles.mono }}>
      <div onClick={() => setExpanded(e => !e)} style={{
        display: 'flex', alignItems: 'center', gap: 10,
        padding: '4px 0',
        fontSize: 12, cursor: 'pointer',
      }}>
        <span style={{
          color: tool.status === 'done' ? termStyles.accent : termStyles.warn,
          width: 12, fontSize: 11,
          textShadow: tool.status === 'done' ? `0 0 4px ${termStyles.accent}` : 'none',
        }}>{glyph}</span>
        <span style={{ color: termStyles.ink, fontWeight: 500 }}>{tool.name}</span>
        <span style={{ color: termStyles.inkMuted }}>(</span>
        <span style={{
          color: termStyles.inkSoft, flex: 1,
          whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
        }}>{tool.preview}</span>
        <span style={{ color: termStyles.inkMuted }}>)</span>
        {tool.stats?.matches != null && (
          <span style={{ fontSize: 10.5, color: termStyles.accent }}>→ {tool.stats.matches}</span>
        )}
        <span style={{ color: termStyles.inkFaint, fontSize: 10.5 }}>{tool.duration}</span>
      </div>
      {expanded && (
        <div style={{
          marginLeft: 22, paddingLeft: 10, marginTop: 4,
          borderLeft: `1px solid ${termStyles.line}`, paddingBottom: 6,
        }}>
          <div style={{ fontSize: 10, color: termStyles.inkMuted, marginBottom: 3 }}>{'>'} input</div>
          <pre style={{
            margin: 0, marginBottom: 8,
            fontFamily: termStyles.mono, fontSize: 11,
            color: termStyles.inkSoft, whiteSpace: 'pre-wrap',
          }}>{JSON.stringify(tool.input, null, 2)}</pre>
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <div style={{ fontSize: 10, color: termStyles.inkMuted, marginBottom: 3, flex: 1 }}>{'>'} output</div>
            {onOpen && (
              <button onClick={(e) => { e.stopPropagation(); onOpen(); }} style={{
                fontSize: 10.5, color: termStyles.accent,
                background: 'none', border: 'none', cursor: 'pointer',
                fontFamily: termStyles.mono,
              }}>[open →]</button>
            )}
          </div>
          <pre style={{
            margin: 0, fontFamily: termStyles.mono, fontSize: 10.5,
            color: termStyles.inkSoft,
            background: termStyles.bgDeep, padding: 8, borderRadius: 2,
            border: `1px solid ${termStyles.line}`,
            whiteSpace: 'pre-wrap', maxHeight: 120, overflow: 'auto',
          }}>{tool.output}</pre>
        </div>
      )}
    </div>
  );
}

function TermReasoning({ text, duration }) {
  const [open, setOpen] = React.useState(false);
  return (
    <div style={{ margin: '6px 0', fontFamily: termStyles.mono }}>
      <div onClick={() => setOpen(o => !o)} style={{
        fontSize: 11, color: termStyles.inkMuted,
        cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 6,
      }}>
        <span style={{ color: termStyles.inkFaint }}>{open ? '▼' : '▸'}</span>
        <span>reasoning · {duration}</span>
        <span style={{
          height: 1, flex: 1,
          background: `linear-gradient(to right, ${termStyles.line}, transparent)`,
        }}/>
      </div>
      {open && (
        <div style={{
          marginTop: 6, marginLeft: 14, paddingLeft: 10,
          borderLeft: `1px solid ${termStyles.line}`,
          fontSize: 11, color: termStyles.inkSoft, lineHeight: 1.7,
          whiteSpace: 'pre-wrap',
        }}>{text}</div>
      )}
    </div>
  );
}

function TermMd({ content }) {
  const parts = content.split(/```(\w+)?\n([\s\S]*?)```/g);
  const out = [];
  for (let i = 0; i < parts.length; i++) {
    if (i % 3 === 0) {
      const t = parts[i]; if (!t) continue;
      t.split('\n').forEach((line, idx) => {
        if (line.startsWith('## ')) out.push(<h3 key={`${i}-${idx}`} style={{
          fontSize: 13.5, fontWeight: 600, color: termStyles.accentStrong,
          margin: '12px 0 6px', fontFamily: termStyles.mono,
          textTransform: 'lowercase', letterSpacing: 0.5,
        }}>── {line.slice(3).toLowerCase()}</h3>);
        else if (line.startsWith('### ')) out.push(<h4 key={`${i}-${idx}`} style={{
          fontSize: 11, fontWeight: 600, color: termStyles.inkSoft,
          margin: '8px 0 3px', fontFamily: termStyles.mono,
          textTransform: 'uppercase', letterSpacing: 1,
        }}>{line.slice(4)}</h4>);
        else if (line.match(/^\d+\. /)) out.push(<div key={`${i}-${idx}`} style={{ margin: '2px 0' }}>{renderInline(line)}</div>);
        else if (line.trim() === '') out.push(<div key={`${i}-${idx}`} style={{ height: 4 }}/>);
        else out.push(<div key={`${i}-${idx}`} style={{ margin: '2px 0' }}>{renderInline(line)}</div>);
      });
    } else if (i % 3 === 2) {
      out.push(<TermCodeBlock key={`c-${i}`} lang={parts[i-1]} code={parts[i].trim()} />);
    }
  }
  return <div style={{
    fontFamily: termStyles.mono, fontSize: 12.5, color: termStyles.ink, lineHeight: 1.65,
  }}>{out}</div>;

  function renderInline(line) {
    const segs = line.split(/(\*\*[^*]+\*\*|`[^`]+`)/g);
    return segs.map((s, k) => {
      if (s.startsWith('**')) return <strong key={k} style={{ color: termStyles.accentStrong, fontWeight: 500 }}>{s.slice(2, -2)}</strong>;
      if (s.startsWith('`')) return <code key={k} style={{
        fontFamily: termStyles.mono, fontSize: 11.5,
        background: termStyles.bgElev, padding: '1px 5px', borderRadius: 2,
        color: termStyles.accent, border: `1px solid ${termStyles.line}`,
      }}>{s.slice(1, -1)}</code>;
      return <span key={k}>{s}</span>;
    });
  }
}

function TermCodeBlock({ lang, code }) {
  return (
    <div style={{
      margin: '8px 0', background: termStyles.bgDeep,
      border: `1px solid ${termStyles.line}`, borderRadius: 3, overflow: 'hidden',
    }}>
      <div style={{
        padding: '4px 10px', borderBottom: `1px solid ${termStyles.line}`,
        display: 'flex', alignItems: 'center', gap: 8,
        fontFamily: termStyles.mono, fontSize: 10,
        color: termStyles.inkMuted,
      }}>
        <span style={{ color: termStyles.accent }}>●</span>
        <span>{lang}</span>
        <span style={{ flex: 1 }}/>
        <span style={{ cursor: 'pointer' }}>[copy]</span>
      </div>
      <pre style={{
        margin: 0, padding: '10px 12px', fontFamily: termStyles.mono, fontSize: 11.5,
        color: termStyles.ink, lineHeight: 1.6, whiteSpace: 'pre', overflow: 'auto',
      }}>{code}</pre>
    </div>
  );
}

function TermUserMsg({ msg }) {
  return (
    <div style={{ padding: '12px 24px 4px', fontFamily: termStyles.mono }}>
      <div style={{
        fontSize: 11, color: termStyles.inkMuted, marginBottom: 4,
        display: 'flex', gap: 8, alignItems: 'center',
      }}>
        <span style={{ color: termStyles.accent }}>{'>'}</span>
        <span style={{ color: termStyles.ink, fontWeight: 500 }}>user@local</span>
        <span>·</span>
        <span>{msg.time}</span>
      </div>
      <div style={{
        paddingLeft: 18, fontSize: 12.5, color: termStyles.ink, lineHeight: 1.6,
        fontFamily: termStyles.mono,
      }}>{msg.content}</div>
    </div>
  );
}

function TermAssistantMsg({ msg, onOpenArtifact }) {
  return (
    <div style={{ padding: '10px 24px 14px', fontFamily: termStyles.mono }}>
      <div style={{
        fontSize: 11, color: termStyles.inkMuted, marginBottom: 6,
        display: 'flex', gap: 8, alignItems: 'center',
      }}>
        <span style={{ color: termStyles.accent, textShadow: `0 0 4px ${termStyles.accent}` }}>δ</span>
        <span style={{ color: termStyles.accentStrong, fontWeight: 500 }}>daimon</span>
        <span>·</span>
        <span>{msg.time}</span>
        <span style={{
          marginLeft: 'auto',
          width: 6, height: 6, borderRadius: 99, background: termStyles.accent,
          animation: 'termBreathe 1.6s ease-in-out infinite',
          boxShadow: `0 0 6px ${termStyles.accent}`,
        }}/>
      </div>
      <div style={{ paddingLeft: 18 }}>
        {msg.reasoning && <TermReasoning text={msg.reasoning} duration={msg.reasoningDuration} />}
        {msg.blocks.map((b, i) => {
          if (b.kind === 'tool') return <TermToolCall key={i} tool={b} onOpen={b.name === 'read_file' ? () => onOpenArtifact('log') : null} />;
          if (b.kind === 'text') return <div key={i} style={{ margin: '6px 0' }}><TermMd content={b.content} /></div>;
          return null;
        })}
      </div>
    </div>
  );
}

function TermWorkspace({ artifact, onClose }) {
  if (!artifact) {
    return (
      <div style={{
        height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center',
        color: termStyles.inkMuted, fontFamily: termStyles.mono, fontSize: 11.5,
      }}>
        <div style={{ textAlign: 'center' }}>
          <div style={{ fontSize: 24, marginBottom: 8, color: termStyles.accent, opacity: 0.5 }}>◌</div>
          <div>// workspace empty</div>
          <div style={{ fontSize: 10, marginTop: 4, color: termStyles.inkFaint }}>click [open →] on any tool</div>
        </div>
      </div>
    );
  }
  if (artifact.type === 'code') {
    return (
      <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        <div style={{
          padding: '8px 14px', borderBottom: `1px solid ${termStyles.line}`,
          display: 'flex', alignItems: 'center', gap: 10, background: termStyles.bgDeep,
        }}>
          <span style={{ color: termStyles.accent, fontSize: 10 }}>●</span>
          <span style={{ fontFamily: termStyles.mono, fontSize: 11.5, color: termStyles.ink, flex: 1 }}>{artifact.title}</span>
          <button style={{
            fontSize: 10.5, padding: '2px 8px', background: 'transparent',
            border: `1px solid ${termStyles.line}`, borderRadius: 2,
            color: termStyles.inkSoft, cursor: 'pointer', fontFamily: termStyles.mono,
          }}>[copy]</button>
          <button style={{
            fontSize: 10.5, padding: '2px 8px', background: termStyles.accentSoft,
            border: `1px solid ${termStyles.accent}`, borderRadius: 2,
            color: termStyles.accent, cursor: 'pointer', fontFamily: termStyles.mono,
          }}>[apply patch]</button>
          <span onClick={onClose} style={{ fontSize: 14, color: termStyles.inkMuted, cursor: 'pointer' }}>×</span>
        </div>
        <div style={{ display: 'flex', flex: 1, overflow: 'hidden', background: termStyles.bgDeep }}>
          <div style={{
            padding: '12px 6px 12px 14px', fontFamily: termStyles.mono, fontSize: 11,
            color: termStyles.inkFaint, textAlign: 'right',
            userSelect: 'none', borderRight: `1px solid ${termStyles.line}`, lineHeight: 1.7,
          }}>
            {artifact.content.split('\n').map((_, i) => <div key={i}>{i + 1}</div>)}
          </div>
          <pre style={{
            margin: 0, padding: '12px 14px', flex: 1, overflow: 'auto',
            fontFamily: termStyles.mono, fontSize: 11.5, color: termStyles.ink,
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
          padding: '8px 14px', borderBottom: `1px solid ${termStyles.line}`,
          display: 'flex', alignItems: 'center', gap: 10, background: termStyles.bgDeep,
        }}>
          <span style={{ color: termStyles.accent, fontSize: 10 }}>∷</span>
          <span style={{ fontFamily: termStyles.mono, fontSize: 11.5, color: termStyles.ink, flex: 1 }}>{artifact.title}</span>
          <span onClick={onClose} style={{ fontSize: 14, color: termStyles.inkMuted, cursor: 'pointer' }}>×</span>
        </div>
        <div style={{ flex: 1, overflow: 'auto', background: termStyles.bgDeep }}>
          {artifact.entries.map((e, i) => (
            <div key={i} style={{
              display: 'flex', gap: 12, padding: '3px 14px',
              fontFamily: termStyles.mono, fontSize: 11,
              borderBottom: `1px solid ${termStyles.line}`,
            }}>
              <span style={{ color: termStyles.inkMuted, width: 60 }}>{e.t}</span>
              <span style={{
                width: 42, fontWeight: 600, fontSize: 10,
                color: e.lvl === 'ERROR' ? termStyles.error : e.lvl === 'WARN' ? termStyles.warn : termStyles.accent,
              }}>{e.lvl}</span>
              <span style={{ color: termStyles.ink, flex: 1 }}>{e.msg}</span>
            </div>
          ))}
        </div>
      </div>
    );
  }
  return null;
}

function TermInput({ showCmd }) {
  return (
    <div style={{ padding: 14, borderTop: `1px solid ${termStyles.line}`, background: termStyles.bgSidebar }}>
      <div style={{
        border: `1px solid ${termStyles.lineStrong}`, borderRadius: 3,
        background: termStyles.bgDeep,
        padding: '9px 12px',
        fontFamily: termStyles.mono,
      }}>
        <div style={{
          display: 'flex', alignItems: 'center', gap: 8, fontSize: 12.5,
          color: termStyles.inkMuted, minHeight: 32,
        }}>
          <span style={{ color: termStyles.accent }}>{'>'}</span>
          <span>type a message or /slash for commands_</span>
        </div>
        <div style={{
          display: 'flex', alignItems: 'center', gap: 6,
          fontSize: 10.5, color: termStyles.inkMuted, marginTop: 6,
        }}>
          <span style={{ cursor: 'pointer', color: termStyles.inkSoft }}>[+ attach]</span>
          <span style={{ cursor: 'pointer', color: termStyles.inkSoft }}>[/ tools]</span>
          <span style={{ cursor: 'pointer', color: termStyles.inkSoft }}>[@ mention]</span>
          <span style={{ flex: 1 }}/>
          <span onClick={showCmd} style={{ cursor: 'pointer', color: termStyles.accent }}>⌘K palette</span>
          <span>·</span>
          <span style={{ color: termStyles.accent }}>↵ send</span>
        </div>
      </div>
    </div>
  );
}

function TermCmdPalette({ onClose }) {
  return (
    <div onClick={onClose} style={{
      position: 'absolute', inset: 0, background: 'rgba(8,9,16,0.7)',
      display: 'flex', alignItems: 'flex-start', justifyContent: 'center',
      paddingTop: 100, zIndex: 50, backdropFilter: 'blur(3px)',
    }}>
      <div onClick={(e) => e.stopPropagation()} style={{
        width: 480, background: termStyles.bgElev,
        border: `1px solid ${termStyles.accent}`,
        borderRadius: 3, overflow: 'hidden',
        boxShadow: `0 0 40px ${termStyles.accent}22, 0 20px 60px rgba(0,0,0,0.6)`,
      }}>
        <div style={{
          padding: '12px 16px', borderBottom: `1px solid ${termStyles.line}`,
          display: 'flex', alignItems: 'center', gap: 10, fontFamily: termStyles.mono,
        }}>
          <span style={{ color: termStyles.accent, fontSize: 12 }}>{'>'}</span>
          <span style={{ fontSize: 13, color: termStyles.ink, flex: 1 }}>
            <span style={{ color: termStyles.accent }}>_</span>
          </span>
          <span style={{ fontSize: 10, color: termStyles.inkMuted }}>esc to close</span>
        </div>
        <div style={{ padding: '4px 0', fontFamily: termStyles.mono }}>
          <div style={{ padding: '4px 16px', fontSize: 10, color: termStyles.inkMuted, textTransform: 'uppercase', letterSpacing: 1 }}>── commands</div>
          {[
            { code: '01', l: 'new chat', s: '⌘ N', hi: true },
            { code: '02', l: 'fork conversation', s: '⌘ ⇧ F' },
            { code: '03', l: 'add to memory', s: '⌘ M' },
            { code: '04', l: 'run tool', s: '⌘ T' },
            { code: '05', l: 'toggle theme', s: '⌘ .' },
          ].map(i => (
            <div key={i.l} style={{
              padding: '6px 16px', display: 'flex', alignItems: 'center', gap: 10,
              fontSize: 12, color: i.hi ? termStyles.accent : termStyles.ink,
              background: i.hi ? termStyles.accentSoft : 'transparent',
              borderLeft: i.hi ? `2px solid ${termStyles.accent}` : '2px solid transparent',
              paddingLeft: i.hi ? 14 : 16,
              cursor: 'pointer',
            }}>
              <span style={{ color: termStyles.inkFaint, fontSize: 10 }}>{i.code}</span>
              <span style={{ flex: 1 }}>{i.l}</span>
              <span style={{ fontSize: 10, color: termStyles.inkMuted }}>{i.s}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function TermChat({ showCmd, cmdOpen, closeCmd }) {
  const [artifact, setArtifact] = React.useState(WORKSPACE_ARTIFACTS.patch);
  return (
    <div style={{
      height: '100%', display: 'flex', flexDirection: 'row',
      background: termStyles.bg, color: termStyles.ink,
      fontFamily: termStyles.mono, position: 'relative',
    }}>
      <TermSidebar />
      <div style={{ flex: 1.3, display: 'flex', flexDirection: 'column', minWidth: 0 }}>
        <div style={{
          padding: '10px 24px', borderBottom: `1px solid ${termStyles.line}`,
          display: 'flex', alignItems: 'center', gap: 12,
          background: termStyles.bgSidebar,
        }}>
          <div style={{ flex: 1 }}>
            <div style={{
              fontSize: 12, color: termStyles.ink, fontWeight: 500,
              fontFamily: termStyles.mono,
            }}>
              <span style={{ color: termStyles.accent }}>~/chat/</span>payment-anomalies
            </div>
            <div style={{
              fontSize: 10.5, color: termStyles.inkMuted, marginTop: 2,
              display: 'flex', gap: 8, fontFamily: termStyles.mono,
            }}>
              <span>14:32</span><span>·</span>
              <span>3 tools</span><span>·</span>
              <span>$0.042</span><span>·</span>
              <span>iter 4</span>
            </div>
          </div>
          <span style={{
            fontSize: 10, color: termStyles.accent, fontFamily: termStyles.mono,
            padding: '2px 8px', border: `1px solid ${termStyles.accent}44`, borderRadius: 2,
            display: 'flex', alignItems: 'center', gap: 5,
          }}>
            <span style={{
              width: 6, height: 6, borderRadius: 99, background: termStyles.accent,
              boxShadow: `0 0 4px ${termStyles.accent}`,
              animation: 'termBreathe 1.6s ease-in-out infinite',
            }}/>
            READY
          </span>
        </div>
        <div style={{ flex: 1, overflow: 'auto' }}>
          {MOCK_CONVO.map(m => m.role === 'user'
            ? <TermUserMsg key={m.id} msg={m} />
            : <TermAssistantMsg key={m.id} msg={m} onOpenArtifact={(k) => setArtifact(WORKSPACE_ARTIFACTS[k])} />)}
        </div>
        <TermInput showCmd={showCmd} />
      </div>
      <div style={{ flex: 1, borderLeft: `1px solid ${termStyles.line}`, minWidth: 0 }}>
        <TermWorkspace artifact={artifact} onClose={() => setArtifact(null)} />
      </div>
      {cmdOpen && <TermCmdPalette onClose={closeCmd} />}
      <style>{`
        @keyframes termCaret { 0%,50% { opacity: 1; } 51%,100% { opacity: 0; } }
        @keyframes termBreathe { 0%,100% { opacity: 1; transform: scale(1); } 50% { opacity: 0.5; transform: scale(0.7); } }
      `}</style>
    </div>
  );
}

Object.assign(window, { TermChat, TermWordmark, termStyles });
