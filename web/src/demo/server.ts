// Сервер демо-сборки (VITE_DEMO=1): API Kintion в памяти страницы поверх
// снимка демо-данных (fixtures.json — то, что заполняет
// deploy/scripts/seed-demo.sh). Чтение — как на сервере (фильтры списка,
// журнал по записи); запись удаётся и живёт до перезагрузки страницы.
// Проверок версий (F-45) и прав нет, правки не пишутся в журнал (он —
// из снимка): это витрина экранов, не сервер.
import type {
  AccountDTO, AddParticipantIn, CardMode, AddValueIn, AuditOperationDTO, DashboardDTO, ErrorBody, FileDTO, GroupDTO, GroupIn,
  HistoryEdit, MeDTO, PatchParticipantIn, PatchValueIn, PositionIn, PrefsDTO, RelatedDTO, SystemPatchIn,
} from '@/api/types'
import type { BatchIn, LinkDTO, PeriodDTO, PeriodIn, RecordDTO, SchemaDTO, ValueDTO, ValueIn } from '@/core/types'

import { addImage } from './files'
import { buildSchema, type SharedFields } from './schema'

/** Снимок демо-данных (web/scripts/demo-capture.mjs). */
export interface Fixtures {
  account: AccountDTO
  dashboard: DashboardDTO
  prefs: PrefsDTO
  groups: GroupDTO[]
  recentGroups: string[]
  templateIds: Record<string, string>
  /** общие свои поля шаблонов дашборда (F-96) */
  sharedFields?: SharedFields
  records: RecordDTO[]
  audit: AuditOperationDTO[]
  files: { sha256: string; mime: string; data: string }[]
}

/** Состояние входа: вошёл, не вошёл, нужна первичная настройка. */
export type DemoAuth = 'in' | 'guest' | 'setup'

export interface DemoOptions {
  auth?: DemoAuth
  prefs?: HistoryEdit
  /** режим карточек (F-108) */
  cardMode?: CardMode
}

export interface DemoResponse {
  status: number
  body?: unknown
}

/** пароль демо-владельца — как в deploy/scripts/seed-demo.sh */
export const DEMO_PASSWORD = 'demo-password-0.1'
/** второй, пустой дашборд — для экрана пустого списка */
export const EMPTY_DASHBOARD = '00000000-0000-4000-8000-00000000e001'
const CSRF = 'demo-csrf'

class HttpError extends Error {
  constructor(readonly status: number, readonly body: ErrorBody) {
    super(body.message ?? body.error)
  }
}

const notFound = () => new HttpError(404, { error: 'not_found', message: 'не найдено' })

function clone<T>(v: T): T {
  return structuredClone(v)
}

/** Поставить элемент на место: над before_id, под after_id, наверх или вниз. */
function place<T extends { id: string }>(list: T[], item: T, p: PositionIn, fallback: 'top' | 'bottom' = 'bottom'): void {
  const at = (id: string) => list.findIndex(x => x.id === id)
  if (p.before_id && at(p.before_id) >= 0) list.splice(at(p.before_id), 0, item)
  else if (p.after_id && at(p.after_id) >= 0) list.splice(at(p.after_id) + 1, 0, item)
  else if ((p.position ?? fallback) === 'top') list.unshift(item)
  else list.push(item)
}

function moving(p: PositionIn): boolean {
  return !!(p.before_id || p.after_id || p.position)
}

export class DemoServer {
  auth: DemoAuth
  prefs: PrefsDTO
  account: AccountDTO
  password = DEMO_PASSWORD
  dashboards: DashboardDTO[]
  /** группы дашборда; вложенность (F-105) — списком «Входит в» */
  groups = new Map<string, { id: string; name: string; parents: string[] }[]>()
  recent = new Map<string, string[]>()
  records = new Map<string, RecordDTO>()
  /** дашборд записи */
  owner = new Map<string, string>()
  audit: AuditOperationDTO[]
  /** дашборд, к которому относится журнал снимка */
  private auditDashboard: string
  templateIds: Record<string, string>
  sharedFields: SharedFields
  /** ошибка для следующего изменяющего запроса (состояния полей F-86) */
  fault: DemoResponse | null = null
  private seq = 0
  /** схема по дашборду: общие поля (F-96) — только у дашборда снимка */
  private schemaCache = new Map<string, SchemaDTO>()

