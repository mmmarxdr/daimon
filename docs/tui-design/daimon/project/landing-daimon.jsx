// Landing A — "The Daimon"
// Editorial-cinema. The hero is a field of particles that flow around the
// wordmark like breath around a candle. Big Fraunces italic typography.
// Every section feels like a chapter.

// ─────────────────────────────────────────────────────────────
// Particle field — breath/smoke around the wordmark
// ─────────────────────────────────────────────────────────────
function ParticleField({ theme, isDark }) {
  const canvasRef = React.useRef(null);
  const mouseRef = React.useRef({ x: 0.5, y: 0.5 });

  React.useEffect(() => {
    const cv = canvasRef.current; if (!cv) return;
    const ctx = cv.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    const PN = 90;
    let particles = [];
    let w = 0, h = 0;

    const resize = () => {
      const r = cv.getBoundingClientRect();
      w = r.width; h = r.height;
      cv.width = w * dpr; cv.height = h * dpr;
      ctx.scale(dpr, dpr);
      ctx.scale = (function(o){ return o; })(ctx.scale); // noop guard
    };

    const init = () => {
      particles = [];
      for (let i = 0; i < PN; i++) {
        particles.push({
          x: Math.random() * w,
          y: Math.random() * h,
          vx: (Math.random() - 0.5) * 0.2,
          vy: -0.1 - Math.random() * 0.2,
          r: 0.6 + Math.random() * 1.6,
          life: Math.random(),
          seed: Math.random() * 1000,
        });
      }
    };

    resize(); init();
    window.addEventListener('resize', () => { resize(); init(); });

    const onMove = (e) => {
      const r = cv.getBoundingClientRect();
      mouseRef.current = {
        x: (e.clientX - r.left) / r.width,
        y: (e.clientY - r.top) / r.height,
      };
    };
    cv.addEventListener('mousemove', onMove);

    const accentRGB = isDark ? '93,191,167' : '45,133,115';
    let t = 0;
    let raf;
    const loop = () => {
      t += 0.005;
      ctx.clearRect(0, 0, w, h);

      // Center of attraction — wordmark is roughly middle
      const cx = w * 0.5, cy = h * 0.45;

      // Draw soft halo behind wordmark
      const g = ctx.createRadialGradient(cx, cy, 0, cx, cy, Math.min(w, h) * 0.5);
      g.addColorStop(0, `rgba(${accentRGB},${isDark ? 0.1 : 0.06})`);
      g.addColorStop(1, 'rgba(0,0,0,0)');
      ctx.fillStyle = g;
      ctx.fillRect(0, 0, w, h);

      // Particles
      const mx = mouseRef.current.x * w;
      const my = mouseRef.current.y * h;
      for (const p of particles) {
        // Drift (flow field approximation: noise-ish sines)
        const angle = Math.sin(p.x * 0.008 + t) * 0.8 + Math.cos(p.y * 0.008 + t * 0.7) * 0.6;
        p.vx += Math.cos(angle) * 0.005;
        p.vy += Math.sin(angle) * 0.005 - 0.003; // rise

        // Mouse repel (gentle)
        const dx = p.x - mx, dy = p.y - my;
        const d2 = dx * dx + dy * dy;
        if (d2 < 22000) {
          const f = (22000 - d2) / 22000 * 0.04;
          p.vx += (dx / Math.sqrt(d2 + 1)) * f;
          p.vy += (dy / Math.sqrt(d2 + 1)) * f;
        }

        // Slight attraction to wordmark center
        const cdx = cx - p.x, cdy = cy - p.y;
        p.vx += cdx * 0.00005;
        p.vy += cdy * 0.00005;

        // Damping
        p.vx *= 0.97; p.vy *= 0.97;

        p.x += p.vx; p.y += p.vy;
        p.life += 0.003;

        if (p.x < -20 || p.x > w + 20 || p.y < -20 || p.y > h + 20 || p.life > 1) {
          p.x = cx + (Math.random() - 0.5) * w * 0.3;
          p.y = cy + Math.random() * h * 0.3;
          p.vx = (Math.random() - 0.5) * 0.2;
          p.vy = -0.1 - Math.random() * 0.2;
          p.life = 0;
        }

        const alpha = Math.sin(p.life * Math.PI) * (isDark ? 0.45 : 0.3);
        ctx.fillStyle = `rgba(${accentRGB},${alpha})`;
        ctx.beginPath();
        ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2);
        ctx.fill();
      }
      raf = requestAnimationFrame(loop);
    };
    loop();

    return () => {
      cancelAnimationFrame(raf);
      cv.removeEventListener('mousemove', onMove);
    };
  }, [isDark]);

  return (
    <canvas ref={canvasRef} style={{
      position: 'absolute', inset: 0, width: '100%', height: '100%',
      pointerEvents: 'none',
    }}/>
  );
}

