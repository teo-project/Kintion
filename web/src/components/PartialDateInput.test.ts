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

  it('только цифры с клавиатуры; значение снаружи показывается', async () => {
    const w = mount(PartialDateInput, { props: { modelValue: { y: 1985, m: 3, d: 12 } } })
    expect(w.findAll('input').map(i => (i.element as HTMLInputElement).value)).toEqual(['12', '3', '1985'])
    await w.findAll('input')[2]!.setValue('19x8')
    expect((w.findAll('input')[2]!.element as HTMLInputElement).value).toBe('198')
  })
})
