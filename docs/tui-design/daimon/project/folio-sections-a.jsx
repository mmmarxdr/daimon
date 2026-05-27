// Studio Δ — Folios 00 to 03 (Cover, Manifesto, Capabilities, Method)

// ─────────────────────────────────────────────────────────────
// Folio 00 — COVER
// ─────────────────────────────────────────────────────────────
function FolioCover({ theme, scrollerRef, animLevel, paperFall }) {
  const ref = React.useRef(null);
  const wmRef = React.useRef(null);
  const seen = useInView(ref, 0.05);
  const mouse = useMousePos(ref);
  const p = useScrollProgress(ref, scrollerRef);

  // Wordmark "compose" timing
  const composed = seen;
  const stages = [composed, composed, composed]; // Top dot, bar, bottom dot

  // Parallax
  const parY = p * 80;

  return (
    <section ref={ref} data-screen-label="00 Cover" style={{
      position: 'relative',
      minHeight: 880,
      padding: '60px 64px 120px',
      borderBottom: `1px solid ${theme.line}`,
      overflow: 'hidden',
    }}>
      {/* Paper falling behind everything */}
      {paperFall && <PaperFall theme={theme} intensity={animLevel} />}

      {/* Top header strip — masthead */}
      <div className="folio-masthead" style={{
        display: 'flex', alignItems: 'baseline', gap: 18,
        fontFamily: '"JetBrains Mono", monospace',
        fontSize: 11, letterSpacing: 0.6, textTransform: 'uppercase',
        color: theme.inkMuted,
        paddingBottom: 14,
        borderBottom: `1px solid ${theme.line}`,
        position: 'relative', zIndex: 2,
      }}>
        <span style={{ fontFamily: '"Fraunces", serif', fontStyle: 'italic', fontSize: 14, color: theme.ink, letterSpacing: 0 }}>
          Studio Δ
        </span>
        <span>· Vol. 01 · Issue Nº 01 · MIT</span>
        <span style={{ flex: 1, height: 1, background: theme.line, transform: 'translateY(-3px)' }}/>
        <span>· Local · Private · Open</span>
        <span>· {new Date().toLocaleDateString('en-US', { month: 'short', year: 'numeric' }).toUpperCase()}</span>
      </div>

      {/* Sub-masthead row: section nav */}
      <div className="folio-subnav" style={{
        display: 'flex', alignItems: 'center', gap: 22,
        paddingTop: 14, paddingBottom: 28,
        fontFamily: '"Inter", system-ui',
        fontSize: 12.5, color: theme.inkSoft,
        position: 'relative', zIndex: 2,
      }}>
        <a href="#manifesto" style={{ color: 'inherit', textDecoration: 'none' }}>Manifesto</a>
        <a href="#capabilities" style={{ color: 'inherit', textDecoration: 'none' }}>Capabilities</a>
        <a href="#method" style={{ color: 'inherit', textDecoration: 'none' }}>Method</a>
        <a href="#field" style={{ color: 'inherit', textDecoration: 'none' }}>Field</a>
        <a href="#works" style={{ color: 'inherit', textDecoration: 'none' }}>Works</a>
        <a href="#contact" style={{ color: 'inherit', textDecoration: 'none' }}>Contact</a>
        <span style={{ flex: 1 }}/>
        <span style={{ fontFamily: '"JetBrains Mono", monospace', fontSize: 11, color: theme.inkMuted }}>
          ● Live · v0.3.0
        </span>
      </div>

      {/* Main hero grid */}
      <div className="folio-cover-hero" style={{
        display: 'grid',
        gridTemplateColumns: '1fr 1.05fr',
        gap: 56,
        alignItems: 'start',
        marginTop: 24,
        position: 'relative', zIndex: 2,
        transform: `translateY(${-parY * 0.3}px)`,
      }}>
        {/* Left: kicker + title */}
        <div>
          <div style={{
            display: 'flex', alignItems: 'baseline', gap: 12,
            fontFamily: '"Fraunces", serif', fontStyle: 'italic',
            fontSize: 14, color: theme.inkMuted,
            marginBottom: 36,
          }}>
            <span style={{ color: theme.rubric }}>·</span>
            <span>Personal-agent operating system · Folio Nº 01</span>
          </div>

          <h1 className="folio-cover-title" style={{
            margin: 0,
            fontFamily: '"Fraunces", serif',
            fontWeight: 300,
            fontSize: 132,
            lineHeight: 0.92,
            letterSpacing: -3.5,
            color: theme.ink,
            textWrap: 'balance',
          }}>
            <CharStagger threshold={0.05}>An agent of </CharStagger>
            <em style={{ fontWeight: 300, color: theme.editorial }}>
              <CharStagger threshold={0.05} delay={0.4}>your own,</CharStagger>
            </em>
            <br/>
            <CharStagger threshold={0.05} delay={0.85}>running on </CharStagger>
            <em style={{ fontWeight: 300, color: theme.editorial }}>
              <CharStagger threshold={0.05} delay={1.25}>your hardware.</CharStagger>
            </em>
          </h1>

          <p className="folio-cover-lede" style={{
            margin: 0, marginTop: 36,
            fontFamily: '"Inter", system-ui',
            fontSize: 19, lineHeight: 1.55,
            color: theme.inkSoft,
            maxWidth: '52ch',
            textWrap: 'pretty',
            opacity: seen ? 1 : 0,
            transform: seen ? 'none' : 'translateY(20px)',
            transition: 'opacity 1s 1.6s, transform 1s 1.6s',
          }}>
            Daimon is a personal agent that listens, reasons and remembers — without
            sending your life through someone else's datacenter. Local-first by
            construction. Yours by design.
          </p>

          {/* CTAs */}
          <div className="folio-cover-ctas" style={{
            display: 'flex', gap: 14, marginTop: 44,
            opacity: seen ? 1 : 0,
            transform: seen ? 'none' : 'translateY(20px)',
            transition: 'opacity 1s 2s, transform 1s 2s',
          }}>
            <button style={{
              padding: '14px 22px', borderRadius: 0,
              background: theme.ink, color: theme.paper,
              border: 'none', cursor: 'pointer',
              fontSize: 13.5, fontWeight: 500,
              fontFamily: '"Inter", system-ui',
              letterSpacing: 0.2,
              display: 'flex', alignItems: 'center', gap: 10,
            }}>
              Install Daimon <span style={{ fontFamily: '"JetBrains Mono", monospace', fontSize: 12, opacity: 0.7 }}>↳ macOS · Linux</span>
            </button>
            <button style={{
              padding: '14px 22px', borderRadius: 0,
              background: 'transparent', color: theme.ink,
              border: `1px solid ${theme.lineStrong}`,
              cursor: 'pointer',
              fontSize: 13.5, fontWeight: 500,
              fontFamily: '"Inter", system-ui',
              letterSpacing: 0.2,
            }}>
              Read the manifesto →
            </button>
          </div>

          {/* Stat strip */}
          <div className="folio-cover-stats" style={{
            display: 'grid', gridTemplateColumns: 'repeat(3, auto)', gap: 64,
            marginTop: 80,
            paddingTop: 24,
            borderTop: `1px solid ${theme.line}`,
            opacity: seen ? 1 : 0,
            transition: 'opacity 1.2s 2.4s',
          }}>
            {[
              { n: '100%', l: 'on-device', d: 'inference' },
              { n: '12+', l: 'tools', d: 'MCP-native' },
              { n: '∞', l: 'memory', d: 'you control' },
            ].map((s, i) => (
              <div key={i}>
                <div style={{
                  fontFamily: '"Fraunces", serif',
                  fontSize: 44, fontWeight: 300, lineHeight: 1,
                  color: theme.ink, letterSpacing: -1,
                }}>{s.n}</div>
                <div style={{
                  marginTop: 8,
                  fontFamily: '"JetBrains Mono", monospace',
                  fontSize: 11, letterSpacing: 0.5, textTransform: 'uppercase',
                  color: theme.inkMuted,
                }}>
                  <span style={{ color: theme.ink }}>{s.l}</span>
                  <span style={{ color: theme.inkFaint, padding: '0 6px' }}>·</span>
                  <span>{s.d}</span>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Right: editorial plate (statue + reaching hand + olive branch) */}
        <div ref={wmRef} className="folio-hero-plate-wrap" style={{ position: 'relative', minHeight: 620 }}>
          <HeroPlate theme={theme} mouse={mouse} seen={seen} />
        </div>
      </div>
    </section>
  );
}

// ─────────────────────────────────────────────────────────────
// ComposedGlyph — the ⫶ wordmark. On enter, three dots descend
// one by one (elastic spring), connected by a line that draws.
// Tilts with mouse subtly afterward.
// ─────────────────────────────────────────────────────────────
function ComposedGlyph({ theme, composed, mouse }) {
  // Dot offsets and timings
  const dotSize = 36;
  const containerH = 360;
  const stagger = [0, 0.3, 0.6];

  const tilt = (mouse.x - 0.5) * 6;

  return (
    <div style={{
      position: 'absolute',
      top: 50, left: 0, right: 0, bottom: 70,
      display: 'flex', alignItems: 'center', justifyContent: 'center',
    }}>
      <div style={{
        position: 'relative',
        width: 240, height: containerH,
        transform: `rotate(${tilt * 0.4}deg)`,
        transition: 'transform 0.4s ease',
      }}>
        {/* Vertical guide rule (dotted) */}
        <div aria-hidden style={{
          position: 'absolute', top: 0, bottom: 0, left: '50%',
          width: 1, marginLeft: -0.5,
          backgroundImage: `linear-gradient(to bottom, ${theme.line} 50%, transparent 50%)`,
          backgroundSize: '1px 6px',
          opacity: composed ? 1 : 0,
          transition: 'opacity 0.8s 1.5s',
        }}/>

        {/* Horizontal bisecting rule (the "bar" of ⫶) */}
        <div aria-hidden style={{
          position: 'absolute', top: '50%', left: 0, right: 0,
          height: 1, marginTop: -0.5,
          background: theme.line,
          opacity: composed ? 1 : 0,
          transition: 'opacity 0.8s 1.7s',
        }}/>

        {/* The three dots */}
        {[0, 1, 2].map(i => {
          const yPos = ['12%', '50%', '88%'][i];
          return (
            <div key={i} style={{
              position: 'absolute',
              top: yPos, left: '50%',
              width: dotSize, height: dotSize, borderRadius: '50%',
              background: theme.ink,
              transform: composed
                ? 'translate(-50%, -50%) scale(1)'
                : 'translate(-50%, -200%) scale(0.6)',
              opacity: composed ? 1 : 0,
              transition: `transform 0.9s cubic-bezier(0.34, 1.6, 0.64, 1) ${stagger[i]}s, opacity 0.5s ${stagger[i]}s`,
              boxShadow: i === 1 ? `0 0 30px ${theme.accent}40` : 'none',
            }}>
              {i === 1 && (
                <span aria-hidden style={{
                  position: 'absolute', inset: -4,
                  borderRadius: '50%',
                  border: `1px solid ${theme.accent}`,
                  animation: composed ? 'pulseRing 2.4s ease-out infinite 1.5s' : 'none',
                }}/>
              )}
            </div>
          );
        })}

        {/* Plate label corners */}
        {[
          { pos: { top: '12%', left: '60%' }, txt: 'a · top' },
          { pos: { top: '50%', left: '60%' }, txt: 'b · bar' },
          { pos: { top: '88%', left: '60%' }, txt: 'c · base' },
        ].map((p, i) => (
          <div key={i} style={{
            position: 'absolute', ...p.pos, marginLeft: 32, transform: 'translateY(-50%)',
            fontFamily: '"JetBrains Mono", monospace',
            fontSize: 10, color: theme.inkMuted, letterSpacing: 0.5,
            opacity: composed ? 1 : 0,
            transition: `opacity 0.8s ${1.8 + i * 0.15}s`,
          }}>{p.txt}</div>
        ))}

        {/* Wordmark below */}
        <div style={{
          position: 'absolute', bottom: -80, left: 0, right: 0,
          textAlign: 'center',
          fontFamily: '"Fraunces", serif', fontWeight: 300,
          fontSize: 36, letterSpacing: 4,
          color: theme.ink,
          opacity: composed ? 1 : 0,
          transform: composed ? 'none' : 'translateY(8px)',
          transition: 'opacity 0.8s 2.3s, transform 0.8s 2.3s',
        }}>DAIMON</div>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Folio 01 — MANIFESTO
// ─────────────────────────────────────────────────────────────
function FolioManifesto({ theme, scrollerRef, dropCaps }) {
  const ref = React.useRef(null);
  const seen = useInView(ref, 0.15);

  return (
    <section ref={ref} id="manifesto" data-screen-label="01 Manifesto" style={{
      position: 'relative',
      padding: '120px 64px',
      borderBottom: `1px solid ${theme.line}`,
    }}>
      <FolioMark theme={theme} roman="I" label="Manifesto" subtitle="On sovereignty, presence, and the inner voice" page={2} />

      <div className="folio-mani-grid" style={{
        display: 'grid',
        gridTemplateColumns: '320px 1fr',
        gap: 80,
        alignItems: 'start',
      }}>
        <div>
          <Kicker theme={theme} n="01">About the studio</Kicker>
          <FoTitle theme={theme} size={64}>
            We treat your data as <em style={{ color: theme.editorial }}>sovereign,</em> not <em style={{ color: theme.editorial }}>negotiable</em>.
          </FoTitle>
        </div>

        <div style={{ paddingTop: 12 }}>
          <div style={{
            fontFamily: '"Inter", system-ui',
            fontSize: 17, lineHeight: 1.75, color: theme.inkSoft,
            maxWidth: '58ch',
          }}>
            {dropCaps && <DropCap theme={theme} letter="T" />}
            <p style={{ margin: 0 }}>
              {!dropCaps && 'T'}he strongest models in the world already run on your laptop. Daimon
              doesn't ship a frontier model — it wires the ones you trust into a daily-use
              agent that sees your files, runs your tools, and keeps every byte on your
              machine unless you say otherwise.
            </p>
            <p style={{ margin: '24px 0 0' }}>
              We believe an agent should feel <em style={{ fontFamily: '"Fraunces", serif', color: theme.ink }}>present without
              being intrusive</em>. The Greek δαίμων was never a master — it was an
              attendant: an inner voice that <em style={{ fontFamily: '"Fraunces", serif', color: theme.ink }}>guides without
              commanding</em>. We took the name seriously.
            </p>
            <p style={{ margin: '24px 0 0' }}>
              No telemetry by default. No silent context exfiltration. No "but our cloud
              is safer than your machine." If a sentence starts with <em style={{ fontFamily: '"Fraunces", serif', color: theme.ink }}>"trust
              us"</em>, we deleted the sentence.
            </p>
          </div>

          {/* Signature line */}
          <div style={{
            marginTop: 56, paddingTop: 20,
            borderTop: `1px solid ${theme.line}`,
            display: 'flex', alignItems: 'baseline', gap: 16,
            fontFamily: '"JetBrains Mono", monospace',
            fontSize: 11, letterSpacing: 0.6, textTransform: 'uppercase',
            color: theme.inkMuted,
          }}>
            <span style={{ color: theme.rubric, fontWeight: 600 }}>FILED UNDER</span>
            <span style={{ color: theme.ink }}>Memory · Tools · Sovereignty</span>
            <span style={{ flex: 1, height: 1, background: theme.line, transform: 'translateY(-3px)' }}/>
            <span style={{ fontFamily: '"Fraunces", serif', fontStyle: 'italic', textTransform: 'none', fontSize: 13, color: theme.ink }}>
              — Studio Δ, MMXXVI
            </span>
          </div>
        </div>
      </div>
    </section>
  );
}

// ─────────────────────────────────────────────────────────────
// Folio 02 — CAPABILITIES
// ─────────────────────────────────────────────────────────────
function FolioCapabilities({ theme }) {
  const ref = React.useRef(null);
  const seen = useInView(ref, 0.1);

  const cards = [
    { n: '01', kicker: 'Memory', title: 'Memory you can read', body: 'A real index of what Daimon knows about you. Editable, exportable, deletable. No black box, no "we are figuring it out".' },
    { n: '02', kicker: 'Tools', title: 'Tools, not promises', body: 'Calendar, files, browsing, shell. Every tool is auditable, gated by trust pills, and shows you exactly what it ran.' },
    { n: '03', kicker: 'MCP', title: 'MCP-native', body: 'First-class Model Context Protocol. Plug new servers in seconds; Daimon discovers their tools and respects their scopes.' },
    { n: '04', kicker: 'BYOM', title: 'Bring your own model', body: 'Llama, Qwen, Mistral, your fine-tuned proprietary stack. OpenAI-compatible proxy. Paste a baseUrl, ship.' },
  ];

  return (
    <section ref={ref} id="capabilities" data-screen-label="02 Capabilities" style={{
      position: 'relative',
      padding: '120px 64px',
      background: theme.paperWash,
      borderBottom: `1px solid ${theme.line}`,
    }}>
      <FolioMark theme={theme} roman="II" label="Capabilities · Skills · Surfaces" subtitle="4 surfaces / 1 loop" page={3} />

      <div className="folio-cap-intro" style={{
        display: 'grid',
        gridTemplateColumns: '440px 1fr',
        gap: 80,
        alignItems: 'start',
        marginBottom: 64,
      }}>
        <div>
          <Kicker theme={theme} n="03">Capabilities matrix</Kicker>
          <FoTitle theme={theme} size={64}>
            Four <em style={{ color: theme.editorial }}>surfaces</em> for personal intelligence.
          </FoTitle>
        </div>
        <Lede theme={theme} style={{ marginTop: 0, paddingTop: 12 }}>
          We don't build a chatbot with extras bolted on. We build four surfaces that share
          one mental model — what you said, what it found, what it did, what it remembers.
          Each surface is small enough to reason about and big enough to ship real work.
        </Lede>
      </div>

      {/* Capabilities grid */}
      <div className="folio-cap-grid" style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(4, 1fr)',
        gap: 1,
        background: theme.line,
        border: `1px solid ${theme.line}`,
      }}>
        {cards.map((c, i) => (
          <CapabilityCard key={c.n} theme={theme} card={c} idx={i} seen={seen} />
        ))}
      </div>
    </section>
  );
}

function CapabilityCard({ theme, card, idx, seen }) {
  const [hover, setHover] = React.useState(false);
  return (
    <div
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        background: theme.paper,
        padding: 32,
        minHeight: 320,
        position: 'relative',
        transformStyle: 'preserve-3d',
        perspective: 1200,
        opacity: seen ? 1 : 0,
        transform: seen ? 'rotateY(0deg) translateY(0)' : 'rotateY(-30deg) translateY(20px)',
        transformOrigin: 'left center',
        transition: `opacity 0.8s cubic-bezier(0.2,0.8,0.2,1) ${idx * 0.12}s, transform 0.9s cubic-bezier(0.2,0.8,0.2,1) ${idx * 0.12}s`,
        cursor: 'default',
      }}>
      {/* Card index */}
      <div style={{
        fontFamily: '"JetBrains Mono", monospace',
        fontSize: 11, color: theme.inkMuted, letterSpacing: 0.6,
        display: 'flex', alignItems: 'baseline', gap: 8,
        marginBottom: 28,
      }}>
        <span style={{ color: theme.rubric, fontWeight: 600 }}>{card.n}</span>
        <span style={{
          color: theme.ink, textTransform: 'uppercase', letterSpacing: 1.4,
        }}>{card.kicker}</span>
      </div>

      <div style={{
        fontFamily: '"Fraunces", serif',
        fontWeight: 350,
        fontSize: 26, lineHeight: 1.2, letterSpacing: -0.5,
        color: theme.ink,
        textWrap: 'balance',
        marginBottom: 18,
      }}>{card.title}</div>

      <div style={{
        fontFamily: '"Inter", system-ui',
        fontSize: 14.5, lineHeight: 1.6, color: theme.inkSoft,
        textWrap: 'pretty',
      }}>{card.body}</div>

      {/* Hover indicator */}
      <div style={{
        position: 'absolute', bottom: 24, left: 32, right: 32,
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        fontFamily: '"JetBrains Mono", monospace',
        fontSize: 11, color: hover ? theme.accent : theme.inkMuted,
        letterSpacing: 0.5,
        transition: 'color 0.3s',
      }}>
        <span>↳ Read more</span>
        <span style={{
          width: 24, height: 1, background: hover ? theme.accent : theme.line,
          transition: 'background 0.3s, width 0.3s',
        }}/>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Folio 03 — METHOD (Listen → Reason → Act → Remember)
// Ink line traces between the four stages as you scroll.
// ─────────────────────────────────────────────────────────────
function FolioMethod({ theme, scrollerRef }) {
  const ref = React.useRef(null);
  const p = useScrollProgress(ref, scrollerRef);
  const seen = useInView(ref, 0.1);

  const stages = [
    { n: '01', label: 'Listen', body: 'It hears you across surfaces — chat, terminal, a hotkey from anywhere on your desk.' },
    { n: '02', label: 'Reason', body: 'A reasoning loop you can read. Tool calls and decisions are visible, not hidden behind "thinking…".' },
    { n: '03', label: 'Act', body: 'It runs the tool, edits the file, hits the API. Every action is auditable and reversible.' },
    { n: '04', label: 'Remember', body: 'Outcomes are written to a memory you own. Reviewable, taggable, exportable, deletable.' },
  ];

  return (
    <section ref={ref} id="method" data-screen-label="03 Method" style={{
      position: 'relative',
      padding: '140px 64px 160px',
      borderBottom: `1px solid ${theme.line}`,
      overflow: 'hidden',
    }}>
      <FolioMark theme={theme} roman="III" label="Method / Loop" subtitle="04 stages, iterative" page={4} />

      <div className="folio-method-intro" style={{
        display: 'grid',
        gridTemplateColumns: '1fr 1fr',
        gap: 80,
        alignItems: 'end',
        marginBottom: 100,
      }}>
        <div>
          <Kicker theme={theme} n="05">Method</Kicker>
          <FoTitle theme={theme} size={72}>
            From <em style={{ color: theme.editorial }}>signal</em> to <em style={{ color: theme.editorial }}>memory</em>.
          </FoTitle>
        </div>
        <Lede theme={theme} style={{ marginTop: 0 }}>
          Every cycle is small, visible, and reversible. The loop doesn't speed up by hiding
          steps from you — it speeds up because the steps are small enough to skim, and
          honest enough to trust.
        </Lede>
      </div>

      {/* Stages: 4 cards in a row, with an SVG ink path connecting them */}
      <div style={{ position: 'relative' }}>
        {/* Ink-line that draws as you scroll */}
        <svg aria-hidden className="folio-method-svg" style={{
          position: 'absolute', top: 60, left: 0, right: 0, height: 80,
          width: '100%', pointerEvents: 'none', overflow: 'visible',
        }} preserveAspectRatio="none">
          <MethodPath theme={theme} progress={Math.max(0, Math.min(1, (p - 0.18) / 0.6))} />
        </svg>

        <div className="folio-method-grid" style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(4, 1fr)',
          gap: 32,
          position: 'relative',
        }}>
          {stages.map((s, i) => (
            <MethodStage key={s.n} stage={s} idx={i} theme={theme} progress={p} />
          ))}
        </div>
      </div>

      {/* Footer aphorism */}
      <div style={{
        marginTop: 100,
        paddingTop: 32,
        borderTop: `1px solid ${theme.line}`,
        display: 'flex', alignItems: 'baseline', gap: 24,
      }}>
        <span style={{
          fontFamily: '"Fraunces", serif', fontStyle: 'italic',
          fontSize: 22, color: theme.ink, lineHeight: 1.4,
          maxWidth: '60ch',
        }}>
          The loop is not the magic. The loop being <em style={{ color: theme.editorial }}>readable</em> is the magic.
        </span>
        <span style={{ flex: 1 }}/>
        <span style={{
          fontFamily: '"JetBrains Mono", monospace',
          fontSize: 11, color: theme.inkMuted, letterSpacing: 0.6, textTransform: 'uppercase',
        }}>— Studio Δ · 2026</span>
      </div>
    </section>
  );
}

