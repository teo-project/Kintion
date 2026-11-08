import { flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { RecSpec } from '@/core/__tests__/kit'
import { envelope, mountRecord, serve, typeAndBlur } from '@/views/__tests__/card'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const liza: RecSpec = {
  id: 'liza', template: 'kin.core.person',
  fields: {
    name: ['Лиза'], surname: ['Василькова', 'Давыдко'], gender: ['female'],
    phone: ['+7 (900) 111-22-33', '+7 901'], email: ['liza@example.org'],
    website: ['example.org', 'javascript:alert(1)'], address: ['Москва, Тверская 1'],
    'custom.born': [{ y: 2001 }], 'custom.n': [42], 'custom.ok': [true], 'custom.friend': ['ivan'],
  },
  custom: [
    { key: 'born', type: 'partial_date', name: 'Дата' },
    { key: 'n', type: 'number', name: 'Число' },
    { key: 'ok', type: 'bool', name: 'Да ли' },
    { key: 'friend', type: 'ref', name: 'Друг' },
  ],
}
const ivan: RecSpec = { id: 'ivan', template: 'kin.core.person', fields: { name: ['Иван'] } }

function row(w: Awaited<ReturnType<typeof mountRecord>>, label: string) {
  const r = w.findAll('.field-row').find(x => x.find('.label').text() === label)
  if (!r) throw new Error(`no row ${label}`)
  return r
}

describe('значения полей по типам', () => {
  it('показ: tel:, mailto:, ссылка, слово варианта, место, multiple, дата, число, да/нет, ref', async () => {
    serve((m, u) => (m === 'GET' && u === '/api/records/liza'
      ? [200, { record: envelope(liza), related: { ivan: envelope(ivan) } }]
      : undefined))
    const w = await mountRecord()
    expect(row(w, 'Фамилия').find('.values').text()).toBe('Василькова') // текущее, не история
    expect(row(w, 'Пол').find('.values').text()).toBe('Женский')
    const phones = row(w, 'Телефон').findAll('a')
    expect(phones.map(a => a.attributes('href'))).toEqual(['tel:+79001112233', 'tel:+7901'])
    expect(row(w, 'Email').find('a').attributes('href')).toBe('mailto:liza@example.org')
    const links = row(w, 'Ссылки')
    expect(links.findAll('a').map(a => a.attributes('href'))).toEqual(['https://example.org'])
    expect(links.text()).toContain('javascript:alert(1)') // показан текстом, не ссылкой
    expect(row(w, 'Адрес').find('.values').text()).toBe('Москва, Тверская 1')
    expect(row(w, 'Дата').find('.values').text()).toBe('2001')
    expect(row(w, 'Число').find('.values').text()).toBe('42')
    expect(row(w, 'Да ли').find('.values').text()).toBe('Да')
    const ref = row(w, 'Друг').find('a')
    expect(ref.text()).toBe('Иван')
    expect(ref.attributes('href')).toBe('/d/d1/r/ivan')
  })

  it('контактное лицо организации — имя', async () => {
    const org: RecSpec = {
      id: 'org', template: 'kin.core.organization',
      fields: { name: ['Ромашка'], director: [{ value: 'Пётр Петров', meta: { phone: '+7 1' } }], website: ['https://romashka.example'] },
    }
    serve((m, u) => (m === 'GET' && u === '/api/records/org' ? [200, { record: envelope(org), related: {} }] : undefined))
    const w = await mountRecord('org')
    expect(w.find('h1').text()).toContain('Ромашка')
    expect(row(w, 'Директор').find('.values').text()).toBe('Пётр Петров')
    expect(row(w, 'Сайт').find('a').attributes('href')).toBe('https://romashka.example')
  })

  it('режим редактирования: текущее значение — PATCH с версией поля, новый телефон — вниз, пустое — удаление', async () => {
    let rec = envelope(liza)
    const calls = serve((m, u) => {
      if (u === '/api/me/prefs') return [200, { history_edit: 'replace' }] // запомнено: заменять (F-80)
      if (m === 'GET' && u.startsWith('/api/records/')) return [200, { record: rec, related: { ivan: envelope(ivan) } }]
      if (u.startsWith('/api/records/liza/fields/')) {
        rec = envelope(liza)
        return [200, { record: rec, related: {} }]
      }
      return undefined
    })
    const w = await mountRecord('liza', true)
    const surname = row(w, 'Фамилия')
    const sv = rec.fields.surname!.values[0]!.id
    expect(surname.findAll('input')).toHaveLength(1) // только текущее значение
    expect((surname.find('input').element as HTMLInputElement).value).toBe('Василькова')
    await typeAndBlur(surname.find('input'), 'Петрова')
    expect(calls.at(-1)).toMatchObject({ method: 'PATCH', url: `/api/records/liza/fields/surname/values/${sv}`, body: { if_rev: 2, value: 'Петрова' } })

    const phone = row(w, 'Телефон')
    const inputs = phone.findAll('input')
    expect(inputs).toHaveLength(3) // оба значения + новое
    await typeAndBlur(inputs[0]!, '')
    await typeAndBlur(row(w, 'Телефон').findAll('input')[2]!, ' +7 902 ')
    const writes = calls.filter(c => c.method !== 'GET' && c.url.startsWith('/api/records/')).slice(1)
    expect(writes[0]).toMatchObject({ method: 'DELETE', url: expect.stringMatching(/fields\/phone\/values\/.+\?if_rev=2$/) })
    expect(writes[1]).toMatchObject({ method: 'POST', url: '/api/records/liza/fields/phone/values', body: { if_rev: 2, value: '+7 902', position: 'bottom' } })
  })

  it('поле с историей: спросить «заменить или добавить», галочка запоминает выбор на сервере (F-80)', async () => {
    let rec = envelope(liza)
    const calls = serve((m, u) => {
      if (u === '/api/me/prefs') return [200, { history_edit: m === 'PUT' ? 'add' : '' }]
      if (m === 'GET' && u.startsWith('/api/records/')) return [200, { record: rec, related: { ivan: envelope(ivan) } }]
      if (u.startsWith('/api/records/liza/fields/')) {
        rec = envelope(liza)
        return [200, { record: rec, related: {} }]
      }
      return undefined
    })
    const w = await mountRecord('liza', true)
    const input = row(w, 'Фамилия').find('input')
    await input.trigger('focusin')
    await input.setValue('Петрова')
    await input.trigger('focusout')
    await flushPromises()
    const dialog = document.body.querySelector('.history-choice')!
    expect(dialog.textContent).toContain('Фамилия')
    ;(dialog.querySelector('[data-name="history-remember"]') as HTMLElement).click()
    ;(dialog.querySelector('.add') as HTMLButtonElement).click()
    await flushPromises()
    expect(calls.find(c => c.method === 'PUT')).toMatchObject({ url: '/api/me/prefs', body: { history_edit: 'add' } })
    expect(calls.find(c => c.method === 'POST')).toMatchObject({
      url: '/api/records/liza/fields/surname/values', body: { if_rev: 2, value: 'Петрова', position: 'top' },
    })
    expect(document.body.querySelector('.history-choice')).toBeNull()

    // выбор запомнен — дальше без вопроса
    await input.trigger('focusin')
    await input.setValue('Сидорова')
    await input.trigger('focusout')
    await flushPromises()
    expect(document.body.querySelector('.history-choice')).toBeNull()
    expect(calls.filter(c => c.method === 'POST')).toHaveLength(2)
  })
})

describe('форматированный текст (F-141)', () => {
  const story: RecSpec = {
    id: 'st', template: 'kin.core.person',
    fields: { name: ['Лиза'], 'custom.story': ['**Родилась** в _Горках_.<img src=x onerror="alert(1)"><a href="javascript:alert(1)">зло</a>'] },
    custom: [{ key: 'story', type: 'rich_text', name: 'История' }],
  }

  it('карточка — с форматированием и без опасного', async () => {
    serve((m, u) => (m === 'GET' && u === '/api/records/st' ? [200, { record: envelope(story), related: {} }] : undefined))
    const w = await mountRecord('st')
    const html = row(w, 'История').find('.rich-text').html()
    expect(html).toContain('<strong>Родилась</strong>')
    expect(html).toContain('<em>Горках</em>')
    expect(html).not.toMatch(/onerror|<img|javascript/)
  })

  it('правка: текст Markdown и предпросмотр, сохранение при уходе из поля', async () => {
    let rec = envelope(story)
    const calls = serve((m, u) => {
      if (u === '/api/me/prefs') return [200, { history_edit: 'replace' }]
      if (m === 'GET' && u.startsWith('/api/records/')) return [200, { record: rec, related: {} }]
      if (u.startsWith('/api/records/st/fields/')) {
        rec = envelope(story)
        return [200, { record: rec, related: {} }]
      }
      return undefined
    })
    const w = await mountRecord('st', true)
    const r = row(w, 'История')
    const area = r.find('textarea')
    expect((area.element as HTMLTextAreaElement).value).toContain('**Родилась**')
    const [, previewBtn] = r.findAll('.rich-modes button')
    await previewBtn!.trigger('click')
    expect(r.find('.rich-preview strong').text()).toBe('Родилась')
    await r.findAll('.rich-modes button')[0]!.trigger('click')
    await typeAndBlur(r.find('textarea'), '# Новое')
    expect(calls.at(-1)).toMatchObject({ method: 'PATCH', url: expect.stringMatching(/fields\/custom\.story\/values\//), body: { value: '# Новое' } })
  })
})
