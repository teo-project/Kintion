// Подсветка линий при наведении (F-127): линия и линии вверх от неё — к узлу,
// из которого она выходит, и к его участникам (линия ребёнка → линии брака).
// Вниз не идём, карточки не трогаем. Направление — из раскладки: шина идёт
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
  // шина: та же семья к тому же ребёнку (родители без отношения — две линии)
  const same = e.group ? layout.edges.filter(x => x.type === 'bus' && x.group === e.group && x.target === e.target) : [e]
  for (const x of same) {
    out.add(x.id)
    if (isUnion(x.source)) marriage(x.source)
  }
  return out
}
