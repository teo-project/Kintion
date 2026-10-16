<script setup lang="ts">
// Новое изображение в поле типа image (F-29, F-72): диалог загрузки (F-107 —
// буфер, перетаскивание, файл), затем
// обрезки под места показа (если поле их объявляет, F-79) и сохранение. Поле с историей или
// multiple — новое значение сверху (старое фото остаётся ниже); без
// истории — замена текущего.
import { DialogClose, DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import { computed, ref } from 'vue'

import { api } from '@/api/client'
import type { FieldView } from '@/core/card'
import { t } from '@/i18n'
import { cropMeta, cropPlaces, cropShapes, type Crops } from '@/ui/crop'
import { useRecord } from '@/ui/record'

import CropEditor from '../CropEditor.vue'
import KinIcon from '../KinIcon.vue'
import ImagePickDialog from './ImagePickDialog.vue'

const props = defineProps<{ field: FieldView; dashboard: string }>()

const s = useRecord()
/** диалог загрузки (F-107) */
const picking = ref(false)
const sha = ref('')
/** обрезки по местам показа (F-79) */
const crop = ref<Crops | null>(null)
const open = ref(false)

const meta = computed(() => cropMeta(props.field.def))
const shapes = computed(() => cropShapes(meta.value))
const places = computed(() => cropPlaces(meta.value))

/** файл из диалога загрузки: на сервер, затем обрезка (F-79) или сразу значение */
async function upload(file: File) {
  const f = await api.uploadFile(props.dashboard, file)
  sha.value = f.sha256
  crop.value = null
  if (meta.value) open.value = true
  else await save(false)
}

async function save(withCrop: boolean) {
  const rec = s.rec.value
  if (!rec || !sha.value) return
  const key = props.field.key
  const m = withCrop && meta.value && crop.value ? { [meta.value.key]: crop.value } : {}
  if (s.draft.value) {
    // черновик (F-88): фото — первое значение новой записи
    const res = await s.createWith(key, { fields: { [key]: [{ value: sha.value, meta: m }] } })
    if (res === 'ok') open.value = false
    return
  }
  const top = rec.fields[key]?.values[0]
  const replace = top && !props.field.def.history && !props.field.def.multiple
  const ok = await s.save(key, rev => replace
    ? api.patchValue(rec.id, key, top.id, { if_rev: rev, value: sha.value, meta: m })
    : api.addValue(rec.id, key, { if_rev: rev, value: sha.value, meta: m, position: 'top' }))
  if (ok || !s.fieldErrors[key]) open.value = false
}
</script>

<template>
  <button
    class="kin-icon-btn kin-semi upload"
    type="button"
    :aria-label="`${t.photo.upload}: ${field.name}`"
    :title="t.photo.upload"
    @click="picking = true"
  >
    <KinIcon name="upload" />
  </button>
  <ImagePickDialog
    v-model:open="picking"
    :upload="upload"
  />
  <DialogRoot v-model:open="open">
    <DialogPortal>
      <DialogOverlay class="kin-overlay" />
      <DialogContent
        class="kin-dialog crop-dialog"
        :aria-describedby="undefined"
      >
        <DialogTitle>{{ t.photo.crop }}</DialogTitle>
        <CropEditor
          v-model="crop"
          :src="api.fileUrl(dashboard, sha)"
          :shapes="shapes"
          :places="places"
        />
        <p
          v-if="s.fieldErrors[field.key]"
          class="kin-error"
          role="alert"
        >
          {{ s.fieldErrors[field.key] }}
        </p>
        <div class="kin-actions">
          <DialogClose
            class="kin-ibtn"
            :aria-label="t.common.cancel"
            :title="t.common.cancel"
          >
            <KinIcon name="close" />
          </DialogClose>
          <button
            class="kin-btn no-crop"
            type="button"
            :disabled="s.saving[field.key]"
            @click="save(false)"
          >
            {{ t.photo.noCrop }}
          </button>
          <button
            class="kin-ibtn kin-ibtn--primary save"
            type="button"
            :disabled="s.saving[field.key] || !crop"
            :aria-label="t.common.save"
            :title="t.common.save"
            @click="save(true)"
          >
            <KinIcon name="check" />
          </button>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
