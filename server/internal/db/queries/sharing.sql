-- Совместный доступ (0.5, KIN-6): ники, гости дашбордов, права на группы.

-- name: GetAccountByNick :one
SELECT * FROM account WHERE nick IS NOT NULL AND lower(nick) = lower(@nick::text);

-- name: SetAccountNick :one
UPDATE account SET nick = sqlc.narg(nick)::text WHERE id = @id RETURNING *;

-- name: CreateAccountWithNick :one
INSERT INTO account (id, login, password_hash, nick, is_admin)
VALUES ($1, $2, $3, $4, false)
RETURNING *;

-- name: GetMember :one
SELECT * FROM dashboard_member WHERE dashboard_id = $1 AND account_id = $2;

-- name: LockMember :one
SELECT * FROM dashboard_member WHERE dashboard_id = $1 AND account_id = $2 FOR UPDATE;

-- name: InsertMember :execrows
INSERT INTO dashboard_member (dashboard_id, account_id, invited_by)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: AcceptMember :exec
UPDATE dashboard_member SET accepted_at = now()
WHERE dashboard_id = $1 AND account_id = $2 AND accepted_at IS NULL;

-- name: DeleteMember :exec
DELETE FROM dashboard_member WHERE dashboard_id = $1 AND account_id = $2;

-- name: ListMembers :many
-- гости дашборда с их правами (для владельца)
SELECT m.account_id, a.nick, a.login, m.created_at, m.accepted_at
FROM dashboard_member m JOIN account a ON a.id = m.account_id
WHERE m.dashboard_id = $1
ORDER BY lower(coalesce(a.nick, a.login)), m.account_id;

-- name: ListGrants :many
SELECT * FROM grp_grant WHERE dashboard_id = $1 ORDER BY account_id, group_id;

-- name: ListMemberGrants :many
SELECT * FROM grp_grant WHERE dashboard_id = $1 AND account_id = $2 ORDER BY group_id;

-- name: UpsertGrant :one
INSERT INTO grp_grant (dashboard_id, group_id, account_id, can_edit)
VALUES ($1, $2, $3, $4)
ON CONFLICT (group_id, account_id) DO UPDATE SET can_edit = EXCLUDED.can_edit
RETURNING *;

-- name: GetGrant :one
SELECT * FROM grp_grant WHERE dashboard_id = $1 AND group_id = $2 AND account_id = $3;

-- name: DeleteGrant :exec
DELETE FROM grp_grant WHERE dashboard_id = $1 AND group_id = $2 AND account_id = $3;

-- name: ListSharedDashboards :many
-- дашборды, где аккаунт — гость (принятые и ждущие подтверждения)
SELECT d.*, m.accepted_at, m.created_at AS invited_at, o.nick AS owner_nick
FROM dashboard_member m
JOIN dashboard d ON d.id = m.dashboard_id
JOIN account o ON o.id = d.owner_id
WHERE m.account_id = $1 AND d.deleted_at IS NULL
ORDER BY d.created_at, d.id;

-- name: GuestGroups :many
-- группы, видимые гостю: группы с выдачей и все вложенные в них по всем путям
-- (F-105); can_edit — есть ли путь от выдачи с правом редактирования; own —
-- техническая группа созданного этим гостем (ему не показывается).
WITH RECURSIVE sub (id, can_edit) AS (
    SELECT g.group_id, g.can_edit FROM grp_grant g
    WHERE g.dashboard_id = @dashboard_id AND g.account_id = @account_id
    UNION
    SELECT l.child_id, s.can_edit FROM grp_link l JOIN sub s ON l.parent_id = s.id
)
SELECT s.id, bool_or(s.can_edit)::boolean AS can_edit,
       coalesce(bool_or(g.created_for = @account_id::uuid), false)::boolean AS own
FROM sub s JOIN grp g ON g.id = s.id
GROUP BY s.id;

-- name: EntitiesInGroups :many
-- записи дашборда с меткой любой из групп
SELECT DISTINCT eg.entity_id FROM entity_group eg
WHERE eg.dashboard_id = @dashboard_id AND eg.group_id = ANY(@group_ids::uuid[]);

-- name: ListLiveGroupLabels :many
-- метки записей не в корзине на действующих шаблонах (счётчики групп гостя)
SELECT eg.entity_id, eg.group_id
FROM entity_group eg
JOIN entity e ON e.id = eg.entity_id
JOIN dashboard_template t ON t.dashboard_id = e.dashboard_id AND t.id = e.template_id
WHERE eg.dashboard_id = $1 AND e.deleted_at IS NULL AND t.active;

-- name: FileUsedBy :many
-- значения полей с текстом sha (кандидаты: фото, поле «Файл»; тип поля
-- проверяет вызывающий по шаблону)
SELECT DISTINCT f.entity_id, f.field_key FROM field_entry f
WHERE f.dashboard_id = $1 AND f.v_text = $2;

-- name: InsertInvite :one
INSERT INTO instance_invite (id, token_hash, created_by, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: LockInviteByToken :one
SELECT * FROM instance_invite WHERE token_hash = $1 FOR UPDATE;

-- name: UseInvite :exec
UPDATE instance_invite SET used_at = now(), used_by = $2 WHERE id = $1;

-- name: ListInvites :many
-- действующие приглашения (не использованы, не истекли); by — только свои
SELECT * FROM instance_invite
WHERE used_at IS NULL AND expires_at > now()
  AND (sqlc.narg(by)::uuid IS NULL OR created_by = sqlc.narg(by)::uuid)
ORDER BY created_at DESC;

-- name: DeleteInvite :exec
-- by — удалить можно только своё (не админ)
DELETE FROM instance_invite
WHERE id = @id AND used_at IS NULL
  AND (sqlc.narg(by)::uuid IS NULL OR created_by = sqlc.narg(by)::uuid);

-- name: ListAccounts :many
SELECT id, login, nick, is_admin, created_at FROM account ORDER BY created_at, id;

-- name: ListGroupGrants :many
-- права на группу с ником гостя (журнал при удалении группы)
SELECT g.account_id, g.can_edit, a.nick
FROM grp_grant g JOIN account a ON a.id = g.account_id
WHERE g.group_id = $1
ORDER BY g.account_id;

-- name: LockGuestGroup :one
-- техническая группа гостя в дашборде (одна, F-149)
SELECT * FROM grp WHERE dashboard_id = $1 AND created_for = $2 FOR UPDATE;

-- name: ListGuestGroupsOf :many
-- технические группы гостя во всех дашбордах (смена ника)
SELECT * FROM grp WHERE created_for = $1 ORDER BY dashboard_id FOR UPDATE;

-- name: SetGroupCreatedFor :exec
UPDATE grp SET created_for = $2 WHERE id = $1;

-- name: AccountUploadedFile :one
-- аккаунт загружал файл sha в дашборд (запись журнала о загрузке)
SELECT EXISTS (
    SELECT 1 FROM audit_log
    WHERE dashboard_id = sqlc.arg(dashboard_id)::uuid AND actor_id = sqlc.arg(actor_id)::uuid AND object = 'file' AND after ->> 'sha256' = sqlc.arg(sha256)::text
)::boolean;
