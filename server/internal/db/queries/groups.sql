-- name: ListGroups :many
SELECT * FROM grp WHERE dashboard_id = $1 ORDER BY sort_key, id;

-- name: GetGroup :one
SELECT * FROM grp WHERE dashboard_id = $1 AND id = $2;

-- name: InsertGroup :one
INSERT INTO grp (id, dashboard_id, name, sort_key, auto_surname, surnames) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: UpdateGroup :one
UPDATE grp SET name = $2, sort_key = $3, auto_surname = $4, surnames = $5 WHERE id = $1 RETURNING *;

-- name: DeleteGroup :exec
DELETE FROM grp WHERE id = $1;

-- Вложенность групп дашборда (F-105): child входит в parent.
-- name: ListGroupLinks :many
SELECT parent_id, child_id FROM grp_link WHERE dashboard_id = $1 ORDER BY parent_id, child_id;

-- name: InsertGroupLink :exec
INSERT INTO grp_link (dashboard_id, parent_id, child_id) VALUES ($1, $2, $3);

-- name: DeleteGroupLink :exec
DELETE FROM grp_link WHERE parent_id = $1 AND child_id = $2;

-- Правки вложенности групп дашборда — по одной: проверка циклов видит
-- согласованный граф.
-- name: LockGroupLinks :exec
SELECT pg_advisory_xact_lock(hashtextextended('grp_link:' || @dashboard_id::text, 0));

-- Группы roots и все вложенные в них по всем путям (F-105): права
-- наследуются вниз. UNION отбрасывает повторы — обход конечен.
-- name: GroupDescendants :many
WITH RECURSIVE sub (id) AS (
    SELECT g.id FROM grp g WHERE g.dashboard_id = @dashboard_id AND g.id = ANY(@roots::uuid[])
    UNION
    SELECT l.child_id FROM grp_link l JOIN sub s ON l.parent_id = s.id
)
SELECT id FROM sub ORDER BY id;

-- Группа оказалась внутри самой себя (цикл через неё).
-- name: GroupInCycle :one
WITH RECURSIVE sub (id) AS (
    SELECT l.child_id FROM grp_link l WHERE l.parent_id = @id::uuid
    UNION
    SELECT l.child_id FROM grp_link l JOIN sub s ON l.parent_id = s.id
)
SELECT EXISTS (SELECT 1 FROM sub WHERE sub.id = @id::uuid);

-- name: ListLabelsByGroup :many
SELECT entity_id FROM entity_group WHERE group_id = $1 ORDER BY entity_id;

-- name: InsertEntityGroup :exec
INSERT INTO entity_group (dashboard_id, entity_id, group_id, created_by) VALUES ($1, $2, $3, $4);

-- Последние уникальные группы, назначенные пользователем (F-81).
-- name: RecentGroupsByAccount :many
SELECT eg.group_id
FROM entity_group eg
WHERE eg.dashboard_id = @dashboard_id AND eg.created_by = @account_id::uuid
GROUP BY eg.group_id
ORDER BY max(eg.created_at) DESC, eg.group_id
LIMIT @lim;

-- name: DeleteEntityGroup :exec
DELETE FROM entity_group WHERE entity_id = $1 AND group_id = $2;

-- Число видимых записей с меткой группы (F-07: только видимое).
-- name: CountGroupLabels :many
SELECT eg.group_id, count(*)::bigint AS n
FROM entity_group eg
JOIN entity e ON e.id = eg.entity_id
JOIN dashboard_template t ON t.dashboard_id = e.dashboard_id AND t.id = e.template_id
WHERE eg.dashboard_id = $1 AND e.deleted_at IS NULL AND t.active
GROUP BY eg.group_id;

-- Список записей дашборда: фильтры по сущности, группе (с вложенными по всем
-- путям, F-105) и тексту
-- (значения всех полей, включая историю, примечание и ручная подпись);
-- корзина — отдельно. Видимые = на действующем шаблоне (F-60). Одна строка:
-- всего найдено и id страницы (total есть и у пустой страницы).
-- name: ListRecords :one
WITH RECURSIVE sub (id) AS (
    SELECT g.id FROM grp g WHERE g.dashboard_id = @dashboard_id AND g.id = sqlc.narg(group_id)::uuid
    UNION
    SELECT l.child_id FROM grp_link l JOIN sub s ON l.parent_id = s.id
), matched AS (
    SELECT e.id
    FROM entity e
    JOIN dashboard_template t ON t.dashboard_id = e.dashboard_id AND t.id = e.template_id
    WHERE e.dashboard_id = @dashboard_id
      AND t.active
      AND (e.deleted_at IS NOT NULL) = @deleted::boolean
      AND (sqlc.narg(entity)::text IS NULL OR e.entity = sqlc.narg(entity)::text)
      AND (sqlc.narg(group_id)::uuid IS NULL
           OR EXISTS (SELECT 1 FROM entity_group eg WHERE eg.entity_id = e.id AND eg.group_id IN (SELECT id FROM sub)))
      AND (sqlc.narg(pattern)::text IS NULL
           OR e.note ILIKE sqlc.narg(pattern)::text
           OR e.title_manual ILIKE sqlc.narg(pattern)::text
           OR EXISTS (SELECT 1 FROM field_entry f WHERE f.entity_id = e.id AND f.v_text ILIKE sqlc.narg(pattern)::text))
)
SELECT (SELECT count(*) FROM matched)::bigint AS total,
       array(SELECT m.id FROM matched m ORDER BY m.id LIMIT @lim OFFSET @off)::uuid[] AS ids;

-- name: ListLiveEntitiesByTemplates :many
-- записи не в корзине на шаблонах дашборда (ракурс «Дерево»: отношения с tree_link)
SELECT * FROM entity
WHERE dashboard_id = @dashboard_id AND deleted_at IS NULL AND template_id = ANY (@template_ids::uuid[])
ORDER BY id;

-- name: FilterEntitiesByGroup :many
-- из ids — записи группы или её подгрупп
WITH RECURSIVE sub (id) AS (
    SELECT g.id FROM grp g WHERE g.dashboard_id = @dashboard_id AND g.id = @group_id
    UNION
    SELECT l.child_id FROM grp_link l JOIN sub s ON l.parent_id = s.id
)
SELECT DISTINCT eg.entity_id FROM entity_group eg
WHERE eg.entity_id = ANY (@ids::uuid[]) AND eg.group_id IN (SELECT id FROM sub);

-- Группы с автодобавлением по фамилии (F-122).
-- name: ListAutoSurnameGroups :many
SELECT * FROM grp WHERE dashboard_id = $1 AND auto_surname AND cardinality(surnames) > 0 ORDER BY sort_key, id;

-- Значения полей-фамилий живых записей указанных шаблонов (все значения истории);
-- сравнение без регистра — в коде: lower() в БД зависит от локали.
-- name: ListSurnameValues :many
SELECT f.entity_id, f.v_text
FROM field_entry f
JOIN entity e ON e.id = f.entity_id
WHERE e.dashboard_id = @dashboard_id AND e.deleted_at IS NULL
  AND e.template_id = @template_id::uuid AND f.field_key = ANY (@keys::text[]) AND f.v_text IS NOT NULL;
