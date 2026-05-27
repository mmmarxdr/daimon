// Daimon Landing — Studio Δ folio assembly + Tweaks panel.

const { useState, useRef, useEffect, useMemo } = React;

const TWEAKS_DEFAULTS = /*EDITMODE-BEGIN*/{
  "paper": "parchment",
  "accent": "teal-editorial",
  "dropCaps": true,
  "animation": "lively",
  "ticker": true,
  "paperFall": true,
  "grain": true
}/*EDITMODE-END*/;

function buildTheme(t) {
  const base = t.paper === 'cold' ? FOLIO_TOKENS.cold : FOLIO_TOKENS.parchment;
  // Accent strategy:
  //  teal-editorial: teal accent for actions, ink-blue for editorial/links (current default)
  //  teal-only:      teal everywhere (closer to Daimon product feel)
  //  ink-only:       ink-blue everywhere (most editorial)
  //  rust:           rust-red editorial, teal accent
  let accent = base.accent;
  let editorial = base.editorial;
  let rubric = base.rubric;
  if (t.accent === 'teal-only') { editorial = base.accent; }
  if (t.accent === 'ink-only')  { accent = base.editorial; }
  if (t.accent === 'rust')      { editorial = base.rubric; }
  return { ...base, accent, editorial, rubric };
}

// Hook: detect mobile viewport
function useIsMobile(bp = 768) {
  const [m, setM] = React.useState(typeof window !== 'undefined' && window.innerWidth <= bp);
  React.useEffect(() => {
    const onR = () => setM(window.innerWidth <= bp);
    window.addEventListener('resize', onR);
    return () => window.removeEventListener('resize', onR);
  }, [bp]);
  return m;
}

