/**
 * Orbit — Holiday Themes config
 * Single source of truth for every value in the holiday designs.
 * Framework-agnostic: tokens → CSS custom properties; art → inline SVG strings;
 * layouts → plain functions. Adapt file location/naming to the codebase.
 */

// ───────────────────────────── Types ─────────────────────────────

export type Mode = 'light' | 'dark';
export type HolidayId = 'christmas' | 'halloween' | 'nye' | 'easter' | 'santos' | 'lunar';

export interface Tokens {
  bg: string; side: string; card: string; cardLine: string; rowLine: string; hover: string; field: string;
  text: string; muted: string; faint: string;
  primary: string;   // "Today" heading, notification badge
  secondary: string; // "Up Next" heading
  icon: string;      // nav icons, date eyebrow, flags, due-today dates
  tag: string;       // #tags
  issue: string;     // issue / PR link marker
  late: string;      // overdue dates
  prio1: string; prio2: string; prio3: string; // !, !!, !!!
  check: string; switchOff: string; glow: string;
  chipBg: string; chipText: string; chipLine: string;
  thread: string; wire: string;
  [extra: string]: string; // holiday-specific extras (cap, web, bat, moon…)
}

export interface Art {
  logo: string;        // 28×28 inline SVG
  chipIcon: string;    // 16×16 inline SVG (countdown chip)
  toggleIcon: string;  // 16×16 inline SVG (sidebar effects row)
  tile: { background: string; overlay: string };
  tileIcon: string;    // stroke colour of the home icon on the white box
  iconBoxes: { calendar: string; quickNotes: string; quickDraw: string; myFiles: string; workspace: string; avatar: string };
  done: [inner: string, outer: string, glow: string];
  bars: string[];      // event bar colours, mapped to calendar slots in order
  festiveBar: string;  // striped gradient for festive events
  cardCorner:
    | { kind: 'snowcap'; paths: [string, string] }
    | { kind: 'cobweb' }
    | { kind: 'svg'; svg: string };
  moon?: boolean;
}

export type Decor =
  | { kind: 'lights'; count: number; colors: string[] }
  | { kind: 'drop'; viewBox: [number, number]; items: [x: number, len: number, scale: number][]; svg: (i: number) => string }
  | { kind: 'bunting'; scallops: number; depth: number; perScallop: number; colors: string[] };

export type Particles =
  | {
      kind: 'fall' | 'rise';
      count: number; viewBox: string; shapes: string[]; colors: string[];
      size: [number, number]; dur: [number, number]; spin?: 'fast' | 'slow';
      opacity: [number, number];
    }
  | {
      kind: 'fly'; // bats
      count: number; path: string; size: [number, number]; dur: [number, number]; opacity: [number, number];
    };

export interface Holiday {
  id: HolidayId;
  name: string;
  tokens: Record<Mode, Tokens>;
  art: Art;
  decor: Decor | null;
  extraDecor?: Decor; // Halloween: spiders (in addition to bats particles)
  particles: Particles;
  copy: {
    chip: (n: number, unit?: 'days' | 'hours', year?: number) => string;
    chipToday: string;
    effectsLabel: string;
    placeholder: string;
  };
  /** Inclusive local-date window. peak = countdown target. */
  window: (year: number) => { start: Date; end: Date; peak: Date };
}

// ───────────────────────────── Helpers ─────────────────────────────

/** Seeded pseudo-random in [0,1): stable layouts across renders. */
export const rnd = (i: number, k: number) => {
  const x = Math.sin(i * 12.9898 + k * 78.233) * 43758.5453;
  return x - Math.floor(x);
};

const d = (y: number, m: number, day: number) => new Date(y, m - 1, day);
const addDays = (dt: Date, n: number) => new Date(dt.getFullYear(), dt.getMonth(), dt.getDate() + n);
const plural = (n: number, w: string) => `${n} ${w}${n === 1 ? '' : 's'}`;

/** Gregorian Easter Sunday (Anonymous / Meeus algorithm). */
export function easterSunday(year: number): Date {
  const a = year % 19, b = Math.floor(year / 100), c = year % 100;
  const dd = Math.floor(b / 4), e = b % 4, f = Math.floor((b + 8) / 25);
  const g = Math.floor((b - f + 1) / 3), h = (19 * a + b - dd - g + 15) % 30;
  const i = Math.floor(c / 4), k = c % 4, l = (32 + 2 * e + 2 * i - h - k) % 7;
  const m = Math.floor((a + 11 * h + 22 * l) / 451);
  const month = Math.floor((h + l - 7 * m + 114) / 31), day = ((h + l - 7 * m + 114) % 31) + 1;
  return d(year, month, day);
}

/** Lunar New Year dates (verify / extend before 2030). */
export const LUNAR_NEW_YEAR: Record<number, [number, number]> = {
  2026: [2, 17], 2027: [2, 6], 2028: [1, 26], 2029: [2, 13], 2030: [2, 3],
};

/** Christmas string lights: scalloped wire + one bulb per scallop. */
export function lightsLayout(n = 24) {
  let wire = 'M0,4';
  for (let i = 0; i < n; i++) wire += ` Q${i * 10 + 5},22 ${i * 10 + 10},4`;
  const colors = ['#ff4d5e', '#ffc94a', '#4fd18b', '#fff3d6'];
  return {
    viewBox: `0 0 ${n * 10} 40`, // render 100% × 40px, preserveAspectRatio="none"
    wire,
    bulbs: Array.from({ length: n }, (_, i) => ({
      leftPct: ((i + 0.5) / n) * 100, // centre; bulb wrapper top = 12px, width 8px
      color: colors[i % 4],
      rotate: i % 2 ? 9 : -9,
      twinkleDur: 1.8 + rnd(i, 2) * 1.6,
      twinkleDelay: -rnd(i, 3) * 3,
    })),
  };
}

/** Santos Populares bunting: deep scallops with k flags along each curve. */
export function buntingLayout(N = 6, Q = 40, k = 7, flagW = 14) {
  let wire = 'M0,4';
  for (let i = 0; i < N; i++) wire += ` Q${i * 10 + 5},${Q} ${i * 10 + 10},4`;
  const colors = ['#ff4d4d', '#ffd23f', '#3ccf6e', '#3fa9ff', '#ff7ac6', '#ff9f1c'];
  const flags: { leftPct: number; topPx: number; color: string; swayDur: number; swayDelay: number }[] = [];
  let c = 0;
  for (let i = 0; i < N; i++) for (let j = 0; j < k; j++) {
    const t = (j + 0.5) / k;
    flags.push({
      leftPct: ((i + t) / N) * 100, // subtract flagW/2 px to centre
      topPx: 4 + 2 * t * (1 - t) * (Q - 4) - 1,
      color: colors[c % colors.length],
      swayDur: 1.8 + rnd(c, 7) * 1.4,
      swayDelay: -rnd(c, 8) * 2,
    });
    c++;
  }
  return { viewBox: `0 0 ${N * 10} 40`, wire, flags, flagSvg: (fill: string) => `<svg viewBox="0 0 14 18" width="14" height="18"><path d="M0 0h14L7 18z" fill="${fill}"/></svg>` };
}

