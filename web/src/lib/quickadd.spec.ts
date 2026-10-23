import { describe, it, expect } from 'vitest'
import { parseQuick } from './quickadd'

// Tuesday 13 October 2026, 10:00 local.
const now = new Date(2026, 9, 13, 10, 0)

describe('parseQuick', () => {
  it('pulls dates, times, priority and tags out of the title', () => {
    expect(parseQuick('Essay tomorrow 14:30 !! #maths', now)).toEqual({ title: 'Essay', due: '2026-10-14', due_time: '14:30', priority: 2, tags: ['maths'] })
  })
  it('understands Portuguese', () => {
    expect(parseQuick('Comprar pão amanhã às 9h #casa', now)).toEqual({ title: 'Comprar pão', due: '2026-10-14', due_time: '09:00', tags: ['casa'] })
  })
  it('takes a weekday as the next one, and next week', () => {
    expect(parseQuick('Call mum friday', now).due).toBe('2026-10-16')
    expect(parseQuick('Gym tuesday', now).due).toBe('2026-10-20') // today is Tuesday: next week's
    expect(parseQuick('Report next week', now)).toMatchObject({ title: 'Report', due: '2026-10-20' })
  })
  it('a time alone means today; 12-hour clocks work', () => {
    expect(parseQuick('Meeting 2pm', now)).toMatchObject({ title: 'Meeting', due: '2026-10-13', due_time: '14:00' })
  })
  it('leaves plain text alone', () => {
    expect(parseQuick('Read chapter 12', now)).toEqual({ title: 'Read chapter 12', tags: [] })
    expect(parseQuick('Friday', now)).toEqual({ title: 'Friday', tags: [] })
  })
  it('reads repeats, in English and Portuguese, and starts on the first day', () => {
    // Today is Tuesday 13 October.
    expect(parseQuick('Bins every other Tuesday', now)).toEqual({ title: 'Bins', due: '2026-10-13', repeat: 'FREQ=WEEKLY;INTERVAL=2;BYDAY=TU', tags: [] })
    expect(parseQuick('Gym every monday and friday 18h', now)).toMatchObject({ title: 'Gym', due: '2026-10-16', due_time: '18:00', repeat: 'FREQ=WEEKLY;BYDAY=MO,FR' })
    expect(parseQuick('Water plants every 3 days', now)).toMatchObject({ title: 'Water plants', due: '2026-10-13', repeat: 'FREQ=DAILY;INTERVAL=3' })
    expect(parseQuick('Rent every month tomorrow', now)).toMatchObject({ title: 'Rent', due: '2026-10-14', repeat: 'FREQ=MONTHLY' })
    expect(parseQuick('Standup every weekday 9:30', now)).toMatchObject({ title: 'Standup', repeat: 'FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR', due_time: '09:30' })
    expect(parseQuick('Lixo terça sim, terça não', now)).toMatchObject({ title: 'Lixo', repeat: 'FREQ=WEEKLY;INTERVAL=2;BYDAY=TU' })
    expect(parseQuick('Natação todas as quintas-feiras', now)).toMatchObject({ title: 'Natação', due: '2026-10-15', repeat: 'FREQ=WEEKLY;BYDAY=TH' })
    expect(parseQuick('Regar de 15 em 15 dias', now)).toMatchObject({ title: 'Regar', repeat: 'FREQ=DAILY;INTERVAL=15' })
    expect(parseQuick('Relatório a cada duas semanas', now)).toMatchObject({ title: 'Relatório', repeat: 'FREQ=WEEKLY;INTERVAL=2' })
    expect(parseQuick('Backup semanalmente !', now)).toMatchObject({ title: 'Backup', repeat: 'FREQ=WEEKLY', priority: 1 })
  })
  it('leaves repeat-like words that are part of the title', () => {
    expect(parseQuick('Weekly review', now)).toEqual({ title: 'Weekly review', tags: [] })
    expect(parseQuick('Read Everyday Things', now)).toEqual({ title: 'Read Everyday Things', tags: [] })
    expect(parseQuick('Plan every month', now).repeat).toBe('FREQ=MONTHLY')
  })
})
