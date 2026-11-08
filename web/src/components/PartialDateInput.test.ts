import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import type { PartialDate } from '@/core/dates'

import PartialDateInput from './PartialDateInput.vue'

async function typeDate(d: string, m: string, y: string) {
  const w = mount(PartialDateInput, { props: { modelValue: null } })
  const [day, month, year] = w.findAll('input')
  await day!.setValue(d)
  await month!.setValue(m)
  await year!.setValue(y)
  const emitted = w.emitted('update:modelValue') ?? []
  return { w, value: emitted.at(-1)?.[0] as PartialDate | null | undefined }
}

describe('PartialDateInput', () => {
  it('пустые части — null; «12 марта» без года', async () => {
    const { w, value } = await typeDate('12', '3', '')
    expect(value).toEqual({ y: null, m: 3, d: 12 })
    expect(w.find('[role=alert]').exists()).toBe(false)
  })

  it('только год', async () => {
    const { value } = await typeDate('', '', '1923')
    expect(value).toEqual({ y: 1923, m: null, d: null })
  })

  it('всё пусто — null', async () => {
    const { value } = await typeDate('5', '', '')
    expect(value).toEqual({ y: null, m: null, d: 5 })
    const w = mount(PartialDateInput, { props: { modelValue: { d: 5 } } })
    await w.findAll('input')[0]!.setValue('')
    expect(w.emitted('update:modelValue')?.at(-1)?.[0]).toBeNull()
  })

  it('29.02 без года можно, в невисокосный год — нет, в високосный — да', async () => {
    expect((await typeDate('29', '2', '')).w.find('[role=alert]').exists()).toBe(false)
    expect((await typeDate('29', '2', '2023')).w.find('[role=alert]').text()).toBe('Нет такой даты')
    expect((await typeDate('29', '2', '2024')).w.find('[role=alert]').exists()).toBe(false)
  })

  it('31.04 — нет такой даты; 13-й месяц — ошибка', async () => {
    expect((await typeDate('31', '4', '')).w.find('[role=alert]').exists()).toBe(true)
    expect((await typeDate('', '13', '')).w.find('[role=alert]').exists()).toBe(true)
  })

  it('только цифры с клавиатуры; значение снаружи — с ведущим нулём; 4 в дне — 04', async () => {
    const w = mount(PartialDateInput, { props: { modelValue: { y: 1985, m: 3, d: 12 } } })
    expect(w.findAll('input').map(i => (i.element as HTMLInputElement).value)).toEqual(['12', '03', '1985'])
    await w.findAll('input')[2]!.setValue('19x8')
    expect((w.findAll('input')[2]!.element as HTMLInputElement).value).toBe('198')
    await w.findAll('input')[0]!.setValue('4')
    expect((w.findAll('input')[0]!.element as HTMLInputElement).value).toBe('04')
  })

  it('«−» в годе — год до начала эры; запрещённые отрицательные и вне пределов — не вводятся', async () => {
    const w = mount(PartialDateInput, { props: { modelValue: null } })
    await w.findAll('input')[2]!.setValue('44')
    await w.findAll('input')[2]!.trigger('keydown', { key: '-' })
    expect(w.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ y: -44, m: null, d: null })
    const v = mount(PartialDateInput, { props: { modelValue: { y: -500, m: 3 } } })
    expect((v.findAll('input')[2]!.element as HTMLInputElement).value).toBe('-500')
    const p = mount(PartialDateInput, { props: { modelValue: null, allowNegative: false, yearMax: 2000 } })
    await p.findAll('input')[2]!.setValue('-44')
    expect(p.emitted('update:modelValue')).toBeUndefined()
    await p.findAll('input')[2]!.setValue('1900')
    await p.findAll('input')[2]!.setValue('2100')
    expect((p.findAll('input')[2]!.element as HTMLInputElement).value).toBe('1900')
    // положительный нижний предел
    // положительный нижний предел: набирается, ниже — ошибка; минус из месяца — не за предел
    const lo = mount(PartialDateInput, { props: { modelValue: null, yearMin: 1800 } })
    await lo.findAll('input')[2]!.setValue('1')
    expect(lo.find('[role=alert]').exists()).toBe(true)
    await lo.findAll('input')[2]!.setValue('1850')
    expect(lo.find('[role=alert]').exists()).toBe(false)
    await lo.findAll('input')[1]!.trigger('keydown', { key: '-' })
    expect((lo.findAll('input')[2]!.element as HTMLInputElement).value).toBe('1850')
  })

  it('пределы — из эры даты в настройках дашборда; у новой даты — эра по умолчанию; min ≥ 1 — без минуса', async () => {
    const eras = { edit: false, default: 'AC', list: ['AC'], years: { AC: { min: 1, max: 400 }, BC: { min: 5000, max: 1 } } }
    const w = mount(PartialDateInput, { props: { modelValue: null, eras } })
    await w.findAll('input')[2]!.setValue('500')
    expect(w.emitted('update:modelValue')).toBeUndefined()
    await w.findAll('input')[2]!.setValue('300')
    await w.findAll('input')[2]!.trigger('keydown', { key: '-' })
    expect(w.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ y: 300, m: null, d: null })
    // обратный счёт: пределы 1 … 5000
    const b = mount(PartialDateInput, { props: { modelValue: { y: 10, e: 'BC' }, eras } })
    await b.findAll('input')[2]!.setValue('4000')
    expect(b.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ y: 4000, m: null, d: null, e: 'BC' })
  })

  it('недавние годы — 3 последних, на 4 часа', async () => {
    const { recentYears, rememberYear } = await import('@/ui/recentYears')
    localStorage.clear()
    rememberYear(1894, 0)
    rememberYear(1917, 1)
    rememberYear(1612, 2)
    rememberYear(1894, 3)
    expect(recentYears(4)).toEqual([1894, 1612, 1917])
    rememberYear(1700, 5)
    expect(recentYears(6)).toEqual([1700, 1894, 1612])
    expect(recentYears(5 + 4 * 3600 * 1000 + 1)).toEqual([])
  })

  it('календарь: год → месяц → день; недавние — только введённый год', async () => {
    localStorage.clear()
    const w = mount(PartialDateInput, { props: { modelValue: { y: 1894 } }, attachTo: document.body })
    await w.find('button.cal').trigger('click')
    await new Promise(r => setTimeout(r, 0))
    const pop = () => document.body.querySelector('.hdp')!
    // год есть — сразу месяцы
    expect(pop().querySelector('.head-title')!.textContent).toContain('1894')
    const mar = [...pop().querySelectorAll<HTMLButtonElement>('.cell')].find(b => b.textContent?.trim() === 'мар')!
    mar.click()
    await w.vm.$nextTick()
    expect(pop().querySelector('.weekdays')).not.toBeNull()
    const d14 = [...pop().querySelectorAll<HTMLButtonElement>('.cell')].find(b => b.textContent?.trim() === '14')!
    d14.click()
    await w.vm.$nextTick()
    expect(w.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ y: 1894, m: 3, d: 14 })
    const { recentYears } = await import('@/ui/recentYears')
    await w.findAll('input')[0]!.trigger('blur')
    expect(recentYears()).toEqual([])
    await w.findAll('input')[2]!.setValue('1895')
    await w.findAll('input')[2]!.trigger('blur')
    expect(recentYears()).toEqual([1895])
    w.unmount()
  })

  it('эра: поле — только если её можно менять; иначе эра даты сохраняется, у новой — пустая (по умолчанию ставит сервер)', async () => {
    const hidden = { edit: false, default: 'AC', list: ['', 'AC'] }
    const w = mount(PartialDateInput, { props: { modelValue: { y: 300, e: 'ACC' }, eras: hidden } })
    expect(w.findAll('input')).toHaveLength(3)
    await w.findAll('input')[0]!.setValue('5')
    expect(w.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ y: 300, m: null, d: 5, e: 'ACC' })
    // год стёрли и ввели снова — эра по умолчанию (явно: дата уже записана,
    // сервер её не поставит), а не прежняя
    await w.findAll('input')[2]!.setValue('')
    expect(w.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ y: null, m: null, d: 5 })
    await w.findAll('input')[2]!.setValue('301')
    expect(w.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ y: 301, m: null, d: 5, e: 'AC' })
    // дата без эры при правке остаётся без эры
    const ne = mount(PartialDateInput, { props: { modelValue: { y: 300 }, eras: hidden } })
    await ne.findAll('input')[2]!.setValue('302')
    expect(ne.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ y: 302, m: null, d: null })
    const n = mount(PartialDateInput, { props: { modelValue: null, eras: hidden } })
    await n.findAll('input')[2]!.setValue('12')
    expect(n.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ y: 12, m: null, d: null })
    await n.findAll('input')[2]!.setValue('')
    await n.findAll('input')[1]!.setValue('3')
    expect(n.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ y: null, m: 3, d: null })

    const e = mount(PartialDateInput, { props: { modelValue: null, eras: { edit: true, default: '', list: ['', 'AC', 'BC'] } } })
    const inputs = e.findAll('input')
    expect(inputs).toHaveLength(4)
    expect(e.findAll('datalist option').map(o => o.attributes('value'))).toEqual(['AC', 'BC'])
    await inputs[2]!.setValue('300')
    await inputs[3]!.setValue('BC')
    expect(e.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ y: 300, m: null, d: null, e: 'BC' })
  })

  it('эра по умолчанию — подсказкой в пустом поле новой даты; у записанной эра как есть; дата из одной эры (F-14)', async () => {
    const eras = { edit: true, default: 'AC', list: ['AC', 'Мезозой'] }
    const w = mount(PartialDateInput, { props: { modelValue: { y: 300, e: 'AC' }, eras } })
    expect((w.findAll('input')[3]!.element as HTMLInputElement).value).toBe('AC')
    expect(w.emitted('update:modelValue')).toBeUndefined()
    // записанная дата без эры — пустое поле без подсказки эры по умолчанию
    const none = mount(PartialDateInput, { props: { modelValue: { y: 300 }, eras } })
    expect((none.findAll('input')[3]!.element as HTMLInputElement).value).toBe('')
    expect(none.findAll('input')[3]!.attributes('placeholder')).toBe('Эра')
    const fresh = mount(PartialDateInput, { props: { modelValue: null, eras } })
    expect(fresh.findAll('input')[3]!.attributes('placeholder')).toBe('AC')
    // одна эра без года, месяца и дня — дата
    const o = mount(PartialDateInput, { props: { modelValue: null, eras } })
    await o.findAll('input')[3]!.setValue('Мезозой')
    expect(o.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ y: null, m: null, d: null, e: 'Мезозой' })
    // эра по умолчанию одна — тоже дата, поле не пустеет
    const d = mount(PartialDateInput, { props: { modelValue: { e: 'AC' }, eras } })
    expect((d.findAll('input')[3]!.element as HTMLInputElement).value).toBe('AC')
    expect(d.emitted('update:modelValue')).toBeUndefined()
    // менять эру нельзя, но дата из одной эры — поле видно, его можно стереть
    const h = mount(PartialDateInput, { props: { modelValue: { e: 'Мезозой' }, eras: { ...eras, edit: false } } })
    expect(h.findAll('input')).toHaveLength(4)
    await h.findAll('input')[3]!.setValue('')
    expect(h.emitted('update:modelValue')?.at(-1)?.[0]).toBeNull()
    // эра с месяцем без года — ошибка
    await o.findAll('input')[1]!.setValue('3')
    expect(o.find('[role=alert]').exists()).toBe(true)
  })
})
