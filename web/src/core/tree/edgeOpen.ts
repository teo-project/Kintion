// Двойной клик по линии дерева (F-127): одно отношение — сразу его карточка,
// несколько (линия от брака к ребёнку: «Мама — ребёнок», «Папа — ребёнок») — меню.
// Отрезок, общий для нескольких линий (линии родителей без пары сходятся над ребёнком,
// отчим и опекун спускаются к тому же ребёнку), несёт отношения всех этих линий.
import { PERSON_H } from './layout'
import type { TreeEdge, TreeLayout } from './model'

export type EdgeOpen = { kind: 'none' } | { kind: 'card'; id: string } | { kind: 'menu'; ids: string[] }

export interface Pt {
  x: number
  y: number
}

type Seg = [number, number, number, number]

/** насколько точка может отстоять от отрезка, чтобы лежать на нём (общие отрезки совпадают точно) */
const ON = 1.5

/** отрезки шины, как её рисует TreeView: вниз — [обход] — вниз — вбок — вниз; height — высота узла */
export function busSegments(layout: TreeLayout, e: TreeEdge, height: (id: string) => number | undefined): Seg[] {
  if (e.type !== 'bus' || e.busY === undefined) return []
  const s = layout.nodes.find(n => n.id === e.source)
  const t = layout.nodes.find(n => n.id === e.target)
  if (!s || !t) return []
  const sx = s.x + s.w / 2
  const pts: [number, number][] = [[sx, s.y + (height(s.id) ?? s.h ?? (s.type === 'person' || s.type === 'ref' ? PERSON_H : 0))]]
  if (e.via) pts.push([sx, e.via.y], [e.via.x, e.via.y])
  const lx = pts[pts.length - 1]![0]
  const tx = t.x + t.w / 2
  pts.push([lx, e.busY], [tx, e.busY], [tx, t.y])
  const out: Seg[] = []
  for (let i = 1; i < pts.length; i++) out.push([pts[i - 1]![0], pts[i - 1]![1], pts[i]![0], pts[i]![1]])
  return out
}

/** ближайшая к p точка отрезка */
function nearest(p: Pt, [x1, y1, x2, y2]: Seg): Pt {
  const dx = x2 - x1
  const dy = y2 - y1
  const l = dx * dx + dy * dy
  const k = l ? Math.max(0, Math.min(1, ((p.x - x1) * dx + (p.y - y1) * dy) / l)) : 0
  return { x: x1 + dx * k, y: y1 + dy * k }
}

const dist = (a: Pt, b: Pt) => Math.hypot(a.x - b.x, a.y - b.y)

/**
 * Что открыть по двойному клику на линию edgeId. at — точка клика на полотне: она
 * притягивается к ближайшему отрезку линии, и к её отношениям добавляются отношения
 * всех линий, у которых в этой точке отрезок той же ориентации (общий отрезок).
 */
export function edgeOpen(layout: TreeLayout, edgeId: string, at?: Pt, height: (id: string) => number | undefined = () => undefined): EdgeOpen {
  const e = layout.edges.find(x => x.id === edgeId)
  const ids = [...(e?.rels ?? [])]
  const segs = e && at ? busSegments(layout, e, height) : []
  if (at && segs.length) {
    const seg = segs.reduce((a, b) => (dist(at, nearest(at, b)) < dist(at, nearest(at, a)) ? b : a))
    const p = nearest(at, seg)
    // только совпадающие отрезки (та же ориентация), а не пересечения под прямым углом
    const vertical = (s: Seg) => Math.abs(s[0] - s[2]) < Math.abs(s[1] - s[3])
    for (const o of layout.edges) {
      if (o === e || !o.rels?.length) continue
      if (busSegments(layout, o, height).some(s => vertical(s) === vertical(seg) && dist(p, nearest(p, s)) <= ON)) ids.push(...o.rels)
    }
  }
  const uniq = [...new Set(ids)]
  if (!uniq.length) return { kind: 'none' }
  return uniq.length === 1 ? { kind: 'card', id: uniq[0]! } : { kind: 'menu', ids: uniq }
}
