// Своя раскладка дерева (F-24). Ряды — по поколениям с проскоком для поздних
// детей (F-115); центра нет (F-110): корни — семьи с самым большим потомством,
// предки остальных — над их детьми в ближайшем свободном месте ряда. Дети
// висят на узле пары; без пары линии от родителей сходятся над ребёнком
// (F-112); петли — отсылки (F-117); «Скрыть умерших» — развилки (F-119).
import type { TreeEdge, TreeInput, TreeLayout, TreeNode, TreePerson, TreeSiblings } from './model'

/** ширина узла человека (F-129: на 15–20% шире прежних 132) */
export const PERSON_W = 156
/** высота узла человека с запасом (настоящая — по содержимому, ФИО полностью — F-123) */
export const PERSON_H = 300
/** середина фото от верха узла: на ней линии к узлу отношения (отступ 8 + половина фото 3:4) */
export const PHOTO_MID = 8 + Math.round(((PERSON_W - 16) * 4) / 3 / 2)
/** наименьшая ширина плашки отношения; иначе — по тексту (F-129) */
export const UNION_W = 44
export const UNION_H = 22
export const SIBLINGS_W = 100
export const DOT = 8
export const MANY_W = 24
const GAP = 24
/** шаг рядов: ±1,5 поколения — три ряда на экране (F-129) */
export const ROW = PERSON_H + 96
/** подпись длиннее — в две строки (F-129) */
const LABEL_ONE_LINE = 30
/** ширина символа подписи 10px с запасом, поля плашки */
const LABEL_CHAR = 6
const LABEL_PAD = 16
const LABEL_LINE = 13
/** ряд только с развилками и пустой ряд — сжаты (F-119) */
const DOT_ROW = 110
const EMPTY_ROW = 24
const DOT_MID = 24
const NOREL_GAP = 40
/** шаг разведения горизонталей шин над рядом (F-112) */
const BUS_STEP = 8
const BUS_LEVELS = 5
const SIB_RISE = 72
/** условное поколение в годах — для рядов несвязанных частей */
const GENERATION = 25

export interface TreeOptions {
  /** сколько поколений вверх от самого молодого ряда (F-110) */
  depth: number
  /** «Скрыть умерших» (F-119) */
  hideDead?: boolean
  /** ширина строки подписи 10px в пикселях (измерение в браузере); нет — оценка по числу символов */
  measure?: (text: string) => number
}

/**
 * Строки подписи на линии (F-129): целиком; длиннее ~30 символов — две
 * строки, разрыв по пробелу ближе к середине; без пробела — посередине слова. Не обрезается.
 */
export function labelLines(label: string): string[] {
  // по символам (кодовым точкам), не по половинкам UTF-16
  const s = Array.from(label.trim())
  if (s.length <= LABEL_ONE_LINE) return [s.join('')]
  const mid = s.length / 2
  let at = -1
  for (let i = 0; i < s.length; i++) if (s[i] === ' ' && (at < 0 || Math.abs(i - mid) < Math.abs(at - mid))) at = i
  if (at < 0) {
    const half = Math.ceil(s.length / 2)
    return [s.slice(0, half).join(''), s.slice(half).join('')]
  }
  return [s.slice(0, at).join(''), s.slice(at + 1).join('')]
}

const estimate = (text: string) => Array.from(text).length * LABEL_CHAR

/** Плашка подписи по тексту: ширина и высота (F-129); measure — ширина строки в браузере. */
export function labelBox(label: string, min = UNION_W, measure: (text: string) => number = estimate): { w: number; h: number } {
  const lines = labelLines(label)
  const w = Math.max(min, Math.ceil(Math.max(...lines.map(measure)) + LABEL_PAD))
  return { w, h: UNION_H + (lines.length - 1) * LABEL_LINE }
}

interface Person extends TreePerson {
  /** развилка вместо группы скрытых (F-119) */
  junction?: 'dot' | 'many'
}

interface Kid {
  id: string
  /** отношения «родитель — ребёнок», по которым ребёнок висит на этой семье */
  rels?: { id: string; parent: string }[]
  dashed?: boolean
  label?: string
}

/** Семья: пара (rel — отношение) или родители без отношения / один родитель (rel null). */
interface Family {
  id: string
  rel: string | null
  label: string
  a: string
  /** '' — один родитель */
  b: string
  order: string
  kids: Kid[]
}

interface Link {
  id: string
  parent: string
  child: string
  dashed?: boolean
  label?: string
}

interface Graph {
  persons: Map<string, Person>
  families: Family[]
  siblings: TreeSiblings[]
  links: Link[]
}

const byId = <T extends { id: string }>(a: T, b: T) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0)

/**
 * Семьи из связей «родитель — ребёнок»: ребёнок висит на паре, где оба его
 * родителя (родные — раньше неродных); нет пары — на двух родителях без
 * отношения; один родитель — на нём. Прочие связи — отдельные линии.
 */
