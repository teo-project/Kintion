-- Эры — настройки дашборда (F-143): показывать и менять эру, эра по умолчанию,
-- хронологический порядок эр, текущий год мира. JSON вида
-- {"show":false,"edit":false,"default":"","order":[],"today":null};
-- пустой объект — всё по умолчанию.

-- +goose Up
ALTER TABLE dashboard ADD COLUMN eras jsonb NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE dashboard DROP COLUMN eras;
