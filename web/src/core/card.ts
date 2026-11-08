// Модель карточки записи (что показать), собранная только из шаблона и
// данных конверта. Порт card.ts прототипа format-v0: без просмотра «на дату»
// (F-74), связи — из participates_in, полей-ссылок и related; порядок
// раздела «Связи» — F-93 (внутри групп — Д-27).

import { compareKeys, formatDate, hiKey, isEmptyDate, loKey, sortKey } from './dates'
import { activeToday, currentStage, flagEnded, roleLabel } from './forms'
import { linksRank, periodHiddenWhenEmpty, periodLabel, showsLinks, systemLabel, type SystemKey } from './registry'
import {
  CUSTOM_PREFIX, NOTE, recordTitle, recordTitleById, SHARED_PREFIX, STAGE, storedRaw, tplOf, valuesOf, valueText, type Ctx,
} from './resolver'
import type { CustomSectionDef, FieldDef, MetaDef, PartialDate, RecordDTO, StageDef, TemplateDef, ValueDTO } from './types'

/** Поле картинки записи (F-72). */
export const PHOTO = 'photo'
/** Раздел полей по умолчанию и раздел свободных полей без своего раздела. */
export const MAIN_SECTION = 'main'
export const CUSTOM_SECTION = 'custom'

export interface ValueView {
  id: string
  text: string
  from: string
  to: string
  note: string | null
  meta: { key: string; name: string; value: unknown }[]
  links: { anchor: string; target: string; title: string }[]
}

export interface FieldView {
  /** ключ значения в API: ключ поля или `custom.<ключ>` */
  key: string
  name: string
  def: FieldDef
  custom: boolean
  /** общее своё поле шаблона в дашборде (F-96) */
  shared: boolean
  empty: boolean
  /** `show: filled` и пусто — поле прячется за «Добавить» */
  hidden: boolean
  /** занято значением, скрытым от гостя (Q-50): «Задано», без правки */
  occupied?: boolean
  /** поле быстрой формы дашборда (`quick_fields` с переопределением, F-124): в правке стоит сразу, даже пустое */
  quick: boolean
  /** что показать на карточке: текущее (верхнее) или все (multiple) */
  shown: string[]
  values: ValueView[]
}

export interface SectionView {
  key: string
  name: string
  fields: FieldView[]
  /** скрытые пустые поля — для «Добавить» */
  addable: FieldView[]
  /** свой блок (F-139): описание и область — запись или весь шаблон в дашборде */
  custom?: { def: CustomSectionDef; shared: boolean }
}

/** Блок, куда можно поставить своё поле (F-139): блоки шаблона, «Свои поля», свои блоки. */
export interface SectionChoice {
  key: string
  name: string
  /** свой блок записи — только для поля записи, не для общего */
  own: boolean
}

/**
 * Блоки карточки по порядку (F-139): блоки полей шаблона (основной — всегда),
 * «Свои поля», затем свои блоки — общие шаблона, потом записи.
 */
export function sectionChoices(ctx: Ctx, rec: RecordDTO): SectionChoice[] {
  const tpl = tplOf(ctx, rec)
  const out: SectionChoice[] = []
  const add = (key: string, name: string, own = false) => {
    if (!out.some(s => s.key === key)) out.push({ key, name, own })
  }
  add(MAIN_SECTION, ctx.words.sections[MAIN_SECTION] ?? MAIN_SECTION)
  for (const f of tpl?.fields ?? []) {
    const k = f.section ?? MAIN_SECTION
    add(k, ctx.words.sections[k] ?? k)
  }
  add(CUSTOM_SECTION, ctx.words.sections[CUSTOM_SECTION] ?? CUSTOM_SECTION)
  for (const s of tpl?.custom_sections ?? []) add(s.key, s.name)
  for (const s of rec.custom_sections ?? []) add(s.key, s.name, true)
  return out
}

