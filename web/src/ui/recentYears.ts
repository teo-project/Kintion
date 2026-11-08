// Недавние годы выбора даты (F-142): 3 последних введённых года — в браузере,
// забываются через 4 часа после последнего использования. Хранилище может быть
// недоступно (приватный режим) — тогда список просто пуст.
const KEY = 'kintion-recent-years'
const TTL = 4 * 60 * 60 * 1000
const MAX = 3

interface Stored { years: number[]; at: number }

export function recentYears(now = Date.now()): number[] {
  try {
    const s = JSON.parse(localStorage.getItem(KEY) ?? 'null') as Stored | null
    if (!s || !Array.isArray(s.years) || now - s.at > TTL) return []
    return s.years.filter(y => Number.isInteger(y) && y !== 0).slice(0, MAX)
  } catch {
    return []
  }
}

/** год использован: в начало списка, без повторов; срок — от этого момента */
export function rememberYear(y: number, now = Date.now()): void {
  if (!Number.isInteger(y) || y === 0) return
  try {
    const years = [y, ...recentYears(now).filter(x => x !== y)].slice(0, MAX)
    localStorage.setItem(KEY, JSON.stringify({ years, at: now }))
  } catch {
    // хранилище недоступно — недавних нет
  }
}
