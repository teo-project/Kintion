-- name: UpsertPack :exec
INSERT INTO pack (id, version, language, loaded, loaded_at)
VALUES ($1, $2, $3, true, now())
ON CONFLICT (id) DO UPDATE
SET version = EXCLUDED.version, language = EXCLUDED.language, loaded = true, loaded_at = now();

-- name: MarkPacksUnloaded :exec
UPDATE pack SET loaded = false WHERE loaded AND NOT (id = ANY (@loaded::text[]));

-- name: ListPacks :many
SELECT * FROM pack ORDER BY id;
