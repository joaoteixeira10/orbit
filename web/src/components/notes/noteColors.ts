// Note colours: the tint a quick note wears on the wall, in light and dark
// mode, plus an "ink" (a saturated system colour for checkboxes and focus
// on that tint). The keys are the API's NoteColor values ('' = plain).
//
// Anything with a data-note="<key>" attribute gets --note-bg / --note-ink /
// --note-line set for the current theme once ensureNoteColorStyles() has run
// (it injects one small <style> generated from this map).
import type { NoteColor } from '@/api/planner'

export interface NoteTint {
  light: string // background in light mode (Stickies-like pastel)
  dark: string // background in dark mode (deep, muted)
  ink: string // checkbox / accent on that tint
}

// The order the picker shows them in.
export const NOTE_COLORS: NoteColor[] = ['', 'yellow', 'orange', 'red', 'pink', 'purple', 'blue', 'teal', 'green', 'brown', 'gray']

export const noteColors: Record<NoteColor, NoteTint> = {
  '': { light: '#ffffff', dark: '#2c2c2e', ink: '' }, // plain = the surface colour
  yellow: { light: '#fff4b8', dark: '#4a4220', ink: '#d9a400' },
  orange: { light: '#ffe2c4', dark: '#4d3420', ink: '#ff9500' },
  red: { light: '#ffd9d6', dark: '#4f2725', ink: '#ff3b30' },
  pink: { light: '#ffdbe8', dark: '#4a2639', ink: '#ff2d55' },
  purple: { light: '#ebdffb', dark: '#3a2e52', ink: '#af52de' },
  blue: { light: '#d8e9ff', dark: '#1f3550', ink: '#007aff' },
  teal: { light: '#cdeff0', dark: '#1d4245', ink: '#30b0c7' },
  green: { light: '#dcf3d2', dark: '#263f27', ink: '#34c759' },
  brown: { light: '#ede1d3', dark: '#3e3328', ink: '#a2845e' },
  gray: { light: '#e9e9ed', dark: '#3a3a3d', ink: '#8e8e93' },
}

// noteTint returns the tint of a colour key (unknown keys read as plain).
export function noteTint(c: string | null | undefined): NoteTint {
  return noteColors[(c ?? '') as NoteColor] ?? noteColors['']
}

// noteColorCss is the stylesheet behind data-note="…".
export function noteColorCss(): string {
  const rules: string[] = []
  for (const key of NOTE_COLORS) {
    const c = noteColors[key]
    // A sheet (Overlay) holding a note editor takes the note's tint too.
    const sel = `[data-note="${key}"],.sheet-panel:has(.note-sheet[data-note="${key}"])`
    if (key === '') {
      rules.push(`${sel}{--note-bg:var(--surface);--note-ink:var(--ws);--note-line:var(--border)}`)
      continue
    }
    rules.push(`${sel}{--note-bg:${c.light};--note-ink:${c.ink};--note-line:rgba(0,0,0,.07)}`)
    rules.push(`html.dark :is(${sel}){--note-bg:${c.dark};--note-line:rgba(255,255,255,.08)}`)
  }
  rules.push('.sheet-panel:has(.note-sheet){background:var(--note-bg)!important;transition:background .25s ease}')
  return rules.join('\n')
}

// ensureNoteColorStyles injects the data-note stylesheet once.
export function ensureNoteColorStyles(): void {
  if (typeof document === 'undefined' || document.getElementById('orbit-note-colors')) return
  const el = document.createElement('style')
  el.id = 'orbit-note-colors'
  el.textContent = noteColorCss()
  document.head.appendChild(el)
}
