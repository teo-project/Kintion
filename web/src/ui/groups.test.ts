import { describe, expect, it } from 'vitest'

import type { GroupDTO } from '@/api/types'
import { records, ruSchema } from '@/core/__tests__/kit'

import { ancestors, descendants, relatedGroups, suggestGroups } from './groups'

const g = (id: string, parents: string[] = [], children: string[] = []): GroupDTO => ({ id, parents, children, name: id, count: 0, auto_surname: false, surnames: [] })
// «Ивановы» — и в «Семье», и в «Гостях» (F-105)
const groups = [
  g('family', [], ['ivanov']), g('guests', [], ['ivanov']), g('ivanov', ['family', 'guests'], ['kids']),
  g('kids', ['ivanov']), g('work'),
]

describe('группы', () => {
  it('вложенные и объемлющие по всем путям (цикл выбрать нельзя)', () => {
    expect([...descendants(groups, 'family')].sort()).toEqual(['family', 'ivanov', 'kids'])
    expect([...descendants(groups, 'guests')].sort()).toEqual(['guests', 'ivanov', 'kids'])
    expect([...ancestors(groups, 'kids')].sort()).toEqual(['family', 'guests', 'ivanov', 'kids'])
    expect([...ancestors(groups, 'work')]).toEqual(['work'])
  })

  it('предложения (F-04): связанные, затем последние; не больше четырёх, без выбранных', () => {
    expect(suggestGroups(groups, ['ivanov'], ['work', 'gone', 'ivanov'], []).map(x => x.id)).toEqual(['ivanov', 'work'])
    expect(suggestGroups(groups, ['ivanov'], ['work'], ['ivanov']).map(x => x.id)).toEqual(['work'])
  })

  it('группы связанных записей: участники, контейнеры и их участники', () => {
    const all = records(ruSchema(), [
      { id: 'anna', template: 'kin.core.person' },
      { id: 'ivan', template: 'kin.core.person' },
      { id: 'w', template: 'kin.core.wedding', parts: [['partner', 'anna'], ['partner', 'ivan']] },
    ])
    all.ivan!.groups = ['ivanov']
    all.w!.groups = ['family']
    all.anna!.groups = ['own']
    expect(relatedGroups(all.anna!, all).sort()).toEqual(['family', 'ivanov'])
  })
})
