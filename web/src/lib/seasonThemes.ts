/**
 * Orbit — Season themes: spring, summer, autumn and winter. Like a holiday
 * theme, a season colours the app (the same --h-* tokens: the sidebar, the
 * main column, every window, popover and menu) and lets something drift
 * across the window; like one, it hangs things along the top of Home
 * (blossoms, bunting, leaves, snowflakes) and dresses a card's corner;
 * unlike one, it has no mark, countdown or capture line. A holiday in its
 * days wins over the season. The words are in i18n: holiday.seasons.*.
 */
import { DOT, PETAL, SNOWCAP, fromAccents, type Look } from '@/lib/holidayThemes';

export type SeasonId = 'spring' | 'summer' | 'autumn' | 'winter';
export type SeasonSetting = 'auto' | 'off' | SeasonId;

export interface Season extends Look {
  id: SeasonId;
  /** Inclusive local-date window (the northern hemisphere's seasons). */
  window: (year: number) => { start: Date; end: Date };
}

const d = (y: number, m: number, day: number) => new Date(y, m - 1, day);
const f2 = (n: number) => n.toFixed(2);

// ── The decorations' shapes ──
/** A five-petal blossom with a golden heart. */
function blossom(cx: number, cy: number, r: number, fill: string) {
  let out = '';
  for (let k = 0; k < 5; k++) {
    const a = ((k * 72 - 90) * Math.PI) / 180;
    out += `<circle cx="${f2(cx + Math.cos(a) * r * 0.62)}" cy="${f2(cy + Math.sin(a) * r * 0.62)}" r="${f2(r * 0.5)}" fill="${fill}"/>`;
  }
  return out + `<circle cx="${cx}" cy="${cy}" r="${f2(r * 0.3)}" fill="#f2cf5b"/>`;
}
/** A maple leaf, tip up, stem down (viewBox 0 0 24 28). */
const MAPLE_R: [number, number][] = [[12, 1], [13.8, 5.6], [16.8, 4.2], [16.2, 8.8], [21.8, 6.8], [20.2, 11.2], [23.2, 12.6], [18.6, 15.4], [19.8, 18.2], [14.6, 17.2], [12.7, 20.4]];
const MAPLE = 'M' + [...MAPLE_R, ...MAPLE_R.slice(1).reverse().map(([x, y]) => [24 - x, y] as [number, number])].map(([x, y]) => `${x} ${y}`).join('L') + 'Z';
const maple = (fill: string) =>
  `<rect x="11.4" y="19.5" width="1.2" height="7.5" fill="#6b4630"/><path d="${MAPLE}" fill="${fill}"/><path d="M12 20V3M12 20L20.5 8M12 20L3.5 8M12 20l6.5-3.5M12 20l-6.5-3.5" stroke="rgba(0,0,0,.16)" stroke-width=".7" fill="none"/>`;
/** An oval leaf, stem up (viewBox 0 0 24 28). */
const ovalLeaf = (fill: string) =>
  `<rect x="11.4" y="0" width="1.2" height="6" fill="#6b4630"/><path d="M12 5C19 9 19.5 20 12 27C4.5 20 5 9 12 5z" fill="${fill}"/><path d="M12 6v19" stroke="rgba(0,0,0,.16)" stroke-width=".8"/>`;
/** A six-armed snowflake, branched (viewBox 0 0 20 20). */
function snowflake(stroke: string) {
  let p = '';
  for (let k = 0; k < 6; k++) {
    const a = (k * 60 * Math.PI) / 180;
    const at = (r: number, da = 0) => `${f2(10 + Math.cos(a + da) * r)} ${f2(10 + Math.sin(a + da) * r)}`;
    p += `M10 10L${at(8.6)}M${at(5)}L${at(7.4, 0.42)}M${at(5)}L${at(7.4, -0.42)}`;
  }
  return `<svg viewBox="0 0 20 20"><path d="${p}" stroke="${stroke}" stroke-width="1.3" stroke-linecap="round" fill="none"/></svg>`;
}
// Where things hang: in the right part of Home's column (x %, thread, scale).
const HANG: [number, number, number][] = [[58, 22, 0.85], [64, 70, 1.1], [70, 38, 0.8], [76, 96, 1.2], [82, 52, 0.9], [88, 116, 1.1], [93, 30, 0.8], [97, 78, 0.95]];

