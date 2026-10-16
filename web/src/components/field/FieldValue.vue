<script setup lang="ts">
// Показ одного значения по типу поля (F-65, F-54): телефон — tel:, почта —
// mailto:, ссылка — внешняя ссылка, ref — подпись цели со ссылкой на её
// карточку, image — превью; остальное — текст по правилам шаблона.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'

import { api } from '@/api/client'
import type { FieldDef, ValueDTO } from '@/core/types'
import { t } from '@/i18n'
import { previewOf } from '@/ui/crop'

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
  <img
    v-else-if="def.type === 'image' && raw"
    class="fv image"
    :class="shape"
    :src="api.previewUrl(dashboard, raw, preview.crop, shape, 128)"
    alt=""
  >
  <span
    v-else
    class="fv"
  >{{ text }}</span>
</template>

<style scoped>
.image {
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
