// Эталоны ответов сервера (testdata/api, пишут Go-тесты) сверяются с типами
// клиента: присваивание проверяет vue-tsc, набор ключей конверта — тест.
import { describe, expect, it } from 'vitest'

import fields from '@testdata/api/record_fields.json'
import person from '@testdata/api/record_person.json'

import type { RecordDTO, RecordOut } from './types'

const personOut: RecordOut = person
const fieldsOut: Pick<RecordOut, 'record'> = fields

const envelopeKeys: (keyof RecordDTO)[] = [
  'id', 'entity', 'template', 'template_id', 'start', 'end', 'note', 'title_manual', 'sensitive',
  'groups', 'custom_fields', 'fields', 'revs', 'participants', 'participates_in', 'deleted',
]

describe('эталоны API', () => {
  it('конверт записи совпадает с типом', () => {
    for (const rec of [personOut.record, fieldsOut.record]) {
      expect(Object.keys(rec).sort()).toEqual([...envelopeKeys].sort())
    }
    expect(personOut.related).toEqual({})
  })

  it('значение поля несёт все ключи', () => {
    const v = Object.values(fieldsOut.record.fields)[0]!.values[0]!
    expect(Object.keys(v).sort()).toEqual(['id', 'links', 'meta', 'note', 'valid_from', 'valid_to', 'value'])
  })
})
