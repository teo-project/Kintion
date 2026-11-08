// Контекст ядра для экранов: реестр, записи ответа (сама запись и related),
// слова языкового файла, «сегодня» (Д-20) и эры дашборда (F-143): порядок,
// показ, текущий год мира вместо настоящей даты.
import { eraOrder, todayDate } from '@/core/dates'
import type { Registry } from '@/core/registry'
import type { Ctx } from '@/core/resolver'
import type { RecordDTO } from '@/core/types'
import { coreWords, t } from '@/i18n'

/** «Сегодня»: год мира (F-143); не задан — настоящая дата. */
function worldToday(reg: Registry) {
  const w = reg.eras.today
  // эру по умолчанию новому году мира ставит сервер; пустая — как у дат (F-14)
  return w ?? todayDate()
}

export function makeCtx(reg: Registry, records: (RecordDTO | undefined)[], related: Record<string, RecordDTO | undefined> = {}): Ctx {
  const all: Record<string, RecordDTO | undefined> = { ...related }
  for (const r of records) if (r) all[r.id] = r
  return { reg, records: all, words: coreWords(t), today: worldToday(reg), eras: eraOrder(reg.eras), dateView: { showEra: reg.eras.show, noEra: reg.eras.default !== '' } }
}
