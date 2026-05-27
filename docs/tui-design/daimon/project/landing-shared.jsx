// Landing shared tokens + primitives.
// Reuses Liminal palette but adds landing-scale scales.

const LANDING_TOKENS = {
  light: {
    ...LIMINAL_TOKENS.light,
    bg:        '#f8f6f1',
    bgElev:    '#ffffff',
    bgDeep:    '#efece5',
    wash:      '#f3f0e9',
    grain:     'rgba(26,24,19,0.015)',
    shadow:    '0 1px 2px rgba(26,24,19,0.06), 0 8px 32px rgba(26,24,19,0.04)',
  },
  dark: {
    ...LIMINAL_TOKENS.dark,
    bg:        '#0b0a08',
    bgElev:    '#141210',
    bgDeep:    '#050504',
    wash:      '#100f0d',
    grain:     'rgba(234,229,216,0.02)',
    shadow:    '0 1px 2px rgba(0,0,0,0.4), 0 24px 80px rgba(0,0,0,0.5)',
  },
};

// ─────────────────────────────────────────────────────────────
// useMousePos — smoothed normalized mouse position relative to an element
// ─────────────────────────────────────────────────────────────
function useMousePos(ref) {
  const [pos, setPos] = React.useState({ x: 0.5, y: 0.5, vx: 0, vy: 0 });
  const raf = React.useRef(null);
  const target = React.useRef({ x: 0.5, y: 0.5 });
  const current = React.useRef({ x: 0.5, y: 0.5 });

  React.useEffect(() => {
    const el = ref.current; if (!el) return;
    const onMove = (e) => {
      const r = el.getBoundingClientRect();
      target.current = {
        x: Math.max(0, Math.min(1, (e.clientX - r.left) / r.width)),
        y: Math.max(0, Math.min(1, (e.clientY - r.top) / r.height)),
      };
    };
    const onLeave = () => { target.current = { x: 0.5, y: 0.5 }; };
    el.addEventListener('mousemove', onMove);
    el.addEventListener('mouseleave', onLeave);

    const tick = () => {
      const t = target.current, c = current.current;
      const nx = c.x + (t.x - c.x) * 0.12;
      const ny = c.y + (t.y - c.y) * 0.12;
      const vx = nx - c.x, vy = ny - c.y;
      current.current = { x: nx, y: ny };
      setPos({ x: nx, y: ny, vx, vy });
      raf.current = requestAnimationFrame(tick);
    };
    tick();
    return () => {
      el.removeEventListener('mousemove', onMove);
      el.removeEventListener('mouseleave', onLeave);
      cancelAnimationFrame(raf.current);
    };
  }, [ref]);

  return pos;
}

// ─────────────────────────────────────────────────────────────
// useInView — fire once when element crosses viewport threshold
// ─────────────────────────────────────────────────────────────
function useInView(ref, threshold = 0.2) {
  const [seen, setSeen] = React.useState(false);
  React.useEffect(() => {
    const el = ref.current; if (!el) return;
    const io = new IntersectionObserver(([e]) => {
      if (e.isIntersecting) { setSeen(true); io.disconnect(); }
    }, { threshold });
    io.observe(el);
    return () => io.disconnect();
  }, [ref, threshold]);
  return seen;
}

// ─────────────────────────────────────────────────────────────
// Reveal — fade+rise on enter viewport
// ─────────────────────────────────────────────────────────────
function Reveal({ children, delay = 0, y = 20, style = {} }) {
  const ref = React.useRef(null);
  const seen = useInView(ref, 0.15);
  return (
    <div ref={ref} style={{
      opacity: seen ? 1 : 0,
      transform: seen ? 'none' : `translateY(${y}px)`,
      transition: `opacity 0.9s cubic-bezier(0.2,0.8,0.2,1) ${delay}s, transform 0.9s cubic-bezier(0.2,0.8,0.2,1) ${delay}s`,
      willChange: 'opacity, transform',
      ...style,
    }}>{children}</div>
  );
}

