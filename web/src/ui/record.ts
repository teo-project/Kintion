// Состояние карточки записи и общий механизм сохранения по полю (F-45):
// каждый запрос несёт версию своего поля (`if_rev`); 409 — поле изменил
// кто-то другой: запись перечитывается (поле — до текущего значения) и
// показывается сообщение; ошибка проверки (400) — сообщение у поля, поле
// остаётся в последнем сохранённом состоянии (F-15).
import { computed, inject, provide, reactive, ref, shallowRef, watch, type InjectionKey, type Ref } from 'vue'

import { api, ApiError } from '@/api/client'
import type { CardMode, RecordOut, RelatedDTO } from '@/api/types'
import { cardView } from '@/core/card'
import type { Registry } from '@/core/registry'
import type { FieldDef, RecordDTO } from '@/core/types'
import { errorText, t } from '@/i18n'
import { useDashboardsStore } from '@/stores/dashboards'

import { makeCtx } from './ctx'
import {
  DRAFT_REF, draftCreateBatch, draftRecord, dropDraft, getDraft, isDraftId, type DraftBase, type DraftSpec,
} from './drafts'
import { cardModePref, historyEditPref, setHistoryEditPref } from './prefs'

/** Выбор при правке поля с историей (F-80). */
export type HistoryChoice = 'replace' | 'add'

/** Итог сохранения поля: сохранено, конфликт версии (F-45), ошибка, отменено пользователем. */
export type SaveResult = 'ok' | 'conflict' | 'error' | 'cancel'

/** Версия поля: системные — из `revs`, поля и `custom.*` — из `fields[key].rev`. */
export function revOf(rec: RecordDTO, key: string): number {
  return rec.revs?.[key] ?? rec.fields[key]?.rev ?? 0
}

/**
 * Состояние карточки. created — запись из черновика создана (F-88):
 * вызывающий переходит по её адресу.
 */
/** ключ ошибки и сохранения общих своих полей шаблона (F-96) */
export const SHARED_REV = 'shared_fields'