// ─────────────────────────────────────────────────────────────
// Hero — big editorial statement
// ─────────────────────────────────────────────────────────────
function DaimonHeroA({ theme, isDark }) {
  const heroRef = React.useRef(null);
  const { x, y } = useMousePos(heroRef);

  return (
    <div ref={heroRef} style={{
      position: 'relative', minHeight: '88vh',
      padding: '60px 40px 80px',
      display: 'flex', flexDirection: 'column',
      alignItems: 'center', justifyContent: 'center',
      overflow: 'hidden',
      background: theme.bg,
    }}>
      <ParticleField theme={theme} isDark={isDark} />
      <Grain opacity={isDark ? 0.06 : 0.04} />

      {/* Tagline label */}
      <div style={{
        position: 'relative', zIndex: 3,
        fontFamily: '"JetBrains Mono", monospace',
        fontSize: 11, letterSpacing: 2.5,
        textTransform: 'uppercase',
        color: theme.inkMuted,
        marginBottom: 48,
        display: 'flex', alignItems: 'center', gap: 12,
      }}>
        <span style={{ width: 24, height: 1, background: theme.lineStrong }}/>
        <span>v0.4 · MIT · self-hosted</span>
        <span style={{ width: 24, height: 1, background: theme.lineStrong }}/>
      </div>

      {/* Wordmark alive */}
      <div style={{ position: 'relative', zIndex: 3, marginBottom: 40 }}>
        <WordmarkAlive theme={theme} size={80} mouseX={x} mouseY={y} />
      </div>

      {/* Headline — huge Fraunces italic */}
      <h1 style={{
        position: 'relative', zIndex: 3,
        margin: 0, textAlign: 'center',
        fontFamily: '"Fraunces", Georgia, serif',
        fontSize: 'clamp(40px, 7vw, 104px)',
        fontWeight: 400, lineHeight: 0.95,
        letterSpacing: -2.5,
        color: theme.ink,
        maxWidth: 1100,
      }}>
        An agent
        <br/>
        <span style={{ fontStyle: 'italic', color: theme.accent, fontWeight: 300 }}>who listens,</span>
        <br/>
        and is yours
        <span style={{
          display: 'inline-block', marginLeft: 10,
          width: '0.04em', height: '0.78em', background: theme.accent,
          verticalAlign: '-0.05em', animation: 'caretBlink 1.1s steps(1) infinite',
        }}/>
      </h1>

      {/* Subhead */}
      <p style={{
        position: 'relative', zIndex: 3,
        marginTop: 36, maxWidth: 580, textAlign: 'center',
        fontSize: 16.5, lineHeight: 1.6, color: theme.inkSoft,
        fontFamily: '"Inter", system-ui, sans-serif',
      }}>
        Daimon is a personal AI agent you run on your own machine. No cloud,
        no accounts, no telemetry. Memory that is yours, tools that are real,
        and a voice that <span style={{
          fontFamily: '"Fraunces", serif', fontStyle: 'italic', color: theme.ink,
        }}>stays inside your walls</span>.
      </p>

      {/* CTAs */}
      <div style={{
        position: 'relative', zIndex: 3, marginTop: 40,
        display: 'flex', gap: 12, alignItems: 'center',
      }}>
        <button style={{
          padding: '13px 22px', borderRadius: 4,
          background: theme.ink, color: theme.bg, border: 'none',
          fontSize: 14, fontWeight: 500, cursor: 'pointer',
          fontFamily: '"Inter", sans-serif',
          display: 'flex', alignItems: 'center', gap: 8,
        }}>
          <span>Summon Daimon</span>
          <span style={{ fontFamily: '"Fraunces", serif', fontStyle: 'italic' }}>—</span>
          <span>self-host in 1 command</span>
        </button>
        <button style={{
          padding: '13px 20px', borderRadius: 4,
          background: 'transparent', color: theme.ink,
          border: `1px solid ${theme.lineStrong}`,
          fontSize: 14, fontWeight: 500, cursor: 'pointer',
          fontFamily: '"Inter", sans-serif',
        }}>View the source →</button>
      </div>

      {/* Floor — breath line */}
      <div style={{
        position: 'absolute', left: 0, right: 0, bottom: 0,
        height: 1, background: `linear-gradient(to right, transparent, ${theme.lineStrong}, transparent)`,
        zIndex: 2,
      }}/>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Section — editorial chapter wrapper with optional background anim
// ─────────────────────────────────────────────────────────────
function Chapter({ num, kicker, title, subtitle, theme, children, accent, bg, sectionBg, divider, clip = true }) {
  return (
    <section style={{
      position: 'relative',
      background: sectionBg || 'transparent',
      borderTop: divider ? `1px solid ${theme.line}` : 'none',
      overflow: clip ? 'hidden' : 'visible',
    }}>
      {bg}
      <div style={{
        position: 'relative', zIndex: 2,
        padding: '120px 40px',
        maxWidth: 1200, margin: '0 auto',
      }}>
        <Reveal>
          <div style={{
            fontFamily: '"JetBrains Mono", monospace',
            fontSize: 11, letterSpacing: 2.5,
            textTransform: 'uppercase', color: theme.inkMuted,
            marginBottom: 18, display: 'flex', alignItems: 'center', gap: 14,
          }}>
            <span style={{ color: accent ? theme.accent : theme.inkMuted }}>§ {num}</span>
            <span style={{ width: 20, height: 1, background: theme.line }}/>
            <span>{kicker}</span>
          </div>
        </Reveal>
        <Reveal delay={0.08}>
          <h2 style={{
            margin: 0, maxWidth: 880,
            fontFamily: '"Fraunces", Georgia, serif',
            fontSize: 'clamp(32px, 5vw, 56px)',
            fontWeight: 400, lineHeight: 1.02,
            letterSpacing: -1,
            color: theme.ink,
          }}>{title}</h2>
        </Reveal>
        {subtitle && (
          <Reveal delay={0.16}>
            <p style={{
              margin: '20px 0 0', maxWidth: 620,
              fontSize: 16, lineHeight: 1.65, color: theme.inkSoft,
              fontFamily: '"Inter", system-ui, sans-serif',
            }}>{subtitle}</p>
          </Reveal>
        )}
        <div style={{ marginTop: 56 }}>{children}</div>
      </div>
    </section>
  );
}

// ─────────────────────────────────────────────────────────────
// Pillars — 5 strengths, staggered editorial layout (not boring grid)
// ─────────────────────────────────────────────────────────────
const PILLARS = [
  {
    k: 'free',
    italic: 'costs nothing',
    title: 'Free. Forever.',
    body: 'MIT-licensed. No seats, no tiers, no billing emails. Use it at home, at work, inside your company. The license doesn\'t bend.',
    glyph: '◯',
  },
  {
    k: 'self',
    italic: 'lives in your walls',
    title: 'Self-hostable.',
    body: 'One Docker image. Run it on a laptop, a Raspberry Pi, your homelab, or a company VM. Daimon never needs the internet to think.',
    glyph: '◐',
  },
  {
    k: 'private',
    italic: 'forgets nothing, tells no one',
    title: 'Private by default.',
    body: 'No telemetry, no analytics, no accounts. Your conversations, memories, and files never leave the machine you chose.',
    glyph: '◉',
  },
  {
    k: 'light',
    italic: 'moves like paper',
    title: 'Featherweight.',
    body: '~40 MB binary. Cold-starts under 300 ms. Runs on modest hardware. Daimon is software, not a fleet of microservices.',
    glyph: '◌',
  },
  {
    k: 'secure',
    italic: 'holds itself accountable',
    title: 'Secure by design.',
    body: 'Sandboxed tool execution, signed releases, reproducible builds, auditable memory. Every action Daimon takes is logged and reversible.',
    glyph: '◍',
  },
];

function PillarsSection({ theme, isDark }) {
  return (
    <Chapter
      num="i"
      kicker="The promises"
      title={<>Five promises. <span style={{ fontStyle: 'italic', color: theme.accent }}>Unbendable.</span></>}
      subtitle="Daimon makes a small number of commitments and keeps them. Everything else follows."
      theme={theme}
      accent
      sectionBg={theme.bg}
      bg={<BgMagneticLines theme={theme} isDark={isDark} />}
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
        {PILLARS.map((p, i) => (
          <Reveal key={p.k} delay={i * 0.06}>
            <PillarRow pillar={p} theme={theme} index={i} />
          </Reveal>
        ))}
      </div>
    </Chapter>
  );
}

function PillarRow({ pillar, theme, index }) {
  const [hover, setHover] = React.useState(false);
  return (
    <div
      onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}
      style={{
        display: 'grid',
        gridTemplateColumns: '80px 1fr 1.5fr',
        alignItems: 'baseline', gap: 32,
        padding: '32px 0',
        borderTop: `1px solid ${theme.line}`,
        borderBottom: index === PILLARS.length - 1 ? `1px solid ${theme.line}` : 'none',
        cursor: 'default', position: 'relative',
      }}
    >
      {/* Accent bar on hover */}
      <div style={{
        position: 'absolute', left: 0, top: 0, bottom: 0, width: 2,
        background: theme.accent,
        transform: hover ? 'scaleY(1)' : 'scaleY(0)',
        transformOrigin: 'top',
        transition: 'transform 0.4s cubic-bezier(0.2,0.8,0.2,1)',
      }}/>

      <div style={{
        fontFamily: '"JetBrains Mono", monospace',
        fontSize: 32, color: theme.accent, lineHeight: 1,
        transform: hover ? 'scale(1.15)' : 'scale(1)',
        transition: 'transform 0.5s cubic-bezier(0.2,0.8,0.2,1)',
      }}>{pillar.glyph}</div>

      <div>
        <div style={{
          fontFamily: '"Fraunces", serif', fontStyle: 'italic',
          fontSize: 14, color: theme.inkMuted, marginBottom: 6,
        }}>— {pillar.italic}</div>
        <h3 style={{
          margin: 0,
          fontFamily: '"Fraunces", Georgia, serif',
          fontSize: 32, fontWeight: 400, lineHeight: 1.05,
          letterSpacing: -0.6, color: theme.ink,
        }}>{pillar.title}</h3>
      </div>

      <p style={{
        margin: 0, paddingTop: 6,
        fontSize: 15.5, lineHeight: 1.65, color: theme.inkSoft,
        fontFamily: '"Inter", system-ui, sans-serif',
        maxWidth: 520,
      }}>{pillar.body}</p>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Features — technical capabilities as editorial cards
// ─────────────────────────────────────────────────────────────
const FEATURES = [
  {
    tag: 'memory',
    title: 'Long-term memory, with receipts.',
    body: 'Daimon remembers what you tell it, tracks its own confidence, and cites the conversation it learned from. You can correct what it got wrong.',
    stat: '3 levels', statLabel: 'of trust (I know / I infer / I assume)',
  },
  {
    tag: 'rag',
    title: 'Knowledge that becomes yours.',
    body: 'Drop a PDF, a Markdown folder, or a zip — Daimon converts, chunks, and embeds it locally. Your documents stay on disk. Injection is explicit and visible.',
    stat: '100%', statLabel: 'local embeddings, no API calls',
  },
  {
    tag: 'tools',
    title: 'Tools that actually touch the world.',
    body: 'Shell, file IO, web fetch, git, HTTP — sandboxed and auditable. Every call shows its input, output, and duration. Nothing is hidden from you.',
    stat: '40+', statLabel: 'built-in tools, extensible via plugins',
  },
  {
    tag: 'mcp',
    title: 'MCP-native. Bring your own brain.',
    body: 'Speak Model Context Protocol out of the box. Plug in any compatible server — your database, your internal APIs, your team\'s knowledge base.',
    stat: 'any', statLabel: 'MCP server, local or networked',
  },
  {
    tag: 'models',
    title: 'Any model. Any provider. Or none.',
    body: 'Run Llama or Qwen locally via Ollama, or connect to OpenAI, Anthropic, Mistral, Groq — or a URL you choose. Swap mid-conversation.',
    stat: '1 line', statLabel: 'to change provider',
  },
  {
    tag: 'stream',
    title: 'Streaming, reasoning, retry — all first class.',
    body: 'See Daimon think before it speaks. Watch tools run in real time. Recover gracefully when something fails. The protocol is its own UI.',
    stat: '<300ms', statLabel: 'first token, local models',
  },
];

function FeaturesSection({ theme, isDark }) {
  return (
    <Chapter
      num="ii"
      kicker="Capabilities"
      title={<>What it <span style={{ fontStyle: 'italic', color: theme.accent }}>actually</span> does.</>}
      subtitle="Not a demo. Not a prompt trick. A runtime with memory, tools, and the full weight of modern agent protocols — on your own hardware."
      theme={theme}
      sectionBg={theme.bgDeep}
      bg={<BgConstellation theme={theme} isDark={isDark} />}
      divider
    >
      <div style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fill, minmax(340px, 1fr))',
        gap: 24,
      }}>
        {FEATURES.map((f, i) => (
          <Reveal key={f.tag} delay={(i % 3) * 0.08}>
            <FeatureCard feature={f} theme={theme} />
          </Reveal>
        ))}
      </div>
    </Chapter>
  );
}

