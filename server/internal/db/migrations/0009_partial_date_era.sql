-- Частичная дата: год int со знаком (года 0 нет, отрицательный — до начала эры)
-- и часть «эра» (F-14, docs/concepts/dates.md). Те же правила — в Go
-- (internal/partialdate) и TS (web/src/core/dates.ts); общие кейсы —
-- testdata/partial_dates.json.
--
-- Тип занят в колонках через домен partial_date, сменить часть на месте
-- PostgreSQL не даёт («cannot alter type … because column … uses it»), поэтому
-- тип пересоздаётся: новый тип и домен, перевод колонок, удаление старых,
-- переименование. Всё — одной транзакцией миграции.

-- +goose Up
CREATE TYPE partial_date_parts_v2 AS (y integer, m smallint, d smallint, e text);

-- +goose StatementBegin
-- Астрономический год: до начала эры −1 → 0, −2 → −1 (года 0 в датах нет).
CREATE FUNCTION pd_astro_year(y integer) RETURNS integer
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN CASE WHEN y < 0 THEN y + 1 ELSE y END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Число дней в месяце; год NULL — неизвестен (у февраля 29). Високосность —
-- по астрономическому году: 1 год до н. э. (−1) — високосный.
CREATE OR REPLACE FUNCTION pd_days_in_month(y integer, m integer) RETURNS integer
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN CASE
    WHEN m IN (1, 3, 5, 7, 8, 10, 12) THEN 31
    WHEN m IN (4, 6, 9, 11) THEN 30
    WHEN m = 2 THEN CASE
        WHEN y IS NULL OR (pd_astro_year(y) % 4 = 0
                           AND (pd_astro_year(y) % 100 <> 0 OR pd_astro_year(y) % 400 = 0)) THEN 29
        ELSE 28 END
    END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Допустимость частей: хотя бы одна из y, m, d есть; года 0 нет; эра — только
