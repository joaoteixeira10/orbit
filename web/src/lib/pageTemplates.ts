// Page templates: a new, empty page can start from one — a few built in
// (meeting notes, class notes, a weekly review) or any page of the workspace
// marked as a template. Their titles and documents may hold placeholders,
// filled in when used: {{date}}, {{time}}, {{weekday}}, {{week}}, {{year}},
// {{monday}} (this week's Monday), {{me}} and {{workspace}}.
import { dateStr, startOfWeek } from './dates'

export interface PageTemplate {
  key: string
  title: string
  content: string
}

const BUILT_IN: Record<'en' | 'pt', PageTemplate[]> = {
  en: [
    {
      key: 'meeting',
      title: 'Meeting — {{date}}',
      content: `**Date:** {{weekday}}, {{date}} · {{time}}
**With:**

## Agenda

1.

## Notes

## Decisions

-

## Action items

- [ ] `,
    },
    {
      key: 'class',
      title: 'Class notes — {{date}}',
      content: `**Course:**
**Topic:**

## Key ideas

-

## Notes

## Questions

-

## Summary

## To review

- [ ] `,
    },
    {
      key: 'weekly',
      title: 'Week {{week}} review',
      content: `*The week of {{monday}}*

## What went well

-

## What didn't

-

## What I learned

-

## Next week's priorities

1.
2.
3.

## Loose ends

- [ ] `,
    },
  ],
  pt: [
    {
      key: 'meeting',
      title: 'Reunião — {{date}}',
      content: `**Data:** {{weekday}}, {{date}} · {{time}}
**Com:**

## Ordem de trabalhos

1.

## Notas

## Decisões

-

## Próximos passos

- [ ] `,
    },
    {
      key: 'class',
      title: 'Apontamentos — {{date}}',
      content: `**Disciplina:**
**Tema:**

## Ideias principais

-

## Notas

## Dúvidas

-

## Resumo

## Para rever

- [ ] `,
    },
    {
      key: 'weekly',
      title: 'Revisão da semana {{week}}',
      content: `*A semana de {{monday}}*

## O que correu bem

-

## O que não correu

-

## O que aprendi

-

## Prioridades da próxima semana

1.
2.
3.

## Pontas soltas

- [ ] `,
    },
  ],
}

export function builtInTemplates(lang: string): PageTemplate[] {
  return BUILT_IN[lang === 'pt' ? 'pt' : 'en']
}

// isoWeek is the ISO 8601 week number of d.
export function isoWeek(d: Date): number {
  const t = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()))
  const day = t.getUTCDay() || 7
  t.setUTCDate(t.getUTCDate() + 4 - day)
  const jan1 = new Date(Date.UTC(t.getUTCFullYear(), 0, 1))
  return Math.ceil(((t.getTime() - jan1.getTime()) / 86400000 + 1) / 7)
}

export interface FillContext {
  now: Date
  locale: string // e.g. en-GB, pt-PT
  me?: string
  workspace?: string
}

// fill puts the values in a template's placeholders (unknown ones stay).
export function fill(text: string, c: FillContext): string {
  const long = (d: Date) => new Intl.DateTimeFormat(c.locale, { day: 'numeric', month: 'long', year: 'numeric' }).format(d)
  const values: Record<string, () => string> = {
    date: () => long(c.now),
    iso: () => dateStr(c.now),
    time: () => `${String(c.now.getHours()).padStart(2, '0')}:${String(c.now.getMinutes()).padStart(2, '0')}`,
    weekday: () => new Intl.DateTimeFormat(c.locale, { weekday: 'long' }).format(c.now),
    week: () => String(isoWeek(c.now)),
    year: () => String(c.now.getFullYear()),
    monday: () => long(startOfWeek(c.now, 1)),
    me: () => c.me ?? '',
    workspace: () => c.workspace ?? '',
  }
  return text.replace(/\{\{\s*(\w+)\s*\}\}/g, (m, k: string) => (values[k] ? values[k]() : m))
}
