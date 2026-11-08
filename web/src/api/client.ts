// Клиент API: fetch, CSRF-токен сессии в заголовке (Д-13), JSON, ошибки.
// 401 на защищённом методе — сессии нет: зовётся обработчик (переход ко входу).

import type { BatchIn, CustomSectionDef, ErasDTO, ErasSettings, FieldDef, SchemaTemplateDTO } from '@/core/types'

import type {
  AddParticipantIn, AddValueIn, AuditOut, AuditQuery, DashboardDTO, ErrorBody, FileDTO, GroupDTO, GroupIn, ListOut, ListQuery, MeDTO,
  OperationOut, PatchParticipantIn, PatchValueIn, PrefsDTO, RecordOut, SchemaDTO, SetupStatusDTO, SystemPatchIn, TreeOut,
  AccountDTO, AdminAccountDTO, InvitationDTO, InviteDTO, MemberDTO, RegisterStatusDTO,
} from './types'

/** замена эры во всех датах дашборда (F-143) */
export interface EraReplace { from: string; to: string }

export const CSRF_HEADER = 'X-Kintion-CSRF'

/** Ошибка ответа API: HTTP-код, код ошибки сервера, поле, текущее состояние (409). */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly field?: string
  readonly current?: unknown

  constructor(status: number, body: ErrorBody) {
    super(body.message ?? body.error)
    this.status = status
    this.code = body.error
    this.field = body.field
    this.current = body.current
  }

  /** поле изменил кто-то другой (F-45) */
  get conflict(): boolean {
    return this.status === 409 && this.code === 'conflict'
  }

  /** значение поля не прошло проверку (F-15) */
  get validation(): boolean {
    return this.status === 400 && this.code === 'validation'
  }
}

/** Сеть недоступна или ответ не JSON. */
export const NETWORK = 'network'

let csrf = ''
let unauthorized: (() => void) | null = null

export function setCsrf(token: string): void {
  csrf = token
}

/** Обработчик 401 на защищённых методах (кроме проверки сессии). */
export function onUnauthorized(fn: (() => void) | null): void {
  unauthorized = fn
}

interface Options {
  /** не звать обработчик 401 (проверка сессии, вход) */
  quiet401?: boolean
  body?: unknown
  form?: FormData
}

async function request<T>(method: string, path: string, opts: Options = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  let body: BodyInit | undefined
  if (opts.form) body = opts.form
  else if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(opts.body)
  }
  if (method !== 'GET' && csrf) headers[CSRF_HEADER] = csrf
  let res: Response
  try {
    res = await fetch(path, { method, headers, body, credentials: 'same-origin' })
  } catch (e) {
    throw new ApiError(0, { error: NETWORK, message: e instanceof Error ? e.message : String(e) })
  }
  if (res.status === 204) return undefined as T
  let data: unknown
  try {
    data = await res.json()
  } catch {
    data = { error: res.ok ? NETWORK : `http_${res.status}` }
  }
  if (!res.ok) {
    if (res.status === 401 && !opts.quiet401) unauthorized?.()
    throw new ApiError(res.status, (data ?? { error: `http_${res.status}` }) as ErrorBody)
  }
  return data as T
}

const enc = encodeURIComponent

function query(q: object): string {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(q)) {
    if (v === undefined || v === null || v === '' || v === false) continue
    p.set(k, v === true ? '1' : String(v))
  }
  const s = p.toString()
  return s ? `?${s}` : ''
}

/** Обрезка изображения (F-79): x, y, w, h в пикселях исходника; у квадрата и круга w = h. */
export interface Crop {
  x?: number
  y?: number
  w?: number
  h?: number
  shape?: string
}

/** rect — прямоугольник пропорций обрезки (портрет дерева, карточка, F-79) */
export type PreviewShape = 'square' | 'circle' | 'rect'
export const PREVIEW_SIZES = [64, 128, 256, 512] as const

function cropOf(v: unknown): Crop | null {
  return v && typeof v === 'object' ? (v as Crop) : null
}