/** Halloween cobweb anchored at top-right (viewBox 0 0 64 64, render 76×76). */
export function cobwebPath() {
  const C = [64, 0], tips = [[0, 0], [8, 22], [24, 42], [42, 58], [64, 64]];
  const dirs = tips.map(([x, y]) => { const dx = x - C[0], dy = y - C[1], l = Math.hypot(dx, dy); return [dx / l, dy / l]; });
  let p = '';
  tips.forEach(t => { p += `M64,0L${t[0]},${t[1]}`; });
  [14, 28, 44, 58].forEach(r => {
    const pts = dirs.map(([dx, dy]) => [C[0] + dx * r, C[1] + dy * r]);
    p += `M${pts[0][0].toFixed(1)},${pts[0][1].toFixed(1)}`;
    for (let j = 1; j < pts.length; j++) {
      const a = pts[j - 1], b = pts[j], mx = (a[0] + b[0]) / 2, my = (a[1] + b[1]) / 2;
      const cx = mx + (C[0] - mx) * 0.18, cy = my + (C[1] - my) * 0.18;
      p += `Q${cx.toFixed(1)},${cy.toFixed(1)} ${b[0].toFixed(1)},${b[1].toFixed(1)}`;
    }
  });
  return p; // stroke: tokens.web, width 0.8, fill none
}

/** Particle instances (seeded). Scale count by viewport area, capped at count. */
export function particleLayout(p: Particles, seed = 0, area = 1) {
  const n = Math.min(p.count, Math.round(p.count * area));
  return Array.from({ length: n }, (_, i) => {
    const k = i + seed * 100;
    const size = Math.round(p.size[0] + rnd(k, 2) * (p.size[1] - p.size[0]));
    const dur = p.dur[0] + rnd(k, 5) * (p.dur[1] - p.dur[0]);
    const opacity = p.opacity[0] + rnd(k, 3) * (p.opacity[1] - p.opacity[0]);
    if (p.kind === 'fly') {
      return { kind: 'fly' as const, size, topPx: 70 + rnd(k, 1) * 700, anim: i % 2 ? 'flyA' : 'flyB', dur, delay: -rnd(k, 4) * 26, opacity, flapDur: 0.14 + rnd(k, 6) * 0.1 };
    }
    return {
      kind: p.kind, size, leftPct: rnd(k, 1) * 100, opacity,
      anim: p.kind === 'rise' ? 'rise' : rnd(k, 4) > 0.5 ? 'fallL' : 'fallR', dur, delay: -rnd(k, 6) * p.dur[1],
      shape: p.shapes[i % p.shapes.length], color: p.colors[i % p.colors.length],
      spinDur: p.spin === 'slow' ? 4 + rnd(k, 7) * 4 : p.spin === 'fast' ? 1.2 + rnd(k, 7) * 2 : null,
      swayDur: p.spin ? null : 1.6 + rnd(k, 7),
    };
  });
}

/** Builds full tokens from an accent set (used by NYE, Easter, Santos, Lunar). */
function fromAccents(mode: Mode, t: {
  bg: string; side: string; card: string; field: string; text: string; muted: string; faint: string;
  a1: string; a2: string; a3: string; late: string; check: string; switchOff: string; glow: string;
  thread: string; wire: string; chipBg: string; chipText: string; chipLine: string;
}): Tokens {
  const line = mode === 'dark'
    ? { cardLine: 'rgba(255,255,255,0.06)', rowLine: 'rgba(255,255,255,0.05)', hover: 'rgba(255,255,255,0.04)' }
    : { cardLine: 'rgba(25,25,40,0.09)', rowLine: 'rgba(25,25,40,0.07)', hover: 'rgba(25,25,40,0.04)' };
  return {
    ...line, bg: t.bg, side: t.side, card: t.card, field: t.field, text: t.text, muted: t.muted, faint: t.faint,
    primary: t.a1, secondary: t.a2, icon: t.a1, tag: t.a3, issue: t.a3, late: t.late,
    prio1: t.a3, prio2: t.a1, prio3: t.late, check: t.check, switchOff: t.switchOff, glow: t.glow,
    chipBg: t.chipBg, chipText: t.chipText, chipLine: t.chipLine, thread: t.thread, wire: t.wire,
  };
}

const ring = (stroke: string, cx = 13, cy = 15, rot = -16, rx = 12, ry = 3.3) =>
  `<ellipse cx="${cx}" cy="${cy}" rx="${rx}" ry="${ry}" fill="none" stroke="${stroke}" stroke-width="1.5" transform="rotate(${rot} ${cx} ${cy})"/>`;

// Shared particle shapes (path d)
const RECT = 'M0 2h10v6H0z';                                      // viewBox 0 0 10 10
const TRI = 'M0 0h8l-4 10z';                                       // viewBox 0 0 10 10
const PETAL = 'M5 0C8.5 2 9.5 6 5 10C0.5 6 1.5 2 5 0z';            // viewBox 0 0 10 10
const DOT = 'M5 0a5 5 0 1 0 0 10a5 5 0 1 0 0-10z';                 // viewBox 0 0 10 10
const STAR = 'M10 1l2.6 5.6 6.1.7-4.5 4.2 1.2 6L10 14.6 4.6 17.5l1.2-6L1.3 7.3l6.1-.7z'; // viewBox 0 0 20 20
export const BAT = 'M12 2.6L11.1 0.8L12 1.8L12.9 0.8L12 2.6C13.4 2.6 14 4 14 4C16 1.5 20 1 24 3C21.5 3.5 20.5 5.5 20.5 7C19 5.8 17.5 6.2 16.5 7.5C15.5 6.2 14 6.5 12 9C10 6.5 8.5 6.2 7.5 7.5C6.5 6.2 5 5.8 3.5 7C3.5 5.5 2.5 3.5 0 3C4 1 8 1.5 10 4C10 4 10.6 2.6 12 2.6Z'; // viewBox 0 0 24 12

// ───────────────────────────── Holidays ─────────────────────────────

