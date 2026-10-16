// Строки интерфейса (В-8, Д-16): языковые файлы i18n/<язык>.json в корне
// репозитория — общие с сервером. В бандл входят все языки; язык инстанса
// (F-38) сервер передаёт атрибутом <html lang> при отдаче index.html.
import en from '@i18n/en.json'
import ru from '@i18n/ru.json'

import type { Words } from '@/core/registry'

export type Messages = typeof ru

const dictionaries: Record<string, Messages> = { ru, en }

export const defaultLanguage = 'ru'

export function messagesFor(lang: string): Messages {
  return dictionaries[lang] ?? ru
}

export const t: Messages = messagesFor(
  typeof document === 'undefined' ? defaultLanguage : document.documentElement.lang,
)

/** Слова для ядра (web/src/core): даты, «Да / Нет», запасные подписи системных полей. */
export function coreWords(m: Messages): Words {
  return { dates: m.dates, yes: m.values.yes, no: m.values.no, systemFields: m.systemFields, sections: m.sections }
}

/** Подстановка `{имя}` в строку языкового файла. */
export function fmt(s: string, values: Record<string, string | number>): string {
  return s.replace(/\{(\w+)\}/g, (all, k: string) => (k in values ? String(values[k]) : all))
}

/** Текст ошибки API для показа: известные коды — из языкового файла, иначе — сообщение сервера. */
export function errorText(e: unknown, m: Messages = t): string {
  if (e && typeof e === 'object' && 'code' in e) {
    const code = String((e as { code: unknown }).code)
    const known = (m.errors as Record<string, string | undefined>)[code]
    if (known) return known
    if ('message' in e && (e as { message: unknown }).message) return String((e as { message: unknown }).message)
  }
  return fmt(m.errors.unknown, { message: e instanceof Error ? e.message : String(e) })
}
