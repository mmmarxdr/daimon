// Daimon Memory — editorial-strong direction
// Two surfaces: Long-term memories (what Daimon learned) + Knowledge (ingested files).
// Editorial voice in Fraunces italic for narrative headers.
// Trust surface: confidence pills, source refs, last-seen date.
// Inline edit + "no es así" quarantine on every card.

const MEM_IS_MAC = typeof navigator !== 'undefined' &&
  /Mac|iPhone|iPad|iPod/i.test(navigator.platform || navigator.userAgent || '');
const MEM_MOD = MEM_IS_MAC ? '⌘' : 'Ctrl';

// ─────────────────────────────────────────────────────────────────
// Confidence visual system — the trust surface
// ─────────────────────────────────────────────────────────────────
const CONFIDENCE_META = {
  certain:  { label: 'certain',  italic: 'I know',        dots: 3, colorKey: 'accent', tone: 'solid' },
  inferred: { label: 'inferred', italic: 'I infer',       dots: 2, colorKey: 'amber',  tone: 'dashed' },
  assumed:  { label: 'assumed',  italic: 'I assume',      dots: 1, colorKey: 'inkMuted', tone: 'dotted' },
};

function ConfidenceGlyph({ conf, theme, size = 10 }) {
  const meta = CONFIDENCE_META[conf];
  const color = theme[meta.colorKey];
  return (
    <div style={{ display: 'inline-flex', gap: 2, alignItems: 'center' }}>
      {[0, 1, 2].map(i => (
        <span key={i} style={{
          width: size, height: size, borderRadius: 99,
          background: i < meta.dots ? color : 'transparent',
          border: i < meta.dots ? 'none' : `1px solid ${theme.line}`,
          transition: 'background 0.2s',
        }}/>
      ))}
    </div>
  );
}

function ConfidencePill({ conf, theme, showIcon = true }) {
  const meta = CONFIDENCE_META[conf];
  const color = theme[meta.colorKey];
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 6,
      fontSize: 10.5, color,
      background: `${color}14`, border: `1px solid ${color}33`,
      borderRadius: 99, padding: '2px 8px 2px 6px',
      fontFamily: '"Inter", system-ui, sans-serif',
      letterSpacing: 0.2,
    }}>
      {showIcon && <ConfidenceGlyph conf={conf} theme={theme} size={4.5} />}
      <span style={{ fontFamily: '"Fraunces", Georgia, serif', fontStyle: 'italic' }}>{meta.italic}</span>
    </span>
  );
}

