import { describe, expect, it } from 'vitest'
import { deflateSync, strToU8 } from 'fflate'
import { bytesOfDataUrl, editorUrl, fileOf, labelsOf, lightPicture, originOf, textOfDataUrl } from './drawio'

const model = '<mxGraphModel><root><mxCell id="0"/><mxCell id="2" value="Order &lt;b&gt;placed&lt;/b&gt;" vertex="1"/><UserObject id="3" label="Shipped" owner="Ana"><mxCell vertex="1"/></UserObject></root></mxGraphModel>'

describe('drawio', () => {
  it('builds the editor address', () => {
    const u = new URL(editorUrl('https://drawio.example/app/', 'pt', true))
    expect(u.origin + u.pathname).toBe('https://drawio.example/app/')
    expect(u.searchParams.get('embed')).toBe('1')
    expect(u.searchParams.get('proto')).toBe('json')
    expect(u.searchParams.get('lang')).toBe('pt')
    expect(u.searchParams.get('dark')).toBe('1')
    expect(originOf('https://drawio.example:8443/app')).toBe('https://drawio.example:8443')
    expect(originOf('nope')).toBe('')
  })

  it('reads the labels of plain and compressed pages', () => {
    const plain = `<mxfile><diagram name="Checkout">${model}</diagram></mxfile>`
    expect(labelsOf(plain).split('\n')).toEqual(['Checkout', 'Order placed', 'Shipped', 'Ana'])
    const packed = btoa(String.fromCharCode(...deflateSync(strToU8(encodeURIComponent(model)))))
    expect(labelsOf(`<mxfile><diagram name="Page-1">${packed}</diagram></mxfile>`).split('\n')).toEqual(['Order placed', 'Shipped', 'Ana'])
    expect(labelsOf('not xml')).toBe('')
  })

  it('finds the file in a draw.io SVG', () => {
    const file = `<mxfile><diagram name="A">${model}</diagram></mxfile>`
    const esc = file.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
    expect(fileOf(`<svg xmlns="http://www.w3.org/2000/svg" content="${esc}"><g/></svg>`)).toBe(file)
    expect(fileOf(`  ${file}`)).toBe(file)
    expect(fileOf('<svg xmlns="http://www.w3.org/2000/svg"/>')).toBe('')
    expect(fileOf('hello')).toBe('')
  })

  it('reads a data: URL', () => {
    expect(textOfDataUrl(`data:image/svg+xml;base64,${btoa('<svg>é</svg>'.replace('é', 'Ã©'))}`)).toBe('<svg>é</svg>')
    expect(textOfDataUrl('data:image/svg+xml,%3Csvg%2F%3E')).toBe('<svg/>')
    const png = bytesOfDataUrl(`data:image/png;base64,${btoa('\x89PNG')}`)
    expect(png.type).toBe('image/png')
    expect([...png.bytes]).toEqual([0x89, 0x50, 0x4e, 0x47])
    expect(lightPicture('<svg style="color-scheme: light dark;">')).toBe('<svg style="color-scheme: light;">')
  })
})