export interface DateView {
  key: SystemKey
  label: string
  text: string
  value: PartialDate | null
  link: { id: string; title: string } | null
  /** `show: filled` в period и пусто — дата прячется за «Добавить» (F-99) */
  hidden: boolean
  /** дата в быстрой форме дашборда (F-124): в правке стоит сразу, даже пустая */
  quick: boolean
}

export interface StageView {
  label: string
  current: { key: string | null; text: string } | null
  values: ValueView[]
  options: StageDef[]
}

export interface ParticipantView {
  participation: string
  record: string
  title: string
  role: string
  label: string
  period: string
  active: boolean
}

export interface RoleView {
  role: string
  /** название роли — её нейтральная форма */
  name: string
  items: ParticipantView[]
}

export interface RelationView {
  /**
   * Группа раздела «Связи» (F-93): `people` — люди через отношение (участники
   * связи с `links`), `link` — прямая ссылка: связь без `links` (событие) или
   * запись из поля-ссылки этой записи.
   */
  kind: 'people' | 'link'
  /** ключ строки раздела (запись может попасть и в связь, и в поле-ссылку) */
  key: string
  /** у ссылки из поля — название поля */
  fieldLabel: string | null
  record: string
  title: string
  templateName: string
  entity: string
  /** как называется эта запись в связи */
  myLabel: string
  others: ParticipantView[]
  /**
   * Строки раздела «Связи» (F-89): форма роли другого участника и его
   * подпись; участники с одинаковой формой — одной строкой, в порядке связи.
   */
  lines: RelationLine[]
  active: boolean
  /** прошлое (F-98): наступил конец связи или дата «по» моего участия — без особых форм (F-74) */
  past: boolean
  stage: string | null
}

export interface RelationLine {
  label: string
  people: { participation: string; record: string; title: string }[]
}

/** Другие участники связи по формам их ролей (F-89). */
export function relationLines(others: ParticipantView[]): RelationLine[] {
  const out: RelationLine[] = []
  for (const o of others) {
    let line = out.find(l => l.label === o.label)
    if (!line) out.push(line = { label: o.label, people: [] })
    line.people.push({ participation: o.participation, record: o.record, title: o.title })
  }
  return out
}

export interface CardView {
  id: string
  title: string
  template: TemplateDef | undefined
  entity: string
  freeedit: boolean
  titleManual: string | null
  note: string | null
  sensitive: boolean
  labels: { title: string; note: string; sensitive: string; groups: string }
  photo: { sha: string; crop: unknown } | null
  dates: DateView[]
  /** поле-признак завершения шаблона (`end_flag`, F-126): в шапке у конца периода, не в разделах */
  endFlag: FieldView | null
  sections: SectionView[]
  stage: StageView | null
  roles: RoleView[]
  relations: RelationView[]
}

function valueView(ctx: Ctx, def: Pick<FieldDef, 'type' | 'options'> | undefined, metaDefs: MetaDef[] | undefined, v: ValueDTO): ValueView {
  return {
    id: v.id,
    text: valueText(ctx, def, storedRaw(v)),
    from: formatDate(v.valid_from, ctx.words.dates, ctx.dateView),
    to: formatDate(v.valid_to, ctx.words.dates, ctx.dateView),
    note: v.note,
    meta: Object.entries(v.meta ?? {}).map(([key, value]) => ({
      key, value, name: metaDefs?.find(m => m.key === key)?.name ?? key,
    })),
    links: (v.links ?? []).map(l => ({ anchor: l.anchor, target: l.target, title: recordTitleById(ctx, l.target) })),
  }
}

