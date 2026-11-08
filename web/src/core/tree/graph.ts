// Модель дерева из записей (F-111, F-118): что и как рисуется — по `tree_link`
// шаблона, вид линии — по варианту поля `line_by`, подписи — резолвером.
// Ключей kin.core код не знает.
import { astro, compareKeys, eraOf, formatDate, formatYear, isEmptyDate, isEraOnly, isReversedEra, sortKey, type PartialDate } from '../dates'
import { currentOption, currentStage, isEnded, roleLabel } from '../forms'
import { evalElements, recordTitle, tplOf, type Ctx } from '../resolver'
import type { RecordDTO, TemplateDef, TreeLinkDef } from '../types'

import type { TreeInput } from './model'

/** tree_link шаблона как объект; нет или `false` — undefined. */
export function treeLinkOf(tpl: TemplateDef | undefined): TreeLinkDef | undefined {
  const t = tpl?.tree_link
  if (!t) return undefined
  return t === true ? {} : t
}

/** Как рисуется отношение: явный `as` или по числу ролей. */
export function treeAs(tpl: TemplateDef, link: TreeLinkDef): TreeLinkDef['as'] {
  return link.as ?? ((tpl.roles?.length ?? 0) === 2 ? 'descent' : 'couple')
}

const dated = (p: PartialDate | null | undefined): p is PartialDate => !!p && !isEmptyDate(p) && p.y != null

/**
 * Ранги ключей дат для раскладки: раскладка сравнивает строки и числа
 * операторами JS, а ключ с эрой надо сравнивать по кодовым точкам (compareKeys)
 * — поэтому ей передаётся место ключа среди всех, '0' + 6 цифр (меньше '9999').
 */
function ranks(keys: string[]): Map<string, number> {
  const sorted = [...new Set(keys)].sort(compareKeys)
  return new Map(sorted.map((k, i) => [k, i]))
}

/**
 * «Год» человека для раскладки: астрономический год, если все даты дерева в
 * одной эре; иначе — место даты рождения среди всех (порядок эр, F-143).
 */
function yearsOf(ctx: Ctx, persons: RecordDTO[]): Map<string, number> {
  const out = new Map<string, number>()
  const starts = persons.map(p => [p.id, p.start?.value] as const).filter((x): x is readonly [string, PartialDate] => dated(x[1]))
  const eras = new Set(starts.map(([, d]) => d.e ?? ''))
  if (eras.size <= 1) {
    // в эре с обратным счётом больший год — раньше (F-143)
    const sign = eras.size === 1 && isReversedEra([...eras][0]!, ctx.eras) ? -1 : 1
    for (const [id, d] of starts) out.set(id, sign * astro(d.y!))
    return out
  }
  const r = ranks(starts.map(([, d]) => sortKey(d, ctx.eras)))
  for (const [id, d] of starts) out.set(id, r.get(sortKey(d, ctx.eras))!)
  return out
}

/**
 * Модель из ответа /tree: люди — участники отношений с tree_link; отношения
 * вне `stages` и с участниками не из `persons` не учитываются.
 */