export const HOLIDAYS: Holiday[] = [
  // ── Christmas ──
  {
    id: 'christmas',
    name: 'Christmas',
    tokens: {
      dark: {
        bg: '#0f1714', side: '#0b110e', card: '#15211b', cardLine: 'rgba(255,255,255,0.06)', rowLine: 'rgba(255,255,255,0.05)', hover: 'rgba(255,255,255,0.04)', field: '#1a2821',
        text: '#f4efe4', muted: '#9aa89f', faint: '#5d6b63',
        primary: '#f0525a', secondary: '#e9b949', icon: '#e9b949', tag: '#e9b949', issue: '#4fbf7a', late: '#f0525a',
        prio1: '#4fbf7a', prio2: '#e9b949', prio3: '#f0525a',
        check: '#4a5a51', switchOff: '#33413a', glow: 'rgba(233,185,73,0.09)',
        chipBg: 'rgba(79,191,122,0.12)', chipText: '#7fd8a1', chipLine: 'rgba(79,191,122,0.25)',
        thread: '#2c3a32', wire: '#2c3a32',
        flake: '#ffffff', cap: '#f4f7f5', capLine: 'rgba(0,0,0,0)', bulbCap: '#2c3a32',
      },
      light: {
        bg: '#fbf8f2', side: '#f2ede3', card: '#ffffff', cardLine: 'rgba(25,45,35,0.09)', rowLine: 'rgba(25,45,35,0.07)', hover: 'rgba(25,45,35,0.04)', field: '#e9e3d6',
        text: '#1d2a23', muted: '#6b766f', faint: '#a9b2ac',
        primary: '#c62a3a', secondary: '#a8770f', icon: '#a8770f', tag: '#a8770f', issue: '#1f8a4c', late: '#c62a3a',
        prio1: '#1f8a4c', prio2: '#a8770f', prio3: '#c62a3a',
        check: '#b7bfb9', switchOff: '#d6d0c3', glow: 'rgba(198,42,58,0.06)',
        chipBg: 'rgba(79,191,122,0.12)', chipText: '#1f7a44', chipLine: 'rgba(79,191,122,0.25)',
        thread: '#8f8473', wire: '#8f8473',
        flake: '#9fb8c8', cap: '#ffffff', capLine: 'rgba(60,90,110,0.22)', bulbCap: '#8f8473',
      },
    },
    art: {
      logo: `<svg viewBox="0 0 28 28" width="28" height="28"><circle cx="13" cy="16" r="8" fill="#c62a3a"/><ellipse cx="13" cy="16.5" rx="12" ry="3.4" fill="none" stroke="#e9b949" stroke-width="1.6" transform="rotate(-18 13 16.5)"/><path d="M6.5 11.5c1-4.5 6-7.5 11.5-6.2l4.5 5.2c-5-1.8-11-1.3-16 1z" fill="#e5484d"/><path d="M6 11.6c4.6-2.3 11-2.6 16.6-.9" stroke="#f4f7f5" stroke-width="2.4" stroke-linecap="round" fill="none"/><circle cx="23" cy="10.6" r="2.4" fill="#f4f7f5" stroke="#cfc9bd" stroke-width="0.6"/></svg>`,
      chipIcon: `<svg viewBox="0 0 16 16" width="14" height="14"><path d="M8 1.5l3.6 5h-2l3.2 4.6H3.2l3.2-4.6h-2z" fill="#4fbf7a"/><rect x="7" y="11.1" width="2" height="3" fill="#a0703a"/><circle cx="8" cy="1.6" r="1.2" fill="#e9b949"/></svg>`,
      // snowflake; stroke uses currentColor = tokens.text
      toggleIcon: `<svg viewBox="0 0 16 16" width="15" height="15" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"><path d="M8 1.5v13M2.4 4.75l11.2 6.5M2.4 11.25l11.2-6.5M6.5 2.5L8 4l1.5-1.5M6.5 13.5L8 12l1.5 1.5"/></svg>`,
      tile: {
        background: 'linear-gradient(140deg,#d8343f,#9c1727)',
        overlay: 'repeating-linear-gradient(45deg,rgba(255,255,255,0.07) 0 7px,transparent 7px 14px)', // candy cane
      },
      tileIcon: '#c62a3a',
      iconBoxes: { calendar: '#1f7a4a', quickNotes: '#c9952b', quickDraw: '#c62a3a', myFiles: '#1f7a4a', workspace: '#1f7a4a', avatar: '#1f7a4a' },
      done: ['#ff7a82', '#c62a3a', 'rgba(240,82,90,0.5)'], // a red bauble
      bars: ['#f0525a', '#4fbf7a', '#e9b949', '#c084fc'],
      festiveBar: 'repeating-linear-gradient(160deg,#f0525a 0 5px,#f4f7f5 5px 10px)', // candy cane
      cardCorner: {
        kind: 'snowcap', // viewBox 0 0 400 18, preserveAspectRatio none; fill tokens.cap, stroke tokens.capLine
        paths: [
          'M0,10 C30,2 60,4 90,6 S150,0 200,4 S270,2 320,5 S380,3 400,10 C380,14 360,11 340,13 S300,16 280,12 S240,15 220,12 S180,14 160,11 S120,15 100,12 S50,14 0,10 Z',
          'M0,10 C40,3 70,6 110,4 S170,1 210,5 S260,1 300,4 S370,2 400,10 C370,14 350,11 320,13 S280,15 250,12 S200,15 180,12 S130,14 110,11 S60,15 0,10 Z',
        ],
      },
    },
    decor: { kind: 'lights', count: 24, colors: ['#ff4d5e', '#ffc94a', '#4fd18b', '#fff3d6'] },
    particles: { kind: 'fall', count: 80, viewBox: '0 0 10 10', shapes: [DOT], colors: ['var(--h-flake)'], size: [2, 5], dur: [9, 20], opacity: [0.3, 0.8] }, // blur(0.6px) when size > 3
    copy: {
      chip: n => `${plural(n, 'day')} to Christmas`,
      chipToday: 'Merry Christmas',
      effectsLabel: 'Let it snow',
      placeholder: 'Jot something down… making a list, checking it twice',
    },
    window: y => ({ start: d(y, 12, 1), end: d(y, 12, 26), peak: d(y, 12, 25) }),
  },

  // ── Halloween ──
  {
    id: 'halloween',
    name: 'Halloween',
    tokens: {
      dark: {
        bg: '#121017', side: '#0c0a10', card: '#1a1621', cardLine: 'rgba(255,255,255,0.06)', rowLine: 'rgba(255,255,255,0.05)', hover: 'rgba(255,255,255,0.04)', field: '#221c2a',
        text: '#f2ead8', muted: '#a59cb0', faint: '#5f5669',
        primary: '#ff7a1a', secondary: '#b48cff', icon: '#ff7a1a', tag: '#b48cff', issue: '#8bd450', late: '#ff5f57',
        prio1: '#8bd450', prio2: '#ff7a1a', prio3: '#ff5f57',
        check: '#4d4458', switchOff: '#352d3f', glow: 'rgba(255,122,26,0.08)',
        chipBg: 'rgba(255,122,26,0.12)', chipText: '#ffa45c', chipLine: 'rgba(255,122,26,0.3)',
        thread: 'rgba(225,215,240,0.45)', wire: 'rgba(225,215,240,0.45)',
        web: 'rgba(225,215,240,0.3)', spider: '#8a77a3', bat: '#5d4d70', moon: '#f3e9c6', moonGlow: 'rgba(243,233,198,0.16)',
      },
      light: {
        bg: '#f8f4ec', side: '#efe9df', card: '#ffffff', cardLine: 'rgba(40,25,50,0.09)', rowLine: 'rgba(40,25,50,0.07)', hover: 'rgba(40,25,50,0.04)', field: '#e6dfd3',
        text: '#221a2a', muted: '#6f6678', faint: '#b0a8b8',
        primary: '#d45f08', secondary: '#7440c9', icon: '#d45f08', tag: '#7440c9', issue: '#4b8a1c', late: '#d1322e',
        prio1: '#4b8a1c', prio2: '#d45f08', prio3: '#d1322e',
        check: '#bdb5c4', switchOff: '#d8d0c6', glow: 'rgba(212,95,8,0.07)',
        chipBg: 'rgba(212,95,8,0.1)', chipText: '#b04e05', chipLine: 'rgba(212,95,8,0.25)',
        thread: 'rgba(60,45,75,0.4)', wire: 'rgba(60,45,75,0.4)',
        web: 'rgba(60,45,75,0.25)', spider: '#2a2230', bat: '#2a2230', moon: '#f0dfa6', moonGlow: 'rgba(240,200,120,0.3)',
      },
    },
    art: {
      logo: `<svg viewBox="0 0 28 28" width="28" height="28"><path d="M14 7.5c1-2.5 2.5-3.5 4-3.8" stroke="#4b8a1c" stroke-width="2" stroke-linecap="round" fill="none"/><ellipse cx="14" cy="16" rx="10" ry="8.5" fill="#ff7a1a"/><path d="M14 7.6c-3 2-3 14.8 0 16.8M14 7.6c3 2 3 14.8 0 16.8M8 8.8c-3 3-3 11.4 0 14.4M20 8.8c3 3 3 11.4 0 14.4" stroke="#d4550a" stroke-width="1" fill="none"/><ellipse cx="14" cy="16.5" rx="13.2" ry="3.2" fill="none" stroke="#9b6bff" stroke-width="1.5" transform="rotate(-16 14 16.5)"/><path d="M9 14l2-2 2 2zM15 14l2-2 2 2zM9.5 18.5c1.5 1.4 3 1.8 4.5 1.8s3-.4 4.5-1.8l-1.2.4-.8 1-.9-.9-1.6.6-1.6-.6-.9.9-.8-1z" fill="#2a1a10"/></svg>`,
      chipIcon: `<svg viewBox="0 0 16 16" width="15" height="15"><path d="M8 4.2c.4-1.4 1.2-2.2 2.2-2.5" stroke="#4b8a1c" stroke-width="1.4" stroke-linecap="round" fill="none"/><ellipse cx="8" cy="9.3" rx="6.2" ry="5.2" fill="#ff7a1a"/><path d="M5 8l1.2-1.3L7.4 8zM8.6 8l1.2-1.3L11 8zM5 10.6c1 .9 2 1.2 3 1.2s2-.3 3-1.2l-.9.3-.6.6-.7-.5-.8.5-.8-.5-.7.5-.6-.6z" fill="#3a1500"/></svg>`,
      toggleIcon: `<svg viewBox="0 0 24 12" width="16" height="10" fill="currentColor"><path d="${BAT}"/></svg>`,
      tile: {
        background: 'linear-gradient(150deg,#ff8f33,#cf500a)',
        overlay: 'repeating-linear-gradient(90deg,rgba(90,30,0,0.12) 0 2px,transparent 2px 18px)', // pumpkin ribs
        // + faint carved face bottom-right: <svg 44×30 viewBox 0 0 44 30, opacity .35> path below
      },
      tileIcon: '#d4550a',
      iconBoxes: { calendar: '#6b3fc4', quickNotes: '#6b3fc4', quickDraw: '#d4550a', myFiles: '#4b8a1c', workspace: '#6b3fc4', avatar: '#6b3fc4' },
      done: ['#ffb066', '#e5620d', 'rgba(255,122,26,0.55)'],
      bars: ['#ff7a1a', '#8bd450', '#9b6bff', '#ff5f57'],
      festiveBar: 'repeating-linear-gradient(160deg,#ff7a1a 0 5px,#2a1f33 5px 10px)',
      cardCorner: { kind: 'cobweb' }, // on BOTH cards
      moon: true, // 70px circle, top 34px right 120px of main column: radial-gradient(circle at 38% 36%,#fffaf0,var(--h-moon) 60%), box-shadow 0 0 50px 18px var(--h-moonGlow), 3 craters rgba(160,140,90,.2–.25)
    },
    // spiders on threads (extra decor) — x%, thread length, scale 1
    decor: null,
    extraDecor: {
      kind: 'drop', viewBox: [20, 16], items: [[64, 34, 1.1], [86, 104, 1.1], [95, 60, 1.1]],
      svg: () => `<svg viewBox="0 0 20 16" width="22" height="18"><path d="M7 7L2 3M7 8L1 8M7 9L2 13M8 10L5 15M13 7L18 3M13 8L19 8M13 9L18 13M12 10L15 15" stroke="var(--h-spider)" stroke-width="1.2" stroke-linecap="round" fill="none"/><ellipse cx="10" cy="9.5" rx="3.6" ry="4.2" fill="var(--h-spider)"/><circle cx="10" cy="4.6" r="2.3" fill="var(--h-spider)"/><circle cx="9.1" cy="4.4" r="0.6" fill="#ff7a1a"/><circle cx="10.9" cy="4.4" r="0.6" fill="#ff7a1a"/></svg>`,
    },
    particles: { kind: 'fly', count: 7, path: BAT, size: [18, 34], dur: [14, 26], opacity: [0.55, 0.95] }, // fill var(--h-bat); height = width/2
    copy: {
      chip: n => `${plural(n, 'day')} to Halloween`,
      chipToday: 'Happy Halloween',
      effectsLabel: 'Spooky effects',
      placeholder: 'Jot something down… before it vanishes',
    },
    window: y => ({ start: d(y, 10, 15), end: d(y, 11, 1), peak: d(y, 10, 31) }),
  },

  // ── New Year's Eve ──
  {
    id: 'nye',
    name: 'New Year’s Eve',
    tokens: {
      dark: fromAccents('dark', { bg: '#0c0e18', side: '#080a12', card: '#151927', field: '#1c2133', text: '#f3efe6', muted: '#9aa0b5', faint: '#545a70', a1: '#f2c75c', a2: '#a9b8ff', a3: '#ff6fae', late: '#ff6363', check: '#464c62', switchOff: '#2c3145', glow: 'rgba(242,199,92,0.09)', thread: 'rgba(242,199,92,0.45)', wire: '#3a3f55', chipBg: 'rgba(242,199,92,0.12)', chipText: '#f5d27e', chipLine: 'rgba(242,199,92,0.3)' }),
      light: fromAccents('light', { bg: '#faf8f3', side: '#f1eee6', card: '#ffffff', field: '#e8e4da', text: '#1b1d28', muted: '#6b6e7c', faint: '#b2b4bd', a1: '#a87a12', a2: '#4a5bd1', a3: '#c4337a', late: '#cf3434', check: '#c3c4cc', switchOff: '#d9d6cc', glow: 'rgba(168,122,18,0.07)', thread: 'rgba(140,110,40,0.5)', wire: '#a59f90', chipBg: 'rgba(168,122,18,0.1)', chipText: '#8a630c', chipLine: 'rgba(168,122,18,0.25)' }),
    },
    art: {
      logo: `<svg viewBox="0 0 28 28" width="28" height="28"><circle cx="13" cy="15" r="8" fill="#2f347a"/>${ring('#f2c75c')}<path d="M22 2l1.1 2.9L26 6l-2.9 1.1L22 10l-1.1-2.9L18 6l2.9-1.1z" fill="#f2c75c"/></svg>`,
      chipIcon: `<svg viewBox="0 0 16 16" width="15" height="15"><path d="M8 1l1.8 4.6L14.5 7l-4.7 1.6L8 13.5 6.2 8.6 1.5 7l4.7-1.4z" fill="#f2c75c"/></svg>`,
      toggleIcon: `<svg viewBox="0 0 16 16" width="16" height="16"><path d="M8 1l1.8 4.6L14.5 7l-4.7 1.6L8 13.5 6.2 8.6 1.5 7l4.7-1.4z" fill="#f2c75c"/></svg>`,
      tile: { background: 'linear-gradient(150deg,#3a3f8f,#141638)', overlay: 'radial-gradient(circle,rgba(242,199,92,0.55) 1px,transparent 1.6px) 0 0/12px 12px' },
      tileIcon: '#2f347a',
      iconBoxes: { calendar: '#4a5bd1', quickNotes: '#a87a12', quickDraw: '#c4337a', myFiles: '#2f347a', workspace: '#4a5bd1', avatar: '#4a5bd1' },
      done: ['#ffe08a', '#c9971f', 'rgba(242,199,92,0.55)'],
      bars: ['#f2c75c', '#a9b8ff', '#ff6fae', '#7d8cff'],
      festiveBar: 'repeating-linear-gradient(160deg,#f2c75c 0 5px,#c9ccd8 5px 10px)',
      cardCorner: { kind: 'svg', svg: `<svg viewBox="0 0 40 40" width="40" height="40"><path d="M28 4l1.6 4.4L34 10l-4.4 1.6L28 16l-1.6-4.4L22 10l4.4-1.6z" fill="#f2c75c"/><path d="M14 14l1 2.6 2.6 1-2.6 1-1 2.6-1-2.6-2.6-1 2.6-1z" fill="#c9ccd8"/><path d="M34 24l.8 2 2 .8-2 .8-.8 2-.8-2-2-.8 2-.8z" fill="#f2c75c"/></svg>` },
    },
    decor: {
      kind: 'drop', viewBox: [20, 20],
      items: [[57, 22, 0.8], [63, 66, 1.1], [69, 38, 0.7], [75, 96, 1.2], [81, 50, 0.8], [87, 118, 1.1], [92, 30, 0.7], [97, 78, 0.9]],
      svg: i => `<svg viewBox="0 0 20 20"><path d="${STAR}" fill="${i % 2 ? '#d9dde8' : '#f2c75c'}"/></svg>`,
    },
    particles: { kind: 'fall', count: 70, viewBox: '0 0 10 10', shapes: [RECT], colors: ['#f2c75c', '#d9dde8', '#ff6fae', '#7d8cff', '#ffe9a8'], size: [6, 11], dur: [7, 14], spin: 'fast', opacity: [0.6, 1] },
    copy: {
      chip: (n, unit = 'days', year) => `${plural(n, unit === 'hours' ? 'hour' : 'day')} to ${year}`,
      chipToday: 'Happy New Year',
      effectsLabel: 'Confetti',
      placeholder: 'Jot something down… resolutions welcome',
    },
    // Window spans the year boundary; resolver checks both y and y-1.
    window: y => ({ start: d(y, 12, 27), end: d(y + 1, 1, 1), peak: d(y + 1, 1, 1) }),
  },

  // ── Easter ──
  {
    id: 'easter',
    name: 'Easter',
    tokens: {
      dark: fromAccents('dark', { bg: '#14111c', side: '#0f0d16', card: '#1e1a29', field: '#262133', text: '#f6f2fb', muted: '#a9a1b9', faint: '#5c556b', a1: '#c6a4ff', a2: '#7fe0b5', a3: '#ffd86b', late: '#ff7a7a', check: '#4a4359', switchOff: '#332d42', glow: 'rgba(198,164,255,0.1)', thread: 'rgba(198,164,255,0.45)', wire: '#463f57', chipBg: 'rgba(127,224,181,0.12)', chipText: '#9be9c6', chipLine: 'rgba(127,224,181,0.3)' }),
      light: fromAccents('light', { bg: '#fbf9f4', side: '#f2eff7', card: '#ffffff', field: '#e8e3ef', text: '#231d2e', muted: '#6f6880', faint: '#b9b3c4', a1: '#7b4fd6', a2: '#1f9a6a', a3: '#a77b00', late: '#d1364a', check: '#cbc5d6', switchOff: '#dcd6e4', glow: 'rgba(123,79,214,0.06)', thread: 'rgba(123,79,214,0.4)', wire: '#b8b0c6', chipBg: 'rgba(31,154,106,0.1)', chipText: '#17805a', chipLine: 'rgba(31,154,106,0.25)' }),
    },
    art: {
      logo: `<svg viewBox="0 0 28 28" width="28" height="28"><path d="M8.6 9.5C7 6 7 1.5 8.8 1.2s2.4 4.3 1.8 8.1z" fill="#f3e9ff"/><path d="M15.4 9.3C15 5.5 16 1 17.8 1.4s1 4.8-.8 8.2z" fill="#f3e9ff"/><path d="M9.1 8.5c-.8-2.4-.7-5.2.1-5.4s1 2.7.6 5.4zM16.2 8.4c0-2.6.6-5.3 1.3-5.2s.1 2.9-.6 5.4z" fill="#ff9ec4"/><circle cx="13" cy="15" r="8" fill="#ffd86b"/>${ring('#c6a4ff')}</svg>`,
      chipIcon: `<svg viewBox="0 0 16 16" width="15" height="15"><path d="M8 1.5c2.9 0 5.2 4.6 5.2 8.2S10.9 14.5 8 14.5 2.8 13.3 2.8 9.7 5.1 1.5 8 1.5z" fill="#ffd86b"/><path d="M3.2 8.2l1.6-1.1 1.6 1.1 1.6-1.1 1.6 1.1 1.6-1.1 1.6 1.1.4.3v1.3l-.4-.3-1.6-1.1-1.6 1.1-1.6-1.1-1.6 1.1-1.6-1.1-1.6 1.1-.4-.3V8.5z" fill="#7b4fd6"/></svg>`,
      toggleIcon: `<svg viewBox="0 0 10 10" width="14" height="14"><path d="${PETAL}" fill="#ffb7d2"/></svg>`,
      tile: { background: 'linear-gradient(150deg,#a57bff,#6a3fc9)', overlay: 'radial-gradient(circle,rgba(255,255,255,0.25) 1.6px,transparent 2.2px) 0 0/13px 13px' },
      tileIcon: '#6a3fc9',
      iconBoxes: { calendar: '#7b4fd6', quickNotes: '#d9a400', quickDraw: '#e2679c', myFiles: '#1f9a6a', workspace: '#7b4fd6', avatar: '#7b4fd6' },
      done: ['#e3d1ff', '#9b6cf0', 'rgba(198,164,255,0.6)'],
      bars: ['#c6a4ff', '#7fe0b5', '#ffd86b', '#ff9ec4'],
      festiveBar: 'repeating-linear-gradient(160deg,#c6a4ff 0 4px,#7fe0b5 4px 8px,#ffd86b 8px 12px,#ff9ec4 12px 16px)',
      cardCorner: { kind: 'svg', svg: `<svg viewBox="0 0 40 40" width="40" height="40"><path d="M10 21c2.6 0 4.6 4.2 4.6 7.6S12.6 34 10 34s-4.6-2-4.6-5.4S7.4 21 10 21z" fill="#c6a4ff"/><path d="M5.6 27.5l1.5-1 1.5 1 1.5-1 1.5 1 1.5-1 1.5 1v1.3l-1.5-1-1.5 1-1.5-1-1.5 1-1.5-1-1.5 1z" fill="#fff8e6"/><path d="M24 14c3.2 0 5.6 5 5.6 9.2S27.2 30 24 30s-5.6-2.6-5.6-6.8S20.8 14 24 14z" fill="#ffd86b"/><circle cx="22" cy="21" r="1.1" fill="#ff9ec4"/><circle cx="26" cy="21" r="1.1" fill="#ff9ec4"/><circle cx="24" cy="25" r="1.1" fill="#ff9ec4"/><path d="M32 23c2.2 0 3.8 3.4 3.8 6.2S34.2 34 32 34s-3.8-2-3.8-4.8 1.6-6.2 3.8-6.2z" fill="#7fe0b5"/><path d="M2 40c1-4 2-7 3-8l1 5 2-7 2 6 2-7 2 7 2-6 2 6 2-7 2 7 2-6 2 6 2-7 2 6 2-5 2 6 1-4 2 8z" fill="#5cc98f"/></svg>` },
    },
    decor: {
      kind: 'drop', viewBox: [20, 26],
      items: [[58, 24, 0.8], [65, 72, 1.05], [72, 40, 0.8], [79, 100, 1.15], [86, 54, 0.9], [92, 118, 1], [97, 34, 0.8]],
      svg: i => {
        const c = ['#c6a4ff', '#7fe0b5', '#ffd86b', '#ff9ec4', '#8fc8ff'][i % 5];
        return `<svg viewBox="0 0 20 26"><path d="M10 1C15 1 19 9 19 15.5S15 25 10 25 1 22 1 15.5 5 1 10 1z" fill="${c}"/><path d="M1.4 12.2l3-2 3 2 2.6-2 2.6 2 3-2 3 2 .3.3v1.8l-.3-.3-3-2-3 2-2.6-2-2.6 2-3-2-3 2-.2-.1z" fill="#fff8e6"/><circle cx="6" cy="19" r="1.2" fill="#fff8e6"/><circle cx="10" cy="20" r="1.2" fill="#fff8e6"/><circle cx="14" cy="19" r="1.2" fill="#fff8e6"/></svg>`;
      },
    },
    particles: { kind: 'fall', count: 38, viewBox: '0 0 10 10', shapes: [PETAL], colors: ['#f3e9ff', '#ffd6e7', '#fff3c2', '#d4f5e6'], size: [7, 12], dur: [12, 20], spin: 'slow', opacity: [0.6, 1] },
    copy: {
      chip: n => `${plural(n, 'day')} to Easter`,
      chipToday: 'Happy Easter',
      effectsLabel: 'Spring petals',
      placeholder: 'Jot something down… before the egg hunt',
    },
    window: y => { const e = easterSunday(y); return { start: addDays(e, -7), end: addDays(e, 1), peak: e }; },
  },

  // ── Santos Populares (Lisbon) ──
  {
    id: 'santos',
    name: 'Santos Populares',
    tokens: {
      dark: fromAccents('dark', { bg: '#0d1320', side: '#090e18', card: '#151d2e', field: '#1c2639', text: '#f4f1ea', muted: '#9ba6b8', faint: '#536075', a1: '#ffc233', a2: '#4cb8ff', a3: '#46d17a', late: '#ff5f57', check: '#435068', switchOff: '#2a3549', glow: 'rgba(255,194,51,0.08)', thread: 'rgba(255,255,255,0.35)', wire: '#5a6680', chipBg: 'rgba(255,194,51,0.12)', chipText: '#ffd166', chipLine: 'rgba(255,194,51,0.3)' }),
      light: fromAccents('light', { bg: '#fffaf0', side: '#f6efe0', card: '#ffffff', field: '#ede4d1', text: '#1a2233', muted: '#667085', faint: '#b4bac6', a1: '#c98a00', a2: '#1a6fd1', a3: '#1f9a50', late: '#d8342c', check: '#c5c9d2', switchOff: '#dfd7c6', glow: 'rgba(26,111,209,0.06)', thread: 'rgba(60,70,90,0.35)', wire: '#8c94a3', chipBg: 'rgba(201,138,0,0.1)', chipText: '#9a6a00', chipLine: 'rgba(201,138,0,0.25)' }),
    },
    art: {
      logo: `<svg viewBox="0 0 28 28" width="28" height="28"><circle cx="13" cy="15" r="8" fill="#3fa9ff"/>${ring('#ffc233')}<path d="M20 2h6l-3 5z" fill="#ff4d4d"/><path d="M14 3h5l-2.5 4z" fill="#46d17a"/></svg>`,
      // sardine
      chipIcon: `<svg viewBox="0 0 16 16" width="15" height="15"><path d="M2 8c3-3.5 8-3.5 11 0-3 3.5-8 3.5-11 0z" fill="#9fb3c8"/><path d="M12.6 8l2.9-2.8v5.6z" fill="#9fb3c8"/><circle cx="4.8" cy="7.6" r="0.7" fill="#1a2233"/></svg>`,
      toggleIcon: `<svg viewBox="0 0 16 16" width="16" height="16"><path d="M2 8c3-3.5 8-3.5 11 0-3 3.5-8 3.5-11 0z" fill="#9fb3c8"/><path d="M12.6 8l2.9-2.8v5.6z" fill="#9fb3c8"/><circle cx="4.8" cy="7.6" r="0.7" fill="#1a2233"/></svg>`,
      tile: {
        background: 'linear-gradient(150deg,#2f6fd6,#163a86)',
        // azulejo grid
        overlay: 'radial-gradient(circle at 50% 50%,transparent 3.5px,rgba(255,255,255,0.18) 4px,rgba(255,255,255,0.18) 5px,transparent 5.5px) 0 0/14px 14px,linear-gradient(rgba(255,255,255,0.1) 1px,transparent 1px) 0 0/14px 14px,linear-gradient(90deg,rgba(255,255,255,0.1) 1px,transparent 1px) 0 0/14px 14px',
      },
      tileIcon: '#1a6fd1',
      iconBoxes: { calendar: '#1a6fd1', quickNotes: '#e0a100', quickDraw: '#e0453c', myFiles: '#1f9a50', workspace: '#1a6fd1', avatar: '#1a6fd1' },
      done: ['#7fc4ff', '#1a6fd1', 'rgba(76,184,255,0.55)'],
      bars: ['#ffc233', '#46d17a', '#4cb8ff', '#ff4d4d'],
      festiveBar: 'repeating-linear-gradient(160deg,#ff4d4d 0 4px,#ffd23f 4px 8px,#3ccf6e 8px 12px,#3fa9ff 12px 16px)',
      // manjerico (basil pot) with paper flag + carnation
      cardCorner: { kind: 'svg', svg: `<svg viewBox="0 0 40 40" width="40" height="40"><path d="M18 24h16v3H18z" fill="#d9784e"/><path d="M19.5 27h13l-2 10h-9z" fill="#c8643b"/><path d="M26 25c-6 0-9-4-8-8 0-4 4-7 8-7s8 3 8 7c1 4-2 8-8 8z" fill="#3caa55"/><path d="M24.6 2h.8v10h-.8z" fill="#8a6a3a"/><path d="M25.4 2h7l-2 2 2 2h-7z" fill="#3fa9ff"/><circle cx="31" cy="13" r="2.4" fill="#ff4d4d"/></svg>` },
    },
    decor: { kind: 'bunting', scallops: 6, depth: 40, perScallop: 7, colors: ['#ff4d4d', '#ffd23f', '#3ccf6e', '#3fa9ff', '#ff7ac6', '#ff9f1c'] },
    particles: { kind: 'fall', count: 45, viewBox: '0 0 10 10', shapes: [RECT, TRI], colors: ['#ff4d4d', '#ffd23f', '#3ccf6e', '#3fa9ff', '#ff7ac6'], size: [5, 9], dur: [9, 16], spin: 'fast', opacity: [0.6, 1] },
    copy: {
      chip: n => `${plural(n, 'day')} to Santo António`,
      chipToday: 'Bom Santo António',
      effectsLabel: 'Arraial confetti',
      placeholder: 'Jot something down… a quadra for the manjerico?',
    },
    window: y => ({ start: d(y, 6, 1), end: d(y, 6, 30), peak: d(y, 6, 13) }),
  },

  // ── Lunar New Year ──
  {
    id: 'lunar',
    name: 'Lunar New Year',
    tokens: {
      dark: fromAccents('dark', { bg: '#160b0b', side: '#100707', card: '#231313', field: '#2c1919', text: '#f8eee4', muted: '#b8a196', faint: '#66504a', a1: '#ff4d3d', a2: '#f5c04a', a3: '#ff8fb3', late: '#ff7a6b', check: '#54403b', switchOff: '#3b2724', glow: 'rgba(245,192,74,0.09)', thread: 'rgba(245,192,74,0.5)', wire: '#4d3632', chipBg: 'rgba(255,77,61,0.13)', chipText: '#ff8a7d', chipLine: 'rgba(255,77,61,0.32)' }),
      light: fromAccents('light', { bg: '#fff8f0', side: '#f8ecdf', card: '#ffffff', field: '#f0e2d2', text: '#2a1612', muted: '#7d675e', faint: '#c4b3aa', a1: '#c8202a', a2: '#a47410', a3: '#c2416f', late: '#b8261f', check: '#dccbc2', switchOff: '#ead9ca', glow: 'rgba(200,32,42,0.06)', thread: 'rgba(164,116,16,0.5)', wire: '#c9b39a', chipBg: 'rgba(200,32,42,0.08)', chipText: '#b01c25', chipLine: 'rgba(200,32,42,0.22)' }),
    },
    art: {
      logo: `<svg viewBox="0 0 28 28" width="28" height="28"><circle cx="13" cy="15" r="8" fill="#e0322a"/>${ring('#f5c04a')}<path d="M11 6.5h4v1.5h-4z" fill="#f5c04a"/></svg>`,
      chipIcon: `<svg viewBox="0 0 16 16" width="15" height="15"><path d="M6 1.5h4V3H6z" fill="#f5c04a"/><path d="M8 2.8c3.6 0 5.2 2.1 5.2 5.1s-1.6 5.1-5.2 5.1-5.2-2.1-5.2-5.1 1.6-5.1 5.2-5.1z" fill="#e0322a"/><path d="M5.6 12.6h4.8v1.2H5.6z" fill="#f5c04a"/><path d="M7.4 13.8h1.2v2H7.4z" fill="#f5c04a"/></svg>`,
      toggleIcon: `<svg viewBox="0 0 10 10" width="14" height="14"><path d="${PETAL}" fill="#ff8fb3"/></svg>`,
      tile: { background: 'linear-gradient(150deg,#ec4436,#9e1018)', overlay: 'radial-gradient(circle,rgba(245,192,74,0.4) 1.2px,transparent 1.8px) 0 0/11px 11px' },
      tileIcon: '#c8202a',
      iconBoxes: { calendar: '#c8202a', quickNotes: '#b8860b', quickDraw: '#c2416f', myFiles: '#7a2e1f', workspace: '#c8202a', avatar: '#c8202a' },
      done: ['#ff8a7d', '#c8202a', 'rgba(255,77,61,0.55)'],
      bars: ['#ff4d3d', '#f5c04a', '#ff8fb3', '#d4a017'],
      festiveBar: 'repeating-linear-gradient(160deg,#e0322a 0 5px,#f5c04a 5px 10px)',
      // plum blossom branch; flowers = 5 petal circles (r*0.5) at distance r*0.62 + centre (r*0.32)
      cardCorner: { kind: 'svg', svg: plumBlossomSvg() },
    },
    decor: {
      kind: 'drop', viewBox: [24, 36],
      items: [[60, 26, 0.9], [69, 76, 1.15], [78, 40, 0.85], [87, 104, 1.25], [95, 50, 0.95]],
      svg: () => `<svg viewBox="0 0 24 36"><path d="M8 1h8v3.5H8z" fill="#f5c04a"/><path d="M12 3.5c7.5 0 10.5 4.2 10.5 10.3S19.5 24 12 24 1.5 19.9 1.5 13.8 4.5 3.5 12 3.5z" fill="#e0322a"/><path d="M11.3 3.6c-3.2 2.3-3.2 18 0 20.3h1.4c3.2-2.3 3.2-18 0-20.3z" fill="#c21d22"/><path d="M8 23h8v3.5H8z" fill="#f5c04a"/><path d="M11 26.5h2v8.5h-2z" fill="#f5c04a"/></svg>`,
    },
    particles: { kind: 'fall', count: 40, viewBox: '0 0 10 10', shapes: [PETAL], colors: ['#ffb7c9', '#ff8fb3', '#ffd6df'], size: [7, 12], dur: [12, 20], spin: 'slow', opacity: [0.6, 1] },
    copy: {
      chip: n => `${plural(n, 'day')} to Lunar New Year`,
      chipToday: 'Happy Lunar New Year',
      effectsLabel: 'Blossoms',
      placeholder: 'Jot something down… wishes for the new year',
    },
    window: y => {
      const md = LUNAR_NEW_YEAR[y];
      if (!md) return { start: d(y, 1, 1), end: d(y, 1, 0), peak: d(y, 1, 1) }; // empty window
      const p = d(y, md[0], md[1]);
      return { start: addDays(p, -7), end: addDays(p, 15), peak: p };
    },
  },
];