// ─────────────────────────────────────────────────────────────────
// Memory card — a single fact or note
// ─────────────────────────────────────────────────────────────────
function MemoryCard({ mem, theme, density, showTrust }) {
  const [hover, setHover] = React.useState(false);
  const [menu, setMenu] = React.useState(false);
  const [status, setStatus] = React.useState('live'); // live | editing | quarantine
  const isNote = mem.kind === 'note';

  const padY = density === 'dense' ? 10 : density === 'sparse' ? 20 : 14;
  const padX = density === 'dense' ? 14 : density === 'sparse' ? 22 : 18;

  return (
    <div
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => { setHover(false); setMenu(false); }}
      style={{
        position: 'relative',
        padding: `${padY}px ${padX}px`,
        background: status === 'quarantine' ? `${theme.red}08` : theme.bgElev,
        border: `1px solid ${status === 'quarantine' ? theme.red + '44' : hover ? theme.lineStrong : theme.line}`,
        borderRadius: 6,
        borderLeft: `2px solid ${status === 'quarantine' ? theme.red : theme[CONFIDENCE_META[mem.confidence].colorKey]}`,
        transition: 'border-color 0.15s',
        fontFamily: '"Inter", system-ui, sans-serif',
      }}
    >
      {/* Header row: kind indicator, cluster, trust, menu */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8,
        fontSize: 10.5, color: theme.inkMuted,
      }}>
        <span style={{
          fontFamily: '"JetBrains Mono", monospace',
          textTransform: 'uppercase', letterSpacing: 0.7,
          color: isNote ? theme.accent : theme.inkMuted,
          fontWeight: isNote ? 600 : 400,
        }}>{isNote ? 'note' : 'fact'}</span>
        <span style={{ color: theme.inkFaint }}>·</span>
        <span style={{ color: theme.inkSoft }}>{mem.cluster}</span>
        <span style={{ flex: 1 }}/>
        {showTrust && <ConfidencePill conf={mem.confidence} theme={theme} />}
        <div style={{ position: 'relative' }}>
          <button
            onClick={() => setMenu(m => !m)}
            style={{
              width: 22, height: 22, borderRadius: 4, cursor: 'pointer',
              background: menu ? theme.bgDeep : 'transparent',
              border: `1px solid ${menu ? theme.line : 'transparent'}`,
              color: theme.inkMuted, fontSize: 14, lineHeight: 1,
              opacity: hover || menu ? 1 : 0,
              transition: 'opacity 0.15s',
            }}
          >⋯</button>
          {menu && <MemoryMenu theme={theme} onQuarantine={() => { setStatus('quarantine'); setMenu(false); }} onEdit={() => { setStatus('editing'); setMenu(false); }} />}
        </div>
      </div>

      {/* Body */}
      {status === 'editing' ? (
        <div>
          <div
            contentEditable suppressContentEditableWarning
            style={{
              fontSize: isNote ? 13 : 13.5, lineHeight: 1.6,
              color: theme.ink, outline: 'none',
              padding: '6px 8px', margin: '-6px -8px',
              borderRadius: 4, background: theme.bgDeep,
              border: `1px solid ${theme.accent}66`,
            }}
          >{mem.content}</div>
          <div style={{ display: 'flex', gap: 6, marginTop: 8 }}>
            <button onClick={() => setStatus('live')} style={editBtnStyle(theme, true)}>Save</button>
            <button onClick={() => setStatus('live')} style={editBtnStyle(theme, false)}>Cancel</button>
          </div>
        </div>
      ) : (
        <div style={{
          fontSize: isNote ? 13 : 13.5, lineHeight: 1.6, color: theme.ink,
          textDecoration: status === 'quarantine' ? 'line-through' : 'none',
          opacity: status === 'quarantine' ? 0.55 : 1,
          fontFamily: isNote ? '"Inter", system-ui, sans-serif' : '"Inter", system-ui, sans-serif',
        }}>{mem.content}</div>
      )}

      {/* Footer: tags, source, last-seen */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 6,
        marginTop: 10, fontSize: 10.5, color: theme.inkMuted,
        fontFamily: '"JetBrains Mono", monospace',
        flexWrap: 'wrap',
      }}>
        {mem.tags.map(t => (
          <span key={t} style={{
            padding: '1px 7px', borderRadius: 99,
            border: `1px solid ${theme.line}`,
            color: theme.inkSoft, fontSize: 10,
          }}>#{t}</span>
        ))}
        <span style={{ flex: 1 }}/>
        {showTrust && (
          <>
            <SourceRef source={mem.source} theme={theme} />
            <span style={{ color: theme.inkFaint }}>·</span>
            <span title={`Confirmed ${mem.confirmedCount}×`}>seen {mem.lastSeen}</span>
          </>
        )}
      </div>

      {status === 'quarantine' && (
        <div style={{
          marginTop: 10, paddingTop: 10, borderTop: `1px dashed ${theme.red}44`,
          fontSize: 11, color: theme.red,
          fontFamily: '"Fraunces", serif', fontStyle: 'italic',
          display: 'flex', alignItems: 'center', gap: 8,
        }}>
          <span>I’ll forget this. <span style={{ color: theme.inkMuted, fontStyle: 'normal', fontFamily: '"Inter", sans-serif' }}>Next time it comes up, I’ll ask.</span></span>
          <span style={{ flex: 1 }}/>
          <button onClick={() => setStatus('live')} style={editBtnStyle(theme, false)}>Undo</button>
        </div>
      )}
    </div>
  );
}

