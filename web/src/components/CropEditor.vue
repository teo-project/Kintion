<script setup lang="ts">
// Редактор обрезок (F-79): своя рамка для каждого места показа — карточка
// (произвольные пропорции, начальная — портрет), дерево (портрет 3:4),
// сеть (квадрат; квадрат или круг — из объявления `shapes`). Рамку
// перетаскивают, размер — ползунком (у карточки — ширина и высота).
// Значение — обрезки всех мест в пикселях исходника.
import { computed, onBeforeUnmount, ref } from 'vue'

import type { PreviewShape } from '@/api/client'
import { t } from '@/i18n'
import { ASPECT, clampCrop, fullCrops, PLACES, type Crops, type Place, type PlaceCrop } from '@/ui/crop'

const props = defineProps<{
  src: string
  shapes: PreviewShape[]
  /** места показа из объявления обрезки; нет — все (F-79); одно — без вкладок */
  places?: Place[]
}>()
const places = computed(() => (props.places?.length ? props.places : PLACES))
const crops = defineModel<Crops | null>({ required: true })

const box = ref<HTMLElement | null>(null)
const natural = ref<{ w: number; h: number } | null>(null)
const place = ref<Place>(places.value[0] ?? 'card')

function onLoad(e: Event) {
  const img = e.target as HTMLImageElement
  natural.value = { w: img.naturalWidth, h: img.naturalHeight }
  crops.value = fullCrops(crops.value, img.naturalWidth, img.naturalHeight, props.shapes[0] ?? 'square')
}

const crop = computed<PlaceCrop | null>(() => crops.value?.[place.value] ?? null)
const free = computed(() => ASPECT[place.value] == null)

const frame = computed(() => {
  const n = natural.value
  const c = crop.value
  if (!n || !c) return null
  return {
    left: `${(c.x / n.w) * 100}%`, top: `${(c.y / n.h) * 100}%`,
    width: `${(c.w / n.w) * 100}%`, height: `${(c.h / n.h) * 100}%`,
  }
})

function update(patch: Partial<PlaceCrop>) {
  const n = natural.value
  const c = crop.value
  if (!n || !c || !crops.value) return
  crops.value = { ...crops.value, [place.value]: clampCrop({ ...c, ...patch }, n.w, n.h, ASPECT[place.value]) }
}

/** Размер рамки с сохранением центра; у фиксированных пропорций высота следует за шириной. */
function onSize(e: Event, side: 'w' | 'h') {
  const c = crop.value
  if (!c) return
  const v = Number((e.target as HTMLInputElement).value)
  const aspect = ASPECT[place.value]
  const w = side === 'w' ? v : c.w
  const h = side === 'h' ? v : aspect != null ? v / aspect : c.h
  update({ w, h, x: c.x + (c.w - w) / 2, y: c.y + (c.h - h) / 2 })
}

let drag: { px: number; py: number; x: number; y: number; scale: number } | null = null

function onDown(e: PointerEvent) {
  const c = crop.value
  const n = natural.value
  if (!c || !n || !box.value) return
  drag = { px: e.clientX, py: e.clientY, x: c.x, y: c.y, scale: n.w / box.value.clientWidth }
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}

function onMove(e: PointerEvent) {
  if (!drag) return
  update({ x: drag.x + (e.clientX - drag.px) * drag.scale, y: drag.y + (e.clientY - drag.py) * drag.scale })
}

function onUp() {
  drag = null
  window.removeEventListener('pointermove', onMove)
  window.removeEventListener('pointerup', onUp)
}

onBeforeUnmount(onUp)

function shapeName(sh: PreviewShape): string {
  return sh === 'circle' ? t.photo.shapes.circle : t.photo.shapes.square
}
</script>

<template>
  <div class="crop-editor">
    <div
      v-if="places.length > 1"
      class="kin-tabs places"
      role="tablist"
      :aria-label="t.photo.place"
    >
      <button
        v-for="p in places"
        :key="p"
        class="kin-tab"
        type="button"
        role="tab"
        :aria-selected="place === p"
        @click="place = p"
      >
        {{ t.photo.places[p] }}
      </button>
    </div>
    <div
      ref="box"
      class="box"
    >
      <img
        :src="src"
        alt=""
        draggable="false"
        @load="onLoad"
      >
      <div
        v-if="frame && crop"
        class="frame"
        :class="crop.shape"
        :style="frame"
        @pointerdown.prevent="onDown"
      />
    </div>
    <template v-if="crop && natural">
      <label class="kin-label">
        {{ free ? t.photo.width : t.photo.size }}
        <input
          type="range"
          :min="Math.min(16, natural.w)"
          :max="natural.w"
          :value="crop.w"
          @input="e => onSize(e, 'w')"
        >
      </label>
      <label
        v-if="free"
        class="kin-label"
      >
        {{ t.photo.height }}
        <input
          type="range"
          :min="Math.min(16, natural.h)"
          :max="natural.h"
          :value="crop.h"
          @input="e => onSize(e, 'h')"
        >
      </label>
      <div
        v-if="place === 'network' && shapes.length > 1"
        class="kin-row"
        role="radiogroup"
        :aria-label="t.photo.shape"
      >
        <button
          v-for="sh in shapes"
          :key="sh"
          class="kin-btn kin-btn--small"
          type="button"
          role="radio"
          :aria-checked="crop.shape === sh"
          @click="update({ shape: sh })"
        >
          {{ shapeName(sh) }}
        </button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.crop-editor {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-2);
}

.box {
  position: relative;
  max-width: 20rem;
  line-height: 0;
  overflow: hidden;
  user-select: none;
  touch-action: none;
}

.box img {
  width: 100%;
  height: auto;
}

.frame {
  position: absolute;
  box-sizing: border-box;
  border: 2px solid var(--kin-color-accent-contrast);
  outline: 2px solid var(--kin-color-accent);
  box-shadow: 0 0 0 9999px var(--kin-color-overlay);
  cursor: move;
}

.frame.circle {
  border-radius: 50%;
}

.places {
  align-self: flex-start;
}

.box {
  border-radius: 12px;
}

[aria-checked='true'] {
  color: var(--kin-color-accent-contrast);
  background: var(--kin-color-accent);
}
</style>
