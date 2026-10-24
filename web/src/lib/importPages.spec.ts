import { describe, it, expect } from 'vitest'
import { zipSync, strToU8 } from 'fflate'
import { csvTable, gather, plan, resolve, rewriteLinks, splitTitle, type InFile } from './importPages'
import { htmlToMarkdown } from './importRun'

const f = (path: string, text = '') => ({ path, data: strToU8(text) })
const ID = '0123456789abcdef0123456789abcdef'
const ID2 = 'fedcba9876543210fedcba9876543210'

describe('planning an import', () => {
  it('lays out a Notion export: sub-pages in their folders, ids off the names, a database with its rows', () => {
    const files: InFile[] = [
      f(`Home ${ID}.md`, '# Home\n\nHi'),
      f(`Home ${ID}/Child ${ID2}.md`, '# Child'),
      f(`Home ${ID}/pic.png`),
      f(`Home ${ID}/Tasks ${ID2}.csv`, 'Name,Done\nA,No'),
      f(`Home ${ID}/Tasks ${ID2}_all.csv`, 'Name,Done\nA,No\nB,Yes'),
      f(`Home ${ID}/Tasks ${ID2}/Row ${ID}.md`, '# Row'),
    ]
    const p = plan(files)
    const by = Object.fromEntries(p.map((x) => [x.title, x]))
    expect(p.map((x) => x.title)).toEqual(['Home', 'Child', 'Tasks', 'Row'])
    expect(by.Child.parent).toBe(by.Home.key)
    expect(by.Tasks.kind).toBe('csv')
    expect(by.Tasks.file!.path).toMatch(/_all\.csv$/)
    expect(by.Row.parent).toBe(by.Tasks.key)
  })
  it('lays out Apple Notes: folders become pages; a note alone in its folder keeps it for its files', () => {
    const p = plan([f('iCloud Notes/Notes/Work/Plan/Plan.txt', 'Plan'), f('iCloud Notes/Notes/Work/Plan/photo.jpg'), f('iCloud Notes/Notes/Shopping.html', '<p>Milk</p>')])
    const by = Object.fromEntries(p.map((x) => [x.title, x]))
    expect(Object.keys(by).sort()).toEqual(['Notes', 'Plan', 'Shopping', 'Work'])
    expect(by.Notes.parent).toBeNull() // "iCloud Notes" wraps everything: no page
    expect(by.Work.parent).toBe(by.Notes.key)
    expect(by.Plan.parent).toBe(by.Work.key)
    expect(by.Plan.folder).toBe('iCloud Notes/Notes/Work/Plan')
  })
  it('opens zips, and zips in them', async () => {
    // (Bytes made here: jsdom's TextEncoder answers from another realm.)
    const u8 = (s: string) => new Uint8Array(strToU8(s))
    const inner = zipSync({ [`Home ${ID}.md`]: u8('# Home') })
    const outer = zipSync({ 'Export-Part-1.zip': inner, 'readme.txt': u8('x') })
    // (jsdom's File can't hold these bytes: a stand-in with what gather reads.)
    const file = { name: 'Export.zip', arrayBuffer: async () => outer.buffer.slice(outer.byteOffset, outer.byteOffset + outer.byteLength) } as unknown as File
    const got = await gather([file])
    expect(got.map((x) => x.path).sort()).toEqual([`Home ${ID}.md`, 'readme.txt'])
  })
})

describe('converting', () => {
  it('resolves relative, encoded links and leaves the others', () => {
    expect(resolve(`Home ${ID}`, `Child%20${ID2}.md`)).toBe(`Home ${ID}/Child ${ID2}.md`)
    expect(resolve('a/b', '../c.png#x')).toBe('a/c.png')
    expect(resolve('a', 'https://example.org')).toBe('')
    expect(resolve('a', '#top')).toBe('')
  })
  it('rewrites the links it knows, keeps the rest', async () => {
    const md = `See [Child](Child%20${ID2}.md), ![pic](pic.png "Pic") and [web](https://x.org).`
    const out = await rewriteLinks(md, '', async (p, img) => (p.endsWith('.md') ? '/w/1/p/9' : img ? '/api/v1/uploads/u1' : null))
    expect(out).toBe('See [Child](/w/1/p/9), ![pic](/api/v1/uploads/u1 "Pic") and [web](https://x.org).')
  })
  it('takes the title off the top, and writes CSV as a table', () => {
    expect(splitTitle('# Home\n\nHi')).toEqual({ title: 'Home', body: 'Hi' })
    expect(csvTable('Name,Notes\nA,"one, two"\nB,"x|y"')).toBe('| Name | Notes |\n| --- | --- |\n| A | one, two |\n| B | x\\|y |')
  })
  it('reads HTML as Markdown, checklists included', () => {
    const md = htmlToMarkdown('<h1>Trip</h1><p><b>Bring</b>:</p><ul data-type="taskList"><li data-type="taskItem" data-checked="true"><p>tent</p></li><li data-type="taskItem" data-checked="false"><p>food</p></li></ul>')
    expect(md).toBe('# Trip\n\n**Bring**:\n\n- [x] tent\n- [ ] food')
  })
})
