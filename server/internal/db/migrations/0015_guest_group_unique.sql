-- Техническая группа гостя (F-149) создаётся при принятии приглашения, одна на
-- гостя в дашборде; право правки на неё — право создавать записи.
--
-- Гостям, принявшим приглашение до этой версии и ещё не создававшим записей,
-- группа появится при следующей выдаче им права владельцем.

-- +goose Up
DROP INDEX grp_created_for_idx;
CREATE UNIQUE INDEX grp_created_for_idx ON grp (dashboard_id, created_for) WHERE created_for IS NOT NULL;

-- +goose Down
DROP INDEX grp_created_for_idx;
CREATE INDEX grp_created_for_idx ON grp (dashboard_id, created_for) WHERE created_for IS NOT NULL;