export function buildGraph(input: TreeInput): Graph {
  const persons = new Map(input.persons.map(p => [p.id, { ...p } as Person]))
  const families: Family[] = input.couples
    .filter(c => persons.has(c.a) && persons.has(c.b) && c.a !== c.b)
    .map(c => ({ id: c.id, rel: c.relation ?? c.id, label: c.label, a: c.a, b: c.b, order: c.order, kids: [] }))
  const byChild = new Map<string, TreeInput['descents']>()
  for (const d of [...input.descents].sort(byId)) {
    if (!persons.has(d.parent) || !persons.has(d.child) || d.parent === d.child) continue
    const list = byChild.get(d.child) ?? byChild.set(d.child, []).get(d.child)!
    if (!list.some(x => x.parent === d.parent)) list.push(d)
  }
  const links: Link[] = []
  const extra = new Map<string, Family>()
  const familyOf = (id: string, a: string, b: string) =>
    extra.get(id) ?? extra.set(id, { id, rel: null, label: '', a, b, order: '9999', kids: [] }).get(id)!
  for (const child of [...byChild.keys()].sort()) {
    const ds = byChild.get(child)!.sort((x, y) => Number(!!x.dashed) - Number(!!y.dashed))
    let best: { f: Family; used: TreeInput['descents']; score: number } | null = null
    for (const f of families) {
      const da = ds.find(d => d.parent === f.a)
      const db = ds.find(d => d.parent === f.b)
      if (!da || !db) continue
      const score = Number(!!da.dashed) + Number(!!db.dashed)
      if (!best || score < best.score || (score === best.score && f.order < best.f.order)) best = { f, used: [da, db], score }
    }
    let used: TreeInput['descents']
    let fam: Family
    if (best && !(best.score > 0 && ds.filter(d => !d.dashed).length >= 2)) {
      fam = best.f
      used = best.used
    } else {
      // без пары: родные вместе (неродной — отдельной линией); родных нет — неродные
      const solid = ds.filter(d => !d.dashed)
      used = (solid.length ? solid : ds).slice(0, 2)
      if (used.length === 2) {
        const [a, b] = used.map(d => d.parent).sort() as [string, string]
        fam = familyOf(`gap:${a}|${b}`, a, b)
      } else fam = familyOf(`solo:${used[0]!.parent}`, used[0]!.parent, '')
    }
    const dashed = used.every(d => d.dashed)
    fam.kids.push({ id: child, rels: used.map(d => ({ id: d.id, parent: d.parent })), dashed, label: dashed ? (used.length > 1 ? used[0]!.kind : used[0]!.label) : undefined })
    for (const d of ds) if (!used.includes(d)) links.push({ id: d.id, parent: d.parent, child, dashed: d.dashed, label: d.label })
  }
  const siblings = input.siblings
    .map(s => ({ ...s, members: [...new Set(s.members)].filter(m => persons.has(m)) }))
    .filter(s => s.members.length > 1)
  return { persons, families: [...families, ...extra.values()], siblings, links }
}

/** Связи графа для обходов. */
function topology(g: Graph) {
  const famOf = new Map<string, Family>()
  const famsOf = new Map<string, Family[]>()
  for (const f of g.families) {
    for (const k of f.kids) famOf.set(k.id, f)
    for (const p of [f.a, f.b]) if (p) (famsOf.get(p) ?? famsOf.set(p, []).get(p)!).push(f)
  }
  for (const list of famsOf.values()) list.sort((x, y) => (x.order < y.order ? -1 : x.order > y.order ? 1 : byId(x, y)))
  const other = (f: Family, id: string) => (f.a === id ? f.b : f.a)
  const partners = (id: string) => (famsOf.get(id) ?? []).map(f => other(f, id)).filter(Boolean)
  const linkKids = new Map<string, string[]>()
  const linkParents = new Map<string, string[]>()
  for (const l of g.links) {
    (linkKids.get(l.parent) ?? linkKids.set(l.parent, []).get(l.parent)!).push(l.child)
    ;(linkParents.get(l.child) ?? linkParents.set(l.child, []).get(l.child)!).push(l.parent)
  }
  /** дети: в семьях и по отдельным линиям (неродной родитель тоже выше ребёнка) */
  const kidsOf = (id: string) => [...(famsOf.get(id) ?? []).flatMap(f => f.kids.map(k => k.id)), ...(linkKids.get(id) ?? [])]
  /** родители: в своей семье и по отдельным линиям */
  const parentsOf = (id: string) => {
    const f = famOf.get(id)
    return [...(f ? [f.a, f.b].filter(Boolean) : []), ...(linkParents.get(id) ?? [])]
  }
  return { famOf, famsOf: (id: string) => famsOf.get(id) ?? [], other, partners, kidsOf, parentsOf }
}
type Topo = ReturnType<typeof topology>

/** Род: корни и их потомки, в том числе дети супругов от других браков (L); супруги рода (M). */
function lineage(g: Graph, roots: string[]) {
  const L = new Set(roots)
  const M = new Set<string>()
  for (let changed = true; changed; ) {
    changed = false
    for (const f of g.families) {
      for (const [x, y] of [[f.a, f.b], [f.b, f.a]] as const) {
        if (y && L.has(x) && !L.has(y) && !M.has(y)) {
          M.add(y)
          changed = true
        }
      }
      if ([f.a, f.b].some(p => p && (L.has(p) || M.has(p)))) {
        for (const k of f.kids) {
          if (L.has(k.id)) continue
          L.add(k.id)
          M.delete(k.id)
          changed = true
        }
      }
    }
  }
  return { L, M }
}

/**
 * Корни раскладки: люди без родителей (группа «Брат / сестра» из таких —
 * целиком), сначала — с самым большим потомством. Корень, чьё потомство уже
 * в роду, не берётся: его предки встанут над детьми (F-110).
 */
