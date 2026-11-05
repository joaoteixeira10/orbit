<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted, onBeforeUnmount, getCurrentInstance, h, render } from 'vue'
import PageEmbed from '@/components/embeds/PageEmbed.vue'
import SubpageList from '@/components/pages/SubpageList.vue'
import { embedOf } from '@/lib/embeds'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { renderMarkdown } from '@/lib/markdown'
import { mediaPrefs, remoteHost, reveal, safeImageSrc, showsAtOnce } from '@/lib/media'
import { downloadUpload, uploadBlobUrl, uploadIdOf } from '@/lib/uploads'
import { openFile } from '@/lib/fileView'
import { evaluate } from '@/lib/mathnotes'
import { guestApi } from '@/lib/shareApi'
import { markTerms, type Term } from '@/lib/glossary'

// A page as it reads: its Markdown rendered like a README on GitHub, with
// the app's type. Task boxes can be ticked without editing (when allowed);
// a double click asks to edit. Code blocks get a copy button, as on GitHub.
// guest: a guest link's secret — pictures, files and what's shown in place
// load through it, and a link to a page asks the guest view (page) to go.
// terms: the workspace's glossary, marked where used (pointing explains them).
// plain: links stay links (no sheet or diagram shown in place, no page tree) — the assistant's answers.
const props = defineProps<{ content: string; editable: boolean; math?: boolean; guest?: string; terms?: Term[]; breaks?: boolean; plain?: boolean }>()
const emit = defineEmits<{ toggle: [index: number]; edit: []; page: [id: string] }>()
const { t } = useI18n()
const router = useRouter()

const html = computed(() => renderMarkdown(props.content, props.breaks))
const root = ref<HTMLElement | null>(null)

// Lucide's copy and check icons, drawn into each button (the HTML is rendered
// outside Vue, so the buttons are added to the DOM after each render).
const svg = (d: string) =>
  `<svg xmlns="http://www.w3.org/2000/svg" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${d}</svg>`
const COPY = svg('<rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>')
const CHECK = svg('<path d="M20 6 9 17l-5-5"/>')

function addCopyButtons() {
  for (const pre of root.value?.querySelectorAll('pre') ?? []) {
    if (pre.querySelector(':scope > .code-copy')) continue
    const b = document.createElement('button')
    b.type = 'button'
    b.className = 'code-copy'
    b.title = t('editor.copyCode')
    b.setAttribute('aria-label', t('editor.copyCode'))
    b.innerHTML = COPY
    pre.appendChild(b)
  }
}
// Images from other sites (lib/media.ts): shown at once when the reader
// allows it or already showed one from that site, else on a click.
function showImage(ph: HTMLElement) {
  const src = safeImageSrc(ph.dataset.src ?? '')
  if (!src) return
  const img = document.createElement('img')
  img.src = src
  img.alt = ph.dataset.alt ?? ''
  if (ph.title) img.title = ph.title
  img.loading = 'lazy'
  img.referrerPolicy = 'no-referrer'
  ph.replaceWith(img)
}
function showImages() {
  for (const ph of root.value?.querySelectorAll<HTMLElement>('.md-remote-img') ?? []) {
    if (showsAtOnce(ph.dataset.src ?? '')) showImage(ph)
  }
}
function revealFrom(ph: HTMLElement) {
  reveal(ph.dataset.src ?? '')
  const host = remoteHost(ph.dataset.src ?? '')
  for (const other of root.value?.querySelectorAll<HTMLElement>('.md-remote-img') ?? []) {
    if (remoteHost(other.dataset.src ?? '') === host) showImage(other)
  }
}
// Uploaded images: fetched with the sign-in token, shown from a blob: URL.
function loadUploads() {
  for (const img of root.value?.querySelectorAll<HTMLImageElement>('img[data-upload]') ?? []) {
    const src = img.dataset.upload ?? ''
    img.removeAttribute('data-upload')
    if (props.guest) {
      img.src = guestApi.uploadUrl(props.guest, src)
      continue
    }
    uploadBlobUrl(src).then((url) => (img.src = url)).catch(() => img.classList.add('broken'))
  }
}
// Sums (lib/mathnotes): each block whose text ends with "=" gets its answer
// after it, as in Quick Notes. A list item's own text only (not its
// sub-list); code is left alone.
function addAnswers() {
  const el = root.value
  if (!el) return
  el.querySelectorAll('.md-calc').forEach((n) => n.remove())
  if (!props.math) return
  const blocks = [...el.querySelectorAll<HTMLElement>('p, li, h1, h2, h3, h4, td, th')].filter(
    (b) => !b.closest('pre') && !(b.tagName === 'LI' && b.querySelector(':scope > p')),
  )
  const own = (b: HTMLElement) => {
    if (b.tagName !== 'LI') return b.textContent ?? ''
    const c = b.cloneNode(true) as HTMLElement
    c.querySelectorAll('ul, ol').forEach((n) => n.remove())
    return c.textContent ?? ''
  }
  const texts = blocks.map((b) => own(b).replace(/\n/g, ' '))
  for (const a of evaluate(texts.join('\n'))) {
    const b = blocks[a.line]
    const s = document.createElement('span')
    s.className = 'md-calc'
    s.textContent = (/\s$/.test(texts[a.line]) ? '' : ' ') + a.result
    const sub = b.tagName === 'LI' ? b.querySelector(':scope > ul, :scope > ol') : null
    if (sub) b.insertBefore(s, sub)
    else b.appendChild(s)
  }
}

