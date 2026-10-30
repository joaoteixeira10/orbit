import { describe, expect, it } from 'vitest'
import { addNode, connect, naturalSize, flowNodes, kindOf, labelsOf, MERMAID_TEMPLATES, nextNodeId, quoteLabel } from './mermaid'

describe('mermaid', () => {
  it('tells the kind from the first word', () => {
    expect(kindOf('graph LR\n A-->B')).toBe('flowchart')
    expect(kindOf('%% a note\n\nsequenceDiagram\n A->>B: x')).toBe('sequence')
    expect(kindOf('stateDiagram-v2\n [*] --> A')).toBe('state')
    expect(kindOf('nonsense')).toBe('')
    for (const tpl of MERMAID_TEMPLATES) expect(kindOf(tpl.code)).toBe(tpl.key)
  })

  it("finds a flowchart's nodes, not its labels", () => {
    const code = `flowchart TD
  A[Start here] --> B{Is it ok?}
  B -- Yes --> C[Done]
  B -->|No| D(Fix it) & E
  classDef hot fill:#f00
  F:::hot --- A
  subgraph Group
    G([Pill]) -.-> H
  end`
    expect(flowNodes(code)).toEqual(['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H'])
  })

  it('adds nodes with ids not used yet', () => {
    const code = 'flowchart TD\n  N1[One] --> N3'
    expect(nextNodeId(code)).toBe('N2')
    const r = addNode(code, 'Two: more', 'N1')
    expect(r.id).toBe('N2')
    expect(r.code).toBe('flowchart TD\n  N1[One] --> N3\n  N1 --> N2["Two: more"]')
    expect(flowNodes(r.code)).toEqual(['N1', 'N3', 'N2'])
    expect(addNode('graph LR\n', '').code).toBe('graph LR\n  N1[N1]')
  })

  it('connects two nodes', () => {
    expect(connect('graph LR\n  A\n  B\n', 'A', 'B')).toBe('graph LR\n  A\n  B\n  A --> B')
    expect(connect('graph LR', 'A', 'B', 'go-to')).toBe('graph LR\n  A -- go to --> B')
  })

  it('quotes labels with syntax in them', () => {
    expect(quoteLabel('Plain words')).toBe('Plain words')
    expect(quoteLabel('a (b)')).toBe('"a (b)"')
    expect(quoteLabel('say "hi"')).toBe('"say #quot;hi#quot;"')
  })

  it('keeps the words for search', () => {
    expect(labelsOf('flowchart TD\n  A[Order placed] --> B{Paid?}\n  %% hidden')).toBe('A Order placed B Paid?')
  })

  it('gives the picture its own size', () => {
    const svg = '<svg id="m" width="100%" xmlns="http://www.w3.org/2000/svg" style="max-width: 210.5px;" viewBox="-8 -8 210.5 300.2"><g/></svg>'
    expect(naturalSize(svg)).toBe('<svg width="211" height="301" id="m" xmlns="http://www.w3.org/2000/svg" viewBox="-8 -8 210.5 300.2"><g/></svg>')
    expect(naturalSize('<svg/>')).toBe('<svg/>')
  })
})
