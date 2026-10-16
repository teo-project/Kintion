-- name: InsertAuditLog :exec
INSERT INTO audit_log (id, dashboard_id, actor_id, operation_id, source, request_id, app_version,
                       schema_version, entity_id, object, object_id, op, before, after)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: ListAuditByOperation :many
SELECT * FROM audit_log WHERE operation_id = $1 ORDER BY at, id;

-- name: ListAuditByDashboard :many
SELECT * FROM audit_log WHERE dashboard_id = $1 ORDER BY at, id;

-- name: ListAuditOperations :many
-- Страница операций журнала дашборда, новые сверху; курсор — (время, операция)
-- последней операции предыдущей страницы. Фильтр по записи включает и участия
-- записи в чужих контейнерах (entity_id участника лежит в снимке).
WITH hits AS (
    SELECT a.operation_id, max(a.at)::timestamptz AS last_at
    FROM audit_log a
    WHERE a.dashboard_id = sqlc.arg(dashboard_id)::uuid
      AND (sqlc.narg(entity)::uuid IS NULL OR a.entity_id = sqlc.narg(entity)::uuid
           OR (a.object = 'participation' AND (a.before ->> 'entity_id' = sqlc.narg(entity)::uuid::text
                                               OR a.after ->> 'entity_id' = sqlc.narg(entity)::uuid::text)))
      AND (sqlc.narg(operation)::uuid IS NULL OR a.operation_id = sqlc.narg(operation)::uuid)
      AND (sqlc.narg(actor)::uuid IS NULL OR a.actor_id = sqlc.narg(actor)::uuid)
      AND (NOT sqlc.arg(system_only)::bool OR a.actor_id IS NULL)
      AND (sqlc.narg(op)::text IS NULL OR a.op = sqlc.narg(op)::text)
    GROUP BY a.operation_id
)
SELECT operation_id, last_at FROM hits
WHERE sqlc.narg(before_at)::timestamptz IS NULL
   OR (last_at, operation_id) < (sqlc.narg(before_at)::timestamptz, sqlc.arg(before_op)::uuid)
ORDER BY last_at DESC, operation_id DESC
LIMIT sqlc.arg(lim)::int;

-- name: ListAuditEntries :many
-- Записи журнала выбранных операций с теми же фильтрами записи, автора и действия.
SELECT a.*, acc.login AS actor_login
FROM audit_log a
LEFT JOIN account acc ON acc.id = a.actor_id
WHERE a.dashboard_id = sqlc.arg(dashboard_id)::uuid
  AND a.operation_id = ANY (sqlc.arg(operations)::uuid[])
  AND (sqlc.narg(entity)::uuid IS NULL OR a.entity_id = sqlc.narg(entity)::uuid
       OR (a.object = 'participation' AND (a.before ->> 'entity_id' = sqlc.narg(entity)::uuid::text
                                           OR a.after ->> 'entity_id' = sqlc.narg(entity)::uuid::text)))
  AND (sqlc.narg(actor)::uuid IS NULL OR a.actor_id = sqlc.narg(actor)::uuid)
  AND (NOT sqlc.arg(system_only)::bool OR a.actor_id IS NULL)
  AND (sqlc.narg(op)::text IS NULL OR a.op = sqlc.narg(op)::text)
ORDER BY a.at, a.id;

-- Снимки записей и их значений на момент очистки (purge пишет последнее
-- состояние каждой строки) — последнее название окончательно удалённой записи
-- (F-94): журнал остаётся после очистки.
-- name: ListAuditSnapshots :many
SELECT * FROM audit_log
WHERE dashboard_id = $1 AND entity_id = ANY (@ids::uuid[]) AND object IN ('entity', 'field_entry') AND op = 'purge'
ORDER BY at, id;
