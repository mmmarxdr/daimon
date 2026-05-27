// Studio Δ — Folios 04 to 07 (Field, Selected Work, Lineage, Contact)

// ─────────────────────────────────────────────────────────────
// Folio 04 — FROM THE FIELD (city ticker + contributors)
// ─────────────────────────────────────────────────────────────
function FolioField({ theme, scrollerRef, showTicker }) {
  const ref = React.useRef(null);
  const seen = useInView(ref, 0.1);
  const p = useScrollProgress(ref, scrollerRef);

  const cities = [
    { city: 'Berlin',         coord: '52.52°N' },
    { city: 'Tokyo',          coord: '35.68°N' },
    { city: 'San Francisco',  coord: '37.77°N' },
    { city: 'São Paulo',      coord: '23.55°S' },
    { city: 'Lagos',          coord: '6.52°N'  },
    { city: 'Bangalore',      coord: '12.97°N' },
    { city: 'Singapore',      coord: '1.35°N'  },
    { city: 'Lisbon',         coord: '38.72°N' },
    { city: 'Mexico City',    coord: '19.43°N' },
    { city: 'Sydney',         coord: '33.87°S' },
    { city: 'Amsterdam',      coord: '52.37°N' },
    { city: 'Cape Town',      coord: '33.92°S' },
    { city: 'Stockholm',      coord: '59.33°N' },
    { city: 'Paris',          coord: '48.86°N' },
    { city: 'Taipei',         coord: '25.03°N' },
    { city: 'Toronto',        coord: '43.65°N' },
    { city: 'Madrid',         coord: '40.42°N' },
  ];

  const contributors = [
    { handle: '@dimitri.k',     role: 'studio'      },
    { handle: '@arenas',        role: 'design'      },
    { handle: '@nuria.solà',    role: 'systems'     },
    { handle: '@tw_ishida',     role: 'memory'      },
    { handle: '@op_huashu',     role: 'tools'       },
    { handle: '@you next →',    role: 'contributor' },
  ];

  return (
    <section ref={ref} id="field" data-screen-label="04 Field" style={{
      position: 'relative',
      padding: '120px 0 120px',
      borderBottom: `1px solid ${theme.line}`,
      background: theme.paperWash,
      overflow: 'hidden',
    }}>
      <div style={{ padding: '0 64px' }}>
        <FolioMark theme={theme} roman="IV" label="From the field" subtitle="Open · 17 cities · 6 contributors" page={5} />

        <div className="folio-field-intro" style={{
          display: 'grid', gridTemplateColumns: '420px 1fr', gap: 80,
          alignItems: 'end',
          marginBottom: 64,
        }}>
          <div>
            <Kicker theme={theme} n="06">In-the-field</Kicker>
            <FoTitle theme={theme} size={64}>
              Running on <em style={{ color: theme.editorial }}>desks</em> across the world.
            </FoTitle>
          </div>
          <Lede theme={theme} style={{ marginTop: 0 }}>
            Daimon is local-first by construction. There is no central registry, no daily ping
            home. The map below is reported voluntarily by people who chose to be on it.
          </Lede>
        </div>
      </div>

      {/* Map plate — minimal grid + dots representing live nodes */}
      <div style={{ padding: '0 64px', marginBottom: 56 }}>
        <FieldMap theme={theme} progress={p} />
      </div>

      {/* Ticker — italics-on-focus */}
      {showTicker && (
        <>
          <div style={{
            padding: '24px 0',
            borderTop: `1px solid ${theme.line}`,
            borderBottom: `1px solid ${theme.line}`,
            background: theme.paper,
          }}>
            <Marquee items={cities} theme={theme} speed={50} gap={42} focal={0.5} accent={theme.editorial} />
          </div>
          <div style={{
            padding: '24px 0',
            borderBottom: `1px solid ${theme.line}`,
            background: theme.paper,
          }}>
            <Marquee
              items={cities.slice().reverse().map(c => ({ city: c.city, coord: c.coord }))}
              theme={theme} speed={38} gap={42} focal={0.5} accent={theme.accent}
            />
          </div>
        </>
      )}

      {/* Contributors */}
      <div style={{ padding: '64px 64px 0' }}>
        <div style={{
          display: 'flex', alignItems: 'baseline', gap: 16,
          fontFamily: '"JetBrains Mono", monospace',
          fontSize: 11, letterSpacing: 0.6, textTransform: 'uppercase',
          color: theme.inkMuted,
          marginBottom: 24,
        }}>
          <span style={{ color: theme.rubric, fontWeight: 600 }}>Nº 07</span>
          <span style={{ color: theme.ink }}>Contributors</span>
          <span style={{ flex: 1, height: 1, background: theme.line, transform: 'translateY(-3px)' }}/>
          <span>06 · open · MIT</span>
        </div>
        <div style={{
          display: 'flex', flexWrap: 'wrap',
          fontFamily: '"Fraunces", serif',
          fontSize: 22, color: theme.ink,
          gap: '4px 0',
          alignItems: 'baseline',
        }}>
          {contributors.map((c, i) => (
            <React.Fragment key={i}>
              {i > 0 && <span style={{ color: theme.inkFaint, padding: '0 14px', fontSize: 18 }}>·</span>}
              <a href="#" style={{
                color: 'inherit', textDecoration: 'none',
                position: 'relative',
                fontStyle: 'italic',
                transition: 'color 0.3s',
              }}
                onMouseEnter={(e) => e.currentTarget.style.color = theme.editorial}
                onMouseLeave={(e) => e.currentTarget.style.color = theme.ink}
              >
                {c.handle}
                <span style={{
                  fontFamily: '"JetBrains Mono", monospace',
                  fontSize: 10.5, color: theme.inkMuted,
                  marginLeft: 8, fontStyle: 'normal',
                  textTransform: 'uppercase', letterSpacing: 0.5,
                }}>{c.role}</span>
              </a>
            </React.Fragment>
          ))}
        </div>
      </div>
    </section>
  );
}

