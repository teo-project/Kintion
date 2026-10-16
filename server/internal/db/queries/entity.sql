-- name: InsertEntity :one
INSERT INTO entity (id, dashboard_id, entity, template_id, start_date, start_link, end_date, end_link,
                    note, title_manual, sensitive, custom_fields, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: GetEntity :one
SELECT * FROM entity WHERE dashboard_id = $1 AND id = $2;

-- name: InsertFieldEntry :one
INSERT INTO field_entry (id, dashboard_id, entity_id, field_key, sort_key, v_text, v_num, v_date,
                         v_bool, v_ref, meta, valid_from, valid_to, note)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: ListFieldEntries :many
SELECT * FROM field_entry WHERE entity_id = $1 ORDER BY field_key, sort_key;

-- name: GetEntityByID :one
SELECT * FROM entity WHERE id = $1;

-- name: GetDashboardTemplate :one
SELECT * FROM dashboard_template WHERE dashboard_id = $1 AND id = $2;

-- Действующий шаблон дашборда по ключу шаблона пака (F-36, F-60).
-- name: FindActiveTemplate :one
SELECT * FROM dashboard_template
WHERE dashboard_id = $1 AND template_key = $2 AND active
ORDER BY id LIMIT 1;

-- name: ListEntitiesByIDs :many
SELECT * FROM entity WHERE dashboard_id = @dashboard_id AND id = ANY (@ids::uuid[]);

-- Системные поля записи: одна запись строки целиком (сервис меняет одно поле).
-- name: UpdateEntitySystem :one
UPDATE entity
SET start_date = $2, start_link = $3, end_date = $4, end_link = $5, note = $6,
    title_manual = $7, sensitive = $8, custom_fields = $9, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: TouchEntity :exec
UPDATE entity SET updated_at = now() WHERE id = $1;

-- Версия поля (F-45): строка блокируется до конца транзакции.
-- name: LockFieldRev :one
INSERT INTO field_rev (entity_id, field_key, rev) VALUES ($1, $2, 0)
ON CONFLICT (entity_id, field_key) DO UPDATE SET rev = field_rev.rev
RETURNING rev;

-- name: BumpFieldRev :one
INSERT INTO field_rev (entity_id, field_key, rev) VALUES ($1, $2, 1)
ON CONFLICT (entity_id, field_key) DO UPDATE SET rev = field_rev.rev + 1
RETURNING rev;

-- name: ListFieldRevsByEntities :many
SELECT * FROM field_rev WHERE entity_id = ANY (@ids::uuid[]);

-- name: ListFieldEntriesByEntities :many
SELECT * FROM field_entry WHERE entity_id = ANY (@ids::uuid[]) ORDER BY entity_id, field_key, sort_key, id;

-- name: ListEntryLinksByEntries :many
SELECT * FROM entry_link WHERE entry_id = ANY (@ids::uuid[]) ORDER BY entry_id, id;

-- name: ListParticipantsByContainers :many
SELECT * FROM participation WHERE container_id = ANY (@ids::uuid[]) ORDER BY container_id, role, sort_key, id;

-- name: ListParticipationsByEntities :many
SELECT * FROM participation WHERE entity_id = ANY (@ids::uuid[]) ORDER BY entity_id, created_at, id;

-- name: ListGroupsByEntities :many
SELECT eg.entity_id, eg.group_id FROM entity_group eg JOIN grp g ON g.id = eg.group_id
WHERE eg.entity_id = ANY (@ids::uuid[]) ORDER BY eg.entity_id, g.sort_key, g.id;

-- Значения полей для сортировки списка (F-133): верхнее значение — первое по sort_key.
-- name: ListSortValues :many
SELECT entity_id, field_key, v_text FROM field_entry
WHERE entity_id = ANY (@ids::uuid[]) AND field_key = ANY (@keys::text[]) AND v_text IS NOT NULL
ORDER BY entity_id, field_key, sort_key, id;
