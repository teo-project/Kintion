<script setup lang="ts">
// Картинка записи (F-72, F-79): превью поля photo с обрезкой своего места
// показа из meta.crop — карточка (портрет / свои пропорции) или сеть и
// списки (квадрат / круг); нет фото — заглушка с первой буквой подписи.
import { computed } from 'vue'

import { api, type PreviewShape } from '@/api/client'
import { previewOf, type Place } from '@/ui/crop'

const props = withDefaults(defineProps<{
  dashboard: string
  photo: { sha: string; crop: unknown } | null
  title: string
  size?: 64 | 128 | 256
  shape?: PreviewShape
  place?: Place
}>(), { size: 64, shape: 'circle', place: 'network' })

/** обрезка и форма места; у сети форма — из сохранённой обрезки, иначе по умолчанию компонента */
const view = computed(() => previewOf(props.photo?.crop, props.place, props.shape))
const form = computed(() => view.value.shape)
const src = computed(() => props.photo
  ? api.previewUrl(props.dashboard, props.photo.sha, view.value.crop, form.value, props.size)
  : '')
const letter = computed(() => props.title.trim().charAt(0).toUpperCase() || '·')
</script>

<template>
  <img
    v-if="src"
    class="avatar"
    :class="form"
    :src="src"
    alt=""
    loading="lazy"
  >
  <span
    v-else
    class="avatar placeholder"
    :class="shape"
    aria-hidden="true"
  >{{ letter }}</span>
</template>

<style scoped>
.avatar {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 2.5rem;
  height: 2.5rem;
  object-fit: cover;
  border-radius: var(--kin-radius);
}

.avatar.circle {
  border-radius: 50%;
}

.avatar.rect {
  width: auto;
  max-width: 8rem;
  height: 8rem;
  object-fit: contain;
}

.placeholder {
  color: var(--kin-color-accent-text);
  background: var(--kin-color-surface-raised);
  font-weight: 800;
}
</style>