/** The landscape behind the liquid glass (components/holiday/GlassScene.vue):
 *  the sky from top to horizon, the sun's glow, five ridges from far (r1) to
 *  near (r5), and the mist between them. Far is pale, near is deep: the air
 *  between. Tokens like the rest (--h-scSky…), so a holiday falls back to
 *  mixes of its own colours (styles/glass.css). Each season adds its own: spring's
 *  cherry trees and autumn's maples (scBark, the crown's three colours
 *  scCrown1…3), winter's pines under snow (scPine, scSnow), summer's lake
 *  and sailboat (scWater1…2, scSail, scHull). */
type Scene = { scSky: string; scLow: string; scSun: string; scR1: string; scR2: string; scR3: string; scR4: string; scR5: string; scMist: string; scBark?: string; scCrown1?: string; scCrown2?: string; scCrown3?: string; scPine?: string; scSnow?: string; scWater1?: string; scWater2?: string; scSail?: string; scHull?: string };
const SCENE: Record<SeasonId, { light: Scene; dark: Scene }> = {
  spring: {
    light: { scSky: '#e4f1f6', scLow: '#fbe9ef', scSun: '#fff6e4', scR1: '#dcebd8', scR2: '#c5e0bf', scR3: '#a7d0a0', scR4: '#86bb80', scR5: '#649e62', scMist: '#fbf2f5', scBark: '#6e4a40', scCrown1: '#ffe1ec', scCrown2: '#ffbcd4', scCrown3: '#f48fb4' },
    dark: { scSky: '#0c1712', scLow: '#241f2c', scSun: '#4a4236', scR1: '#3b5244', scR2: '#304539', scR3: '#26382e', scR4: '#1c2b23', scR5: '#131f19', scMist: '#2f4237', scBark: '#21161a', scCrown1: '#d79ab3', scCrown2: '#b46f8c', scCrown3: '#8c4c68' },
  },
  summer: {
    light: { scSky: '#cde7f6', scLow: '#fdeed4', scSun: '#fff3c6', scR1: '#d6dfd4', scR2: '#ecd8a8', scR3: '#e0c27e', scR4: '#c9a660', scR5: '#93a75c', scMist: '#fff7e8', scWater1: '#9fd5e6', scWater2: '#4f9fc2', scSail: '#fffdf6', scHull: '#c9563c' },
    dark: { scSky: '#0b1626', scLow: '#2c2f40', scSun: '#5e5232', scR1: '#3f506a', scR2: '#34445b', scR3: '#2a374b', scR4: '#212b3b', scR5: '#171f2b', scMist: '#36455c', scWater1: '#26405e', scWater2: '#101b2a', scSail: '#c8d2e0', scHull: '#7a3a2c' },
  },
  autumn: {
    light: { scSky: '#f5e4d2', scLow: '#f8d6b4', scSun: '#ffe6c2', scR1: '#efd2b8', scR2: '#e5bc9b', scR3: '#d9a07c', scR4: '#c88660', scR5: '#ad6a47', scMist: '#fbefe2', scBark: '#5a3a2a', scCrown1: '#f6b24a', scCrown2: '#e2692e', scCrown3: '#b8402a' },
    dark: { scSky: '#1a120c', scLow: '#3a2618', scSun: '#7a4422', scR1: '#5a3e2b', scR2: '#4a3324', scR3: '#3b281c', scR4: '#2c1e15', scR5: '#1f150e', scMist: '#4a3526', scBark: '#140d09', scCrown1: '#b8742e', scCrown2: '#9a4524', scCrown3: '#76301e' },
  },
  winter: {
    light: { scSky: '#deeaf5', scLow: '#f3f7fb', scSun: '#ffffff', scR1: '#e2ebf3', scR2: '#cfdcea', scR3: '#b9cbdd', scR4: '#9eb5cb', scR5: '#8099b3', scMist: '#f5f9fc', scPine: '#3d6658', scSnow: '#ffffff' },
    dark: { scSky: '#0a1320', scLow: '#1e2f46', scSun: '#3d5a80', scR1: '#3b5170', scR2: '#30445e', scR3: '#26374d', scR4: '#1d2a3c', scR5: '#141e2b', scMist: '#2f4460', scPine: '#12241e', scSnow: '#c9d7e8' },
  },
};

