<script setup lang="ts">
// Новый файл в поле типа file (KIN-27): диалог загрузки (F-107 — буфер,
// перетаскивание, файл), любой тип; сразу сохраняется значением. Поле с
// историей или multiple — новое значение сверху; без них — замена текущего.
import { ref } from 'vue'

import { api } from '@/api/client'
import type { FieldView } from '@/core/card'
import { FILE_META } from '@/core/resolver'
import { t } from '@/i18n'
import { useRecord } from '@/ui/record'

import KinIcon from '../KinIcon.vue'
import ImagePickDialog from './ImagePickDialog.vue'

const props = defineProps<{ field: FieldView; dashboard: string }>()

const s = useRecord()
const picking = ref(false)

/** файл из диалога: на сервер, затем значение; ошибка сохранения — в диалоге */
async function upload(file: File) {
  const f = await api.uploadFile(props.dashboard, file, true)
  const rec = s.rec.value
  if (!rec) return
  const key = props.field.key
  const meta = { [FILE_META]: { name: file.name || f.sha256.slice(0, 12), mime: f.mime, size: f.size } }
  if (s.draft.value) {
    // черновик (F-88): файл — первое значение новой записи
    if (await s.createWith(key, { fields: { [key]: [{ value: f.sha256, meta }] } }) !== 'ok') throw new Error(s.fieldErrors[key] || t.errors.unknown)
    return
  }
  const top = rec.fields[key]?.values[0]
  const replace = top && !props.field.def.history && !props.field.def.multiple
  const ok = await s.save(key, rev => replace
    ? api.patchValue(rec.id, key, top.id, { if_rev: rev, value: f.sha256, meta })
    : api.addValue(rec.id, key, { if_rev: rev, value: f.sha256, meta, position: 'top' }))
  // конфликт (409) — поле уже перечитано; диалог остаётся с ошибкой, файл можно отправить снова
  if (!ok) throw new Error(s.fieldErrors[key] || t.errors.conflict)
}
</script>

<template>
  <button
    class="kin-icon-btn kin-semi upload"
    type="button"
    :aria-label="`${t.file.upload}: ${field.name}`"
    :title="t.file.upload"
    @click="picking = true"
  >
    <KinIcon name="upload" />
  </button>
  <ImagePickDialog
    v-model:open="picking"
    any
    :upload="upload"
  />
</template>