  constructor(fx: Fixtures, opts: DemoOptions = {}) {
    this.auth = opts.auth ?? 'in'
    this.prefs = {
      ...fx.prefs,
      ...(opts.prefs !== undefined ? { history_edit: opts.prefs } : {}),
      ...(opts.cardMode ? { card_mode: opts.cardMode } : {}),
    }
    this.account = clone(fx.account)
    const d = fx.dashboard.id
    this.dashboards = [clone(fx.dashboard), { id: EMPTY_DASHBOARD, name: 'Новый дашборд', created_at: fx.dashboard.created_at }]
    this.groups.set(d, fx.groups.map(g => ({ id: g.id, name: g.name, parents: [...g.parents] })))
    this.groups.set(EMPTY_DASHBOARD, [])
    this.recent.set(d, [...fx.recentGroups])
    for (const r of fx.records) {
      this.records.set(r.id, clone(r))
      this.owner.set(r.id, d)
    }
    this.audit = clone(fx.audit)
    this.auditDashboard = d
    this.templateIds = fx.templateIds
    this.sharedFields = fx.sharedFields ?? {}
  }

  /** Новый id в форме UUID (по порядку: снимки демо повторяемы). */
  uid(): string {
    return `00000000-0000-4000-8000-${(++this.seq).toString(16).padStart(12, '0')}`
  }

  schema(d: string): SchemaDTO {
    let out = this.schemaCache.get(d)
    if (!out) {
      out = buildSchema(this.templateIds, d === this.dashboards[0]?.id ? this.sharedFields : {})
      this.schemaCache.set(d, out)
    }
    return out
  }

  me(): MeDTO {
    return { account: clone(this.account), csrf: CSRF }
  }

  /** Обработать запрос API: метод, путь с запросом, тело (JSON или FormData). */
  async handle(method: string, url: URL, body: unknown): Promise<DemoResponse> {
    try {
      if (method !== 'GET' && this.fault && url.pathname.startsWith('/api/') && !url.pathname.startsWith('/api/me')) {
        const f = this.fault
        this.fault = null
        return f
      }
      return await this.route(method, url, body)
    } catch (e) {
      if (e instanceof HttpError) return { status: e.status, body: e.body }
      return { status: 500, body: { error: 'internal', message: e instanceof Error ? e.message : String(e) } }
    }
  }

  private guard(): void {
    if (this.auth !== 'in') throw new HttpError(401, { error: 'unauthorized', message: 'нужен вход' })
  }

