-- name: InsertDashboard :one
INSERT INTO dashboard (id, owner_id, name) VALUES ($1, $2, $3) RETURNING *;

-- name: GetDashboard :one
SELECT * FROM dashboard WHERE id = $1 AND deleted_at IS NULL;

-- name: ListDashboardsByOwner :many
SELECT * FROM dashboard WHERE owner_id = $1 AND deleted_at IS NULL ORDER BY created_at, id;

-- name: ListLiveDashboardIDs :many
SELECT id FROM dashboard WHERE deleted_at IS NULL ORDER BY id;

-- name: RenameDashboard :one
UPDATE dashboard SET name = $2 WHERE id = $1 RETURNING *;

-- name: InsertDashboardTemplate :one
INSERT INTO dashboard_template (id, dashboard_id, pack_id, template_key, entity, pack_version, overrides, active)
VALUES ($1, $2, $3, $4, $5, $6, '{}', true)
RETURNING *;

-- name: ListDashboardTemplates :many
SELECT * FROM dashboard_template WHERE dashboard_id = $1 ORDER BY id;

-- Шаблоны, чей пак (или сам шаблон) не загружен, — неактивны (F-60).
-- name: SetTemplatesActive :exec
UPDATE dashboard_template
SET active = (pack_id || '/' || template_key) = ANY (@loaded::text[])
WHERE active IS DISTINCT FROM ((pack_id || '/' || template_key) = ANY (@loaded::text[]));

-- name: LockDashboardTemplate :one
SELECT * FROM dashboard_template WHERE dashboard_id = $1 AND id = $2 FOR UPDATE;

-- name: UpdateDashboardTemplateOverrides :one
UPDATE dashboard_template SET overrides = $3, rev = rev + 1
WHERE dashboard_id = $1 AND id = $2
RETURNING *;

-- Описание общих своих полей для записи значения: разделяемая блокировка до
-- конца транзакции — правка описания (FOR UPDATE) ждёт (F-96).
-- name: ShareLockDashboardTemplate :one
SELECT * FROM dashboard_template WHERE dashboard_id = $1 AND id = $2 FOR SHARE;

-- name: SetDashboardEras :one
UPDATE dashboard SET eras = $2 WHERE id = $1 RETURNING *;

-- name: GetDashboardEras :one
SELECT eras FROM dashboard WHERE id = $1;

-- Эры для правки: строка дашборда заблокирована до конца транзакции (F-45).
-- name: LockDashboardEras :one
SELECT eras FROM dashboard WHERE id = $1 FOR UPDATE;

-- Эры, встречающиеся в датах дашборда (F-143): пустая — наша эра у датированных;
-- даты из одной эры — тоже. Только даты видимых записей — не в корзине, на
-- действующих шаблонах (F-07, F-60).
-- name: ListUsedEras :many
WITH vis AS (SELECT en.id FROM entity en JOIN dashboard_template dt ON dt.id = en.template_id
             WHERE en.dashboard_id = @dashboard_id AND dt.active AND en.deleted_at IS NULL)
SELECT DISTINCT coalesce(x.e, '')::text AS era FROM (
    SELECT (start_date).e AS e, (start_date).y AS y FROM entity WHERE entity.id IN (SELECT id FROM vis)
    UNION ALL SELECT (end_date).e, (end_date).y FROM entity WHERE entity.id IN (SELECT id FROM vis)
    UNION ALL SELECT (valid_from).e, (valid_from).y FROM participation
        WHERE participation.container_id IN (SELECT id FROM vis) AND participation.entity_id IN (SELECT id FROM vis)
    UNION ALL SELECT (valid_to).e, (valid_to).y FROM participation
        WHERE participation.container_id IN (SELECT id FROM vis) AND participation.entity_id IN (SELECT id FROM vis)
    UNION ALL SELECT (v_date).e, (v_date).y FROM field_entry WHERE field_entry.entity_id IN (SELECT id FROM vis)
    UNION ALL SELECT (valid_from).e, (valid_from).y FROM field_entry WHERE field_entry.entity_id IN (SELECT id FROM vis)
    UNION ALL SELECT (valid_to).e, (valid_to).y FROM field_entry WHERE field_entry.entity_id IN (SELECT id FROM vis)
) x WHERE x.y IS NOT NULL OR x.e IS NOT NULL
ORDER BY 1;

-- name: TouchEntities :exec
UPDATE entity SET updated_at = now() WHERE id = ANY (@ids::uuid[]);