-- при годе, непустая, без пробелов по краям (unicode.IsSpace в Go) и управляющих
-- символов (Cc), до 64 символов.
CREATE FUNCTION pd_valid(y integer, m integer, d integer, e text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN CASE
    WHEN y IS NULL AND m IS NULL AND d IS NULL THEN false
    WHEN y = 0 THEN false
    WHEN m IS NOT NULL AND (m < 1 OR m > 12) THEN false
    WHEN d IS NOT NULL AND (d < 1 OR d > 31) THEN false
    WHEN e IS NOT NULL AND (y IS NULL OR e = '' OR char_length(e) > 64
                            OR e ~ '^[ \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]' OR e ~ '[ \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]$'
                            OR e ~ '[\u0001-\u001f\u007f-\u009f]') THEN false
    WHEN m IS NOT NULL AND d IS NOT NULL THEN d <= pd_days_in_month(y, m)
    ELSE true
    END;
-- +goose StatementEnd

CREATE DOMAIN partial_date_v2 AS partial_date_parts_v2
    CONSTRAINT partial_date_valid
    CHECK (VALUE::text IS NULL OR pd_valid((VALUE).y, (VALUE).m, (VALUE).d, (VALUE).e));

-- старые функции зависят от старого домена
DROP FUNCTION pd_next_anniversary(partial_date, date);
DROP FUNCTION pd_age_at(partial_date, date);
DROP FUNCTION pd_sort_key(partial_date);
DROP FUNCTION pd_exact(partial_date);
DROP FUNCTION pd_hi(partial_date);
DROP FUNCTION pd_lo(partial_date);

-- перевод колонок: части переносятся как есть, эра — пустая (наша)
ALTER TABLE entity
    ALTER COLUMN start_date TYPE partial_date_v2 USING CASE WHEN start_date::text IS NOT NULL
        THEN ROW((start_date).y, (start_date).m, (start_date).d, NULL)::partial_date_parts_v2::partial_date_v2 END,
    ALTER COLUMN end_date TYPE partial_date_v2 USING CASE WHEN end_date::text IS NOT NULL
        THEN ROW((end_date).y, (end_date).m, (end_date).d, NULL)::partial_date_parts_v2::partial_date_v2 END;
ALTER TABLE participation
    ALTER COLUMN valid_from TYPE partial_date_v2 USING CASE WHEN valid_from::text IS NOT NULL
        THEN ROW((valid_from).y, (valid_from).m, (valid_from).d, NULL)::partial_date_parts_v2::partial_date_v2 END,
    ALTER COLUMN valid_to TYPE partial_date_v2 USING CASE WHEN valid_to::text IS NOT NULL
        THEN ROW((valid_to).y, (valid_to).m, (valid_to).d, NULL)::partial_date_parts_v2::partial_date_v2 END;
ALTER TABLE field_entry
    ALTER COLUMN v_date TYPE partial_date_v2 USING CASE WHEN v_date::text IS NOT NULL
        THEN ROW((v_date).y, (v_date).m, (v_date).d, NULL)::partial_date_parts_v2::partial_date_v2 END,
    ALTER COLUMN valid_from TYPE partial_date_v2 USING CASE WHEN valid_from::text IS NOT NULL
        THEN ROW((valid_from).y, (valid_from).m, (valid_from).d, NULL)::partial_date_parts_v2::partial_date_v2 END,
    ALTER COLUMN valid_to TYPE partial_date_v2 USING CASE WHEN valid_to::text IS NOT NULL
        THEN ROW((valid_to).y, (valid_to).m, (valid_to).d, NULL)::partial_date_parts_v2::partial_date_v2 END;

DROP DOMAIN partial_date;
DROP FUNCTION pd_valid(integer, integer, integer);
DROP TYPE partial_date_parts;

ALTER DOMAIN partial_date_v2 RENAME TO partial_date;
ALTER TYPE partial_date_parts_v2 RENAME TO partial_date_parts;

COMMENT ON TYPE partial_date_parts IS 'Части частичной даты; в колонках используется домен partial_date';
COMMENT ON DOMAIN partial_date IS 'Частичная дата: год (со знаком, без 0), месяц, день — каждая часть необязательна, хотя бы одна есть; эра — необязательная надпись при годе (F-14)';

-- +goose StatementBegin
-- Календарная дата (date) из частей: только для нашей эры и в пределах date
-- PostgreSQL (с 4713 г. до н. э.); иначе NULL — make_date за пределами не вызывается.
-- Отрицательный год make_date понимает как год до н. э. — как и частичная дата.
CREATE FUNCTION pd_make_date(y integer, m integer, d integer, e text) RETURNS date
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN CASE WHEN e IS NULL AND y BETWEEN -4713 AND 5874897
    THEN make_date(y, m, d) END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Нижняя граница; NULL без года, с эрой и вне пределов date.
CREATE FUNCTION pd_lo(p partial_date) RETURNS date
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
RETURN CASE WHEN (p).y IS NOT NULL
    THEN pd_make_date((p).y, coalesce((p).m, 1), coalesce((p).d, 1), (p).e) END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Верхняя граница; NULL без года, с эрой и вне пределов date.
CREATE FUNCTION pd_hi(p partial_date) RETURNS date
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
RETURN CASE WHEN (p).y IS NOT NULL
    THEN pd_make_date((p).y, coalesce((p).m, 12),
                      coalesce((p).d, pd_days_in_month((p).y, coalesce((p).m, 12))), (p).e) END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Точная дата, если известны все части (наша эра, в пределах date).
CREATE FUNCTION pd_exact(p partial_date) RETURNS date
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
RETURN CASE WHEN (p).y IS NOT NULL AND (p).m IS NOT NULL AND (p).d IS NOT NULL
    THEN pd_make_date((p).y, (p).m, (p).d, (p).e) END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Место эры в ключе сортировки: эры вне списка eras — первыми (новая эра —
-- наверх, F-143), между собой по тексту (пустая — наша — раньше всех), с
-- концом-разделителем chr(1); эры из списка — дальше, по месту в нём.
CREATE FUNCTION pd_era_key(e text, eras text[]) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN CASE WHEN array_position(eras, coalesce(e, '')) IS NULL
    THEN '0' || coalesce(e, '') || chr(1)
    ELSE '1' || lpad((array_position(eras, coalesce(e, '')) - 1)::text, 5, '0') END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Ключ сортировки (Д-9, F-14): датированные — '0' + место эры + год
-- (y + 2^31, 10 цифр: отрицательные раньше) + ММДД нижней и верхней границы;
-- без года — '1' + ММДД границ, после датированных. Порядок эр — список eras
-- (настройки дашборда, F-143). Сравнивать с COLLATE "C".
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
-- Полных лет на дату at (наша эра), считая от верхней границы (не меньше
-- стольких). NULL без года, с эрой и до нижней границы. 29.02 в невисокосный
-- год — 28.02. Считается по частям, без make_date: годы любые (bigint — разность
-- годов со знаком больше int).
CREATE FUNCTION pd_age_at(p partial_date, at date) RETURNS bigint
LANGUAGE plpgsql IMMUTABLE STRICT PARALLEL SAFE
AS $$
DECLARE
    ay  integer := extract(year FROM at);  -- до н. э. — отрицательный
    am  integer := extract(month FROM at);
    ad  integer := extract(day FROM at);
    hm  integer;
    hd  integer;
    age bigint;
BEGIN
    IF (p).y IS NULL OR (p).e IS NOT NULL THEN
        RETURN NULL;
    END IF;
    IF (pd_astro_year(ay), am, ad) < (pd_astro_year((p).y), coalesce((p).m, 1), coalesce((p).d, 1)) THEN
        RETURN NULL;
    END IF;
    hm := coalesce((p).m, 12);
    hd := least(coalesce((p).d, pd_days_in_month((p).y, hm)), pd_days_in_month(ay, hm));
    age := pd_astro_year(ay)::bigint - pd_astro_year((p).y);
    IF (am, ad) < (hm, hd) THEN
        age := age - 1;
    END IF;
    RETURN greatest(age, 0);
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Ближайшая годовщина не раньше from_date (включительно); нужны месяц и день,
-- наша эра. Год известен — не раньше самой даты. 29.02 в невисокосный год — 28.02.
CREATE FUNCTION pd_next_anniversary(p partial_date, from_date date) RETURNS date
LANGUAGE plpgsql IMMUTABLE STRICT PARALLEL SAFE
AS $$
DECLARE
    yr   integer;
    cand date;
BEGIN
    IF (p).m IS NULL OR (p).d IS NULL OR (p).e IS NOT NULL THEN
        RETURN NULL;
    END IF;
    yr := extract(year FROM from_date)::integer;  -- до н. э. — отрицательный
    yr := greatest(yr, coalesce((p).y, yr));
    LOOP
        cand := pd_make_date(yr, (p).m, least((p).d, pd_days_in_month(yr, (p).m)), NULL);
        IF cand IS NULL THEN
            RETURN NULL;
        END IF;
        IF cand >= from_date THEN
            RETURN cand;
        END IF;
        yr := CASE WHEN yr = -1 THEN 1 ELSE yr + 1 END;
    END LOOP;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- Откат возможен, только пока нет дат с эрой и годов вне 1..9999 (старый домен
-- их не примет): иначе миграция падает, данные не теряются.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM entity WHERE (start_date).e IS NOT NULL OR (end_date).e IS NOT NULL)
       OR EXISTS (SELECT 1 FROM participation WHERE (valid_from).e IS NOT NULL OR (valid_to).e IS NOT NULL)
       OR EXISTS (SELECT 1 FROM field_entry
                  WHERE (v_date).e IS NOT NULL OR (valid_from).e IS NOT NULL OR (valid_to).e IS NOT NULL) THEN
        RAISE EXCEPTION 'есть даты с эрой: откат 0009 потерял бы данные';
    END IF;
