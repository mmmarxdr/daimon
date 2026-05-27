// Folio backgrounds — canvas-based ambient animations.
// PaperFall: leaves of paper drift down slowly (cover folio).
// InkTrace: an ink line that the cursor "draws" through method stages.
// PlateWipe: duotone → full-color reveal (used for plates).

// ─────────────────────────────────────────────────────────────
// PaperFall — sheets of paper drift slowly across the cover.
// ─────────────────────────────────────────────────────────────
function PaperFall({ theme, intensity = 'calm' }) {
  const canvasRef = React.useRef(null);
  const sheetsRef = React.useRef([]);
  const rafRef = React.useRef(null);

  React.useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    const dpr = window.devicePixelRatio || 1;

    const resize = () => {
      const r = canvas.getBoundingClientRect();
      canvas.width = r.width * dpr;
      canvas.height = r.height * dpr;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    };
    resize();
    const ro = new ResizeObserver(resize);
    ro.observe(canvas);

    // Generate sheets
    const count = intensity === 'lively' ? 14 : 8;
    sheetsRef.current = Array.from({ length: count }, () => spawn(canvas, true));

    function spawn(c, initial) {
      const r = c.getBoundingClientRect();
      const w = 60 + Math.random() * 100;
      const h = w * (1.2 + Math.random() * 0.3);
      return {
        x: Math.random() * r.width,
        y: initial ? Math.random() * r.height : -h - Math.random() * 200,
        w, h,
        rot: Math.random() * Math.PI * 2,
        rotSpeed: (Math.random() - 0.5) * 0.004,
        vy: 0.15 + Math.random() * 0.25,
        vx: (Math.random() - 0.5) * 0.15,
        sway: Math.random() * Math.PI * 2,
        swaySpeed: 0.005 + Math.random() * 0.005,
        opacity: 0.05 + Math.random() * 0.08,
      };
    }

    const tick = () => {
      const r = canvas.getBoundingClientRect();
      ctx.clearRect(0, 0, r.width, r.height);

      sheetsRef.current.forEach((s, i) => {
        s.sway += s.swaySpeed;
        s.x += s.vx + Math.sin(s.sway) * 0.3;
        s.y += s.vy;
        s.rot += s.rotSpeed;

        if (s.y > r.height + s.h) {
          sheetsRef.current[i] = spawn(canvas, false);
          return;
        }

        ctx.save();
        ctx.translate(s.x, s.y);
        ctx.rotate(s.rot);
        // Paper sheet — soft warm fill with darker edge
        ctx.globalAlpha = s.opacity;
        ctx.fillStyle = theme.ink;
        ctx.fillRect(-s.w / 2, -s.h / 2, s.w, s.h);
        // Single ink rule on the page
        ctx.globalAlpha = s.opacity * 0.7;
        ctx.strokeStyle = theme.ink;
        ctx.lineWidth = 0.5;
        ctx.beginPath();
        ctx.moveTo(-s.w / 2 + 8, -s.h / 4);
        ctx.lineTo(s.w / 2 - 8, -s.h / 4);
        ctx.stroke();
        ctx.beginPath();
        ctx.moveTo(-s.w / 2 + 8, 0);
        ctx.lineTo(s.w / 2 - 12, 0);
        ctx.stroke();
        ctx.beginPath();
        ctx.moveTo(-s.w / 2 + 8, s.h / 4);
        ctx.lineTo(s.w / 2 - 20, s.h / 4);
        ctx.stroke();
        ctx.restore();
      });

      rafRef.current = requestAnimationFrame(tick);
    };
    tick();
    return () => {
      cancelAnimationFrame(rafRef.current);
      ro.disconnect();
    };
  }, [theme.ink, intensity]);

  return (
    <canvas ref={canvasRef} aria-hidden style={{
      position: 'absolute', inset: 0, width: '100%', height: '100%',
      pointerEvents: 'none', opacity: 0.55,
    }}/>
  );
}

// ─────────────────────────────────────────────────────────────
// InkTrace — when scrolled into view, an ink path is "drawn"
// along an SVG path. Used for the Method folio.
// ─────────────────────────────────────────────────────────────
function InkTrace({ theme, d, sectionRef, scrollerRef, strokeWidth = 1.5, dashArray, color }) {
  const pathRef = React.useRef(null);
  const [len, setLen] = React.useState(0);
  React.useEffect(() => {
    if (pathRef.current) setLen(pathRef.current.getTotalLength());
  }, [d]);
  const p = useScrollProgress(sectionRef, scrollerRef);
  // 0..1 over the section's progress through viewport
  const drawn = Math.max(0, Math.min(1, (p - 0.15) / 0.6));
  const dashOffset = len * (1 - drawn);
  return (
    <svg aria-hidden style={{
      position: 'absolute', inset: 0, width: '100%', height: '100%',
      pointerEvents: 'none', overflow: 'visible',
    }} preserveAspectRatio="none">
      <path
        ref={pathRef}
        d={d}
        fill="none"
        stroke={color || theme.editorial}
        strokeWidth={strokeWidth}
        strokeLinecap="round"
        strokeDasharray={dashArray || `${len} ${len}`}
        strokeDashoffset={dashOffset}
        style={{ transition: 'stroke-dashoffset 0.2s linear' }}
      />
    </svg>
  );
}

