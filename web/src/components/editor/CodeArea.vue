<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref, watch, markRaw } from 'vue'
import { useI18n } from 'vue-i18n'
import { Annotation, Compartment, EditorSelection, EditorState, Prec, RangeSetBuilder, StateField, type Extension } from '@codemirror/state'
import { Decoration, EditorView, ViewPlugin, WidgetType, drawSelection, dropCursor, highlightSpecialChars, keymap, placeholder as cmPlaceholder, rectangularSelection, type DecorationSet } from '@codemirror/view'
import { history } from '@codemirror/commands'
import { highlightSelectionMatches, search } from '@codemirror/search'
import { HighlightStyle, indentUnit, syntaxHighlighting } from '@codemirror/language'
import { markdown as markdownLang, markdownLanguage } from '@codemirror/lang-markdown'
import { tags } from '@lezer/highlight'
import { AtSign, CalendarDays, FileText } from 'lucide-vue-next'
import { evaluate, type MathLine } from '@/lib/mathnotes'
import { noteTokens } from '@/lib/noteSyntax'
import { markdownOf, type MentionItem } from '@/lib/mentions'
import { currentLocale } from '@/i18n'
import SlashMenu from './SlashMenu.vue'
import type { SlashItem, SlashState } from './slash'
import { addsCursor, boxSelects, copyCutLines, vsKeymap } from './vscode'

// A text field that edits like Visual Studio Code (CodeMirror underneath):
// several cursors (Alt-click, Mod-D, Mod-Alt-↑↓), box selections
// (Shift-Alt-drag), moving and copying lines, find and replace — see
// vscode.ts for every key. In Markdown it colours the source and continues
// lists on Enter. Optional: sums (a line ending in "=" shows its answer;
// Tab writes it in), files (pasted or dropped: upload gives the Markdown to
// put in), @-mentions (mentions gives what "@" offers), submit (Mod-Enter),
// syntax (colour the text as a language, as a note's: lib/noteSyntax).
const props = withDefaults(
  defineProps<{
    modelValue: string
    markdown?: boolean
    mono?: boolean
    syntax?: string
    placeholder?: string
    readonly?: boolean
    label?: string
    sums?: boolean
    maxlength?: number
    upload?: ((f: File) => Promise<string>) | null
    mentions?: ((q: string) => MentionItem[]) | null
    submit?: boolean // Mod-Enter emits submit (instead of a new line)
    testid?: string // on the editable element (for tests)
  }>(),
  { markdown: true, mono: false, placeholder: '', readonly: false, label: '', sums: false, maxlength: 0, upload: null, mentions: null, submit: false, testid: '' },
)
const emit = defineEmits<{ 'update:modelValue': [value: string]; change: []; focus: []; blur: []; submit: [] }>()
const { t } = useI18n()

const host = ref<HTMLElement | null>(null)
let view: EditorView | null = null
const readonlyCo = new Compartment()
// A value set from outside: not news to tell back.
const outside = Annotation.define<boolean>()

// ---- Sums: the answer after each "=", in grey ----
class Answer extends WidgetType {
  constructor(readonly text: string) {
    super()
  }
  eq(o: Answer) {
    return o.text === this.text
  }
  toDOM() {
    const s = document.createElement('span')
    s.className = 'cm-answer'
    s.textContent = this.text
    return s
  }
}
function answersOf(state: EditorState): MathLine[] {
  return evaluate(state.doc.toString(), { markdown: props.markdown })
}
const sumsField = StateField.define<{ answers: MathLine[]; deco: DecorationSet }>({
  create: (s) => build(s),
  update: (v, tr) => (tr.docChanged ? build(tr.state) : v),
  provide: (f) => EditorView.decorations.from(f, (v) => v.deco),
})
function build(state: EditorState) {
  const answers = answersOf(state)
  const deco = Decoration.set(
    answers
      .filter((a) => a.line < state.doc.lines)
      .map((a) => {
        const line = state.doc.line(a.line + 1)
        return Decoration.widget({ widget: new Answer((/\s$/.test(line.text) ? '' : ' ') + a.result), side: 1 }).range(line.to)
      }),
  )
  return { answers, deco }
}
// Tab on a line with an answer writes it in.
function takeAnswer(v: EditorView): boolean {
  const s = v.state
  if (!props.sums || s.selection.ranges.length > 1 || !s.selection.main.empty) return false
  const line = s.doc.lineAt(s.selection.main.head)
  const a = s.field(sumsField).answers.find((x) => x.line === line.number - 1)
  if (!a) return false
  const text = line.text.replace(/\s+$/, '') + ' ' + a.result
  v.dispatch({ changes: { from: line.from, to: line.to, insert: text }, selection: { anchor: line.from + text.length }, userEvent: 'input' })
  return true
}