function MemoryMenu({ theme, onEdit, onQuarantine }) {
  return (
    <div style={{
      position: 'absolute', right: 0, top: 26, zIndex: 10,
      background: theme.bgElev, border: `1px solid ${theme.lineStrong}`,
      borderRadius: 6, padding: 4, minWidth: 180,
      boxShadow: '0 8px 24px rgba(0,0,0,0.12)',
      fontFamily: '"Inter", system-ui, sans-serif', fontSize: 12,
    }}>
      {[
        { label: 'Edit', detail: 'correct the wording', onClick: onEdit },
        { label: 'Pin', detail: 'surface this often' },
        { label: 'Trace origin', detail: 'open source conversation' },
        { divider: true },
        { label: 'That’s not right', detail: 'mark for forgetting', onClick: onQuarantine, danger: true },
      ].map((it, i) => it.divider ? (
        <div key={i} style={{ height: 1, background: theme.line, margin: '4px 2px' }}/>
      ) : (
        <div key={i} onClick={it.onClick} style={{
          padding: '7px 10px', borderRadius: 4, cursor: 'pointer',
          color: it.danger ? theme.red : theme.ink,
        }}>
          <div style={{ fontWeight: 500 }}>{it.label}</div>
          <div style={{
            fontSize: 10.5, color: it.danger ? theme.red + 'cc' : theme.inkMuted,
            fontFamily: '"Fraunces", serif', fontStyle: 'italic', marginTop: 1,
          }}>{it.detail}</div>
        </div>
      ))}
    </div>
  );
}

function SourceRef({ source, theme }) {
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 4,
      color: theme.inkMuted, fontSize: 10.5,
      cursor: 'pointer',
    }} title={`From "${source.conv}" on ${source.date}`}>
      <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2">
        <path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1"/>
        <path d="M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1"/>
      </svg>
      <span style={{
        maxWidth: 120, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
      }}>{source.conv}</span>
    </span>
  );
}

function editBtnStyle(theme, primary) {
  return {
    fontSize: 11, padding: '4px 10px', borderRadius: 4,
    fontFamily: '"Inter", sans-serif', fontWeight: 500,
    cursor: 'pointer',
    background: primary ? theme.accent : 'transparent',
    color: primary ? theme.bgElev : theme.inkSoft,
    border: primary ? 'none' : `1px solid ${theme.line}`,
  };
}

