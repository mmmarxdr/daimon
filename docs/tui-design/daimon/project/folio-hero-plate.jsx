// HeroPlate — editorial collage replacing the abstract glyph in the cover.
// Layers (back→front):
//  1. Paper background w/ stippled dot field top-left (printer noise)
//  2. Big rubric-red sun/disc behind the figure
//  3. Light cloud wisp passing in front of disc
//  4. <image-slot> for the user's real photo (Greek statue / hero image)
//     — when empty shows a stylized statue silhouette as placeholder
//  5. Reaching hand silhouette from the right (the "human" hand)
//  6. Olive branch growing between figure and hand (animated draw)
//  7. Sacred geometry circle + plumb line (construction marks)
//  8. Torn paper strip at bottom-right edge
//  9. Plate corners + caption

function HeroPlate({ theme, mouse, seen }) {
  const dx = (mouse.x - 0.5);
  const dy = (mouse.y - 0.5);
  const isMobile = window.useIsMobile ? window.useIsMobile() : false;

  // Responsive sizing
  const plateH = isMobile ? 360 : 560;
  const discSize = isMobile ? 200 : 320;
  const handW = isMobile ? 130 : 220;
  const handH = isMobile ? 170 : 280;
  const captionFs = isMobile ? 11 : 13;

  return (
    <div style={{ position: 'relative', width: '100%', height: plateH, overflow: 'visible' }}>
      {/* Top metadata */}
      <div style={{
        position: 'absolute', top: 0, left: 0, right: 0,
        display: 'flex', justifyContent: 'space-between', gap: 8,
        fontFamily: '"JetBrains Mono", monospace',
        fontSize: isMobile ? 8.5 : 10, letterSpacing: 0.5, textTransform: 'uppercase',
        color: theme.inkMuted, zIndex: 6, paddingBottom: 8,
      }}>
        <span><span style={{ color: theme.rubric, fontWeight: 600 }}>FIG. 01</span> · DM/26 · DAIMON / THE INTERMEDIARY</span>
        <span>PLATE Nº 01</span>
      </div>

      {/* The plate frame */}
      <div style={{
        position: 'absolute', inset: '24px 0 60px 0',
        background: theme.paperDeep,
        border: `1px solid ${theme.line}`,
        overflow: 'hidden',
      }}>
        {/* Stippled dot field — printer noise top-left */}
        <DotField theme={theme} seen={seen} />

        {/* Big rubric disc */}
        <div style={{
          position: 'absolute',
          top: '12%', left: isMobile ? '30%' : '36%',
          width: discSize, height: discSize,
          borderRadius: '50%',
          background: theme.rubric,
          opacity: seen ? 0.88 : 0,
          transform: `translate(${dx * 3}px, ${dy * 3}px) scale(${seen ? 1 : 0.7})`,
          transformOrigin: 'center',
          transition: 'opacity 1.6s cubic-bezier(0.2,0.8,0.2,1), transform 1.8s cubic-bezier(0.2,0.8,0.2,1)',
          mixBlendMode: 'multiply',
        }}/>

        {/* Cloud wisp */}
        <CloudWisp theme={theme} seen={seen} dx={dx} dy={dy} />

        {/* Sacred-geometry construction circle */}
        <ConstructionCircle theme={theme} seen={seen} />

        {/* Image slot for user's real photo — sits in figure position */}
        <div style={{
          position: 'absolute',
          top: '8%', left: '18%',
          width: '54%', height: '88%',
          opacity: seen ? 1 : 0,
          transition: 'opacity 1.4s 0.4s',
          transform: `translate(${dx * 6}px, ${dy * 6}px)`,
          zIndex: 3,
        }}>
          <image-slot
            id="daimon-hero-statue"
            placeholder="Drop the Greek statue collage here"
            shape="rect"
            style={{
              width: '100%', height: '100%',
              border: 'none', background: 'transparent',
            }}
          ></image-slot>

          {/* Placeholder statue silhouette — only visible when image-slot empty.
              The image-slot when filled has higher visual weight and covers this. */}
          <div style={{
            position: 'absolute', inset: 0,
            pointerEvents: 'none',
            zIndex: -1,
          }}>
            <StatueSilhouette theme={theme} seen={seen} />
          </div>
        </div>

        {/* Reaching hand from the right */}
        <div style={{
          position: 'absolute',
          top: '38%', right: isMobile ? '2%' : '6%',
          width: handW, height: handH,
          opacity: seen ? 0.92 : 0,
          transform: `translate(${-dx * 8}px, ${-dy * 8}px) translateX(${seen ? 0 : 24}px)`,
          transition: 'opacity 1.2s 0.7s, transform 1.4s 0.7s cubic-bezier(0.2,0.8,0.2,1)',
          zIndex: 4,
        }}>
          <ReachingHand theme={theme} />
        </div>

        {/* Olive branch */}
        <OliveBranch theme={theme} seen={seen} dx={dx} dy={dy} />

        {/* Torn paper bottom-right */}
        <TornPaper theme={theme} seen={seen} />

        {/* Stage markers */}
        <StageMarkers theme={theme} seen={seen} />

        {/* Corner registration marks */}
        {['tl','tr','bl','br'].map((p) => (
          <span key={p} aria-hidden style={{
            position: 'absolute',
            ...(p[0] === 't' ? { top: 10 } : { bottom: 10 }),
            ...(p[1] === 'l' ? { left: 10 } : { right: 10 }),
            color: theme.inkMuted, opacity: 0.55,
            fontFamily: '"JetBrains Mono", monospace', fontSize: 14,
            lineHeight: 1, zIndex: 5,
          }}>+</span>
        ))}
      </div>

      {/* Bottom caption */}
      <div style={{
        position: 'absolute', bottom: 0, left: 0, right: 0,
        paddingTop: 12,
        display: 'flex', justifyContent: 'space-between', alignItems: 'baseline',
        gap: 8, flexWrap: 'wrap',
        fontFamily: '"JetBrains Mono", monospace',
        fontSize: isMobile ? 8.5 : 10, letterSpacing: 0.5, textTransform: 'uppercase',
        color: theme.inkMuted, zIndex: 6,
      }}>
        <span>{isMobile ? 'SHA · d41m0n' : 'SHA · d41m0n · COMPOSED IN STUDIO Δ'}</span>
        <span style={{
          color: theme.ink, fontStyle: 'italic', fontFamily: '"Fraunces", serif',
          textTransform: 'none', letterSpacing: 0, fontSize: captionFs,
        }}>
          δαίμων — the intermediary
        </span>
      </div>
    </div>
  );
}

