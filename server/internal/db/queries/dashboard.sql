-- name: InsertDashboard :one
INSERT INTO dashboard (id, owner_id, name) VALUES ($1, $2, $3) RETURNING *;

-- name: GetDashboard :one
SELECT * FROM dashboard WHERE id = $1 AND deleted_at IS NULL;

-- name: ListDashboardsByOwner :many
SELECT * FROM dashboard WHERE owner_id = $1 AND deleted_at IS NULL ORDER BY created_at, id;

-- name: ListLiveDashboardIDs :many
SELECT id FROM dashboard WHERE deleted_at IS NULL ORDER BY id;

-- name: RenameDashboard :one
UPDATE dashboard SET name = $2 WHERE id = $1 RETURNING *;

-- name: InsertDashboardTemplate :one
INSERT INTO dashboard_template (id, dashboard_id, pack_id, template_key, entity, pack_version, overrides, active)
VALUES ($1, $2, $3, $4, $5, $6, '{}', true)
RETURNING *;

-- name: ListDashboardTemplates :many
SELECT * FROM dashboard_template WHERE dashboard_id = $1 ORDER BY id;

-- Шаблоны, чей пак (или сам шаблон) не загружен, — неактивны (F-60).
-- name: SetTemplatesActive :exec
UPDATE dashboard_template
SET active = (pack_id || '/' || template_key) = ANY (@loaded::text[])
WHERE active IS DISTINCT FROM ((pack_id || '/' || template_key) = ANY (@loaded::text[]));

-- name: LockDashboardTemplate :one
SELECT * FROM dashboard_template WHERE dashboard_id = $1 AND id = $2 FOR UPDATE;

-- name: UpdateDashboardTemplateOverrides :one
UPDATE dashboard_template SET overrides = $3, rev = rev + 1
WHERE dashboard_id = $1 AND id = $2
RETURNING *;

-- Описание общих своих полей для записи значения: разделяемая блокировка до
-- конца транзакции — правка описания (FOR UPDATE) ждёт (F-96).
-- name: ShareLockDashboardTemplate :one
SELECT * FROM dashboard_template WHERE dashboard_id = $1 AND id = $2 FOR SHARE;