// ─────────────────────────────────────────────────────────────────
// Knowledge card — ingested file, markdown-ified, injectable
// ─────────────────────────────────────────────────────────────────
function KnowledgeCard({ kn, theme, density }) {
  const [hover, setHover] = React.useState(false);
  const isIndexing = kn.status === 'indexing';
  const padY = density === 'dense' ? 12 : density === 'sparse' ? 20 : 16;
  const padX = density === 'dense' ? 14 : density === 'sparse' ? 22 : 18;

  return (
    <div
      onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}
      style={{
        position: 'relative', padding: `${padY}px ${padX}px`,
        background: theme.bgElev,
        border: `1px solid ${hover ? theme.lineStrong : theme.line}`,
        borderRadius: 6,
        fontFamily: '"Inter", system-ui, sans-serif',
        overflow: 'hidden',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12 }}>
        <FileTypeGlyph type={kn.type} theme={theme} indexing={isIndexing} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{
            fontSize: 13.5, fontWeight: 500, color: theme.ink,
            fontFamily: '"JetBrains Mono", monospace',
            whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
            marginBottom: 3,
          }}>{kn.title}</div>
          <div style={{
            fontSize: 11, color: theme.inkMuted,
            fontFamily: '"JetBrains Mono", monospace',
            display: 'flex', gap: 8, flexWrap: 'wrap',
          }}>
            <span>{kn.originalName}</span>
            <span style={{ color: theme.inkFaint }}>·</span>
            <span>{kn.originalSize}</span>
            <span style={{ color: theme.inkFaint }}>·</span>
            <span>{kn.chunks} chunks</span>
          </div>
        </div>
        {isIndexing ? (
          <span style={{
            fontSize: 10, color: theme.amber,
            background: `${theme.amber}14`, border: `1px solid ${theme.amber}44`,
            borderRadius: 99, padding: '2px 8px',
            fontFamily: '"Fraunces", serif', fontStyle: 'italic',
            display: 'flex', alignItems: 'center', gap: 6,
          }}>
            <span style={{
              width: 5, height: 5, borderRadius: 99, background: theme.amber,
              animation: 'memBreathe 1.1s ease-in-out infinite',
            }}/>
            indexing
          </span>
        ) : (
          <span style={{
            fontSize: 10, color: theme.green,
            background: `${theme.green}14`, border: `1px solid ${theme.green}44`,
            borderRadius: 99, padding: '2px 8px',
            fontFamily: '"Fraunces", serif', fontStyle: 'italic',
          }}>ready</span>
        )}
      </div>

      <div style={{
        marginTop: 12, fontSize: 12.5, color: theme.inkSoft, lineHeight: 1.55,
      }}>{kn.summary}</div>

      <div style={{
        marginTop: 12, display: 'flex', alignItems: 'center', gap: 10,
        fontSize: 10.5, color: theme.inkMuted,
        fontFamily: '"JetBrains Mono", monospace',
      }}>
        <span>ingested {kn.ingestedAt}</span>
        <span style={{ color: theme.inkFaint }}>·</span>
        <span>used {kn.lastUsed}</span>
        <span style={{ flex: 1 }}/>
        <span title="times injected into context" style={{ color: theme.accent }}>
          {kn.injections}× injected
        </span>
      </div>
    </div>
  );
}

