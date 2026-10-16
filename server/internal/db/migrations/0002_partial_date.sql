-- Частичная дата (F-14, docs/concepts/dates.md): составной тип + домен с проверкой
-- и функции. Те же правила — в Go (internal/partialdate) и TS (web/src/core/dates.ts);
-- общие кейсы — testdata/partial_dates.json. Операторов «раньше / возможно раньше»
-- и приведений к date нет (Q-07, В-13).

-- +goose Up
CREATE TYPE partial_date_parts AS (y smallint, m smallint, d smallint);

COMMENT ON TYPE partial_date_parts IS 'Части частичной даты; в колонках используется домен partial_date';

-- +goose StatementBegin
-- Число дней в месяце; год NULL — неизвестен (у февраля 29). Без ошибок на любом входе.
CREATE FUNCTION pd_days_in_month(y integer, m integer) RETURNS integer
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN CASE
    WHEN m IN (1, 3, 5, 7, 8, 10, 12) THEN 31
    WHEN m IN (4, 6, 9, 11) THEN 30
    WHEN m = 2 THEN CASE
        WHEN y IS NULL OR (y % 4 = 0 AND (y % 100 <> 0 OR y % 400 = 0)) THEN 29
        ELSE 28 END
    END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Допустимость частей: хотя бы одна есть, диапазоны, день есть в месяце.
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

-- Внимание: «date IS NULL» у составного типа истинно и для строки из одних NULL,
-- а «IS NOT NULL» ложно при любой пустой части; проверять NULL — через ::text.
-- VALUE::text IS NULL — настоящий NULL (необязательная колонка); строка из одних
-- NULL «(,,)» — пустая дата, она отклоняется.
CREATE DOMAIN partial_date AS partial_date_parts
    CONSTRAINT partial_date_valid
    CHECK (VALUE::text IS NULL OR pd_valid((VALUE).y, (VALUE).m, (VALUE).d));

COMMENT ON DOMAIN partial_date IS 'Частичная дата: год, месяц, день — каждая часть необязательна, хотя бы одна есть (F-14)';

-- +goose StatementBegin
-- Нижняя граница; NULL без года.
CREATE FUNCTION pd_lo(p partial_date) RETURNS date
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
RETURN CASE WHEN (p).y IS NOT NULL
    THEN make_date((p).y, coalesce((p).m, 1), coalesce((p).d, 1)) END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Верхняя граница; NULL без года.
CREATE FUNCTION pd_hi(p partial_date) RETURNS date
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
RETURN CASE WHEN (p).y IS NOT NULL
    THEN make_date((p).y, coalesce((p).m, 12),
                   coalesce((p).d, pd_days_in_month((p).y, coalesce((p).m, 12)))) END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Точная дата, если известны все части.
CREATE FUNCTION pd_exact(p partial_date) RETURNS date
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
RETURN CASE WHEN (p).y IS NOT NULL AND (p).m IS NOT NULL AND (p).d IS NOT NULL
    THEN make_date((p).y, (p).m, (p).d) END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Ключ сортировки (Д-9): датированные — '0' + lo + hi, без года — '1' + ММДД границ.
-- Сравнивать с COLLATE "C".
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
-- Полных лет на дату at, считая от верхней границы (не меньше стольких).
-- NULL без года и до нижней границы. 29.02 в невисокосный год — 28.02.
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
-- Ближайшая годовщина не раньше from_date (включительно); нужны месяц и день.
-- Год известен — не раньше самой даты. 29.02 в невисокосный год — 28.02.
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

-- +goose Down
DROP FUNCTION pd_next_anniversary(partial_date, date);
DROP FUNCTION pd_age_at(partial_date, date);
DROP FUNCTION pd_sort_key(partial_date);
DROP FUNCTION pd_exact(partial_date);
DROP FUNCTION pd_hi(partial_date);
DROP FUNCTION pd_lo(partial_date);
DROP DOMAIN partial_date;
DROP FUNCTION pd_valid(integer, integer, integer);
DROP FUNCTION pd_days_in_month(integer, integer);
DROP TYPE partial_date_parts;
