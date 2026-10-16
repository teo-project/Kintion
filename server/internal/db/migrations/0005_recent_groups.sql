-- Автор и время метки группы: последние группы пользователя (F-81).

-- +goose Up
ALTER TABLE entity_group
    ADD COLUMN created_by uuid REFERENCES account (id) ON DELETE SET NULL,
    ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX entity_group_recent_idx ON entity_group (dashboard_id, created_by, created_at DESC);

-- +goose Down
DROP INDEX entity_group_recent_idx;
ALTER TABLE entity_group DROP COLUMN created_at, DROP COLUMN created_by;
