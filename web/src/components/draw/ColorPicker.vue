<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Pipette, Plus, X } from 'lucide-vue-next'
import { hexOf, hexToHsv, hsvToHex, type HSV } from '@/lib/colors'
import { usePrefsStore } from '@/stores/prefs'

// Choosing a colour, every way: a square of shades with a hue strip, its
// code, how see-through, the eyedropper, the standard colours, the last
// ones used, and the person's own palette (kept with their settings).
const props = defineProps<{ color: string; opacity: number; recent: string[] }>()
const emit = defineEmits<{ 'update:color': [c: string]; 'update:opacity': [o: number]; dropper: [] }>()
const { t } = useI18n()
const prefs = usePrefsStore()

const STANDARD = ['#1c1c1e', '#8e8e93', '#ffffff', '#ff3b30', '#ff9500', '#ffcc00', '#34c759', '#30b0c7', '#007aff', '#5856d6', '#af52de', '#ff2d55', '#a2845e']

// The shade being chosen (kept apart from the code, so a grey keeps its hue).
const hsv = ref<HSV>(hexToHsv(props.color))
watch(
  () => props.color,
  (c) => {
    if (hsvToHex(hsv.value) !== c) hsv.value = hexToHsv(c)
  },
)
const set = (next: HSV) => {
  hsv.value = next
  emit('update:color', hsvToHex(next))
}
const hueColor = computed(() => hsvToHex({ h: hsv.value.h, s: 1, v: 1 }))

const area = ref<HTMLElement | null>(null)
function pickArea(e: PointerEvent) {
  const r = area.value!.getBoundingClientRect()
  const s = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width))
  const v = 1 - Math.min(1, Math.max(0, (e.clientY - r.top) / r.height))
  set({ ...hsv.value, s, v })
}
function areaDown(e: PointerEvent) {
  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  pickArea(e)
}
function areaKey(e: KeyboardEvent) {
  const step = e.shiftKey ? 0.1 : 0.02
  const d: Record<string, [number, number]> = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, step], ArrowDown: [0, -step] }
  const m = d[e.key]
  if (!m) return
  e.preventDefault()
  set({ ...hsv.value, s: Math.min(1, Math.max(0, hsv.value.s + m[0])), v: Math.min(1, Math.max(0, hsv.value.v + m[1])) })
}

const code = ref(props.color)
watch(
  () => props.color,
  (c) => (code.value = c),
)
function typed() {
  const c = hexOf(code.value)
  if (c) emit('update:color', c)
  else code.value = props.color
}

// The eyedropper: the browser's own (anywhere on screen) where there is one,
// else a click on the sketch.
async function dropper() {
  const E = (window as unknown as { EyeDropper?: new () => { open: () => Promise<{ sRGBHex: string }> } }).EyeDropper
  if (!E) return emit('dropper')
  try {
    const { sRGBHex } = await new E().open()
    const c = hexOf(sRGBHex)
    if (c) emit('update:color', c)
  } catch {
    /* cancelled */
  }
}

const mine = computed(() => prefs.sketchColors)
function keep() {
  if (mine.value.includes(props.color)) return
  void prefs.setSketchColors([...mine.value, props.color].slice(-40))
}
const forget = (c: string) => void prefs.setSketchColors(mine.value.filter((x) => x !== c))
</script>