END
$$;
-- +goose StatementEnd

DROP FUNCTION pd_next_anniversary(partial_date, date);
DROP FUNCTION pd_age_at(partial_date, date);
DROP FUNCTION pd_sort_key(partial_date, text[]);
DROP FUNCTION pd_era_key(text, text[]);
DROP FUNCTION pd_exact(partial_date);
DROP FUNCTION pd_hi(partial_date);
DROP FUNCTION pd_lo(partial_date);
DROP FUNCTION pd_make_date(integer, integer, integer, text);

CREATE TYPE partial_date_parts_v1 AS (y smallint, m smallint, d smallint);

-- +goose StatementBegin
CREATE FUNCTION pd_valid(y integer, m integer, d integer) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN CASE
    WHEN y IS NULL AND m IS NULL AND d IS NULL THEN false
    WHEN y IS NOT NULL AND (y < 1 OR y > 9999) THEN false
    WHEN m IS NOT NULL AND (m < 1 OR m > 12) THEN false
    WHEN d IS NOT NULL AND (d < 1 OR d > 31) THEN false
    WHEN m IS NOT NULL AND d IS NOT NULL THEN d <= pd_days_in_month(y, m)
    ELSE true
    END;
-- +goose StatementEnd

CREATE DOMAIN partial_date_v1 AS partial_date_parts_v1
    CONSTRAINT partial_date_valid
    CHECK (VALUE::text IS NULL OR pd_valid((VALUE).y, (VALUE).m, (VALUE).d));

ALTER TABLE entity
    ALTER COLUMN start_date TYPE partial_date_v1 USING CASE WHEN start_date::text IS NOT NULL
        THEN ROW((start_date).y, (start_date).m, (start_date).d)::partial_date_parts_v1::partial_date_v1 END,
    ALTER COLUMN end_date TYPE partial_date_v1 USING CASE WHEN end_date::text IS NOT NULL
        THEN ROW((end_date).y, (end_date).m, (end_date).d)::partial_date_parts_v1::partial_date_v1 END;
ALTER TABLE participation
    ALTER COLUMN valid_from TYPE partial_date_v1 USING CASE WHEN valid_from::text IS NOT NULL
        THEN ROW((valid_from).y, (valid_from).m, (valid_from).d)::partial_date_parts_v1::partial_date_v1 END,
    ALTER COLUMN valid_to TYPE partial_date_v1 USING CASE WHEN valid_to::text IS NOT NULL
        THEN ROW((valid_to).y, (valid_to).m, (valid_to).d)::partial_date_parts_v1::partial_date_v1 END;