function fieldView(ctx: Ctx, rec: RecordDTO, def: FieldDef, custom: boolean, shared = false, quick = false): FieldView {
  const key = shared ? SHARED_PREFIX + def.key : custom ? CUSTOM_PREFIX + def.key : def.key
  const values = valuesOf(rec, key).map(v => valueView(ctx, def, def.meta, v))
  const shown = (def.multiple ? values : values.slice(0, 1)).map(v => v.text)
  const occupied = !!rec.fields[key]?.hidden
  const empty = values.length === 0 && !occupied
  return {
    key, name: def.name, def, custom: custom || shared, shared, values, shown,
    empty,
    hidden: def.show === 'filled' && empty,
    occupied,
    quick,
  }
}

function period(ctx: Ctx, from: PartialDate | null, to: PartialDate | null): string {
  return [formatDate(from, ctx.words.dates, ctx.dateView), formatDate(to, ctx.words.dates, ctx.dateView)].filter(Boolean).join(' — ')
}

function participantView(ctx: Ctx, container: RecordDTO, p: RecordDTO['participants'][number]): ParticipantView {
  return {
    participation: p.id,
    record: p.entity_id,
    title: recordTitleById(ctx, p.entity_id),
    role: p.role,
    label: roleLabel(ctx, container, p.role, p.entity_id),
    period: period(ctx, p.valid_from, p.valid_to),
    active: activeToday(ctx, container, p),
  }
}

/** Порядок связей (Д-27): по start записи-связи, без даты — в конце по времени создания (id — UUIDv7). */
function relationOrder(ctx: Ctx, a: RecordDTO, b: RecordDTO): number {
  const ka = a.start?.value && !isEmptyDate(a.start.value) ? sortKey(a.start.value, ctx.eras) : '￿'
  const kb = b.start?.value && !isEmptyDate(b.start.value) ? sortKey(b.start.value, ctx.eras) : '￿'
  if (ka !== kb) return compareKeys(ka, kb)
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0
}

/** Сравнение весов (F-103); оба без веса (Infinity) — равны. */
function rankOrder(a: number, b: number): number {
  return a === b ? 0 : a < b ? -1 : 1
}

function hasEnd(p: RecordDTO['end']): boolean {
  return !!p && (!!p.link || !isEmptyDate(p.value))
}

/**
 * Дата наступила (F-98): прошла её верхняя граница (2025 — после конца 2025).
 * Без года наступление не установить — не наступила. Сравнение — по месту
 * эры и году (F-143); «сегодня» — год мира без дня: до его начала.
 */
function passed(ctx: Ctx, d: PartialDate | null | undefined): boolean {
  const h = hiKey(d, ctx.eras)
  const today = loKey(ctx.today, ctx.eras)
  return h != null && today != null && compareKeys(h, today) < 0
}

/**
 * Прошлая связь (F-98): наступил конец связи или дата «по» моего участия.
 * Конец-ссылка без своей даты — по дате записи, к которой привязан (F-83);
 * её даты нет — когда конец, неизвестно: связь текущая.
 */
function isPast(ctx: Ctx, c: RecordDTO, p: { valid_to: PartialDate | null }): boolean {
  if (!isEmptyDate(p.valid_to) && passed(ctx, p.valid_to)) return true
  if (flagEnded(ctx, c)) return true
  const end = c.end
  if (!end || !hasEnd(end)) return false
  const d = !isEmptyDate(end.value) ? end.value : (end.link ? ctx.records[end.link]?.start?.value : null)
  return passed(ctx, d)
}

/** Картинка записи (F-72): текущее значение поля `photo` шаблона и его обрезка; нет — null. */
export function photoOf(ctx: Ctx, rec: RecordDTO): { sha: string; crop: unknown } | null {
  const tpl = tplOf(ctx, rec)
  const v = tpl?.fields?.some(f => f.key === PHOTO && f.type === 'image') ? valuesOf(rec, PHOTO)[0] : undefined
  return v && typeof v.value === 'string' ? { sha: v.value, crop: v.meta?.crop ?? null } : null
}