function plumBlossomSvg() {
  const flower = (cx: number, cy: number, r: number, fill: string) => {
    let s = '';
    for (let k = 0; k < 5; k++) {
      const a = ((k * 72 - 90) * Math.PI) / 180;
      s += `<circle cx="${(cx + Math.cos(a) * r * 0.62).toFixed(2)}" cy="${(cy + Math.sin(a) * r * 0.62).toFixed(2)}" r="${(r * 0.5).toFixed(2)}" fill="${fill}"/>`;
    }
    return s + `<circle cx="${cx}" cy="${cy}" r="${(r * 0.32).toFixed(2)}" fill="#f5c04a"/>`;
  };
  return `<svg viewBox="0 0 40 40" width="40" height="40"><path d="M4 38c8-6 14-12 20-23l1.2.7C19 27 13 33 5 39z" fill="#7a4a35"/><path d="M17 26c3-1 6-1 9 0l-.3 1c-3-.8-5.6-.8-8.4 0z" fill="#7a4a35"/>${flower(26, 13, 7, '#ffb7c9')}${flower(14, 25, 5, '#ff8fb3')}${flower(33, 24, 4, '#ffd6df')}</svg>`;
}

// ───────────────────────────── Resolution ─────────────────────────────

export type HolidaySetting = 'auto' | 'off' | HolidayId;

