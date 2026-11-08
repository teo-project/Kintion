-- Эры (F-14, F-143), продолжение 0011:
-- 1. Триггер ставит эру по умолчанию дашборда только новым датам: дата
--    появилась в строке (INSERT или колонка была NULL). Существующие даты при
--    других правках строки не трогаются: пустая эра при непустой эре по
--    умолчанию — «без эры», а не наша и не по умолчанию.
-- 2. Замена эры в датах (pd_era_matches, pd_with_era): дата с годом и эрой
--    old или дата из одной эры old получает эру new; дата из одной эры при
--    замене на пустую очищается (колонка NULL — без эры она пустая).

-- +goose Up
-- +goose StatementBegin
-- Триггер: новым датам с годом и пустой эрой — эра по умолчанию дашборда
-- (F-143), если она непустая. «Новая» — INSERT или колонка была NULL.
-- Колонки-даты — аргументы триггера. Меняются только эти колонки строки:
-- jsonb_populate_record берёт остальные из NEW как есть.
CREATE OR REPLACE FUNCTION pd_apply_default_era() RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    def   text := pd_dashboard_default_era(NEW.dashboard_id);
    row_j jsonb;
    old_j jsonb;
    patch jsonb := '{}';
    col   text;
BEGIN
    IF def IS NULL THEN
        RETURN NEW;
    END IF;
    row_j := to_jsonb(NEW);
    IF TG_OP = 'UPDATE' THEN
        old_j := to_jsonb(OLD);
    END IF;
    FOREACH col IN ARRAY TG_ARGV LOOP
        IF jsonb_typeof(row_j -> col) = 'object' AND row_j -> col ->> 'y' IS NOT NULL
           AND row_j -> col ->> 'e' IS NULL
           AND (old_j IS NULL OR jsonb_typeof(old_j -> col) IS DISTINCT FROM 'object') THEN
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

-- +goose StatementBegin
-- Дата попадает под замену эры old (NULL — пустая): с годом и эрой old или
-- из одной эры old (F-143).
CREATE FUNCTION pd_era_matches(p partial_date, old text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN p::text IS NOT NULL AND (p).e IS NOT DISTINCT FROM old
    AND ((p).y IS NOT NULL OR ((p).e IS NOT NULL AND (p).m IS NULL AND (p).d IS NULL));
-- +goose StatementEnd

-- +goose StatementBegin
-- Дата с эрой new (NULL — пустая); дата из одной эры без эры — пустая (NULL).
CREATE FUNCTION pd_with_era(p partial_date, new text) RETURNS partial_date
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN CASE
    WHEN p::text IS NULL THEN NULL
    WHEN (p).y IS NULL AND (p).m IS NULL AND (p).d IS NULL AND new IS NULL THEN NULL
    ELSE ROW((p).y, (p).m, (p).d, new)::partial_date_parts::partial_date
    END;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION pd_with_era(partial_date, text);
DROP FUNCTION pd_era_matches(partial_date, text);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION pd_apply_default_era() RETURNS trigger
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
