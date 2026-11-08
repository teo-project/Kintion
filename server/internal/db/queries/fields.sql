-- name: GetFieldEntry :one
SELECT * FROM field_entry WHERE dashboard_id = $1 AND id = $2;

-- name: ListFieldEntriesByKey :many
SELECT * FROM field_entry WHERE entity_id = $1 AND field_key = $2 ORDER BY sort_key, id;

-- name: ListFieldKeysByEntity :many
SELECT DISTINCT field_key FROM field_entry WHERE entity_id = $1 ORDER BY field_key;

-- name: UpdateFieldEntry :one
UPDATE field_entry
SET sort_key = $2, v_text = $3, v_num = $4, v_date = $5, v_bool = $6, v_ref = $7, meta = $8,
    valid_from = $9, valid_to = $10, note = $11, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteFieldEntry :exec
DELETE FROM field_entry WHERE id = $1;

-- name: InsertEntryLink :one
INSERT INTO entry_link (id, dashboard_id, entry_id, target_id, anchor) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DeleteEntryLinksByEntry :exec
DELETE FROM entry_link WHERE entry_id = $1;

-- Ключи полей с значениями у записей шаблона дашборда (и в корзине) с префиксом.
-- name: ListFieldKeysByTemplate :many
SELECT DISTINCT fe.field_key
FROM field_entry fe
JOIN entity e ON e.id = fe.entity_id
WHERE e.dashboard_id = $1 AND e.template_id = $2 AND starts_with(fe.field_key, @prefix::text)
ORDER BY fe.field_key;

-- Блоки, в которые записи шаблона в дашборде (в том числе в корзине) ставят
-- свои поля (F-139): общий блок с полями не удаляется.
-- name: ListCustomSectionsByTemplate :many
SELECT DISTINCT (f->>'section')::text AS section
FROM entity e, jsonb_array_elements(e.custom_fields) f
WHERE e.dashboard_id = $1 AND e.template_id = $2 AND coalesce(f->>'section', '') <> ''
ORDER BY 1;

-- Ключи своих блоков записей шаблона в дашборде (в том числе в корзине):
-- новый общий блок не занимает ключ блока записи (F-139).
-- name: ListRecordSectionKeysByTemplate :many
SELECT DISTINCT (s->>'key')::text AS key
FROM entity e, jsonb_array_elements(e.custom_sections) s
WHERE e.dashboard_id = $1 AND e.template_id = $2
ORDER BY 1;

-- Значения-строки поля записи (используемые варианты выбора) и пометка
-- своего значения (F-104).
-- name: ListFieldTextsByEntity :many
SELECT DISTINCT v_text::text AS text, (meta @> '{"free": true}')::boolean AS free FROM field_entry
WHERE entity_id = $1 AND field_key = $2 AND v_text IS NOT NULL
ORDER BY 1;

-- Значения-строки поля у записей шаблона дашборда (и в корзине).
-- name: ListFieldTextsByTemplate :many
SELECT DISTINCT fe.v_text::text AS text, (fe.meta @> '{"free": true}')::boolean AS free
FROM field_entry fe
JOIN entity e ON e.id = fe.entity_id
WHERE e.dashboard_id = $1 AND e.template_id = $2 AND fe.field_key = $3 AND fe.v_text IS NOT NULL
ORDER BY 1;
