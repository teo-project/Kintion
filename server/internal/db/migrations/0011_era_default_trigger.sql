-- Эры (F-14, F-143), продолжение 0009–0010:
-- 1. Дата может состоять из одной эры, без года, месяца и дня («мезозойская
--    эра»); эра при месяце или дне без года — по-прежнему нельзя.
-- 2. Пустая эра — эра по умолчанию дашборда: триггер на таблицах с датами при
--    записи ставит её датам с годом и пустой эрой, если эра по умолчанию
--    непустая, — для любого пути записи (форма, API, импорт). Существующие
--    такие даты переводятся здесь же. При восстановлении из бэкапа (pg_restore)
--    триггеры создаются после данных — данные не меняются.
-- 3. Ключ сортировки учитывает эры с обратным счётом (минимальный год больше
--    максимального, F-143): внутри такой эры больший год — раньше.
-- Те же правила — в Go (internal/partialdate) и TS (web/src/core/dates.ts);
-- общие кейсы — testdata/partial_dates.json.

-- +goose Up
-- +goose StatementBegin
-- Допустимость частей: хотя бы одна часть есть (y, m, d или эра); года 0 нет;
-- эра непустая, без пробелов по краям (unicode.IsSpace в Go) и управляющих
-- символов (Cc), до 64 символов; эра — при годе или одна, без месяца и дня.
CREATE OR REPLACE FUNCTION pd_valid(y integer, m integer, d integer, e text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN CASE
    WHEN y IS NULL AND m IS NULL AND d IS NULL AND e IS NULL THEN false
    WHEN y = 0 THEN false
    WHEN m IS NOT NULL AND (m < 1 OR m > 12) THEN false
    WHEN d IS NOT NULL AND (d < 1 OR d > 31) THEN false
    WHEN e IS NOT NULL AND ((y IS NULL AND (m IS NOT NULL OR d IS NOT NULL)) OR e = '' OR char_length(e) > 64
                            OR e ~ '^[ \u0085   -     　]' OR e ~ '[ \u0085   -     　]$'
                            OR e ~ '[\u0001-\u001f\u007f-\u009f]') THEN false
    WHEN m IS NOT NULL AND d IS NOT NULL THEN d <= pd_days_in_month(y, m)
    ELSE true
    END;
-- +goose StatementEnd

COMMENT ON DOMAIN partial_date IS 'Частичная дата: год (со знаком, без 0), месяц, день — каждая часть необязательна, хотя бы одна есть; эра — необязательная надпись при годе или одна (F-14)';

DROP FUNCTION pd_sort_key(partial_date, text[]);

-- +goose StatementBegin
-- Ключ сортировки (Д-9, F-14): датированные — '0' + место эры + год
-- (10 цифр: y + 2^31, в эре с обратным счётом — 2^31 − y) + ММДД нижней и
-- верхней границы; дата из одной эры — '0' + место эры (раньше всех дат эры);
-- без года — '1' + ММДД границ, после датированных. Порядок эр — список eras,
-- эры с обратным счётом — reversed (настройки дашборда, F-143). Сравнивать с
-- COLLATE "C".
CREATE FUNCTION pd_sort_key(p partial_date, eras text[] DEFAULT '{}', reversed text[] DEFAULT '{}') RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN CASE
    WHEN p::text IS NULL THEN NULL
    WHEN (p).y IS NOT NULL
        THEN '0' || pd_era_key((p).e, coalesce(eras, '{}'))
             || lpad((CASE WHEN coalesce((p).e, '') = ANY (coalesce(reversed, '{}'))
                           THEN 2147483648 - (p).y::bigint
                           ELSE (p).y::bigint + 2147483648 END)::text, 10, '0')
             || lpad(coalesce((p).m, 1)::text, 2, '0') || lpad(coalesce((p).d, 1)::text, 2, '0')
             || lpad(coalesce((p).m, 12)::text, 2, '0')
             || lpad(coalesce((p).d, pd_days_in_month((p).y, coalesce((p).m, 12)))::text, 2, '0')
    WHEN (p).e IS NOT NULL AND (p).m IS NULL AND (p).d IS NULL
        THEN '0' || pd_era_key((p).e, coalesce(eras, '{}'))
    ELSE '1' || lpad(coalesce((p).m, 1)::text, 2, '0') || lpad(coalesce((p).d, 1)::text, 2, '0')
             || lpad(coalesce((p).m, 12)::text, 2, '0')
             || lpad(coalesce((p).d, pd_days_in_month(NULL, coalesce((p).m, 12)))::text, 2, '0')
    END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Эра по умолчанию дашборда (F-143); пустая — NULL. Тело SQL-стандарта:
-- таблица связывается при создании, от search_path не зависит.
CREATE FUNCTION pd_dashboard_default_era(dash_id uuid) RETURNS text
LANGUAGE sql STABLE PARALLEL SAFE
RETURN (SELECT nullif(d.eras ->> 'default', '') FROM dashboard d WHERE d.id = dash_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- Триггер: датам с годом и пустой эрой — эра по умолчанию дашборда (F-143),
-- если она непустая. Колонки-даты — аргументы триггера. Меняются только эти
-- колонки строки: jsonb_populate_record берёт остальные из NEW как есть.
CREATE FUNCTION pd_apply_default_era() RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    def   text := pd_dashboard_default_era(NEW.dashboard_id);
    row_j jsonb;
    patch jsonb := '{}';
    col   text;
BEGIN
    IF def IS NULL THEN
        RETURN NEW;
    END IF;
    row_j := to_jsonb(NEW);
    FOREACH col IN ARRAY TG_ARGV LOOP
        IF jsonb_typeof(row_j -> col) = 'object' AND row_j -> col ->> 'y' IS NOT NULL
           AND row_j -> col ->> 'e' IS NULL THEN
            patch := patch || jsonb_build_object(col, (row_j -> col) || jsonb_build_object('e', def));
        END IF;
    END LOOP;
    IF patch <> '{}' THEN
        NEW := jsonb_populate_record(NEW, patch);
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER entity_default_era BEFORE INSERT OR UPDATE ON entity
    FOR EACH ROW EXECUTE FUNCTION pd_apply_default_era('start_date', 'end_date');
CREATE TRIGGER participation_default_era BEFORE INSERT OR UPDATE ON participation
    FOR EACH ROW EXECUTE FUNCTION pd_apply_default_era('valid_from', 'valid_to');
CREATE TRIGGER field_entry_default_era BEFORE INSERT OR UPDATE ON field_entry
    FOR EACH ROW EXECUTE FUNCTION pd_apply_default_era('v_date', 'valid_from', 'valid_to');

-- существующие даты с пустой эрой в дашбордах с непустой эрой по умолчанию —
-- через триггер (UPDATE строки ставит эру)
UPDATE entity SET start_date = start_date
WHERE pd_dashboard_default_era(dashboard_id) IS NOT NULL
  AND (((start_date).y IS NOT NULL AND (start_date).e IS NULL) OR ((end_date).y IS NOT NULL AND (end_date).e IS NULL));
UPDATE participation SET valid_from = valid_from
WHERE pd_dashboard_default_era(dashboard_id) IS NOT NULL
  AND (((valid_from).y IS NOT NULL AND (valid_from).e IS NULL) OR ((valid_to).y IS NOT NULL AND (valid_to).e IS NULL));
UPDATE field_entry SET v_date = v_date
WHERE pd_dashboard_default_era(dashboard_id) IS NOT NULL
  AND (((v_date).y IS NOT NULL AND (v_date).e IS NULL) OR ((valid_from).y IS NOT NULL AND (valid_from).e IS NULL)
       OR ((valid_to).y IS NOT NULL AND (valid_to).e IS NULL));

-- +goose Down
-- Откат возможен, только пока нет дат из одной эры (старый pd_valid их не
-- примет): иначе миграция падает, данные не теряются. Эры, поставленные
-- триггером, остаются.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM entity WHERE ((start_date).e IS NOT NULL AND (start_date).y IS NULL)
                                       OR ((end_date).e IS NOT NULL AND (end_date).y IS NULL))
       OR EXISTS (SELECT 1 FROM participation WHERE ((valid_from).e IS NOT NULL AND (valid_from).y IS NULL)
                                                 OR ((valid_to).e IS NOT NULL AND (valid_to).y IS NULL))
       OR EXISTS (SELECT 1 FROM field_entry WHERE ((v_date).e IS NOT NULL AND (v_date).y IS NULL)
                                               OR ((valid_from).e IS NOT NULL AND (valid_from).y IS NULL)
                                               OR ((valid_to).e IS NOT NULL AND (valid_to).y IS NULL)) THEN
        RAISE EXCEPTION 'есть даты из одной эры: откат 0011 потерял бы данные';
    END IF;
END
$$;
-- +goose StatementEnd

DROP TRIGGER field_entry_default_era ON field_entry;
DROP TRIGGER participation_default_era ON participation;
DROP TRIGGER entity_default_era ON entity;
DROP FUNCTION pd_apply_default_era();
DROP FUNCTION pd_dashboard_default_era(uuid);

DROP FUNCTION pd_sort_key(partial_date, text[], text[]);

-- +goose StatementBegin
CREATE FUNCTION pd_sort_key(p partial_date, eras text[] DEFAULT '{}') RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN CASE
    WHEN p::text IS NULL THEN NULL
    WHEN (p).y IS NOT NULL
        THEN '0' || pd_era_key((p).e, coalesce(eras, '{}')) || lpad(((p).y::bigint + 2147483648)::text, 10, '0')
             || lpad(coalesce((p).m, 1)::text, 2, '0') || lpad(coalesce((p).d, 1)::text, 2, '0')
             || lpad(coalesce((p).m, 12)::text, 2, '0')
             || lpad(coalesce((p).d, pd_days_in_month((p).y, coalesce((p).m, 12)))::text, 2, '0')
    ELSE '1' || lpad(coalesce((p).m, 1)::text, 2, '0') || lpad(coalesce((p).d, 1)::text, 2, '0')
             || lpad(coalesce((p).m, 12)::text, 2, '0')
             || lpad(coalesce((p).d, pd_days_in_month(NULL, coalesce((p).m, 12)))::text, 2, '0')
    END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION pd_valid(y integer, m integer, d integer, e text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN CASE
    WHEN y IS NULL AND m IS NULL AND d IS NULL THEN false
    WHEN y = 0 THEN false
    WHEN m IS NOT NULL AND (m < 1 OR m > 12) THEN false
    WHEN d IS NOT NULL AND (d < 1 OR d > 31) THEN false
    WHEN e IS NOT NULL AND (y IS NULL OR e = '' OR char_length(e) > 64
                            OR e ~ '^[ \u0085   -     　]' OR e ~ '[ \u0085   -     　]$'
                            OR e ~ '[\u0001-\u001f\u007f-\u009f]') THEN false
    WHEN m IS NOT NULL AND d IS NOT NULL THEN d <= pd_days_in_month(y, m)
    ELSE true
    END;
-- +goose StatementEnd

COMMENT ON DOMAIN partial_date IS 'Частичная дата: год (со знаком, без 0), месяц, день — каждая часть необязательна, хотя бы одна есть; эра — необязательная надпись при годе (F-14)';
