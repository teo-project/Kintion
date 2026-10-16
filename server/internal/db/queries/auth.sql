-- name: CountAccounts :one
SELECT count(*) FROM account;

-- name: CreateAccount :one
INSERT INTO account (id, login, password_hash, is_admin)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM account WHERE id = $1;

-- name: GetAccountByLogin :one
SELECT * FROM account WHERE login = $1;

-- name: GetAccountPrefs :one
SELECT prefs FROM account WHERE id = $1;

-- name: UpdateAccountPrefs :one
UPDATE account SET prefs = $2 WHERE id = $1 RETURNING prefs;

-- name: UpdatePasswordHash :exec
UPDATE account SET password_hash = $2 WHERE id = $1;

-- name: DeleteUnusedBootstrap :exec
DELETE FROM bootstrap WHERE used_at IS NULL;

-- name: InsertBootstrap :exec
INSERT INTO bootstrap (token_hash) VALUES ($1);

-- name: LockBootstrap :one
SELECT * FROM bootstrap WHERE token_hash = $1 FOR UPDATE;

-- name: MarkBootstrapUsed :exec
UPDATE bootstrap SET used_at = now() WHERE token_hash = $1;

-- name: CreateSession :one
INSERT INTO session (id, account_id, token_hash, expires_at, user_agent)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetSessionByTokenHash :one
SELECT sqlc.embed(session), sqlc.embed(account)
FROM session JOIN account ON account.id = session.account_id
WHERE session.token_hash = $1 AND session.expires_at > now();

-- name: TouchSession :exec
UPDATE session SET last_seen_at = now() WHERE id = $1;

-- name: DeleteSession :exec
DELETE FROM session WHERE id = $1;

-- name: DeleteOtherSessions :exec
DELETE FROM session WHERE account_id = $1 AND id <> $2;

-- name: DeleteAccountSessions :exec
DELETE FROM session WHERE account_id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM session WHERE expires_at <= now();
