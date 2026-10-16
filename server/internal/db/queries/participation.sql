-- name: InsertParticipation :one
INSERT INTO participation (id, dashboard_id, container_id, entity_id, role, valid_from, valid_to, note, sort_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetParticipation :one
SELECT * FROM participation WHERE dashboard_id = $1 AND id = $2;

-- name: UpdateParticipation :one
UPDATE participation SET valid_from = $2, valid_to = $3, note = $4, sort_key = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteParticipation :exec
DELETE FROM participation WHERE id = $1;

-- name: ListParticipantsByContainerRole :many
SELECT * FROM participation WHERE container_id = $1 AND role = $2 ORDER BY sort_key, id;

-- Вложена ли запись target в root (root ⊃ … ⊃ target) — запрет циклов вложенности.
-- Учитываются все участия, в том числе записей в корзине: их можно восстановить.
-- name: ContainsEntity :one
WITH RECURSIVE inner_ids (id) AS (
    SELECT p.entity_id FROM participation p WHERE p.container_id = @root::uuid
    UNION
    SELECT p.entity_id FROM participation p JOIN inner_ids i ON p.container_id = i.id
)
SELECT EXISTS (SELECT 1 FROM inner_ids WHERE id = @target::uuid);