function FieldMap({ theme, progress }) {
  // Simplified world projection — grid + dots for cities
  // SVG viewBox 1000x420, equirectangular-ish
  const cityCoords = [
    { city: 'Berlin',         x: 540, y: 140 },
    { city: 'Tokyo',          x: 850, y: 175 },
    { city: 'San Francisco',  x: 175, y: 195 },
    { city: 'São Paulo',      x: 350, y: 295 },
    { city: 'Lagos',          x: 525, y: 245 },
    { city: 'Bangalore',      x: 720, y: 235 },
    { city: 'Singapore',      x: 800, y: 260 },
    { city: 'Lisbon',         x: 470, y: 175 },
    { city: 'Mexico City',    x: 215, y: 230 },
    { city: 'Sydney',         x: 905, y: 320 },
    { city: 'Amsterdam',      x: 528, y: 138 },
    { city: 'Cape Town',      x: 555, y: 330 },
    { city: 'Stockholm',      x: 555, y: 110 },
    { city: 'Paris',          x: 510, y: 145 },
    { city: 'Taipei',         x: 830, y: 220 },
    { city: 'Toronto',        x: 270, y: 175 },
    { city: 'Madrid',         x: 480, y: 175 },
  ];

  return (
    <div className="folio-field-map" style={{
      position: 'relative',
      border: `1px solid ${theme.line}`,
      background: theme.paper,
      aspectRatio: '1000 / 420',
    }}>
      <svg viewBox="0 0 1000 420" style={{ display: 'block', width: '100%' }}>
        {/* Lat/lon grid */}
        {Array.from({ length: 11 }).map((_, i) => (
          <line key={`v${i}`} x1={i * 100} y1={0} x2={i * 100} y2={420}
                stroke={theme.line} strokeWidth={0.5} />
        ))}
        {Array.from({ length: 5 }).map((_, i) => (
          <line key={`h${i}`} x1={0} y1={i * 105} x2={1000} y2={i * 105}
                stroke={theme.line} strokeWidth={0.5} />
        ))}

        {/* Continent silhouettes — stippled dots */}
        <ContinentStipple theme={theme} />

        {/* City dots */}
        {cityCoords.map((c, i) => {
          const t = i / cityCoords.length;
          const visible = progress > 0.1 + t * 0.4;
          return (
            <g key={c.city} style={{
              opacity: visible ? 1 : 0,
              transition: `opacity 0.6s ${i * 0.04}s`,
            }}>
              <circle cx={c.x} cy={c.y} r={3} fill={theme.editorial} />
              <circle cx={c.x} cy={c.y} r={6} fill={theme.editorial} opacity={0.18}>
                {visible && (
                  <animate attributeName="r" from="3" to="14" dur="2.4s" begin={`${i * 0.18}s`} repeatCount="indefinite"/>
                )}
                {visible && (
                  <animate attributeName="opacity" from="0.4" to="0" dur="2.4s" begin={`${i * 0.18}s`} repeatCount="indefinite"/>
                )}
              </circle>
              <text x={c.x + 8} y={c.y + 3}
                    fill={theme.inkMuted}
                    fontFamily='"JetBrains Mono", monospace'
                    fontSize={9}
                    style={{ letterSpacing: 0.3 }}>
                {c.city}
              </text>
            </g>
          );
        })}

        {/* Corner registration marks */}
        {[[20, 20], [980, 20], [20, 400], [980, 400]].map(([x, y], i) => (
          <g key={i} stroke={theme.inkMuted} strokeWidth={0.5}>
            <line x1={x - 6} y1={y} x2={x + 6} y2={y}/>
            <line x1={x} y1={y - 6} x2={x} y2={y + 6}/>
          </g>
        ))}
      </svg>

      {/* Plate caption */}
      <div style={{
        position: 'absolute', bottom: -24, left: 0, right: 0,
        marginTop: 12,
        display: 'flex', alignItems: 'baseline', gap: 12,
        fontFamily: '"JetBrains Mono", monospace',
        fontSize: 10.5, letterSpacing: 0.6, textTransform: 'uppercase',
        color: theme.inkMuted,
      }}>
        <span style={{ color: theme.rubric, fontWeight: 600 }}>FIG. 02</span>
        <span style={{ color: theme.ink, fontWeight: 500 }}>Reported nodes · 17 of 17</span>
        <span style={{ flex: 1, height: 1, background: theme.line, transform: 'translateY(-3px)' }}/>
        <span>Open Atlas · MMXXVI</span>
      </div>
    </div>
  );
}