function MethodPath({ theme, progress }) {
  // 4 stages spread across full width — line zigzags slightly between them
  // The SVG is full width 100, but we use percentages.
  const yMid = 40;
  const stops = [12, 37, 62, 87]; // x % for the 4 stages (centered)
  const path = `
    M ${stops[0]}% ${yMid}
    Q ${(stops[0] + stops[1]) / 2}% ${yMid - 30}, ${stops[1]}% ${yMid}
    Q ${(stops[1] + stops[2]) / 2}% ${yMid + 30}, ${stops[2]}% ${yMid}
    Q ${(stops[2] + stops[3]) / 2}% ${yMid - 30}, ${stops[3]}% ${yMid}
  `;
  const ref = React.useRef(null);
  const [len, setLen] = React.useState(800);
  React.useEffect(() => {
    if (ref.current) setLen(ref.current.getTotalLength());
  }, []);
  return (
    <>
      <path
        d={path}
        fill="none"
        stroke={theme.line}
        strokeWidth={1}
        strokeDasharray="3 4"
      />
      <path
        ref={ref}
        d={path}
        fill="none"
        stroke={theme.editorial}
        strokeWidth={1.6}
        strokeLinecap="round"
        strokeDasharray={`${len} ${len}`}
        strokeDashoffset={len * (1 - progress)}
        style={{ transition: 'stroke-dashoffset 0.15s linear' }}
      />
      {/* Cursor pen tip */}
      {progress > 0 && progress < 1 && (
        <circle r={4} fill={theme.editorial}>
          <animateMotion dur="0.001s" fill="freeze">
            <mpath href="#__none" />
          </animateMotion>
        </circle>
      )}
    </>
  );
}