  private async route(method: string, url: URL, body: unknown): Promise<DemoResponse> {
    const p = url.pathname.split('/').filter(Boolean).map(decodeURIComponent)
    const q = url.searchParams
    const rel = p.slice(1)
    const key = `${method} /${rel.map((s, i) => (isParam(rel, i) ? '*' : s)).join('/')}`
    const b = (body ?? {}) as Record<string, unknown>
    if (p[0] === 'healthz') return { status: 200, body: { status: 'ok', db: 'ok' } }
    switch (key) {
      case 'GET /setup':
        return { status: 200, body: { needed: this.auth === 'setup' } }
      case 'POST /setup':
        return this.setup(String(b.token ?? ''), String(b.login ?? ''), String(b.password ?? ''))
      case 'POST /login':
        if (this.auth === 'setup' || b.login !== this.account.login || b.password !== this.password) {
          throw new HttpError(401, { error: 'bad_credentials', message: 'неверный логин или пароль' })
        }
        this.auth = 'in'
        return { status: 200, body: this.me() }
      case 'POST /logout':
        this.auth = this.auth === 'setup' ? 'setup' : 'guest'
        return { status: 204 }
      case 'GET /me':
        this.guard()
        return { status: 200, body: this.me() }
    }
    this.guard()
    switch (key) {
      case 'POST /me/password':
        return this.changePassword(String(b.current ?? ''), String(b.new ?? ''))
      case 'GET /me/prefs':
        return { status: 200, body: clone(this.prefs) }
      case 'PUT /me/prefs': {
        const v = b.history_edit
        if (v !== '' && v !== 'replace' && v !== 'add') {
          throw new HttpError(400, { error: 'validation', field: 'history_edit', message: 'только replace, add или пусто' })
        }
        const mode = b.card_mode ?? 'read'
        if (mode !== 'read' && mode !== 'inline' && mode !== 'form') {
          throw new HttpError(400, { error: 'validation', field: 'card_mode', message: 'только read, inline или form' })
        }
        this.prefs = { history_edit: v, card_mode: mode, tree_ribbon: b.tree_ribbon !== false, tree_hide_dead: b.tree_hide_dead === true }
        return { status: 200, body: clone(this.prefs) }
      }
      case 'GET /dashboards':
        return { status: 200, body: clone(this.dashboards) }
      case 'POST /dashboards': {
        const d = { id: this.uid(), name: String(b.name ?? ''), created_at: new Date().toISOString() }
        this.dashboards.push(d)
        this.groups.set(d.id, [])
        return { status: 201, body: clone(d) }
      }
      case 'PATCH /dashboards/*': {
        const d = this.dashboard(p[2])
        d.name = String(b.name ?? d.name)
        return { status: 200, body: clone(d) }
      }
      case 'GET /dashboards/*/schema':
        return { status: 200, body: this.schema(this.dashboard(p[2]).id) }
      case 'GET /dashboards/*/records':
        return { status: 200, body: this.list(this.dashboard(p[2]).id, q) }
      case 'GET /dashboards/*/tree':
        return { status: 200, body: this.tree(this.dashboard(p[2]).id, q.get('group')) }
      case 'GET /dashboards/*/groups':
        return { status: 200, body: this.groupList(this.dashboard(p[2]).id) }
      case 'GET /dashboards/*/groups/recent':
        return { status: 200, body: [...(this.recent.get(this.dashboard(p[2]).id) ?? [])] }
      case 'POST /dashboards/*/groups':
        return { status: 201, body: this.createGroup(this.dashboard(p[2]).id, b as GroupIn) }
      case 'PATCH /dashboards/*/groups/*':
        return { status: 200, body: this.patchGroup(this.dashboard(p[2]).id, p[4] ?? '', b as GroupIn) }
      case 'DELETE /dashboards/*/groups/*':
        this.deleteGroup(this.dashboard(p[2]).id, p[4] ?? '')
        return { status: 204 }
      case 'POST /dashboards/*/operations':
        return { status: 200, body: this.operations(this.dashboard(p[2]).id, b as unknown as BatchIn) }
      case 'GET /dashboards/*/audit':
        return { status: 200, body: this.auditPage(this.dashboard(p[2]).id, q.get('entity')) }
      case 'POST /dashboards/*/files':
        return { status: 201, body: await this.upload(body) }
      case 'GET /records/*':
        return this.out(this.record(p[2]))
      case 'DELETE /records/*':
        return this.out(this.touch(this.record(p[2]), r => (r.deleted = true)))
      case 'POST /records/*/restore':
        return this.out(this.touch(this.record(p[2]), r => (r.deleted = false)))
      case 'DELETE /records/*/purge':
        this.purge(this.record(p[2]))
        return { status: 204 }
      case 'PATCH /records/*':
        return this.out(this.patchRecord(this.record(p[2]), b as unknown as SystemPatchIn))
      case 'PUT /records/*/groups':
        return this.out(this.setGroups(this.record(p[2]), (b.groups as string[] | undefined) ?? []))
      case 'POST /records/*/fields/*/values':
        return this.out(this.addValue(this.record(p[2]), p[4] ?? '', b as unknown as AddValueIn))
      case 'PATCH /records/*/fields/*/values/*':
        return this.out(this.patchValue(this.record(p[2]), p[4] ?? '', p[6] ?? '', b as unknown as PatchValueIn))
      case 'DELETE /records/*/fields/*/values/*':
        return this.out(this.deleteValue(this.record(p[2]), p[4] ?? '', p[6] ?? ''))
      case 'POST /records/*/participants':
        return this.out(this.addParticipant(this.record(p[2]), b as unknown as AddParticipantIn))
      case 'PATCH /records/*/participants/*':
        return this.out(this.patchParticipant(this.record(p[2]), p[4] ?? '', b as unknown as PatchParticipantIn))
      case 'DELETE /records/*/participants/*':
        return this.out(this.deleteParticipant(this.record(p[2]), p[4] ?? ''))
    }
    throw notFound()
  }