function ContinentStipple({ theme }) {
  // Approximate continent shapes via dot density. Hand-tuned so it
  // reads as "world" without being heavy.
  const dots = [];
  // North America
  for (let i = 0; i < 40; i++) dots.push([150 + Math.random() * 180, 150 + Math.random() * 100]);
  // South America
  for (let i = 0; i < 28; i++) dots.push([300 + Math.random() * 80, 230 + Math.random() * 130]);
  // Europe
  for (let i = 0; i < 22; i++) dots.push([460 + Math.random() * 120, 110 + Math.random() * 70]);
  // Africa
  for (let i = 0; i < 32; i++) dots.push([490 + Math.random() * 100, 200 + Math.random() * 150]);
  // Asia
  for (let i = 0; i < 60; i++) dots.push([600 + Math.random() * 280, 110 + Math.random() * 160]);
  // Oceania
  for (let i = 0; i < 16; i++) dots.push([870 + Math.random() * 70, 290 + Math.random() * 60]);

  return (
    <g fill={theme.inkFaint} opacity={0.5}>
      {dots.map(([x, y], i) => <circle key={i} cx={x} cy={y} r={1.2} />)}
    </g>
  );
}

// ─────────────────────────────────────────────────────────────
// Folio 05 — SELECTED WORK / skills
// ─────────────────────────────────────────────────────────────
function FolioWorks({ theme }) {
  const ref = React.useRef(null);
  const seen = useInView(ref, 0.1);

  const works = [
    {
      n: '01 / 12', cat: 'MEMORY · DEFAULT', year: '2026',
      title: 'Living memory',
      body: 'A real-time index of facts, preferences and projects. Editable in plain text; source-cited; never scraped without consent.',
      glyph: 'MEM',
    },
    {
      n: '02 / 12', cat: 'TOOLS · MCP', year: '2026',
      title: 'Tool theatre',
      body: 'Tool calls render as small theatrical acts: title, inputs, decisions, outcome. Trust pills gate sensitive operations.',
      glyph: 'TLS',
    },
    {
      n: '03 / 12', cat: 'REASONING · OPEN', year: '2026',
      title: 'Visible reasoning',
      body: 'No hidden chain-of-thought tax. Reasoning panels are first-class — collapsible, copyable, exportable as markdown.',
      glyph: 'RSN',
    },
    {
      n: '04 / 12', cat: 'INSTALL · LOCAL', year: '2026',
      title: 'One binary',
      body: 'Single static binary. No Electron, no daemon-of-daemons. Boots in 200ms, runs offline, ships in under 28MB.',
      glyph: 'BIN',
    },
  ];

  return (
    <section ref={ref} id="works" data-screen-label="05 Works" style={{
      position: 'relative',
      padding: '120px 64px',
      borderBottom: `1px solid ${theme.line}`,
    }}>
      <FolioMark theme={theme} roman="V" label="Selected work · 2026 catalogue" subtitle="Edited by Studio Δ" page={6} />

      <div style={{ marginBottom: 80 }}>
        <Kicker theme={theme} n="08">Selected work</Kicker>
        <FoTitle theme={theme} size={84} style={{ maxWidth: '20ch' }}>
          Surfaces that turn briefs into <em style={{ color: theme.editorial }}>memorable, shippable artefacts</em>.
        </FoTitle>
      </div>

      <div className="folio-work-grid" style={{
        display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 1,
        background: theme.line, border: `1px solid ${theme.line}`,
      }}>
        {works.map((w, i) => (
          <WorkCard key={w.n} work={w} idx={i} theme={theme} seen={seen} />
        ))}
      </div>

      <div style={{ marginTop: 32, display: 'flex', justifyContent: 'flex-end' }}>
        <a href="#" style={{
          fontFamily: '"JetBrains Mono", monospace',
          fontSize: 12, color: theme.ink, textDecoration: 'none',
          letterSpacing: 0.5, textTransform: 'uppercase',
          borderBottom: `1px solid ${theme.lineStrong}`,
          paddingBottom: 4,
        }}>
          View full catalogue · 12 surfaces →
        </a>
      </div>
    </section>
  );
}

