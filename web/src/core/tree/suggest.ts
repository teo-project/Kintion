// Подсказки «Существующий» в быстром добавлении (F-130): сначала дети
// партнёра по паре; затем ближайшие по раскладке в нужную сторону (ребёнок —
// ниже, родитель — выше, партнёр — рядом по ряду) и все в пределах двух шагов
// по связям дерева — на случай отсылок на другом конце полотна. Только люди
// из модели дерева, то есть видимые (F-07). Шаблонов код не знает: сторона —
// по роли новой записи в отношении с tree_link.
import type { TreeInput, TreeLayout } from './model'

/** Куда от записи встанет выбранный: ребёнок — ниже, родитель — выше, пара — рядом. */
export type Side = 'down' | 'up' | 'same'

/** сколько ближайших по раскладке брать сверх детей партнёра и двух шагов */
const NEAREST = 6

export function suggestExisting(input: TreeInput, layout: TreeLayout, me: string, side: Side): string[] {
  const parentsOf = (id: string) => input.descents.filter(d => d.child === id).map(d => d.parent)
  const kidsOf = (id: string) => input.descents.filter(d => d.parent === id).map(d => d.child)
  const partnersOf = (id: string) => input.couples.flatMap(c => (c.a === id ? [c.b] : c.b === id ? [c.a] : []))
  // уже связанные так же — не предлагать
  const taken = new Set([me, ...(side === 'down' ? kidsOf(me) : side === 'up' ? parentsOf(me) : partnersOf(me))])

  // узлы человека: сам и отсылки — расстояние до ближайшего из них
  const at = new Map<string, { x: number; y: number; row: number }[]>()
  for (const n of layout.nodes) {
    if (!n.person || (n.type !== 'person' && n.type !== 'ref')) continue
    ;(at.get(n.person) ?? at.set(n.person, []).get(n.person)!).push({ x: n.x + n.w / 2, y: n.y, row: n.row })
  }
  const mine = at.get(me) ?? []
  const myRow = layout.nodes.find(n => n.type === 'person' && n.person === me)?.row ?? mine[0]?.row
  const rowOf = (id: string) => layout.nodes.find(n => n.type === 'person' && n.person === id)?.row ?? at.get(id)?.[0]?.row
  const fits = (id: string) => {
    const r = rowOf(id)
    if (r === undefined || myRow === undefined) return false
    return side === 'down' ? r > myRow : side === 'up' ? r < myRow : Math.abs(r - myRow) <= 1
  }
  const dist = (id: string) => {
    let best = Infinity
    for (const a of at.get(id) ?? []) for (const b of mine) best = Math.min(best, Math.hypot(a.x - b.x, a.y - b.y))
    return best
  }

  const out: string[] = []
  const push = (id: string) => {
    if (!taken.has(id) && !out.includes(id)) out.push(id)
  }
  // 1. дети партнёра по паре (для ребёнка)
  if (side === 'down') for (const p of partnersOf(me)) for (const k of kidsOf(p).sort((x, y) => dist(x) - dist(y))) push(k)

  // соседи по связям дерева: пары, родитель — ребёнок, братья / сёстры
  const near = (id: string) => [
    ...parentsOf(id), ...kidsOf(id), ...partnersOf(id),
    ...input.siblings.filter(s => s.members.includes(id)).flatMap(s => s.members),
  ]
  const steps = new Set<string>()
  for (const a of near(me)) {
    steps.add(a)
    for (const b of near(a)) steps.add(b)
  }
  // 2. ближайшие по раскладке в нужную сторону и в пределах двух шагов — вместе, по расстоянию
  const all = input.persons.map(p => p.id).filter(id => fits(id) && !taken.has(id))
  const byDist = [...all].sort((x, y) => dist(x) - dist(y) || (x < y ? -1 : 1))
  const pick = new Set([...byDist.slice(0, NEAREST), ...byDist.filter(id => steps.has(id))])
  for (const id of byDist) if (pick.has(id)) push(id)
  return out
}