  // ---------- вход ----------

  private setup(token: string, login: string, password: string): DemoResponse {
    if (this.auth !== 'setup') throw new HttpError(409, { error: 'setup_done', message: 'настройка уже выполнена' })
    if (!token.trim()) throw new HttpError(403, { error: 'bad_token', message: 'неверный токен первого запуска' })
    if (!login.trim()) throw new HttpError(400, { error: 'validation', field: 'login', message: 'нужен логин' })
    this.checkPassword(password)
    this.account = { ...this.account, login: login.trim().toLowerCase() }
    this.password = password
    this.auth = 'in'
    return { status: 201, body: this.me() }
  }

  private checkPassword(password: string): void {
    if ([...password].length < 8) {
      throw new HttpError(400, { error: 'validation', field: 'password', message: 'пароль: не короче 8 символов' })
    }
  }

  private changePassword(current: string, next: string): DemoResponse {
    this.checkPassword(next)
    if (current !== this.password) throw new HttpError(400, { error: 'validation', field: 'current', message: 'текущий пароль неверен' })
    this.password = next
    return { status: 204 }
  }

  // ---------- дашборды, записи ----------

  private dashboard(id: string | undefined): DashboardDTO {
    const d = this.dashboards.find(x => x.id === id)
    if (!d) throw notFound()
    return d
  }

  private record(id: string | undefined): RecordDTO {
    const r = id ? this.records.get(id) : undefined
    if (!r) throw notFound()
    return r
  }

  /** Связанные записи для подписей: в демо — все живые записи дашборда. */
  private related(d: string, skip: Set<string> = new Set()): RelatedDTO {
    const out: RelatedDTO = {}
    for (const r of this.records.values()) {
      if (!r.deleted && !skip.has(r.id) && this.owner.get(r.id) === d) out[r.id] = clone(r)
    }
    return out
  }

  private out(r: RecordDTO): DemoResponse {
    return { status: 200, body: { record: clone(r), related: this.related(this.owner.get(r.id) ?? '', new Set([r.id])) } }
  }

  private touch(r: RecordDTO, fn: (r: RecordDTO) => void): RecordDTO {
    fn(r)
    return r
  }

  /** Тексты записи для поиска: примечание, ручная подпись, строковые значения. */
  private texts(r: RecordDTO): string[] {
    const out = [r.note ?? '', r.title_manual ?? '']
    for (const f of Object.values(r.fields)) for (const v of f?.values ?? []) if (typeof v.value === 'string') out.push(v.value)
    return out
  }

  private list(d: string, q: URLSearchParams) {
    const deleted = q.get('deleted') === '1'
    const entity = q.get('entity')
    const group = q.get('group')
    const text = (q.get('q') ?? '').trim().toLowerCase()
    const groups = group ? this.subtree(d, group) : null
    const limit = Number(q.get('limit') ?? 50) || 50
    const offset = Number(q.get('offset') ?? 0) || 0
    const matched = [...this.records.values()]
      .filter(r => this.owner.get(r.id) === d && r.deleted === deleted)
      .filter(r => !entity || r.entity === entity)
      .filter(r => !groups || r.groups.some(g => groups.has(g)))
      .filter(r => !text || this.texts(r).some(s => s.toLowerCase().includes(text)))
      .sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
    const items = matched.slice(offset, offset + limit)
    return { items: clone(items), total: matched.length, related: this.related(d, new Set(items.map(r => r.id))) }
  }