function WorkCard({ work, idx, theme, seen }) {
  const [hover, setHover] = React.useState(false);
  return (
    <a href="#" style={{
      background: theme.paper,
      padding: '40px 36px',
      minHeight: 320,
      display: 'flex', flexDirection: 'column',
      gap: 16,
      textDecoration: 'none',
      color: 'inherit',
      position: 'relative',
      cursor: 'pointer',
      opacity: seen ? 1 : 0,
      transform: seen ? 'none' : `translateY(${20 + idx * 6}px)`,
      transition: `opacity 0.9s ${idx * 0.1}s, transform 0.9s ${idx * 0.1}s, background 0.3s`,
    }}
      onMouseEnter={(e) => { setHover(true); e.currentTarget.style.background = theme.paperElev; }}
      onMouseLeave={(e) => { setHover(false); e.currentTarget.style.background = theme.paper; }}
    >
      {/* Top row: glyph plate + meta */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 24, marginBottom: 14 }}>
        <div style={{
          width: 120, height: 88,
          background: theme.paperDeep,
          border: `1px solid ${theme.line}`,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          fontFamily: '"Fraunces", serif',
          fontSize: 26, fontWeight: 300, fontStyle: 'italic',
          color: theme.editorial,
          letterSpacing: 1,
          position: 'relative',
          transform: hover ? 'translateY(-2px)' : 'none',
          transition: 'transform 0.3s',
        }}>
          {work.glyph}
          {/* corner marks */}
          {['tl','tr','bl','br'].map((p) => (
            <span key={p} style={{
              position: 'absolute', width: 8, height: 1,
              background: theme.inkMuted,
              ...(p[0] === 't' ? { top: 6 } : { bottom: 6 }),
              ...(p[1] === 'l' ? { left: 6 } : { right: 6 }),
            }}/>
          ))}
        </div>
        <div style={{
          fontFamily: '"JetBrains Mono", monospace',
          fontSize: 11, letterSpacing: 0.6, textTransform: 'uppercase',
          color: theme.inkMuted,
          textAlign: 'right',
          lineHeight: 1.6,
        }}>
          <div style={{ color: theme.rubric, fontWeight: 600 }}>{work.n}</div>
          <div style={{ color: theme.ink }}>{work.cat}</div>
          <div>{work.year}</div>
        </div>
      </div>

      <h3 style={{
        margin: 0,
        fontFamily: '"Fraunces", serif',
        fontWeight: 350,
        fontSize: 32, lineHeight: 1.15, letterSpacing: -0.6,
        color: theme.ink,
        textWrap: 'balance',
      }}>{work.title}</h3>

      <p style={{
        margin: 0,
        fontFamily: '"Inter", system-ui',
        fontSize: 14.5, lineHeight: 1.6, color: theme.inkSoft,
        textWrap: 'pretty',
        flex: 1,
      }}>{work.body}</p>

      <div style={{
        paddingTop: 16, borderTop: `1px solid ${theme.line}`,
        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        fontFamily: '"JetBrains Mono", monospace',
        fontSize: 11, letterSpacing: 0.5, textTransform: 'uppercase',
        color: hover ? theme.accent : theme.inkMuted,
        transition: 'color 0.3s',
      }}>
        <span>↳ Open dossier</span>
        <span style={{
          width: hover ? 56 : 24, height: 1, background: hover ? theme.accent : theme.line,
          transition: 'all 0.4s',
        }}/>
      </div>
    </a>
  );
}

