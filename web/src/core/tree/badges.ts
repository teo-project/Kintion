// Значки отсылок на основной карточке (F-117): на каждую отсылку «↪» свой круглый значок
// там, где в карточку входила бы линия к копии — луч из центра карточки к центру копии
// пересекает край. Значок — наполовину на краю; места заняты — сдвиг вдоль края (через углы
// на соседнюю сторону), весь край занят — следующее кольцо снаружи. Значки не перекрывают
// друг друга (единая проверка по всем значкам узла) и не лежат на входах настоящих линий.
import type { TreeLayout } from './model'
import { PERSON_H, PHOTO_MID } from './layout'

/** диаметр значка */
export const BADGE_D = 20
/** зазор между значками и до линий */
export const BADGE_GAP = 4
const STEP = BADGE_D + BADGE_GAP

/** вход линии в карточку: точка на краю и наружная нормаль (линия уходит от края по ней) */
export interface Entry {
  x: number
  y: number
  nx: number
  ny: number
}

export interface Placed {
  id: string
  /** центр значка в координатах карточки (левый верхний угол — 0, 0) */
  x: number
  y: number
  /** сторона карточки, у которой значок: туда приходит линия «телепортации» */
  side: Side
}

export type Side = 'top' | 'right' | 'bottom' | 'left'

/** направление к копии: смещение её центра от центра карточки */
export interface Target {
  id: string
  dx: number
  dy: number
}

/** прямоугольник кольца k: карточка, раздутая на k шагов */
interface Ring {
  x0: number
  y0: number
  W: number
  H: number
}

const ringOf = (w: number, h: number, k: number): Ring => ({ x0: 0 - k * STEP, y0: 0 - k * STEP, W: w + 2 * k * STEP, H: h + 2 * k * STEP })
const perim = (r: Ring) => 2 * (r.W + r.H)

/** точка периметра по длине дуги от левого верхнего угла по часовой стрелке */
function pointAt(r: Ring, s0: number): [number, number, Side] {
  const P = perim(r)
  const s = ((s0 % P) + P) % P
  if (s < r.W) return [r.x0 + s, r.y0, 'top']
  if (s < r.W + r.H) return [r.x0 + r.W, r.y0 + s - r.W, 'right']
  if (s < 2 * r.W + r.H) return [r.x0 + r.W - (s - r.W - r.H), r.y0 + r.H, 'bottom']
  return [r.x0, r.y0 + r.H - (s - 2 * r.W - r.H), 'left']
}

/** длина дуги до точки, где луч из центра в направлении (dx, dy) пересекает кольцо */
function rayParam(r: Ring, dx: number, dy: number): number {
  if (!dx && !dy) dy = -1
  const cx = r.x0 + r.W / 2
  const cy = r.y0 + r.H / 2
  const t = Math.min(dx ? r.W / 2 / Math.abs(dx) : Infinity, dy ? r.H / 2 / Math.abs(dy) : Infinity)
  const x = cx + dx * t
  const y = cy + dy * t
  const eps = 1e-6
  if (Math.abs(y - r.y0) < eps) return x - r.x0
  if (Math.abs(x - (r.x0 + r.W)) < eps) return r.W + (y - r.y0)
  if (Math.abs(y - (r.y0 + r.H)) < eps) return r.W + r.H + (r.x0 + r.W - x)
  return 2 * r.W + r.H + (r.y0 + r.H - y)
}

/** расстояние от точки до луча линии (от входа наружу) */
function toEntry(px: number, py: number, e: Entry): number {
  const t = Math.max(0, (px - e.x) * e.nx + (py - e.y) * e.ny)
  return Math.hypot(px - (e.x + e.nx * t), py - (e.y + e.ny * t))
}

/**
 * Размещение значков на карточке w×h. Порядок — по месту на краю (по часовой от верхнего
 * левого угла), каждый значок — в ближайшее свободное место к своему: центры значков не
 * ближе BADGE_D + BADGE_GAP, до луча любой линии — не ближе BADGE_D / 2 + BADGE_GAP.
 * Сдвиг вдоль кольца — не дальше половины длинной стороны (значок остаётся со стороны
 * копии, может зайти за угол); дальше — следующее кольцо снаружи.
 */
