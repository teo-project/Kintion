// Выделение в дереве (F-131): Ctrl + рамка — люди, чьи узлы пересекают рамку
// (сам узел или отсылка); координаты — полотна.
import { PERSON_H } from './layout'
import type { TreeLayout } from './model'

export interface Box { x0: number; y0: number; x1: number; y1: number }

/** Люди (id записей), чьи узлы пересекают рамку; height — отрисованная высота узла (нет — с запасом). */
export function personsInBox(layout: TreeLayout, box: Box, height: (nodeId: string) => number = () => PERSON_H): string[] {
  const x0 = Math.min(box.x0, box.x1)
  const x1 = Math.max(box.x0, box.x1)
  const y0 = Math.min(box.y0, box.y1)
  const y1 = Math.max(box.y0, box.y1)
  const out = new Set<string>()
  for (const n of layout.nodes) {
    if ((n.type !== 'person' && n.type !== 'ref') || !n.person) continue
    if (n.x < x1 && n.x + n.w > x0 && n.y < y1 && n.y + height(n.id) > y0) out.add(n.person)
  }
  return [...out]
}

/** Ctrl + клик: добавить или убрать из выделения. */
export function toggle(sel: ReadonlySet<string>, id: string): Set<string> {
  const out = new Set(sel)
  if (out.has(id)) out.delete(id)
  else out.add(id)
  return out
}