// ─── Dot field ────────────────────────────────────────────────
function DotField({ theme, seen }) {
  const dots = [];
  for (let r = 0; r < 8; r++) {
    for (let c = 0; c < 22; c++) {
      const fade = Math.max(0, 1 - r / 7 - c / 24);
      if (Math.random() > fade * 1.5) continue;
      dots.push({ x: c * 14 + 18, y: r * 14 + 18, op: fade });
    }
  }
  return (
    <svg aria-hidden style={{
      position: 'absolute', top: 0, left: 0, width: 360, height: 160,
      opacity: seen ? 0.7 : 0,
      transition: 'opacity 1.4s',
      pointerEvents: 'none',
    }}>
      {dots.map((d, i) => (
        <circle key={i} cx={d.x} cy={d.y} r={1.4} fill={theme.inkMuted} opacity={d.op}/>
      ))}
    </svg>
  );
}

// ─── Cloud wisp ────────────────────────────────────────────────
function CloudWisp({ theme, seen, dx, dy }) {
  return (
    <svg aria-hidden style={{
      position: 'absolute', top: '20%', left: '40%',
      width: 240, height: 90,
      opacity: seen ? 0.5 : 0,
      transform: `translate(${dx * 5}px, ${dy * 5}px)`,
      transition: 'opacity 1.6s 0.4s, transform 0.6s ease',
      mixBlendMode: 'screen', pointerEvents: 'none', zIndex: 2,
    }} viewBox="0 0 240 90">
      <ellipse cx="60" cy="50" rx="55" ry="22" fill={theme.paper} opacity="0.75"/>
      <ellipse cx="120" cy="40" rx="62" ry="28" fill={theme.paper} opacity="0.85"/>
      <ellipse cx="180" cy="55" rx="48" ry="20" fill={theme.paper} opacity="0.7"/>
    </svg>
  );
}

