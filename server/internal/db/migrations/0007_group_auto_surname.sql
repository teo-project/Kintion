-- Автодобавление в группу по фамилии (F-122): галочка и список фамилий.

-- +goose Up
ALTER TABLE grp ADD COLUMN auto_surname boolean NOT NULL DEFAULT false;
ALTER TABLE grp ADD COLUMN surnames text[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE grp DROP COLUMN surnames;
ALTER TABLE grp DROP COLUMN auto_surname;
