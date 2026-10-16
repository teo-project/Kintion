-- name: SetEntityDeleted :one
UPDATE entity SET deleted_at = now(), deleted_by = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: RestoreEntity :one
UPDATE entity SET deleted_at = NULL, deleted_by = NULL, updated_at = now() WHERE id = $1 RETURNING *;

-- name: DeleteEntity :exec
DELETE FROM entity WHERE id = $1;

-- Значения ref других записей, указывающие на запись (Д-32).
-- name: ListRefEntriesTo :many
SELECT * FROM field_entry WHERE v_ref = @target::uuid AND entity_id <> @target::uuid ORDER BY entity_id, field_key, sort_key, id;

-- Привязки значений других записей к записи (кроме значений ref на неё — они удаляются целиком).
-- name: ListLinksTo :many
SELECT l.id, l.entry_id, l.target_id, l.anchor, f.entity_id, f.field_key
FROM entry_link l JOIN field_entry f ON f.id = l.entry_id
WHERE l.target_id = @target::uuid AND f.entity_id <> @target::uuid AND f.v_ref IS DISTINCT FROM @target::uuid
ORDER BY l.id;

-- name: DeleteEntryLink :exec
DELETE FROM entry_link WHERE id = $1;

-- name: ListEntitiesLinkingTo :many
SELECT * FROM entity WHERE (start_link = @target::uuid OR end_link = @target::uuid) AND id <> @target::uuid ORDER BY id;

-- name: ListParticipationsOf :many
SELECT * FROM participation WHERE container_id = $1 OR entity_id = $1 ORDER BY id;
