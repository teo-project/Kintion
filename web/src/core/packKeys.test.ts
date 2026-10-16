// Проверка «пакам — данные» (план §3 п.7): в рабочем коде web/src нет строк
// с id и ключами паков — ни kin.*, ни ключей полей, ролей, этапов, шаблонов
// и сущностей. Разрешены только системные ключи. src/demo — не рабочий код:
// демо-сборка на снимке seed-demo.sh (знает его записи), в обычную сборку не входит.
import ts from 'typescript'
import { describe, expect, it } from 'vitest'

import { ruFiles } from './__tests__/kit'
import type { TemplateDef } from './types'

const sources = import.meta.glob(['../**/*.ts', '../**/*.vue', '!../**/*.test.ts', '!../**/__tests__/**', '!../demo/**'], {
  eager: true, import: 'default', query: '?raw',
}) as Record<string, string>

const allowed = new Set(['start', 'end', 'stage', 'title', 'note', 'id', 'default', 'photo'])
const fieldTypes = new Set([
  'text', 'number', 'partial_date', 'bool', 'choice', 'ref', 'image', 'place', 'phone', 'email', 'url', 'contact',
])

function packKeys(): Set<string> {
  const keys = new Set<string>()
  for (const json of Object.values(ruFiles)) {
    const j = json as Partial<TemplateDef> & { files?: unknown }
    if (j.files) continue
    if (j.id) keys.add(j.id)
    for (const f of j.fields ?? []) keys.add(f.key)
    for (const r of j.roles ?? []) keys.add(r.key)
    for (const s of j.stages ?? []) keys.add(s.key)
  }
  for (const k of [...keys]) if (allowed.has(k) || fieldTypes.has(k)) keys.delete(k)
  return keys
}

function literals(path: string, text: string): string[] {
  let code = text
  if (path.endsWith('.vue')) {
    code = [...text.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)].map(m => m[1]).join('\n')
  }
  const out: string[] = []
  const visit = (n: ts.Node) => {
    if (ts.isStringLiteralLike(n) || ts.isTemplateHead(n) || ts.isTemplateMiddle(n) || ts.isTemplateTail(n)) {
      out.push(n.text)
    }
    ts.forEachChild(n, visit)
  }
  visit(ts.createSourceFile(path, code, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS))
  return out
}

describe('пакам — данные', () => {
  it('в коде web/src нет ключей паков', () => {
    const forbidden = packKeys()
    expect(forbidden.has('surname') && forbidden.has('partner') && forbidden.has('gender')).toBe(true)
    expect(Object.keys(sources).some(p => p.endsWith('resolver.ts'))).toBe(true)
    const bad: string[] = []
    for (const [path, text] of Object.entries(sources)) {
      for (const s of literals(path, text)) {
        if (s.includes('kin.') || forbidden.has(s)) bad.push(`${path}: ${JSON.stringify(s)}`)
      }
    }
    expect(bad).toEqual([])
  })
})
