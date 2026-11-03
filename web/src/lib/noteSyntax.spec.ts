import { describe, expect, it } from 'vitest'
import { guessNoteLang, noteTokens, tokensHtml } from './noteSyntax'

const VAULT = `# 4. Add entity to group

Get groups id

\`\`\`
vault read -field=id identity/group/name/linux-users
\`\`\`

then:

\`\`\`
vault write identity/group/id/<GROUP_ID> \\
  member_entity_ids="<ENTITY_ID>"
\`\`\``

describe('guessNoteLang', () => {
  it.each([
    ['vault notes', VAULT, 'markdown'],
    ['json', '{\n  "name": "orbit",\n  "port": 8080,\n  "tags": ["a", "b"]\n}', 'json'],
    ['a shebang', '#!/bin/bash\nset -e\necho hi', 'bash'],
    ['python', 'import os\n\ndef main():\n    for f in os.listdir("."):\n        print(f)\n\nif __name__ == "__main__":\n    main()', 'python'],
    ['nginx', 'server {\n    listen 443 ssl;\n    server_name orbit.example.org;\n    location / {\n        proxy_pass http://127.0.0.1:8080;\n        proxy_set_header Host $host;\n    }\n}', 'nginx'],
    ['yaml', 'services:\n  orbit:\n    image: harbor.ubonito.org/orbit/orbit:0.1.0\n    ports:\n      - "8080:8080"\n    environment:\n      PUBLIC_URL: https://orbit.example.org\n    volumes:\n      - data:/data', 'yaml'],
    ['sql', "SELECT id, title FROM tasks\nWHERE due < CURRENT_DATE AND completed_at IS NULL\nORDER BY due DESC\nLIMIT 20;", 'sql'],
    ['go', 'package main\n\nimport "fmt"\n\nfunc main() {\n\tfor i := 0; i < 3; i++ {\n\t\tfmt.Println(i)\n\t}\n}', 'go'],
    ['dockerfile', 'FROM node:22-alpine AS web\nWORKDIR /src\nCOPY package.json ./\nRUN npm ci\nCOPY . .\nRUN npm run build', 'dockerfile'],
    ['prose', 'Call Ana tomorrow about the report and ask her if the meeting is still on Friday. Bring the printed copies.', 'text'],
    ['prose pt', 'Ligar à Ana amanhã sobre o relatório e perguntar se a reunião ainda é na sexta.\nLevar as cópias impressas.', 'text'],
    ['a list', 'Groceries for the week\n- milk\n- bread\n- eggs', 'markdown'],
    ['short', 'milk', 'text'],
    ['shell commands', 'docker compose up -d\ndocker logs -f orbit\ncurl -fsS http://localhost:8080/api/v1/readyz', 'bash'],
    ['math', 'rent = 500\nfood = 200\ntotal = rent + food =', 'text'],
    ['labels', 'Meeting: Friday\nRoom: 3\nWho: Ana, Rui', 'text'],
    ['typescript', 'interface User {\n  id: string\n  name: string\n}\nexport const me: User = { id: "1", name: "Ana" }', 'typescript'],
  ])('%s', (_, text, want) => {
    expect(guessNoteLang(text)).toBe(want)
  })
})

describe('noteTokens', () => {
  it('colours Markdown: a heading, fences, the code in them, inline marks', () => {
    const toks = noteTokens(VAULT, 'markdown')
    const of = (cls: string) => toks.filter((t) => t.cls.split(' ').includes(cls)).map((t) => VAULT.slice(t.from, t.to))
    expect(of('md-heading')).toEqual(['# 4. Add entity to group'])
    expect(of('md-fence')).toHaveLength(4)
    expect(of('md-codeblock').join('')).toContain('vault read -field=id')
    // A block that doesn't say its language: one line of shell is enough.
    expect(toks.some((t) => t.cls.includes('md-codeblock') && t.cls.includes('hljs-'))).toBe(true)
    const t = noteTokens('- **milk** and `jq .` see https://example.org', 'markdown')
    expect(t.map((x) => x.cls)).toEqual(['md-bullet', 'md-strong', 'md-code', 'md-link'])
  })
  it('colours code with highlight.js classes, in order, none overlapping', () => {
    const code = '{"a": 1, "b": [true, "x"]}'
    const toks = noteTokens(code, 'json')
    expect(toks.some((t) => t.cls.includes('hljs-attr'))).toBe(true)
    for (let i = 1; i < toks.length; i++) expect(toks[i].from).toBeGreaterThanOrEqual(toks[i - 1].to)
  })
  it('plain text has no colours; HTML is escaped', () => {
    expect(noteTokens('a <b> c', 'text')).toEqual([])
    expect(tokensHtml('<b> **x**', noteTokens('<b> **x**', 'markdown'))).toBe('&lt;b&gt; <span class="md-strong">**x**</span>')
  })
})