function MethodStage({ stage, idx, theme, progress }) {
  // Each stage activates at progress thresholds 0.2, 0.4, 0.6, 0.8 ish
  const thresholds = [0.22, 0.36, 0.50, 0.64];
  const active = progress > thresholds[idx];

  return (
    <div className="folio-method-stage" style={{
      position: 'relative',
      paddingTop: 100,
    }}>
      {/* The dot on the line */}
      <div className="folio-method-dot" style={{
        position: 'absolute',
        top: 32, left: '50%', transform: 'translateX(-50%)',
        width: active ? 18 : 8, height: active ? 18 : 8,
        borderRadius: '50%',
        background: active ? theme.editorial : theme.line,
        boxShadow: active ? `0 0 0 6px ${theme.editorialSoft}` : 'none',
        transition: 'all 0.5s cubic-bezier(0.34, 1.4, 0.64, 1)',
        zIndex: 2,
      }}/>

      <div style={{
        fontFamily: '"JetBrains Mono", monospace',
        fontSize: 11, color: theme.inkMuted, letterSpacing: 0.8,
        marginBottom: 8,
        textAlign: 'center',
      }}>
        <span style={{ color: theme.rubric, fontWeight: 600 }}>{stage.n}</span>
      </div>
      <div style={{
        fontFamily: '"Fraunces", serif',
        fontWeight: 350,
        fontSize: 36, lineHeight: 1, letterSpacing: -0.5,
        color: theme.ink,
        textAlign: 'center',
        marginBottom: 18,
        fontStyle: 'italic',
        transform: active ? 'translateY(0)' : 'translateY(6px)',
        opacity: active ? 1 : 0.5,
        transition: 'all 0.6s cubic-bezier(0.2,0.8,0.2,1)',
      }}>{stage.label}</div>
      <div style={{
        fontFamily: '"Inter", system-ui',
        fontSize: 13.5, lineHeight: 1.55, color: theme.inkSoft,
        textAlign: 'center', textWrap: 'pretty',
        opacity: active ? 1 : 0.4,
        transition: 'opacity 0.6s',
      }}>{stage.body}</div>
    </div>
  );
}

Object.assign(window, {
  FolioCover, FolioManifesto, FolioCapabilities, FolioMethod,
});