const startOfDay = (dt: Date) => new Date(dt.getFullYear(), dt.getMonth(), dt.getDate());

export function resolveHoliday(setting: HolidaySetting, now = new Date()): Holiday | null {
  if (setting === 'off') return null;
  if (setting !== 'auto') return HOLIDAYS.find(h => h.id === setting) ?? null;
  const today = startOfDay(now);
  for (const h of HOLIDAYS) {
    for (const y of [now.getFullYear(), now.getFullYear() - 1]) { // -1 catches NYE on Jan 1
      const w = h.window(y);
      if (today >= w.start && today <= w.end) return h;
    }
  }
  return null;
}

/** Countdown text for the chip, or chipToday on the peak day. */
export function countdown(h: Holiday, now = new Date()): string {
  const y = now.getFullYear();
  let w = h.window(y);
  if (startOfDay(now) > w.end) w = h.window(y + 1);
  if (h.id === 'nye') {
    if (now.getMonth() === 0 && now.getDate() === 1) return h.copy.chipToday;
    const target = w.peak; // Jan 1 00:00
    const hours = Math.ceil((+target - +now) / 3.6e6);
    return hours <= 24
      ? h.copy.chip(hours, 'hours', target.getFullYear())
      : h.copy.chip(Math.ceil(hours / 24), 'days', target.getFullYear());
  }
  const days = Math.round((+startOfDay(w.peak) - +startOfDay(now)) / 864e5);
  return days <= 0 ? h.copy.chipToday : h.copy.chip(days);
}

/** Tokens → CSS custom properties (prefix --h-). Apply on the app root. */
export function cssVars(h: Holiday, mode: Mode): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(h.tokens[mode])) out[`--h-${k}`] = v;
  return out;
}
