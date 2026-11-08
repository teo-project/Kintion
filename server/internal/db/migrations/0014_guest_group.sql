-- Гость-редактор (0.5, KIN-6): техническая группа «Созданные <ник>».
--
-- Запись, созданная гостем, сразу получает метку его технической группы: на
-- неё у гостя право редактирования, поэтому своё созданное он может править,
-- даже если не выбрал другой группы. Самому гостю группа не показывается;
-- владелец видит её как обычную группу и может переложить записи.

-- +goose Up
ALTER TABLE grp ADD COLUMN created_for uuid REFERENCES account (id) ON DELETE SET NULL;
CREATE INDEX grp_created_for_idx ON grp (dashboard_id, created_for) WHERE created_for IS NOT NULL;

-- +goose Down
DROP INDEX grp_created_for_idx;
ALTER TABLE grp DROP COLUMN created_for;
