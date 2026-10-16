// Тестовая семья дерева (по данным прототипа F-114): 43 человека, 7 рядов —
// братья без родителей, поздний ребёнок, петля, два брака, отчим, усыновление,
// опекун, родители без отношения, предки из других родов.
import type { TreeInput } from '../model'

// id, год рождения, умер
const P: [string, number, boolean][] = [
  ['fyodor', 1858, true], ['pyotr', 1860, true], ['maria', 1864, true], ['anna', 1865, true], ['evdokia', 1862, true],
  ['grigory', 1890, true], ['ivan', 1888, true], ['elena', 1892, true], ['nikolai', 1905, true], ['zinaida', 1907, true],
  ['boris', 1932, true], ['sergei', 1915, true], ['lidiya', 1917, true], ['tamara', 1922, true], ['akim', 1858, true],
  ['ulyana', 1862, true], ['semyon', 1890, true], ['praskovya', 1895, true], ['vera', 1919, true], ['mikhail', 1915, true],
  ['olga', 1940, true], ['andrei', 1950, false], ['irina', 1952, false], ['viktor', 1950, false], ['gennady', 1925, true],
  ['zoya', 1928, true], ['natalia', 1953, false], ['oleg', 1950, false], ['pavel', 1946, true], ['kirill', 1974, false],
  ['dmitry', 1972, false], ['ksenia', 1974, false], ['alyona', 1989, false], ['maksim', 1980, false], ['sonya', 1985, false],
  ['artyom', 1996, false], ['polina', 1999, false], ['daria', 1997, false], ['pyotr-k', 1940, true], ['nina', 1943, false],
  ['igor', 1968, false], ['marina', 1970, false], ['lev', 2022, false],
]

// пара: id, a, b, начало, дети ([id, вид])
const U: [string, string, string, string, (string | [string, string])[]][] = [
  ['u-fyodor', 'fyodor', 'evdokia', '1886', ['grigory']],
  ['u-pyotr', 'pyotr', 'anna', '1885', ['ivan', 'nikolai']],
  ['u-ivan', 'ivan', 'elena', '1912', ['sergei', 'vera']],
  ['u-nik1', 'nikolai', 'zinaida', '1930', ['boris']],
  ['u-nik2', 'nikolai', 'olga', '1972', ['kirill']],
  ['u-sergei1', 'sergei', 'lidiya', '1938', ['olga']],
  ['u-sergei2', 'sergei', 'tamara', '1948', ['andrei', 'natalia']],
  ['u-akim', 'akim', 'ulyana', '1886', ['semyon']],
  ['u-novikov', 'gennady', 'zoya', '1949', ['irina']],
  ['u-koval0', 'pyotr-k', 'nina', '1965', ['igor']],
  ['u-belov', 'semyon', 'praskovya', '1919', ['tamara']],
  ['u-andrei', 'andrei', 'irina', '1971', ['dmitry']],
  ['u-irina2', 'irina', 'viktor', '1987', ['alyona']],
  ['u-natalia', 'natalia', 'oleg', '1978', ['maksim', ['sonya', 'adopted']]],
  ['u-dmitry', 'dmitry', 'ksenia', '1995', ['artyom', 'polina']],
  ['u-koval', 'igor', 'marina', '1994', ['daria']],
  ['u-artyom', 'artyom', 'daria', '2020', ['lev']],
]

export function family(): TreeInput {
  const input: TreeInput = {
    persons: P.map(([id, year, dead]) => ({ id, year, dead })),
    couples: U.map(([id, a, b, order]) => ({ id, a, b, order, label: 'Брак' })),
    siblings: [{ id: 'sib-top', members: ['fyodor', 'pyotr', 'maria'], label: 'Брат / сестра' }],
    descents: [],
  }
  for (const [id, a, b, , kids] of U) {
    for (const k of kids) {
      const [child, kind] = typeof k === 'string' ? [k, ''] : k
      for (const parent of [a, b]) {
        input.descents.push({ id: `${id}:${parent}>${child}`, parent, child, dashed: !!kind, kind: kind ? 'Приёмный' : undefined, label: kind ? 'Приёмный родитель' : undefined })
      }
    }
  }
  // родители без отношения; отчим; опекун
  input.descents.push(
    { id: 'pc-vera', parent: 'vera', child: 'pavel' },
    { id: 'pc-mikhail', parent: 'mikhail', child: 'pavel' },
    { id: 'pc-viktor', parent: 'viktor', child: 'dmitry', dashed: true, kind: 'Неродной', label: 'Отчим' },
    { id: 'pc-guard', parent: 'vera', child: 'boris', dashed: true, kind: 'Опекун', label: 'Опекунша' },
  )
  return input
}
