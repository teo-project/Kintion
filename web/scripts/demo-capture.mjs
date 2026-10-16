#!/usr/bin/env node
// Снимок демо-данных для демо-сборки (VITE_DEMO=1): читает установку,
// заполненную deploy/scripts/seed-demo.sh, и пишет web/src/demo/fixtures.json.
//   node web/scripts/demo-capture.mjs
// KIN_DEMO_URL (по умолчанию http://localhost:18080), KIN_DEMO_LOGIN, KIN_DEMO_PASSWORD —
// как у seed-demo.sh. Только чтение: установка не меняется (кроме сессии входа).
import { writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const URL_ = process.env.KIN_DEMO_URL ?? 'http://localhost:18080'
const LOGIN = process.env.KIN_DEMO_LOGIN ?? 'demo'
const PASSWORD = process.env.KIN_DEMO_PASSWORD ?? 'demo-password-0.1'
const OUT = fileURLToPath(new URL('../src/demo/fixtures.json', import.meta.url))

let cookie = ''

async function req(method, path, body) {
  const res = await fetch(URL_ + path, {
    method,
    headers: { Accept: 'application/json', ...(body ? { 'Content-Type': 'application/json' } : {}), ...(cookie ? { Cookie: cookie } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  })
  const set = res.headers.get('set-cookie')
  if (set) cookie = set.split(';')[0]
  if (!res.ok) throw new Error(`${method} ${path}: ${res.status} ${await res.text()}`)
  return res
}

const json = async (path) => (await req('GET', path)).json()

const me = await (await req('POST', '/api/login', { login: LOGIN, password: PASSWORD })).json()
const dashboards = await json('/api/dashboards')
const d = dashboards[0]
if (!d) throw new Error('нет дашборда: сначала deploy/scripts/seed-demo.sh')
const D = encodeURIComponent(d.id)

const schema = await json(`/api/dashboards/${D}/schema`)
const groups = await json(`/api/dashboards/${D}/groups`)
const recentGroups = await json(`/api/dashboards/${D}/groups/recent`)
const prefs = await json('/api/me/prefs')

const ids = []
for (const deleted of [false, true]) {
  const list = await json(`/api/dashboards/${D}/records?limit=500${deleted ? '&deleted=1' : ''}`)
  ids.push(...list.items.map(r => r.id))
}
const records = []
for (const id of ids) records.push((await json(`/api/records/${encodeURIComponent(id)}`)).record)

const audit = []
let before = ''
for (;;) {
  const page = await json(`/api/dashboards/${D}/audit?limit=200${before ? `&before=${encodeURIComponent(before)}` : ''}`)
  audit.push(...page.operations)
  if (!page.next) break
  before = page.next
}

// файлы изображений: значения полей типа image
const imageKeys = new Set()
for (const t of schema.templates) for (const f of t.definition.fields ?? []) if (f.type === 'image') imageKeys.add(f.key)
const shas = new Set()
for (const r of records) {
  for (const [k, f] of Object.entries(r.fields)) {
    if (imageKeys.has(k)) for (const v of f.values) if (typeof v.value === 'string') shas.add(v.value)
  }
}
const files = []
for (const sha of shas) {
  const res = await req('GET', `/api/files/${D}/${sha}`)
  const buf = Buffer.from(await res.arrayBuffer())
  files.push({ sha256: sha, mime: res.headers.get('content-type') ?? 'image/png', data: buf.toString('base64') })
}

const out = {
  source: 'deploy/scripts/seed-demo.sh',
  account: me.account,
  dashboard: d,
  prefs,
  groups,
  recentGroups,
  /** строки dashboard_template: ключ шаблона → id (схема строится из pack-storage) */
  templateIds: Object.fromEntries(schema.templates.map(t => [t.key, t.id])),
  /** общие свои поля шаблонов дашборда (F-96): ключ шаблона → версия и поля */
  sharedFields: Object.fromEntries(schema.templates
    .filter(t => t.definition.custom_fields?.length)
    .map(t => [t.key, { rev: t.rev ?? 0, custom_fields: t.definition.custom_fields }])),
  records,
  audit,
  files,
}
writeFileSync(OUT, JSON.stringify(out, null, 1) + '\n')
console.log(`записей ${records.length}, операций журнала ${audit.length}, файлов ${files.length} → ${OUT}`)