// ---- @-mentions: the word at the cursor, when it starts with "@" ----
const mention = reactive<SlashState>({ open: false, items: [], index: 0, rect: null, pick: () => {} })
const ICONS = { person: markRaw(AtSign), page: markRaw(FileText), date: markRaw(CalendarDays) }
let mentionAt = -1
function closeMention() {
  mention.open = false
  mention.items = []
  mentionAt = -1
}
function findMention(v: EditorView) {
  const s = v.state
  if (!props.mentions || s.selection.ranges.length > 1 || !s.selection.main.empty || !v.hasFocus) return closeMention()
  const head = s.selection.main.head
  const line = s.doc.lineAt(head)
  const m = /(?:^|[\s([“"])@([^\s@[\]()]{0,40})$/.exec(line.text.slice(0, head - line.from))
  if (!m) return closeMention()
  mentionAt = head - m[1].length - 1
  mention.items = props.mentions(m[1]).map((it): SlashItem => ({ key: it.key, title: it.title, desc: it.desc, icon: ICONS[it.kind], aliases: [], run: () => pickMention(it) }))
  mention.index = 0
  const at = mentionAt
  mention.rect = () => {
    const c = view?.coordsAtPos(at)
    return c ? new DOMRect(c.left, c.top, 1, c.bottom - c.top) : null
  }
  mention.pick = (it) => it.run(null as any, null as any)
  mention.open = true
}
function pickMention(it: MentionItem) {
  if (!view || mentionAt < 0) return
  const s = markdownOf(it) + ' '
  const from = mentionAt
  const to = view.state.selection.main.head
  closeMention()
  view.dispatch({ changes: { from, to, insert: s }, selection: { anchor: from + s.length }, userEvent: 'input' })
  view.focus()
}
const mentionKeys = Prec.highest(
  keymap.of([
    { key: 'ArrowDown', run: () => (mention.open && mention.items.length ? ((mention.index = (mention.index + 1) % mention.items.length), true) : false) },
    { key: 'ArrowUp', run: () => (mention.open && mention.items.length ? ((mention.index = (mention.index - 1 + mention.items.length) % mention.items.length), true) : false) },
    { key: 'Enter', run: () => (mention.open && mention.items.length ? (mention.pick(mention.items[mention.index]), true) : false) },
    { key: 'Tab', run: () => (mention.open && mention.items.length ? (mention.pick(mention.items[mention.index]), true) : false) },
    { key: 'Escape', run: () => (mention.open ? (closeMention(), true) : false) },
  ]),
)

// ---- Files pasted or dropped ----
async function addFiles(v: EditorView, files: File[], at: number) {
  if (!props.upload) return
  let pos = at
  for (const f of files) {
    try {
      const md = await props.upload(f)
      const before = v.state.sliceDoc(Math.max(0, pos - 1), pos)
      const s = `${before && before !== '\n' ? '\n' : ''}${md}\n`
      v.dispatch({ changes: { from: pos, insert: s }, selection: { anchor: pos + s.length }, userEvent: 'input' })
      pos += s.length
    } catch {
      /* the upload says why (a toast) */
    }
  }
}
const fileHandlers = EditorView.domEventHandlers({
  paste(e, v) {
    const files = [...(e.clipboardData?.files ?? [])]
    if (!files.length || !props.upload) return false
    e.preventDefault()
    void addFiles(v, files, v.state.selection.main.head)
    return true
  },
  drop(e, v) {
    const files = [...(e.dataTransfer?.files ?? [])]
    if (!files.length || !props.upload) return false
    e.preventDefault()
    void addFiles(v, files, v.posAtCoords({ x: e.clientX, y: e.clientY }) ?? v.state.doc.length)
    return true
  },
})

// ---- Looks ----
const mdColours = HighlightStyle.define([
  { tag: tags.heading, fontWeight: '700', color: 'var(--text)' },
  { tag: tags.strong, fontWeight: '700' },
  { tag: tags.emphasis, fontStyle: 'italic' },
  { tag: tags.strikethrough, textDecoration: 'line-through' },
  { tag: tags.link, color: 'var(--accent-text)' },
  { tag: tags.url, color: 'var(--text-3)' },
  { tag: tags.monospace, fontFamily: 'var(--font-mono)', backgroundColor: 'var(--fill-2)', borderRadius: '3px' },
  { tag: tags.quote, color: 'var(--text-2)' },
  { tag: [tags.processingInstruction, tags.contentSeparator, tags.labelName], color: 'var(--text-3)' },
])
const theme = EditorView.theme({
  '&': { backgroundColor: 'transparent', color: 'inherit', fontSize: 'inherit' },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': { fontFamily: 'inherit', lineHeight: 'inherit', overflow: 'visible' },
  '.cm-content': { padding: '0', caretColor: 'var(--accent)' },
  '.cm-line': { padding: '0' },
  '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--accent)', borderLeftWidth: '1.5px' },
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, ::selection': {
    backgroundColor: 'color-mix(in srgb, var(--accent) 22%, transparent) !important',
  },
  '.cm-selectionMatch': { backgroundColor: 'color-mix(in srgb, var(--accent) 10%, transparent)' },
  '.cm-placeholder': { color: 'var(--text-faint)' },
  '.cm-answer': { color: 'var(--accent-text)', opacity: '0.75', fontWeight: '500' },
  '.cm-panels': { backgroundColor: 'var(--surface)', color: 'var(--text)', borderRadius: '10px', zIndex: '5' },
  '.cm-panels-top': { borderBottom: '0.5px solid var(--border)', position: 'sticky', top: '0' },
  '.cm-panel.cm-search, .cm-panel.cm-gotoLine': { padding: '6px 8px', fontSize: '13px' },
  '.cm-panel input[type=text], .cm-textfield': {
    backgroundColor: 'var(--fill-2)', border: '0', borderRadius: '6px', padding: '3px 6px', color: 'var(--text)', fontSize: '13px',
  },
  '.cm-button': { backgroundImage: 'none', backgroundColor: 'var(--fill-2)', border: '0', borderRadius: '6px', padding: '3px 8px', color: 'var(--text)', fontSize: '12.5px' },
  '.cm-panel label': { fontSize: '12.5px', color: 'var(--text-2)' },
  '.cm-searchMatch': { backgroundColor: 'color-mix(in srgb, #ffcc00 40%, transparent)' },
  '.cm-searchMatch-selected': { backgroundColor: 'color-mix(in srgb, #ff9500 50%, transparent)' },
})

// CodeMirror's own words (the find panel), in Portuguese.
const PT_PHRASES: Record<string, string> = {
  Find: 'Procurar', Replace: 'Substituir', next: 'seguinte', previous: 'anterior', all: 'todas', 'match case': 'maiúsculas',
  'by word': 'palavra inteira', regexp: 'expressão regular', replace: 'substituir', 'replace all': 'substituir todas', close: 'fechar',
  'current match': 'ocorrência atual', 'on line': 'na linha', 'replaced match on line $': 'substituída na linha $',
  'replaced $ matches': '$ substituídas', 'Go to line': 'Ir para a linha', go: 'ir', 'Control character': 'Carácter de controlo',
}

// ---- Colours by language (a note's): the same stretches as its card ----
const syntaxCo = new Compartment()
const marks = new Map<string, Decoration>()
const markOf = (cls: string) => marks.get(cls) ?? marks.set(cls, Decoration.mark({ class: cls })).get(cls)!
function syntaxColours(lang: string | undefined): Extension {
  if (!lang || lang === 'text') return []
  const build = (v: EditorView): DecorationSet => {
    const text = v.state.doc.toString()
    if (text.length > 60000) return Decoration.none
    const b = new RangeSetBuilder<Decoration>()
    for (const t of noteTokens(text, lang)) if (t.to > t.from) b.add(t.from, t.to, markOf(t.cls))
    return b.finish()
  }
  return ViewPlugin.fromClass(
    class {
      decorations: DecorationSet
      constructor(v: EditorView) {
        this.decorations = build(v)
      }
      update(u: { docChanged: boolean; view: EditorView }) {
        if (u.docChanged) this.decorations = build(u.view)
      }
    },
    { decorations: (p) => p.decorations },
  )
}

function extensions(): Extension[] {
  const ext: Extension[] = [
    history(),
    drawSelection(),
    dropCursor(),
    highlightSpecialChars(),
    EditorState.allowMultipleSelections.of(true),
    EditorView.clickAddsSelectionRange.of(addsCursor),
    rectangularSelection({ eventFilter: boxSelects }),
    highlightSelectionMatches({ minSelectionLength: 2 }),
    search({ top: true }),
    indentUnit.of('  '),
    EditorView.lineWrapping,
    mentionKeys,
    Prec.high(keymap.of([{ key: 'Tab', run: takeAnswer }])),
    keymap.of(vsKeymap({ submit: props.submit ? () => emit('submit') : undefined })),
    copyCutLines,
    fileHandlers,
    theme,
    readonlyCo.of(EditorState.readOnly.of(props.readonly)),
    syntaxCo.of(syntaxColours(props.syntax)),
    EditorView.contentAttributes.of({ 'aria-label': props.label || t('editor.mdLabel'), spellcheck: 'true', autocorrect: 'on', ...(props.testid ? { 'data-testid': props.testid } : {}) }),
    EditorView.updateListener.of((u) => {
      if (u.docChanged && !u.transactions.some((tr) => tr.annotation(outside))) {
        emit('update:modelValue', u.state.doc.toString())
        emit('change')
      }
      if (u.focusChanged) {
        if (u.view.hasFocus) emit('focus')
        else emit('blur')
      }
      if (u.docChanged || u.selectionSet || u.focusChanged) findMention(u.view)
    }),
  ]
  if (props.placeholder) ext.push(cmPlaceholder(props.placeholder))
  if (props.markdown) ext.push(markdownLang({ base: markdownLanguage, addKeymap: true }), syntaxHighlighting(mdColours))
  if (props.sums) ext.push(sumsField)
  if (props.maxlength) ext.push(EditorState.changeFilter.of((tr) => tr.newDoc.length <= props.maxlength))
  if (currentLocale() === 'pt') ext.push(EditorState.phrases.of(PT_PHRASES))
  return ext
}

onMounted(() => {
  view = new EditorView({ parent: host.value!, state: EditorState.create({ doc: props.modelValue, extensions: extensions() }) })
})
onBeforeUnmount(() => {
  view?.destroy()
  view = null
})

// A new value from outside (someone else's save): replace, keeping the
// cursor where it can be.
watch(
  () => props.modelValue,
  (v) => {
    if (!view || v === view.state.doc.toString()) return
    const sel = view.state.selection.main
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: v },
      selection: EditorSelection.single(Math.min(sel.anchor, v.length), Math.min(sel.head, v.length)),
      annotations: [outside.of(true)],
    })
  },
)
watch(
  () => props.syntax,
  (lang) => view?.dispatch({ effects: syntaxCo.reconfigure(syntaxColours(lang)) }),
)
watch(
  () => props.readonly,
  (ro) => view?.dispatch({ effects: readonlyCo.reconfigure(EditorState.readOnly.of(ro)) }),
)

function focus(at: 'start' | 'end' | 'keep' = 'keep') {
  if (!view) return
  if (at !== 'keep') {
    const pos = at === 'start' ? 0 : view.state.doc.length
    view.dispatch({ selection: { anchor: pos } })
  }
  view.focus()
}
defineExpose({ focus, view: () => view })
</script>

<template>
  <div ref="host" class="code-area" :class="{ mono }" />
  <SlashMenu v-if="mentions" :state="mention" :context="host" :label="t('mention.label')" :empty="t('mention.none')" @hover="(i) => (mention.index = i)" />
</template>

<style scoped>
.code-area {
  width: 100%;
  min-width: 0;
}
.code-area.mono {
  font-family: var(--font-mono);
}
/* The field itself shows no focus ring (a box around it may). */
.code-area :deep(.cm-content:focus-visible),
.code-area :deep(.cm-editor.cm-focused) {
  outline: none;
  box-shadow: none;
}
</style>
