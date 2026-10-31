// How a file is shown in place (the file viewer): a PDF's pages, a Word
// document as a page, a spreadsheet's tabs, a presentation's slides, a
// picture, a video or a song, text — or nothing (Download opens it).
import { reactive } from 'vue'
import type { PDFDocumentProxy } from 'pdfjs-dist'
import { filesApi } from './filesApi'

export type ViewKind = 'pdf' | 'docx' | 'sheet' | 'csv' | 'pptx' | 'image' | 'video' | 'audio' | 'markdown' | 'text' | 'none'

const ext = (name: string) => name.split('.').pop()?.toLowerCase() ?? ''

export function viewKind(name: string, type: string): ViewKind {
  const e = ext(name)
  if (e === 'pdf' || type === 'application/pdf') return 'pdf'
  if (e === 'docx' || e === 'docm' || e === 'dotx') return 'docx'
  if (e === 'xlsx' || e === 'xlsm') return 'sheet'
  if (e === 'csv' || e === 'tsv') return 'csv'
  if (e === 'pptx' || e === 'pptm' || e === 'ppsx') return 'pptx'
  if (type.startsWith('image/') && type !== 'image/svg+xml') return 'image'
  if (type.startsWith('video/') || ['mp4', 'webm', 'mov', 'm4v'].includes(e)) return 'video'
  if (type.startsWith('audio/') || ['mp3', 'm4a', 'wav', 'ogg', 'flac', 'aac'].includes(e)) return 'audio'
  if (e === 'md' || e === 'markdown') return 'markdown'
  if (type.startsWith('text/') || ['txt', 'log', 'json', 'yaml', 'yml', 'xml', 'ini', 'toml', 'sql', 'js', 'ts', 'go', 'py', 'sh', 'css', 'html'].includes(e)) return 'text'
  return 'none'
}

// The browser's own type for a kind it plays (the server sends files as
// bytes to download; a <video> wants to know what they are).
export function playType(name: string, type: string): string {
  if (type.startsWith('video/') || type.startsWith('audio/') || type.startsWith('image/')) return type
  const e = ext(name)
  return ({ mp4: 'video/mp4', m4v: 'video/mp4', mov: 'video/quicktime', webm: 'video/webm', mp3: 'audio/mpeg', m4a: 'audio/mp4', wav: 'audio/wav', ogg: 'audio/ogg', flac: 'audio/flac', aac: 'audio/aac' } as Record<string, string>)[e] ?? type
}

export async function openPdf(bytes: Uint8Array): Promise<PDFDocumentProxy> {
  const pdfjs = await import('pdfjs-dist')
  pdfjs.GlobalWorkerOptions.workerSrc = (await import('pdfjs-dist/build/pdf.worker.min.mjs?url')).default
  return pdfjs.getDocument({ data: bytes }).promise
}

// pdfText is what a PDF says (its text layer), at most max characters.
export async function pdfText(bytes: Uint8Array, max = 200_000): Promise<string> {
  const doc = await openPdf(bytes)
  let out = ''
  for (let i = 1; i <= doc.numPages && out.length < max; i++) {
    const page = await doc.getPage(i)
    const tc = await page.getTextContent()
    out += tc.items.map((x) => ('str' in x ? x.str : '')).join(' ') + '\n'
  }
  await doc.loadingTask.destroy()
  return out.slice(0, max)
}

// sendPdfText reads an uploaded PDF's text here and gives it to the server,
// so search finds what it says (the server reads other files itself).
export async function sendPdfText(file: File, uploadId: string): Promise<void> {
  if (file.type !== 'application/pdf' && ext(file.name) !== 'pdf') return
  try {
    const text = await pdfText(new Uint8Array(await file.arrayBuffer()))
    if (text.trim()) await filesApi.setText(uploadId, text)
  } catch {
    /* a PDF without text, or one pdf.js can't read: found by its name */
  }
}

// The file viewer, over whatever is open (App mounts it): openFile shows one.
export const fileViewer = reactive<{ open: boolean; id: string }>({ open: false, id: '' })
export function openFile(id: string) {
  Object.assign(fileViewer, { open: true, id })
}
export function closeFile() {
  fileViewer.open = false
}
