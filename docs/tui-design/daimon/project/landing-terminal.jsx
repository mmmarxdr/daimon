// Landing B — "The Terminal"
// Experiential-technical. Hero is a live chat demo typing itself + ASCII
// field background reacting to mouse. Every section uses terminal/code
// metaphors. Less poetic, more "this works and it's fast".

// ─────────────────────────────────────────────────────────────
// ASCII field — grid of glyphs that subtly shifts with mouse
// ─────────────────────────────────────────────────────────────
function AsciiField({ theme, isDark }) {
  const canvasRef = React.useRef(null);
  const mouseRef = React.useRef({ x: -10000, y: -10000 });
  const seenRef = React.useRef(false);

  React.useEffect(() => {
    const cv = canvasRef.current; if (!cv) return;
    const ctx = cv.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    let w = 0, h = 0;
    let cols = 0, rows = 0;
    const CELL = 18;
    const CHARS = '·.·:·*+·:·◦·•·-·|·_·⫶·';
    let grid = [];

    const resize = () => {
      const r = cv.getBoundingClientRect();
      w = r.width; h = r.height;
      cv.width = w * dpr; cv.height = h * dpr;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      cols = Math.ceil(w / CELL);
      rows = Math.ceil(h / CELL);
      grid = [];
      for (let r2 = 0; r2 < rows; r2++) {
        for (let c = 0; c < cols; c++) {
          grid.push({
            x: c * CELL + CELL / 2,
            y: r2 * CELL + CELL / 2,
            ch: CHARS[Math.floor(Math.random() * CHARS.length)],
            base: Math.random() * 0.3,
          });
        }
      }
    };
    resize();
    window.addEventListener('resize', resize);

    const onMove = (e) => {
      const r = cv.getBoundingClientRect();
      mouseRef.current = { x: e.clientX - r.left, y: e.clientY - r.top };
    };
    const onLeave = () => { mouseRef.current = { x: -10000, y: -10000 }; };
    cv.addEventListener('mousemove', onMove);
    cv.addEventListener('mouseleave', onLeave);

    ctx.font = `12px "JetBrains Mono", monospace`;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    const accentRGB = isDark ? '93,191,167' : '45,133,115';
    const inkRGB = isDark ? '234,229,216' : '26,24,19';

    let raf;
    let t = 0;
    const loop = () => {
      t += 0.01;
      ctx.clearRect(0, 0, w, h);
      const mx = mouseRef.current.x, my = mouseRef.current.y;
      const RAD = 150, RAD2 = RAD * RAD;
      for (const g of grid) {
        const dx = g.x - mx, dy = g.y - my;
        const d2 = dx * dx + dy * dy;
        const closeness = Math.max(0, 1 - d2 / RAD2);

        const breath = 0.5 + 0.5 * Math.sin(t + g.x * 0.01 + g.y * 0.015);
        const alpha = g.base + closeness * 0.8 + breath * 0.05;
        // Near mouse: accent; far: ink faint
        if (closeness > 0.05) {
          ctx.fillStyle = `rgba(${accentRGB},${alpha})`;
        } else {
          ctx.fillStyle = `rgba(${inkRGB},${alpha * 0.35})`;
        }
        // Swap char sometimes near mouse for "alive" feel
        if (closeness > 0.4 && Math.random() < 0.015) {
          g.ch = CHARS[Math.floor(Math.random() * CHARS.length)];
        }
        ctx.fillText(g.ch, g.x, g.y);
      }
      raf = requestAnimationFrame(loop);
    };
    loop();

    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener('resize', resize);
      cv.removeEventListener('mousemove', onMove);
      cv.removeEventListener('mouseleave', onLeave);
    };
  }, [isDark]);

  return <canvas ref={canvasRef} style={{ position: 'absolute', inset: 0, width: '100%', height: '100%' }}/>;
}