export function cardView(ctx: Ctx, rec: RecordDTO): CardView {
  const tpl = tplOf(ctx, rec)
  const entity = tpl?.entity ?? rec.entity
  const label = (key: string) => systemLabel(ctx.reg, entity, key, ctx.words)

  const dates: DateView[] = []
  for (const key of ['start', 'end'] as const) {
    const l = periodLabel(ctx.reg, tpl, key, ctx.words)
    if (l === null) continue // шаблон или сущность скрыли поле (F-70)
    const p = rec[key]
    dates.push({
      key, label: l, value: p?.value ?? null,
      text: formatDate(p?.value, ctx.words.dates, ctx.dateView),
      link: p?.link ? { id: p.link, title: recordTitleById(ctx, p.link) } : null,
      hidden: periodHiddenWhenEmpty(ctx.reg, tpl, key) && isEmptyDate(p?.value) && !p?.link,
      quick: !!tpl?.quick_fields?.includes(key),
    })
  }

  // разделы: поля шаблона, общие свои поля шаблона (F-96), затем свободные поля записи (F-61, F-75);
  // своё поле — в любом блоке (F-139), блок которого нет — в «Свои поля»
  const choices = sectionChoices(ctx, rec)
  const sharedKeys = new Set((tpl?.custom_sections ?? []).map(s => s.key))
  const sections = new Map<string, SectionView>()
  const section = (key: string) => {
    let s = sections.get(key)
    if (!s) {
      const c = choices.find(x => x.key === key)
      const def = c && (sharedKeys.has(key) || c.own) ? { key, name: c.name } : undefined
      s = { key, name: c?.name ?? ctx.words.sections[key] ?? key, fields: [], addable: [], ...(def ? { custom: { def, shared: !c!.own } } : {}) }
      sections.set(key, s)
    }
    return s
  }
  const put = (fv: FieldView, key: string) => {
    const s = section(fv.custom && !choices.some(c => c.key === key) ? CUSTOM_SECTION : key)
    ;(fv.hidden ? s.addable : s.fields).push(fv)
  }
  let endFlag: FieldView | null = null
  for (const def of tpl?.fields ?? []) {
    // quick_fields шаблона — уже с переопределением дашборда (overrides, F-124)
    const fv = fieldView(ctx, rec, def, false, false, !!tpl?.quick_fields?.includes(def.key))
    if (tpl?.end_flag && def.key === tpl.end_flag) endFlag = fv
    else put(fv, def.section ?? MAIN_SECTION)
  }
  for (const def of tpl?.custom_fields ?? []) put(fieldView(ctx, rec, def, true, true), def.section || CUSTOM_SECTION)
  for (const def of rec.custom_fields) put(fieldView(ctx, rec, def, true), def.section || CUSTOM_SECTION)
  // свои блоки — и пустые (их видно в правке); порядок — блоки шаблона, затем свои
  for (const c of choices) if (sharedKeys.has(c.key) || c.own) section(c.key)
  const rank = (key: string) => {
    const i = choices.findIndex(c => c.key === key)
    return i < 0 ? choices.length : i
  }
  const ordered = [...sections.values()].sort((a, b) => rank(a.key) - rank(b.key))

  const photo = photoOf(ctx, rec)

  let stage: StageView | null = null
  if (tpl?.stages?.length) {
    const stages = tpl.stages
    const vals = valuesOf(rec, STAGE)
    const top = vals[0]
    const cur = currentStage(ctx, rec)
    const stageType = { type: 'choice' as const, options: stages.filter(s => s.name).map(s => ({ key: s.key, name: s.name! })) }
    stage = {
      label: label(STAGE),
      current: top ? { key: cur?.key ?? null, text: cur?.name ?? String(top.value) } : null,
      values: vals.map(v => valueView(ctx, stageType, stages.find(s => s.key === v.value)?.meta, v)),
      options: stages.filter(s => s.name),
    }
  }

  // участники этой записи по ролям шаблона (F-39)
  const roles: RoleView[] = (tpl?.roles ?? []).map(r => ({
    role: r.key,
    name: r.forms.default,
    items: rec.participants.filter(p => p.role === r.key).map(p => participantView(ctx, rec, p)),
  }))

  // где участвует эта запись — со стороны другого участника
  const containers: [RecordDTO, RecordDTO['participates_in'][number]][] = []
  for (const p of rec.participates_in) {
    const c = ctx.records[p.container_id]
    if (c && tplOf(ctx, c)) containers.push([c, p])
  }
  const relationView = ([c, p]: (typeof containers)[number]): RelationView => {
    const ctpl = tplOf(ctx, c)!
    const people = showsLinks(ctx.reg, ctpl)
    const others = people
      ? c.participants.filter(o => o.id !== p.id && o.entity_id !== rec.id).map(o => participantView(ctx, c, o))
      : []
    return {
      kind: people ? 'people' : 'link',
      key: `c:${c.id}`,
      fieldLabel: null,
      record: c.id,
      title: recordTitle(ctx, c),
      templateName: ctpl.name,
      entity: ctpl.entity,
      myLabel: roleLabel(ctx, c, p.role, rec.id),
      others,
      lines: relationLines(others),
      active: activeToday(ctx, c, p),
      past: isPast(ctx, c, p),
      stage: ctpl.stages?.length ? (currentStage(ctx, c)?.name ?? null) : null,
    }
  }
  // F-93: сначала люди через отношения, затем прямые ссылки — связи без
  // `links` (события) и записи из полей-ссылок; внутри группы — Д-27.
  // Отношения — по весу шаблона (F-103), при равном — Д-27; прошлые
  // отделяются фильтром и сохраняют этот порядок.
  const byOrder = <T>(list: [RecordDTO, T][]) => list.sort(([a], [b]) => relationOrder(ctx, a, b))
  const peopleOf = containers
    .filter(([c]) => showsLinks(ctx.reg, tplOf(ctx, c)))
    .sort(([a], [b]) => rankOrder(linksRank(tplOf(ctx, a)), linksRank(tplOf(ctx, b))) || relationOrder(ctx, a, b))
  const direct: [RecordDTO, RelationView][] = containers
    .filter(([c]) => !showsLinks(ctx.reg, tplOf(ctx, c)))
    .map(cp => [cp[0], relationView(cp)])
  // строка ссылки — значение поля: одна запись в двух полях или значениях — две строки
  for (const fv of [...sections.values()].flatMap(sec => sec.fields)) {
    if (fv.def.type !== 'ref') continue
    const vals = valuesOf(rec, fv.key)
    for (const v of fv.def.multiple ? vals : vals.slice(0, 1)) {
      const target = typeof v.value === 'string' ? ctx.records[v.value] : undefined
      if (!target || target.id === rec.id) continue
      const key = `f:${fv.key}:${v.id}`
      const ttpl = tplOf(ctx, target)
      direct.push([target, {
        kind: 'link', key, fieldLabel: fv.name,
        record: target.id, title: recordTitle(ctx, target), templateName: ttpl?.name ?? '',
        entity: ttpl?.entity ?? target.entity, myLabel: '', others: [], lines: [],
        active: true, past: false, stage: null,
      }])
    }
  }
  const relations: RelationView[] = [
    ...peopleOf.map(relationView),
    ...byOrder(direct).map(([, v]) => v),
  ]

  return {
    id: rec.id,
    title: recordTitle(ctx, rec),
    template: tpl,
    entity,
    freeedit: !!tpl?.title?.some(e => e.freeedit),
    titleManual: rec.title_manual,
    note: rec.note,
    sensitive: rec.sensitive,
    labels: { title: label('title'), note: label(NOTE), sensitive: label('sensitive'), groups: label('groups') },
    photo,
    dates,
    endFlag,
    sections: ordered,
    stage,
    roles,
    relations,
  }
}