  /** Ракурс «Дерево», как на сервере: отношения с tree_link, участники — видимые и из группы. */
  private tree(d: string, group: string | null) {
    if (group && !(this.groups.get(d) ?? []).some(g => g.id === group)) {
      throw new HttpError(400, { error: 'validation', field: 'group', message: 'нет такой группы' })
    }
    const groups = group ? this.subtree(d, group) : null
    const defs = new Map(this.schema(d).templates.map(t => [t.id, t.definition]))
    const live = (id: string) => {
      const r = this.records.get(id)
      return r && !r.deleted && this.owner.get(id) === d ? r : undefined
    }
    const keep = (id: string) => !!live(id) && (!groups || live(id)!.groups.some(g => groups.has(g)))
    const used = new Set<string>()
    const relations: RecordDTO[] = []
    for (const r of [...this.records.values()].sort((a, b) => (a.id < b.id ? -1 : 1))) {
      if (!live(r.id) || !defs.get(r.template_id)?.tree_link) continue
      const parts = r.participants.filter(x => keep(x.entity_id))
      const distinct = new Set(parts.map(x => x.entity_id))
      if (distinct.size < 2) continue
      relations.push({ ...clone(r), participants: clone(parts) })
      for (const id of distinct) used.add(id)
    }
    const records = [...used].sort().map(id => clone(this.records.get(id)!))
    return { relations, records }
  }

  private blank(d: string, template: string): RecordDTO {
    const tpl = this.schema(d).templates.find(t => t.key === template)
    if (!tpl) throw new HttpError(400, { error: 'validation', field: 'template', message: `нет шаблона ${template}` })
    const r: RecordDTO = {
      id: this.uid(), entity: tpl.entity, template: tpl.key, template_id: tpl.id, start: null, end: null, note: null,
      title_manual: null, sensitive: false, groups: [], custom_fields: [], fields: {},
      revs: { custom_fields: 0, end: 0, groups: 0, note: 0, sensitive: 0, start: 0, title: 0 },
      participants: [], participates_in: [], deleted: false,
    }
    this.records.set(r.id, r)
    this.owner.set(r.id, d)
    return r
  }

  private value(v: ValueIn, ref: (s: string) => string): ValueDTO {
    return {
      id: this.uid(),
      value: typeof v.value === 'string' ? ref(v.value) : (v.value ?? null),
      valid_from: v.valid_from ?? null,
      valid_to: v.valid_to ?? null,
      note: v.note ?? null,
      meta: v.meta ?? {},
      links: (v.links ?? []).map((l): LinkDTO => ({ target: ref(l.target), anchor: l.anchor })),
    }
  }

  private period(p: PeriodIn | null | undefined, ref: (s: string) => string = s => s): PeriodDTO | null {
    return p ? { value: p.value ?? null, link: p.link ? ref(p.link) : null } : null
  }

  /** Пакетная операция: записи со значениями и группами, участия (временные id — ref). */
  private operations(d: string, batch: BatchIn) {
    const ids: Record<string, string> = {}
    const created: RecordDTO[] = []
    for (const n of batch.records) {
      const template = n.template ?? n.entity ?? ''
      const r = this.blank(d, template)
      ids[n.ref] = r.id
      created.push(r)
    }
    const ref = (s: string) => ids[s] ?? s
    batch.records.forEach((n, i) => {
      const r = created[i]
      if (!r) return
      r.start = this.period(n.start, ref)
      r.end = this.period(n.end, ref)
      r.note = n.note ?? null
      r.groups = [...(n.groups ?? [])]
      for (const [k, vals] of Object.entries(n.fields ?? {})) r.fields[k] = { rev: 0, values: vals.map(v => this.value(v, ref)) }
      this.noteRecent(d, r.groups)
    })
    for (const p of batch.participants) {
      this.participate(this.record(ref(p.container)), ref(p.entity), p.role, p, { position: 'bottom' })
    }
    return { operation_id: this.uid(), ids, records: clone(created), related: this.related(d), errors: [] }
  }

