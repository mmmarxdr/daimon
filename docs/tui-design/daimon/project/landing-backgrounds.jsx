// Landing A — per-section background animations.
// Each section gets its own signature motion, cohesive but distinct.
// All low-opacity, pointer-events: none. The glyph ⫶ is the visual DNA:
// each animation amplifies a different property of it.

// ─────────────────────────────────────────────────────────────
// BgMagneticLines — Pillars (§i)
// Horizontal lines that bend toward the mouse. Magnetism.
// ─────────────────────────────────────────────────────────────
function BgMagneticLines({ theme, isDark }) {
  const canvasRef = React.useRef(null);
  const mouseRef = React.useRef({ x: -10000, y: -10000 });

  React.useEffect(() => {
    const cv = canvasRef.current; if (!cv) return;
    const ctx = cv.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    let w = 0, h = 0;
    let lines = [];

    const resize = () => {
      const r = cv.getBoundingClientRect();
      w = r.width; h = r.height;
      cv.width = w * dpr; cv.height = h * dpr;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      const spacing = 42;
      lines = [];
      for (let y = spacing; y < h; y += spacing) {
        lines.push({ y, phase: Math.random() * Math.PI * 2 });
      }
    };
    resize();
    window.addEventListener('resize', resize);

    const parent = cv.parentElement;
    const onMove = (e) => {
      const r = cv.getBoundingClientRect();
      mouseRef.current = { x: e.clientX - r.left, y: e.clientY - r.top };
    };
    const onLeave = () => { mouseRef.current = { x: -10000, y: -10000 }; };
    parent.addEventListener('mousemove', onMove);
    parent.addEventListener('mouseleave', onLeave);

    const accentRGB = isDark ? '93,191,167' : '45,133,115';
    const inkRGB = isDark ? '234,229,216' : '26,24,19';

    let raf, t = 0;
    const loop = () => {
      t += 0.006;
      ctx.clearRect(0, 0, w, h);
      const mx = mouseRef.current.x, my = mouseRef.current.y;
      const RAD = 260;

      for (const ln of lines) {
        ctx.beginPath();
        const STEP = 8;
        for (let x = 0; x <= w; x += STEP) {
          // Base drift
          const drift = Math.sin(x * 0.006 + ln.phase + t) * 1.2;
          // Magnetic pull toward mouse
          const dx = x - mx, dy = ln.y - my;
          const d = Math.sqrt(dx * dx + dy * dy);
          let pull = 0;
          if (d < RAD) {
            const str = (1 - d / RAD);
            pull = -dy * str * str * 0.5;
          }
          const y = ln.y + drift + pull;
          if (x === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        }
        const dToMouse = Math.abs(ln.y - my);
        const near = dToMouse < RAD ? (1 - dToMouse / RAD) : 0;
        const alpha = (isDark ? 0.06 : 0.05) + near * (isDark ? 0.25 : 0.18);
        ctx.strokeStyle = near > 0.3
          ? `rgba(${accentRGB},${alpha})`
          : `rgba(${inkRGB},${alpha * 0.55})`;
        ctx.lineWidth = 1;
        ctx.stroke();
      }
      raf = requestAnimationFrame(loop);
    };
    loop();

    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener('resize', resize);
      parent.removeEventListener('mousemove', onMove);
      parent.removeEventListener('mouseleave', onLeave);
    };
  }, [isDark]);

  return <canvas ref={canvasRef} style={bgStyle} />;
}

