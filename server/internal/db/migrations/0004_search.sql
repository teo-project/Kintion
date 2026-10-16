-- Поиск по записям (T13): ILIKE по текстовым значениям полей и примечаниям,
-- триграммный индекс pg_trgm (доверенное расширение — владелец БД ставит сам).

-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX field_entry_text_trgm_idx ON field_entry USING gin (v_text gin_trgm_ops) WHERE v_text IS NOT NULL;
CREATE INDEX entity_note_trgm_idx ON entity USING gin (note gin_trgm_ops) WHERE note IS NOT NULL;
CREATE INDEX grp_dashboard_idx ON grp (dashboard_id, sort_key);

-- +goose Down
DROP INDEX grp_dashboard_idx;
DROP INDEX entity_note_trgm_idx;
DROP INDEX field_entry_text_trgm_idx;
DROP EXTENSION IF EXISTS pg_trgm;