  private patchRecord(r: RecordDTO, patch: SystemPatchIn): RecordDTO {
    const revs = (r.revs ??= {})
    const bump = (k: string) => (revs[k] = (revs[k] ?? 0) + 1)
    const set = <K extends keyof RecordDTO>(k: K, v: RecordDTO[K], rev: string) => {
      r[k] = v
      bump(rev)
    }
    if ('start' in patch) set('start', this.period(patch.start), 'start')
    if ('end' in patch) set('end', this.period(patch.end), 'end')
    if ('note' in patch) set('note', patch.note || null, 'note')
    if ('title_manual' in patch) set('title_manual', patch.title_manual || null, 'title')
    if (patch.sensitive !== undefined) set('sensitive', patch.sensitive, 'sensitive')
    if (patch.custom_fields) set('custom_fields', clone(patch.custom_fields), 'custom_fields')
    return r
  }

  private field(r: RecordDTO, key: string) {
    let f = r.fields[key]
    if (!f) {
      f = { rev: 0, values: [] }
      r.fields[key] = f
    }
    return f
  }

  private addValue(r: RecordDTO, key: string, v: AddValueIn): RecordDTO {
    const f = this.field(r, key)
    place(f.values, this.value(v as ValueIn, s => s), v)
    f.rev++
    return r
  }

  private patchValue(r: RecordDTO, key: string, id: string, v: PatchValueIn): RecordDTO {
    const f = this.field(r, key)
    const cur = f.values.find(x => x.id === id)
    if (!cur) throw notFound()
    if ('value' in v) cur.value = v.value ?? null
    if ('valid_from' in v) cur.valid_from = v.valid_from ?? null
    if ('valid_to' in v) cur.valid_to = v.valid_to ?? null
    if ('note' in v) cur.note = v.note || null
    if ('meta' in v) cur.meta = v.meta ?? {}
    if ('links' in v) cur.links = (v.links ?? []).map(l => ({ target: l.target, anchor: l.anchor }))
    if (moving(v)) {
      f.values = f.values.filter(x => x.id !== id)
      place(f.values, cur, v)
    }
    f.rev++
    return r
  }

  private deleteValue(r: RecordDTO, key: string, id: string): RecordDTO {
    const f = this.field(r, key)
    f.values = f.values.filter(x => x.id !== id)
    f.rev++
    if (!f.values.length) delete r.fields[key]
    return r
  }

  // ---------- участники ----------

  private participate(c: RecordDTO, entity: string, role: string, p: Omit<PatchParticipantIn, 'if_rev'>, pos: PositionIn): string {
    const e = this.record(entity)
    const id = this.uid()
    const period = { valid_from: p.valid_from ?? null, valid_to: p.valid_to ?? null, note: p.note ?? null }
    place(c.participants, { id, entity_id: e.id, role, ...period }, pos)
    e.participates_in.push({ id, container_id: c.id, role, ...period })
    return id
  }

  private addParticipant(c: RecordDTO, p: AddParticipantIn): RecordDTO {
    this.participate(c, p.entity, p.role, p, p)
    return c
  }

  private patchParticipant(c: RecordDTO, id: string, p: PatchParticipantIn): RecordDTO {
    const cur = c.participants.find(x => x.id === id)
    if (!cur) throw notFound()
    const side = this.records.get(cur.entity_id)?.participates_in.find(x => x.id === id)
    for (const k of ['valid_from', 'valid_to', 'note'] as const) {
      if (!(k in p)) continue
      const v = p[k] ?? null
      Object.assign(cur, { [k]: v })
      if (side) Object.assign(side, { [k]: v })
    }
    if (moving(p)) {
      c.participants = c.participants.filter(x => x.id !== id)
      place(c.participants, cur, p)
    }
    return c
  }