// ─────────────────────────────────────────────────────────────
// TopNav — shared nav bar
// ─────────────────────────────────────────────────────────────
function LandingNav({ theme, isDark, onToggle, direction }) {
  return (
    <div style={{
      position: 'sticky', top: 0, zIndex: 50,
      padding: '18px 40px',
      display: 'flex', alignItems: 'center', gap: 24,
      background: `${theme.bg}ee`,
      backdropFilter: 'blur(10px)',
      WebkitBackdropFilter: 'blur(10px)',
      borderBottom: `1px solid ${theme.line}`,
      fontFamily: '"Inter", system-ui, sans-serif',
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <span style={{
          fontFamily: '"JetBrains Mono", monospace',
          fontSize: 22, color: theme.accent, lineHeight: 0.9,
          display: 'inline-block', transform: 'translateY(-2px)',
        }}>⫶</span>
        <span style={{ fontSize: 15, fontWeight: 600, color: theme.ink, letterSpacing: -0.2 }}>
          Daimon
        </span>
        <span style={{
          fontFamily: '"Fraunces", serif', fontStyle: 'italic',
          fontSize: 12, color: theme.inkMuted, marginLeft: 2,
        }}>{direction === 'daimon' ? '— an agent who listens' : '— the open agent'}</span>
      </div>
      <span style={{ flex: 1 }}/>
      <div style={{ display: 'flex', gap: 22, fontSize: 12.5, color: theme.inkSoft }}>
        {['Features', 'Compare', 'Install', 'Docs', 'GitHub'].map(l => (
          <a key={l} href="#" style={{ color: 'inherit', textDecoration: 'none' }}>{l}</a>
        ))}
      </div>
      <button onClick={onToggle} title="Toggle theme" style={{
        width: 28, height: 28, borderRadius: 99,
        background: theme.bgElev, border: `1px solid ${theme.line}`,
        cursor: 'pointer', color: theme.ink,
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        fontSize: 13, padding: 0,
      }}>{isDark ? '☾' : '☀'}</button>
      <button style={{
        padding: '7px 14px', borderRadius: 4,
        background: theme.ink, color: theme.bg,
        border: 'none', cursor: 'pointer', fontSize: 12.5, fontWeight: 500,
        fontFamily: 'inherit',
      }}>Get started →</button>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Grain overlay — adds texture, kills the "too clean" feel
// ─────────────────────────────────────────────────────────────
function Grain({ opacity = 0.04 }) {
  return (
    <div aria-hidden style={{
      position: 'absolute', inset: 0, pointerEvents: 'none', zIndex: 2,
      opacity,
      backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='200' height='200'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='2' /%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)' opacity='0.6'/%3E%3C/svg%3E")`,
      mixBlendMode: 'overlay',
    }}/>
  );
}

// ─────────────────────────────────────────────────────────────
// Animated wordmark — the ⫶ glyph alive, follows mouse subtly
// ─────────────────────────────────────────────────────────────
function WordmarkAlive({ theme, size = 120, mouseX = 0.5, mouseY = 0.5, breathing = true }) {
  // Three dots vertically. The middle pulses with breath, the outer two
  // tilt slightly toward the mouse. All glow softly.
  const tilt = (mouseX - 0.5) * 12; // degrees
  const nudge = (mouseY - 0.5) * size * 0.08;
  return (
    <div style={{
      position: 'relative',
      width: size, height: size * 1.4,
      display: 'flex', flexDirection: 'column',
      alignItems: 'center', justifyContent: 'center',
      gap: size * 0.15,
      transform: `rotate(${tilt * 0.3}deg) translateY(${nudge}px)`,
      transition: 'transform 0.3s cubic-bezier(0.2,0.8,0.2,1)',
    }}>
      {/* Top dot */}
      <div style={{
        width: size * 0.22, height: size * 0.22, borderRadius: '50%',
        background: theme.accent,
        boxShadow: `0 0 ${size * 0.4}px ${theme.accent}88`,
        opacity: 0.92,
        animation: breathing ? 'glyphBreathe 4s ease-in-out infinite' : 'none',
      }}/>
      {/* Middle bar — presence */}
      <div style={{
        width: size * 0.14, height: size * 0.56,
        background: theme.accent,
        borderRadius: size * 0.07,
        boxShadow: `0 0 ${size * 0.5}px ${theme.accent}77`,
        opacity: 0.98,
        animation: breathing ? 'glyphBreathe2 4s ease-in-out infinite' : 'none',
      }}/>
      {/* Bottom dot */}
      <div style={{
        width: size * 0.22, height: size * 0.22, borderRadius: '50%',
        background: theme.accent,
        boxShadow: `0 0 ${size * 0.4}px ${theme.accent}88`,
        opacity: 0.92,
        animation: breathing ? 'glyphBreathe 4s ease-in-out infinite 0.4s' : 'none',
      }}/>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Global landing styles + keyframes
// ─────────────────────────────────────────────────────────────
function LandingStyles() {
  return (
    <style>{`
      @keyframes glyphBreathe { 0%,100% { transform: scale(1); opacity: 0.92; } 50% { transform: scale(1.15); opacity: 1; } }
      @keyframes glyphBreathe2 { 0%,100% { transform: scaleY(1); opacity: 0.98; } 50% { transform: scaleY(1.06); opacity: 1; } }
      @keyframes caretBlink { 0%,49% { opacity: 1; } 50%,100% { opacity: 0; } }
      @keyframes slideIn { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: none; } }
      @keyframes drift { 0%,100% { transform: translate(0,0); } 50% { transform: translate(8px, -12px); } }
      @keyframes auroraShift { 0% { transform: translate(0%,0%) rotate(0deg); } 50% { transform: translate(-5%,3%) rotate(180deg); } 100% { transform: translate(0%,0%) rotate(360deg); } }
      @keyframes pulseRing { 0% { transform: scale(0.95); opacity: 0.5; } 100% { transform: scale(1.8); opacity: 0; } }
      @keyframes typeIn { from { width: 0; } to { width: 100%; } }
      @keyframes textShimmer { 0%,100% { background-position: 0% 50%; } 50% { background-position: 100% 50%; } }

      /* Page-turn wipe: sheet covers the viewport, then slides away
         toward the bottom-left with a subtle rotation, like flipping a
         page from right to left. */
      @keyframes pageTurn {
        0%   { transform: translate(0%, 0%) rotate(0deg); opacity: 1; }
        100% { transform: translate(-110%, 110%) rotate(-12deg); opacity: 1; }
      }
      @keyframes pageTurnEdge {
        0% { transform: translateX(100%); opacity: 0; }
        15% { opacity: 1; }
        85% { opacity: 1; }
        100% { transform: translateX(-100%); opacity: 0; }
      }
      html { scroll-behavior: smooth; }
      ::selection { background: rgba(93,191,167,0.35); color: inherit; }
    `}</style>
  );
}

Object.assign(window, {
  LANDING_TOKENS, useMousePos, useInView, Reveal, LandingNav,
  Grain, WordmarkAlive, LandingStyles,
});
