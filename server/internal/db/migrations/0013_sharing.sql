-- Совместный доступ (0.5, KIN-6; F-03): гости дашборда и права на группы.
--
-- Ник — адрес для приглашений (F-03): уникален без учёта регистра (техническая
-- уникальность адреса, не доменных данных — F-07).
-- dashboard_member — гость дашборда: первое приглашение ждёт подтверждения
-- (accepted_at IS NULL), дальнейшие выдачи в тот же дашборд действуют сразу.
-- grp_grant — право гостя на группу: чтение или редактирование; наследуется
-- вниз по вложенности групп по всем путям (F-105).
-- instance_invite — приглашение на инстанс (F-28, режим invite): одноразовая
-- ссылка от админа; в базе — только хеш токена.

-- +goose Up
CREATE UNIQUE INDEX account_nick_idx ON account (lower(nick)) WHERE nick IS NOT NULL;

CREATE TABLE dashboard_member (
    dashboard_id uuid        NOT NULL REFERENCES dashboard (id) ON DELETE CASCADE,
    account_id   uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    invited_by   uuid        REFERENCES account (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    accepted_at  timestamptz,
    PRIMARY KEY (dashboard_id, account_id)
);
CREATE INDEX dashboard_member_account_idx ON dashboard_member (account_id);

CREATE TABLE grp_grant (
    dashboard_id uuid        NOT NULL,
    group_id     uuid        NOT NULL,
    account_id   uuid        NOT NULL,
    can_edit     boolean     NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, account_id),
    FOREIGN KEY (dashboard_id, group_id) REFERENCES grp (dashboard_id, id) ON DELETE CASCADE,
    FOREIGN KEY (dashboard_id, account_id) REFERENCES dashboard_member (dashboard_id, account_id) ON DELETE CASCADE
);
CREATE INDEX grp_grant_member_idx ON grp_grant (dashboard_id, account_id);

CREATE TABLE instance_invite (
    id          uuid        PRIMARY KEY,
    token_hash  bytea       NOT NULL UNIQUE,
    created_by  uuid        REFERENCES account (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    used_by     uuid        REFERENCES account (id) ON DELETE SET NULL
);

-- +goose Down
DROP TABLE instance_invite;
DROP TABLE grp_grant;
DROP TABLE dashboard_member;
DROP INDEX account_nick_idx;