export const SEASONS: Season[] = [
  // ── Spring: new leaves and blossom ──
  {
    id: 'spring',
    tokens: {
      dark: { ...fromAccents('dark', { bg: '#0f1510', side: '#0b100c', card: '#172019', field: '#1e2a20', text: '#eef5ec', muted: '#9fb0a2', faint: '#56665a', a1: '#6fd17f', a2: '#ff9ec4', a3: '#f2cf5b', late: '#ff7a7a', check: '#45544a', switchOff: '#2d3a30', glow: 'rgba(111,209,127,0.08)', thread: 'rgba(111,209,127,0.4)', wire: '#34443a', chipBg: 'rgba(255,158,196,0.12)', chipText: '#ffb3d1', chipLine: 'rgba(255,158,196,0.3)' }), ...SCENE.spring.dark },
      light: { ...fromAccents('light', { bg: '#f8fbf5', side: '#eef4ea', card: '#ffffff', field: '#e3ecdd', text: '#1c2a1f', muted: '#66756a', faint: '#adb8af', a1: '#2f8a45', a2: '#c4457a', a3: '#a77b00', late: '#d1364a', check: '#c3cdc4', switchOff: '#d9e2d4', glow: 'rgba(61,154,79,0.07)', thread: 'rgba(47,138,69,0.35)', wire: '#a9b8ab', chipBg: 'rgba(196,69,122,0.1)', chipText: '#a83866', chipLine: 'rgba(196,69,122,0.25)' }), ...SCENE.spring.light },
    },
    art: {
      tile: { background: 'linear-gradient(150deg,#5cbf6c,#2b7f40)', overlay: 'radial-gradient(circle,rgba(255,214,231,0.45) 1.4px,transparent 2px) 0 0/13px 13px' },
      tileIcon: '#2b7f40',
      iconBoxes: { calendar: '#2f8a45', quickNotes: '#c99a1a', quickDraw: '#c4457a', myFiles: '#2f8a45', workspace: '#2f8a45', avatar: '#2f8a45' },
      done: ['#a6eab0', '#3d9a4f', 'rgba(111,209,127,0.5)'],
      bars: ['#6fd17f', '#ff9ec4', '#f2cf5b', '#8fc8ff'],
      festiveBar: 'repeating-linear-gradient(160deg,#6fd17f 0 5px,#ff9ec4 5px 10px)',
      // A cherry twig in flower.
      cardCorner: { kind: 'svg', svg: `<svg viewBox="0 0 40 40" width="40" height="40"><path d="M2 37c9-4 17-11 24-24l1.3.7C20 27 12 34 3 38.5z" fill="#7a4a35"/><path d="M13 28.5c4-.6 7.3.3 10.4 2l-.5.9c-3-1.5-6-2.3-9.6-1.9z" fill="#7a4a35"/><ellipse cx="19" cy="22" rx="3.2" ry="1.4" transform="rotate(-40 19 22)" fill="#6fbf6c"/><ellipse cx="9" cy="33" rx="2.8" ry="1.2" transform="rotate(20 9 33)" fill="#6fbf6c"/>${blossom(27, 11, 7, '#ffb7d2')}${blossom(13, 24, 5, '#ffd6e7')}${blossom(25, 31, 4.5, '#ff9ec4')}</svg>` },
    },
    decor: { kind: 'drop', viewBox: [20, 20], items: HANG, svg: i => `<svg viewBox="0 0 20 20">${blossom(10, 10, 9, ['#ffb7d2', '#ffd6e7', '#ff9ec4', '#fff0f6'][i % 4])}</svg>` },
    particles: { kind: 'fall', count: 26, viewBox: '0 0 10 10', shapes: [PETAL], colors: ['#ffd6e7', '#ffb7d2', '#fff4f8', '#e8f7d9'], size: [7, 11], dur: [14, 22], spin: 'slow', opacity: [0.55, 0.95] },
    window: y => ({ start: d(y, 3, 20), end: d(y, 6, 20) }),
  },

  // ── Summer: sun and sea ──
  {
    id: 'summer',
    tokens: {
      dark: { ...fromAccents('dark', { bg: '#0e1418', side: '#0a0f13', card: '#162028', field: '#1c2832', text: '#f5f1e8', muted: '#9fadb8', faint: '#55636e', a1: '#ffb347', a2: '#4cc9e8', a3: '#ff7f8f', late: '#ff6b5e', check: '#435260', switchOff: '#2a3742', glow: 'rgba(255,179,71,0.09)', thread: 'rgba(255,179,71,0.4)', wire: '#34424e', chipBg: 'rgba(76,201,232,0.12)', chipText: '#7fdaf0', chipLine: 'rgba(76,201,232,0.3)' }), ...SCENE.summer.dark },
      light: { ...fromAccents('light', { bg: '#fffaf2', side: '#f8efe0', card: '#ffffff', field: '#f0e5d2', text: '#1f2430', muted: '#6c7280', faint: '#b5b8c0', a1: '#c46a00', a2: '#0a7fa6', a3: '#c9405c', late: '#d8342c', check: '#cfc8bb', switchOff: '#e6dccb', glow: 'rgba(255,170,40,0.1)', thread: 'rgba(196,106,0,0.35)', wire: '#c2b6a2', chipBg: 'rgba(10,127,166,0.1)', chipText: '#08739a', chipLine: 'rgba(10,127,166,0.25)' }), ...SCENE.summer.light },
    },
    art: {
      tile: { background: 'linear-gradient(150deg,#ffb347,#ee6a2f)', overlay: 'radial-gradient(circle at 82% 18%,rgba(255,255,255,0.4),transparent 42%)' },
      tileIcon: '#e06a1f',
      iconBoxes: { calendar: '#0a7fa6', quickNotes: '#e09a12', quickDraw: '#c9405c', myFiles: '#0a7fa6', workspace: '#0a7fa6', avatar: '#0a7fa6' },
      done: ['#ffd99a', '#f08a1c', 'rgba(255,179,71,0.55)'],
      bars: ['#ffb347', '#4cc9e8', '#ff7f8f', '#7ed99b'],
      festiveBar: 'repeating-linear-gradient(160deg,#ffb347 0 5px,#4cc9e8 5px 10px)',
      // The sun and a slice of lemon.
      cardCorner: { kind: 'svg', svg: `<svg viewBox="0 0 40 40" width="40" height="40"><g stroke="#ffb347" stroke-width="2" stroke-linecap="round">${[0, 45, 90, 135, 180, 225, 270, 315].map(a => { const r = (a * Math.PI) / 180; return `<path d="M${f2(28 + Math.cos(r) * 9)} ${f2(12 + Math.sin(r) * 9)}L${f2(28 + Math.cos(r) * 12)} ${f2(12 + Math.sin(r) * 12)}"/>`; }).join('')}</g><circle cx="28" cy="12" r="6.5" fill="#ffb347"/><circle cx="26" cy="10" r="2.4" fill="#ffd27a"/><circle cx="12" cy="29" r="8" fill="#f5c400"/><circle cx="12" cy="29" r="6.6" fill="#fff4b8"/><g stroke="#f5d84a" stroke-width="1">${[0, 60, 120].map(a => { const r = (a * Math.PI) / 180; return `<path d="M${f2(12 - Math.cos(r) * 6)} ${f2(29 - Math.sin(r) * 6)}L${f2(12 + Math.cos(r) * 6)} ${f2(29 + Math.sin(r) * 6)}"/>`; }).join('')}</g></svg>` },
    },
    // Motes of sunlight rising slowly.
    decor: { kind: 'bunting', scallops: 6, depth: 40, perScallop: 7, colors: ['#ffb347', '#4cc9e8', '#ff7f8f', '#7ed99b', '#ffe066', '#0a7fa6'] },
    particles: { kind: 'rise', count: 30, viewBox: '0 0 10 10', shapes: [DOT], colors: ['#ffd27a', '#fff1c9', '#ffb347'], size: [2, 5], dur: [16, 28], opacity: [0.25, 0.6] },
    window: y => ({ start: d(y, 6, 21), end: d(y, 9, 22) }),
  },

  // ── Autumn: amber, rust and falling leaves ──
  {
    id: 'autumn',
    tokens: {
      dark: { ...fromAccents('dark', { bg: '#15100c', side: '#100c09', card: '#211913', field: '#2a2019', text: '#f6ece0', muted: '#b3a291', faint: '#66574b', a1: '#ff8a3d', a2: '#e8b04a', a3: '#d58ab4', late: '#ff6b5e', check: '#54463b', switchOff: '#3a2e25', glow: 'rgba(255,138,61,0.08)', thread: 'rgba(255,138,61,0.4)', wire: '#45372c', chipBg: 'rgba(255,138,61,0.12)', chipText: '#ffa66b', chipLine: 'rgba(255,138,61,0.3)' }), ...SCENE.autumn.dark },
      light: { ...fromAccents('light', { bg: '#fbf6ef', side: '#f3eadf', card: '#ffffff', field: '#ece0d0', text: '#2a1f17', muted: '#776a5e', faint: '#bcb0a4', a1: '#b14d14', a2: '#8f630c', a3: '#7d3c62', late: '#c0392b', check: '#d1c5b8', switchOff: '#e3d7c8', glow: 'rgba(194,86,26,0.07)', thread: 'rgba(177,77,20,0.35)', wire: '#c4b4a2', chipBg: 'rgba(177,77,20,0.1)', chipText: '#9a420f', chipLine: 'rgba(177,77,20,0.25)' }), ...SCENE.autumn.light },
    },
    art: {
      tile: { background: 'linear-gradient(150deg,#e07a35,#9c3a14)', overlay: 'repeating-linear-gradient(135deg,rgba(255,220,160,0.1) 0 6px,transparent 6px 14px)' },
      tileIcon: '#b14d14',
      iconBoxes: { calendar: '#b14d14', quickNotes: '#c9842f', quickDraw: '#7d3c62', myFiles: '#6f7a2a', workspace: '#b14d14', avatar: '#b14d14' },
      done: ['#ffc08a', '#d2601d', 'rgba(255,138,61,0.55)'],
      bars: ['#ff8a3d', '#e8b04a', '#d58ab4', '#a8b84f'],
      festiveBar: 'repeating-linear-gradient(160deg,#ff8a3d 0 5px,#e8b04a 5px 10px)',
      // A twig with leaves and an acorn.
      cardCorner: { kind: 'svg', svg: `<svg viewBox="0 0 40 40" width="40" height="40"><path d="M3 37C12 31 20 22 30 8l1 .7C22 23 14 32 4 38.5z" fill="#6b4630"/><g transform="translate(20 0) scale(.62) rotate(28 12 14)">${maple('#d9622b')}</g><g transform="translate(3 13) scale(.55) rotate(-62 12 14)">${ovalLeaf('#e8a33d')}</g><g transform="translate(13 20) scale(.48) rotate(72 12 14)">${maple('#b8442a')}</g><rect x="29.6" y="22" width=".9" height="3" fill="#6b4630"/><ellipse cx="30" cy="31" rx="4.2" ry="5" fill="#c98b3c"/><path d="M25 28a5 3.6 0 0 1 10 0z" fill="#7a5233"/></svg>` },
    },
    decor: { kind: 'drop', viewBox: [24, 28], items: HANG, svg: i => { const c = ['#d9622b', '#e8a33d', '#b8442a', '#c9842f', '#8f8a2d'][i % 5]; return `<svg viewBox="0 0 24 28">${i % 3 === 2 ? ovalLeaf(c) : `<g transform="rotate(180 12 14)">${maple(c)}</g>`}</svg>`; } },
    particles: { kind: 'fall', count: 22, viewBox: '0 0 10 10', shapes: [PETAL], colors: ['#d9622b', '#e8a33d', '#b8442a', '#c9842f'], size: [9, 14], dur: [12, 20], spin: 'slow', opacity: [0.6, 0.95] },
    window: y => ({ start: d(y, 9, 23), end: d(y, 12, 20) }),
  },

  // ── Winter: ice and snow ──
  {
    id: 'winter',
    tokens: {
      dark: { ...fromAccents('dark', { bg: '#0c1118', side: '#090d13', card: '#141b25', field: '#1a2330', text: '#eef3f8', muted: '#9aa8b8', faint: '#525f6e', a1: '#7cc4ff', a2: '#a9b4ff', a3: '#6fe0d6', late: '#ff7a85', check: '#415064', switchOff: '#283444', glow: 'rgba(124,196,255,0.08)', thread: 'rgba(124,196,255,0.4)', wire: '#2f3b4c', chipBg: 'rgba(124,196,255,0.12)', chipText: '#a4d6ff', chipLine: 'rgba(124,196,255,0.3)' }), flake: '#ffffff', cap: '#eef3f8', capLine: 'rgba(0,0,0,0)', ...SCENE.winter.dark },
      light: { ...fromAccents('light', { bg: '#f6f9fc', side: '#ebf1f7', card: '#ffffff', field: '#e1e9f1', text: '#17202b', muted: '#637083', faint: '#aab4c0', a1: '#2a72ad', a2: '#4f5fb3', a3: '#1f8a87', late: '#cf3a4a', check: '#c2ccd6', switchOff: '#d6dee7', glow: 'rgba(47,127,193,0.07)', thread: 'rgba(42,114,173,0.35)', wire: '#a9b6c4', chipBg: 'rgba(42,114,173,0.1)', chipText: '#22669e', chipLine: 'rgba(42,114,173,0.25)' }), flake: '#9fb8cf', cap: '#ffffff', capLine: 'rgba(60,90,120,0.22)', ...SCENE.winter.light },
    },
    art: {
      tile: { background: 'linear-gradient(150deg,#6fb3f0,#2a64a6)', overlay: 'radial-gradient(circle,rgba(255,255,255,0.5) 1.2px,transparent 1.8px) 0 0/12px 12px' },
      tileIcon: '#2a64a6',
      iconBoxes: { calendar: '#2a72ad', quickNotes: '#4f5fb3', quickDraw: '#1f8a87', myFiles: '#2a72ad', workspace: '#2a72ad', avatar: '#2a72ad' },
      done: ['#bfe3ff', '#3a85c6', 'rgba(124,196,255,0.55)'],
      bars: ['#7cc4ff', '#a9b4ff', '#6fe0d6', '#d6e2ee'],
      festiveBar: 'repeating-linear-gradient(160deg,#7cc4ff 0 5px,#eef3f8 5px 10px)',
      // Snow along the top of the cards.
      cardCorner: { kind: 'snowcap', paths: SNOWCAP },
    },
    decor: { kind: 'drop', viewBox: [20, 20], items: HANG, svg: i => snowflake(['var(--h-flake)', '#7cc4ff', '#a9b4ff'][i % 3]) },
    particles: { kind: 'fall', count: 55, viewBox: '0 0 10 10', shapes: [DOT], colors: ['var(--h-flake)'], size: [2, 5], dur: [10, 22], opacity: [0.3, 0.75] },
    // Spans the year boundary; the resolver checks this year's and last year's.
    window: y => ({ start: d(y, 12, 21), end: d(y + 1, 3, 19) }),
  },
];

const startOfDay = (dt: Date) => new Date(dt.getFullYear(), dt.getMonth(), dt.getDate());

/** The season the setting shows today: off → none; a season → that one;
 *  auto → the one today is in. */
export function resolveSeason(setting: SeasonSetting, now = new Date()): Season | null {
  if (setting === 'off') return null;
  if (setting !== 'auto') return SEASONS.find(s => s.id === setting) ?? null;
  const today = startOfDay(now);
  for (const s of SEASONS) {
    for (const y of [now.getFullYear(), now.getFullYear() - 1]) { // -1 catches winter after New Year
      const w = s.window(y);
      if (today >= w.start && today <= w.end) return s;
    }
  }
  return null;
}