function chooseRoots(g: Graph, t: Topo) {
  const year = (id: string) => g.persons.get(id)?.year ?? Infinity
  const tops = [...g.persons.keys()].filter(id => !t.famOf.has(id))
  const topSet = new Set(tops)
  const units: { members: string[]; sib?: TreeSiblings }[] = []
  const inUnit = new Set<string>()
  for (const s of g.siblings) {
    if (s.members.every(m => topSet.has(m) && !inUnit.has(m))) {
      units.push({ members: [...s.members].sort((x, y) => year(x) - year(y) || (x < y ? -1 : 1)), sib: s })
      for (const m of s.members) inUnit.add(m)
    }
  }
  for (const id of tops) if (!inUnit.has(id)) units.push({ members: [id] })
  const desc = (members: string[]) => {
    const seen = new Set(members)
    const q = [...members]
    for (let i = 0; i < q.length; i++) {
      for (const k of t.kidsOf(q[i]!)) {
        if (!seen.has(k)) {
          seen.add(k)
          q.push(k)
        }
      }
    }
    return seen
  }
  const sized = units.map(u => ({ ...u, desc: desc(u.members), year: Math.min(...u.members.map(year)) }))
  sized.sort((x, y) => y.desc.size - x.desc.size || x.year - y.year || (x.members[0]! < y.members[0]! ? -1 : 1))
  const chosen: typeof sized = []
  let lin = lineage(g, [])
  for (const u of sized) {
    const covered = (id: string) => lin.L.has(id) || lin.M.has(id)
    if ([...u.desc].some(covered)) continue
    chosen.push(u)
    lin = lineage(g, chosen.flatMap(c => c.members))
  }
  return { units: chosen, ...lin }
}

/**
 * Ряды (F-115) по полному графу: ребёнок рода — ниже родителей, поздний
 * (младшему родителю 40+ / 50+) — на 1–2 ряда ниже; супруг — в ряду супруга;
 * предки не из рода — на ряд выше детей. Несвязанные части — по годам.
 */
function computeRows(g: Graph, t: Topo): Map<string, number> {
  const { units, L } = chooseRoots(g, t)
  const row = new Map<string, number>()
  const ids = [...g.persons.keys()].sort()
  const year = (id: string) => g.persons.get(id)?.year
  const late = (id: string, f: Family) => {
    const py = [f.a, f.b].map(p => (p ? year(p) : undefined)).filter((y): y is number => y !== undefined)
    const y = year(id)
    if (y === undefined || !py.length) return 0
    const age = y - Math.max(...py)
    return age >= 50 ? 2 : age >= 40 ? 1 : 0
  }
  const below = (id: string, f: Family) => {
    const rs = [f.a, f.b].filter(Boolean).map(p => row.get(p)).filter((r): r is number => r !== undefined)
    return rs.length ? Math.max(...rs) + 1 + late(id, f) : undefined
  }
  const relax = () => {
    for (let it = 0; it < ids.length + 2; it++) {
      let changed = false
      const set = (id: string, r: number | undefined) => {
        if (r === undefined || row.get(id) === r) return
        row.set(id, r)
        changed = true
      }
      for (const id of ids) {
        const f = t.famOf.get(id)
        if (L.has(id) && f && below(id, f) !== undefined) {
          set(id, below(id, f))
          continue
        }
        if (row.has(id)) continue
        const pr = t.partners(id).sort((x, y) => Number(L.has(y)) - Number(L.has(x))).map(p => row.get(p)).find(r => r !== undefined)
        if (pr !== undefined) {
          set(id, pr)
          continue
        }
        const kr = t.kidsOf(id).map(k => row.get(k)).filter((r): r is number => r !== undefined)
        if (kr.length) {
          set(id, Math.min(...kr) - 1)
          continue
        }
        if (f) set(id, below(id, f))
      }
      if (!changed) break
    }
  }
  /** ряд по году рождения относительно уже разложенных */
  const estimate = (members: string[]) => {
    const ys = members.map(year).filter((y): y is number => y !== undefined)
    const fit = [...row].map(([id, r]) => (year(id) ?? NaN) - GENERATION * r).filter(v => !Number.isNaN(v)).sort((x, y) => x - y)
    if (!ys.length || !fit.length) return 0
    return Math.round((Math.min(...ys) - fit[Math.floor(fit.length / 2)]!) / GENERATION)
  }
  for (const u of units) {
    if (!u.members.some(m => row.has(m))) {
      const r = estimate(u.members)
      for (const m of u.members) row.set(m, r)
    }
    relax()
  }
  for (const id of ids) {
    if (row.has(id)) continue
    row.set(id, estimate([id]))
    relax()
  }
  // родитель всегда выше ребёнка — и по отдельным линиям (неродной, опекун)
  const pairs: [string, string][] = [
    ...g.families.flatMap(f => f.kids.flatMap(k => [f.a, f.b].filter(Boolean).map(p => [p, k.id] as [string, string]))),
    ...g.links.map(l => [l.parent, l.child] as [string, string]),
  ]
  // супруг не из рода — в ряду супруга из рода (F-112)
  const spouses = g.families.filter(f => f.rel && f.b && L.has(f.a) !== L.has(f.b)).map(f => (L.has(f.a) ? [f.a, f.b] : [f.b, f.a]) as [string, string])
  for (let it = 0; it < ids.length + 2; it++) {
    let changed = false
    for (const [p, c] of pairs) {
      if (row.get(c)! > row.get(p)!) continue
      row.set(c, row.get(p)! + 1)
      changed = true
    }
    for (const [l, o] of spouses) {
      if (row.get(o) === row.get(l) || t.kidsOf(o).some(k => row.get(k)! <= row.get(l)!)) continue
      if (t.parentsOf(o).some(p => row.get(p)! >= row.get(l)!)) continue
      row.set(o, row.get(l)!)
      changed = true
    }
    if (!changed) break
  }
  const min = Math.min(...row.values())
  for (const [id, r] of row) row.set(id, r - min)
  return row
}

