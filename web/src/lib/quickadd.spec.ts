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
})
