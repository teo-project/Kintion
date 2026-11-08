// Подсветка линий при наведении (F-127): линия и линии вверх от неё — к узлу,
// из которого она выходит, и к его участникам (линия ребёнка → линии брака).
// Вниз не идём, карточки не трогаем. Связь «Брат / сестра» — одно отношение:
// любая её линия или плашка подсвечивает плашку и все линии к участникам. Направление — из раскладки: шина идёт
// сверху (source) вниз (target), прямая — между узлом отношения и участником.
import type { TreeLayout } from './model'

/** id линий, которые подсвечиваются при наведении на линию edgeId. */
export function upLines(layout: TreeLayout, edgeId: string): Set<string> {
  const out = new Set<string>()
  const e = layout.edges.find(x => x.id === edgeId)
  if (!e) return out
  out.add(e.id)
  const nodes = new Map(layout.nodes.map(n => [n.id, n]))
  /** узел отношения: плашка пары или её развилка (точка без человека) */
  const isUnion = (id: string) => {
    const n = nodes.get(id)
    return !!n && (n.type === 'union' || (n.type === 'dot' && !n.person))
  }
  const marriage = (u: string) => {
    for (const x of layout.edges) if (x.type === 'straight' && (x.source === u || x.target === u)) out.add(x.id)
  }
  if (e.type === 'straight') {
    for (const end of [e.source, e.target]) if (isUnion(end)) marriage(end)
    return out
  }
  if (nodes.get(e.source)?.type === 'siblings') return siblingLines(layout, e.source)
  // шина: та же семья к тому же ребёнку (родители без отношения — две линии)
  const same = e.group ? layout.edges.filter(x => x.type === 'bus' && x.group === e.group && x.target === e.target) : [e]
  for (const x of same) {
    out.add(x.id)
    // плашка пары из трёх и более — только линии к родителям этого ребёнка (F-136)
    if (x.lines) for (const id of x.lines) out.add(id)
    else if (isUnion(x.source)) marriage(x.source)
  }
  return out
}

/** Все линии плашки «Брат / сестра» к участникам. */
function siblingLines(layout: TreeLayout, nodeId: string): Set<string> {
  return new Set(layout.edges.filter(e => e.source === nodeId).map(e => e.id))
}

/** Плашка «Брат / сестра», которую подсвечивает наведение на линию edgeId или на узел nodeId. */
export function litSiblings(layout: TreeLayout, edgeId: string | null, nodeId: string | null): string | null {
  const id = edgeId ? layout.edges.find(e => e.id === edgeId)?.source : nodeId
  return id && layout.nodes.find(n => n.id === id)?.type === 'siblings' ? id : null
}

/** Наведение на плашку пары из трёх и более (F-136) или «Брат / сестра»: все её линии к участникам. */
export function jointLines(layout: TreeLayout, nodeId: string): Set<string> {
  const n = layout.nodes.find(x => x.id === nodeId)
  if (n?.type === 'siblings') return siblingLines(layout, nodeId)
  if (!n?.joint) return new Set()
  return new Set(layout.edges.filter(e => e.type === 'straight' && (e.source === nodeId || e.target === nodeId)).map(e => e.id))
}
