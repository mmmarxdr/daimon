// Studio Δ · Folio Nº 01 — tokens + shared atoms
// Editorial-magazine palette layered over the Daimon design system.

const FOLIO_TOKENS = {
  parchment: {
    // Warm, like Open Design's #efe7d2 but with our ink
    paper:      '#efe7d2',
    paperElev:  '#f5eedb',
    paperDeep:  '#e3d9bf',
    paperWash:  '#e8dfc6',
    ink:        '#1a1813',
    inkSoft:    '#3d3a32',
    inkMuted:   '#7a7465',
    inkFaint:   '#b5a98c',
    line:       'rgba(26,24,19,0.10)',
    lineStrong: 'rgba(26,24,19,0.20)',
    accent:     '#2d8573',     // Daimon teal (for "live/active")
    accentSoft: 'rgba(45,133,115,0.10)',
    editorial:  '#1f3a5f',     // Ink-blue for links/editorial accents
    editorialSoft: 'rgba(31,58,95,0.08)',
    rubric:     '#9a3a2a',     // Rust red — for plate numbers, drop caps
    grain:      'rgba(26,24,19,0.022)',
  },
  cold: {
    paper:      '#f7f5f0',
    paperElev:  '#ffffff',
    paperDeep:  '#ece9e2',
    paperWash:  '#f1eee7',
    ink:        '#0f0e0c',
    inkSoft:    '#2d2b27',
    inkMuted:   '#6e6a60',
    inkFaint:   '#bcb6a8',
    line:       'rgba(15,14,12,0.08)',
    lineStrong: 'rgba(15,14,12,0.18)',
    accent:     '#2d8573',
    accentSoft: 'rgba(45,133,115,0.08)',
    editorial:  '#1f3a5f',
    editorialSoft: 'rgba(31,58,95,0.07)',
    rubric:     '#9a3a2a',
    grain:      'rgba(15,14,12,0.018)',
  },
};

// ─────────────────────────────────────────────────────────────
// useScrollY — element scroll position normalized 0..1
// ─────────────────────────────────────────────────────────────
function useScrollProgress(ref, scrollerRef) {
  const [p, setP] = React.useState(0);
  React.useEffect(() => {
    const el = ref.current;
    const scroller = scrollerRef?.current || window;
    if (!el) return;
    const calc = () => {
      const r = el.getBoundingClientRect();
      const sr = scroller === window
        ? { top: 0, height: window.innerHeight }
        : scroller.getBoundingClientRect();
      // 0 when section's top hits scroller bottom; 1 when section's bottom hits scroller top
      const start = sr.top + sr.height;
      const end = sr.top - r.height;
      const t = (start - r.top) / (start - end);
      setP(Math.max(0, Math.min(1, t)));
    };
    calc();
    const onScroll = () => requestAnimationFrame(calc);
    scroller.addEventListener('scroll', onScroll, { passive: true });
    window.addEventListener('resize', onScroll);
    return () => {
      scroller.removeEventListener('scroll', onScroll);
      window.removeEventListener('resize', onScroll);
    };
  }, [ref, scrollerRef]);
  return p;
}