// A link to a sheet, a diagram or a whiteboard alone on its line shows it
// (PageEmbed, mounted into the rendered HTML with the app's plugins).
const app = getCurrentInstance()?.appContext ?? null
const embeds: HTMLElement[] = []
function addSheets() {
  for (const el of embeds.splice(0)) render(null, el)
  if (props.plain) return
  for (const a of root.value?.querySelectorAll<HTMLAnchorElement>('a.md-sheet, a.md-diagram, a.md-whiteboard') ?? []) {
    const p = a.parentElement
    if (!p || p.tagName !== 'P' || p.textContent?.trim() !== a.textContent?.trim()) continue
    const ref = embedOf(a.getAttribute('href') ?? '')
    if (!ref) continue
    const box = document.createElement('div')
    p.replaceWith(box)
    const vnode = h(PageEmbed, { ...ref, guest: props.guest })
    vnode.appContext = app
    render(vnode, box)
    embeds.push(box)
  }
  // ::pagetree alone in a paragraph: the page's sub-pages, live (not for guests).
  for (const p of root.value?.querySelectorAll<HTMLElement>('p') ?? []) {
    if (p.textContent?.trim() !== '::pagetree' || props.guest) continue
    const box = document.createElement('div')
    p.replaceWith(box)
    const vnode = h(SubpageList)
    vnode.appContext = app
    render(vnode, box)
    embeds.push(box)
  }
}
onBeforeUnmount(() => {
  for (const el of embeds.splice(0)) render(null, el)
})

// ---- Glossary terms: marked, and a card explaining the one pointed at ----
let card: HTMLElement | null = null
function hideCard() {
  card?.remove()
  card = null
}
function showCard(span: HTMLElement) {
  const term = props.terms?.[Number(span.dataset.term)]
  if (!term) return
  hideCard()
  card = document.createElement('div')
  card.className = 'md-term-card'
  card.setAttribute('role', 'tooltip')
  const b = document.createElement('div')
  b.className = 'md-term-name'
  b.textContent = term.term
  const d = document.createElement('div')
  d.textContent = term.definition
  const from = document.createElement('div')
  from.className = 'md-term-from'
  from.textContent = term.pageTitle
  card.append(b, d, from)
  document.body.append(card)
  const r = span.getBoundingClientRect()
  card.style.left = `${Math.max(8, Math.min(r.left, window.innerWidth - 300))}px`
  card.style.top = `${r.bottom + 6}px`
}
function onOver(e: Event) {
  const span = (e.target as HTMLElement).closest?.('.md-term') as HTMLElement | null
  if (span) showCard(span)
  else hideCard()
}
onBeforeUnmount(hideCard)
// Terms that come later are marked in what's shown (nothing else redrawn).
function addTerms() {
  if (props.terms?.length && root.value && !root.value.querySelector('.md-term')) markTerms(root.value, props.terms)
}
watch(() => props.terms, () => nextTick(addTerms))

