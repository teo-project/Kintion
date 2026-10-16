// Клиент API: fetch, CSRF-токен сессии в заголовке (Д-13), JSON, ошибки.
// 401 на защищённом методе — сессии нет: зовётся обработчик (переход ко входу).

import type { BatchIn, FieldDef, SchemaTemplateDTO } from '@/core/types'

import type {
  AddParticipantIn, AddValueIn, AuditOut, AuditQuery, DashboardDTO, ErrorBody, FileDTO, GroupDTO, GroupIn, ListOut, ListQuery, MeDTO,
  OperationOut, PatchParticipantIn, PatchValueIn, PrefsDTO, RecordOut, SchemaDTO, SetupStatusDTO, SystemPatchIn, TreeOut,
} from './types'

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
  prefs: () => request<PrefsDTO>('GET', '/api/me/prefs'),
  setPrefs: (p: PrefsDTO) => request<PrefsDTO>('PUT', '/api/me/prefs', { body: p }),

  dashboards: () => request<DashboardDTO[]>('GET', '/api/dashboards'),
  createDashboard: (title: string) => request<DashboardDTO>('POST', '/api/dashboards', { body: { name: title } }),
  renameDashboard: (d: string, title: string) =>
    request<DashboardDTO>('PATCH', `/api/dashboards/${enc(d)}`, { body: { name: title } }),
  /** Ð½Ð°Ð±Ð¾Ñ Ð¿Ð¾Ð»ÐµÐ¹ Ð±ÑÑÑÑÐ¾Ð¹ ÑÐ¾ÑÐ¼Ñ ÑÐ°Ð±Ð»Ð¾Ð½Ð° Ð² Ð´Ð°ÑÐ±Ð¾ÑÐ´Ðµ (F-124); Ð¿ÑÑÑÐ¾ â Ð¿Ð¾ ÑÐ¼Ð¾Ð»ÑÐ°Ð½Ð¸Ñ Ð¿Ð°ÐºÐ° */
  putQuickFields: (d: string, tmpl: string, body: { if_rev: number; quick_fields: string[] }) =>
    request<SchemaTemplateDTO>('PUT', `/api/dashboards/${enc(d)}/templates/${enc(tmpl)}/quick_fields`, { body }),
  schema: (d: string) => request<SchemaDTO>('GET', `/api/dashboards/${enc(d)}/schema`),
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
  putSharedFields: (d: string, tmpl: string, body: { if_rev: number; custom_fields: FieldDef[] }) =>
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

  uploadFile(d: string, file: Blob): Promise<FileDTO> {
    const form = new FormData()
    form.append('file', file)
    return request<FileDTO>('POST', `/api/dashboards/${enc(d)}/files`, { form })
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

