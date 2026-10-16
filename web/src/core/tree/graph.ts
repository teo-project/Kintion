// Модель дерева из записей (F-111, F-118): что и как рисуется — по `tree_link`
// шаблона, вид линии — по варианту поля `line_by`, подписи — резолвером.
// Ключей kin.core код не знает.
import { formatDate, lo } from '../dates'
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

function yearOf(rec: RecordDTO): number | undefined {
  const s = lo(rec.start?.value)
  return s ? Number(s.slice(0, 4)) : undefined
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
      // больше двух участников — каждый следующий в паре с первым; клик по любому узлу — карточка отношения (Q-36)
      const order = lo(rel.start?.value) ?? '9999'
      for (let i = 1; i < members.length; i++) {
        out.couples.push({ id: i === 1 ? rel.id : `${rel.id}:${i}`, relation: rel.id, a: members[0]!, b: members[i]!, order, label })
      }
    }
    for (const m of members) used.add(m)
  }
  for (const p of persons) {
    if (used.has(p.id)) out.persons.push({ id: p.id, year: yearOf(p), dead: isEnded(ctx, p) })
  }
  return out
}

/** Подписи узла человека (F-116): имя по `tree_title` (нет — `title`), даты полностью, примечание. */
export function personView(ctx: Ctx, rec: RecordDTO): { title: string; dates: string; note: string; dead: boolean } {
  const tpl = tplOf(ctx, rec)
  const title = (tpl?.tree_title ? evalElements(ctx, rec, tpl.tree_title) : recordTitle(ctx, rec)).trim()
  const from = formatDate(rec.start?.value, ctx.words.dates)
  const to = formatDate(rec.end?.value, ctx.words.dates)
  return {
    title: title || recordTitle(ctx, rec),
    // нет даты рождения — только «– смерть», без подставленного начала (F-42)
    dates: to ? `${from} – ${to}`.trim() : from,
    note: rec.note ?? '',
    dead: isEnded(ctx, rec),
  }
}
