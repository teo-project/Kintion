<script setup lang="ts">
// Быстрая форма (+) (F-73, F-74): поля — `quick_fields` шаблона по умолчанию
// новой записи (код ключей не знает), предзаполнение — из `new` пункта;
// значения `new`, которых нет в форме, пишутся без показа (пол не правится).
// Сохранение — одна пакетная операция: новая запись, отношение, участия
// и этап (F-42, F-43). «В карточке» — то же как черновик полной карточки
// (F-88): запись и связь появятся с первым сохранённым полем.
import { DialogClose, DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import { computed, reactive, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'

import { api } from '@/api/client'
import { validateDate, type PartialDate } from '@/core/dates'
import { buildDraft, draftBatch, existingBatch, existingSide, fitsExisting, NEW_REF, type QuickAddOption, type QuickDraft } from '@/core/quickAdd'
import type { Ctx } from '@/core/resolver'
import { personView, treeInput } from '@/core/tree/graph'
import { layoutTree } from '@/core/tree/layout'
import { suggestExisting } from '@/core/tree/suggest'
import type { RecordDTO } from '@/core/types'
import { makeCtx } from '@/ui/ctx'
import { errorText, t } from '@/i18n'
import { DRAFT_REF, newDraft } from '@/ui/drafts'
import { useRecord } from '@/ui/record'
import { isDateLike } from '@/core/resolver'


import ValueEditor from '../field/ValueEditor.vue'
import GroupPicker from '../GroupPicker.vue'
import PartialDateInput from '../PartialDateInput.vue'
import KinIcon from '../KinIcon.vue'

/** treeGroup — фильтр группы дерева: подсказки по тому же дереву, новая запись — в эту группу */
const props = defineProps<{ option: QuickAddOption; dashboard: string; treeGroup?: string }>()
const open = defineModel<boolean>('open', { required: true })
/** created — id новой записи (дерево центрируется на ней, F-129) */
const emit = defineEmits<{ created: [id: string] }>()

const s = useRecord()
const dev = import.meta.env.DEV

const entity = ref('')
const draft = shallowRef<QuickDraft | null>(null)
const values = reactive<Record<string, unknown>>({})
const groups = ref<string[]>([])
const error = ref('')
const busy = ref(false)

function rebuild() {
  error.value = ''
  draft.value = null
  for (const k of Object.keys(values)) delete values[k]
  const ctx = s.ctx.value
  const rec = s.rec.value
  if (!ctx || !rec) return
  try {
    const d = buildDraft(ctx, rec, props.option, entity.value || undefined)
    draft.value = d
    entity.value = d.entity
    Object.assign(values, d.values)
  } catch (e) {
    error.value = errorText(e)
  }
}

// ---------- «Новый / Существующий» (F-130) ----------

const mode = ref<'new' | 'existing'>('new')
/** выбранная существующая запись */
const picked = ref<string | null>(null)
const search = ref('')
interface Pick { id: string; title: string }
const found = shallowRef<Pick[]>([])
const suggested = shallowRef<Pick[]>([])
let suggestSeq = 0
/** полное имя — как в узле дерева (`tree_title`, иначе `title`; F-123): по нему и поиск */
const fullName = (ctx: Ctx, r: RecordDTO) => personView(ctx, r).title

watch([open, () => props.option], ([v]) => {
  if (!v) return
  entity.value = ''
  groups.value = props.treeGroup ? [props.treeGroup] : []
  mode.value = 'new'
  picked.value = null
  search.value = ''
  found.value = []
  suggested.value = []
  suggestSeq++
  rebuild()
}, { immediate: true })

/** подсказки по дереву: дети партнёра, ближайшие в нужную сторону, два шага по связям */
async function loadSuggested() {
  const rec = s.rec.value
  const reg = s.reg.value
  const side = existingSide(props.option)
  const my = ++suggestSeq
  suggested.value = []
  if (!rec || !reg || !side) return
  const want = entity.value
  try {
    // дерево — с тем же фильтром группы, что на экране (из дерева)
    const tree = await api.tree(props.dashboard, props.treeGroup || undefined)
    if (my !== suggestSeq) return
    const ctx = makeCtx(reg, [...tree.relations, ...tree.records])
    const input = treeInput(ctx, tree.relations, tree.records)
    const ids = suggestExisting(input, layoutTree(input, { depth: 99 }), rec.id, side)
    suggested.value = ids
      .map(id => ctx.records[id])
      .filter((r): r is RecordDTO => !!r && r.entity === want && fitsExisting(ctx, props.option, r))
      .map(r => ({ id: r.id, title: fullName(ctx, r) }))
  } catch {
    if (my === suggestSeq) suggested.value = []
  }
}

/** слова запроса — без регистра; совпадение — все слова в подписи (ФИО) записи */
const words = (q: string) => q.toLocaleLowerCase().split(/\s+/).filter(Boolean)

const SEARCH_PAGE = 100
const SEARCH_SHOW = 20
let searchSeq = 0
/** поиск идёт — «никого не найдено» не показывать */
const searching = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | undefined
// поиск по ФИО среди видимых записей сущности (F-07): сервер ищет по самому
// длинному слову, остальное — по подписи записи (примечания и прочие поля не в счёт)
watch([search, mode, entity], () => {
  clearTimeout(searchTimer)
  picked.value = null
  // прежние результаты — не к новому запросу
  found.value = []
  const q = words(search.value)
  const my = ++searchSeq
  searching.value = false
  if (mode.value !== 'existing' || !q.length) return
  searching.value = true
  searchTimer = setTimeout(async () => {
    const reg = s.reg.value
    if (!reg) {
      searching.value = false
      return
    }
    const longest = [...q].sort((x, y) => y.length - x.length)[0]!
    try {
      // страницами, пока не наберётся достаточно совпадений по всем словам
      const out: Pick[] = []
      for (let offset = 0; out.length < SEARCH_SHOW; offset += SEARCH_PAGE) {
        const page = await api.records(props.dashboard, { entity: entity.value, q: longest, limit: SEARCH_PAGE, offset })
        if (my !== searchSeq) return
        const ctx = makeCtx(reg, page.items, page.related)
        out.push(...page.items
          .filter(r => r.id !== s.rec.value?.id)
          .map(r => ({ id: r.id, title: fullName(ctx, r) }))
          .filter(c => q.every(w => c.title.toLocaleLowerCase().includes(w))))
        if (offset + SEARCH_PAGE >= page.total) break
      }
      found.value = out.slice(0, SEARCH_SHOW)
    } catch (e) {
      if (my === searchSeq) error.value = errorText(e)
    } finally {
      if (my === searchSeq) searching.value = false
    }
  }, 250)
})

// другая сущность — подсказки заново
watch(entity, () => {
  suggestSeq++
  suggested.value = []
  if (mode.value === 'existing') void loadSuggested()
})

function setMode(m: 'new' | 'existing') {
  mode.value = m
  picked.value = null
  error.value = ''
  if (m === 'existing' && !suggested.value.length) void loadSuggested()
}

/** список выбора: найденные, а без запроса — подсказки */
const choices = computed(() => (search.value.trim() ? found.value : suggested.value))

const entityNames = computed(() => (draft.value?.entities ?? []).map(id => ({
  id, name: s.reg.value?.entities.get(id)?.name ?? id,
})))

function onEntity(e: Event) {
  entity.value = (e.target as HTMLSelectElement).value
  rebuild()
}

const router = useRouter()

/** Открыть полную карточку черновиком: введённое — предзаполнено, связь — вместе с первым полем (F-88). */
function toCard() {
  const d = draft.value
  const rec = s.rec.value
  if (!d || !rec || NEW_REF !== DRAFT_REF) return
  const b = draftBatch(rec, { ...d, values: { ...values } }, groups.value)
  const [person, ...rest] = b.records
  if (!person?.template) return
  const id = newDraft({
    dashboard: props.dashboard, template: person.template, groups: [...groups.value],
    base: { fields: person.fields, note: person.note, start: person.start, end: person.end },
    with: { records: rest, participants: b.participants },
  })
  open.value = false
  void router.push({ name: 'record', params: { d: props.dashboard, id } })
}

async function submit() {
  const d = draft.value
  const rec = s.rec.value
  if (!d || !rec) return
  if (mode.value === 'existing') return submitExisting(d, rec)
  if (Object.values(values).some(v => isDateLike(v) && validateDate(v as PartialDate))) {
    error.value = t.card.dateError
    return
  }
  error.value = ''
  busy.value = true
  try {
    const out = await api.operations(props.dashboard, draftBatch(rec, { ...d, values: { ...values } }, groups.value))
    const id = out.ids[NEW_REF]
    if (id) emit('created', id)
    open.value = false
    if (out.errors.length) s.notice.value = out.errors.map(x => `${x.field}: ${x.message}`).join('; ')
    await s.load()
  } catch (e) {
    error.value = errorText(e)
  } finally {
    busy.value = false
  }
}

/** «Существующий»: отношение с выбранной записью (F-130) */
async function submitExisting(d: QuickDraft, rec: RecordDTO) {
  const other = picked.value
  if (!other) return
  error.value = ''
  busy.value = true
  try {
    const out = await api.operations(props.dashboard, existingBatch(rec, d, other))
    emit('created', other)
    open.value = false
    if (out.errors.length) s.notice.value = out.errors.map(x => `${x.field}: ${x.message}`).join('; ')
    await s.load()
  } catch (e) {
    error.value = errorText(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <DialogRoot v-model:open="open">
    <DialogPortal>
      <DialogOverlay class="kin-overlay" />
      <DialogContent
        class="kin-dialog quick-form"
        :aria-describedby="undefined"
      >
        <DialogTitle>{{ option.item.name }}</DialogTitle>
        <form
          class="kin-form"
          @submit.prevent="submit"
        >
          <div
            class="kin-seg"
            role="group"
            :aria-label="option.item.name"
          >
            <button
              type="button"
              class="seg-item"
              :class="{ active: mode === 'new' }"
              :aria-pressed="mode === 'new'"
              data-testid="mode-new"
              @click="setMode('new')"
            >
              {{ t.quickAdd.new }}
            </button>
            <button
              type="button"
              class="seg-item"
              :class="{ active: mode === 'existing' }"
              :aria-pressed="mode === 'existing'"
              data-testid="mode-existing"
              @click="setMode('existing')"
            >
              {{ t.quickAdd.existing }}
            </button>
          </div>
          <label
            v-if="entityNames.length > 1"
            class="kin-label"
          >
            {{ t.quickAdd.entity }}
            <select
              class="kin-input"
              :value="entity"
              @change="onEntity"
            >
              <option
                v-for="e in entityNames"
                :key="e.id"
                :value="e.id"
              >{{ e.name }}</option>
            </select>
          </label>
          <template v-if="draft && mode === 'existing'">
            <label class="kin-label">
              {{ t.quickAdd.search }}
              <input
                v-model="search"
                class="kin-input"
                type="search"
                data-testid="existing-search"
              >
            </label>
            <p
              v-if="!search.trim() && suggested.length"
              class="kin-muted"
            >
              {{ t.quickAdd.suggested }}
            </p>
            <ul
              v-if="choices.length"
              class="choices"
              :aria-label="t.quickAdd.choose"
            >
              <li
                v-for="c in choices"
                :key="c.id"
              >
                <button
                  type="button"
                  class="kin-btn kin-btn--small cand"
                  :class="{ on: picked === c.id }"
                  :aria-pressed="picked === c.id"
                  data-testid="candidate"
                  :data-id="c.id"
                  @click="picked = picked === c.id ? null : c.id"
                >
                  <KinIcon :name="picked === c.id ? 'check' : 'plus'" />
                  {{ c.title }}
                </button>
              </li>
            </ul>
            <p
              v-else-if="search.trim() && !searching"
              class="kin-muted"
            >
              {{ t.quickAdd.nobody }}
            </p>
          </template>
          <template v-else-if="draft">
            <div
              v-for="inp in draft.inputs"
              :key="inp.addr"
              class="kin-label"
            >
              <PartialDateInput
                v-if="inp.kind === 'start' || inp.kind === 'end'"
                :model-value="(values[inp.addr] as PartialDate | null) ?? null"
                :label="inp.label"
                @update:model-value="v => (values[inp.addr] = v)"
              />
              <template v-else>
                {{ inp.label }}
                <input
                  v-if="inp.kind === 'note'"
                  class="kin-input"
                  :value="values[inp.addr] ?? ''"
                  :aria-label="inp.label"
                  @input="e => (values[inp.addr] = (e.target as HTMLInputElement).value)"
                >
                <ValueEditor
                  v-else-if="inp.kind === 'field'"
                  :model-value="values[inp.addr] ?? null"
                  :def="{ ...inp.def, name: inp.label }"
                  :dashboard="dashboard"
                  @update:model-value="v => (values[inp.addr] = v)"
                  @submit="submit"
                />
              </template>
            </div>
            <p
              v-if="dev && draft.problems.length"
              class="kin-muted"
            >
              {{ draft.problems.join('; ') }}
            </p>
          </template>
          <GroupPicker
            v-if="mode === 'new'"
            v-model="groups"
            :dashboard="dashboard"
            :suggest="s.rec.value?.groups"
          />
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
              :aria-label="t.common.cancel"
              :title="t.common.cancel"
            >
              <KinIcon name="close" />
            </DialogClose>
            <button
              class="kin-ibtn kin-ibtn--primary submit"
              type="submit"
              :disabled="!draft || busy || (mode === 'existing' && !picked)"
              :aria-label="t.common.save"
              :title="t.common.save"
            >
              <KinIcon name="check" />
            </button>
            <button
              v-if="mode === 'new'"
              class="kin-ibtn to-card"
              type="button"
              :disabled="!draft || busy"
              :aria-label="t.quickAdd.toCard"
              :title="t.quickAdd.toCard"
              @click="toCard"
            >
              <KinIcon name="forward" />
            </button>
          </div>
        </form>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style scoped>
.cand {
  justify-content: flex-start;
  width: 100%;
  text-align: left;
}

.cand.on {
  color: var(--kin-color-accent-text);
  background: var(--kin-color-accent-soft);
  box-shadow: inset 0 0 0 1px var(--kin-color-accent);
}

/* выбор существующего — список с прокруткой (F-130) */
.choices {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-1);
  max-height: 14rem;
  margin: 0;
  padding: 0;
  overflow: auto;
  list-style: none;
}
</style>