export const api = {
  setupStatus: () => request<SetupStatusDTO>('GET', '/api/setup', { quiet401: true }),
  setup: (token: string, login: string, password: string) =>
    request<MeDTO>('POST', '/api/setup', { body: { token, login, password }, quiet401: true }),
  login: (login: string, password: string) =>
    request<MeDTO>('POST', '/api/login', { body: { login, password }, quiet401: true }),
  logout: () => request<void>('POST', '/api/logout', { quiet401: true }),
  me: () => request<MeDTO>('GET', '/api/me', { quiet401: true }),
  changePassword: (current: string, next: string) =>
    request<void>('POST', '/api/me/password', { body: { current, new: next } }),
  setNick: (nick: string) => request<AccountDTO>('PUT', '/api/me/nick', { body: { nick } }),
  registerStatus: (token?: string) =>
    request<RegisterStatusDTO>('GET', `/api/register${query({ token })}`, { quiet401: true }),
  register: (body: { token?: string; login: string; password: string; nick: string }) =>
    request<MeDTO>('POST', '/api/register', { body, quiet401: true }),

  /** гости дашборда и их права (владелец, F-03) */
  members: (d: string) => request<MemberDTO[]>('GET', `/api/dashboards/${enc(d)}/members`),
  putGrant: (d: string, body: { nick?: string; account_id?: string; group_id: string; can_edit: boolean }) =>
    request<MemberDTO[]>('PUT', `/api/dashboards/${enc(d)}/grants`, { body }),
  deleteGrant: (d: string, g: string, acc: string) =>
    request<MemberDTO[]>('DELETE', `/api/dashboards/${enc(d)}/grants/${enc(g)}/${enc(acc)}`),
  deleteMember: (d: string, acc: string) => request<MemberDTO[]>('DELETE', `/api/dashboards/${enc(d)}/members/${enc(acc)}`),
  invitations: () => request<InvitationDTO[]>('GET', '/api/invitations'),
  acceptInvitation: (d: string) => request<void>('POST', `/api/invitations/${enc(d)}/accept`),
  declineInvitation: (d: string) => request<void>('POST', `/api/invitations/${enc(d)}/decline`),

  /** админ инстанса: аккаунты и приглашения на инстанс (F-28) */
  adminAccounts: () => request<AdminAccountDTO[]>('GET', '/api/admin/accounts'),
  adminCreateAccount: (body: { login: string; password: string; nick: string }) =>
    request<AdminAccountDTO>('POST', '/api/admin/accounts', { body }),
  /** ссылки-приглашения: в invite — админ, в invite-all — любой (свои) */
  invites: () => request<InviteDTO[]>('GET', '/api/invites'),
  createInvite: () => request<InviteDTO>('POST', '/api/invites'),
  deleteInvite: (id: string) => request<void>('DELETE', `/api/invites/${enc(id)}`),

  prefs: () => request<PrefsDTO>('GET', '/api/me/prefs'),
  setPrefs: (p: PrefsDTO) => request<PrefsDTO>('PUT', '/api/me/prefs', { body: p }),

  dashboards: () => request<DashboardDTO[]>('GET', '/api/dashboards'),
  createDashboard: (title: string) => request<DashboardDTO>('POST', '/api/dashboards', { body: { name: title } }),
  renameDashboard: (d: string, title: string) =>
    request<DashboardDTO>('PATCH', `/api/dashboards/${enc(d)}`, { body: { name: title } }),
  /** Набор полей быстрой формы шаблона в дашборде (F-124); пусто — по умолчанию пака */
  putQuickFields: (d: string, tmpl: string, body: { if_rev: number; quick_fields: string[] }) =>
    request<SchemaTemplateDTO>('PUT', `/api/dashboards/${enc(d)}/templates/${enc(tmpl)}/quick_fields`, { body }),
  schema: (d: string) => request<SchemaDTO>('GET', `/api/dashboards/${enc(d)}/schema`),
  /** эры дашборда с действующим списком (F-143) */
  eras: (d: string) => request<ErasDTO>('GET', `/api/dashboards/${enc(d)}/eras`),
  /** эры дашборда (F-143); replace — замена эры во всех датах («к уже созданным», переименование), одна или список по порядку */
  putEras: (d: string, body: { if_rev: number; eras: ErasSettings; replace?: EraReplace | EraReplace[] }) =>
    request<ErasDTO>('PUT', `/api/dashboards/${enc(d)}/eras`, { body }),
  records: (d: string, q: ListQuery) => request<ListOut>('GET', `/api/dashboards/${enc(d)}/records${query(q)}`),
  /** ракурс «Дерево» (F-110): отношения с tree_link и их участники; group — с подгруппами */
  tree: (d: string, group?: string) => request<TreeOut>('GET', `/api/dashboards/${enc(d)}/tree${query({ group })}`),
  groups: (d: string) => request<GroupDTO[]>('GET', `/api/dashboards/${enc(d)}/groups`),
  /** последние группы, назначенные этим пользователем (F-81) */
  recentGroups: (d: string) => request<string[]>('GET', `/api/dashboards/${enc(d)}/groups/recent`),
  operations: (d: string, batch: BatchIn) =>
    request<OperationOut>('POST', `/api/dashboards/${enc(d)}/operations`, { body: batch }),

  audit: (d: string, q: AuditQuery = {}) => request<AuditOut>('GET', `/api/dashboards/${enc(d)}/audit${query(q)}`),

  record: (id: string) => request<RecordOut>('GET', `/api/records/${enc(id)}`),
  /** в корзину (F-44) */
  deleteRecord: (id: string) => request<RecordOut>('DELETE', `/api/records/${enc(id)}`),
  restoreRecord: (id: string) => request<RecordOut>('POST', `/api/records/${enc(id)}/restore`),
  purgeRecord: (id: string) => request<void>('DELETE', `/api/records/${enc(id)}/purge`),
  /** общие свои поля шаблона в дашборде целиком (F-96) */
  putSharedFields: (d: string, tmpl: string, body: { if_rev: number; custom_fields: FieldDef[]; custom_sections?: CustomSectionDef[] }) =>
    request<SchemaTemplateDTO>('PUT', `/api/dashboards/${enc(d)}/templates/${enc(tmpl)}/custom_fields`, { body }),
  patchRecord: (id: string, patch: SystemPatchIn) =>
    request<RecordOut>('PATCH', `/api/records/${enc(id)}`, { body: patch }),
  addValue: (id: string, key: string, v: AddValueIn) =>
    request<RecordOut>('POST', `/api/records/${enc(id)}/fields/${enc(key)}/values`, { body: v }),
  patchValue: (id: string, key: string, vid: string, v: PatchValueIn) =>
    request<RecordOut>('PATCH', `/api/records/${enc(id)}/fields/${enc(key)}/values/${enc(vid)}`, { body: v }),
  deleteValue: (id: string, key: string, vid: string, ifRev: number) =>
    request<RecordOut>('DELETE', `/api/records/${enc(id)}/fields/${enc(key)}/values/${enc(vid)}${query({ if_rev: ifRev })}`),
  addParticipant: (id: string, p: AddParticipantIn) =>
    request<RecordOut>('POST', `/api/records/${enc(id)}/participants`, { body: p }),
  patchParticipant: (id: string, pid: string, p: PatchParticipantIn) =>
    request<RecordOut>('PATCH', `/api/records/${enc(id)}/participants/${enc(pid)}`, { body: p }),
  deleteParticipant: (id: string, pid: string, ifRev: number) =>
    request<RecordOut>('DELETE', `/api/records/${enc(id)}/participants/${enc(pid)}${query({ if_rev: ifRev })}`),

  createGroup: (d: string, g: GroupIn) => request<GroupDTO>('POST', `/api/dashboards/${enc(d)}/groups`, { body: g }),
  /** массово поставить / снять метку группы (F-131); changed — сколько меток изменилось */
  groupMembers: (d: string, g: string, body: { add?: string[]; remove?: string[] }) =>
    request<{ changed: number }>('POST', `/api/dashboards/${enc(d)}/groups/${enc(g)}/records`, { body }),
  patchGroup: (d: string, g: string, patch: GroupIn) =>
    request<GroupDTO>('PATCH', `/api/dashboards/${enc(d)}/groups/${enc(g)}`, { body: patch }),
  deleteGroup: (d: string, g: string) => request<void>('DELETE', `/api/dashboards/${enc(d)}/groups/${enc(g)}`),
  setGroups: (id: string, ifRev: number, groups: string[]) =>
    request<RecordOut>('PUT', `/api/records/${enc(id)}/groups`, { body: { if_rev: ifRev, groups } }),

  /** загрузка: изображение; any — любой файл (поле «Файл», KIN-27) */
  uploadFile(d: string, file: Blob, any = false): Promise<FileDTO> {
    const form = new FormData()
    form.append('file', file)
    return request<FileDTO>('POST', `/api/dashboards/${enc(d)}/files${any ? '?any=1' : ''}`, { form })
  },
  fileUrl: (d: string, sha: string) => `/api/files/${enc(d)}/${enc(sha)}`,

  /** Адрес превью: обрезка всегда в запросе (план §5), без неё — центральный квадрат (rect — портрет). */
  previewUrl(d: string, sha: string, crop: unknown, shape: PreviewShape = 'square', size: (typeof PREVIEW_SIZES)[number] = 64): string {
    const c = cropOf(crop)
    const q: Record<string, number | string | undefined> = { shape, size }
    if (c && c.w != null) Object.assign(q, { x: c.x ?? 0, y: c.y ?? 0, w: c.w, h: c.h ?? c.w })
    return `/api/files/${enc(d)}/${enc(sha)}/preview${query(q)}`
  },
}

