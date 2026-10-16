-- name: InsertFile :one
-- Тот же файл в том же дашборде — та же строка: вставка пропускается,
-- ничего не возвращается (pgx.ErrNoRows).
INSERT INTO file (id, dashboard_id, sha256, mime, size, width, height, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (dashboard_id, sha256) DO NOTHING
RETURNING *;

-- name: GetFile :one
SELECT * FROM file WHERE dashboard_id = $1 AND sha256 = $2;