// ─────────────────────────────────────────────────────────────
// Folio 06 — LINEAGE / TESTIMONIALS
// ─────────────────────────────────────────────────────────────
function FolioLineage({ theme }) {
  const ref = React.useRef(null);
  const seen = useInView(ref, 0.1);

  return (
    <section ref={ref} id="lineage" data-screen-label="06 Lineage" style={{
      position: 'relative',
      padding: '120px 64px',
      borderBottom: `1px solid ${theme.line}`,
      background: theme.paperWash,
    }}>
      <FolioMark theme={theme} roman="VI" label="Collaborators / Lineage" subtitle="Standing on shoulders" page={7} />

      <div className="folio-lineage-grid" style={{
        display: 'grid', gridTemplateColumns: '1fr 380px', gap: 80,
        alignItems: 'start',
      }}>
        {/* Left: pull quote */}
        <div>
          <Kicker theme={theme} n="09">Field reports</Kicker>
          <blockquote className="folio-pullquote" style={{
            margin: 0,
            fontFamily: '"Fraunces", serif',
            fontWeight: 300,
            fontSize: 56, lineHeight: 1.12, letterSpacing: -1.2,
            color: theme.ink,
            textWrap: 'balance',
            position: 'relative',
            paddingLeft: 28,
          }}>
            <span aria-hidden className="folio-pullquote-mark" style={{
              position: 'absolute', left: -4, top: -10,
              fontSize: 110, lineHeight: 0.7,
              color: theme.editorial, opacity: 0.45,
              fontStyle: 'italic',
            }}>“</span>
            Daimon helped me <em style={{ color: theme.editorial }}>stop renting</em> an AI
            and start <em style={{ color: theme.editorial }}>owning one</em> — same
            quality, half the noise, none of the surveillance.
          </blockquote>

          <div style={{
            marginTop: 32, display: 'flex', alignItems: 'center', gap: 16,
            fontFamily: '"Inter", system-ui', fontSize: 13,
            color: theme.inkMuted,
          }}>
            <div style={{
              width: 44, height: 44, borderRadius: '50%',
              background: theme.editorial, color: theme.paper,
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontFamily: '"Fraunces", serif', fontSize: 20, fontStyle: 'italic',
            }}>m</div>
            <div>
              <div style={{ color: theme.ink, fontWeight: 500 }}>Mira Aalto</div>
              <div>Independent researcher · Helsinki</div>
            </div>
          </div>
        </div>

        {/* Right: lineage list */}
        <div>
          <div style={{
            fontFamily: '"JetBrains Mono", monospace',
            fontSize: 11, letterSpacing: 0.6, textTransform: 'uppercase',
            color: theme.inkMuted,
            marginBottom: 20,
            paddingBottom: 14,
            borderBottom: `1px solid ${theme.line}`,
          }}>
            <span style={{ color: theme.rubric, fontWeight: 600 }}>Nº 10</span>
            <span style={{ padding: '0 12px' }}>·</span>
            <span style={{ color: theme.ink }}>Standing on the shoulders of</span>
          </div>
          {[
            { who: 'llama.cpp',         what: 'inference'   },
            { who: 'Ollama',            what: 'runtime'     },
            { who: 'Model Context Proto', what: 'tool spec' },
            { who: 'Local-First Web',   what: 'philosophy'  },
            { who: 'huashu-design',     what: 'editorial'   },
          ].map((l, i) => (
            <a key={i} href="#" style={{
              display: 'flex', alignItems: 'baseline', gap: 12,
              padding: '14px 0',
              borderBottom: `1px solid ${theme.line}`,
              textDecoration: 'none',
              color: 'inherit',
              fontFamily: '"Fraunces", serif',
              transition: 'padding-left 0.3s',
            }}
              onMouseEnter={(e) => e.currentTarget.style.paddingLeft = '8px'}
              onMouseLeave={(e) => e.currentTarget.style.paddingLeft = '0'}
            >
              <span style={{
                fontSize: 18, color: theme.ink, fontStyle: 'italic',
              }}>{l.who}</span>
              <span style={{ flex: 1, height: 1, background: theme.line, transform: 'translateY(-3px)' }}/>
              <span style={{
                fontFamily: '"JetBrains Mono", monospace',
                fontSize: 10.5, color: theme.inkMuted, letterSpacing: 0.5,
                textTransform: 'uppercase',
              }}>{l.what} →</span>
            </a>
          ))}
        </div>
      </div>
    </section>
  );
}

