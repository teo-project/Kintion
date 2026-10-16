<script setup lang="ts">
// Диалог загрузки изображения (F-107): вставка из буфера (Ctrl+V, пока диалог
// открыт), перетаскивание файла в область диалога, выбор файла с диска.
// Принимаются только изображения; остальное — понятная ошибка в диалоге.
// Загрузку делает вызывающий (upload): успех закрывает диалог, ошибка
// остаётся в нём; пока загрузка идёт, диалог не закрывается.
import { DialogClose, DialogContent, DialogDescription, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import { onBeforeUnmount, ref, watch } from 'vue'

import { errorText, t } from '@/i18n'

import KinIcon from '../KinIcon.vue'

const props = defineProps<{ upload: (file: File) => Promise<void> }>()
const open = defineModel<boolean>('open', { required: true })

/** форматы, которые принимает сервер (F-29) */
const ACCEPT = ['image/jpeg', 'image/png', 'image/webp', 'image/gif']

const input = ref<HTMLInputElement | null>(null)
const busy = ref(false)
const error = ref('')
const over = ref(false)

async function take(file: File | undefined, empty: string) {
  if (busy.value) return
  if (!file) {
    error.value = empty
    return
  }
  if (!ACCEPT.includes(file.type)) {
    error.value = t.photo.notImage
    return
  }
  error.value = ''
  busy.value = true
  try {
    await props.upload(file)
    open.value = false
  } catch (e) {
    error.value = errorText(e)
  } finally {
    busy.value = false
  }
}

/** первый файл из буфера или перетаскивания (картинка — если есть) */
function firstFile(list: FileList | null | undefined): File | undefined {
  const files = [...(list ?? [])]
  return files.find(f => ACCEPT.includes(f.type)) ?? files[0]
}

function onPaste(e: ClipboardEvent) {
  if (!open.value) return
  e.preventDefault()
  void take(firstFile(e.clipboardData?.files), t.photo.noImageInClipboard)
}

function onDrop(e: DragEvent) {
  over.value = false
  void take(firstFile(e.dataTransfer?.files), t.photo.notImage)
}

function onFile(e: Event) {
  const el = e.target as HTMLInputElement
  const file = el.files?.[0]
  el.value = ''
  if (file) void take(file, t.photo.notImage)
}

/** пока файл загружается, диалог не закрыть: результат применится к полю */
function close(v: boolean) {
  if (!v && busy.value) return
  open.value = v
}

// Ctrl+V — пока диалог открыт, где бы ни был фокус внутри него
watch(open, v => {
  error.value = ''
  over.value = false
  if (v) document.addEventListener('paste', onPaste)
  else document.removeEventListener('paste', onPaste)
}, { immediate: true })
onBeforeUnmount(() => document.removeEventListener('paste', onPaste))
</script>

<template>
  <DialogRoot
    :open="open"
    @update:open="close"
  >
    <DialogPortal>
      <DialogOverlay class="kin-overlay" />
      <DialogContent class="kin-dialog image-pick">
        <DialogTitle>{{ t.photo.upload }}</DialogTitle>
        <div
          class="drop"
          :class="{ over, busy }"
          @dragenter.prevent="over = true"
          @dragover.prevent="over = true"
          @dragleave="over = false"
          @drop.prevent="onDrop"
        >
          <KinIcon name="upload" />
          <DialogDescription class="kin-muted hint">
            {{ t.photo.pickHint }}
          </DialogDescription>
          <button
            class="kin-btn choose-file"
            type="button"
            :disabled="busy"
            @click="input?.click()"
          >
            {{ t.photo.chooseFile }}
          </button>
          <input
            ref="input"
            class="file"
            type="file"
            :accept="ACCEPT.join(',')"
            :aria-label="t.photo.chooseFile"
            @change="onFile"
          >
        </div>
        <p
          v-if="error"
          class="kin-error"
          role="alert"
        >
          {{ error }}
        </p>
        <div class="kin-actions">
          <DialogClose
            class="kin-ibtn"
            :disabled="busy"
            :aria-label="t.common.cancel"
            :title="t.common.cancel"
          >
            <KinIcon name="close" />
          </DialogClose>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style scoped>
.drop {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-2);
  align-items: center;
  padding: var(--kin-space-4) var(--kin-space-3);
  text-align: center;
  border: 2px dashed var(--kin-color-border-strong);
  border-radius: 16px;
}

.drop.over {
  background: var(--kin-color-accent-soft);
  border-color: var(--kin-color-accent);
}

.drop.busy {
  opacity: 0.6;
}

.hint {
  margin: 0;
}

.file {
  display: none;
}
</style>