/** Срез глубины (F-110): остаются ряды не выше `minRow`; семья с одним оставшимся родителем — без отношения. */
function cut(g: Graph, rows: Map<string, number>, minRow: number): Graph {
  const keep = (id: string) => !!id && (rows.get(id) ?? 0) >= minRow
  const persons = new Map([...g.persons].filter(([id]) => keep(id)))
  const families: Family[] = []
  for (const f of g.families) {
    const kids = f.kids.filter(k => keep(k.id))
    if (keep(f.a) && (!f.b || keep(f.b))) families.push({ ...f, kids })
    else if (keep(f.a) || keep(f.b)) {
      if (kids.length) families.push({ ...f, rel: null, label: '', a: keep(f.a) ? f.a : f.b, b: '', kids })
    }
  }
  return {
    persons,
    families,
    siblings: g.siblings.map(s => ({ ...s, members: s.members.filter(keep) })).filter(s => s.members.length > 1),
    links: g.links.filter(l => keep(l.parent) && keep(l.child)),
  }
}

/**
 * «Скрыть умерших» (F-119): скрытые (умершие; семьи и группы, где скрыты все)
 * остаются, только если связывают двух и более видимых: висячие скрытые
 * отрезаются по графу связей, каждая связная группа оставшихся — одна развилка
 * `j:N` (точка — один человек или пара, «•••» — больше).
 */
function collapse(g: Graph, rows: Map<string, number>): { g: Graph; rows: Map<string, number> } {
  const dead = (id: string) => !!g.persons.get(id)?.dead
  const adj = new Map<string, Set<string>>()
  const vtx = (v: string) => adj.get(v) ?? adj.set(v, new Set()).get(v)!
  const link = (x: string, y: string) => {
    vtx(x).add(y)
    vtx(y).add(x)
  }
  for (const id of g.persons.keys()) vtx(`P:${id}`)
  const famById = new Map(g.families.map(f => [f.id, f]))
  const sibById = new Map(g.siblings.map(s => [s.id, s]))
  for (const f of g.families) {
    link(`P:${f.a}`, `F:${f.id}`)
    if (f.b) link(`P:${f.b}`, `F:${f.id}`)
    for (const k of f.kids) link(`F:${f.id}`, `P:${k.id}`)
  }
  for (const l of g.links) link(`P:${l.parent}`, `P:${l.child}`)
  for (const s of g.siblings) for (const m of s.members) link(`S:${s.id}`, `P:${m}`)
  const hidden = (v: string) => {
    const id = v.slice(2)
    if (v.startsWith('P:')) return dead(id)
    if (v.startsWith('F:')) {
      const f = famById.get(id)!
      return dead(f.a) && (!f.b || dead(f.b))
    }
    return sibById.get(id)!.members.every(dead)
  }
  for (let changed = true; changed; ) {
    changed = false
    for (const [v, ns] of adj) {
      if (!hidden(v) || ns.size > 1) continue
      for (const n of ns) adj.get(n)!.delete(v)
      adj.delete(v)
      changed = true
    }
  }
  const alias = new Map<string, string>()
  const persons = new Map<string, Person>()
  for (const [id, p] of g.persons) if (!dead(id) && adj.has(`P:${id}`)) persons.set(id, p)
  const comps = new Map<string, string[]>()
  let n = 0
  for (const v of [...adj.keys()].sort()) {
    if (!hidden(v) || alias.has(v) || !adj.has(v)) continue
    const j = `j:${n++}`
    const comp = [v]
    alias.set(v, j)
    for (let i = 0; i < comp.length; i++) {
      for (const x of adj.get(comp[i]!)!) {
        if (hidden(x) && !alias.has(x)) {
          alias.set(x, j)
          comp.push(x)
        }
      }
    }
    const seen = new Set(comp.flatMap(x => [...adj.get(x)!].filter(y => !hidden(y))))
    if (seen.size < 2) {
      for (const x of comp) {
        for (const y of adj.get(x)!) adj.get(y)?.delete(x)
        adj.delete(x)
        alias.delete(x)
      }
      continue
    }
    const ps = comp.filter(x => x.startsWith('P:')).map(x => x.slice(2))
    const pair = ps.length === 2 && comp.some(x => {
      const f = x.startsWith('F:') ? famById.get(x.slice(2)) : undefined
      return !!f && ps.includes(f.a) && ps.includes(f.b)
    })
    persons.set(j, { id: j, junction: ps.length > 2 || (ps.length === 2 && !pair) ? 'many' : 'dot' })
    comps.set(j, ps)
  }
  const mapP = (id: string) => (id && adj.has(`P:${id}`) ? (alias.get(`P:${id}`) ?? id) : '')
  const kidsOfJ = new Map<string, Kid[]>()
  const families: Family[] = []
  for (const f of g.families) {
    const v = `F:${f.id}`
    if (!adj.has(v)) continue
    const j = alias.get(v)
    if (j) {
      const ks = kidsOfJ.get(j) ?? kidsOfJ.set(j, []).get(j)!
      for (const k of f.kids) if (mapP(k.id) === k.id && !ks.some(x => x.id === k.id)) ks.push(k)
      continue
    }
    let a = mapP(f.a)
    let b = mapP(f.b)
    if (!a) [a, b] = [b, a]
    if (a === b) b = ''
    if (!a) continue
    const kids = f.kids
      .map(k => ({ ...k, id: mapP(k.id) }))
      .filter((k, i, all) => k.id && k.id !== a && k.id !== b && all.findIndex(x => x.id === k.id) === i)
    families.push({ ...f, a, b, kids })
  }
  for (const [j, kids] of kidsOfJ) if (kids.length) families.push({ id: `jf:${j}`, rel: null, label: '', a: j, b: '', order: '9999', kids })
  const links = g.links
    .map(l => ({ ...l, parent: mapP(l.parent), child: mapP(l.child) }))
    .filter(l => l.parent && l.child && l.parent !== l.child && !(l.parent.startsWith('j:') && l.child.startsWith('j:')))
  const siblings = g.siblings
    .map(s => ({ ...s, members: s.members.filter(m => mapP(m) === m) }))
    .filter(s => s.members.length > 1)
  // ряд развилки: над её детьми; иначе — ряд супруга, ниже родителя или ряд скрытых
  const out = new Map(rows)
  for (const [j, ps] of comps) {
    const kids = families.filter(f => f.a === j && !f.b).flatMap(f => f.kids.map(k => out.get(k.id) ?? 0))
    const asPartner = families.find(f => f.b && (f.a === j || f.b === j))
    const asKid = families.find(f => f.kids.some(k => k.id === j))
    if (kids.length) out.set(j, Math.min(...kids) - 1)
    else if (asPartner) out.set(j, out.get(asPartner.a === j ? asPartner.b : asPartner.a) ?? 0)
    else if (asKid) out.set(j, (out.get(asKid.a) ?? 0) + 1)
    else out.set(j, Math.min(...ps.map(p => rows.get(p) ?? 0)))
  }
  return { g: { persons, families, siblings, links }, rows: out }
}

