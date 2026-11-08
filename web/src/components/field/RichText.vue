<script setup lang="ts">
// Показ форматированного текста (F-141): Markdown → HTML, всегда очищенный
// (ui/richText.ts) — v-html получает только безопасное подмножество.
import { computed } from 'vue'

import { renderRichText } from '@/ui/richText'

const props = defineProps<{ source: string }>()

const html = computed(() => renderRichText(props.source))
</script>

<template>
  <!-- eslint-disable vue/no-v-html -- HTML очищен DOMPurify (renderRichText) -->
  <div
    class="rich-text"
    v-html="html"
  />
  <!-- eslint-enable vue/no-v-html -->
</template>

<style scoped>
.rich-text {
  min-width: 0;
  font-weight: 400;
  line-height: 1.5;
  overflow-wrap: anywhere;
}

.rich-text :deep(> :first-child) {
  margin-top: 0;
}

.rich-text :deep(> :last-child) {
  margin-bottom: 0;
}

.rich-text :deep(p),
.rich-text :deep(ul),
.rich-text :deep(ol),
.rich-text :deep(blockquote),
.rich-text :deep(pre),
.rich-text :deep(table) {
  margin: 0 0 var(--kin-space-2);
}

.rich-text :deep(h1),
.rich-text :deep(h2),
.rich-text :deep(h3),
.rich-text :deep(h4),
.rich-text :deep(h5),
.rich-text :deep(h6) {
  margin: var(--kin-space-3) 0 var(--kin-space-1);
  font-size: 1rem;
  font-weight: 700;
}

.rich-text :deep(h1) {
  font-size: 1.25rem;
}

.rich-text :deep(h2) {
  font-size: 1.125rem;
}

.rich-text :deep(ul),
.rich-text :deep(ol) {
  padding-left: 1.4em;
}

.rich-text :deep(blockquote) {
  padding-left: var(--kin-space-2);
  color: var(--kin-color-text-muted);
  border-left: 3px solid var(--kin-color-border-strong);
}

.rich-text :deep(code) {
  padding: 0 0.2em;
  font-size: 0.9em;
  background: var(--kin-color-surface-sunken);
  border-radius: 4px;
}

.rich-text :deep(pre) {
  padding: var(--kin-space-2);
  overflow-x: auto;
  background: var(--kin-color-surface-sunken);
  border-radius: var(--kin-radius);
}

.rich-text :deep(pre code) {
  padding: 0;
  background: none;
}

.rich-text :deep(table) {
  border-collapse: collapse;
}

.rich-text :deep(th),
.rich-text :deep(td) {
  padding: 2px var(--kin-space-2);
  border: 1px solid var(--kin-color-border);
}

.rich-text :deep(hr) {
  border: 0;
  border-top: 1px solid var(--kin-color-border);
}
</style>