-- Замена эры во всех датах дашборда (F-143): даты с годом и эрой @old_era
-- (NULL — пустая) и даты из одной эры @old_era получают эру @new_era (NULL —
-- пустая); прочие не меняются. Дата из одной эры при замене на пустую
-- очищается (колонка NULL). Правила — pd_era_matches и pd_with_era (миграция
-- 0012). Одна строка — один UPDATE на таблицу (все её колонки-даты сразу).
-- Меняются все даты (и записей в корзине и неактивных шаблонов — вернутся
-- согласованными); число изменённых — по видимым записям (F-07), по снимку до
-- изменения.
-- Версии полей (F-45) повышаются в том же запросе — ровно у тех, что замена
-- действительно изменила (RETURNING old.*): правка, сохранённая параллельно,
-- либо видна замене и получает новую версию, либо не тронута. Ключи — как у
-- правки поля: start, end, participants:<роль> у контейнера, field_key у
-- значения. Версия роли — только по видимым участникам: скрытого карточка не
-- показывает, и 409 из-за его дат был бы запретом по невидимым данным (F-07).
-- touched — записи с новыми версиями: им updated_at отдельным запросом.
-- name: ReplaceEra :one
WITH
  vis AS (SELECT en.id FROM entity en JOIN dashboard_template dt ON dt.id = en.template_id
             WHERE en.dashboard_id = @dashboard_id AND dt.active AND en.deleted_at IS NULL),
  before AS (SELECT ((SELECT count(*) FROM entity WHERE entity.id IN (SELECT id FROM vis) AND pd_era_matches(start_date, sqlc.narg(old_era)::text))
      + (SELECT count(*) FROM entity WHERE entity.id IN (SELECT id FROM vis) AND pd_era_matches(end_date, sqlc.narg(old_era)::text))
      + (SELECT count(*) FROM participation WHERE participation.container_id IN (SELECT id FROM vis) AND participation.entity_id IN (SELECT id FROM vis) AND pd_era_matches(valid_from, sqlc.narg(old_era)::text))
      + (SELECT count(*) FROM participation WHERE participation.container_id IN (SELECT id FROM vis) AND participation.entity_id IN (SELECT id FROM vis) AND pd_era_matches(valid_to, sqlc.narg(old_era)::text))
      + (SELECT count(*) FROM field_entry WHERE field_entry.entity_id IN (SELECT id FROM vis) AND pd_era_matches(v_date, sqlc.narg(old_era)::text))
      + (SELECT count(*) FROM field_entry WHERE field_entry.entity_id IN (SELECT id FROM vis) AND pd_era_matches(valid_from, sqlc.narg(old_era)::text))
      + (SELECT count(*) FROM field_entry WHERE field_entry.entity_id IN (SELECT id FROM vis) AND pd_era_matches(valid_to, sqlc.narg(old_era)::text)))::bigint AS n),
  e AS (UPDATE entity SET start_date = CASE WHEN pd_era_matches(start_date, sqlc.narg(old_era)::text) THEN pd_with_era(start_date, sqlc.narg(new_era)::text) ELSE start_date END,
            end_date = CASE WHEN pd_era_matches(end_date, sqlc.narg(old_era)::text) THEN pd_with_era(end_date, sqlc.narg(new_era)::text) ELSE end_date END
         WHERE entity.dashboard_id = @dashboard_id AND (pd_era_matches(start_date, sqlc.narg(old_era)::text) OR pd_era_matches(end_date, sqlc.narg(old_era)::text))
         RETURNING entity.id, pd_era_matches(old.start_date, sqlc.narg(old_era)::text) AS s, pd_era_matches(old.end_date, sqlc.narg(old_era)::text) AS en),
  p AS (UPDATE participation SET valid_from = CASE WHEN pd_era_matches(valid_from, sqlc.narg(old_era)::text) THEN pd_with_era(valid_from, sqlc.narg(new_era)::text) ELSE valid_from END,
            valid_to = CASE WHEN pd_era_matches(valid_to, sqlc.narg(old_era)::text) THEN pd_with_era(valid_to, sqlc.narg(new_era)::text) ELSE valid_to END
         WHERE participation.dashboard_id = @dashboard_id AND (pd_era_matches(valid_from, sqlc.narg(old_era)::text) OR pd_era_matches(valid_to, sqlc.narg(old_era)::text))
         RETURNING participation.container_id, participation.entity_id, participation.role),
  f AS (UPDATE field_entry SET v_date = CASE WHEN pd_era_matches(v_date, sqlc.narg(old_era)::text) THEN pd_with_era(v_date, sqlc.narg(new_era)::text) ELSE v_date END,
            valid_from = CASE WHEN pd_era_matches(valid_from, sqlc.narg(old_era)::text) THEN pd_with_era(valid_from, sqlc.narg(new_era)::text) ELSE valid_from END,
            valid_to = CASE WHEN pd_era_matches(valid_to, sqlc.narg(old_era)::text) THEN pd_with_era(valid_to, sqlc.narg(new_era)::text) ELSE valid_to END
         WHERE field_entry.dashboard_id = @dashboard_id AND (pd_era_matches(v_date, sqlc.narg(old_era)::text) OR pd_era_matches(valid_from, sqlc.narg(old_era)::text) OR pd_era_matches(valid_to, sqlc.narg(old_era)::text))
         RETURNING field_entry.entity_id, field_entry.field_key),
  k AS (SELECT id AS entity_id, 'start'::text AS field_key FROM e WHERE s
        UNION SELECT id, 'end' FROM e WHERE en
        UNION SELECT container_id, 'participants:' || role FROM p WHERE p.entity_id IN (SELECT id FROM vis)
        UNION SELECT entity_id, field_key FROM f),
  b AS (INSERT INTO field_rev (entity_id, field_key, rev) SELECT entity_id, field_key, 1 FROM k
        ON CONFLICT (entity_id, field_key) DO UPDATE SET rev = field_rev.rev + 1 RETURNING field_rev.entity_id)
SELECT n AS changed, ARRAY(SELECT DISTINCT entity_id FROM b)::uuid[] AS touched FROM before;
