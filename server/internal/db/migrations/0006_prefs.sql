-- Настройки пользователя на сервере: запомненные выборы интерфейса (F-80).

-- +goose Up
ALTER TABLE account ADD COLUMN prefs jsonb NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE account DROP COLUMN prefs;
