-- name: GetSchemaInfo :one
SELECT version, app_version, migrated_at
FROM schema_info
WHERE singleton;

-- name: UpsertSchemaInfo :exec
INSERT INTO schema_info (singleton, version, app_version, migrated_at)
VALUES (true, $1, $2, now())
ON CONFLICT (singleton) DO UPDATE
SET version     = EXCLUDED.version,
    app_version = EXCLUDED.app_version,
    migrated_at = EXCLUDED.migrated_at;