  private deleteParticipant(c: RecordDTO, id: string): RecordDTO {
    const cur = c.participants.find(x => x.id === id)
    if (!cur) throw notFound()
    c.participants = c.participants.filter(x => x.id !== id)
    const e = this.records.get(cur.entity_id)
    if (e) e.participates_in = e.participates_in.filter(x => x.id !== id)
    return c
  }

  private purge(r: RecordDTO): void {
    for (const p of r.participants) {
      const e = this.records.get(p.entity_id)
      if (e) e.participates_in = e.participates_in.filter(x => x.id !== p.id)
    }
    for (const p of r.participates_in) {
      const c = this.records.get(p.container_id)
      if (c) c.participants = c.participants.filter(x => x.id !== p.id)
    }
    this.records.delete(r.id)
    this.owner.delete(r.id)
  }

  // ---------- группы ----------

  private groupList(d: string): GroupDTO[] {
    const list = this.groups.get(d) ?? []
    const order = new Map(list.map((g, i) => [g.id, i]))
    return list.map(g => ({
      id: g.id,
      name: g.name,
      count: [...this.records.values()].filter(r => !r.deleted && this.owner.get(r.id) === d && r.groups.includes(g.id)).length,
      parents: g.parents.filter(p => order.has(p)).sort((x, y) => order.get(x)! - order.get(y)!),
      children: list.filter(c => c.parents.includes(g.id)).map(c => c.id),
      auto_surname: false,
      surnames: [],
    }))
  }

  /** Группа и все вложенные в неё по всем путям (F-105). */
  private subtree(d: string, id: string): Set<string> {
    const out = new Set([id])
    const list = this.groups.get(d) ?? []
    for (let grew = true; grew;) {
      grew = false
      for (const g of list) {
        if (!out.has(g.id) && g.parents.some(p => out.has(p))) {
          out.add(g.id)
          grew = true
        }
      }
    }
    return out
  }

  /** «Входит в» и «Включает в себя» группы id; цикл — ошибка проверки. */
  private setLinks(d: string, id: string, g: GroupIn): void {
    const list = this.groups.get(d) ?? []
    const ids = new Set(list.map(x => x.id))
    for (const [field, xs] of [['parents', g.parents], ['children', g.children]] as const) {
      for (const x of xs ?? []) {
        if (x === id || !ids.has(x)) {
          throw new HttpError(400, { error: 'validation', field, message: x === id ? 'группа не может входить в саму себя' : `нет группы ${x}` })
        }
      }
    }
    const before = list.map(x => ({ ...x, parents: [...x.parents] }))
    const cur = list.find(x => x.id === id)!
    if (g.parents !== undefined) cur.parents = [...new Set(g.parents)]
    if (g.children !== undefined) {
      for (const c of list) {
        const want = g.children.includes(c.id)
        if (want && !c.parents.includes(id)) c.parents.push(id)
        if (!want) c.parents = c.parents.filter(p => p !== id)
      }
    }
    const below = this.subtree(d, id)
    if (cur.parents.some(p => below.has(p))) {
      this.groups.set(d, before)
      throw new HttpError(400, { error: 'validation', field: g.parents !== undefined ? 'parents' : 'children', message: 'группа не может оказаться внутри самой себя' })
    }
  }

  private createGroup(d: string, g: GroupIn): GroupDTO {
    const name = (g.name ?? '').trim()
    if (!name) throw new HttpError(400, { error: 'validation', field: 'name', message: 'нужно название' })
    const list = this.groups.get(d) ?? []
    const item = { id: this.uid(), name, parents: [] as string[] }
    const before = list.map(x => ({ ...x, parents: [...x.parents] }))
    place(list, item, g)
    this.groups.set(d, list)
    try {
      this.setLinks(d, item.id, g)
    } catch (e) {
      this.groups.set(d, before) // не создана — и в списке её нет
      throw e
    }
    return this.groupList(d).find(x => x.id === item.id)!
  }

