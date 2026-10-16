// Реестр демо-сборки: сущности и шаблоны паков по умолчанию прямо из
// pack-storage (как их читает сервер, packstorage.DefaultPacks) — в порядке
// файлов пака. Строки dashboard_template (id) — из снимка демо-данных.
import type { EntityDef, FieldDef, SchemaDTO, SchemaTemplateDTO, TemplateDef } from '@/core/types'

/** паки, включённые по умолчанию (TEO_KIN_PACKS, Д-31) */
const PACKS = ['kin.core']
const LANG = 'ru'

const files = import.meta.glob<unknown>('../../../pack-storage/ru/kin.core/**/*.json', { eager: true, import: 'default' })

interface PackManifest {
  id: string
  version: string
  files: string[]
}

function file(pack: string, rel: string): unknown {
  const v = files[`../../../pack-storage/${LANG}/${pack}/${rel}`]
  if (v === undefined) throw new Error(`демо: нет файла пака ${pack}/${rel}`)
  return v
}

/** Реестр дашборда; templateIds — ключ шаблона → строка dashboard_template. */
/** Общие свои поля шаблона в дашборде (F-96): ключ шаблона → версия и поля. */
export type SharedFields = Record<string, { rev: number; custom_fields: FieldDef[] }>

export function buildSchema(templateIds: Record<string, string>, shared: SharedFields = {}): SchemaDTO {
  const out: SchemaDTO = { language: LANG, entities: [], templates: [] }
  for (const pack of PACKS) {
    const manifest = file(pack, `${pack}.pack.json`) as PackManifest
    for (const rel of manifest.files) {
      const def = file(pack, rel)
      if (rel.startsWith('entities/')) {
        out.entities.push(def as EntityDef)
        continue
      }
      const tpl = def as TemplateDef
      const own = shared[tpl.id]
      const tplOut: SchemaTemplateDTO = {
        id: templateIds[tpl.id] ?? `tpl-${tpl.id}`,
        key: tpl.id,
        entity: tpl.entity,
        pack: manifest.id,
        pack_version: manifest.version,
        default: tpl.id === tpl.entity,
        definition: own ? { ...tpl, custom_fields: own.custom_fields } : tpl,
        ...(own ? { rev: own.rev } : {}),
      }
      out.templates.push(tplOut)
    }
  }
  return out
}