interface Item {
  kind: 'person' | 'ref' | 'union' | 'gap'
  id: string
  person?: string
  fam?: Family
  w: number
}

/** Разложить модель: глубина и «Скрыть умерших» пересчитывают всё полотно. */
export function layoutTree(input: TreeInput, opts: TreeOptions): TreeLayout {
  const full = buildGraph(input)
  if (!full.persons.size) return { nodes: [], edges: [] }
  const allRows = computeRows(full, topology(full))
  const maxRow = Math.max(...allRows.values())
  let g = cut(full, allRows, maxRow - Math.max(1, opts.depth) + 1)
  let rows = allRows
  if (opts.hideDead) ({ g, rows } = collapse(g, allRows))
  return place(g, new Map(rows), opts.measure)
}

function place(g: Graph, rows: Map<string, number>, measure?: (text: string) => number): TreeLayout {
  const box = (label: string, min = UNION_W) => labelBox(label, min, measure)
  const t = topology(g)
  const { units, L, M } = chooseRoots(g, t)
  const row = (id: string) => rows.get(id) ?? 0
  const junction = (id: string) => g.persons.get(id)?.junction
  const pw = (id: string) => (junction(id) ? (junction(id) === 'many' ? MANY_W + 8 : DOT * 2) : PERSON_W)

  // высоты рядов: с карточками — полные, только с развилками и пустые — сжатые
  const cardRows = new Set<number>()
  const anyRows = new Set<number>()
  for (const id of g.persons.keys()) {
    anyRows.add(row(id))
    if (!junction(id)) cardRows.add(row(id))
  }
  const ys = new Map<number, number>()
  {
    let y = 0
    for (let r = Math.min(...anyRows); r <= Math.max(...anyRows) + 1; r++) {
      ys.set(r, y)
      y += cardRows.has(r) ? ROW : anyRows.has(r) ? DOT_ROW : EMPTY_ROW
    }
  }
  const rowY = (r: number) => ys.get(r) ?? 0
  const rowMid = (r: number) => (cardRows.has(r) ? PHOTO_MID : DOT_MID)

  // у супруга рода — один «дом»: в остальных браках с родом он — отсылка
  const home = new Map<string, string>()
  for (const f of [...g.families].sort((x, y) => (x.order < y.order ? -1 : 1))) {
    for (const [x, y] of [[f.a, f.b], [f.b, f.a]] as const) if (y && L.has(x) && M.has(y) && !home.has(y)) home.set(y, f.id)
  }
  const drawnAt = (id: string) =>
    t.famsOf(id).filter(f => {
      const o = t.other(f, id)
      if (!o || !L.has(o)) return true
      if (row(o) !== row(id)) return row(o) < row(id)
      return f.a === id
    })
  const unionItem = (f: Family): Item => (f.rel ? { kind: 'union', id: f.id, fam: f, w: box(f.label).w } : { kind: 'gap', id: f.id, fam: f, w: f.b ? NOREL_GAP : 0 })
  const refItem = (p: string, f: Family): Item => ({ kind: 'ref', id: `ref:${p}:${f.id}`, person: p, fam: f, w: pw(p) })
  const personItem = (p: string): Item => ({ kind: 'person', id: p, person: p, w: pw(p) })
  const partnerItem = (f: Family, id: string): Item => {
    const o = t.other(f, id)
    return L.has(o) || (M.has(o) && home.get(o) !== f.id) || !canStand(o, row(id)) ? refItem(o, f) : { ...personItem(o), fam: f }
  }
  function personRow(id: string): Item[] {
    const fs = drawnAt(id)
    const side = (f: Family, dir: 'l' | 'r'): Item[] => {
      if (!f.b) return [unionItem(f)]
      const pi = partnerItem(f, id)
      const out = [unionItem(f), pi]
      if (pi.kind === 'person') {
        for (const of of t.famsOf(pi.id)) {
          const o = t.other(of, pi.id)
          if (of === f || L.has(o)) continue
          out.push(unionItem(of))
          // супруг встанет в ряд id — проверять по нему, а не по прежнему ряду (Q-36)
          if (o) out.push(canStand(o, row(id)) ? { ...personItem(o), fam: of } : refItem(o, of))
        }
      }
      return dir === 'r' ? out : out.reverse()
    }
    const self = personItem(id)
    if (!fs.length) return [self]
    if (fs.length === 1) return [self, ...side(fs[0]!, 'r')]
    return [...side(fs[0]!, 'l'), self, ...fs.slice(1).flatMap(f => side(f, 'r'))]
  }
  const kidsOf = (items: Item[]) =>
    [...new Set(items.filter(it => it.fam && it.kind !== 'ref').flatMap(it => it.fam!.kids.map(k => k.id).filter(k => L.has(k))))]
  const rowWidth = (items: Item[]) => items.reduce((s, it) => s + it.w, 0) + GAP * Math.max(0, items.length - 1)
  const widthMemo = new Map<string, number>()
  const visiting = new Set<string>()
  function width(id: string): number {
    const m = widthMemo.get(id)
    if (m !== undefined) return m
    if (visiting.has(id)) return PERSON_W
    visiting.add(id)
    const items = personRow(id)
    const kids = kidsOf(items)
    const kw = kids.reduce((s, k) => s + width(k), 0) + GAP * 2 * Math.max(0, kids.length - 1)
    const w = Math.max(rowWidth(items), kw)
    visiting.delete(id)
    widthMemo.set(id, w)
    return w
  }

  const nodes: TreeNode[] = []
  const edges: TreeEdge[] = []
  const pos = new Map<string, number>()
  const drawn = new Set<string>()
  const famNode = new Map<string, string>()
  const refOf = new Map<string, string>()
  const spans = new Map<number, [number, number][]>()
  const occupy = (r: number, x0: number, x1: number) => (spans.get(r) ?? spans.set(r, []).get(r)!).push([x0, x1])

  function placeRow(all: Item[], left: number, r: number) {
    // узел отношения уже стоит (петля из пар: A–B, B–C, A–C) — второй раз
    // не ставится, как и отсылка к партнёру рядом с ним (Q-36)
    const skip = new Set<number>()
    const seen = new Set<string>()
    all.forEach((it, i) => {
      if (it.kind !== 'union') return
      const f = it.fam!
      if (pos.has(f.id) || seen.has(f.id)) {
        skip.add(i)
        for (const j of [i - 1, i + 1]) {
          const n = all[j]
          if (!n || n.fam !== f) continue
          // партнёр здесь был бы отсылкой: уже стоит или стоит раньше в этом ряду
          const already = n.kind === 'person' && (pos.has(n.id) || all.slice(0, j).some(x => x.kind === 'person' && x.id === n.id))
          if (n.kind === 'ref' || already) skip.add(j)
        }
      }
      seen.add(f.id)
    })
    const items = all.filter((_, i) => !skip.has(i))
    const y = rowY(r)
    const mid = y + rowMid(r)
    let x = left
    const placed: (Item & { node?: string })[] = []
    for (const it0 of items) {
      // уже стоит на полотне или не может стоять в этом ряду (ряды сдвинулись) — здесь отсылка
      const it = it0.kind === 'person' && it0.fam && (pos.has(it0.id) || (row(it0.id) !== r && !canStand(it0.id, r))) ? refItem(it0.id, it0.fam) : it0
      const cx = x + it.w / 2
      let node: string | undefined
      if (it.kind === 'person' || it.kind === 'ref') {
        const p = it.person!
        if (it.kind === 'person' && pos.has(p)) {
          x += it.w + GAP
          continue
        }
        const j = junction(p)
        node = it.id
        if (j) {
          const w = j === 'many' ? MANY_W : DOT
          nodes.push({ id: it.id, type: 'dot', x: cx - w / 2, y: mid - DOT / 2, w, row: r, person: p, many: j === 'many' })
        } else {
          const near = it.kind === 'ref' && it.fam ? (it.fam.a === p ? it.fam.b : it.fam.a) : ''
          nodes.push({ id: it.id, type: it.kind, x: cx - PERSON_W / 2, y, w: PERSON_W, row: r, person: p, ...(near ? { near, nearLabel: it.fam!.rel ? it.fam!.label : undefined } : {}) })
        }
        if (it.kind === 'person') {
          pos.set(p, cx)
          rows.set(p, r)
        } else {
          pos.set(it.id, cx)
          if (it.fam) refOf.set(`${it.fam.id}:${p}`, it.id)
        }
        occupy(r, cx - it.w / 2, cx + it.w / 2)
      } else if (it.kind === 'union') {
        const f = it.fam!
        node = f.id
        const lb = box(f.label)
        if (junction(f.a) && (!f.b || junction(f.b))) nodes.push({ id: f.id, type: 'dot', x: cx - DOT / 2, y: mid - DOT / 2, w: DOT, row: r })
        else nodes.push({ id: f.id, type: 'union', x: cx - lb.w / 2, y: mid - lb.h / 2, w: lb.w, h: lb.h, row: r, relation: f.rel!, label: f.label })
        pos.set(f.id, cx)
        famNode.set(f.id, f.id)
        occupy(r, cx - lb.w / 2, cx + lb.w / 2)
      }
      if (it.fam) drawn.add(it.fam.id)
      placed.push({ ...it, node })
      x += it.w + GAP
    }
    placed.forEach((it, i) => {
      if (it.kind !== 'union') return
      // линия — только к участнику этой пары (соседом мог стать чужой после пропуска дубля)
      const mine = (n: (typeof placed)[number] | undefined) => !!n && (n.person === it.fam!.a || n.person === it.fam!.b)
      const l = mine(placed[i - 1]) ? placed[i - 1] : undefined
      const rr = mine(placed[i + 1]) ? placed[i + 1] : undefined
      if (l?.node && l.kind !== 'gap') edges.push({ id: `s:${l.node}>${it.node}`, type: 'straight', source: l.node, sourceHandle: 'r', target: it.node!, targetHandle: 'l', rels: [it.fam!.rel!] })
      if (rr?.node && rr.kind !== 'gap') edges.push({ id: `s:${it.node}>${rr.node}`, type: 'straight', source: it.node!, sourceHandle: 'r', target: rr.node, targetHandle: 'l', rels: [it.fam!.rel!] })
    })
  }

  function placeTree(id: string, left: number) {
    if (pos.has(id)) return
    const items = personRow(id)
    const w = width(id)
    // место по ширине поддерева; занято (петли, ряды с проскоком) — ближайшее свободное
    placeFree(items, row(id), left + (w - rowWidth(items)) / 2)
    const kids = kidsOf(items)
    const kw = kids.reduce((s, k) => s + width(k), 0) + GAP * 2 * Math.max(0, kids.length - 1)
    let kx = left + (w - kw) / 2
    for (const k of kids) {
      placeTree(k, kx)
      kx += width(k) + GAP * 2
    }
  }

  /** ближайшее к want свободное место ряда */
  function placeFree(items: Item[], r: number, want: number) {
    const w = rowWidth(items)
    const occ = spans.get(r) ?? []
    const fits = (x: number) => occ.every(([a, b]) => x + w <= a - GAP || x >= b + GAP)
    let at = want
    for (let d = 0; d < 100000; d += 8) {
      if (fits(want + d)) {
        at = want + d
        break
      }
      if (fits(want - d)) {
        at = want - d
        break
      }
    }
    placeRow(items, at, r)
  }
  /** может ли человек стоять в ряду r: родители выше, дети ниже (F-115) */
  const canStand = (id: string, r: number) => t.kidsOf(id).every(k => row(k) > r) && t.parentsOf(id).every(p => row(p) < r)
  const right = () => Math.max(0, ...[...spans.values()].flat().map(([, b]) => b))

  const sibBox = (label: string) => box(label, SIBLINGS_W)
  const sibDrawn = new Set<string>()
  const busCount = new Map<number, number>()
  const nextBus = (childRow: number) => {
    const n = busCount.get(childRow) ?? 0
    busCount.set(childRow, n + 1)
    return rowY(childRow) - 22 - (n % BUS_LEVELS) * BUS_STEP
  }
  interface Bus { edge: TreeEdge; fromRow: number; toRow: number }
  const buses: Bus[] = []
  const bus = (id: string, source: string, target: string, busY: number, fromRow: number, toRow: number, extra: Partial<TreeEdge> = {}) => {
    const edge: TreeEdge = { id, type: 'bus', source, sourceHandle: 'b', target, targetHandle: 't', busY, ...extra }
    edges.push(edge)
    buses.push({ edge, fromRow, toRow })
  }

  // корни
  let left = 0
  for (const u of units) {
    const start = left
    for (const m of u.members) {
      placeTree(m, left)
      left += width(m) + GAP * 3
    }
    if (u.sib && u.members.length > 1) {
      const s = u.sib
      const cx = (start + left - GAP * 3) / 2
      const mr = row(u.members[0]!)
      nodes.push({ id: s.id, type: 'siblings', x: cx - sibBox(s.label).w / 2, y: rowY(mr) - SIB_RISE, ...sibBox(s.label), row: mr - 1, relation: s.id, label: s.label })
      for (const m of u.members) bus(`b:${s.id}>${m}`, s.id, m, rowY(mr) - 24, mr - 1, mr, { rels: [s.id] })
      sibDrawn.add(s.id)
    }
    left += GAP * 4
  }

  // остальное — жадно: предки над детьми, супруги рядом, дети под семьёй
  const ordered = [...g.families].sort((x, y) => (x.order < y.order ? -1 : x.order > y.order ? 1 : byId(x, y)))
  for (let guard = 0; guard < 4 * (g.persons.size + g.families.length) + 10; guard++) {
    let changed = false
    for (const f of ordered) {
      if (drawn.has(f.id)) continue
      const pa = pos.has(f.a)
      const pb = !!f.b && pos.has(f.b)
      if (pa || pb) {
        if (!f.b || !f.rel || (pa && pb && !f.rel)) {
          if (!f.rel && f.b && !(pa && pb)) {
            // родители без отношения: второй — рядом
            const [x, o] = pa ? [f.a, f.b] : [f.b, f.a]
            const r = canStand(o, row(x)) ? row(x) : row(o)
            rows.set(o, r)
            placeFree([personItem(o)], r, pos.get(x)! + PERSON_W / 2 + NOREL_GAP)
          }
          drawn.add(f.id)
          changed = true
          continue
        }
        const [x, o] = pa ? [f.a, f.b] : [f.b, f.a]
        // супруг в этом ряду встал бы рядом со своим ребёнком или родителем — здесь отсылка, сам он — в своём ряду
        const here = !pos.has(o) && canStand(o, row(x))
        if (here) rows.set(o, row(x))
        placeFree([unionItem(f), here ? { ...personItem(o), fam: f } : refItem(o, f)], row(x), pos.get(x)! + PERSON_W / 2 + GAP)
        edges.push({ id: `s:${x}>${f.id}`, type: 'straight', source: x, sourceHandle: 'r', target: f.id, targetHandle: 'l', rels: f.rel ? [f.rel] : undefined })
        changed = true
        continue
      }
      const kids = f.kids.filter(k => pos.has(k.id))
      if (!kids.length) continue
      // ряды уже согласованы (родители выше детей); супруг из другого ряда — отсылкой
      const r = row(f.a)
      const items = [personItem(f.a), unionItem(f), ...(f.b ? [row(f.b) === r ? personItem(f.b) : refItem(f.b, f)] : [])]
      const cx = kids.reduce((s, k) => s + pos.get(k.id)!, 0) / kids.length
      placeFree(items, r, cx - rowWidth(items) / 2)
      changed = true
    }
    for (const f of ordered) {
      if (!drawn.has(f.id)) continue
      const fx = pos.get(f.id) ?? pos.get(f.a) ?? pos.get(f.b) ?? 0
      for (const k of f.kids) {
        if (pos.has(k.id)) continue
        placeFree([personItem(k.id)], row(k.id), fx - PERSON_W / 2)
        changed = true
      }
    }
    if (!changed) {
      const rest = [...g.persons.keys()].filter(id => !pos.has(id)).sort()
      if (!rest.length) break
      placeFree([personItem(rest[0]!)], row(rest[0]!), right() + GAP * 4)
    }
  }

  // отношения линии к ребёнку: от пары — все, от одного родителя — его
  const relsOf = (k: Kid, parent?: string) => {
    const all = k.rels ?? []
    const mine = parent ? all.filter(r => r.parent === parent) : all
    return [...new Set((mine.length ? mine : all).map(r => r.id))]
  }
  // линии к детям
  for (const f of ordered) {
    const kids = f.kids.filter(k => pos.has(k.id))
    if (!kids.length) continue
    const levels = new Map<number, number>()
    const busAt = (k: string) => levels.get(row(k)) ?? levels.set(row(k), nextBus(row(k))).get(row(k))!
    const fromRow = Math.min(...[f.a, f.b].filter(Boolean).map(row))
    for (const k of kids) {
      const style = { dashed: k.dashed || undefined, label: k.label }
      const node = famNode.get(f.id)
      if (node) {
        bus(`b:${f.id}>${k.id}`, node, k.id, busAt(k.id), fromRow, row(k.id), { ...style, group: f.id, rels: relsOf(k) })
        continue
      }
      for (const p of [f.a, f.b]) {
        if (!p) continue
        const src = pos.has(p) ? p : refOf.get(`${f.id}:${p}`)
        if (src) bus(`b:${f.id}:${p}>${k.id}`, src, k.id, busAt(k.id), row(p), row(k.id), { dashed: k.dashed || undefined, label: f.b ? undefined : k.label, group: f.id, rels: relsOf(k, p) })
      }
    }
  }
  // отдельные линии «родитель — ребёнок» (отчим, опекун)
  for (const l of g.links) {
    if (!pos.has(l.parent) || !pos.has(l.child)) continue
    bus(`l:${l.id}`, l.parent, l.child, nextBus(row(l.child)), row(l.parent), row(l.child), { dashed: l.dashed || undefined, label: l.label, group: l.id, rels: [l.id] })
  }
  // «Брат / сестра» не у корня: узел над участниками, если у них нет общих родителей
  for (const s of g.siblings) {
    if (sibDrawn.has(s.id)) continue
    const ms = s.members.filter(m => pos.has(m))
    if (ms.length < 2) continue
    const fams = new Set(ms.map(m => t.famOf.get(m)?.id ?? m))
    if (fams.size === 1) continue
    const mr = Math.min(...ms.map(row))
    const cx = ms.reduce((sum, m) => sum + pos.get(m)!, 0) / ms.length
    nodes.push({ id: s.id, type: 'siblings', x: cx - sibBox(s.label).w / 2, y: rowY(mr) - SIB_RISE, ...sibBox(s.label), row: mr - 1, relation: s.id, label: s.label })
    for (const m of ms) bus(`b:${s.id}>${m}`, s.id, m, rowY(row(m)) - 24, mr - 1, row(m), { rels: [s.id] })
  }

  // линии через промежуточные ряды — по свободному каналу между узлами (F-110, F-112)
  const usedX: number[] = []
  const free = (x: number, rs: number[]) =>
    rs.every(r => !(spans.get(r) ?? []).some(([a, b]) => x > a - 8 && x < b + 8)) && usedX.every(u => Math.abs(u - x) > 6)
  for (const b of buses) {
    if (b.toRow - b.fromRow < 2) continue
    const mids: number[] = []
    for (let r = b.fromRow + 1; r < b.toRow; r++) mids.push(r)
    const tx = pos.get(b.edge.target) ?? 0
    let best: number | null = null
    for (let d = 0; d < 4000 && best === null; d += 4) {
      if (free(tx + d, mids)) best = tx + d
      else if (free(tx - d, mids)) best = tx - d
    }
    if (best === null) continue
    usedX.push(best)
    b.edge.via = { x: best, y: rowY(b.fromRow + 1) - 62 }
  }
  return { nodes, edges }
}