// ─────────────────────────────────────────────────────────────
// Live chat hero — simulates a real turn writing itself
// ─────────────────────────────────────────────────────────────
function LiveChatDemo({ theme, isDark }) {
  const [step, setStep] = React.useState(0);
  const [typed, setTyped] = React.useState('');

  const script = React.useMemo(() => [
    { kind: 'user', content: 'analyze yesterday\'s payment logs, find the anomaly' },
    { kind: 'think', content: 'looking for error clusters around webhook timeouts…', dur: 2200 },
    { kind: 'tool', name: 'read_file', input: '/var/log/payments/2026-04-18.log', dur: 800 },
    { kind: 'tool', name: 'grep', input: 'webhook timeout|retry exhausted', dur: 500 },
    { kind: 'tool', name: 'git_log', input: '--since=yesterday services/payments/', dur: 400 },
    { kind: 'answer', content: 'Found it. Commit `2c1d9e7` made the handler async but kept the 2s timeout. Patch coming up.' },
  ], []);

  React.useEffect(() => {
    if (step >= script.length) {
      const t = setTimeout(() => { setStep(0); setTyped(''); }, 4000);
      return () => clearTimeout(t);
    }
    const cur = script[step];
    if (cur.kind === 'user' || cur.kind === 'answer') {
      if (typed.length < cur.content.length) {
        const t = setTimeout(() => setTyped(cur.content.slice(0, typed.length + 1)), 18 + Math.random() * 25);
        return () => clearTimeout(t);
      } else {
        const t = setTimeout(() => { setStep(step + 1); setTyped(''); }, 900);
        return () => clearTimeout(t);
      }
    } else {
      const t = setTimeout(() => setStep(step + 1), cur.dur || 1000);
      return () => clearTimeout(t);
    }
  }, [step, typed, script]);

  return (
    <div style={{
      fontFamily: '"JetBrains Mono", monospace',
      fontSize: 13, lineHeight: 1.6,
      color: theme.ink,
      minHeight: 380,
    }}>
      {script.slice(0, step).map((s, i) => (
        <ScriptLine key={i} s={s} theme={theme} final />
      ))}
      {step < script.length && (
        <ScriptLine s={script[step]} typed={typed} theme={theme} />
      )}
    </div>
  );
}

function ScriptLine({ s, typed, theme, final }) {
  if (s.kind === 'user') {
    return (
      <div style={{ marginBottom: 14 }}>
        <span style={{ color: theme.inkMuted }}>you ▸ </span>
        <span style={{ color: theme.ink }}>{final ? s.content : typed}</span>
        {!final && <Caret theme={theme} />}
      </div>
    );
  }
  if (s.kind === 'think') {
    return (
      <div style={{
        marginBottom: 14,
        color: theme.inkMuted,
        fontFamily: '"Fraunces", serif', fontStyle: 'italic', fontSize: 13,
      }}>
        <span>⫶ pondering — </span>
        <span>{s.content}</span>
        {!final && <span style={{ marginLeft: 6 }}><Dots theme={theme} /></span>}
      </div>
    );
  }
  if (s.kind === 'tool') {
    return (
      <div style={{
        marginBottom: 8, paddingLeft: 18,
        borderLeft: `2px solid ${final ? theme.accent : theme.amber}`,
        display: 'flex', alignItems: 'center', gap: 10,
      }}>
        <span style={{
          fontSize: 10.5, color: theme.accent,
          background: theme.accentSoft, padding: '1px 7px', borderRadius: 3,
        }}>{s.name}</span>
        <span style={{ color: theme.inkSoft, fontSize: 12 }}>{s.input}</span>
        <span style={{ flex: 1 }}/>
        <span style={{
          fontSize: 10.5, color: final ? theme.green : theme.amber,
        }}>{final ? '✓ done' : '… running'}</span>
      </div>
    );
  }
  if (s.kind === 'answer') {
    return (
      <div style={{ marginTop: 14 }}>
        <span style={{ color: theme.accent }}>⫶ daimon ▸ </span>
        <span style={{
          color: theme.ink,
          fontFamily: '"Inter", system-ui, sans-serif', fontSize: 13.5,
        }}>
          {(final ? s.content : typed).split(/(`[^`]+`)/g).map((part, i) =>
            part.startsWith('`') ? (
              <code key={i} style={{
                fontFamily: '"JetBrains Mono", monospace',
                fontSize: 12, color: theme.amber,
                background: `${theme.amber}15`, padding: '1px 5px', borderRadius: 2,
              }}>{part.slice(1, -1)}</code>
            ) : <span key={i}>{part}</span>
          )}
        </span>
        {!final && <Caret theme={theme} />}
      </div>
    );
  }
  return null;
}

