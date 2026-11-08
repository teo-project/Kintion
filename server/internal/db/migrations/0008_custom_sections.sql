-- Свои блоки карточки (F-139): блоки записи с названием — в записи; общие
-- блоки шаблона в дашборде — в overrides.custom_sections шаблона дашборда.

-- +goose Up
ALTER TABLE entity ADD COLUMN custom_sections jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE entity DROP COLUMN custom_sections;