// ─── Construction circle ──────────────────────────────────────
function ConstructionCircle({ theme, seen }) {
  return (
    <svg aria-hidden style={{
      position: 'absolute', top: '8%', left: '28%',
      width: 380, height: 380,
      pointerEvents: 'none', zIndex: 2,
    }} viewBox="0 0 380 380">
      <circle cx="190" cy="190" r="170"
              fill="none" stroke={theme.ink} strokeWidth="0.5" strokeDasharray="2 4"
              opacity={seen ? 0.35 : 0}
              style={{
                transition: 'opacity 1.8s 0.6s',
                transformOrigin: '190px 190px',
                animation: seen ? 'rotateSlow 80s linear infinite' : 'none',
              }}/>
      {/* Plumb line */}
      <line x1="190" y1="0" x2="190" y2="380"
            stroke={theme.ink} strokeWidth="0.5" strokeDasharray="3 5"
            opacity={seen ? 0.18 : 0}
            style={{ transition: 'opacity 1.8s 0.8s' }}/>
      {/* Tick marks at cardinal points */}
      {[0, 90, 180, 270].map(deg => {
        const rad = (deg - 90) * Math.PI / 180;
        const x = 190 + Math.cos(rad) * 170;
        const y = 190 + Math.sin(rad) * 170;
        return (
          <circle key={deg} cx={x} cy={y} r={2}
                  fill={theme.editorial}
                  opacity={seen ? 0.6 : 0}
                  style={{ transition: 'opacity 1s 1.2s' }}/>
        );
      })}
    </svg>
  );
}

// ─── Statue silhouette (placeholder for user image) ────────────
function StatueSilhouette({ theme, seen }) {
  // Stylized Greek bust — head, neck, partial torso/shoulders
  // Uses a single semi-organic path. Marble-tone with vertical fragmentation lines.
  return (
    <svg aria-hidden style={{
      width: '100%', height: '100%',
      opacity: seen ? 1 : 0,
      transition: 'opacity 1.4s 0.5s',
    }} viewBox="0 0 320 480" preserveAspectRatio="xMidYMid meet">
      <defs>
        <linearGradient id="marbleGrad" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%"   stopColor="#f5f0e3" />
          <stop offset="50%"  stopColor="#e8dfca" />
          <stop offset="100%" stopColor="#bfb39a" />
        </linearGradient>
        <filter id="marbleNoise">
          <feTurbulence type="fractalNoise" baseFrequency="0.9" numOctaves="2"/>
          <feColorMatrix values="0 0 0 0 0.55  0 0 0 0 0.5  0 0 0 0 0.4  0 0 0 0.18 0"/>
          <feComposite in2="SourceGraphic" operator="in"/>
        </filter>
      </defs>

      {/* Bust silhouette */}
      <g>
        {/* Head */}
        <path d="
          M 160 60
          C 210 60, 240 100, 238 150
          C 238 175, 234 195, 226 215
          C 222 232, 222 248, 232 258
          L 245 270
          L 250 285
          L 248 305
          L 245 320
          L 270 335
          L 295 360
          L 300 410
          L 290 470
          L 30  470
          L 22  410
          L 28  370
          L 50  340
          L 75  320
          L 75  300
          L 72  280
          L 80  265
          L 92  254
          C 102 244, 100 230, 95 215
          C 86 195, 82 175, 82 150
          C 80 100, 110 60, 160 60 Z
          "
          fill="url(#marbleGrad)" />
        <path d="
          M 160 60
          C 210 60, 240 100, 238 150
          C 238 175, 234 195, 226 215
          C 222 232, 222 248, 232 258
          L 245 270
          L 250 285
          L 248 305
          L 245 320
          L 270 335
          L 295 360
          L 300 410
          L 290 470
          L 30  470
          L 22  410
          L 28  370
          L 50  340
          L 75  320
          L 75  300
          L 72  280
          L 80  265
          L 92  254
          C 102 244, 100 230, 95 215
          C 86 195, 82 175, 82 150
          C 80 100, 110 60, 160 60 Z
          "
          fill="black" filter="url(#marbleNoise)" opacity="0.5" />

        {/* Eye sockets and nose suggestion */}
        <ellipse cx="135" cy="155" rx="6" ry="4" fill="#5a5040" opacity="0.55"/>
        <ellipse cx="185" cy="155" rx="6" ry="4" fill="#5a5040" opacity="0.55"/>
        <path d="M 158 158 L 156 192 L 168 192 Z" fill="#7a6a52" opacity="0.45"/>
        <path d="M 145 215 Q 160 222 175 215" stroke="#6a5d48" strokeWidth="1.5" fill="none" opacity="0.45"/>

        {/* Hair waves */}
        <path d="M 92 110 Q 105 95 120 100 Q 130 90 145 95 Q 160 80 175 92 Q 190 82 205 95 Q 220 85 232 105"
              fill="none" stroke="#7a6e54" strokeWidth="1.4" opacity="0.55"/>
        <path d="M 88 130 Q 105 120 125 125 Q 145 115 165 122 Q 185 110 205 122 Q 225 115 235 130"
              fill="none" stroke="#7a6e54" strokeWidth="1.2" opacity="0.45"/>

        {/* Crack — fragmentation line across face */}
        <path d="M 200 80 L 175 130 L 195 175 L 158 220 L 180 270 L 140 320"
              fill="none" stroke="#3a3225" strokeWidth="1.2" opacity="0.35"
              strokeDasharray="6 3 2 4"/>

        {/* Drapery folds on shoulders */}
        <path d="M 60 380 Q 95 365 120 385 M 200 385 Q 230 370 260 388"
              fill="none" stroke="#7a6e54" strokeWidth="1" opacity="0.4"/>
        <path d="M 75 410 Q 110 395 145 415 M 175 415 Q 210 400 245 418"
              fill="none" stroke="#7a6e54" strokeWidth="0.8" opacity="0.35"/>
      </g>

      {/* Hand reaching up from below — stylized, ink, faint */}
      <g opacity="0.0">
        {/* placeholder — actual hand is rendered separately */}
      </g>
    </svg>
  );
}