function FeatureCard({ feature, theme }) {
  const [hover, setHover] = React.useState(false);
  return (
    <div
      onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}
      style={{
        padding: 24,
        background: theme.bgElev,
        border: `1px solid ${hover ? theme.lineStrong : theme.line}`,
        borderRadius: 6,
        display: 'flex', flexDirection: 'column', gap: 16,
        position: 'relative', overflow: 'hidden',
        transition: 'border-color 0.2s, transform 0.4s cubic-bezier(0.2,0.8,0.2,1)',
        transform: hover ? 'translateY(-2px)' : 'none',
        minHeight: 260,
      }}
    >
      <div style={{
        fontFamily: '"JetBrains Mono", monospace',
        fontSize: 10.5, color: theme.accent,
        textTransform: 'uppercase', letterSpacing: 2,
      }}>{feature.tag}</div>
      <h3 style={{
        margin: 0,
        fontFamily: '"Fraunces", serif',
        fontSize: 22, fontWeight: 400, lineHeight: 1.2,
        letterSpacing: -0.4, color: theme.ink,
      }}>{feature.title}</h3>
      <p style={{
        margin: 0, flex: 1,
        fontSize: 13.5, lineHeight: 1.6, color: theme.inkSoft,
      }}>{feature.body}</p>
      <div style={{
        paddingTop: 14, borderTop: `1px solid ${theme.line}`,
        display: 'flex', alignItems: 'baseline', gap: 10,
      }}>
        <span style={{
          fontFamily: '"Fraunces", serif',
          fontSize: 28, fontWeight: 500, color: theme.accent,
          lineHeight: 1,
        }}>{feature.stat}</span>
        <span style={{
          fontFamily: '"Fraunces", serif', fontStyle: 'italic',
          fontSize: 12.5, color: theme.inkMuted,
        }}>— {feature.statLabel}</span>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Comparison — rendered as prose-ish table
// ─────────────────────────────────────────────────────────────
const COMPARISON = {
  rows: [
    { label: 'runs on your machine',       daimon: 'yes', gpt: 'no',      claude: 'no',      local: 'yes' },
    { label: 'your data stays with you',   daimon: 'yes', gpt: 'no',      claude: 'no',      local: 'yes' },
    { label: 'memory across sessions',     daimon: 'yes', gpt: 'partial', claude: 'partial', local: 'no' },
    { label: 'tool execution',             daimon: 'yes', gpt: 'yes',     claude: 'yes',     local: 'no' },
    { label: 'MCP support',                daimon: 'yes', gpt: 'no',      claude: 'yes',     local: 'no' },
    { label: 'free, forever',              daimon: 'yes', gpt: 'no',      claude: 'no',      local: 'yes' },
    { label: 'swap models mid-conversation', daimon: 'yes', gpt: 'no',    claude: 'no',      local: 'partial' },
    { label: 'open source',                daimon: 'yes', gpt: 'no',      claude: 'no',      local: 'varies' },
  ],
};

function CompareSection({ theme, isDark }) {
  return (
    <Chapter
      num="iii"
      kicker="The landscape"
      title={<>Where Daimon <span style={{ fontStyle: 'italic', color: theme.accent }}>sits</span>.</>}
      subtitle="Honest comparison. Daimon isn't trying to replace ChatGPT — it's for people who want a different set of trade-offs."
      theme={theme}
      sectionBg={theme.bg}
      bg={<BgAurora theme={theme} isDark={isDark} />}
    >
      <Reveal>
        <div style={{
          background: theme.bgElev,
          border: `1px solid ${theme.line}`,
          borderRadius: 6,
          overflow: 'hidden',
          fontFamily: '"Inter", system-ui, sans-serif',
        }}>
          {/* Header */}
          <div style={{
            display: 'grid', gridTemplateColumns: '2fr 1fr 1fr 1fr 1fr',
            padding: '16px 20px',
            background: theme.wash,
            borderBottom: `1px solid ${theme.line}`,
            fontSize: 11, letterSpacing: 1.2, textTransform: 'uppercase',
            color: theme.inkMuted, fontFamily: '"JetBrains Mono", monospace',
          }}>
            <div></div>
            <div style={{
              color: theme.accent, fontWeight: 600, fontSize: 12.5,
              fontFamily: '"Inter", sans-serif', textTransform: 'none', letterSpacing: -0.2,
            }}>Daimon</div>
            <div>ChatGPT</div>
            <div>Claude.ai</div>
            <div>Local LLM</div>
          </div>
          {COMPARISON.rows.map((r, i) => (
            <CompareRow key={i} row={r} theme={theme} />
          ))}
        </div>
      </Reveal>
    </Chapter>
  );
}

function CompareRow({ row, theme }) {
  const cells = [
    { v: row.daimon, emphasis: true },
    { v: row.gpt },
    { v: row.claude },
    { v: row.local },
  ];
  return (
    <div style={{
      display: 'grid', gridTemplateColumns: '2fr 1fr 1fr 1fr 1fr',
      padding: '14px 20px',
      borderBottom: `1px solid ${theme.line}`,
      alignItems: 'center',
      fontSize: 13.5, color: theme.ink,
    }}>
      <div style={{
        fontFamily: '"Fraunces", serif', fontStyle: 'italic',
        color: theme.inkSoft, fontSize: 14.5,
      }}>{row.label}</div>
      {cells.map((c, i) => <CompareCell key={i} {...c} theme={theme} />)}
    </div>
  );
}

function CompareCell({ v, emphasis, theme }) {
  const colors = {
    yes:     { bg: emphasis ? theme.accent : `${theme.accent}22`, fg: emphasis ? theme.bg : theme.accent, text: 'yes' },
    no:      { bg: 'transparent', fg: theme.inkFaint, text: 'no' },
    partial: { bg: `${theme.amber}18`, fg: theme.amber, text: 'partial' },
    varies:  { bg: theme.line, fg: theme.inkMuted, text: 'varies' },
  };
  const c = colors[v] || colors.no;
  return (
    <div>
      <span style={{
        display: 'inline-block', padding: '3px 10px', borderRadius: 99,
        background: c.bg, color: c.fg,
        fontSize: 11.5, fontWeight: emphasis && v === 'yes' ? 600 : 400,
        border: v === 'no' ? `1px solid ${theme.line}` : 'none',
        fontFamily: '"JetBrains Mono", monospace',
      }}>{c.text}</span>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Install — terminal block, typewriter
// ─────────────────────────────────────────────────────────────
function InstallSection({ theme, isDark }) {
  const [tab, setTab] = React.useState('docker');
  const snippets = {
    docker: [
      { c: '$ ', t: 'plain' },
      { c: 'docker run -p 7070:7070 -v ~/.daimon:/data ', t: 'cmd' },
      { c: 'ghcr.io/daimon/daimon:latest\n', t: 'path' },
      { c: '\n', t: 'plain' },
      { c: '→ daimon awake at ', t: 'muted' },
      { c: 'http://localhost:7070', t: 'link' },
    ],
    curl: [
      { c: '$ ', t: 'plain' },
      { c: 'curl -fsSL ', t: 'cmd' },
      { c: 'https://daimon.dev/install.sh', t: 'path' },
      { c: ' | sh\n', t: 'plain' },
      { c: '\n', t: 'plain' },
      { c: '→ installing to ', t: 'muted' },
      { c: '~/.daimon', t: 'link' },
    ],
    brew: [
      { c: '$ ', t: 'plain' },
      { c: 'brew install ', t: 'cmd' },
      { c: 'daimon-agent/tap/daimon\n', t: 'path' },
      { c: '\n', t: 'plain' },
      { c: '→ then run ', t: 'muted' },
      { c: 'daimon up', t: 'link' },
    ],
  };

  const colorOf = (t) => ({
    plain: '#c2bca9', cmd: '#e3b67a', path: '#5dbfa7', muted: '#7a7465', link: '#8ed9c3',
  })[t] || '#c2bca9';

  return (
    <Chapter
      num="iv"
      kicker="Install"
      title={<>One command. <span style={{ fontStyle: 'italic', color: theme.accent }}>That's the whole ritual.</span></>}
      subtitle="Daimon ships as a single binary and a single image. No services to wire, no databases to provision."
      theme={theme}
      sectionBg={theme.wash}
      bg={<BgScanlines theme={theme} isDark={isDark} />}
      divider
    >
      <Reveal>
        <div style={{
          background: '#0a0908',
          borderRadius: 8,
          border: `1px solid ${isDark ? theme.line : 'rgba(0,0,0,0.3)'}`,
          overflow: 'hidden',
          maxWidth: 820, margin: '0 auto',
          boxShadow: theme.shadow,
        }}>
          {/* Tabs */}
          <div style={{
            display: 'flex', gap: 2,
            padding: '10px 16px 0',
            borderBottom: `1px solid rgba(234,229,216,0.08)`,
            background: 'rgba(255,255,255,0.015)',
            alignItems: 'center',
          }}>
            {['docker', 'curl', 'brew'].map(k => (
              <button key={k} onClick={() => setTab(k)} style={{
                padding: '8px 16px', background: 'transparent', border: 'none',
                color: tab === k ? '#5dbfa7' : '#7a7465',
                fontFamily: '"JetBrains Mono", monospace', fontSize: 12,
                cursor: 'pointer', borderBottom: tab === k ? '2px solid #5dbfa7' : '2px solid transparent',
                marginBottom: -1,
              }}>{k}</button>
            ))}
            <span style={{ flex: 1 }}/>
            <span style={{
              fontFamily: '"Fraunces", serif', fontStyle: 'italic',
              color: '#7a7465', fontSize: 12, paddingBottom: 10,
            }}>macOS, Linux, Windows (WSL)</span>
          </div>

          {/* Code */}
          <pre style={{
            margin: 0, padding: '24px 28px',
            fontFamily: '"JetBrains Mono", monospace',
            fontSize: 13.5, lineHeight: 1.75,
            color: '#c2bca9',
            whiteSpace: 'pre-wrap',
          }}>
            {snippets[tab].map((s, i) => (
              <span key={i} style={{ color: colorOf(s.t) }}>{s.c}</span>
            ))}
          </pre>
        </div>
      </Reveal>

      <Reveal delay={0.15}>
        <div style={{
          marginTop: 32, textAlign: 'center',
          fontFamily: '"Fraunces", serif', fontStyle: 'italic',
          fontSize: 15, color: theme.inkMuted,
        }}>
          Or read <a href="#" style={{ color: theme.accent }}>the docs</a> · <a href="#" style={{ color: theme.accent }}>browse the source</a> · <a href="#" style={{ color: theme.accent }}>join the room</a>
        </div>
      </Reveal>
    </Chapter>
  );
}

// ─────────────────────────────────────────────────────────────
// CTA — final editorial close. Rings bleed UP into the Install section
// so the two animations "meet". No hard border — gradient fade instead.
// ─────────────────────────────────────────────────────────────
function FinalCTA({ theme, isDark }) {
  const ref = React.useRef(null);
  const { x, y } = useMousePos(ref);
  return (
    <section ref={ref} style={{
      position: 'relative', overflow: 'visible',
      padding: '140px 40px 120px',
      background: theme.wash,
      // No border — a gradient at the very top instead, fading from Install's wash to CTA's wash
    }}>
      {/* Soft divider: thin glow line instead of hard border */}
      <div aria-hidden style={{
        position: 'absolute', left: 0, right: 0, top: 0, height: 1,
        background: `linear-gradient(to right, transparent, ${theme.accent}55, transparent)`,
        zIndex: 3,
      }}/>
      {/* Bleed container — rings can extend above the section */}
      <div aria-hidden style={{
        position: 'absolute', inset: 0,
        overflow: 'visible', pointerEvents: 'none',
      }}>
        <BgPulseRings theme={theme} isDark={isDark} />
      </div>

      <Reveal>
        <div style={{
          position: 'relative', zIndex: 2,
          maxWidth: 900, margin: '0 auto', textAlign: 'center',
        }}>
          <WordmarkAlive theme={theme} size={64} mouseX={x} mouseY={y} />
          <h2 style={{
            margin: '32px 0 0',
            fontFamily: '"Fraunces", Georgia, serif',
            fontSize: 'clamp(32px, 6vw, 72px)',
            fontWeight: 400, lineHeight: 1, letterSpacing: -1.5,
            color: theme.ink,
          }}>
            An agent <span style={{ fontStyle: 'italic', color: theme.accent }}>of your own.</span>
          </h2>
          <p style={{
            margin: '24px auto 0', maxWidth: 540,
            fontFamily: '"Fraunces", serif', fontStyle: 'italic',
            fontSize: 17, lineHeight: 1.55, color: theme.inkSoft,
          }}>
            Take the hour. Stand up Daimon on something you already own.
            Teach it a few things. See how it feels to have an agent that
            nobody else can read.
          </p>
          <div style={{
            marginTop: 36, display: 'flex', justifyContent: 'center', gap: 12,
          }}>
            <button style={{
              padding: '14px 28px', borderRadius: 4,
              background: theme.ink, color: theme.bg, border: 'none',
              fontSize: 14, fontWeight: 500, cursor: 'pointer',
              fontFamily: '"Inter", sans-serif',
            }}>Download Daimon</button>
            <button style={{
              padding: '14px 24px', borderRadius: 4,
              background: 'transparent', color: theme.ink,
              border: `1px solid ${theme.lineStrong}`,
              fontSize: 14, fontWeight: 500, cursor: 'pointer',
              fontFamily: '"Inter", sans-serif',
            }}>Star on GitHub · 4.2k</button>
          </div>
        </div>
      </Reveal>
    </section>
  );
}

// ─────────────────────────────────────────────────────────────
// Footer
// ─────────────────────────────────────────────────────────────
function LandingFooter({ theme }) {
  return (
    <footer style={{
      padding: '40px 40px 60px',
      borderTop: `1px solid ${theme.line}`,
      background: theme.bg,
      fontFamily: '"Inter", system-ui, sans-serif',
      fontSize: 12.5, color: theme.inkMuted,
      display: 'flex', alignItems: 'center', gap: 24, flexWrap: 'wrap',
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <span style={{ fontFamily: '"JetBrains Mono", monospace', color: theme.accent, fontSize: 16 }}>⫶</span>
        <span style={{ color: theme.ink, fontWeight: 500 }}>Daimon</span>
        <span style={{ fontFamily: '"Fraunces", serif', fontStyle: 'italic' }}>— v0.4.2 · MIT</span>
      </div>
      <span style={{ flex: 1 }}/>
      <div style={{ display: 'flex', gap: 22 }}>
        {['Docs', 'GitHub', 'Discord', 'Changelog', 'Privacy (none)'].map(l => (
          <a key={l} href="#" style={{ color: 'inherit', textDecoration: 'none' }}>{l}</a>
        ))}
      </div>
    </footer>
  );
}

// ─────────────────────────────────────────────────────────────
// Full page
// ─────────────────────────────────────────────────────────────
function LandingDaimon({ theme, isDark, onToggle }) {
  return (
    <div style={{
      background: theme.bg, color: theme.ink, minHeight: '100%',
      fontFamily: '"Inter", system-ui, sans-serif',
    }}>
      <BgKeyframes />
      <LandingNav theme={theme} isDark={isDark} onToggle={onToggle} direction="daimon" />
      <DaimonHeroA theme={theme} isDark={isDark} />
      <PillarsSection theme={theme} isDark={isDark} />
      <FeaturesSection theme={theme} isDark={isDark} />
      <CompareSection theme={theme} isDark={isDark} />
      <InstallSection theme={theme} isDark={isDark} />
      <FinalCTA theme={theme} isDark={isDark} />
      <LandingFooter theme={theme} />
    </div>
  );
}

Object.assign(window, { LandingDaimon });
