<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { currentLocale } from '@/i18n'
import type { Snapshot } from './drawing'

// Excalidraw (a React component) in a Vue page: React, ReactDOM and
// Excalidraw are loaded only here, as their own chunk, the first time a
// drawing opens. Fonts come from this origin (see vite.config.ts).
//
// It reports each change of the drawing itself (not of zoom or scroll): the
// scene as JSON, an SVG preview and the words in it — the parent saves them.
const props = defineProps<{
  scene: string // Excalidraw JSON ("" = empty)
  readonly?: boolean
  name?: string
}>()
const emit = defineEmits<{
  change: [scene: string, version: number]
  ready: []
}>()


const host = ref<HTMLElement | null>(null)
const loading = ref(true)
const failed = ref('')
let root: any = null // React's Root
let api: any = null
let lib: any = null
let lastVersion = -1

const dark = () => document.documentElement.classList.contains('dark')

function parse(scene: string) {
  if (!scene) return { elements: [], appState: {}, files: {} }
  try {
    const d = JSON.parse(scene)
    return { elements: d.elements ?? [], appState: d.appState ?? {}, files: d.files ?? {} }
  } catch {
    return { elements: [], appState: {}, files: {} }
  }
}

function render() {
  if (!root || !lib) return
  const { createElement, Excalidraw } = lib
  const data = parse(props.scene)
  root.render(
    createElement(Excalidraw, {
      initialData: { ...data, appState: { ...data.appState, collaborators: new Map() }, scrollToContent: true },
      excalidrawAPI: (a: unknown) => {
        api = a
        emit('ready')
      },
      onChange: (elements: readonly unknown[]) => {
        const v = lib.getSceneVersion(elements)
        if (lastVersion < 0) {
          lastVersion = v // the scene as loaded
          return
        }
        if (v === lastVersion) return // zoom, scroll, selection
        lastVersion = v
        emit('change', '', v)
      },
      theme: dark() ? 'dark' : 'light',
      langCode: currentLocale() === 'pt' ? 'pt-PT' : 'en',
      viewModeEnabled: !!props.readonly,
      name: props.name || undefined,
      UIOptions: {
        canvasActions: { loadScene: false, saveToActiveFile: false, toggleTheme: null, export: { saveFileToDisk: true } },
      },
    }),
  )
}

onMounted(async () => {
  try {
    ;(window as any).EXCALIDRAW_ASSET_PATH = '/excalidraw-assets/'
    const [react, dom, ex] = await Promise.all([import('react'), import('react-dom/client'), import('@excalidraw/excalidraw')])
    await import('@excalidraw/excalidraw/index.css')
    lib = { createElement: react.createElement, ...ex }
    if (!host.value) return
    root = dom.createRoot(host.value)
    render()
  } catch (e: any) {
    failed.value = e?.message || String(e)
  } finally {
    loading.value = false
  }
})
onBeforeUnmount(() => {
  root?.unmount()
  root = null
  api = null
})
watch(
  () => props.readonly,
  (ro) => api?.updateScene({ appState: { viewModeEnabled: !!ro } }),
)

// snapshot is what to save now: the scene (images included), an SVG preview
// (fonts not inlined: it's a picture on a card) and the words in it.
async function snapshot(): Promise<Snapshot | null> {
  if (!api || !lib) return null
  const elements = api.getSceneElements()
  const appState = api.getAppState()
  const files = api.getFiles()
  const scene = lib.serializeAsJSON(elements, appState, files, 'database')
  let preview = ''
  if (elements.length) {
    try {
      const svg: SVGSVGElement = await lib.exportToSvg({
        elements,
        appState: { ...appState, exportBackground: true, exportWithDarkMode: false, viewBackgroundColor: '#ffffff' },
        files,
        exportPadding: 12,
        skipInliningFonts: true,
      })
      preview = svg.outerHTML
    } catch {
      /* no preview this time */
    }
  }
  const text = lib.getTextFromElements ? lib.getTextFromElements(elements) : ''
  return { scene, preview, text }
}

// replace shows a newer version saved elsewhere.
function replace(scene: string) {
  if (!api || !lib) return
  const d = parse(scene)
  lastVersion = -1
  if (d.files && Object.keys(d.files).length) api.addFiles(Object.values(d.files))
  // Not a step to undo here (someone else's drawing, or a saved one).
  api.updateScene({ elements: d.elements, ...(lib.CaptureUpdateAction ? { captureUpdate: lib.CaptureUpdateAction.NEVER } : {}) })
  lastVersion = lib.getSceneVersion(api.getSceneElements())
}
// sceneNow is the drawing as it is, at once (drawing together sends it).
function sceneNow(): string {
  if (!api || !lib) return props.scene
  return lib.serializeAsJSON(api.getSceneElements(), api.getAppState(), api.getFiles(), 'database')
}

defineExpose({ snapshot, replace, sceneNow })
</script>

<template>
  <div class="excalidraw-host">
    <div ref="host" class="absolute inset-0" />
    <div v-if="loading" class="absolute inset-0 grid place-items-center text-[13px] text-text-3">…</div>
    <div v-if="failed" class="absolute inset-0 grid place-items-center p-6 text-center text-[13px] text-crit-text">{{ failed }}</div>
  </div>
</template>

<style scoped>
.excalidraw-host {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 300px;
}
/* Excalidraw's own styles take over inside; keep its font stack. */
.excalidraw-host :deep(.excalidraw) {
  --ui-font: -apple-system, BlinkMacSystemFont, 'SF Pro Text', system-ui, sans-serif;
}
</style>