function FileTypeGlyph({ type, theme, indexing }) {
  const label = type.toUpperCase();
  return (
    <div style={{
      width: 32, height: 40, flexShrink: 0,
      background: theme.bgDeep,
      border: `1px solid ${theme.line}`,
      borderRadius: 3,
      display: 'flex', flexDirection: 'column',
      alignItems: 'center', justifyContent: 'center',
      fontFamily: '"JetBrains Mono", monospace',
      fontSize: 8, color: theme.accent, fontWeight: 600,
      letterSpacing: 0.5,
      position: 'relative',
      overflow: 'hidden',
    }}>
      {/* fake page corner */}
      <div style={{
        position: 'absolute', top: 0, right: 0,
        width: 8, height: 8,
        borderLeft: `1px solid ${theme.line}`,
        borderBottom: `1px solid ${theme.line}`,
        background: theme.bg,
      }}/>
      {indexing && (
        <div style={{
          position: 'absolute', left: 0, right: 0, bottom: 0, height: 2,
          background: theme.amber,
          animation: 'memIndex 1.6s linear infinite',
          transformOrigin: 'left',
        }}/>
      )}
      <span>{label}</span>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Filter/toolbar
// ─────────────────────────────────────────────────────────────────
function MemoryToolbar({ theme, filter, setFilter, sort, setSort, query, setQuery, counts }) {
  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 10,
      marginBottom: 18, flexWrap: 'wrap',
    }}>
      {/* Search */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 8,
        padding: '7px 12px',
        background: theme.bgElev, border: `1px solid ${theme.line}`,
        borderRadius: 5, minWidth: 260, flex: 1, maxWidth: 380,
      }}>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke={theme.inkMuted} strokeWidth="2">
          <circle cx="11" cy="11" r="7"/><path d="m21 21-5-5"/>
        </svg>
        <input
          value={query} onChange={e => setQuery(e.target.value)}
          placeholder="search what I remember…"
          style={{
            flex: 1, border: 'none', outline: 'none', background: 'transparent',
            fontSize: 12.5, color: theme.ink,
            fontFamily: '"Fraunces", serif', fontStyle: 'italic',
          }}
        />
        {query && <span onClick={() => setQuery('')} style={{ color: theme.inkMuted, cursor: 'pointer', fontSize: 14 }}>×</span>}
      </div>

      {/* Filter chips */}
      <div style={{ display: 'flex', gap: 4 }}>
        {[
          { k: 'all',       l: 'all',       n: counts.total },
          { k: 'certain',   l: 'certain',   n: counts.certain,  color: theme.accent },
          { k: 'inferred',  l: 'inferred',  n: counts.inferred, color: theme.amber },
          { k: 'assumed',   l: 'assumed',   n: counts.assumed,  color: theme.inkMuted },
        ].map(f => {
          const on = filter === f.k;
          const c = f.color || theme.ink;
          return (
            <button key={f.k} onClick={() => setFilter(f.k)} style={{
              display: 'flex', alignItems: 'center', gap: 6,
              padding: '6px 10px', borderRadius: 5,
              fontSize: 11.5, cursor: 'pointer',
              fontFamily: '"Inter", system-ui, sans-serif',
              background: on ? (f.color ? `${f.color}18` : theme.bgDeep) : 'transparent',
              color: on ? c : theme.inkSoft,
              border: `1px solid ${on ? (f.color ? c + '55' : theme.lineStrong) : theme.line}`,
              fontWeight: on ? 500 : 400,
            }}>
              {f.l}
              <span style={{
                fontSize: 10, color: on ? c : theme.inkMuted,
                fontFamily: '"JetBrains Mono", monospace',
              }}>{f.n}</span>
            </button>
          );
        })}
      </div>

      <span style={{ flex: 1 }}/>

      {/* Sort */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 4,
        fontSize: 11, color: theme.inkMuted,
      }}>
        <span style={{ fontFamily: '"Fraunces", serif', fontStyle: 'italic' }}>ordered by</span>
        <select value={sort} onChange={e => setSort(e.target.value)} style={{
          background: 'transparent', border: `1px solid ${theme.line}`,
          color: theme.ink, fontSize: 11.5, padding: '5px 8px', borderRadius: 4,
          fontFamily: '"Inter", sans-serif', cursor: 'pointer',
        }}>
          <option value="recent">most recent</option>
          <option value="confidence">confidence</option>
          <option value="confirmations">most confirmed</option>
          <option value="cluster">by cluster</option>
        </select>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Cluster sections — when sort=cluster, group visually
// ─────────────────────────────────────────────────────────────────
const CLUSTER_META = {
  identity:       { label: 'Identity',       daimon: 'who you are to me' },
  preferences:    { label: 'Preferences',    daimon: 'how you like things' },
  projects:       { label: 'Projects',       daimon: 'what we are building together' },
  relationships:  { label: 'Relationships',  daimon: 'the people around you' },
  technical:      { label: 'Technical',      daimon: 'your tools and terrain' },
};

