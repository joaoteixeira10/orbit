import { describe, expect, it } from 'vitest'
import { inside, pathTo, sizeText, tree, type Folder } from './filesApi'

const f = (id: string, parent_id: string, name = id): Folder => ({ id, parent_id, name, workspace_id: 'w', created_by: '', created_at: '' })
const folders = [f('a', '', 'Design'), f('b', 'a', 'Logos'), f('c', 'b', 'Old'), f('d', '', 'Admin')]

describe('folders', () => {
  it('walks down from the top', () => {
    expect(pathTo(folders, 'c').map((x) => x.name)).toEqual(['Design', 'Logos', 'Old'])
    expect(pathTo(folders, '')).toEqual([])
    // A loop in the data doesn't hang.
    expect(pathTo([f('x', 'y'), f('y', 'x')], 'x')).toHaveLength(2)
  })
  it('knows what is inside a folder', () => {
    expect([...inside(folders, 'a')].sort()).toEqual(['a', 'b', 'c'])
    expect([...inside(folders, 'd')]).toEqual(['d'])
  })
  it('lists the tree in order, with depths', () => {
    expect(tree(folders).map((x) => `${x.depth}:${x.folder.name}`)).toEqual(['0:Admin', '0:Design', '1:Logos', '2:Old'])
  })
  it('reads sizes', () => {
    expect(sizeText(512, 'en')).toBe('512 B')
    expect(sizeText(1536, 'en')).toBe('1.5 KB')
    expect(sizeText(3 * 1024 * 1024, 'pt')).toBe('3 MB')
  })
})
