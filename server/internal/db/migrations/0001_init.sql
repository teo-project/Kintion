-- Первая миграция: только версия схемы. Данные — в следующих (T06).

-- +goose Up
CREATE TABLE schema_info (
    singleton   boolean     PRIMARY KEY DEFAULT true CHECK (singleton),
    version     bigint      NOT NULL,
    app_version text        NOT NULL,
    migrated_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE schema_info IS 'Версия схемы и приложения, применившего последнюю миграцию (одна строка)';

-- +goose Down
DROP TABLE schema_info;