function ClusterHeader({ cluster, count, theme }) {
  const meta = CLUSTER_META[cluster] || { label: cluster, daimon: cluster };
  return (
    <div style={{
      display: 'flex', alignItems: 'baseline', gap: 12,
      margin: '28px 0 14px', padding: '0 2px',
    }}>
      <h3 style={{
        margin: 0, fontFamily: '"Fraunces", Georgia, serif',
        fontSize: 20, fontWeight: 500, color: theme.ink,
        letterSpacing: -0.3,
      }}>{meta.label}</h3>
      <span style={{
        fontFamily: '"Fraunces", serif', fontStyle: 'italic',
        fontSize: 13, color: theme.inkMuted,
      }}>— {meta.daimon}</span>
      <span style={{ flex: 1, height: 1, background: theme.line, alignSelf: 'center', marginTop: 2 }}/>
      <span style={{
        fontSize: 10.5, color: theme.inkMuted,
        fontFamily: '"JetBrains Mono", monospace',
      }}>{count} {count === 1 ? 'thing' : 'things'} I know</span>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Tab bar — memories vs knowledge
// ─────────────────────────────────────────────────────────────────
function MemoryTabs({ tab, setTab, theme }) {
  const tabs = [
    { k: 'memory',    l: 'Long-term memory', italic: 'what I remember',      count: MEMORIES.length },
    { k: 'knowledge', l: 'Knowledge',        italic: 'what you’ve given me', count: KNOWLEDGE.length },
  ];
  return (
    <div style={{
      display: 'flex', gap: 0,
      borderBottom: `1px solid ${theme.line}`,
      marginBottom: 24,
    }}>
      {tabs.map(t => {
        const on = tab === t.k;
        return (
          <button key={t.k} onClick={() => setTab(t.k)} style={{
            background: 'transparent', border: 'none', cursor: 'pointer',
            padding: '0 0 14px', marginRight: 32, position: 'relative',
            fontFamily: '"Inter", system-ui, sans-serif',
            display: 'flex', alignItems: 'baseline', gap: 10,
          }}>
            <span style={{
              fontSize: 15, fontWeight: on ? 600 : 400,
              color: on ? theme.ink : theme.inkMuted,
              letterSpacing: -0.1,
            }}>{t.l}</span>
            <span style={{
              fontFamily: '"Fraunces", serif', fontStyle: 'italic',
              fontSize: 13, color: on ? theme.accent : theme.inkFaint,
            }}>— {t.italic}</span>
            <span style={{
              fontSize: 10.5, color: theme.inkMuted,
              fontFamily: '"JetBrains Mono", monospace',
              background: on ? theme.bgDeep : 'transparent',
              padding: '1px 7px', borderRadius: 99,
              border: `1px solid ${theme.line}`,
            }}>{t.count}</span>
            {on && <div style={{
              position: 'absolute', left: 0, right: 0, bottom: -1, height: 2,
              background: theme.accent,
            }}/>}
          </button>
        );
      })}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Memory screen — main composition
// ─────────────────────────────────────────────────────────────────
function DaimonMemory({ theme, isDark, onToggleTheme, tweaks = {} }) {
  const density   = tweaks.density   || 'normal';       // sparse | normal | dense
  const layout    = tweaks.layout    || 'grid';         // grid | list | reading
  const showTrust = tweaks.showTrust !== false;         // trust surface visible
  const editorial = tweaks.editorial !== false;         // Daimon voice on/off

  const [tab, setTab]       = React.useState('memory');
  const [filter, setFilter] = React.useState('all');
  const [sort, setSort]     = React.useState('cluster');
  const [query, setQuery]   = React.useState('');

  // Filter + sort memories
  let items = MEMORIES;
  if (filter !== 'all') items = items.filter(m => m.confidence === filter);
  if (query.trim()) {
    const q = query.toLowerCase();
    items = items.filter(m => m.content.toLowerCase().includes(q) || m.tags.some(t => t.includes(q)));
  }
  const sortedItems = [...items].sort((a, b) => {
    if (sort === 'confidence') {
      const order = { certain: 0, inferred: 1, assumed: 2 };
      return order[a.confidence] - order[b.confidence];
    }
    if (sort === 'confirmations') return b.confirmedCount - a.confirmedCount;
    if (sort === 'cluster') return a.cluster.localeCompare(b.cluster);
    return 0; // recent = source order
  });

  // Group by cluster when sort=cluster
  const groupedByCluster = sort === 'cluster'
    ? Object.entries(sortedItems.reduce((acc, m) => {
        (acc[m.cluster] = acc[m.cluster] || []).push(m); return acc;
      }, {}))
    : null;

  const counts = {
    total: MEMORIES.length,
    certain: MEMORIES.filter(m => m.confidence === 'certain').length,
    inferred: MEMORIES.filter(m => m.confidence === 'inferred').length,
    assumed: MEMORIES.filter(m => m.confidence === 'assumed').length,
  };

  // layout → grid template
  const gridCols = layout === 'reading' ? '1fr'
    : layout === 'list' ? '1fr'
    : 'repeat(auto-fill, minmax(340px, 1fr))';
  const gridGap = density === 'dense' ? 8 : density === 'sparse' ? 16 : 12;

  return (
    <div style={{
      height: '100%', display: 'flex', flexDirection: 'row',
      background: theme.bg, color: theme.ink,
      fontFamily: '"Inter", system-ui, sans-serif',
    }}>
      {/* Sidebar — reuse the Liminal one for continuity */}
      <LiminalSidebar theme={theme} onToggleTheme={onToggleTheme} isDark={isDark} />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, overflow: 'hidden' }}>
        {/* Editorial preamble — Daimon speaks */}
        {editorial && <MemoryPreamble theme={theme} counts={counts} />}

        {/* Tabs */}
        <div style={{ padding: '0 48px', borderBottom: `1px solid ${theme.line}` }}>
          <MemoryTabs tab={tab} setTab={setTab} theme={theme} />
        </div>

        {/* Scroll area */}
        <div style={{ flex: 1, overflow: 'auto' }}>
          <div style={{
            maxWidth: layout === 'reading' ? 720 : 1100,
            margin: '0 auto', padding: '24px 48px 80px',
          }}>
            {tab === 'memory' ? (
              <>
                <MemoryToolbar
                  theme={theme} filter={filter} setFilter={setFilter}
                  sort={sort} setSort={setSort}
                  query={query} setQuery={setQuery} counts={counts}
                />
                {sortedItems.length === 0 ? (
                  <MemoryEmpty theme={theme} query={query} />
                ) : groupedByCluster ? (
                  groupedByCluster.map(([cluster, group]) => (
                    <div key={cluster}>
                      <ClusterHeader cluster={cluster} count={group.length} theme={theme} />
                      <div style={{ display: 'grid', gridTemplateColumns: gridCols, gap: gridGap }}>
                        {group.map(m => (
                          <MemoryCard key={m.id} mem={m} theme={theme} density={density} showTrust={showTrust} />
                        ))}
                      </div>
                    </div>
                  ))
                ) : (
                  <div style={{ display: 'grid', gridTemplateColumns: gridCols, gap: gridGap }}>
                    {sortedItems.map(m => (
                      <MemoryCard key={m.id} mem={m} theme={theme} density={density} showTrust={showTrust} />
                    ))}
                  </div>
                )}
              </>
            ) : (
              <KnowledgeView theme={theme} density={density} editorial={editorial} />
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function MemoryPreamble({ theme, counts }) {
  return (
    <div style={{
      padding: '24px 48px 18px',
      background: theme.bg,
      borderBottom: `1px solid ${theme.line}`,
    }}>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 14, marginBottom: 6 }}>
        <LiminalGlyph size={20} theme={theme} animate />
        <h1 style={{
          margin: 0, fontFamily: '"Fraunces", Georgia, serif',
          fontSize: 28, fontWeight: 500, color: theme.ink,
          letterSpacing: -0.6,
        }}>
          <span style={{ fontStyle: 'italic', color: theme.accent, fontWeight: 400 }}>what I remember</span>
          <span style={{ color: theme.inkMuted, fontWeight: 400 }}> &nbsp;·&nbsp; </span>
          <span>of you</span>
        </h1>
      </div>
      <div style={{
        fontFamily: '"Fraunces", serif', fontStyle: 'italic',
        fontSize: 14.5, color: theme.inkSoft,
        maxWidth: 640, lineHeight: 1.55, marginLeft: 34,
      }}>
        {counts.total} things live in me: <span style={{ color: theme.accent }}>{counts.certain} I know</span>,{' '}
        <span style={{ color: theme.amber }}>{counts.inferred} I infer from patterns</span>,{' '}
        and <span style={{ color: theme.inkMuted }}>{counts.assumed} I merely assume</span>.
        Correct me where I’m wrong — I’ll forget what you ask me to forget.
      </div>
    </div>
  );
}

function MemoryEmpty({ theme, query }) {
  return (
    <div style={{
      padding: 60, textAlign: 'center',
      display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 14,
    }}>
      <LiminalGlyph size={36} theme={theme} animate={false} />
      <div style={{
        fontFamily: '"Fraunces", serif', fontStyle: 'italic',
        fontSize: 17, color: theme.inkMuted,
      }}>
        {query ? 'nothing I remember matches that.' : 'nothing here yet — we’ve only just met.'}
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────
// Knowledge view
// ─────────────────────────────────────────────────────────────────
function KnowledgeView({ theme, density, editorial }) {
  return (
    <>
      {editorial && (
        <div style={{
          padding: '4px 0 20px',
          fontFamily: '"Fraunces", serif', fontStyle: 'italic',
          fontSize: 14, color: theme.inkSoft,
          maxWidth: 640, lineHeight: 1.55,
        }}>
          You have handed me <span style={{ color: theme.ink }}>{KNOWLEDGE.length} documents</span>.
          I’ve converted them to markdown, chunked them into{' '}
          <span style={{ color: theme.ink }}>
            {KNOWLEDGE.reduce((a, k) => a + k.chunks, 0)} pieces
          </span>, and injected them into our conversations{' '}
          <span style={{ color: theme.accent }}>
            {KNOWLEDGE.reduce((a, k) => a + k.injections, 0)} times
          </span>.
        </div>
      )}

      {/* Drop zone */}
      <div style={{
        border: `2px dashed ${theme.line}`, borderRadius: 6,
        padding: '20px 24px', marginBottom: 16,
        display: 'flex', alignItems: 'center', gap: 14,
        background: theme.bgElev,
      }}>
        <div style={{
          width: 36, height: 36, borderRadius: 6,
          background: theme.accentSoft, color: theme.accent,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          fontSize: 20, fontFamily: '"JetBrains Mono", monospace',
        }}>+</div>
        <div style={{ flex: 1 }}>
          <div style={{ fontSize: 13.5, color: theme.ink, fontWeight: 500 }}>
            Drop files here, or <span style={{ color: theme.accent, textDecoration: 'underline', cursor: 'pointer' }}>choose</span>
          </div>
          <div style={{
            fontFamily: '"Fraunces", serif', fontStyle: 'italic',
            fontSize: 12, color: theme.inkMuted, marginTop: 2,
          }}>I accept PDFs, Markdown, Word, HTML, and zip archives.</div>
        </div>
        <button style={{
          fontSize: 11.5, padding: '7px 14px',
          background: 'transparent', border: `1px solid ${theme.line}`,
          borderRadius: 4, color: theme.inkSoft, cursor: 'pointer',
          fontFamily: '"Inter", sans-serif',
        }}>Connect source…</button>
      </div>

      <div style={{ display: 'grid', gap: density === 'dense' ? 8 : 12 }}>
        {KNOWLEDGE.map(k => <KnowledgeCard key={k.id} kn={k} theme={theme} density={density} />)}
      </div>
    </>
  );
}

// ─────────────────────────────────────────────────────────────────
// Animations
// ─────────────────────────────────────────────────────────────────
function MemoryStyles() {
  return (
    <style>{`
      @keyframes memBreathe { 0%,100% { opacity: 1; transform: scale(1); } 50% { opacity: 0.4; transform: scale(0.7); } }
      @keyframes memIndex { 0% { transform: scaleX(0); } 50% { transform: scaleX(1); } 100% { transform: scaleX(0); transform-origin: right; } }
    `}</style>
  );
}

Object.assign(window, { DaimonMemory, MemoryStyles });
