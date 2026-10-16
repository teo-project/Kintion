// Демо-сервер: снимок совпадает с конвертами API (testdata/api), фильтры
// списка, вход, запись в памяти, подстановка ошибки (состояния полей F-86).
import { describe, expect, it } from 'vitest'

import person from '@testdata/api/record_person.json'
import fieldsRef from '@testdata/api/record_fields.json'

import type { ListOut, OperationOut, RecordOut } from '@/api/types'

import fixturesRaw from './fixtures.json'
import { DemoServer, EMPTY_DASHBOARD, type Fixtures } from './server'
import { demoStates } from './states'

const fx = fixturesRaw as unknown as Fixtures
const D = fx.dashboard.id
const call = (s: DemoServer, method: string, path: string, body?: unknown) =>
  s.handle(method, new URL(path, 'http://demo'), body)

describe('демо-сервер', () => {
  it('записи снимка — в форме конверта API', () => {
    const keys = Object.keys(person.record).sort()
    const valueKeys = Object.keys(Object.values(fieldsRef.record.fields)[0]!.values[0]!).sort()
    for (const r of fx.records) {
      expect(Object.keys(r).sort()).toEqual(keys)
      for (const f of Object.values(r.fields)) for (const v of f!.values) expect(Object.keys(v).sort()).toEqual(valueKeys)
    }
  })

  it('реестр — из pack-storage со строками шаблонов снимка', async () => {
    const res = await call(new DemoServer(fx), 'GET', `/api/dashboards/${D}/schema`)
    const schema = res.body as { entities: { id: string }[]; templates: { key: string; id: string; default: boolean }[] }
    expect(schema.entities.map(e => e.id)).toContain('kin.core.person')
    const tpl = schema.templates.find(t => t.key === 'kin.core.person')
    expect(tpl).toMatchObject({ id: fx.templateIds['kin.core.person'], default: true })
    // общее поле людей дашборда (F-96) — из снимка: «выбор или своё»
    const shared = (tpl as unknown as { rev: number; definition: { custom_fields?: { key: string; free?: boolean }[] } })
    expect(shared.rev).toBe(1)
    expect(shared.definition.custom_fields).toEqual([expect.objectContaining({ key: 'faith', free: true })])
    // у другого дашборда своих общих полей нет
    const empty = (await call(new DemoServer(fx), 'GET', `/api/dashboards/${EMPTY_DASHBOARD}/schema`)).body as typeof schema
    expect(empty.templates.find(t => t.key === 'kin.core.person')).not.toHaveProperty('rev')
  })

  it('вход: без сессии — 401, неверный пароль — bad_credentials', async () => {
    const s = new DemoServer(fx, { auth: 'guest' })
    expect((await call(s, 'GET', '/api/me')).status).toBe(401)
    expect((await call(s, 'POST', '/api/login', { login: 'demo', password: 'x' })).body).toMatchObject({ error: 'bad_credentials' })
    expect((await call(s, 'POST', '/api/login', { login: 'demo', password: 'demo-password-0.1' })).status).toBe(200)
    expect((await call(s, 'GET', '/api/me')).status).toBe(200)
  })

  it('дерево: отношения с tree_link и их участники, фильтр по группе', async () => {
    const s = new DemoServer(fx)
    const tree = async (q = '') => (await call(s, 'GET', `/api/dashboards/${D}/tree${q}`)).body as { relations: { participants: { entity_id: string }[] }[]; records: { id: string }[] }
    const all = await tree()
    expect(all.records.length).toBeGreaterThan(43)
    const ids = new Set(all.records.map(r => r.id))
    expect(all.relations.every(r => r.participants.every(p => ids.has(p.entity_id)))).toBe(true)
    const lebedev = fx.groups.find(g => g.name === 'Лебедевы')!.id
    expect((await tree(`?group=${lebedev}`)).records).toHaveLength(43)
    expect((await call(s, 'GET', `/api/dashboards/${D}/tree?group=nope`)).status).toBe(400)
  })

  it('список: сущность, поиск, группа, корзина, пустой дашборд', async () => {
    const s = new DemoServer(fx)
    const list = async (q: string, d = D) => (await call(s, 'GET', `/api/dashboards/${d}/records?${q}`)).body as ListOut
    const people = await list('entity=kin.core.person')
    expect(people.total).toBe(53) // 10 + род Лебедевых для дерева
    expect((await list('entity=kin.core.person&q=василь')).items.length).toBeGreaterThan(1)
    // «Петровы» — и в «Семье», и в «Гостях» (F-105): фильтр по «Семье» находит и их
    const family = fx.groups.find(g => g.name === 'Семья')!.id
    const petrovs = fx.groups.find(g => g.name === 'Петровы')!
    expect(petrovs.parents).toHaveLength(2)
    const inFamily = (await list(`entity=kin.core.person&group=${family}`)).items
    expect(inFamily.every(r => r.groups.includes(family) || r.groups.includes(petrovs.id))).toBe(true)
    expect(inFamily.some(r => r.groups.includes(petrovs.id))).toBe(true)
    expect((await list('deleted=1')).total).toBe(1)
    expect((await list('', EMPTY_DASHBOARD)).total).toBe(0)
  })

  it('операция создаёт запись, значение и участие; ошибка подставляется один раз', async () => {
    const s = new DemoServer(fx)
    const liza = fx.records.find(r => r.fields.name?.values[0]?.value === 'Лиза')!.id
    const op = (await call(s, 'POST', `/api/dashboards/${D}/operations`, {
      records: [{ ref: 'n', template: 'kin.core.friend' }, { ref: 'm', template: 'kin.core.person', fields: { name: [{ value: 'Мария' }] } }],
      participants: [{ container: 'n', entity: liza, role: 'friend' }, { container: 'n', entity: 'm', role: 'friend' }],
    })).body as OperationOut
    const rel = (await call(s, 'GET', `/api/records/${op.ids.n}`)).body as RecordOut
    expect(rel.record.participants.map(p => p.entity_id)).toEqual([liza, op.ids.m])
    expect(rel.related[op.ids.m!]?.fields.name?.values[0]?.value).toBe('Мария')

    s.fault = { status: 400, body: { error: 'validation', field: 'phone' } }
    const add = () => call(s, 'POST', `/api/records/${liza}/fields/phone/values`, { if_rev: 0, value: '+7', position: 'bottom' })
    expect((await add()).status).toBe(400)
    const ok = (await add()).body as RecordOut
    expect(ok.record.fields.phone?.values.at(-1)?.value).toBe('+7')
  })

  it('журнал — только у дашборда снимка', async () => {
    const s = new DemoServer(fx)
    const ops = async (d: string) => ((await call(s, 'GET', `/api/dashboards/${d}/audit`)).body as { operations: unknown[] }).operations
    expect((await ops(D)).length).toBe(fx.audit.length)
    expect(await ops(EMPTY_DASHBOARD)).toEqual([])
  })

  it('состояния собираются по снимку', () => {
    const states = demoStates(fx)
    expect(new Set(states.map(s => s.id)).size).toBe(states.length)
    expect(states.some(s => s.path.includes('undefined'))).toBe(false)
  })

  it('группы (F-105): цикл и несуществующая группа — 400, состояние не меняется', async () => {
    const s = new DemoServer(fx)
    const path = `/api/dashboards/${D}/groups`
    const groups = async () => (await call(s, 'GET', path)).body as { id: string; name: string; parents: string[] }[]
    const a = ((await call(s, 'POST', path, { name: 'A' })).body as { id: string }).id
    const b = ((await call(s, 'POST', path, { name: 'B', parents: [a] })).body as { id: string }).id
    const n = (await groups()).length
    expect((await call(s, 'POST', path, { name: 'N', parents: [b], children: [a] })).status).toBe(400)
    expect((await call(s, 'POST', path, { name: 'M', parents: ['нет-такой'] })).status).toBe(400)
    expect((await call(s, 'PATCH', `${path}/${a}`, { parents: ['нет-такой'] })).status).toBe(400)
    const after = await groups()
    expect(after).toHaveLength(n)
    expect(after.find(g => g.id === a)!.parents).toEqual([])
    // и во внутреннем состоянии нет ссылок на несозданную группу
    expect(s.groups.get(D)!.flatMap(g => g.parents).every(p => after.some(g => g.id === p))).toBe(true)
  })

  it('группы (F-105): удаление разрывает связи, вложенные к родителям не переходят', async () => {
    const s = new DemoServer(fx)
    const path = `/api/dashboards/${D}/groups`
    const groups = async () => (await call(s, 'GET', path)).body as { id: string; parents: string[]; children: string[] }[]
    const a = ((await call(s, 'POST', path, { name: 'A' })).body as { id: string }).id
    const b = ((await call(s, 'POST', path, { name: 'B', parents: [a] })).body as { id: string }).id
    const c = ((await call(s, 'POST', path, { name: 'C', parents: [b] })).body as { id: string }).id
    expect((await call(s, 'DELETE', `${path}/${b}`)).status).toBe(204)
    const after = await groups()
    expect(after.some(g => g.id === b)).toBe(false)
    expect(after.find(g => g.id === c)!.parents).toEqual([])
    expect(after.find(g => g.id === a)!.children).toEqual([])
  })
})