// ─────────────────────────────────────────────────────────────
// PlatePlaceholder — a "printed plate" frame with caption.
// Inside: a duotone → color wipe on enter viewport (fake "press").
// We render a content prop (children) inside a clip-path that's
// revealed top-to-bottom as the user scrolls past.
// ─────────────────────────────────────────────────────────────
function Plate({ theme, no, year, label, children, style = {}, height = 360, captionLeft = true }) {
  const ref = React.useRef(null);
  const seen = useInView(ref, 0.18);
  return (
    <figure ref={ref} style={{
      margin: 0, position: 'relative',
      ...style,
    }}>
      <div style={{
        position: 'relative',
        height,
        background: theme.paperDeep,
        border: `1px solid ${theme.line}`,
        overflow: 'hidden',
      }}>
        {/* Duotone layer — visible until "wipe" finishes */}
        <div style={{
          position: 'absolute', inset: 0,
          background: theme.paperDeep,
          opacity: seen ? 0 : 1,
          transition: 'opacity 1.4s cubic-bezier(0.6,0,0.3,1) 0.1s',
        }}/>
        {/* Color content reveals via clip-path: top sheet sweeps down */}
        <div style={{
          position: 'absolute', inset: 0,
          clipPath: seen ? 'inset(0% 0% 0% 0%)' : 'inset(100% 0% 0% 0%)',
          transition: 'clip-path 1.6s cubic-bezier(0.65,0.02,0.32,0.95) 0.05s',
        }}>
          {children}
        </div>
        {/* Plate corner registration marks — printer's marks */}
        {['tl','tr','bl','br'].map((p) => (
          <span key={p} aria-hidden style={{
            position: 'absolute', width: 14, height: 14,
            ...(p[0] === 't' ? { top: 8 } : { bottom: 8 }),
            ...(p[1] === 'l' ? { left: 8 } : { right: 8 }),
            opacity: 0.35,
            color: theme.inkMuted,
            fontSize: 10,
            fontFamily: '"JetBrains Mono", monospace',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
          }}>+</span>
        ))}
      </div>
      <figcaption style={{
        marginTop: 12,
        display: 'flex', alignItems: 'baseline', gap: 12,
        flexDirection: captionLeft ? 'row' : 'row-reverse',
        fontFamily: '"JetBrains Mono", monospace',
        fontSize: 10.5, letterSpacing: 0.6, textTransform: 'uppercase',
        color: theme.inkMuted,
      }}>
        <span style={{ color: theme.rubric, fontWeight: 600 }}>FIG. {String(no).padStart(2,'0')}</span>
        <span style={{ color: theme.ink, fontWeight: 500 }}>{label}</span>
        <span style={{ flex: 1, height: 1, background: theme.line, transform: 'translateY(-3px)' }}/>
        <span>{year}</span>
      </figcaption>
    </figure>
  );
}

// ─────────────────────────────────────────────────────────────
// Marquee — infinite horizontal scroll. Used for the field ticker.
// items: array of {label, value}
// onActiveChange: fires when an item passes the focal point — used
// to make the active label go italic + accent.
// ─────────────────────────────────────────────────────────────
function Marquee({ items, theme, speed = 60, gap = 40, focal = 0.5, accent }) {
  const containerRef = React.useRef(null);
  const trackRef = React.useRef(null);
  const [activeIdx, setActiveIdx] = React.useState(-1);

  React.useEffect(() => {
    const c = containerRef.current, t = trackRef.current;
    if (!c || !t) return;
    let x = 0;
    let raf;
    let last = performance.now();
    const tick = (now) => {
      const dt = (now - last) / 1000;
      last = now;
      x -= speed * dt;
      // Reset when first half scrolled out
      const halfWidth = t.scrollWidth / 2;
      if (-x >= halfWidth) x += halfWidth;
      t.style.transform = `translateX(${x}px)`;

      // Determine active item — find which child overlaps the focal point
      const cr = c.getBoundingClientRect();
      const focalX = cr.left + cr.width * focal;
      const children = t.children;
      let bestIdx = -1, bestDist = Infinity;
      for (let i = 0; i < children.length / 2; i++) {
        const r = children[i].getBoundingClientRect();
        const cx = r.left + r.width / 2;
        const d = Math.abs(cx - focalX);
        if (d < bestDist) { bestDist = d; bestIdx = i; }
      }
      if (bestIdx !== activeIdx) setActiveIdx(bestIdx);

      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [items.length, speed, focal, activeIdx]);

  const renderItems = (offset) => items.map((it, i) => (
    <span key={`${offset}-${i}`} style={{
      display: 'inline-flex', alignItems: 'baseline', gap: 6,
      paddingRight: gap,
      color: i === activeIdx ? (accent || theme.accent) : theme.inkSoft,
      fontStyle: i === activeIdx ? 'italic' : 'normal',
      fontFamily: i === activeIdx ? '"Fraunces", serif' : '"JetBrains Mono", monospace',
      fontSize: i === activeIdx ? 18 : 13,
      fontWeight: i === activeIdx ? 400 : 400,
      transition: 'color 0.4s ease, font-size 0.3s ease',
      whiteSpace: 'nowrap',
    }}>
      <span style={{
        color: theme.inkFaint, fontFamily: '"JetBrains Mono", monospace',
        fontSize: 11,
      }}>·{it.coord}</span>
      <span>{it.city}</span>
    </span>
  ));

  return (
    <div ref={containerRef} style={{
      width: '100%', overflow: 'hidden',
      maskImage: 'linear-gradient(90deg, transparent, black 8%, black 92%, transparent)',
      WebkitMaskImage: 'linear-gradient(90deg, transparent, black 8%, black 92%, transparent)',
    }}>
      <div ref={trackRef} style={{
        display: 'inline-flex', alignItems: 'center',
        whiteSpace: 'nowrap', willChange: 'transform',
      }}>
        {renderItems(0)}
        {renderItems(1)}
      </div>
    </div>
  );
}

Object.assign(window, { PaperFall, InkTrace, Plate, Marquee });
