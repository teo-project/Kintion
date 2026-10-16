-- Основная схема 0.1.
--
-- Все id — uuid, генерирует приложение (UUIDv7, Д-8). Уникальность — только
-- технических вещей (F-07): первичные ключи, (dashboard_id, id) для составных
-- FK, логин аккаунта, хеш токена сессии, файл в дашборде, пары меток, версия поля.
-- Все ссылки между объектами данных — составные FK (dashboard_id, x_id): связь
-- через границу дашборда невозможна на уровне БД (F-01).
-- Названия таблиц и колонок — в языковых файлах i18n/*.json, раздел schema (F-76).

-- +goose Up
CREATE TABLE account (
    id            uuid        PRIMARY KEY,
    login         text        NOT NULL UNIQUE,
    password_hash text        NOT NULL,
    nick          text,
    is_admin      boolean     NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE session (
    id           uuid        PRIMARY KEY,
    account_id   uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    token_hash   bytea       NOT NULL UNIQUE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    user_agent   text        NOT NULL DEFAULT ''
);
CREATE INDEX session_account_idx ON session (account_id);

-- Первый запуск (F-28): хеш bootstrap-токена.
CREATE TABLE bootstrap (
    token_hash bytea       PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    used_at    timestamptz
);

CREATE TABLE dashboard (
    id         uuid        PRIMARY KEY,
    owner_id   uuid        NOT NULL REFERENCES account (id),
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX dashboard_owner_idx ON dashboard (owner_id);

-- Паки, известные инстансу (журнал, F-60).
CREATE TABLE pack (
    id        text        PRIMARY KEY,
    version   text        NOT NULL,
    language  text        NOT NULL,
    loaded    boolean     NOT NULL DEFAULT true,
    loaded_at timestamptz NOT NULL DEFAULT now()
);

-- Шаблон в дашборде (F-36, F-58): ссылка на шаблон пака + ручные правки.
CREATE TABLE dashboard_template (
    id           uuid    PRIMARY KEY,
    dashboard_id uuid    NOT NULL REFERENCES dashboard (id),
    pack_id      text    NOT NULL,
    template_key text    NOT NULL,
    entity       text    NOT NULL,
    pack_version text    NOT NULL,
    overrides    jsonb   NOT NULL DEFAULT '{}',
    active       boolean NOT NULL DEFAULT true,
    -- версия правок шаблона (F-45); общие свои поля (F-96) — в overrides.custom_fields
    rev          bigint  NOT NULL DEFAULT 0,
    UNIQUE (dashboard_id, id)
);
CREATE INDEX dashboard_template_dashboard_idx ON dashboard_template (dashboard_id);

-- Запись (сущность). start/end — системные даты (F-70) с привязкой к записи-событию.
CREATE TABLE entity (
    id            uuid         PRIMARY KEY,
    dashboard_id  uuid         NOT NULL REFERENCES dashboard (id),
    entity        text         NOT NULL,
    template_id   uuid         NOT NULL,
    start_date    partial_date,
    start_link    uuid,
    end_date      partial_date,
    end_link      uuid,
    note          text,
    title_manual  text,
    sensitive     boolean      NOT NULL DEFAULT false,
    custom_fields jsonb        NOT NULL DEFAULT '[]',
    created_by    uuid         REFERENCES account (id),
    created_at    timestamptz  NOT NULL DEFAULT now(),
    updated_at    timestamptz  NOT NULL DEFAULT now(),
    deleted_at    timestamptz,
    deleted_by    uuid         REFERENCES account (id),
    UNIQUE (dashboard_id, id),
    FOREIGN KEY (dashboard_id, template_id) REFERENCES dashboard_template (dashboard_id, id),
    FOREIGN KEY (dashboard_id, start_link) REFERENCES entity (dashboard_id, id) ON DELETE SET NULL (start_link),
    FOREIGN KEY (dashboard_id, end_link) REFERENCES entity (dashboard_id, id) ON DELETE SET NULL (end_link)
);
-- активные записи (F-60)
CREATE INDEX entity_active_idx ON entity (dashboard_id, entity) WHERE deleted_at IS NULL;
CREATE INDEX entity_template_idx ON entity (template_id);
CREATE INDEX entity_start_link_idx ON entity (start_link) WHERE start_link IS NOT NULL;
CREATE INDEX entity_end_link_idx ON entity (end_link) WHERE end_link IS NOT NULL;

-- Группы (F-02): метки записей; порядок — ручной, один на дашборд.
CREATE TABLE grp (
    id           uuid NOT NULL PRIMARY KEY,
    dashboard_id uuid NOT NULL REFERENCES dashboard (id),
    name         text NOT NULL,
    sort_key     text COLLATE "C" NOT NULL,
    UNIQUE (dashboard_id, id)
);

-- Вложенность групп — многие ко многим (F-105): child входит в parent.
-- Права наследуются вниз по всем путям; циклы запрещает сервер.
CREATE TABLE grp_link (
    dashboard_id uuid NOT NULL,
    parent_id    uuid NOT NULL,
    child_id     uuid NOT NULL,
    PRIMARY KEY (parent_id, child_id),
    FOREIGN KEY (dashboard_id, parent_id) REFERENCES grp (dashboard_id, id) ON DELETE CASCADE,
    FOREIGN KEY (dashboard_id, child_id) REFERENCES grp (dashboard_id, id) ON DELETE CASCADE,
    CHECK (parent_id <> child_id)
);
CREATE INDEX grp_link_child_idx ON grp_link (child_id);
CREATE INDEX grp_link_dashboard_idx ON grp_link (dashboard_id);

CREATE TABLE entity_group (
    dashboard_id uuid NOT NULL,
    entity_id    uuid NOT NULL,
    group_id     uuid NOT NULL,
    PRIMARY KEY (entity_id, group_id),
    FOREIGN KEY (dashboard_id, entity_id) REFERENCES entity (dashboard_id, id) ON DELETE CASCADE,
    FOREIGN KEY (dashboard_id, group_id) REFERENCES grp (dashboard_id, id) ON DELETE CASCADE
);
CREATE INDEX entity_group_group_idx ON entity_group (group_id);

-- Участие записи в контейнере (отношении, событии) в роли; повтор пары допустим.
CREATE TABLE participation (
    id           uuid         PRIMARY KEY,
    dashboard_id uuid         NOT NULL,
    container_id uuid         NOT NULL,
    entity_id    uuid         NOT NULL,
    role         text         NOT NULL,
    valid_from   partial_date,
    valid_to     partial_date,
    note         text,
    sort_key     text COLLATE "C" NOT NULL,
    created_at   timestamptz  NOT NULL DEFAULT now(),
    updated_at   timestamptz  NOT NULL DEFAULT now(),
    FOREIGN KEY (dashboard_id, container_id) REFERENCES entity (dashboard_id, id) ON DELETE CASCADE,
    FOREIGN KEY (dashboard_id, entity_id) REFERENCES entity (dashboard_id, id) ON DELETE CASCADE
);
CREATE INDEX participation_container_idx ON participation (container_id, role, sort_key);
CREATE INDEX participation_entity_idx ON participation (entity_id);

-- Значение поля (F-11, F-12): формальное — в типизированной колонке, расширенное — в meta.
CREATE TABLE field_entry (
    id           uuid         PRIMARY KEY,
    dashboard_id uuid         NOT NULL,
    entity_id    uuid         NOT NULL,
    field_key    text         NOT NULL,
    sort_key     text COLLATE "C" NOT NULL,
    v_text       text,
    v_num        numeric,
    v_date       partial_date,
    v_bool       boolean,
    v_ref        uuid,
    meta         jsonb        NOT NULL DEFAULT '{}',
    valid_from   partial_date,
    valid_to     partial_date,
    note         text,
    created_at   timestamptz  NOT NULL DEFAULT now(),
    updated_at   timestamptz  NOT NULL DEFAULT now(),
    UNIQUE (dashboard_id, id),
    FOREIGN KEY (dashboard_id, entity_id) REFERENCES entity (dashboard_id, id) ON DELETE CASCADE,
    -- очистка цели удаляет значение-ссылку (Д-32)
    FOREIGN KEY (dashboard_id, v_ref) REFERENCES entity (dashboard_id, id) ON DELETE CASCADE
);
CREATE INDEX field_entry_entity_idx ON field_entry (entity_id, field_key, sort_key);
CREATE INDEX field_entry_ref_idx ON field_entry (v_ref) WHERE v_ref IS NOT NULL;

-- Версия поля для одновременной правки (F-45): поля шаблона, custom.*, системные поля записи.
CREATE TABLE field_rev (
    entity_id uuid   NOT NULL REFERENCES entity (id) ON DELETE CASCADE,
    field_key text   NOT NULL,
    rev       bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (entity_id, field_key)
);

-- Привязка значения к записи-событию: в начало, в конец, без места (F-11).
CREATE TABLE entry_link (
    id           uuid NOT NULL PRIMARY KEY,
    dashboard_id uuid NOT NULL,
    entry_id     uuid NOT NULL,
    target_id    uuid NOT NULL,
    anchor       text NOT NULL CHECK (anchor IN ('start', 'end', 'none')),
    FOREIGN KEY (dashboard_id, entry_id) REFERENCES field_entry (dashboard_id, id) ON DELETE CASCADE,
    FOREIGN KEY (dashboard_id, target_id) REFERENCES entity (dashboard_id, id) ON DELETE CASCADE
);
CREATE INDEX entry_link_entry_idx ON entry_link (entry_id);
CREATE INDEX entry_link_target_idx ON entry_link (target_id);

-- Файлы дашборда (F-29, F-30): имя на томе — sha256 содержимого.
CREATE TABLE file (
    id           uuid        PRIMARY KEY,
    dashboard_id uuid        NOT NULL REFERENCES dashboard (id),
    sha256       text        NOT NULL,
    mime         text        NOT NULL,
    size         bigint      NOT NULL,
    width        integer,
    height       integer,
    uploaded_by  uuid        REFERENCES account (id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (dashboard_id, sha256)
);

-- Журнал (F-08, F-16): только вставка. entity_id и dashboard_id — без FK,
-- записи переживают очистку.
CREATE TABLE audit_log (
    id             uuid        PRIMARY KEY,
    dashboard_id   uuid,
    at             timestamptz NOT NULL DEFAULT now(),
    actor_id       uuid,
    operation_id   uuid        NOT NULL,
    source         text        NOT NULL,
    request_id     text        NOT NULL DEFAULT '',
    app_version    text        NOT NULL,
    schema_version bigint      NOT NULL,
    entity_id      uuid,
    object         text        NOT NULL,
    object_id      uuid,
    op             text        NOT NULL,
    before         jsonb,
    after          jsonb
);
CREATE INDEX audit_log_dashboard_idx ON audit_log (dashboard_id, at, id);
CREATE INDEX audit_log_entity_idx ON audit_log (entity_id, at) WHERE entity_id IS NOT NULL;
CREATE INDEX audit_log_operation_idx ON audit_log (operation_id);

-- +goose StatementBegin
CREATE FUNCTION audit_log_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log: только вставка (% запрещено)', TG_OP;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER audit_log_no_update_delete
    BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_append_only();

CREATE TRIGGER audit_log_no_truncate
    BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_append_only();

-- +goose Down
DROP TABLE audit_log;
DROP FUNCTION audit_log_append_only();
DROP TABLE file;
DROP TABLE entry_link;
DROP TABLE field_rev;
DROP TABLE field_entry;
DROP TABLE participation;
DROP TABLE entity_group;
DROP TABLE grp_link;
DROP TABLE grp;
DROP TABLE entity;
DROP TABLE dashboard_template;
DROP TABLE pack;
DROP TABLE dashboard;
DROP TABLE bootstrap;
DROP TABLE session;
DROP TABLE account;