// ─────────────────────────────────────────────────────────────
// Folio 07 — CONTACT / INSTALL / COLOPHON
// ─────────────────────────────────────────────────────────────
function FolioContact({ theme }) {
  const ref = React.useRef(null);
  const seen = useInView(ref, 0.1);
  const [copied, setCopied] = React.useState(false);

  const cmd = 'curl -fsSL daimon.sh/install | sh';

  return (
    <section ref={ref} id="contact" data-screen-label="07 Contact" style={{
      position: 'relative',
      padding: '160px 64px 80px',
    }}>
      <FolioMark theme={theme} roman="VII" label="Contact / Conversation" subtitle="One command to ship" page={8} />

      <div className="folio-contact-grid" style={{
        display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 80,
        alignItems: 'start',
        marginBottom: 120,
      }}>
        <div>
          <Kicker theme={theme} n="11">Begin</Kicker>
          <FoTitle theme={theme} size={84}>
            Let's build something <em style={{ color: theme.editorial }}>local</em> and <em style={{ color: theme.editorial }}>quietly</em> useful.
          </FoTitle>
          <Lede theme={theme} style={{ marginTop: 36 }}>
            Star us on GitHub, drop into the issues, or run the install line tonight.
            One command and the loop is yours.
          </Lede>
        </div>

        <div style={{ paddingTop: 24 }}>
          {/* Install command */}
          <div style={{
            fontFamily: '"JetBrains Mono", monospace',
            fontSize: 10.5, letterSpacing: 0.6, textTransform: 'uppercase',
            color: theme.inkMuted, marginBottom: 12,
          }}>
            <span style={{ color: theme.rubric, fontWeight: 600 }}>↳</span>
            <span style={{ paddingLeft: 8, color: theme.ink }}>One command, no signup</span>
          </div>
          <div className="folio-install" style={{
            border: `1px solid ${theme.lineStrong}`,
            background: theme.paperElev,
            padding: '24px 28px',
            display: 'flex', alignItems: 'center', justifyContent: 'space-between',
            gap: 24,
          }}>
            <code style={{
              fontFamily: '"JetBrains Mono", monospace',
              fontSize: 17, color: theme.ink,
              letterSpacing: -0.2,
            }}>
              <span style={{ color: theme.editorial }}>$</span> {cmd}
            </code>
            <button
              onClick={() => { navigator.clipboard?.writeText(cmd); setCopied(true); setTimeout(() => setCopied(false), 1500); }}
              style={{
                fontFamily: '"JetBrains Mono", monospace',
                fontSize: 11, letterSpacing: 0.5, textTransform: 'uppercase',
                color: copied ? theme.accent : theme.inkMuted,
                background: 'transparent',
                border: 'none', cursor: 'pointer',
                padding: 0,
                transition: 'color 0.3s',
              }}>
              {copied ? '✓ copied' : 'copy'}
            </button>
          </div>

          {/* CTAs */}
          <div style={{ display: 'flex', gap: 14, marginTop: 24 }}>
            <button style={{
              padding: '14px 22px', borderRadius: 0,
              background: theme.ink, color: theme.paper,
              border: 'none', cursor: 'pointer',
              fontSize: 13.5, fontWeight: 500,
              fontFamily: '"Inter", system-ui',
              flex: 1,
            }}>Star on GitHub</button>
            <button style={{
              padding: '14px 22px', borderRadius: 0,
              background: 'transparent', color: theme.ink,
              border: `1px solid ${theme.lineStrong}`,
              cursor: 'pointer',
              fontSize: 13.5, fontWeight: 500,
              fontFamily: '"Inter", system-ui',
              flex: 1,
            }}>Open an issue →</button>
          </div>

          <MetaRow theme={theme} items={['● Live · v0.3.0', 'MIT', 'macOS · Linux']} style={{ marginTop: 28 }} />
        </div>
      </div>

      {/* Final colophon — wordmark dis-assembles */}
      <ColophonGlyph theme={theme} />

      {/* Footer */}
      <div className="folio-footer-grid" style={{
        marginTop: 80,
        paddingTop: 28,
        borderTop: `1px solid ${theme.line}`,
        display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 1fr', gap: 32,
      }}>
        {[
          { h: 'Studio',  links: ['Manifesto', 'Method', 'Capabilities', 'Lineage'] },
          { h: 'Library', links: ['12 Surfaces', 'MCP Servers', 'Memory shapes', 'Themes'] },
          { h: 'Connect', links: ['GitHub', 'Issues', 'Contributors', 'Releases'] },
          { h: 'Docs',    links: ['Quickstart', 'Architecture', 'Privacy', 'Roadmap'] },
        ].map((col) => (
          <div key={col.h}>
            <div style={{
              fontFamily: '"JetBrains Mono", monospace',
              fontSize: 10.5, letterSpacing: 0.8, textTransform: 'uppercase',
              color: theme.inkMuted,
              marginBottom: 14,
              paddingBottom: 10,
              borderBottom: `1px solid ${theme.line}`,
            }}>{col.h}</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {col.links.map(l => (
                <a key={l} href="#" style={{
                  fontFamily: '"Inter", system-ui', fontSize: 13,
                  color: theme.inkSoft, textDecoration: 'none',
                  transition: 'color 0.2s',
                }}
                  onMouseEnter={(e) => e.currentTarget.style.color = theme.editorial}
                  onMouseLeave={(e) => e.currentTarget.style.color = theme.inkSoft}
                >{l}</a>
              ))}
            </div>
          </div>
        ))}
      </div>

      {/* FIN */}
      <div className="folio-fin" style={{
        marginTop: 56, paddingTop: 20,
        borderTop: `1px solid ${theme.line}`,
        display: 'flex', alignItems: 'baseline', gap: 16,
        fontFamily: '"JetBrains Mono", monospace',
        fontSize: 10.5, letterSpacing: 0.6, textTransform: 'uppercase',
        color: theme.inkMuted,
      }}>
        <span>● Daimon · MIT · 2026 / Vol. 01 / Issue Nº 01</span>
        <span style={{ flex: 1, height: 1, background: theme.line, transform: 'translateY(-3px)' }}/>
        <span style={{ fontFamily: '"Fraunces", serif', fontStyle: 'italic', textTransform: 'none', fontSize: 13, color: theme.ink }}>
          Studio Δ · FIN.
        </span>
      </div>
    </section>
  );
}