export function placeBadges(w: number, h: number, targets: Target[], entries: Entry[]): Placed[] {
  const r0 = ringOf(w, h, 0)
  const want = targets
    .map(t => ({ t, s: rayParam(r0, t.dx, t.dy) }))
    .sort((a, b) => a.s - b.s || (a.t.id < b.t.id ? -1 : a.t.id > b.t.id ? 1 : 0))
  const out: Placed[] = []
  const free = (x: number, y: number) =>
    out.every(p => Math.hypot(p.x - x, p.y - y) >= STEP - 1e-6) && entries.every(e => toEntry(x, y, e) >= BADGE_D / 2 + BADGE_GAP - 1e-6)
  for (const { t } of want) {
    for (let k = 0; ; k++) {
      const r = ringOf(w, h, k)
      const P = perim(r)
      const s0 = rayParam(r, t.dx, t.dy)
      const dev = Math.min(P / 2, Math.max(r.W, r.H) / 2 + STEP)
      let hit: [number, number, Side] | null = null
      for (let d = 0; d <= dev && !hit; d++) {
        for (const s of d ? [s0 + d, s0 - d] : [s0]) {
          const pt = pointAt(r, s)
          if (free(pt[0], pt[1])) {
            hit = pt
            break
          }
        }
      }
      if (hit) {
        out.push({ id: t.id, x: hit[0], y: hit[1], side: hit[2] })
        break
      }
    }
  }
  return out
}

/** вход линии по ручке узла (ручки — как в TreeView: верх/низ по центру, бока — на уровне фото) */
function entryOf(handle: string, w: number, h: number): Entry {
  if (handle === 'b') return { x: w / 2, y: h, nx: 0, ny: 1 }
  if (handle === 'l') return { x: 0, y: PHOTO_MID, nx: -1, ny: 0 }
  if (handle === 'r') return { x: w, y: PHOTO_MID, nx: 1, ny: 0 }
  return { x: w / 2, y: 0, nx: 0, ny: -1 }
}

export interface RefBadge extends Placed {
  /** сосед по паре, рядом с которым стоит копия, и подпись отношения (подсказка) */
  near?: string
  nearLabel?: string
}

/**
 * Значки по основным узлам-людям: id узла → значки (id значка — id отсылки).
 * size — настоящие размеры узла на полотне (высота карточки — по содержимому).
 */
export function refBadges(layout: TreeLayout, size: (id: string) => { w: number; h: number } | undefined): Map<string, RefBadge[]> {
  const out = new Map<string, RefBadge[]>()
  const mains = new Map(layout.nodes.filter(n => n.type === 'person' && n.person).map(n => [n.person!, n]))
  const byMain = new Map<string, typeof layout.nodes>()
  for (const r of layout.nodes) {
    const m = r.type === 'ref' && r.person ? mains.get(r.person) : undefined
    if (m) (byMain.get(m.id) ?? byMain.set(m.id, []).get(m.id)!).push(r)
  }
  const dims = (n: { id: string; w: number }) => {
    const s = size(n.id)
    return { w: s?.w || n.w, h: s?.h || PERSON_H }
  }
  for (const [mid, refs] of byMain) {
    const m = mains.get(refs[0]!.person!)!
    const { w, h } = dims(m)
    // входы настоящих линий (линия «телепортации» приходит в сам значок)
    const handles = new Set<string>()
    for (const e of layout.edges) {
      if (e.source === mid) handles.add(e.sourceHandle)
      if (e.target === mid) handles.add(e.targetHandle)
    }
    const entries = [...handles].map(hd => entryOf(hd, w, h))
    const targets = refs.map(r => {
      const rd = dims(r)
      return { id: r.id, dx: r.x + rd.w / 2 - (m.x + w / 2), dy: r.y + rd.h / 2 - (m.y + h / 2) }
    })
    const byId = new Map(refs.map(r => [r.id, r]))
    out.set(
      mid,
      placeBadges(w, h, targets, entries).map(p => ({ ...p, near: byId.get(p.id)?.near, nearLabel: byId.get(p.id)?.nearLabel })),
    )
  }
  return out
}