// ─────────────────────────────────────────────────────────────
// BgConstellation — Features (§ii)
// Dots that connect with lines near the mouse. Network graph.
// ─────────────────────────────────────────────────────────────
function BgConstellation({ theme, isDark }) {
  const canvasRef = React.useRef(null);
  const mouseRef = React.useRef({ x: -10000, y: -10000 });

  React.useEffect(() => {
    const cv = canvasRef.current; if (!cv) return;
    const ctx = cv.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    let w = 0, h = 0;
    let dots = [];

    const resize = () => {
      const r = cv.getBoundingClientRect();
      w = r.width; h = r.height;
      cv.width = w * dpr; cv.height = h * dpr;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      dots = [];
      const DENSITY = 0.00012;
      const count = Math.max(40, Math.floor(w * h * DENSITY));
      for (let i = 0; i < count; i++) {
        dots.push({
          x: Math.random() * w,
          y: Math.random() * h,
          vx: (Math.random() - 0.5) * 0.15,
          vy: (Math.random() - 0.5) * 0.15,
          r: 0.7 + Math.random() * 1.1,
        });
      }
    };
    resize();
    window.addEventListener('resize', resize);

    const parent = cv.parentElement;
    const onMove = (e) => {
      const r = cv.getBoundingClientRect();
      mouseRef.current = { x: e.clientX - r.left, y: e.clientY - r.top };
    };
    const onLeave = () => { mouseRef.current = { x: -10000, y: -10000 }; };
    parent.addEventListener('mousemove', onMove);
    parent.addEventListener('mouseleave', onLeave);

    const accentRGB = isDark ? '93,191,167' : '45,133,115';
    const inkRGB = isDark ? '234,229,216' : '26,24,19';
    const CONN_RAD = 120, MOUSE_RAD = 200;

    let raf;
    const loop = () => {
      ctx.clearRect(0, 0, w, h);
      const mx = mouseRef.current.x, my = mouseRef.current.y;

      // Move dots
      for (const d of dots) {
        d.x += d.vx; d.y += d.vy;
        if (d.x < 0 || d.x > w) d.vx *= -1;
        if (d.y < 0 || d.y > h) d.vy *= -1;
      }

      // Connections near mouse
      for (let i = 0; i < dots.length; i++) {
        const a = dots[i];
        const adm = Math.hypot(a.x - mx, a.y - my);
        if (adm > MOUSE_RAD) continue;
        const mf = 1 - adm / MOUSE_RAD;
        for (let j = i + 1; j < dots.length; j++) {
          const b = dots[j];
          const dd = Math.hypot(a.x - b.x, a.y - b.y);
          if (dd < CONN_RAD) {
            const alpha = (1 - dd / CONN_RAD) * mf * (isDark ? 0.35 : 0.25);
            ctx.strokeStyle = `rgba(${accentRGB},${alpha})`;
            ctx.lineWidth = 0.6;
            ctx.beginPath();
            ctx.moveTo(a.x, a.y);
            ctx.lineTo(b.x, b.y);
            ctx.stroke();
          }
        }
      }

      // Dots
      for (const d of dots) {
        const adm = Math.hypot(d.x - mx, d.y - my);
        const near = adm < MOUSE_RAD ? (1 - adm / MOUSE_RAD) : 0;
        const alpha = (isDark ? 0.12 : 0.1) + near * (isDark ? 0.5 : 0.4);
        ctx.fillStyle = near > 0.2
          ? `rgba(${accentRGB},${alpha})`
          : `rgba(${inkRGB},${alpha * 0.6})`;
        ctx.beginPath();
        ctx.arc(d.x, d.y, d.r, 0, Math.PI * 2);
        ctx.fill();
      }

      raf = requestAnimationFrame(loop);
    };
    loop();

    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener('resize', resize);
      parent.removeEventListener('mousemove', onMove);
      parent.removeEventListener('mouseleave', onLeave);
    };
  }, [isDark]);

  return <canvas ref={canvasRef} style={bgStyle} />;
}

