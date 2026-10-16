// Контекст ядра для экранов: реестр, записи ответа (сама запись и related),
// слова языкового файла и «сегодня» (Д-20).
import { todayISO } from '@/core/dates'
import type { Registry } from '@/core/registry'
import type { Ctx } from '@/core/resolver'
import type { RecordDTO } from '@/core/types'
import { coreWords, t } from '@/i18n'

export function makeCtx(reg: Registry, records: (RecordDTO | undefined)[], related: Record<string, RecordDTO | undefined> = {}): Ctx {
  const all: Record<string, RecordDTO | undefined> = { ...related }
  for (const r of records) if (r) all[r.id] = r
  return { reg, records: all, words: coreWords(t), today: todayISO() }
}
