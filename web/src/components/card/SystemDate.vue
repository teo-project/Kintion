<script setup lang="ts">
// Системная дата записи (F-70): `start` / `end` с подписью из period
// шаблона или сущности. В режиме чтения — только значение; в режиме
// редактирования (F-85) дата сразу открыта для ввода, привязка к записи-
// событию — во всплывающей форме (F-69). Конец привязывается только
// к шаблонам `end_events` (этапа или шаблона; нет ключа — к любым, F-57, Д-28).
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { computed, nextTick, ref, watch } from 'vue'

import { api } from '@/api/client'
import type { DateView } from '@/core/card'
import { validateDate, type PartialDate } from '@/core/dates'
import { anchorEntities, endEvents } from '@/core/participants'
import { recordTitleById, tplOf, type Ctx } from '@/core/resolver'
import type { RecordDTO } from '@/core/types'
import { t } from '@/i18n'
import { useRecord, type SaveResult } from '@/ui/record'

import EditCell from '../field/EditCell.vue'

import PartialDateInput from '../PartialDateInput.vue'
import RecordPicker from '../RecordPicker.vue'
import KinIcon from '../KinIcon.vue'

const props = defineProps<{ item: DateView; dashboard: string; startEditing?: boolean }>()

const s = useRecord()
/** «По месту» (F-108): дата открыта для ввода двойным кликом или ручкой */
const single = ref(false)
const row = ref<HTMLElement | null>(null)

async function startSingle() {
  if (!s.inlineMode.value) return
  single.value = true
  await nextTick()
  row.value?.querySelector<HTMLElement>('.value-cell input, .value-cell select')?.focus()
}

watch(() => props.startEditing, v => {
  if (v) void startSingle()
}, { immediate: true })

watch(() => s.inlineMode.value, v => {
  if (!v) single.value = false
})

/** сохранено (или без изменений) и фокус ушёл — снова обычный вид */
function onSettled(res: SaveResult) {
  if (!single.value || (res !== 'ok' && res !== 'cancel')) return
  setTimeout(() => {
    if (!row.value?.querySelector('.value-cell')?.contains(document.activeElement)) single.value = false
  })
}
const open = ref(false)
const draft = ref<PartialDate | null>(null)
const link = ref<string | null>(null)
const picking = ref(false)

watch(open, v => {
  if (!v) return
  draft.value = props.item.value
  link.value = props.item.link?.id ?? null
  picking.value = false
})

const allowed = computed(() =>
  props.item.key === 'end' && s.ctx.value && s.rec.value ? endEvents(s.ctx.value, s.rec.value) : undefined)

function accept(cand: RecordDTO, ctx: Ctx): boolean {
  const ids = allowed.value
  if (!ids) return true
  const id = tplOf(ctx, cand)?.id
  return !!id && ids.includes(id)
}

function linkTitle(id: string): string {
  return s.ctx.value ? recordTitleById(s.ctx.value, id) : id
}

function pick(id: string) {
  link.value = id
  picking.value = false
}

/** Дата из поля ввода; привязка остаётся прежней. */
function saveDate(v: unknown): Promise<SaveResult> {
  const rec = s.rec.value
  if (!rec) return Promise.resolve('error')
  const key = props.item.key
  const value = (v as PartialDate | null) ?? null
  if (s.draft.value) {
    if (value) return s.createWith(key, { [key]: { value } })
    s.clearDraftValue(key)
    return Promise.resolve('ok')
  }
  const link = props.item.link?.id ?? null
  const period = value || link ? { value, link } : null
  return s.saveField(key, rev => api.patchRecord(rec.id, { if_rev: rev, [key]: period }))
}

async function save() {
  const rec = s.rec.value
  if (!rec || (draft.value && validateDate(draft.value))) return
  const key = props.item.key
  const period = draft.value || link.value ? { value: draft.value, link: link.value } : null
  const ok = await s.save(key, rev => api.patchRecord(rec.id, { if_rev: rev, [key]: period }))
  if (ok || !s.fieldErrors[key]) open.value = false
}
</script>

<template>
  <div
    ref="row"
    class="sys-row kin-hover-row"
  >
    <span class="kin-muted label">{{ item.label }}</span>
    <EditCell
      v-if="s.editing.value || single"
      class="value-cell"
      :def="{ type: 'partial_date', name: item.label }"
      :value="item.value"
      :dashboard="dashboard"
      :save="saveDate"
      @settled="onSettled"
      @escape="single = false"
    />
    <span
      v-else
      class="value"
      @dblclick="startSingle"
    >{{ item.text || t.common.empty }}</span>
    <button
      v-if="s.inlineMode.value && !single"
      class="kin-icon-btn kin-semi handle"
      type="button"
      :aria-label="`${t.common.edit}: ${item.label}`"
      :title="t.common.edit"
      @click="startSingle"
    >
      <KinIcon name="edit" />
    </button>
    <span
      v-if="item.link"
      class="kin-muted link"
      :title="`${t.card.link}: ${item.link.title}`"
    ><KinIcon name="link" /></span>
    <PopoverRoot
      v-if="s.editing.value && !s.draft.value"
      v-model:open="open"
    >
      <PopoverTrigger
        class="kin-icon-btn kin-semi"
        :aria-label="`${t.card.link}: ${item.label}`"
        :title="t.card.link"
      >
        <KinIcon name="link" />
      </PopoverTrigger>
      <PopoverPortal>
        <PopoverContent
          class="kin-popover"
          side="bottom"
          align="start"
          :side-offset="4"
        >
          <form
            class="kin-form"
            @submit.prevent="save"
          >
            <PartialDateInput
              v-model="draft"
              :label="item.label"
            />
            <div class="kin-label">
              {{ t.card.link }}
              <div class="kin-row">
                <span>{{ link ? linkTitle(link) : t.card.linkNone }}</span>
                <button
                  v-if="link"
                  class="kin-icon-btn"
                  type="button"
                  :aria-label="t.common.delete"
                  :title="t.common.delete"
                  @click="link = null"
                >
                  <KinIcon name="close" />
                </button>
                <button
                  class="kin-ibtn"
                  type="button"
                  :aria-label="t.card.refPick"
                  :title="t.card.refPick"
                  @click="picking = !picking"
                >
                  <KinIcon name="search" />
                </button>
              </div>
              <RecordPicker
                v-if="picking && s.reg.value && s.rec.value"
                :dashboard="dashboard"
                :reg="s.reg.value"
                :exclude="[s.rec.value.id]"
                :entities="anchorEntities(s.reg.value)"
                :accept="allowed ? accept : undefined"
                @pick="pick"
              />
            </div>
            <p
              v-if="s.fieldErrors[item.key]"
              class="kin-error"
              role="alert"
            >
              {{ s.fieldErrors[item.key] }}
            </p>
            <div class="kin-actions">
              <button
                class="kin-ibtn"
                type="button"
                :aria-label="t.common.cancel"
                :title="t.common.cancel"
                @click="open = false"
              >
                <KinIcon name="close" />
              </button>
              <button
                class="kin-ibtn kin-ibtn--primary"
                type="submit"
                :disabled="s.saving[item.key]"
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
  </div>
</template>

<style scoped>
.sys-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kin-space-2) 12px;
  align-items: baseline;
}

.label {
  flex: none;
  width: 130px;
  font-size: var(--kin-font-tiny);
}

.value {
  font-weight: 600;
}

.link {
  display: inline-flex;
  align-self: center;
}

.link {
  font-size: var(--kin-font-small);
}
</style>