  private patchGroup(d: string, id: string, g: GroupIn): GroupDTO {
    let list = this.groups.get(d) ?? []
    const cur = list.find(x => x.id === id)
    if (!cur) throw notFound()
    this.setLinks(d, id, g)
    list = this.groups.get(d) ?? []
    const item = list.find(x => x.id === id)!
    if (g.name !== undefined) item.name = g.name.trim() || item.name
    if (moving(g)) {
      list = list.filter(x => x.id !== id)
      place(list, item, g)
      this.groups.set(d, list)
    }
    return this.groupList(d).find(x => x.id === id)!
  }

  /** Удаление группы: метки снимаются, её связи разрываются — вложенные к её родителям не переходят (Д-29, F-105). */
  private deleteGroup(d: string, id: string): void {
    const list = this.groups.get(d) ?? []
    const gone = list.find(x => x.id === id)
    if (!gone) throw notFound()
    const rest = list.filter(x => x.id !== id)
    for (const c of rest) {
      if (!c.parents.includes(id)) continue
      c.parents = c.parents.filter(p => p !== id)
    }
    this.groups.set(d, rest)
    for (const r of this.records.values()) r.groups = r.groups.filter(g => g !== id)
    this.recent.set(d, (this.recent.get(d) ?? []).filter(g => g !== id))
  }

  private setGroups(r: RecordDTO, groups: string[]): RecordDTO {
    const added = groups.filter(g => !r.groups.includes(g))
    r.groups = [...groups]
    const revs = (r.revs ??= {})
    revs.groups = (revs.groups ?? 0) + 1
    this.noteRecent(this.owner.get(r.id) ?? '', added)
    return r
  }

  /** Последние назначенные группы (F-81): новые — первыми. */
  private noteRecent(d: string, groups: string[]): void {
    if (!groups.length) return
    const list = (this.recent.get(d) ?? []).filter(g => !groups.includes(g))
    this.recent.set(d, [...groups, ...list])
  }

  // ---------- журнал, файлы ----------

  /** Журнал — снимок seed-demo.sh (он у дашборда снимка); правки демо в журнал не пишутся. */
  private auditPage(d: string, entity: string | null) {
    const touches = (e: AuditOperationDTO['entries'][number]) =>
      !entity || e.entity_id === entity
      || (e.object === 'participation' && (e.before?.entity_id === entity || e.after?.entity_id === entity))
    const operations = (d === this.auditDashboard ? this.audit : [])
      .map(op => ({ ...op, entries: op.entries.filter(touches) }))
      .filter(op => op.entries.length)
    const records: RelatedDTO = {}
    for (const r of this.records.values()) records[r.id] = clone(r)
    return { operations: clone(operations), next: null, records, related: {} }
  }

  /** Загрузка файла: sha — условный (демо), изображение — в памяти страницы. */
  private async upload(body: unknown): Promise<FileDTO> {
    const file = body instanceof FormData ? body.get('file') : null
    if (!(file instanceof Blob)) throw new HttpError(400, { error: 'validation', field: 'file', message: 'нужен файл' })
    const sha = this.uid().replace(/-/g, '').padStart(64, '0')
    const url = await new Promise<string>((resolve, reject) => {
      const rd = new FileReader()
      rd.onload = () => resolve(String(rd.result))
      rd.onerror = () => reject(rd.error ?? new Error('чтение файла'))
      rd.readAsDataURL(file)
    })
    const { width, height } = await addImage(sha, url)
    return { sha256: sha, mime: file.type || 'image/png', size: file.size, width, height, created_at: new Date().toISOString() }
  }
}

/** Сегмент пути — параметр (id), а не имя метода: по месту в известных путях. */
function isParam(p: string[], i: number): boolean {
  const head = p[0]
  if (head === 'dashboards') return i === 1 || (p[2] === 'groups' && i === 3 && p[3] !== 'recent')
  if (head === 'records') return i === 1 || (p[2] === 'fields' && (i === 3 || i === 5)) || (p[2] === 'participants' && i === 3)
  return false
}
