<script setup lang="ts">
// Свой блок карточки (F-139): переименовать или убрать во всплывающей форме.
// Блок записи — список custom_sections записи (версия — как у своих полей);
// общий блок шаблона — список шаблона в дашборде с версией шаблона (F-45).
// Блок, в котором есть поля, не убирается — у общего блока поля могут быть
// у других записей шаблона, это проверяет сервер.
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { computed, ref } from 'vue'

import { api } from '@/api/client'
import type { CustomSectionDef } from '@/core/types'
import { fmt, t } from '@/i18n'
import { withSection } from '@/ui/customFields'
import { SHARED_REV, useRecord } from '@/ui/record'
import KinIcon from '../KinIcon.vue'

const props = defineProps<{
  def: CustomSectionDef
  /** общий блок шаблона в дашборде */
  shared: boolean
  /** в блоке есть поля этой карточки */
  used: boolean
}>()

/** ключ версии своих полей и блоков записи */
const REV = 'custom_fields'

const s = useRecord()
const open = ref(false)
const name = ref('')

const tplDef = computed(() => {
  const row = s.rec.value?.template_id
  return row ? s.reg.value?.byRowId.get(row)?.definition : undefined
})
const errKey = computed(() => (props.shared ? SHARED_REV : REV))

function onOpen(v: boolean) {
  open.value = v
  if (!v) return
  s.fieldErrors[REV] = undefined
  s.fieldErrors[SHARED_REV] = undefined
  name.value = props.def.name
}

async function save(remove = false) {
  const rec = s.rec.value
  if (!rec || (!remove && !name.value.trim())) return
  const next = (list: CustomSectionDef[]) =>
    withSection(list, props.def.key, remove ? null : { key: props.def.key, name: name.value.trim() })
  let ok: boolean
  if (props.shared) {
    ok = await s.saveShared(tplDef.value?.custom_fields ?? [], next(tplDef.value?.custom_sections ?? []))
  } else {
    const sections = next(rec.custom_sections ?? [])
    ok = await s.save(REV, rev => api.patchRecord(rec.id, { if_rev: rev, custom_sections: sections }))
  }
  if (ok) open.value = false
}
</script>

<template>
  <PopoverRoot
    :open="open"
    @update:open="onOpen"
  >
    <PopoverTrigger
      class="kin-icon-btn kin-semi section-edit"
      :aria-label="fmt(t.custom.sectionEdit, { name: def.name })"
      :title="fmt(t.custom.sectionEdit, { name: def.name })"
    >
      <KinIcon name="settings" />
    </PopoverTrigger>
    <PopoverPortal>
      <PopoverContent
        class="kin-popover"
        side="bottom"
        align="start"
        :side-offset="4"
      >
        <form
          class="form"
          @submit.prevent="save()"
        >
          <strong>{{ fmt(t.custom.sectionEdit, { name: def.name }) }}</strong>
          <span
            v-if="shared"
            class="kin-muted"
          >{{ fmt(t.custom.sectionShared, { name: tplDef?.name ?? '' }) }}</span>
          <label class="kin-label">
            {{ t.custom.sectionName }}
            <input
              v-model="name"
              class="kin-input"
              name="section-name"
              required
              maxlength="200"
            >
          </label>
          <p
            v-if="s.fieldErrors[errKey]"
            class="kin-error"
            role="alert"
          >
            {{ s.fieldErrors[errKey] }}
          </p>
          <div class="kin-actions">
            <button
              class="kin-ibtn kin-ibtn--danger"
              type="button"
              :disabled="s.saving[errKey] || used"
              :title="used ? t.custom.sectionRemoveHint : t.custom.sectionRemove"
              :aria-label="t.custom.sectionRemove"
              @click="save(true)"
            >
              <KinIcon name="trash" />
            </button>
            <button
              class="kin-ibtn kin-ibtn--primary"
              type="submit"
              :disabled="s.saving[errKey] || !name.trim()"
              :aria-label="t.common.save"
              :title="t.common.save"
            >
              <KinIcon name="check" />
            </button>
          </div>
        </form>
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-2);
  min-width: 16rem;
}
</style>