// ─────────────────────────────────────────────────────────────
// Plate / folio metadata strip — appears at section starts
// ─────────────────────────────────────────────────────────────
function FolioMark({ theme, roman, label, subtitle, page, total = 8 }) {
  return (
    <div className="folio-mark" style={{
      display: 'flex', alignItems: 'baseline', gap: 16,
      fontFamily: '"JetBrains Mono", monospace',
      fontSize: 11, letterSpacing: 0.6, textTransform: 'uppercase',
      color: theme.inkMuted,
      paddingBottom: 14,
      borderBottom: `1px solid ${theme.line}`,
      marginBottom: 48,
    }}>
      <span style={{
        color: theme.rubric, fontWeight: 600, fontSize: 12,
        fontFamily: '"Fraunces", serif', fontStyle: 'italic',
        letterSpacing: 0,
      }}>{roman}.</span>
      <span style={{ color: theme.ink, fontWeight: 500 }}>{label}</span>
      <span style={{ flex: 1, height: 1, background: theme.line, transform: 'translateY(-3px)' }}/>
      {subtitle && <span style={{ color: theme.inkMuted }}>{subtitle}</span>}
      <span style={{ color: theme.inkFaint }}>{String(page).padStart(3,'0')} / {String(total).padStart(3,'0')}</span>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// EditorialKicker — small label above section titles
// ─────────────────────────────────────────────────────────────
function Kicker({ theme, n, children }) {
  return (
    <div style={{
      display: 'flex', alignItems: 'baseline', gap: 10,
      fontFamily: '"JetBrains Mono", monospace',
      fontSize: 11, letterSpacing: 1.2, textTransform: 'uppercase',
      color: theme.inkMuted,
      marginBottom: 28,
    }}>
      {n && <span style={{ color: theme.rubric, fontWeight: 600 }}>Nº {n}</span>}
      <span style={{ color: theme.inkSoft }}>{children}</span>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// EditorialTitle — Fraunces display with italic mix support
// Pass children as array: ['Skills, ', {italic: 'systems'}, ' and surfaces']
// or as JSX with <em>…</em> tags.
// ─────────────────────────────────────────────────────────────
function FoTitle({ theme, size = 84, children, style = {} }) {
  return (
    <h2 className="folio-title-lg" style={{
      margin: 0,
      fontFamily: '"Fraunces", serif',
      fontWeight: 350,
      fontSize: size,
      lineHeight: 1.05,
      letterSpacing: -1.5,
      color: theme.ink,
      textWrap: 'pretty',
      maxWidth: '18ch',
      ...style,
    }}>{children}</h2>
  );
}

// ─────────────────────────────────────────────────────────────
// Lede — subtitle paragraph under title
// ─────────────────────────────────────────────────────────────
function Lede({ theme, children, style = {} }) {
  return (
    <p style={{
      margin: 0, marginTop: 24,
      fontFamily: '"Inter", system-ui, sans-serif',
      fontSize: 18, lineHeight: 1.6,
      color: theme.inkSoft,
      maxWidth: '52ch',
      textWrap: 'pretty',
      ...style,
    }}>{children}</p>
  );
}

// ─────────────────────────────────────────────────────────────
// SplitFlap — counter that flips digits when the value changes.
// Used for paginas/sections.
// ─────────────────────────────────────────────────────────────
function SplitFlap({ value, theme, size = 11 }) {
  const [shown, setShown] = React.useState(value);
  const [flipping, setFlipping] = React.useState(false);
  React.useEffect(() => {
    if (value === shown) return;
    setFlipping(true);
    const t = setTimeout(() => { setShown(value); setFlipping(false); }, 260);
    return () => clearTimeout(t);
  }, [value, shown]);
  return (
    <span style={{
      display: 'inline-block', position: 'relative',
      fontFamily: '"JetBrains Mono", monospace', fontSize: size,
      color: theme.ink, minWidth: '3ch', textAlign: 'right',
      transform: flipping ? 'rotateX(50deg)' : 'rotateX(0deg)',
      transition: 'transform 0.26s cubic-bezier(0.6,0.05,0.4,0.95)',
      transformOrigin: 'center',
    }}>{String(shown).padStart(3,'0')}</span>
  );
}

// ─────────────────────────────────────────────────────────────
// FootnoteRule — small horizontal rule used between editorial blocks
// ─────────────────────────────────────────────────────────────
function HRule({ theme, style = {} }) {
  return <div style={{ height: 1, background: theme.line, ...style }}/>;
}

// ─────────────────────────────────────────────────────────────
// MetaRow — bullet-separated taxonomy
// items: ['Local-first', 'BYOM', 'MIT']
// ─────────────────────────────────────────────────────────────
function MetaRow({ theme, items = [], style = {}, accent = false }) {
  return (
    <div style={{
      fontFamily: '"JetBrains Mono", monospace',
      fontSize: 11, letterSpacing: 0.5,
      color: theme.inkMuted,
      display: 'flex', flexWrap: 'wrap', gap: '6px 0',
      ...style,
    }}>
      {items.map((it, i) => (
        <React.Fragment key={i}>
          {i > 0 && <span style={{ color: theme.inkFaint, padding: '0 10px' }}>·</span>}
          <span style={{ color: accent && i === 0 ? theme.accent : 'inherit' }}>{it}</span>
        </React.Fragment>
      ))}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Drop cap — first letter rendered large
// ─────────────────────────────────────────────────────────────
function DropCap({ theme, letter, animated = true, color }) {
  const ref = React.useRef(null);
  const seen = useInView(ref, 0.3);
  const c = color || theme.rubric;
  return (
    <span ref={ref} style={{
      fontFamily: '"Fraunces", serif',
      fontWeight: 300,
      fontSize: 96,
      lineHeight: 0.85,
      float: 'left',
      paddingRight: 14,
      paddingTop: 6,
      color: c,
      // The "ink runs into the letter" effect: we mask a gradient that
      // sweeps from top to bottom, controlled by the seen state.
      backgroundImage: animated
        ? `linear-gradient(180deg, ${c} 0%, ${c} 50%, ${theme.inkFaint} 50.1%, ${theme.inkFaint} 100%)`
        : 'none',
      backgroundSize: '100% 200%',
      backgroundPosition: seen ? '0% 0%' : '0% 100%',
      WebkitBackgroundClip: animated ? 'text' : 'initial',
      WebkitTextFillColor: animated ? 'transparent' : c,
      backgroundClip: animated ? 'text' : 'initial',
      transition: 'background-position 1.4s cubic-bezier(0.6,0,0.3,1) 0.1s',
    }}>{letter}</span>
  );
}

// ─────────────────────────────────────────────────────────────
// CharStagger — letter-by-letter reveal of a string. For section titles.
// ─────────────────────────────────────────────────────────────
function CharStagger({ children, delay = 0, charDelay = 0.025, y = 24, style = {}, threshold = 0.2 }) {
  const ref = React.useRef(null);
  const seen = useInView(ref, threshold);
  const text = React.Children.toArray(children);
  // Recursively wrap text nodes into per-character spans
  let i = 0;
  const wrap = (node) => {
    if (typeof node === 'string') {
      return Array.from(node).map((ch, k) => {
        const idx = i++;
        return (
          <span key={`c${idx}-${k}`} style={{
            display: 'inline-block',
            opacity: seen ? 1 : 0,
            transform: seen ? 'none' : `translateY(${y}px)`,
            transition: `opacity 0.7s cubic-bezier(0.2,0.8,0.2,1) ${delay + idx * charDelay}s, transform 0.7s cubic-bezier(0.2,0.8,0.2,1) ${delay + idx * charDelay}s`,
            whiteSpace: ch === ' ' ? 'pre' : 'normal',
          }}>{ch}</span>
        );
      });
    }
    if (React.isValidElement(node)) {
      return React.cloneElement(node, { key: `e${i++}` }, React.Children.toArray(node.props.children).map(wrap));
    }
    return node;
  };
  return (
    <span ref={ref} style={{ display: 'inline-block', ...style }}>
      {text.map(wrap)}
    </span>
  );
}

Object.assign(window, {
  FOLIO_TOKENS, useScrollProgress,
  FolioMark, Kicker, FoTitle, Lede, SplitFlap, HRule, MetaRow,
  DropCap, CharStagger,
});