function ColophonGlyph({ theme }) {
  const ref = React.useRef(null);
  const seen = useInView(ref, 0.4);

  return (
    <div ref={ref} style={{
      position: 'relative',
      height: 220,
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      borderTop: `1px solid ${theme.line}`,
      borderBottom: `1px solid ${theme.line}`,
      paddingTop: 40, paddingBottom: 40,
    }}>
      {/* The three dots — drift apart subtly when seen */}
      <div style={{ position: 'relative', width: 360, height: 100 }}>
        {[0, 1, 2].map(i => {
          const positions = ['12%', '50%', '88%'];
          // When seen, dots float subtly outward
          const driftX = seen ? [-18, 0, 18][i] : 0;
          const driftY = seen ? [-4, 0, 4][i] : 0;
          return (
            <div key={i} style={{
              position: 'absolute',
              left: positions[i], top: '50%',
              width: i === 1 ? 32 : 22, height: i === 1 ? 32 : 22,
              borderRadius: '50%',
              background: theme.ink,
              transform: `translate(-50%, -50%) translate(${driftX}px, ${driftY}px)`,
              transition: 'transform 2.4s cubic-bezier(0.2,0.8,0.2,1)',
              opacity: 0.92,
            }}/>
          );
        })}
        <div style={{
          position: 'absolute', bottom: -32, left: 0, right: 0,
          textAlign: 'center',
          fontFamily: '"Fraunces", serif', fontStyle: 'italic',
          fontSize: 14, color: theme.inkMuted,
          letterSpacing: 0.5,
        }}>
          composed in studio Δ · MMXXVI
        </div>
      </div>
    </div>
  );
}

Object.assign(window, {
  FolioField, FolioWorks, FolioLineage, FolioContact,
});