// ─── Reaching hand ─────────────────────────────────────────────
function ReachingHand({ theme }) {
  // Simplified hand silhouette — palm + 5 fingers, slightly cropped
  return (
    <svg aria-hidden style={{ width: '100%', height: '100%' }} viewBox="0 0 220 280">
      <defs>
        <filter id="handNoise">
          <feTurbulence type="fractalNoise" baseFrequency="1.2" numOctaves="1"/>
          <feColorMatrix values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 0.15 0"/>
          <feComposite in2="SourceGraphic" operator="in"/>
        </filter>
      </defs>
      <g>
        <path d="
          M 200 280
          L 200 200
          C 200 180, 195 165, 188 155
          L 188 80
          C 188 65, 178 55, 168 60
          C 162 64, 160 72, 160 82
          L 160 145
          L 152 145
          L 152 60
          C 152 45, 142 38, 132 42
          C 124 46, 122 56, 122 66
          L 122 148
          L 114 148
          L 114 75
          C 114 60, 104 52, 94 56
          C 86 60, 84 70, 84 80
          L 84 160
          L 76 160
          L 76 110
          C 76 100, 66 92, 56 96
          C 48 100, 46 108, 46 118
          L 46 175
          C 46 195, 52 215, 62 230
          C 70 245, 84 258, 100 268
          C 120 278, 145 282, 175 282
          Z
        " fill={theme.ink} />
        <path d="
          M 200 280
          L 200 200
          C 200 180, 195 165, 188 155
          L 188 80
          C 188 65, 178 55, 168 60
          C 162 64, 160 72, 160 82
          L 160 145
          L 152 145
          L 152 60
          C 152 45, 142 38, 132 42
          C 124 46, 122 56, 122 66
          L 122 148
          L 114 148
          L 114 75
          C 114 60, 104 52, 94 56
          C 86 60, 84 70, 84 80
          L 84 160
          L 76 160
          L 76 110
          C 76 100, 66 92, 56 96
          C 48 100, 46 108, 46 118
          L 46 175
          C 46 195, 52 215, 62 230
          C 70 245, 84 258, 100 268
          C 120 278, 145 282, 175 282
          Z
        " fill={theme.paper} filter="url(#handNoise)" opacity="0.4" />

        {/* Fingertip → reaching toward statue */}
        <circle cx="84" cy="80" r="6" fill={theme.editorial} opacity="0.9"/>
        <circle cx="84" cy="80" r="14" fill={theme.editorial} opacity="0.18">
          <animate attributeName="r" from="6" to="22" dur="2.4s" repeatCount="indefinite"/>
          <animate attributeName="opacity" from="0.4" to="0" dur="2.4s" repeatCount="indefinite"/>
        </circle>
      </g>
    </svg>
  );
}

// ─── Olive branch — animated draw ──────────────────────────────
function OliveBranch({ theme, seen, dx, dy }) {
  // Curves from the statue's shoulder area toward the reaching hand
  const ref = React.useRef(null);
  const [len, setLen] = React.useState(800);
  React.useEffect(() => {
    if (ref.current) setLen(ref.current.getTotalLength());
  }, []);

  const path = "M 220 280 Q 290 240, 350 260 Q 400 275, 450 250";

  return (
    <svg aria-hidden style={{
      position: 'absolute', inset: 0,
      width: '100%', height: '100%', pointerEvents: 'none',
      transform: `translate(${-dx * 4}px, ${-dy * 4}px)`,
      zIndex: 4,
    }} viewBox="0 0 600 540" preserveAspectRatio="xMidYMid meet">
      {/* Stem */}
      <path
        ref={ref}
        d={path}
        fill="none"
        stroke={theme.editorial}
        strokeWidth={1.6}
        strokeLinecap="round"
        strokeDasharray={`${len} ${len}`}
        strokeDashoffset={seen ? 0 : len}
        style={{ transition: 'stroke-dashoffset 2.4s 1s cubic-bezier(0.4,0,0.2,1)' }}
      />

      {/* Leaves — ellipses positioned along the stem, fade in after path draws */}
      {[
        { cx: 260, cy: 263, rx: 14, ry: 5, rot: -20, delay: 1.6 },
        { cx: 285, cy: 250, rx: 12, ry: 4.5, rot: 30, delay: 1.8 },
        { cx: 320, cy: 260, rx: 16, ry: 5.5, rot: -15, delay: 2.0 },
        { cx: 360, cy: 268, rx: 13, ry: 4.5, rot: 20, delay: 2.2 },
        { cx: 400, cy: 262, rx: 15, ry: 5, rot: -25, delay: 2.4 },
        { cx: 430, cy: 254, rx: 12, ry: 4.5, rot: 25, delay: 2.6 },
      ].map((leaf, i) => (
        <ellipse key={i}
          cx={leaf.cx} cy={leaf.cy} rx={leaf.rx} ry={leaf.ry}
          fill={theme.editorial}
          transform={`rotate(${leaf.rot} ${leaf.cx} ${leaf.cy})`}
          opacity={seen ? 0.85 : 0}
          style={{
            transition: `opacity 0.6s ${leaf.delay}s, transform 0.8s ${leaf.delay}s`,
            transformOrigin: `${leaf.cx}px ${leaf.cy}px`,
          }}/>
      ))}

      {/* Olive fruits — small darker dots */}
      {[
        { cx: 305, cy: 256, delay: 2.8 },
        { cx: 380, cy: 264, delay: 3.0 },
        { cx: 420, cy: 252, delay: 3.2 },
      ].map((o, i) => (
        <circle key={i} cx={o.cx} cy={o.cy} r={2.4}
                fill={theme.ink}
                opacity={seen ? 0.85 : 0}
                style={{ transition: `opacity 0.5s ${o.delay}s` }}/>
      ))}
    </svg>
  );
}

// ─── Torn paper bottom-right ───────────────────────────────────
function TornPaper({ theme, seen }) {
  // SVG fragment with jagged top edge — looks like a piece of paper torn off
  return (
    <svg aria-hidden style={{
      position: 'absolute', bottom: 0, right: 0,
      width: 280, height: 80,
      opacity: seen ? 0.92 : 0,
      transition: 'opacity 1.2s 1s, transform 1.2s 1s',
      transform: seen ? 'translateY(0)' : 'translateY(20px)',
      zIndex: 5, pointerEvents: 'none',
    }} viewBox="0 0 280 80">
      <path d="M 0 80 L 0 28 L 16 22 L 32 30 L 48 18 L 68 26 L 86 16 L 110 24 L 132 14 L 156 22 L 180 18 L 208 26 L 232 16 L 258 22 L 280 18 L 280 80 Z"
            fill={theme.paperElev} stroke={theme.line} strokeWidth="0.5"/>
      <text x="20" y="58" fill={theme.inkMuted}
            fontFamily='"JetBrains Mono", monospace'
            fontSize="9" letterSpacing="0.6"
            style={{ textTransform: 'uppercase' }}>
        FRAGMENT · MMXXVI · NO. 01
      </text>
    </svg>
  );
}

// ─── Stage markers ─────────────────────────────────────────────
function StageMarkers({ theme, seen }) {
  const items = [
    { y: '14%', label: 'a · the daimon' },
    { y: '46%', label: 'b · the contact' },
    { y: '78%', label: 'c · the hand'    },
  ];
  return (
    <div data-stage-markers>
      {items.map((it, i) => (
        <div key={i} style={{
          position: 'absolute', right: 26, top: it.y,
          fontFamily: '"JetBrains Mono", monospace',
          fontSize: 9.5, color: theme.inkMuted, letterSpacing: 0.5,
          textTransform: 'uppercase',
          opacity: seen ? 0.85 : 0,
          transform: seen ? 'translateX(0)' : 'translateX(8px)',
          transition: `opacity 0.7s ${1.4 + i * 0.15}s, transform 0.7s ${1.4 + i * 0.15}s`,
          display: 'flex', alignItems: 'center', gap: 6,
          zIndex: 6,
        }}>
          <span style={{ width: 14, height: 1, background: theme.inkMuted }}/>
          {it.label}
        </div>
      ))}
    </div>
  );
}

Object.assign(window, { HeroPlate });
