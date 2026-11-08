<script setup lang="ts">
// Показ одного значения по типу поля (F-65, F-54): телефон — tel:, почта —
// mailto:, ссылка — внешняя ссылка, ref — подпись цели со ссылкой на её
// карточку, image — превью (клик — просмотр с зумом); файл (KIN-27) — имя и
// размер: изображение — превью и просмотр, PDF — в новой вкладке, прочее —
// скачивание; форматированный текст — очищенный HTML (F-141);
// остальное — текст по правилам шаблона.
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'

import { api } from '@/api/client'
import { storedRaw, isFileValue } from '@/core/resolver'
import type { FieldDef, ValueDTO } from '@/core/types'
import { fmt, t } from '@/i18n'
import { previewOf } from '@/ui/crop'
import KinIcon from '../KinIcon.vue'
import ImageLightbox from './ImageLightbox.vue'
import RichText from './RichText.vue'

const props = defineProps<{ def: Pick<FieldDef, 'type'>; value: ValueDTO; text: string; dashboard: string }>()

const raw = computed(() => (typeof props.value.value === 'string' ? props.value.value : ''))

/** Только http(s): прочие схемы (javascript: и т. п.) ссылкой не становятся. */
const href = computed(() => {
  const v = raw.value.trim()
  if (/^https?:\/\//i.test(v)) return v
  if (/^[a-z][a-z0-9+.-]*:/i.test(v)) return ''
  return v ? `https://${v}` : ''
})

/** фото в поле — обрезка карточки (F-79) */
const preview = computed(() => previewOf(props.value.meta?.crop, 'card'))
const shape = computed(() => preview.value.shape)

const tel = computed(() => raw.value.replace(/[^\d+*#,;]/g, ''))

const viewing = ref(false)

/** поле «Файл»: сведения из meta; превью — у изображений поддерживаемых форматов */
const file = computed(() => {
  const v = storedRaw(props.value)
  return isFileValue(v) ? v : raw.value ? { sha256: raw.value, name: raw.value.slice(0, 12) } : null
})
/** превью — по признаку сервера (у огромного скана его нет), не по типу */
const fileImage = computed(() => file.value?.preview === true)
/** в браузере открываются PDF и изображения (сервер отдаёт их без «скачать») */
const fileInline = computed(() => file.value?.mime === 'application/pdf' || ['image/jpeg', 'image/png', 'image/webp', 'image/gif'].includes(file.value?.mime ?? ''))

function sizeText(n?: number): string {
  if (n == null) return ''
  if (n < 1024) return fmt(t.file.bytes, { n })
  if (n < 1024 * 1024) return fmt(t.file.kb, { n: Math.round(n / 1024) })
  return fmt(t.file.mb, { n: (n / 1024 / 1024).toFixed(1) })
}
</script>

<template>
  <a
    v-if="def.type === 'phone' && tel"
    class="fv"
    :href="`tel:${tel}`"
  >{{ text }}</a>
  <a
    v-else-if="def.type === 'email' && raw"
    class="fv"
    :href="`mailto:${raw.trim()}`"
  >{{ text }}</a>
  <a
    v-else-if="def.type === 'url' && href"
    class="fv"
    :href="href"
    target="_blank"
    rel="noopener noreferrer"
  >{{ text }}</a>
  <template v-else-if="def.type === 'ref'">
    <RouterLink
      v-if="raw"
      class="fv"
      :to="{ name: 'record', params: { d: dashboard, id: raw } }"
    >
      {{ text }}
    </RouterLink>
    <span
      v-else
      class="fv kin-muted"
    >{{ t.card.refMissing }}</span>
  </template>
  <RichText
    v-else-if="def.type === 'rich_text' && raw"
    class="fv rich"
    :source="raw"
  />
  <button
    v-else-if="def.type === 'image' && raw"
    class="fv view"
    type="button"
    :aria-label="t.file.view"
    :title="t.file.view"
    @click="viewing = true"
  >
    <img
      class="image"
      :class="shape"
      :src="api.previewUrl(dashboard, raw, preview.crop, shape, 128)"
      alt=""
    >
    <ImageLightbox
      v-model:open="viewing"
      :src="api.fileUrl(dashboard, raw)"
    />
  </button>
  <span
    v-else-if="def.type === 'file' && file"
    class="fv file"
  >
    <button
      v-if="fileImage"
      class="view"
      type="button"
      :aria-label="t.file.view"
      :title="t.file.view"
      @click="viewing = true"
    >
      <img
        class="image"
        :src="api.previewUrl(dashboard, file.sha256, null, 'square', 64)"
        alt=""
      >
    </button>
    <KinIcon
      v-else
      name="file"
    />
    <a
      class="name"
      :href="api.fileUrl(dashboard, file.sha256)"
      :target="fileInline ? '_blank' : undefined"
      :rel="fileInline ? 'noopener' : undefined"
      :download="fileInline ? undefined : file.name"
      :title="fileInline ? t.file.open : t.file.download"
    >{{ file.name }}</a>
    <span
      v-if="file.size != null"
      class="kin-muted size"
    >{{ sizeText(file.size) }}</span>
    <ImageLightbox
      v-if="fileImage"
      v-model:open="viewing"
      :src="api.fileUrl(dashboard, file.sha256)"
      :name="file.name"
    />
  </span>
  <span
    v-else
    class="fv"
  >{{ text }}</span>
</template>

<style scoped>
.rich {
  flex-basis: 100%;
}

.view {
  padding: 0;
  cursor: zoom-in;
  background: none;
  border: 0;
  border-radius: var(--kin-radius);
}

.file {
  display: inline-flex;
  gap: var(--kin-space-2);
  align-items: center;
  min-width: 0;
}

.file .name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file .image {
  width: 2.5rem;
  height: 2.5rem;
}

.size {
  font-size: 12px;
  white-space: nowrap;
}

.image {
  display: block;
  width: 4rem;
  height: 4rem;
  border-radius: var(--kin-radius);
}

.image.circle {
  border-radius: 50%;
}

.image.rect {
  width: auto;
  max-width: 6rem;
  height: 5rem;
  object-fit: contain;
}
</style>
