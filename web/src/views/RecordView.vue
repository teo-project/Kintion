<script setup lang="ts">
// Карточка записи: подпись (с ручной правкой при freeedit, F-71), системные
// даты с подписями из period (F-70), примечание, чувствительность, разделы
// полей main / additional / свои (F-61): скрытые пустые поля (`show: filled`)
// добавляются через «Добавить». Два режима (F-85): чтение и редактирование,
// где поля сразу открыты для ввода; режим карточек аккаунта (F-108): «По
// месту» — ручки у полей, «Форма» — карточка сразу в правке. Сохранение — по
// полю (F-45, ui/record.ts).
import {
  PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger, TabsList, TabsRoot, TabsTrigger,
} from 'reka-ui'
import { computed, reactive, ref, toRef, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

import { api } from '@/api/client'
import AuditList from '@/components/AuditList.vue'
import GroupsRow from '@/components/card/GroupsRow.vue'
import NoteEditor from '@/components/card/NoteEditor.vue'
import SystemDate from '@/components/card/SystemDate.vue'
import TitleEditor from '@/components/card/TitleEditor.vue'
import CustomFieldEditor from '@/components/field/CustomFieldEditor.vue'
import FieldRow from '@/components/field/FieldRow.vue'
import HistoryChoiceDialog from '@/components/field/HistoryChoiceDialog.vue'
import HistoryPopover from '@/components/field/HistoryPopover.vue'
import KinCheckbox from '@/components/KinCheckbox.vue'
import KinIcon from '@/components/KinIcon.vue'

import ParticipantsBlock from '@/components/participants/ParticipantsBlock.vue'
import QuickAddMenu from '@/components/quickadd/QuickAddMenu.vue'
import RelationsBlock from '@/components/relations/RelationsBlock.vue'
import RecordAvatar from '@/components/RecordAvatar.vue'
import { isEmptyDate } from '@/core/dates'
import type { FieldView, SectionView } from '@/core/card'
import { STAGE } from '@/core/resolver'
import { errorText, t } from '@/i18n'
import { rememberDashboard } from '@/stores/dashboards'
import { provideRecord, useRecordState } from '@/ui/record'

/** embedded — карточка поверх дерева (F-113): без «Назад» */
const props = defineProps<{ d: string; id: string; embedded?: boolean }>()

/** ключ версии чувствительности (план §4) */
const SENSITIVE = 'sensitive'

const router = useRouter()
// черновик (F-88) стал записью — её адрес вместо адреса черновика
const s = useRecordState(toRef(props, 'd'), toRef(props, 'id'), id => {
  void router.replace({ name: 'record', params: { d: props.d, id } })
})
provideRecord(s)

/**
 * «Назад» (F-101): туда, откуда пришёл, — по истории роутера (`history.state.back`);
 * истории нет (прямой заход по адресу) — в список.
 */
function goBack(e: MouseEvent, toList: (e?: MouseEvent) => unknown) {
  if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return
  const state: unknown = window.history.state
  if (state && typeof state === 'object' && 'back' in state && typeof state.back === 'string') {
    e.preventDefault()
    router.back()
    return
  }
  void toList(e)
}

/** поля, открытые через «Добавить» (до первого значения) */
const revealed = reactive(new Set<string>())
/** системные даты в revealed и addOpen — с префиксом, чтобы не спутать с полями */
const DATE_PREFIX = 'period:'
const addOpen = ref<Record<string, boolean>>({})

/** вкладка: карточка или её изменения (T25) */
const tab = ref<'card' | 'changes'>('card')

watch(() => [props.d, props.id], () => {
  revealed.clear()
  tab.value = 'card'
  rememberDashboard(props.d)
  void s.load()
}, { immediate: true })

const card = s.card

/** Порядок полей раздела — как в шаблоне; открытые через «Добавить» — на своём месте. */
function shownFields(sec: SectionView): FieldView[] {
  const extra = sec.addable.filter(f => revealed.has(f.key))
  if (!extra.length) return sec.fields
  const order = (f: FieldView) => {
    const i = card.value?.template?.fields?.findIndex(x => !f.custom && x.key === f.def.key) ?? -1
    return i < 0 ? Number.MAX_SAFE_INTEGER : i
  }
  return [...sec.fields, ...extra].sort((a, b) => order(a) - order(b))
}

function hiddenFields(sec: SectionView): FieldView[] {
  return sec.addable.filter(f => !revealed.has(f.key))
}

/** системные даты (F-99): пустые с `show: filled` — за «Добавить», пока не открыты */
const shownDates = computed(() => card.value?.dates.filter(x => !x.hidden || revealed.has(DATE_PREFIX + x.key)) ?? [])
const hiddenDates = computed(() => card.value?.dates.filter(x => x.hidden && !revealed.has(DATE_PREFIX + x.key)) ?? [])

/** «Умер» (F-126): при заполненной дате конца не нужен; пустой — за той же кнопкой «Добавить дату» */
const FLAG_KEY = DATE_PREFIX + 'end_flag'
/** по записи, а не по показанным датам: шаблон мог скрыть конец, а значение у записи есть */
const endFilled = computed(() => {
  const e = s.rec.value?.end
  return !!e && (!!e.link || !isEmptyDate(e.value))
})
const endFlag = computed(() => (card.value?.endFlag && !endFilled.value ? card.value.endFlag : null))
/** пустой признак скрыт при любом `show` пака: он появляется за «Добавить дату» (F-126) */
const flagShown = computed(() => !!endFlag.value && (!endFlag.value.empty || revealed.has(FLAG_KEY)))
/** что предлагает «Добавить дату»: скрытые даты и скрытый признак завершения */
const dateAdds = computed(() => [
  ...hiddenDates.value.map(x => ({ key: x.key as string, label: x.label })),
  ...(endFlag.value && !flagShown.value ? [{ key: 'end_flag', label: endFlag.value.def.name }] : []),
])

function revealDate(x: { key: string }) {
  revealed.add(DATE_PREFIX + x.key)
  addOpen.value = { ...addOpen.value, [DATE_PREFIX]: false }
}

function reveal(sec: SectionView, f: FieldView) {
  revealed.add(f.key)
  addOpen.value = { ...addOpen.value, [sec.key]: false }
}

/** запись может участвовать в связях — показать «Где участвует» */
const participant = computed(() => !!card.value && s.reg.value?.entities.get(card.value.entity)?.participant === true)

const sections = computed(() => card.value?.sections.filter(sec => sec.fields.length || sec.addable.length) ?? [])
/** основные поля — в шапке карточки (F-90), остальные разделы — отдельными островами */
const headSections = computed(() => sections.value.filter(sec => sec.key === 'main'))
const restSections = computed(() => sections.value.filter(sec => sec.key !== 'main'))
/** «Связи» — правая колонка */
const showRelations = computed(() => !s.draft.value && (participant.value || !!card.value?.relations.length))

const trashBusy = ref(false)
const trashError = ref('')

/** в корзину и обратно (F-44): запись не теряется, очистка — в корзине */
async function trash(restore: boolean) {
  const rec = s.rec.value
  if (!rec) return
  trashBusy.value = true
  trashError.value = ''
  try {
    s.apply(await (restore ? api.restoreRecord(rec.id) : api.deleteRecord(rec.id)))
  } catch (e) {
    trashError.value = errorText(e)
  } finally {
    trashBusy.value = false
  }
}

async function toggleSensitive(v: boolean) {
  const rec = s.rec.value
  if (!rec) return
  await s.save(SENSITIVE, rev => api.patchRecord(rec.id, { if_rev: rev, sensitive: v }))
}
</script>

<template>
  <main
    class="record kin-page"
    :class="{ embedded }"
  >
    <div
      v-if="!embedded"
      class="kin-page-head"
    >
      <RouterLink
        v-slot="{ href, navigate }"
        :to="{ name: 'dashboard', params: { d }, query: card ? { e: card.entity } : {} }"
        custom
      >
        <a
          class="kin-ibtn kin-ibtn--ghost back"
          :href="href"
          :aria-label="t.card.back"
          :title="t.card.back"
          @click="e => goBack(e, navigate)"
        >
          <KinIcon name="back" />
        </a>
      </RouterLink>
    </div>
    <p
      v-if="s.error.value"
      class="kin-notice kin-notice--error"
      role="alert"
    >
      {{ s.error.value }}
    </p>
    <p
      v-if="s.notice.value"
      class="kin-notice conflict"
      role="status"
    >
      {{ s.notice.value }}
    </p>
    <div
      v-if="card && s.rec.value"
      class="kin-columns"
      :class="{ single: !showRelations || tab === 'changes' }"
    >
      <div class="kin-stack">
        <section class="kin-island head-island">
          <header class="head kin-hover-row">
            <div class="photo">
              <RecordAvatar
                :dashboard="d"
                :photo="card.photo"
                :title="card.title"
                :size="256"
                place="card"
              />
            </div>
            <div class="head-main">
              <div class="title-row">
                <div class="title-box">
                  <h1 class="title">
                    {{ card.title }}
                    <TitleEditor
                      v-if="card.freeedit && s.editing.value && !s.draft.value"
                      :label="card.labels.title"
                    />
                  </h1>
                  <p class="kin-muted meta">
                    {{ card.template?.name ?? s.rec.value.template }}
                  </p>
                </div>
                <div class="head-actions">
                  <button
                    v-if="s.writable.value && !s.draft.value"
                    class="kin-ibtn mode"
                    :class="{ 'kin-ibtn--primary': s.editing.value }"
                    type="button"
                    :aria-pressed="s.editing.value"
                    :aria-label="s.editing.value ? t.card.done : t.card.edit"
                    :title="s.editing.value ? t.card.done : t.card.edit"
                    @click="s.editing.value = !s.editing.value"
                  >
                    <KinIcon :name="s.editing.value ? 'check' : 'edit'" />
                  </button>
                  <QuickAddMenu
                    v-if="!s.draft.value"
                    :dashboard="d"
                  />
                </div>
              </div>
              <p
                v-if="s.rec.value.deleted"
                class="kin-notice kin-notice--error deleted"
              >
                <span>{{ t.card.deletedRecord }}</span>
                <button
                  class="kin-ibtn restore"
                  type="button"
                  :disabled="trashBusy"
                  :aria-label="t.trash.restore"
                  :title="t.trash.restore"
                  @click="trash(true)"
                >
                  <KinIcon name="restore" />
                </button>
              </p>
              <p
                v-if="trashError"
                class="kin-error"
                role="alert"
              >
                {{ trashError }}
              </p>
              <p
                v-if="s.draft.value"
                class="kin-notice draft-hint"
                role="status"
              >
                {{ t.card.draftHint }}
              </p>
              <TabsRoot
                v-else
                :model-value="tab"
                @update:model-value="v => (tab = v === 'changes' ? 'changes' : 'card')"
              >
                <TabsList class="kin-tabs tabs">
                  <TabsTrigger
                    class="kin-tab"
                    value="card"
                  >
                    {{ t.audit.cardTab }}
                  </TabsTrigger>
                  <TabsTrigger
                    class="kin-tab"
                    value="changes"
                  >
                    {{ t.audit.tab }}
                  </TabsTrigger>
                </TabsList>
              </TabsRoot>
              <template v-if="tab === 'card'">
                <p
                  v-if="!card.template"
                  class="kin-notice"
                >
                  {{ t.card.inactiveTemplate }}
                </p>
                <div class="basics">
                  <GroupsRow
                    :label="card.labels.groups"
                    :dashboard="d"
                  />
                  <NoteEditor
                    :label="card.labels.note"
                    :dashboard="d"
                  />
                  <SystemDate
                    v-for="item in shownDates"
                    :key="item.key"
                    :item="item"
                    :dashboard="d"
                    :start-editing="revealed.has(DATE_PREFIX + item.key) && !item.value"
                  />
                  <FieldRow
                    v-if="flagShown && endFlag"
                    :field="endFlag"
                    :dashboard="d"
                    :start-editing="revealed.has(FLAG_KEY) && endFlag.empty"
                  />
                  <PopoverRoot
                    v-if="dateAdds.length && (s.editing.value || s.inlineMode.value)"
                    :open="addOpen[DATE_PREFIX] ?? false"
                    @update:open="v => (addOpen = { ...addOpen, [DATE_PREFIX]: v })"
                  >
                    <PopoverTrigger
                      class="kin-ibtn kin-ibtn--dashed add-date"
                      :aria-label="t.card.addDate"
                      :title="t.card.addDate"
                    >
                      <KinIcon name="plus" />
                    </PopoverTrigger>
                    <PopoverPortal>
                      <PopoverContent
                        class="kin-popover"
                        side="bottom"
                        align="start"
                        :side-offset="4"
                      >
                        <ul class="add-list">
                          <li
                            v-for="x in dateAdds"
                            :key="x.key"
                          >
                            <button
                              class="kin-btn kin-btn--small"
                              type="button"
                              @click="revealDate(x)"
                            >
                              <KinIcon name="plus" />
                              {{ x.label }}
                            </button>
                          </li>
                        </ul>
                      </PopoverContent>
                    </PopoverPortal>
                  </PopoverRoot>
                  <div
                    v-if="card.stage"
                    class="stage-row kin-hover-row"
                  >
                    <span class="kin-muted label">{{ card.stage.label }}</span>
                    <span class="stage-value">{{ card.stage.current?.text || t.common.empty }}</span>
                    <HistoryPopover
                      :field-key="STAGE"
                      :label="card.stage.label"
                      :def="{ type: 'choice', name: card.stage.label }"
                      :views="card.stage.values"
                      :stages="card.template?.stages ?? []"
                      :dashboard="d"
                    />
                  </div>
                </div>
              </template>
            </div>
          </header>
          <template v-if="tab === 'card'">
            <section
              v-for="sec in headSections"
              :key="sec.key"
              class="block head-block"
            >
              <h2 class="kin-section-title section-title">
                {{ sec.name }}
              </h2>
              <FieldRow
                v-for="f in shownFields(sec)"
                :key="f.key"
                :field="f"
                :dashboard="d"
                :start-editing="revealed.has(f.key) && f.empty"
              >
                <template
                  v-if="f.custom && !s.rec.value.deleted && (s.editing.value || s.inlineMode.value)"
                  #tools
                >
                  <CustomFieldEditor
                    :def="f.def"
                    :shared="f.shared"
                  />
                </template>
              </FieldRow>
              <PopoverRoot
                v-if="hiddenFields(sec).length && (s.editing.value || s.inlineMode.value)"
                :open="addOpen[sec.key] ?? false"
                @update:open="v => (addOpen = { ...addOpen, [sec.key]: v })"
              >
                <PopoverTrigger
                  class="kin-ibtn kin-ibtn--dashed add-field"
                  :aria-label="t.card.addField"
                  :title="t.card.addField"
                >
                  <KinIcon name="plus" />
                </PopoverTrigger>
                <PopoverPortal>
                  <PopoverContent
                    class="kin-popover"
                    side="bottom"
                    align="start"
                    :side-offset="4"
                  >
                    <ul class="add-list">
                      <li
                        v-for="f in hiddenFields(sec)"
                        :key="f.key"
                      >
                        <button
                          class="kin-btn kin-btn--small"
                          type="button"
                          @click="reveal(sec, f)"
                        >
                          <KinIcon name="plus" />
                          {{ f.name }}
                        </button>
                      </li>
                    </ul>
                  </PopoverContent>
                </PopoverPortal>
              </PopoverRoot>
            </section>
          </template>
        </section>

        <section
          v-if="tab === 'changes'"
          class="kin-island"
        >
          <AuditList
            :dashboard="d"
            :entity="s.rec.value.id"
          />
        </section>
        <template v-else>
          <section
            v-if="card.roles.length"
            class="kin-island block"
          >
            <h2 class="kin-section-title section-title">
              {{ t.participants.title }}
            </h2>
            <ParticipantsBlock
              :roles="card.roles"
              :dashboard="d"
            />
          </section>

          <section
            v-for="sec in restSections"
            :key="sec.key"
            class="kin-island block"
          >
            <h2 class="kin-section-title section-title">
              {{ sec.name }}
            </h2>
            <FieldRow
              v-for="f in shownFields(sec)"
              :key="f.key"
              :field="f"
              :dashboard="d"
              :start-editing="revealed.has(f.key) && f.empty"
            >
              <template
                v-if="f.custom && !s.rec.value.deleted && (s.editing.value || s.inlineMode.value)"
                #tools
              >
                <CustomFieldEditor
                  :def="f.def"
                  :shared="f.shared"
                />
              </template>
            </FieldRow>
            <PopoverRoot
              v-if="hiddenFields(sec).length && (s.editing.value || s.inlineMode.value)"
              :open="addOpen[sec.key] ?? false"
              @update:open="v => (addOpen = { ...addOpen, [sec.key]: v })"
            >
              <PopoverTrigger
                class="kin-ibtn kin-ibtn--dashed add-field"
                :aria-label="t.card.addField"
                :title="t.card.addField"
              >
                <KinIcon name="plus" />
              </PopoverTrigger>
              <PopoverPortal>
                <PopoverContent
                  class="kin-popover"
                  side="bottom"
                  align="start"
                  :side-offset="4"
                >
                  <ul class="add-list">
                    <li
                      v-for="f in hiddenFields(sec)"
                      :key="f.key"
                    >
                      <button
                        class="kin-btn kin-btn--small"
                        type="button"
                        @click="reveal(sec, f)"
                      >
                        <KinIcon name="plus" />
                        {{ f.name }}
                      </button>
                    </li>
                  </ul>
                </PopoverContent>
              </PopoverPortal>
            </PopoverRoot>
          </section>

          <div
            v-if="!s.rec.value.deleted && !s.draft.value"
            class="card-foot"
          >
            <CustomFieldEditor
              v-if="s.editing.value || s.inlineMode.value"
              @added="key => revealed.add(key)"
            />
            <KinCheckbox
              v-if="s.editing.value"
              class="kin-muted sensitive"
              :model-value="s.rec.value.sensitive"
              :disabled="s.saving[SENSITIVE]"
              @update:model-value="toggleSensitive"
            >
              {{ card.labels.sensitive }}
            </KinCheckbox>
            <span class="kin-spacer" />
            <button
              class="kin-ibtn kin-ibtn--danger to-trash"
              type="button"
              :disabled="trashBusy"
              :aria-label="t.trash.toTrash"
              :title="t.trash.toTrash"
              @click="trash(false)"
            >
              <KinIcon name="trash" />
            </button>
          </div>
          <p
            v-if="s.fieldErrors[SENSITIVE]"
            class="kin-error"
            role="alert"
          >
            {{ s.fieldErrors[SENSITIVE] }}
          </p>
        </template>
      </div>
      <aside
        v-if="showRelations && tab === 'card'"
        class="kin-stack"
      >
        <section class="kin-island block relations-island">
          <RelationsBlock
            :relations="card.relations"
            :dashboard="d"
          />
        </section>
      </aside>
    </div>
    <p
      v-else-if="!s.error.value"
      class="kin-muted"
    >
      {{ t.common.loading }}
    </p>
    <HistoryChoiceDialog />
  </main>
</template>

<style scoped>
.kin-columns.single {
  grid-template-columns: minmax(0, 1fr);
}

.head {
  display: flex;
  gap: 22px;
  align-items: flex-start;
}

.photo {
  flex: none;
}

.photo :deep(.avatar) {
  width: 168px;
  height: 224px;
  object-fit: cover;
  border-radius: 14px;
}

.photo :deep(.avatar.rect) {
  width: auto;
  max-width: 168px;
  height: auto;
  max-height: 224px;
  object-fit: contain;
}

.photo :deep(.placeholder) {
  box-sizing: border-box;
  font-size: 3rem;
  border: 1px dashed var(--kin-color-border-strong);
  border-radius: 14px;
}

.head-main {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}

.title-row {
  display: flex;
  gap: var(--kin-space-3);
  align-items: flex-start;
}

.title-box {
  flex: 1;
  min-width: 0;
}

.title {
  margin: 0;
  font-size: 1.75rem;
  font-weight: 800;
  letter-spacing: -0.02em;
  line-height: 1.2;
  overflow-wrap: anywhere;
}

.meta {
  margin: 2px 0 0;
  font-size: var(--kin-font-tiny);
  font-weight: 600;
}

.head-actions {
  display: flex;
  gap: 6px;
  align-items: center;
}

.deleted {
  display: flex;
  gap: var(--kin-space-2);
  align-items: center;
  justify-content: space-between;
}

.basics {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-top: 12px;
  border-top: 1px solid var(--kin-color-border);
}

.head-block {
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid var(--kin-color-border);
}

.stage-row {
  display: flex;
  gap: var(--kin-space-2);
  align-items: baseline;
}

.stage-value {
  font-weight: 600;
}

.label {
  flex: none;
  width: 130px;
  font-size: var(--kin-font-tiny);
}

.add-field,
.add-date {
  margin-top: var(--kin-space-2);
}

.add-list {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}

.card-foot {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kin-space-3);
  align-items: center;
}

.sensitive {
  font-size: var(--kin-font-tiny);
}

@media (max-width: 640px) {
  .head {
    flex-direction: column;
    gap: var(--kin-space-3);
    align-items: stretch;
  }

  .photo {
    align-self: flex-start;
  }

  .photo :deep(.avatar) {
    width: 96px;
    height: 128px;
  }

  .photo :deep(.placeholder) {
    font-size: 2rem;
  }
}
</style>
