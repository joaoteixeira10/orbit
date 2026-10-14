import { describe, it, expect, afterEach } from 'vitest'
import { i18n } from '@/i18n'
import { renderNotification } from './inbox'

afterEach(() => {
  i18n.global.locale.value = 'en'
})

describe('renderNotification', () => {
  it('words one and many in English and Portuguese', () => {
    const n = { verb: 'task_added', count: 1, params: { actor: 'Ana', title: 'Milk', ws: 'Home' } }
    expect(renderNotification(n)).toEqual({ title: 'Ana added a task', body: 'Milk · Home' })
    i18n.global.locale.value = 'pt'
    expect(renderNotification({ ...n, count: 3 })).toEqual({ title: 'Ana adicionou 3 tarefas', body: 'A mais recente: Milk · Home' })
  })
  it('translates roles and drops a dangling separator', () => {
    i18n.global.locale.value = 'pt'
    expect(renderNotification({ verb: 'added', count: 1, params: { actor: 'Ana', ws: 'Casa', role: 'viewer' } }).body).toBe('Como leitor')
    i18n.global.locale.value = 'en'
    expect(renderNotification({ verb: 'task_due', count: 1, params: { title: 'Essay', ws: '' } }).body).toBe('Due now')
  })
  it('lists the daily summary', () => {
    const r = renderNotification({
      verb: 'daily',
      count: 1,
      params: { tasks: 2, events: 1, items: [{ k: 'event', t: 'Class', at: '10:30' }, { k: 'overdue', t: 'Essay' }, { k: 'task', t: 'Lab', at: '14:00' }] },
    })
    expect(r.title).toBe('Your day: 2 tasks, 1 event')
    expect(r.body).toBe('◷ 10:30 Class\n⚠ Essay\n○ Lab (14:00)')
  })
  it('names an untitled page', () => {
    expect(renderNotification({ verb: 'page_added', count: 1, params: { actor: 'Ana', title: '', ws: 'Work' } }).body).toBe('Untitled · Work')
  })
})