// ─────────────────────────────────────────────────────────────
// BgAurora — Compare (§iii)
// Slow, wide teal/ink blobs drifting. Like ink in water. Low opacity.
// ─────────────────────────────────────────────────────────────
function BgAurora({ theme, isDark }) {
  const accentRGB = isDark ? '93,191,167' : '45,133,115';
  const inkRGB = isDark ? '234,229,216' : '26,24,19';
  return (
    <div aria-hidden style={{
      ...bgStyle,
      overflow: 'hidden',
    }}>
      <div style={{
        position: 'absolute', width: '60vw', height: '60vw',
        borderRadius: '50%', top: '-20%', left: '-10%',
        background: `radial-gradient(circle, rgba(${accentRGB},${isDark ? 0.22 : 0.14}) 0%, rgba(${accentRGB},0) 60%)`,
        filter: 'blur(40px)',
        animation: 'auroraA 22s ease-in-out infinite',
      }}/>
      <div style={{
        position: 'absolute', width: '55vw', height: '55vw',
        borderRadius: '50%', bottom: '-30%', right: '-15%',
        background: `radial-gradient(circle, rgba(${inkRGB},${isDark ? 0.08 : 0.05}) 0%, rgba(${inkRGB},0) 60%)`,
        filter: 'blur(40px)',
        animation: 'auroraB 28s ease-in-out infinite',
      }}/>
      <div style={{
        position: 'absolute', width: '40vw', height: '40vw',
        borderRadius: '50%', top: '40%', left: '30%',
        background: `radial-gradient(circle, rgba(${accentRGB},${isDark ? 0.12 : 0.07}) 0%, rgba(${accentRGB},0) 60%)`,
        filter: 'blur(40px)',
        animation: 'auroraC 32s ease-in-out infinite',
      }}/>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// BgScanlines — Install (§iv)
// Subtle vertical scanlines + a soft sweep from left to right.
// Nod to CRT / punch-card. Syncs with the blinking caret.
// ─────────────────────────────────────────────────────────────
function BgScanlines({ theme, isDark }) {
  const accentRGB = isDark ? '93,191,167' : '45,133,115';
  return (
    <div aria-hidden style={{
      ...bgStyle,
      overflow: 'hidden',
    }}>
      {/* Vertical scanlines */}
      <div style={{
        position: 'absolute', inset: 0,
        backgroundImage: `repeating-linear-gradient(
          90deg,
          rgba(${isDark ? '234,229,216' : '26,24,19'}, ${isDark ? 0.04 : 0.03}) 0px,
          rgba(${isDark ? '234,229,216' : '26,24,19'}, ${isDark ? 0.04 : 0.03}) 1px,
          transparent 1px, transparent 42px
        )`,
      }}/>
      {/* Sweep */}
      <div style={{
        position: 'absolute', top: 0, bottom: 0, width: 200,
        background: `linear-gradient(90deg, rgba(${accentRGB},0) 0%, rgba(${accentRGB},${isDark ? 0.18 : 0.1}) 50%, rgba(${accentRGB},0) 100%)`,
        animation: 'scanSweep 9s linear infinite',
        filter: 'blur(12px)',
      }}/>
      {/* Horizontal tick — a single line that moves vertically slowly */}
      <div style={{
        position: 'absolute', left: 0, right: 0, height: 1,
        background: `linear-gradient(90deg, transparent, rgba(${accentRGB},${isDark ? 0.4 : 0.25}), transparent)`,
        animation: 'scanTick 7s ease-in-out infinite',
      }}/>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// BgPulseRings — centered, fixed. Rings emit from the middle
// and extend past the section bounds so they "meet" the section above.
// ─────────────────────────────────────────────────────────────
function BgPulseRings({ theme, isDark }) {
  const accentRGB = isDark ? '93,191,167' : '45,133,115';
  return (
    <div aria-hidden style={{
      ...bgStyle,
      overflow: 'visible',
    }}>
      {[0, 1, 2, 3, 4].map(i => (
        <div key={i} style={{
          position: 'absolute',
          left: '50%', top: '50%',
          transform: 'translate(-50%, -50%)',
          width: 600, height: 600, borderRadius: '50%',
          border: `1px solid rgba(${accentRGB},1)`,
          animation: `pulseRingX 6s ease-out infinite ${i * 1.15}s`,
        }}/>
      ))}
      {/* Core glow */}
      <div style={{
        position: 'absolute',
        left: '50%', top: '50%',
        transform: 'translate(-50%, -50%)',
        width: 16, height: 16, borderRadius: 99,
        background: `rgba(${accentRGB},0.7)`,
        boxShadow: `0 0 48px rgba(${accentRGB},0.6), 0 0 96px rgba(${accentRGB},0.3)`,
      }}/>
    </div>
  );
}

// Shared style
const bgStyle = {
  position: 'absolute', inset: 0,
  width: '100%', height: '100%', display: 'block',
  pointerEvents: 'none', zIndex: 0,
};

// Keyframes for backgrounds
function BgKeyframes() {
  return (
    <style>{`
      @keyframes auroraA {
        0%, 100% { transform: translate(0,0) scale(1); }
        50% { transform: translate(8%, -6%) scale(1.15); }
      }
      @keyframes auroraB {
        0%, 100% { transform: translate(0,0) scale(1); }
        50% { transform: translate(-6%, 8%) scale(1.1); }
      }
      @keyframes auroraC {
        0%, 100% { transform: translate(0,0) scale(1); opacity: 0.7; }
        50% { transform: translate(-10%, -4%) scale(0.9); opacity: 1; }
      }
      @keyframes scanSweep {
        0% { transform: translateX(-200px); }
        100% { transform: translateX(calc(100vw + 200px)); }
      }
      @keyframes scanTick {
        0% { top: 0%; opacity: 0; }
        5% { opacity: 1; }
        95% { opacity: 1; }
        100% { top: 100%; opacity: 0; }
      }
      @keyframes pulseRingX {
        0% { transform: translate(-50%, -50%) scale(0.1); opacity: 0.75; }
        60% { opacity: 0.4; }
        100% { transform: translate(-50%, -50%) scale(3.2); opacity: 0; }
      }
    `}</style>
  );
}

Object.assign(window, {
  BgMagneticLines, BgConstellation, BgAurora, BgScanlines, BgPulseRings, BgKeyframes,
});
