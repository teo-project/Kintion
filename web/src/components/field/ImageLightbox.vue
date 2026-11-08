<script setup lang="ts">
// Просмотр изображения во всплывающем окне (KIN-27): оригинал по размеру
// окна; зум — колесом (к точке под курсором), щипком двумя пальцами и
// кнопками; увеличенное — перетаскивается. Esc и клик по фону — закрыть.
import { DialogClose, DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import { reactive, ref, watch } from 'vue'

import { t } from '@/i18n'

import KinIcon from '../KinIcon.vue'

defineProps<{ src: string; name?: string }>()
const open = defineModel<boolean>('open', { required: true })

const MIN = 1
const MAX = 8
const view = reactive({ scale: 1, x: 0, y: 0 })
const stage = ref<HTMLElement | null>(null)

function reset() {
  Object.assign(view, { scale: 1, x: 0, y: 0 })
}
watch(open, reset)

/** масштаб к точке (cx, cy) относительно центра сцены: точка остаётся на месте */
function zoomTo(scale: number, cx = 0, cy = 0) {
  const s = Math.min(MAX, Math.max(MIN, scale))
  const k = s / view.scale
  view.x = cx - (cx - view.x) * k
  view.y = cy - (cy - view.y) * k
  view.scale = s
  if (s === MIN) reset()
}

function fromCenter(e: { clientX: number; clientY: number }): [number, number] {
  const r = stage.value?.getBoundingClientRect()
  return r ? [e.clientX - r.left - r.width / 2, e.clientY - r.top - r.height / 2] : [0, 0]
}

function onWheel(e: WheelEvent) {
  e.preventDefault()
  zoomTo(view.scale * Math.exp(-e.deltaY * 0.0015), ...fromCenter(e))
}

// указатели: один — перетаскивание, два — щипок
const pts = new Map<number, { x: number; y: number }>()
let pinch: { dist: number; scale: number } | null = null
let dragged = false
/** нажатие началось на фоне, а не на картинке: захват указателя меняет цель клика */
let downOnStage = false

function dist(): number {
  const [a, b] = [...pts.values()]
  return a && b ? Math.hypot(a.x - b.x, a.y - b.y) : 0
}

function onDown(e: PointerEvent) {
  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  pts.set(e.pointerId, { x: e.clientX, y: e.clientY })
  dragged = false
  downOnStage = e.target === e.currentTarget
  if (pts.size === 2) pinch = { dist: dist(), scale: view.scale }
}

function onMove(e: PointerEvent) {
  const p = pts.get(e.pointerId)
  if (!p) return
  const dx = e.clientX - p.x
  const dy = e.clientY - p.y
  pts.set(e.pointerId, { x: e.clientX, y: e.clientY })
  if (pinch && pts.size === 2) {
    const [a, b] = [...pts.values()]
    if (a && b && pinch.dist > 0) zoomTo(pinch.scale * (dist() / pinch.dist), ...fromCenter({ clientX: (a.x + b.x) / 2, clientY: (a.y + b.y) / 2 }))
    dragged = true
    return
  }
  if (view.scale > MIN && pts.size === 1) {
    view.x += dx
    view.y += dy
    if (Math.abs(dx) + Math.abs(dy) > 2) dragged = true
  }
}

function onUp(e: PointerEvent) {
  pts.delete(e.pointerId)
  if (pts.size < 2) pinch = null
}

/** клик по фону (нажатие началось не на картинке, без перетаскивания) — закрыть */
function onStageClick() {
  if (!dragged && downOnStage) open.value = false
}

function onDblClick(e: MouseEvent) {
  if (view.scale > MIN) reset()
  else zoomTo(2.5, ...fromCenter(e))
}
</script>

<template>
  <DialogRoot v-model:open="open">
    <DialogPortal>
      <DialogOverlay class="kin-overlay lightbox-overlay" />
      <DialogContent
        class="lightbox"
        :aria-describedby="undefined"
      >
        <DialogTitle class="title">
          {{ name || t.file.view }}
        </DialogTitle>
        <div
          ref="stage"
          class="stage"
          @wheel="onWheel"
          @pointerdown="onDown"
          @pointermove="onMove"
          @pointerup="onUp"
          @pointercancel="onUp"
          @click="onStageClick"
          @dblclick="onDblClick"
        >
          <img
            :src="src"
            :alt="name ?? ''"
            draggable="false"
            :class="{ zoomed: view.scale > 1 }"
            :style="{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.scale})` }"
          >
        </div>
        <div class="tools">
          <button
            class="kin-ibtn"
            type="button"
            :aria-label="t.file.zoomOut"
            :title="t.file.zoomOut"
            :disabled="view.scale <= MIN"
            @click="zoomTo(view.scale / 1.5)"
          >
            <KinIcon name="zoomOut" />
          </button>
          <button
            class="kin-btn kin-btn--small"
            type="button"
            :title="t.file.zoomReset"
            @click="reset"
          >
            {{ Math.round(view.scale * 100) }}%
          </button>
          <button
            class="kin-ibtn"
            type="button"
            :aria-label="t.file.zoomIn"
            :title="t.file.zoomIn"
            :disabled="view.scale >= MAX"
            @click="zoomTo(view.scale * 1.5)"
          >
            <KinIcon name="zoomIn" />
          </button>
          <a
            class="kin-ibtn"
            :href="src"
            target="_blank"
            rel="noopener"
            :aria-label="t.file.open"
            :title="t.file.open"
          >
            <KinIcon name="external" />
          </a>
          <DialogClose
            class="kin-ibtn"
            :aria-label="t.common.close"
            :title="t.common.close"
          >
            <KinIcon name="close" />
          </DialogClose>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style scoped>
.lightbox-overlay {
  background: rgb(0 0 0 / 82%);
}

.lightbox {
  position: fixed;
  inset: 0;
  z-index: 41;
  display: flex;
  flex-direction: column;
  outline: none;
}

.title {
  margin: 0;
  padding: var(--kin-space-2) var(--kin-space-3);
  overflow: hidden;
  font-size: 14px;
  font-weight: 500;
  color: #fff;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.stage {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: center;
  min-height: 0;
  overflow: hidden;
  touch-action: none;
  user-select: none;
}

img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  transform-origin: center;
  cursor: zoom-in;
}

img.zoomed {
  cursor: grab;
}

.tools {
  display: flex;
  gap: var(--kin-space-2);
  align-items: center;
  justify-content: center;
  padding: var(--kin-space-2);
  background: var(--kin-color-surface);
}
</style>