export function treeInput(ctx: Ctx, relations: RecordDTO[], persons: RecordDTO[]): TreeInput {
  const out: TreeInput = { persons: [], couples: [], siblings: [], descents: [] }
  const known = new Set(persons.map(p => p.id))
  const used = new Set<string>()
  for (const rel of relations) {
    const tpl = tplOf(ctx, rel)
    const link = treeLinkOf(tpl)
    if (!tpl || !link) continue
    if (link.stages && !link.stages.includes(currentStage(ctx, rel)?.key ?? '')) continue
    const as = treeAs(tpl, link)
    const roles = tpl.roles ?? []
    const of = (role: string | undefined) => [...new Set(rel.participants.filter(p => p.role === role && known.has(p.entity_id)).map(p => p.entity_id))]
    if (as === 'descent') {
      const parent = of(roles[0]?.key)[0]
      const child = of(roles[1]?.key)[0]
      if (!parent || !child || parent === child) continue
      const field = link.line_by ? tpl.fields?.find(f => f.key === link.line_by) : undefined
      const option = field ? currentOption(ctx, rel, field) : undefined
      const dashed = option?.tree_line === 'dashed'
      out.descents.push({
        id: rel.id, parent, child, dashed,
        kind: dashed ? option?.name : undefined,
        label: dashed ? roleLabel(ctx, rel, roles[0]!.key, parent) : undefined,
      })
      used.add(parent).add(child)
      continue
    }
    const members = of(roles[0]?.key)
    if (members.length < 2) continue
    const label = recordTitle(ctx, rel)
    if (as === 'siblings') out.siblings.push({ id: rel.id, members, label })
    else {
      // подпись пары — с годами (F-144): «Брак 1938 – 1945»
      const label = coupleLabel(ctx, rel)
      // больше двух участников — каждый следующий в паре с первым; клик по любому узлу — карточка отношения (Q-36)
      // дата из одной эры — тоже по месту эры (F-14)
      const start = rel.start?.value
      const order = dated(start) || isEraOnly(start) ? sortKey(start!, ctx.eras) : '9999'
      for (let i = 1; i < members.length; i++) {
        out.couples.push({ id: i === 1 ? rel.id : `${rel.id}:${i}`, relation: rel.id, a: members[0]!, b: members[i]!, order, label })
      }
    }
    for (const m of members) used.add(m)
  }
  // ключи дат пар — рангами (см. ranks); без даты — '9999', после всех
  const r = ranks(out.couples.map(c => c.order).filter(o => o !== '9999'))
  for (const c of out.couples) if (c.order !== '9999') c.order = '0' + String(r.get(c.order)!).padStart(6, '0')
  // режим и ранги — только по людям, которые попадут в дерево
  const years = yearsOf(ctx, persons.filter(p => used.has(p.id)))
  for (const p of persons) {
    if (used.has(p.id)) out.persons.push({ id: p.id, year: years.get(p.id), dead: isEnded(ctx, p) })
  }
  return out
}

/** Год даты для подписи (F-14): год с эрой по виду дашборда; дата из одной эры — эра; без года — пусто. */
function yearText(ctx: Ctx, p: PartialDate | null | undefined): string {
  if (!p) return ''
  if (p.y != null) return formatYear(p.y, eraOf(p), ctx.words.dates, ctx.dateView)
  return isEraOnly(p) ? eraOf(p) : ''
}

/**
 * Подпись узла отношения в дереве (F-144): название и годы — «Брак 1938 – 1945»,
 * без конца — «Брак 1938», без начала — «Брак – 1945», без обоих — «Брак».
 */
export function coupleLabel(ctx: Ctx, rel: RecordDTO): string {
  const title = recordTitle(ctx, rel)
  const from = yearText(ctx, rel.start?.value)
  const to = yearText(ctx, rel.end?.value)
  const years = to ? (from ? `${from} – ${to}` : `– ${to}`) : from
  return years ? `${title} ${years}` : title
}

/** Подписи узла человека (F-116): имя по `tree_title` (нет — `title`), даты полностью, примечание. */
export function personView(ctx: Ctx, rec: RecordDTO): { title: string; dates: string; note: string; dead: boolean } {
  const tpl = tplOf(ctx, rec)
  const title = (tpl?.tree_title ? evalElements(ctx, rec, tpl.tree_title) : recordTitle(ctx, rec)).trim()
  const from = formatDate(rec.start?.value, ctx.words.dates, ctx.dateView)
  const to = formatDate(rec.end?.value, ctx.words.dates, ctx.dateView)
  return {
    title: title || recordTitle(ctx, rec),
    // нет даты рождения — только «– смерть», без подставленного начала (F-42)
    dates: to ? `${from} – ${to}`.trim() : from,
    note: rec.note ?? '',
    dead: isEnded(ctx, rec),
  }
}