function afterRender() {
  addTerms()
  addSheets()
  addCopyButtons()
  showImages()
  loadUploads()
  addAnswers()
}
onMounted(afterRender)
watch(html, () => nextTick(afterRender))
watch(() => mediaPrefs.remoteImages, () => nextTick(showImages))

async function copyCode(b: HTMLElement) {
  const code = b.closest('pre')?.querySelector('code') ?? b.closest('pre')
  try {
    await navigator.clipboard.writeText(code?.textContent ?? '')
  } catch {
    return // clipboard blocked (insecure context / permission)
  }
  b.innerHTML = CHECK
  b.classList.add('done')
  b.title = t('editor.codeCopied')
  setTimeout(() => {
    b.innerHTML = COPY
    b.classList.remove('done')
    b.title = t('editor.copyCode')
  }, 1500)
}

function onClick(e: MouseEvent) {
  const target = e.target as HTMLElement
  const ph = target.closest('.md-remote-img') as HTMLElement | null
  if (ph) {
    revealFrom(ph)
    return
  }
  const file = target.closest('a[data-upload]') as HTMLAnchorElement | null
  if (file) {
    e.preventDefault()
    if (props.guest) window.open(guestApi.uploadUrl(props.guest, file.dataset.upload ?? ''), '_blank', 'noopener')
    else if (uploadIdOf(file.dataset.upload ?? '')) openFile(uploadIdOf(file.dataset.upload ?? ''))
    else void downloadUpload(file.dataset.upload ?? '', file.dataset.name || 'file')
    return
  }
  // A link within Orbit (a page, a date) opens here, not in a new tab.
  const inner = target.closest('a.md-internal') as HTMLAnchorElement | null
  if (inner && props.guest) {
    e.preventDefault()
    const m = /^\/w\/[^/]+\/p\/([^/?#]+)/.exec(inner.getAttribute('href') ?? '')
    if (m) emit('page', m[1])
    return
  }
  if (inner && !e.metaKey && !e.ctrlKey && !e.shiftKey) {
    e.preventDefault()
    void router.push(inner.getAttribute('href') ?? '/')
    return
  }
  const copy = target.closest('.code-copy') as HTMLElement | null
  if (copy) {
    e.preventDefault()
    void copyCode(copy)
    return
  }
  const box = target.closest('input[data-task]') as HTMLInputElement | null
  if (!box) return
  if (!props.editable) {
    e.preventDefault()
    return
  }
  emit('toggle', Number(box.dataset.task))
}
function onDblClick(e: MouseEvent) {
  if (props.editable && !(e.target as HTMLElement).closest('a, input, pre, .page-embed')) emit('edit')
}
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -- sanitised by renderMarkdown -->
  <article
    ref="root"
    class="md-view"
    :class="{ editable }"
    @click="onClick"
    @mouseover="onOver"
    @focusin="onOver"
    @mouseleave="hideCard"
    @dblclick="onDblClick"
    @keydown.enter.space="(e: KeyboardEvent) => { const ph = (e.target as HTMLElement).closest('.md-remote-img') as HTMLElement | null; if (ph) { e.preventDefault(); revealFrom(ph) } }"
    v-html="html"
  />
</template>

<style scoped>
.md-view {
  font-size: 16px;
  line-height: 1.65;
  color: var(--text);
  overflow-wrap: anywhere;
  padding-bottom: 24px;
}
.md-view :deep(> :first-child) {
  margin-top: 0;
}
.md-view :deep(h1),
.md-view :deep(h2),
.md-view :deep(h3),
.md-view :deep(h4) {
  margin: 1.6em 0 0.5em;
  font-family: var(--font-display);
  font-weight: 700;
  letter-spacing: -0.018em;
  line-height: 1.25;
}
.md-view :deep(h1) {
  font-size: 1.75em;
  padding-bottom: 0.3em;
  border-bottom: 0.5px solid var(--border);
}
.md-view :deep(h2) {
  font-size: 1.4em;
  padding-bottom: 0.25em;
  border-bottom: 0.5px solid var(--border);
}
.md-view :deep(h3) {
  font-size: 1.15em;
}
.md-view :deep(p),
.md-view :deep(ul),
.md-view :deep(ol),
.md-view :deep(blockquote),
.md-view :deep(pre),
.md-view :deep(table),
.md-view :deep(.md-alert) {
  margin: 0 0 1em;
}
.md-view :deep(ul),
.md-view :deep(ol) {
  padding-left: 1.6em;
}
.md-view :deep(ul) {
  list-style: disc;
}
.md-view :deep(ol) {
  list-style: decimal;
}
.md-view :deep(li + li) {
  margin-top: 0.2em;
}
.md-view :deep(li > ul),
.md-view :deep(li > ol) {
  margin: 0.2em 0 0;
}
/* Task lists: the app's round checkboxes. */
.md-view :deep(li:has(> input[type='checkbox'])),
.md-view :deep(li:has(> p > input[type='checkbox'])) {
  list-style: none;
  margin-left: -1.4em;
}
.md-view :deep(input[type='checkbox']) {
  appearance: none;
  width: 17px;
  height: 17px;
  margin: 0 8px 0 0;
  vertical-align: -3px;
  border-radius: 999px;
  border: 1.5px solid var(--text-faint);
  cursor: default;
  transition: background 0.15s, border-color 0.15s;
}
.md-view.editable :deep(input[type='checkbox']) {
  cursor: pointer;
}
.md-view :deep(input[type='checkbox']:checked) {
  border-color: var(--ws, var(--accent));
  background: var(--ws, var(--accent)) url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='white' stroke-width='3.5' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='M20 6 9 17l-5-5'/%3E%3C/svg%3E") center / 11px no-repeat;
}
.md-view :deep(li:has(> input:checked)),
.md-view :deep(li:has(> p > input:checked)) {
  color: var(--text-3);
  text-decoration: line-through;
}
.md-view :deep(a) {
  color: var(--accent-text);
  text-decoration: underline;
  text-decoration-color: color-mix(in srgb, var(--accent-text) 35%, transparent);
  text-underline-offset: 2px;
}
.md-view :deep(blockquote) {
  padding: 0 1em;
  border-left: 3px solid var(--border-2);
  color: var(--text-2);
}
.md-view :deep(code) {
  padding: 0.15em 0.4em;
  border-radius: 6px;
  background: var(--fill-2);
  font: 0.86em var(--font-mono);
}
.md-view :deep(pre) {
  padding: 14px 16px;
  border-radius: 12px;
  background: var(--fill-2);
  overflow-x: auto;
  line-height: 1.5;
}
.md-view :deep(pre) {
  position: relative;
}
/* The copy button: top right of a code block, shown on hover (always on touch). */
.md-view :deep(.code-copy) {
  position: absolute;
  top: 8px;
  right: 8px;
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 7px;
  color: var(--text-2);
  background: var(--surface);
  box-shadow: 0 0 0 0.5px var(--border-2), var(--shadow-sm);
  opacity: 0;
  cursor: pointer;
  transition: opacity var(--speed) var(--ease), color var(--speed);
}
/* The language, left of the copy button, also on hover. */
.md-view :deep(pre[data-lang])::before {
  content: attr(data-lang);
  position: absolute;
  top: 14px;
  right: 46px;
  font: 500 11px var(--font-sans);
  color: var(--text-3);
  opacity: 0;
  transition: opacity var(--speed) var(--ease);
  pointer-events: none;
}
.md-view :deep(pre[data-lang]:hover)::before {
  opacity: 1;
}
.md-view :deep(pre:hover .code-copy),
.md-view :deep(.code-copy:focus-visible),
.md-view :deep(.code-copy.done) {
  opacity: 1;
}
.md-view :deep(.code-copy:hover) {
  color: var(--text);
}
.md-view :deep(.code-copy.done) {
  color: var(--ok-text);
}
@media (hover: none) {
  .md-view :deep(.code-copy) {
    opacity: 0.8;
  }
}
.md-view :deep(pre code) {
  padding: 0;
  background: none;
  font-size: 13.5px;
}
.md-view :deep(hr) {
  height: 0;
  margin: 2em 0;
  border: 0;
  border-top: 0.5px solid var(--border-2);
}
.md-view :deep(table) {
  display: block;
  max-width: 100%;
  overflow-x: auto;
  border-collapse: collapse;
  font-size: 14.5px;
}
.md-view :deep(th),
.md-view :deep(td) {
  padding: 7px 13px;
  border: 0.5px solid var(--border-2);
}
.md-view :deep(th) {
  font-weight: 600;
  background: var(--fill-2);
}
.md-view :deep(tr:nth-child(2n) td) {
  background: color-mix(in srgb, var(--fill-2) 50%, transparent);
}
.md-view :deep(.md-remote-img) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  border-radius: 10px;
  border: 1px dashed var(--border-2);
  background: var(--fill-2);
  color: var(--text-2);
  font-size: 13px;
  cursor: pointer;
}
.md-view :deep(.md-remote-img::before) {
  content: '';
  width: 16px;
  height: 16px;
  background: currentColor;
  -webkit-mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Crect width='18' height='18' x='3' y='3' rx='2'/%3E%3Ccircle cx='9' cy='9' r='2'/%3E%3Cpath d='m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21'/%3E%3C/svg%3E") center / contain no-repeat;
  mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='black' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Crect width='18' height='18' x='3' y='3' rx='2'/%3E%3Ccircle cx='9' cy='9' r='2'/%3E%3Cpath d='m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21'/%3E%3C/svg%3E") center / contain no-repeat;
}
.md-view :deep(.md-remote-img:hover) {
  color: var(--text);
  border-color: var(--accent);
}
.md-view :deep(img.broken) {
  min-width: 120px;
  min-height: 60px;
  background: var(--fill-2);
}
.md-view :deep(a.md-file)::before {
  content: '📎 ';
}
/* @-mentions: a person, a date (opens the calendar), a page. */
.md-view :deep(.md-mention),
.md-view :deep(a.md-date) {
  padding: 0 0.3em;
  border-radius: 5px;
  background: var(--accent-bg);
  color: var(--accent-text);
  font-weight: 500;
  text-decoration: none;
  white-space: nowrap;
}
.md-view :deep(a.md-date)::before {
  content: '@';
  opacity: 0.6;
}
.md-view :deep(a.md-page) {
  text-decoration-style: dotted;
}
.md-view :deep(img) {
  max-width: 100%;
  border-radius: 10px;
}
.md-view :deep(mark) {
  padding: 0 0.15em;
  border-radius: 3px;
  background: color-mix(in srgb, #ffcc00 45%, transparent);
  color: inherit;
}
.md-view :deep(kbd) {
  padding: 0.1em 0.45em;
  border-radius: 5px;
  border: 0.5px solid var(--border-2);
  box-shadow: inset 0 -1px 0 var(--border-2);
  font: 0.85em var(--font-mono);
}
/* GitHub alerts, in the app's colours. */
.md-view :deep(.md-alert) {
  padding: 10px 16px;
  border-left: 3px solid var(--c);
  border-radius: 4px 12px 12px 4px;
  background: color-mix(in srgb, var(--c) 9%, transparent);
}
.md-view :deep(.md-alert > :last-child) {
  margin-bottom: 0;
}
.md-view :deep(.md-alert-title) {
  margin-bottom: 4px;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.02em;
  color: var(--c);
}
.md-view :deep(.md-alert-note) {
  --c: #007aff;
}
.md-view :deep(.md-alert-tip) {
  --c: #34c759;
}
.md-view :deep(.md-alert-important) {
  --c: #af52de;
}
.md-view :deep(.md-alert-warning) {
  --c: #ff9500;
}
.md-view :deep(.md-alert-caution) {
  --c: #ff3b30;
}
.md-view :deep(.footnotes) {
  margin-top: 2em;
  padding-top: 0.8em;
  border-top: 0.5px solid var(--border);
  font-size: 13.5px;
  color: var(--text-2);
}
.md-view :deep(sup a) {
  text-decoration: none;
}
</style>