ALTER TABLE field_entry
    ALTER COLUMN v_date TYPE partial_date_v1 USING CASE WHEN v_date::text IS NOT NULL
        THEN ROW((v_date).y, (v_date).m, (v_date).d)::partial_date_parts_v1::partial_date_v1 END,
    ALTER COLUMN valid_from TYPE partial_date_v1 USING CASE WHEN valid_from::text IS NOT NULL
        THEN ROW((valid_from).y, (valid_from).m, (valid_from).d)::partial_date_parts_v1::partial_date_v1 END,
    ALTER COLUMN valid_to TYPE partial_date_v1 USING CASE WHEN valid_to::text IS NOT NULL
        THEN ROW((valid_to).y, (valid_to).m, (valid_to).d)::partial_date_parts_v1::partial_date_v1 END;

DROP DOMAIN partial_date;
DROP FUNCTION pd_valid(integer, integer, integer, text);
DROP TYPE partial_date_parts;
ALTER DOMAIN partial_date_v1 RENAME TO partial_date;
ALTER TYPE partial_date_parts_v1 RENAME TO partial_date_parts;
COMMENT ON TYPE partial_date_parts IS 'Части частичной даты; в колонках используется домен partial_date';
COMMENT ON DOMAIN partial_date IS 'Частичная дата: год, месяц, день — каждая часть необязательна, хотя бы одна есть (F-14)';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION pd_days_in_month(y integer, m integer) RETURNS integer
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN CASE
    WHEN m IN (1, 3, 5, 7, 8, 10, 12) THEN 31
    WHEN m IN (4, 6, 9, 11) THEN 30
    WHEN m = 2 THEN CASE
        WHEN y IS NULL OR (y % 4 = 0 AND (y % 100 <> 0 OR y % 400 = 0)) THEN 29
        ELSE 28 END
    END;
-- +goose StatementEnd
DROP FUNCTION pd_astro_year(integer);

-- +goose StatementBegin
CREATE FUNCTION pd_lo(p partial_date) RETURNS date
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
RETURN CASE WHEN (p).y IS NOT NULL
    THEN make_date((p).y, coalesce((p).m, 1), coalesce((p).d, 1)) END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pd_hi(p partial_date) RETURNS date
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
RETURN CASE WHEN (p).y IS NOT NULL
    THEN make_date((p).y, coalesce((p).m, 12),
                   coalesce((p).d, pd_days_in_month((p).y, coalesce((p).m, 12)))) END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pd_exact(p partial_date) RETURNS date
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
RETURN CASE WHEN (p).y IS NOT NULL AND (p).m IS NOT NULL AND (p).d IS NOT NULL
    THEN make_date((p).y, (p).m, (p).d) END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pd_sort_key(p partial_date) RETURNS text
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
RETURN CASE
    WHEN (p).y IS NOT NULL
        THEN '0' || to_char(pd_lo(p), 'YYYYMMDD') || to_char(pd_hi(p), 'YYYYMMDD')
    ELSE '1' || lpad(coalesce((p).m, 1)::text, 2, '0') || lpad(coalesce((p).d, 1)::text, 2, '0')
             || lpad(coalesce((p).m, 12)::text, 2, '0')
             || lpad(coalesce((p).d, pd_days_in_month(NULL, coalesce((p).m, 12)))::text, 2, '0')
    END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pd_age_at(p partial_date, at date) RETURNS integer
LANGUAGE plpgsql IMMUTABLE STRICT PARALLEL SAFE
AS $$
DECLARE
    hi  date;
    ay  integer := extract(year FROM at);
    hm  integer;
    hd  integer;
    age integer;
BEGIN
    IF (p).y IS NULL OR at < pd_lo(p) THEN
        RETURN NULL;
    END IF;
    hi := pd_hi(p);
    hm := extract(month FROM hi);
    hd := least(extract(day FROM hi)::integer, pd_days_in_month(ay, hm));
    age := ay - (p).y;
    IF (extract(month FROM at), extract(day FROM at)) < (hm, hd) THEN
        age := age - 1;
    END IF;
    RETURN greatest(age, 0);
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pd_next_anniversary(p partial_date, from_date date) RETURNS date
LANGUAGE plpgsql IMMUTABLE STRICT PARALLEL SAFE
AS $$
DECLARE
    yr   integer;
    cand date;
BEGIN
    IF (p).m IS NULL OR (p).d IS NULL THEN
        RETURN NULL;
    END IF;
    yr := greatest(extract(year FROM from_date)::integer, coalesce((p).y, 0));
    LOOP
        cand := make_date(yr, (p).m, least((p).d, pd_days_in_month(yr, (p).m)));
        IF cand >= from_date THEN
            RETURN cand;
        END IF;
        yr := yr + 1;
    END LOOP;
END
$$;
-- +goose StatementEnd