export function useRecordState(dashboard: Ref<string>, id: Ref<string>, created?: (id: string) => void) {
  const dashboards = useDashboardsStore()
  const reg = shallowRef<Registry | null>(null)
  // реестр правят и другие экраны (настройки дашборда) — карточка видит сразу
  watch(() => dashboards.current(dashboard.value), r => {
    if (r && reg.value && r !== reg.value) reg.value = r
  })
  const rec = shallowRef<RecordDTO | null>(null)
  const related = shallowRef<RelatedDTO>({})
  const error = ref('')
  /** сообщение о конфликте версии (F-45) */
  const notice = ref('')
  /** ошибки по ключам полей (F-15) */
  const fieldErrors = reactive<Record<string, string | undefined>>({})
  const saving = reactive<Record<string, boolean | undefined>>({})
  /** режим редактирования карточки (F-85) */
  const editing = ref(false)
  /** режим карточек аккаунта (F-108) */
  const mode = ref<CardMode>('read')
  /** черновик новой записи (F-88): на сервере записи ещё нет */
  const draft = shallowRef<DraftSpec | null>(null)
  /** открытый вопрос «заменить или добавить в историю» (F-80) */
  const historyAsk = shallowRef<{ label: string; resolve: (v: HistoryChoice | null) => void } | null>(null)

  /**
   * Как править поле с историей (F-80): запомненный выбор пользователя или
   * вопрос; null — пользователь передумал.
   */
  async function historyChoice(label: string): Promise<HistoryChoice | null> {
    const pref = await historyEditPref()
    if (pref) return pref
    historyAsk.value?.resolve(null)
    return new Promise(resolve => {
      historyAsk.value = { label, resolve }
    })
  }

  /** Ответ на вопрос; remember — запомнить на сервере. */
  async function answerHistory(v: HistoryChoice | null, remember = false) {
    const ask = historyAsk.value
    historyAsk.value = null
    if (v && remember) {
      try {
        await setHistoryEditPref(v)
      } catch (e) {
        notice.value = errorText(e)
      }
    }
    ask?.resolve(v)
  }

  /** право на правку: в 0.1 — запись владельца не в корзине (F-108: без него — только чтение) */
  const writable = computed(() => !!rec.value && !rec.value.deleted)
  /** «По месту» (F-108): ручки у полей карточки, пока она не в форме */
  const inlineMode = computed(() => mode.value === 'inline' && writable.value && !editing.value)
  // запись ушла в корзину — правки больше нет (пустая карточка при смене
  // записи — не повод); восстановлена в режиме «Форма» — снова форма (F-108)
  watch(() => rec.value?.deleted, (d, was) => {
    if (d) editing.value = false
    else if (d === false && was === true && mode.value === 'form') editing.value = true
  })

  const ctx = computed(() => (reg.value ? makeCtx(reg.value, [rec.value ?? undefined], related.value) : null))
  const card = computed(() => (ctx.value && rec.value ? cardView(ctx.value, rec.value) : null))

  function apply(out: RecordOut) {
    related.value = out.related
    rec.value = out.record
  }

  let loadSeq = 0

  /**
   * Загрузить запись из маршрута. Другая запись или дашборд — прежняя
   * карточка сразу убирается (её нельзя править по чужому адресу); ответ
   * применяется, только если это всё ещё последняя загрузка.
   */
  async function load() {
    const my = ++loadSeq
    const d = dashboard.value
    const rid = id.value
    error.value = ''
    const other = rec.value?.id !== rid
    if (other) {
      // запись из черновика только что создана — правка продолжается
      const keepEditing = !!draft.value && !isDraftId(rid)
      rec.value = null
      draft.value = null
      editing.value = keepEditing
      related.value = {}
      notice.value = ''
      for (const k of Object.keys(fieldErrors)) fieldErrors[k] = undefined
    }
    if (isDraftId(rid)) return loadDraft(my, d, rid)
    try {
      const [r, out, m] = await Promise.all([dashboards.registry(d), api.record(rid), cardModePref()])
      if (my !== loadSeq || d !== dashboard.value || rid !== id.value) return
      reg.value = dashboards.current(d) ?? r
      mode.value = m
      apply(out)
      // «Форма» (F-108): другая запись — сразу в правке, в т. ч. по ссылке и «назад»
      if (other && m === 'form' && !out.record.deleted) editing.value = true
    } catch (e) {
      if (my === loadSeq) error.value = errorText(e)
    }
  }

  /** Черновик новой записи (F-88): карточка без сервера, сразу в режиме редактирования. */
  async function loadDraft(my: number, d: string, rid: string) {
    const spec = getDraft(rid)
    try {
      const r = await dashboards.registry(d)
      if (my !== loadSeq || d !== dashboard.value || rid !== id.value) return
      const tpl = spec?.dashboard === d ? r.templates.get(spec.template) : undefined
      if (!spec || !tpl) {
        error.value = t.card.draftLost
        return
      }
      reg.value = dashboards.current(d) ?? r
      related.value = {}
      draft.value = spec
      rec.value = draftRecord(rid, spec, tpl.entity, tpl.id)
      editing.value = true
    } catch (e) {
      if (my === loadSeq) error.value = errorText(e)
    }
  }

  /**
   * Первое непустое значение черновика (F-88): запись, это значение и связь,
   * из которой создаём, — одной операцией (строгий пакет: значение не
   * сохранилось — записи нет).
   */
  async function createWith(key: string, part: DraftBase): Promise<SaveResult> {
    const spec = draft.value
    if (!spec || saving[key]) return 'error'
    fieldErrors[key] = undefined
    notice.value = ''
    saving[key] = true
    const rid = id.value
    try {
      const out = await api.operations(spec.dashboard, draftCreateBatch(spec, part))
      const nid = out.ids[DRAFT_REF]
      if (!nid) return 'error'
      dropDraft(rid)
      created?.(nid)
      return 'ok'
    } catch (e) {
      fieldErrors[key] = errorText(e)
      return 'error'
    } finally {
      saving[key] = false
    }
  }

  /**
   * Очистили предзаполненное в черновике (F-88): значение поля, примечание
   * или дата больше не уйдут с первым сохранением.
   */
  function clearDraftValue(key: string) {
    const spec = draft.value
    if (!spec || !rec.value || !spec.base) return
    const base = { ...spec.base }
    if (key === 'note' || key === 'start' || key === 'end') delete base[key]
    else if (base.fields?.[key]) {
      const fields = { ...base.fields }
      delete fields[key]
      base.fields = fields
    } else return
    spec.base = base
    const tpl = reg.value?.templates.get(spec.template)
    if (tpl) rec.value = draftRecord(rec.value.id, spec, tpl.entity, tpl.id)
  }

  /** Группы черновика — только в интерфейсе, уйдут с первым значением. */
  function setDraftGroups(ids: string[]) {
    const spec = draft.value
    if (!spec || !rec.value) return
    spec.groups = [...ids]
    rec.value = { ...rec.value, groups: [...ids] }
  }

  /**
   * Сохранить одно поле: `fn` получает его текущую версию и делает запрос.
   * true — сохранено; false — конфликт или ошибка (сообщение уже показано).
   */
  async function save(key: string, fn: (rev: number) => Promise<RecordOut>): Promise<boolean> {
    return (await saveField(key, fn)) === 'ok'
  }

  /** То же, что save, но с итогом: конфликт отличается от ошибки проверки (F-86). */
  async function saveField(key: string, fn: (rev: number) => Promise<RecordOut>): Promise<SaveResult> {
    if (!rec.value || draft.value) return 'error'
    fieldErrors[key] = undefined
    notice.value = ''
    saving[key] = true
    const rid = rec.value.id
    try {
      const out = await fn(revOf(rec.value, key))
      if (out.record.id === rid && rec.value?.id === rid) apply(out)
      return 'ok'
    } catch (e) {
      if (e instanceof ApiError && e.conflict) {
        notice.value = t.errors.conflict
        await load()
        return 'conflict'
      }
      fieldErrors[key] = errorText(e)
      return 'error'
    } finally {
      saving[key] = false
    }
  }

  /**
   * Общие свои поля шаблона записи в дашборде целиком (F-96) — с версией
   * шаблона (F-45); ошибка — под ключом SHARED_REV.
   */
  async function saveShared(list: FieldDef[]): Promise<boolean> {
    const r = reg.value
    const row = rec.value?.template_id
    const tpl = row ? r?.byRowId.get(row) : undefined
    if (!r || !row || !tpl) return false
    fieldErrors[SHARED_REV] = undefined
    notice.value = ''
    saving[SHARED_REV] = true
    try {
      const dto = await api.putSharedFields(dashboard.value, row, { if_rev: tpl.rev ?? 0, custom_fields: list })
      reg.value = dashboards.replaceTemplate(dashboard.value, r, dto)
      return true
    } catch (e) {
      if (e instanceof ApiError && e.conflict) {
        notice.value = t.errors.conflict
        dashboards.forget(dashboard.value)
        reg.value = await dashboards.registry(dashboard.value)
        return false
      }
      fieldErrors[SHARED_REV] = errorText(e)
      return false
    } finally {
      saving[SHARED_REV] = false
    }
  }

  return {
    saveShared,
    reg, rec, related, ctx, card, error, notice, fieldErrors, saving, editing, mode, writable, inlineMode, historyAsk, draft,
    load, save, saveField, apply, historyChoice, answerHistory, createWith, setDraftGroups, clearDraftValue,
  }
}

export type RecordState = ReturnType<typeof useRecordState>

const KEY: InjectionKey<RecordState> = Symbol('record')

export function provideRecord(s: RecordState): void {
  provide(KEY, s)
}

/** Карточка, если компонент внутри неё (поле ввода годится и вне карточки). */
export function useRecordMaybe(): RecordState | null {
  return inject(KEY, null)
}

export function useRecord(): RecordState {
  const s = inject(KEY)
  if (!s) throw new Error('useRecord outside of a record card')
  return s
}