<template>
  <div class="cp" data-testid="color-picker">
    <div
      ref="area"
      class="cp-area"
      :style="{ background: `linear-gradient(to top, #000, transparent), linear-gradient(to right, #fff, ${hueColor})` }"
      role="slider"
      tabindex="0"
      :aria-label="t('sketch.shade')"
      :aria-valuetext="color"
      @pointerdown="areaDown"
      @pointermove="(e) => e.buttons && pickArea(e)"
      @keydown="areaKey"
    >
      <span class="cp-knob" :style="{ left: `${hsv.s * 100}%`, top: `${(1 - hsv.v) * 100}%`, background: color }" />
    </div>
    <input
      class="cp-hue"
      type="range"
      min="0"
      max="359"
      :value="Math.round(hsv.h)"
      :aria-label="t('sketch.hue')"
      @input="set({ ...hsv, h: Number(($event.target as HTMLInputElement).value), s: hsv.s || 1, v: hsv.v || 1 })"
    />
    <input
      class="cp-alpha"
      type="range"
      min="5"
      max="100"
      :value="Math.round(opacity * 100)"
      :style="{ '--c': color }"
      :aria-label="t('sketch.opacity')"
      data-testid="color-opacity"
      @input="emit('update:opacity', Number(($event.target as HTMLInputElement).value) / 100)"
    />
    <div class="flex items-center gap-2">
      <span class="cp-now" :style="{ background: color, opacity }" />
      <input v-model="code" class="input min-w-0 flex-1 !py-1 font-mono text-[13px]" maxlength="7" :aria-label="t('sketch.code')" data-testid="color-code" @keydown.enter.prevent="typed" @blur="typed" />
      <span class="w-10 text-right text-[12px] tabular-nums text-text-3">{{ Math.round(opacity * 100) }}%</span>
      <button type="button" class="icon-btn h-8 w-8" :title="t('sketch.eyedropper')" :aria-label="t('sketch.eyedropper')" data-testid="color-dropper" @click="dropper"><Pipette class="h-4 w-4" /></button>
    </div>

    <div class="cp-row">
      <button v-for="c in STANDARD" :key="c" type="button" class="cp-swatch" :class="{ on: c === color }" :style="{ background: c }" :title="c" :aria-label="c" @click="emit('update:color', c)" />
    </div>
    <template v-if="recent.length">
      <div class="cp-cap">{{ t('sketch.recent') }}</div>
      <div class="cp-row">
        <button v-for="c in recent" :key="c" type="button" class="cp-swatch" :class="{ on: c === color }" :style="{ background: c }" :title="c" :aria-label="c" @click="emit('update:color', c)" />
      </div>
    </template>
    <div class="cp-cap">{{ t('sketch.mine') }}</div>
    <div class="cp-row" data-testid="my-colors">
      <span v-for="c in mine" :key="c" class="cp-mine">
        <button type="button" class="cp-swatch" :class="{ on: c === color }" :style="{ background: c }" :title="c" :aria-label="c" @click="emit('update:color', c)" @contextmenu.prevent="forget(c)" />
        <button type="button" class="cp-forget" :aria-label="t('sketch.forget', { c })" @click="forget(c)"><X class="h-2.5 w-2.5" :stroke-width="3" /></button>
      </span>
      <button type="button" class="cp-add" :title="t('sketch.keep')" :aria-label="t('sketch.keep')" data-testid="color-keep" @click="keep"><Plus class="h-3.5 w-3.5" /></button>
    </div>
  </div>
</template>

<style scoped>
.cp {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
}
.cp-area {
  position: relative;
  height: 140px;
  border-radius: 10px;
  cursor: crosshair;
  touch-action: none;
}
.cp-area:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--ring);
}
.cp-knob {
  position: absolute;
  width: 18px;
  height: 18px;
  margin: -9px 0 0 -9px;
  border: 2.5px solid #fff;
  border-radius: 50%;
  box-shadow: 0 0 0 0.5px rgba(0, 0, 0, 0.4), 0 1px 3px rgba(0, 0, 0, 0.3);
  pointer-events: none;
}
.cp-hue,
.cp-alpha {
  width: 100%;
  height: 14px;
  border-radius: 7px;
  appearance: none;
  -webkit-appearance: none;
  cursor: pointer;
}
.cp-hue {
  background: linear-gradient(to right, #f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00);
}
.cp-alpha {
  background:
    linear-gradient(to right, transparent, var(--c)),
    repeating-conic-gradient(#d1d1d6 0 25%, #fff 0 50%) 0 0 / 10px 10px;
}
.cp-hue::-webkit-slider-thumb,
.cp-alpha::-webkit-slider-thumb {
  -webkit-appearance: none;
  width: 18px;
  height: 18px;
  border: 2.5px solid #fff;
  border-radius: 50%;
  background: transparent;
  box-shadow: 0 0 0 0.5px rgba(0, 0, 0, 0.4), 0 1px 3px rgba(0, 0, 0, 0.3);
}
.cp-hue::-moz-range-thumb,
.cp-alpha::-moz-range-thumb {
  width: 14px;
  height: 14px;
  border: 2.5px solid #fff;
  border-radius: 50%;
  background: transparent;
  box-shadow: 0 0 0 0.5px rgba(0, 0, 0, 0.4);
}
.cp-now {
  width: 28px;
  height: 28px;
  flex: none;
  border-radius: 50%;
  box-shadow: inset 0 0 0 0.5px rgba(0, 0, 0, 0.2);
}
.cp-cap {
  margin-bottom: -4px;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-3);
}
.cp-row {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
}
.cp-swatch {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  box-shadow: inset 0 0 0 0.5px rgba(0, 0, 0, 0.25);
}
.cp-swatch.on {
  box-shadow: 0 0 0 2px var(--surface), 0 0 0 3.5px var(--accent);
}
.cp-mine {
  position: relative;
  display: inline-flex;
}
.cp-forget {
  position: absolute;
  top: -5px;
  right: -5px;
  display: none;
  width: 14px;
  height: 14px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--surface);
  color: var(--text-2);
  box-shadow: 0 0 0 0.5px var(--border), 0 1px 2px rgba(0, 0, 0, 0.2);
}
.cp-mine:hover .cp-forget,
.cp-forget:focus-visible {
  display: flex;
}
.cp-add {
  display: inline-flex;
  width: 22px;
  height: 22px;
  align-items: center;
  justify-content: center;
  border: 1px dashed var(--border-strong, var(--border));
  border-radius: 50%;
  color: var(--text-2);
}
</style>