// Sticky folio counter — top-right edge. Tracks which section is in view
// and updates with a split-flap effect.
function FolioCounter({ theme, sections, scrollerRef }) {
  const [activeIdx, setActiveIdx] = useState(0);
  useEffect(() => {
    const scroller = scrollerRef.current;
    if (!scroller) return;
    const onScroll = () => {
      const sr = scroller.getBoundingClientRect();
      const focal = sr.top + sr.height * 0.35;
      let bestIdx = 0, bestDist = Infinity;
      sections.forEach((sec, i) => {
        const r = sec.ref.current?.getBoundingClientRect();
        if (!r) return;
        const cy = r.top + 120;
        const d = Math.abs(cy - focal);
        if (r.top <= focal && d < bestDist) { bestDist = d; bestIdx = i; }
      });
      setActiveIdx(bestIdx);
    };
    scroller.addEventListener('scroll', onScroll, { passive: true });
    onScroll();
    return () => scroller.removeEventListener('scroll', onScroll);
  }, [sections, scrollerRef]);

  const active = sections[activeIdx];
  const total = sections.length;
  const isMobile = useIsMobile();

  if (isMobile) {
    return (
      <div style={{
        position: 'fixed', bottom: 12, left: '50%', transform: 'translateX(-50%)',
        display: 'flex', alignItems: 'center', gap: 10,
        padding: '8px 14px',
        background: theme.paperElev,
        border: `1px solid ${theme.line}`,
        borderRadius: 999,
        pointerEvents: 'auto', zIndex: 30,
        fontFamily: '"JetBrains Mono", monospace',
        boxShadow: '0 4px 16px rgba(0,0,0,0.06)',
      }}>
        <span style={{
          color: theme.rubric, fontWeight: 600, fontSize: 11,
          fontFamily: '"Fraunces", serif', fontStyle: 'italic',
        }}>{active.roman}.</span>
        <span style={{
          textTransform: 'uppercase', fontSize: 10, letterSpacing: 0.8,
          color: theme.ink, fontWeight: 500,
        }}>{active.label}</span>
        <span style={{ fontSize: 9.5, color: theme.inkMuted, letterSpacing: 0.4 }}>
          {String(activeIdx + 1).padStart(2, '0')}/{String(total).padStart(2, '0')}
        </span>
        <div style={{ display: 'flex', gap: 4, marginLeft: 4 }}>
          {sections.map((_, i) => (
            <span key={i} style={{
              width: 4, height: 4, borderRadius: '50%',
              background: i === activeIdx ? theme.editorial : theme.line,
              transition: 'all 0.3s',
              transform: i === activeIdx ? 'scale(1.4)' : 'scale(1)',
            }}/>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div style={{
      position: 'absolute', top: 280, right: 32,
      width: 38,
      display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 14,
      pointerEvents: 'auto', zIndex: 30,
      fontFamily: '"JetBrains Mono", monospace',
      writingMode: 'vertical-rl',
      transform: 'rotate(180deg)',
    }}>
      <span style={{
        writingMode: 'horizontal-tb', transform: 'rotate(180deg)',
        fontSize: 11, color: theme.rubric, fontWeight: 600,
        letterSpacing: 0.4,
        fontFamily: '"Fraunces", serif', fontStyle: 'italic',
      }}>
        {active.roman}.
      </span>
      <span style={{
        writingMode: 'horizontal-tb', transform: 'rotate(180deg)',
        textTransform: 'uppercase', fontSize: 10, letterSpacing: 1.2,
        color: theme.ink, fontWeight: 500,
      }}>{active.label}</span>
      <span style={{
        writingMode: 'horizontal-tb', transform: 'rotate(180deg)',
        fontSize: 10, color: theme.inkMuted, letterSpacing: 0.4,
      }}>
        {String(activeIdx + 1).padStart(2, '0')} / {String(total).padStart(2, '0')}
      </span>
      {/* Progress dots */}
      <div style={{
        writingMode: 'horizontal-tb', transform: 'rotate(180deg)',
        display: 'flex', flexDirection: 'column', gap: 6,
        marginTop: 8,
      }}>
        {sections.map((_, i) => (
          <span key={i} style={{
            width: 4, height: 4, borderRadius: '50%',
            background: i === activeIdx ? theme.editorial : theme.line,
            transition: 'all 0.3s',
            transform: i === activeIdx ? 'scale(1.4)' : 'scale(1)',
          }}/>
        ))}
      </div>
    </div>
  );
}

// Grain overlay
function FolioGrain({ theme }) {
  return (
    <div aria-hidden style={{
      position: 'absolute', inset: 0, pointerEvents: 'none', zIndex: 1,
      opacity: 1,
      backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='200' height='200'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='2' /%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)' opacity='0.45'/%3E%3C/svg%3E")`,
      mixBlendMode: 'multiply',
      filter: 'contrast(1.1)',
    }}/>
  );
}

function FolioApp() {
  const [t, setTweak] = useTweaks(TWEAKS_DEFAULTS);
  const theme = useMemo(() => buildTheme(t), [t.paper, t.accent]);

  const scrollerRef = useRef(null);

  // Sections with refs — for the counter
  const refs = {
    cover:        useRef(null),
    manifesto:    useRef(null),
    capabilities: useRef(null),
    method:       useRef(null),
    field:        useRef(null),
    works:        useRef(null),
    lineage:      useRef(null),
    contact:      useRef(null),
  };

  const sectionsList = [
    { id: 'cover',        roman: '0',   label: 'Cover',        ref: refs.cover },
    { id: 'manifesto',    roman: 'I',   label: 'Manifesto',    ref: refs.manifesto },
    { id: 'capabilities', roman: 'II',  label: 'Capabilities', ref: refs.capabilities },
    { id: 'method',       roman: 'III', label: 'Method',       ref: refs.method },
    { id: 'field',        roman: 'IV',  label: 'Field',        ref: refs.field },
    { id: 'works',        roman: 'V',   label: 'Works',        ref: refs.works },
    { id: 'lineage',      roman: 'VI',  label: 'Lineage',      ref: refs.lineage },
    { id: 'contact',      roman: 'VII', label: 'Contact',      ref: refs.contact },
  ];

  // Inject editorial styles
  return (
    <div style={{
      width: '100%', minHeight: '100vh',
      background: theme.paper,
      color: theme.ink,
      fontFamily: '"Inter", system-ui, sans-serif',
      position: 'relative',
    }}>
      <FolioStyles />
      {t.grain && <FolioGrain theme={theme} />}

      <div ref={scrollerRef} style={{
        height: '100vh', overflowY: 'auto', overflowX: 'hidden',
        position: 'relative',
      }}>
        <div ref={refs.cover}>
          <FolioCover theme={theme} scrollerRef={scrollerRef} animLevel={t.animation} paperFall={t.paperFall} />
        </div>
        <div ref={refs.manifesto}>
          <FolioManifesto theme={theme} scrollerRef={scrollerRef} dropCaps={t.dropCaps} />
        </div>
        <div ref={refs.capabilities}>
          <FolioCapabilities theme={theme} scrollerRef={scrollerRef} />
        </div>
        <div ref={refs.method}>
          <FolioMethod theme={theme} scrollerRef={scrollerRef} />
        </div>
        <div ref={refs.field}>
          <FolioField theme={theme} scrollerRef={scrollerRef} showTicker={t.ticker} />
        </div>
        <div ref={refs.works}>
          <FolioWorks theme={theme} scrollerRef={scrollerRef} />
        </div>
        <div ref={refs.lineage}>
          <FolioLineage theme={theme} scrollerRef={scrollerRef} />
        </div>
        <div ref={refs.contact}>
          <FolioContact theme={theme} scrollerRef={scrollerRef} />
        </div>

        <FolioCounter theme={theme} sections={sectionsList} scrollerRef={scrollerRef} />
      </div>

      <TweaksPanel title="Tweaks">
        <TweakSection label="Paper">
          <TweakRadio
            value={t.paper}
            options={[
              { value: 'parchment', label: 'Parchment' },
              { value: 'cold',      label: 'Cold white' },
            ]}
            onChange={(v) => setTweak('paper', v)}
          />
        </TweakSection>

        <TweakSection label="Accent strategy">
          <TweakSelect
            value={t.accent}
            options={[
              { value: 'teal-editorial', label: 'Teal + ink-blue (default)' },
              { value: 'teal-only',      label: 'Teal only' },
              { value: 'ink-only',       label: 'Ink-blue only' },
              { value: 'rust',           label: 'Teal + rust' },
            ]}
            onChange={(v) => setTweak('accent', v)}
          />
        </TweakSection>

        <TweakSection label="Editorial">
          <TweakToggle  label="Drop caps"       value={t.dropCaps} onChange={(v) => setTweak('dropCaps', v)} />
          <TweakToggle  label="Field ticker"    value={t.ticker}   onChange={(v) => setTweak('ticker', v)} />
          <TweakToggle  label="Paper-fall hero" value={t.paperFall} onChange={(v) => setTweak('paperFall', v)} />
          <TweakToggle  label="Paper grain"     value={t.grain}    onChange={(v) => setTweak('grain', v)} />
        </TweakSection>

        <TweakSection label="Animation">
          <TweakRadio
            value={t.animation}
            options={[
              { value: 'calm',   label: 'Calm' },
              { value: 'lively', label: 'Lively' },
            ]}
            onChange={(v) => setTweak('animation', v)}
          />
        </TweakSection>
      </TweaksPanel>
    </div>
  );
}

function FolioStyles() {
  return (
    <style>{`
      @keyframes pulseRing { 0% { transform: scale(0.95); opacity: 0.5; } 100% { transform: scale(2.4); opacity: 0; } }
      @keyframes glyphBreathe { 0%,100% { transform: scale(1); opacity: 0.92; } 50% { transform: scale(1.1); opacity: 1; } }
      @keyframes inkBleed { 0%,100% { opacity: 0.92; } 50% { opacity: 1; } }
      @keyframes typeIn { from { width: 0; } to { width: 100%; } }
      @keyframes drift { 0%,100% { transform: translate(0,0); } 50% { transform: translate(8px, -12px); } }
      @keyframes rotateSlow { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }

      html { scroll-behavior: smooth; }
      body { margin: 0; }
      ::selection { background: rgba(31,58,95,0.18); color: inherit; }

      /* Custom scrollbar styling for the inner scroller */
      ::-webkit-scrollbar { width: 10px; height: 10px; }
      ::-webkit-scrollbar-track { background: transparent; }
      ::-webkit-scrollbar-thumb { background: rgba(0,0,0,0.12); border-radius: 0; border: 3px solid transparent; background-clip: content-box; }
      ::-webkit-scrollbar-thumb:hover { background: rgba(0,0,0,0.22); border: 3px solid transparent; background-clip: content-box; }

      /* ── Responsive · ≤768px ─────────────────────────── */
      @media (max-width: 768px) {
        /* Section padding — collapse generous editorial breathing room */
        section[data-screen-label] {
          padding-left: 20px !important;
          padding-right: 20px !important;
          padding-top: 64px !important;
          padding-bottom: 64px !important;
        }
        section[data-screen-label="00 Cover"] {
          padding-top: 28px !important;
          padding-bottom: 56px !important;
          min-height: auto !important;
        }
        section[data-screen-label="04 Field"] {
          padding-left: 0 !important;
          padding-right: 0 !important;
        }
        section[data-screen-label="04 Field"] .folio-inner-pad,
        section[data-screen-label="04 Field"] > div {
          padding-left: 20px !important;
          padding-right: 20px !important;
        }

        /* All editorial 2-col grids → single column */
        section[data-screen-label] [class~="folio-grid"],
        section[data-screen-label] > div > div[style*="grid-template-columns"] {
          grid-template-columns: 1fr !important;
          gap: 32px !important;
        }

        /* Cover: collapse hero grid */
        .folio-cover-hero { grid-template-columns: 1fr !important; gap: 36px !important; }
        .folio-cover-title { font-size: 48px !important; letter-spacing: -1.4px !important; line-height: 1 !important; }
        .folio-cover-lede  { font-size: 16px !important; }
        .folio-cover-stats { grid-template-columns: 1fr 1fr 1fr !important; gap: 16px !important; margin-top: 40px !important; }
        .folio-cover-stats > div > div:first-child { font-size: 28px !important; }
        .folio-cover-ctas  { flex-direction: column !important; align-items: stretch !important; }
        .folio-cover-ctas button { width: 100% !important; justify-content: center !important; }
        .folio-masthead { flex-wrap: wrap !important; row-gap: 6px !important; font-size: 10px !important; }
        .folio-masthead > span:nth-child(3) { display: none !important; }
        .folio-subnav { flex-wrap: wrap !important; gap: 14px !important; row-gap: 8px !important; padding-bottom: 20px !important; }
        .folio-hero-plate-wrap { min-height: 380px !important; }

        /* Section titles */
        .folio-title-lg { font-size: 40px !important; letter-spacing: -0.6px !important; max-width: 100% !important; }

        /* FolioMark — tighten */
        .folio-mark { flex-wrap: wrap !important; row-gap: 6px !important; margin-bottom: 32px !important; font-size: 10px !important; }
        .folio-mark > span:nth-child(3) { display: none !important; }

        /* Capabilities / Works grids */
        .folio-cap-grid  { grid-template-columns: 1fr !important; }
        .folio-work-grid { grid-template-columns: 1fr !important; }

        /* Method stages: stack vertically */
        .folio-method-grid { grid-template-columns: 1fr !important; gap: 28px !important; }
        .folio-method-svg  { display: none !important; }
        .folio-method-stage { padding-top: 0 !important; text-align: left !important; }
        .folio-method-stage > div { text-align: left !important; }
        .folio-method-stage .folio-method-dot {
          position: relative !important; left: 0 !important; transform: none !important;
          display: inline-block; margin-right: 10px; vertical-align: middle;
        }

        /* Field map: hide tiny text labels for legibility */
        .folio-field-map svg text { display: none; }

        /* Lineage / Contact grids */
        .folio-lineage-grid { grid-template-columns: 1fr !important; }
        .folio-contact-grid { grid-template-columns: 1fr !important; }
        .folio-footer-grid  { grid-template-columns: 1fr 1fr !important; }
        .folio-install code { font-size: 13px !important; word-break: break-all; }

        /* Cards: shrink min-heights so content doesn't float in empty space */
        .folio-cap-grid > div { min-height: auto !important; padding: 24px 22px !important; }
        .folio-cap-grid > div > div:last-child { position: static !important; margin-top: 16px !important; }
        .folio-work-grid > a { min-height: auto !important; padding: 28px 22px !important; }

        /* HeroPlate: hide busy chrome that doesn't read at small sizes */
        .folio-hero-plate-wrap [data-stage-markers] { display: none !important; }

        /* Pull-quote */
        .folio-pullquote { font-size: 32px !important; letter-spacing: -0.6px !important; padding-left: 20px !important; }
        .folio-pullquote-mark { font-size: 76px !important; }

        /* Footer FIN */
        .folio-fin { flex-wrap: wrap !important; row-gap: 8px !important; }
      }
    `}</style>
  );
}

window.useIsMobile = useIsMobile;

ReactDOM.createRoot(document.getElementById('root')).render(<FolioApp />);