function Caret({ theme }) {
  return <span style={{
    display: 'inline-block', width: 7, height: 14,
    background: theme.accent, verticalAlign: 'middle',
    marginLeft: 2, animation: 'caretBlink 1.1s steps(1) infinite',
  }}/>;
}

function Dots({ theme }) {
  return (
    <span style={{ display: 'inline-flex', gap: 3 }}>
      {[0, 1, 2].map(i => (
        <span key={i} style={{
          width: 3, height: 3, borderRadius: 99, background: theme.accent,
          animation: `glyphBreathe 1.2s ease-in-out infinite ${i * 0.15}s`,
        }}/>
      ))}
    </span>
  );
}

// ─────────────────────────────────────────────────────────────
// Hero
// ─────────────────────────────────────────────────────────────
function TerminalHero({ theme, isDark }) {
  const ref = React.useRef(null);
  return (
    <div ref={ref} style={{
      position: 'relative', minHeight: '88vh',
      padding: '40px 40px 60px',
      display: 'grid', gridTemplateColumns: '1.05fr 1fr',
      gap: 60, alignItems: 'center',
      overflow: 'hidden',
      background: theme.bg,
    }}>
      <AsciiField theme={theme} isDark={isDark} />
      <Grain opacity={isDark ? 0.05 : 0.03} />

      {/* Left — statement */}
      <div style={{ position: 'relative', zIndex: 3, paddingLeft: 40 }}>
        <div style={{
          fontFamily: '"JetBrains Mono", monospace',
          fontSize: 11, color: theme.accent, letterSpacing: 2,
          display: 'flex', alignItems: 'center', gap: 10, marginBottom: 22,
        }}>
          <span style={{
            width: 6, height: 6, borderRadius: 99, background: theme.accent,
            animation: 'glyphBreathe 1.6s ease-in-out infinite',
          }}/>
          <span>READY · v0.4.2 · MIT</span>
        </div>
        <h1 style={{
          margin: 0,
          fontFamily: '"Inter", system-ui, sans-serif',
          fontSize: 'clamp(36px, 5.5vw, 76px)',
          fontWeight: 600, lineHeight: 1.02,
          letterSpacing: -2,
          color: theme.ink,
        }}>
          The agent that runs<br/>
          <span style={{
            fontFamily: '"Fraunces", serif', fontStyle: 'italic',
            color: theme.accent, fontWeight: 400,
          }}>on your hardware.</span>
        </h1>
        <p style={{
          margin: '26px 0 0', maxWidth: 480,
          fontSize: 16, lineHeight: 1.6, color: theme.inkSoft,
        }}>
          Real tools, real memory, real MCP. 40 MB binary. Zero accounts.
          Your data never leaves the process you started.
        </p>
        <div style={{ marginTop: 32, display: 'flex', gap: 10, alignItems: 'center' }}>
          <button style={{
            padding: '12px 20px', borderRadius: 4,
            background: theme.accent, color: isDark ? theme.bg : '#fff',
            border: 'none', cursor: 'pointer',
            fontFamily: '"JetBrains Mono", monospace', fontSize: 13,
            fontWeight: 500,
            display: 'flex', alignItems: 'center', gap: 10,
          }}>
            <span>$ docker run daimon</span>
            <span style={{ opacity: 0.6 }}>↵</span>
          </button>
          <button style={{
            padding: '12px 18px', borderRadius: 4,
            background: 'transparent', color: theme.ink,
            border: `1px solid ${theme.lineStrong}`,
            fontSize: 13, cursor: 'pointer',
            fontFamily: '"Inter", sans-serif', fontWeight: 500,
          }}>Read docs</button>
        </div>

        {/* Stat strip */}
        <div style={{
          marginTop: 44, display: 'flex', gap: 36, flexWrap: 'wrap',
          paddingTop: 24, borderTop: `1px solid ${theme.line}`,
        }}>
          {[
            { v: '40 MB', l: 'binary' },
            { v: '<300ms', l: 'first token' },
            { v: '4.2k', l: 'stars' },
            { v: '128', l: 'contributors' },
          ].map(s => (
            <div key={s.l}>
              <div style={{
                fontFamily: '"JetBrains Mono", monospace',
                fontSize: 22, color: theme.ink, fontWeight: 500,
              }}>{s.v}</div>
              <div style={{
                fontFamily: '"Fraunces", serif', fontStyle: 'italic',
                fontSize: 11.5, color: theme.inkMuted, marginTop: 2,
              }}>— {s.l}</div>
            </div>
          ))}
        </div>
      </div>

      {/* Right — live demo in a terminal frame */}
      <div style={{ position: 'relative', zIndex: 3, paddingRight: 40 }}>
        <div style={{
          background: theme.bgCode,
          borderRadius: 8,
          border: `1px solid ${isDark ? theme.line : 'rgba(0,0,0,0.25)'}`,
          overflow: 'hidden',
          boxShadow: theme.shadow,
        }}>
          {/* Chrome */}
          <div style={{
            padding: '10px 16px', display: 'flex', alignItems: 'center', gap: 10,
            borderBottom: '1px solid rgba(234,229,216,0.08)',
            background: 'rgba(255,255,255,0.02)',
          }}>
            <div style={{
              fontFamily: '"JetBrains Mono", monospace', fontSize: 11,
              color: '#7a7465',
            }}>~/projects/helix · daimon</div>
            <span style={{ flex: 1 }}/>
            <div style={{
              fontFamily: '"JetBrains Mono", monospace', fontSize: 10.5,
              color: '#5dbfa7',
              display: 'flex', alignItems: 'center', gap: 6,
            }}>
              <span style={{
                width: 5, height: 5, borderRadius: 99, background: '#5dbfa7',
                animation: 'glyphBreathe 1.6s ease-in-out infinite',
              }}/>
              live
            </div>
          </div>
          <div style={{ padding: '20px 24px', minHeight: 380 }}>
            <LiveChatDemo theme={{
              ink: '#eae5d8', inkSoft: '#c2bca9', inkMuted: '#7a7465',
              accent: '#5dbfa7', accentSoft: 'rgba(93,191,167,0.12)',
              green: '#7aba8a', amber: '#e3b67a',
            }} isDark />
          </div>
        </div>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Pillars — status badges, terminal vibe
// ─────────────────────────────────────────────────────────────
const PILLARS_T = [
  { k: 'FREE',    v: 'MIT',      sub: 'no seats, no tiers, no billing' },
  { k: 'LOCAL',   v: 'docker',   sub: 'self-host on any machine' },
  { k: 'PRIVATE', v: '0 B out',  sub: 'no telemetry, no accounts' },
  { k: 'LIGHT',   v: '40 MB',    sub: 'cold-start under 300 ms' },
  { k: 'SAFE',    v: 'sandbox',  sub: 'every tool call auditable' },
];

function PillarsT({ theme }) {
  return (
    <SectionT num="01" kicker="STATUS" title="Five properties. All green." theme={theme}>
      <div style={{
        display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
        gap: 0, border: `1px solid ${theme.line}`, borderRadius: 6, overflow: 'hidden',
      }}>
        {PILLARS_T.map((p, i) => (
          <PillarBadge key={p.k} p={p} theme={theme} index={i} />
        ))}
      </div>
    </SectionT>
  );
}

function PillarBadge({ p, theme, index }) {
  const [hover, setHover] = React.useState(false);
  return (
    <Reveal delay={index * 0.05}>
      <div
        onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}
        style={{
          padding: '24px 22px',
          borderRight: `1px solid ${theme.line}`,
          background: hover ? theme.bgElev : 'transparent',
          transition: 'background 0.2s',
          position: 'relative', minHeight: 140,
        }}
      >
        <div style={{
          fontFamily: '"JetBrains Mono", monospace',
          fontSize: 10.5, color: theme.accent, letterSpacing: 2,
          display: 'flex', alignItems: 'center', gap: 8, marginBottom: 14,
        }}>
          <span style={{
            width: 6, height: 6, borderRadius: 99, background: theme.accent,
            boxShadow: `0 0 8px ${theme.accent}`,
          }}/>
          {p.k}
        </div>
        <div style={{
          fontFamily: '"JetBrains Mono", monospace',
          fontSize: 22, color: theme.ink, fontWeight: 500, marginBottom: 8,
          letterSpacing: -0.5,
        }}>{p.v}</div>
        <div style={{
          fontFamily: '"Fraunces", serif', fontStyle: 'italic',
          fontSize: 13, color: theme.inkMuted, lineHeight: 1.4,
        }}>— {p.sub}</div>
      </div>
    </Reveal>
  );
}

// ─────────────────────────────────────────────────────────────
// Section wrapper — terminal-ish with § number
// ─────────────────────────────────────────────────────────────
function SectionT({ num, kicker, title, subtitle, theme, children }) {
  return (
    <section style={{ padding: '100px 40px', maxWidth: 1200, margin: '0 auto' }}>
      <Reveal>
        <div style={{
          display: 'flex', alignItems: 'center', gap: 12,
          fontFamily: '"JetBrains Mono", monospace',
          fontSize: 11, letterSpacing: 2, color: theme.inkMuted,
          marginBottom: 18,
        }}>
          <span style={{ color: theme.accent }}>{num}</span>
          <span style={{ width: 20, height: 1, background: theme.line }}/>
          <span>{kicker}</span>
        </div>
      </Reveal>
      <Reveal delay={0.08}>
        <h2 style={{
          margin: 0, fontSize: 'clamp(28px, 4vw, 44px)',
          fontWeight: 600, lineHeight: 1.1, letterSpacing: -1,
          color: theme.ink, maxWidth: 760,
        }}>{title}</h2>
      </Reveal>
      {subtitle && <Reveal delay={0.15}><p style={{
        margin: '18px 0 0', maxWidth: 580,
        fontSize: 15.5, lineHeight: 1.65, color: theme.inkSoft,
      }}>{subtitle}</p></Reveal>}
      <div style={{ marginTop: 48 }}>{children}</div>
    </section>
  );
}

// ─────────────────────────────────────────────────────────────
// Features — as a "ps" output / manifest
// ─────────────────────────────────────────────────────────────
const FEATURES_T = [
  { tag: 'memory',  v: '3 levels', title: 'Long-term memory, with receipts', desc: 'Daimon tracks what it knows, infers, and assumes — separately. Sources are cited to the exact conversation. You correct what it got wrong.' },
  { tag: 'rag',     v: 'local',    title: 'Knowledge ingestion',              desc: 'Drop PDFs, Markdown, HTML, zips. Daimon chunks and embeds them locally. Every injection into context is visible and traceable.' },
  { tag: 'tools',   v: '40+',      title: 'Real tool execution',              desc: 'Shell, filesystem, HTTP, git, web fetch — sandboxed, logged, reversible. Extend via plugins written in any language.' },
  { tag: 'mcp',     v: 'native',   title: 'Model Context Protocol',           desc: 'Speaks MCP out of the box. Plug in any compatible server — local or networked.' },
  { tag: 'models',  v: 'any',      title: 'Bring your own model',             desc: 'Ollama, LM Studio, llama.cpp, OpenAI, Anthropic, Mistral, Groq, or a custom endpoint. Swap mid-conversation.' },
  { tag: 'stream',  v: 'realtime', title: 'First-class streaming & retry',    desc: 'Token-by-token output, parallel tool calls, graceful retries. Watch the agent think.' },
];

function FeaturesT({ theme, isDark }) {
  return (
    <SectionT num="02" kicker="CAPABILITIES" title="A runtime, not a prompt." theme={theme}>
      <div style={{
        border: `1px solid ${theme.line}`, borderRadius: 6,
        overflow: 'hidden', background: theme.bgElev,
        fontFamily: '"JetBrains Mono", monospace',
      }}>
        <div style={{
          padding: '10px 20px', background: theme.wash,
          borderBottom: `1px solid ${theme.line}`,
          fontSize: 10.5, color: theme.inkMuted, letterSpacing: 1.5,
          display: 'grid', gridTemplateColumns: '90px 100px 1fr', gap: 20,
        }}>
          <span>MODULE</span>
          <span>BUILD</span>
          <span>DESCRIPTION</span>
        </div>
        {FEATURES_T.map((f, i) => (
          <Reveal key={f.tag} delay={i * 0.04}>
            <FeatureRowT f={f} theme={theme} last={i === FEATURES_T.length - 1} />
          </Reveal>
        ))}
      </div>
    </SectionT>
  );
}

function FeatureRowT({ f, theme, last }) {
  const [hover, setHover] = React.useState(false);
  return (
    <div
      onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}
      style={{
        display: 'grid', gridTemplateColumns: '90px 100px 1fr', gap: 20,
        padding: '22px 20px',
        borderBottom: last ? 'none' : `1px solid ${theme.line}`,
        background: hover ? theme.wash : 'transparent',
        alignItems: 'start', cursor: 'default',
      }}
    >
      <div style={{
        fontSize: 11.5, color: theme.accent,
        textTransform: 'uppercase', letterSpacing: 1.5,
      }}>{f.tag}</div>
      <div style={{
        fontSize: 13, color: theme.ink, fontWeight: 500,
        background: theme.accentSoft, border: `1px solid ${theme.accent}33`,
        padding: '2px 10px', borderRadius: 3, display: 'inline-block',
        width: 'fit-content', color: theme.accent,
      }}>{f.v}</div>
      <div>
        <h3 style={{
          margin: 0, fontSize: 15.5, fontWeight: 600, color: theme.ink,
          fontFamily: '"Inter", sans-serif', letterSpacing: -0.2,
          marginBottom: 6,
        }}>{f.title}</h3>
        <p style={{
          margin: 0, fontSize: 13, lineHeight: 1.6, color: theme.inkSoft,
          fontFamily: '"Inter", sans-serif',
        }}>{f.desc}</p>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Compare — terminal diff style
// ─────────────────────────────────────────────────────────────
function CompareT({ theme }) {
  return (
    <SectionT num="03" kicker="DIFF" title="Diffed against the alternatives." subtitle="Daimon is not for everyone. Here is what you trade away." theme={theme}>
      <CompareSection theme={theme} />
    </SectionT>
  );
}

// Reuses the component from landing-daimon.jsx (CompareSection) but strips the chapter wrapper.
// We already have CompareSection on window via landing-daimon.jsx — but it includes its own Chapter.
// So we inline a terminal-styled variant:

function CompareSection_T_Inline({ theme }) {
  // noop — not used, Compare is re-used from A with its own wrapper
}

// ─────────────────────────────────────────────────────────────
// Install — big typewriter
// ─────────────────────────────────────────────────────────────
function InstallT({ theme, isDark }) {
  return (
    <SectionT num="04" kicker="INSTALL" title="One command. Done." theme={theme}>
      <InstallTerminal theme={theme} isDark={isDark} />
    </SectionT>
  );
}

function InstallTerminal({ theme, isDark }) {
  const [tab, setTab] = React.useState('docker');
  const tabs = {
    docker: 'docker run -p 7070:7070 -v ~/.daimon:/data ghcr.io/daimon/daimon:latest',
    curl:   'curl -fsSL https://daimon.dev/install.sh | sh',
    brew:   'brew install daimon-agent/tap/daimon && daimon up',
    npm:    'npx daimon-cli@latest',
  };
  const [typed, setTyped] = React.useState('');
  React.useEffect(() => {
    setTyped('');
    const full = tabs[tab];
    let i = 0;
    const id = setInterval(() => {
      i++;
      setTyped(full.slice(0, i));
      if (i >= full.length) clearInterval(id);
    }, 22);
    return () => clearInterval(id);
  }, [tab]);

  return (
    <div style={{
      background: '#0a0908', borderRadius: 8,
      border: `1px solid rgba(234,229,216,0.1)`,
      overflow: 'hidden', maxWidth: 820, margin: '0 auto',
      boxShadow: theme.shadow,
    }}>
      <div style={{
        display: 'flex', padding: '10px 16px 0',
        borderBottom: '1px solid rgba(234,229,216,0.08)',
        background: 'rgba(255,255,255,0.02)',
      }}>
        {Object.keys(tabs).map(k => (
          <button key={k} onClick={() => setTab(k)} style={{
            padding: '8px 14px', background: 'transparent', border: 'none',
            color: tab === k ? '#5dbfa7' : '#7a7465',
            fontFamily: '"JetBrains Mono", monospace', fontSize: 12,
            cursor: 'pointer',
            borderBottom: tab === k ? '2px solid #5dbfa7' : '2px solid transparent',
            marginBottom: -1,
          }}>{k}</button>
        ))}
      </div>
      <div style={{ padding: '28px 28px 24px', minHeight: 90 }}>
        <div style={{
          fontFamily: '"JetBrains Mono", monospace', fontSize: 13.5,
          lineHeight: 1.8, color: '#c2bca9',
          wordBreak: 'break-all',
        }}>
          <span style={{ color: '#7a7465' }}>$ </span>
          <span>{typed}</span>
          <span style={{
            display: 'inline-block', width: 7, height: 14,
            background: '#5dbfa7', verticalAlign: 'middle',
            marginLeft: 2, animation: 'caretBlink 1.1s steps(1) infinite',
          }}/>
        </div>
        <div style={{
          marginTop: 14, fontFamily: '"Fraunces", serif', fontStyle: 'italic',
          fontSize: 13, color: '#7a7465',
        }}>→ daimon awake at <span style={{ color: '#8ed9c3' }}>http://localhost:7070</span></div>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// CTA
// ─────────────────────────────────────────────────────────────
function FinalCTAT({ theme, isDark }) {
  return (
    <section style={{
      position: 'relative', overflow: 'hidden',
      padding: '100px 40px',
      background: theme.bgCode,
      color: '#eae5d8',
      borderTop: `1px solid ${theme.line}`,
    }}>
      <div style={{
        maxWidth: 900, margin: '0 auto',
        display: 'grid', gridTemplateColumns: '1fr auto', gap: 40,
        alignItems: 'center',
      }}>
        <Reveal>
          <div>
            <div style={{
              fontFamily: '"JetBrains Mono", monospace',
              fontSize: 11, color: '#5dbfa7', letterSpacing: 2, marginBottom: 14,
              display: 'flex', alignItems: 'center', gap: 10,
            }}>
              <span style={{
                width: 6, height: 6, borderRadius: 99, background: '#5dbfa7',
                animation: 'glyphBreathe 1.6s ease-in-out infinite',
              }}/>
              READY · IT'S YOURS FOR THE TAKING
            </div>
            <h2 style={{
              margin: 0,
              fontFamily: '"Inter", sans-serif',
              fontSize: 'clamp(32px, 5vw, 56px)',
              fontWeight: 600, lineHeight: 1.05, letterSpacing: -1.2,
              color: '#eae5d8',
            }}>
              Start the agent.<br/>
              <span style={{
                fontFamily: '"Fraunces", serif', fontStyle: 'italic',
                color: '#5dbfa7', fontWeight: 400,
              }}>Keep the receipts.</span>
            </h2>
          </div>
        </Reveal>
        <Reveal delay={0.1}>
          <div style={{
            display: 'flex', flexDirection: 'column', gap: 10,
          }}>
            <button style={{
              padding: '14px 22px', borderRadius: 4,
              background: '#5dbfa7', color: '#0a0908', border: 'none',
              fontFamily: '"JetBrains Mono", monospace', fontSize: 13.5,
              fontWeight: 600, cursor: 'pointer',
              display: 'flex', alignItems: 'center', gap: 10,
            }}>
              <span>$ daimon up</span>
              <span style={{ opacity: 0.5 }}>↵</span>
            </button>
            <button style={{
              padding: '12px 18px', borderRadius: 4,
              background: 'transparent', color: '#eae5d8',
              border: '1px solid rgba(234,229,216,0.18)',
              fontSize: 13, cursor: 'pointer', fontFamily: 'inherit',
            }}>github.com/daimon · 4.2k ★</button>
          </div>
        </Reveal>
      </div>
    </section>
  );
}

// ─────────────────────────────────────────────────────────────
// Full page
// ─────────────────────────────────────────────────────────────
function LandingTerminal({ theme, isDark, onToggle }) {
  return (
    <div style={{
      background: theme.bg, color: theme.ink, minHeight: '100%',
      fontFamily: '"Inter", system-ui, sans-serif',
      overflow: 'hidden',
    }}>
      <LandingNav theme={theme} isDark={isDark} onToggle={onToggle} direction="terminal" />
      <TerminalHero theme={theme} isDark={isDark} />
      <PillarsT theme={theme} />
      <FeaturesT theme={theme} isDark={isDark} />
      <CompareSection theme={theme} />
      <InstallT theme={theme} isDark={isDark} />
      <FinalCTAT theme={theme} isDark={isDark} />
      <LandingFooter theme={theme} />
    </div>
  );
}

Object.assign(window, { LandingTerminal });
