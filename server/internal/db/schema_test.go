package db_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"kintion/i18n"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/partialdate"
)

// Тест F-07: уникальны только технические вещи. Новый уникальный индекс —
// осознанно добавить сюда с объяснением.
func TestUniqueConstraintsAllowList(t *testing.T) {
	pool := migrated(t)
	rows, err := pool.Query(context.Background(), `
		SELECT c.relname, string_agg(a.attname, ',' ORDER BY k.ord)
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum, ord)
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum
		WHERE n.nspname = 'public' AND i.indisunique
		GROUP BY c.relname, i.indexrelid
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var table, cols string
		if err := rows.Scan(&table, &cols); err != nil {
			t.Fatal(err)
		}
		got = append(got, table+"("+cols+")")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	allowed := []string{
		// первичные ключи
		"account(id)", "session(id)", "bootstrap(token_hash)", "dashboard(id)", "pack(id)",
		"dashboard_template(id)", "entity(id)", "grp(id)", "participation(id)", "field_entry(id)",
		"entry_link(id)", "file(id)", "audit_log(id)", "schema_info(singleton)", "goose_db_version(id)",
		"entity_group(entity_id,group_id)", "field_rev(entity_id,field_key)", "grp_link(parent_id,child_id)",
		// (dashboard_id, id) — цели составных FK (F-01)
		"dashboard_template(dashboard_id,id)", "entity(dashboard_id,id)", "grp(dashboard_id,id)",
		"field_entry(dashboard_id,id)",
		// технические
		"account(login)", "session(token_hash)", "file(dashboard_id,sha256)",
	}
	for _, g := range got {
		if !slices.Contains(allowed, g) {
			t.Errorf("unique index not in allow list (F-07): %s", g)
		}
	}
}

// F-76: у каждой таблицы и колонки есть название во всех языковых файлах.
func TestSchemaNamesInLanguageFiles(t *testing.T) {
	pool := migrated(t)
	rows, err := pool.Query(context.Background(), `
		SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name <> 'goose_db_version'
		ORDER BY 1, ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	cols := map[string][]string{}
	for rows.Next() {
		var tbl, col string
		if err := rows.Scan(&tbl, &col); err != nil {
			t.Fatal(err)
		}
		cols[tbl] = append(cols[tbl], col)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, lang := range i18n.Languages() {
		data, err := i18n.File(lang)
		if err != nil {
			t.Fatal(err)
		}
		var f struct {
			Schema map[string]map[string]string `json:"schema"`
		}
		if err := json.Unmarshal(data, &f); err != nil {
			t.Fatal(err)
		}
		for tbl, cs := range cols {
			names := f.Schema[tbl]
			if names["_table"] == "" {
				t.Errorf("%s.json: no name for table %s", lang, tbl)
			}
			for _, c := range cs {
				if names[c] == "" {
					t.Errorf("%s.json: no name for column %s.%s", lang, tbl, c)
				}
			}
		}
	}
}

// fixture — два дашборда с записями, шаблонами, группами и значениями.
type fixture struct {
	pool                *pgxpool.Pool
	account             uuid.UUID
	dash                [2]uuid.UUID
	tmpl, grp, a, b, fe [2]uuid.UUID
}

func newID() uuid.UUID { return uuid.Must(uuid.NewV7()) }

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) error {
	t.Helper()
	_, err := pool.Exec(context.Background(), sql, args...)
	return err
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{pool: migrated(t), account: newID()}
	p := f.pool
	must(t, exec(t, p, "INSERT INTO account (id, login, password_hash) VALUES ($1, 'owner', 'x')", f.account))
	for i := range 2 {
		f.dash[i], f.tmpl[i], f.grp[i], f.a[i], f.b[i], f.fe[i] = newID(), newID(), newID(), newID(), newID(), newID()
		must(t, exec(t, p, "INSERT INTO dashboard (id, owner_id, name) VALUES ($1, $2, 'd')", f.dash[i], f.account))
		must(t, exec(t, p, `INSERT INTO dashboard_template (id, dashboard_id, pack_id, template_key, entity, pack_version)
			VALUES ($1, $2, 'p', 't', 'e', '1')`, f.tmpl[i], f.dash[i]))
		must(t, exec(t, p, "INSERT INTO grp (id, dashboard_id, name, sort_key) VALUES ($1, $2, 'g', 'a')", f.grp[i], f.dash[i]))
		for _, e := range []uuid.UUID{f.a[i], f.b[i]} {
			must(t, exec(t, p, "INSERT INTO entity (id, dashboard_id, entity, template_id) VALUES ($1, $2, 'e', $3)", e, f.dash[i], f.tmpl[i]))
		}
		must(t, exec(t, p, `INSERT INTO field_entry (id, dashboard_id, entity_id, field_key, sort_key, v_text)
			VALUES ($1, $2, $3, 'k', 'a', 'v')`, f.fe[i], f.dash[i], f.a[i]))
	}
	return f
}

func isFKViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// Тест F-01: ссылка через границу дашборда отклоняется БД.
func TestCrossDashboardRejected(t *testing.T) {
	f := newFixture(t)
	p := f.pool
	d0, a0, b1 := f.dash[0], f.a[0], f.b[1]
	cases := map[string]func() error{
		"participation container": func() error {
			return exec(t, p, `INSERT INTO participation (id, dashboard_id, container_id, entity_id, role, sort_key)
				VALUES ($1, $2, $3, $4, 'r', 'a')`, newID(), d0, b1, a0)
		},
		"participation entity": func() error {
			return exec(t, p, `INSERT INTO participation (id, dashboard_id, container_id, entity_id, role, sort_key)
				VALUES ($1, $2, $3, $4, 'r', 'a')`, newID(), d0, a0, b1)
		},
		"field_entry v_ref": func() error {
			return exec(t, p, `INSERT INTO field_entry (id, dashboard_id, entity_id, field_key, sort_key, v_ref)
				VALUES ($1, $2, $3, 'k', 'b', $4)`, newID(), d0, a0, b1)
		},
		"field_entry entity": func() error {
			return exec(t, p, `INSERT INTO field_entry (id, dashboard_id, entity_id, field_key, sort_key)
				VALUES ($1, $2, $3, 'k', 'b')`, newID(), d0, b1)
		},
		"entity start_link": func() error {
			return exec(t, p, "UPDATE entity SET start_link = $1 WHERE id = $2", b1, a0)
		},
		"entity end_link": func() error {
			return exec(t, p, "UPDATE entity SET end_link = $1 WHERE id = $2", b1, a0)
		},
		"entry_link target": func() error {
			return exec(t, p, "INSERT INTO entry_link (id, dashboard_id, entry_id, target_id, anchor) VALUES ($1, $2, $3, $4, 'start')",
				newID(), d0, f.fe[0], b1)
		},
		"entry_link entry": func() error {
			return exec(t, p, "INSERT INTO entry_link (id, dashboard_id, entry_id, target_id, anchor) VALUES ($1, $2, $3, $4, 'start')",
				newID(), d0, f.fe[1], a0)
		},
		"entity_group group": func() error {
			return exec(t, p, "INSERT INTO entity_group (dashboard_id, entity_id, group_id) VALUES ($1, $2, $3)", d0, a0, f.grp[1])
		},
		"entity_group entity": func() error {
			return exec(t, p, "INSERT INTO entity_group (dashboard_id, entity_id, group_id) VALUES ($1, $2, $3)", d0, b1, f.grp[0])
		},
		"entity template": func() error {
			return exec(t, p, "INSERT INTO entity (id, dashboard_id, entity, template_id) VALUES ($1, $2, 'e', $3)", newID(), d0, f.tmpl[1])
		},
		"group link parent": func() error {
			return exec(t, p, "INSERT INTO grp_link (dashboard_id, parent_id, child_id) VALUES ($1, $2, $3)", d0, f.grp[1], f.grp[0])
		},
		"group link child": func() error {
			return exec(t, p, "INSERT INTO grp_link (dashboard_id, parent_id, child_id) VALUES ($1, $2, $3)", d0, f.grp[0], f.grp[1])
		},
	}
	for name, fn := range cases {
		if err := fn(); !isFKViolation(err) {
			t.Errorf("%s: cross-dashboard link not rejected by FK: %v", name, err)
		}
	}
	// внутри дашборда те же ссылки допустимы
	must(t, exec(t, p, "UPDATE entity SET start_link = $1, end_link = $1 WHERE id = $2", f.b[0], a0))
	must(t, exec(t, p, "INSERT INTO entity_group (dashboard_id, entity_id, group_id) VALUES ($1, $2, $3)", d0, a0, f.grp[0]))
}

func count(t *testing.T, p *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	must(t, p.QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}

// Поведение ссылок при очистке записи (§4): CASCADE / SET NULL, журнал остаётся.
func TestPurgeReferenceBehavior(t *testing.T) {
	f := newFixture(t)
	p := f.pool
	d, a, b := f.dash[0], f.a[0], f.b[0]
	ev := newID()
	must(t, exec(t, p, "INSERT INTO entity (id, dashboard_id, entity, template_id) VALUES ($1, $2, 'e', $3)", ev, d, f.tmpl[0]))
	// a участвует в b; b — контейнер с участником ev
	must(t, exec(t, p, `INSERT INTO participation (id, dashboard_id, container_id, entity_id, role, sort_key)
		VALUES ($1, $2, $3, $4, 'r', 'a'), ($5, $2, $4, $3, 'r', 'a')`, newID(), d, b, a, newID()))
	// значение b ссылается на a; привязка значения b к a
	refEntry := newID()
	must(t, exec(t, p, `INSERT INTO field_entry (id, dashboard_id, entity_id, field_key, sort_key, v_ref)
		VALUES ($1, $2, $3, 'ref', 'a', $4)`, refEntry, d, b, a))
	must(t, exec(t, p, "INSERT INTO field_entry (id, dashboard_id, entity_id, field_key, sort_key) VALUES ($1, $2, $3, 'x', 'a')", ev, d, b))
	must(t, exec(t, p, "INSERT INTO entry_link (id, dashboard_id, entry_id, target_id, anchor) VALUES ($1, $2, $3, $4, 'end')", newID(), d, ev, a))
	// b начинается событием a
	must(t, exec(t, p, "UPDATE entity SET start_link = $1, end_link = $1, start_date = '(2021,,)' WHERE id = $2", a, b))
	must(t, exec(t, p, "INSERT INTO entity_group (dashboard_id, entity_id, group_id) VALUES ($1, $2, $3)", d, a, f.grp[0]))
	must(t, exec(t, p, "INSERT INTO field_rev (entity_id, field_key, rev) VALUES ($1, 'k', 3)", a))
	must(t, exec(t, p, `INSERT INTO audit_log (id, dashboard_id, operation_id, source, app_version, schema_version, entity_id, object, object_id, op)
		VALUES ($1, $2, $3, 'ui', 't', 3, $4, 'entity', $4, 'create')`, newID(), d, newID(), a))

	// мягкое удаление ничего не трогает
	must(t, exec(t, p, "UPDATE entity SET deleted_at = now() WHERE id = $1", a))
	if n := count(t, p, "SELECT count(*) FROM participation WHERE entity_id = $1 OR container_id = $1", a); n != 2 {
		t.Fatalf("soft delete changed participation: %d", n)
	}

	// очистка
	must(t, exec(t, p, "DELETE FROM entity WHERE id = $1", a))
	checks := map[string]string{
		"own values":           "SELECT count(*) FROM field_entry WHERE entity_id = $1",
		"participation":        "SELECT count(*) FROM participation WHERE entity_id = $1 OR container_id = $1",
		"v_ref values":         "SELECT count(*) FROM field_entry WHERE v_ref = $1",
		"entry links":          "SELECT count(*) FROM entry_link WHERE target_id = $1",
		"group labels":         "SELECT count(*) FROM entity_group WHERE entity_id = $1",
		"field revisions":      "SELECT count(*) FROM field_rev WHERE entity_id = $1",
		"start/end link to it": "SELECT count(*) FROM entity WHERE start_link = $1 OR end_link = $1",
	}
	for name, q := range checks {
		if n := count(t, p, q, a); n != 0 {
			t.Errorf("%s: %d rows left after purge", name, n)
		}
	}
	if n := count(t, p, "SELECT count(*) FROM entity WHERE id = $1 AND (start_date).y = 2021 AND dashboard_id = $2", b, d); n != 1 {
		t.Error("start date or dashboard of the other record lost on SET NULL")
	}
	if n := count(t, p, "SELECT count(*) FROM field_entry WHERE id = $1", ev); n != 1 {
		t.Error("other record's own value removed")
	}
	if n := count(t, p, "SELECT count(*) FROM audit_log WHERE entity_id = $1", a); n != 1 {
		t.Error("audit log of purged record lost")
	}
}

func TestAuditLogAppendOnly(t *testing.T) {
	f := newFixture(t)
	p := f.pool
	id := newID()
	must(t, exec(t, p, `INSERT INTO audit_log (id, dashboard_id, operation_id, source, app_version, schema_version, object, op)
		VALUES ($1, $2, $3, 'ui', 't', 3, 'dashboard', 'create')`, id, f.dash[0], newID()))
	for _, q := range []string{
		"UPDATE audit_log SET op = 'x' WHERE id = $1",
		"DELETE FROM audit_log WHERE id = $1",
	} {
		if err := exec(t, p, q, id); err == nil || !strings.Contains(err.Error(), "только вставка") {
			t.Errorf("%s: %v", q, err)
		}
	}
	if err := exec(t, p, "TRUNCATE audit_log"); err == nil {
		t.Error("TRUNCATE allowed")
	}
	if n := count(t, p, "SELECT count(*) FROM audit_log"); n != 1 {
		t.Fatalf("audit_log rows: %d", n)
	}
}

func TestFieldEntryPartialDate(t *testing.T) {
	f := newFixture(t)
	q := sqlc.New(f.pool)
	ctx := context.Background()
	d := partialdate.Date{M: 2, D: 29}
	from := partialdate.Date{Y: 1985}
	row, err := q.InsertFieldEntry(ctx, sqlc.InsertFieldEntryParams{
		ID: newID(), DashboardID: f.dash[0], EntityID: f.a[0], FieldKey: "k", SortKey: "b",
		VDate: &d, ValidFrom: &from, Meta: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.VDate == nil || *row.VDate != d || row.ValidTo != nil || *row.ValidFrom != from {
		t.Fatalf("row %+v", row)
	}
	list, err := q.ListFieldEntries(ctx, f.a[0])
	if err != nil || len(list) != 2 {
		t.Fatalf("list %v %v", list, err)
	}
	// пустая и недопустимая даты отклоняются колонкой
	for _, lit := range []string{"(,,)", "(2023,2,29)", "(,4,31)"} {
		if err := exec(t, f.pool, `INSERT INTO field_entry (id, dashboard_id, entity_id, field_key, sort_key, v_date)
			VALUES ($1, $2, $3, 'k', 'c', $4::partial_date)`, newID(), f.dash[0], f.a[0], lit); err == nil {
			t.Errorf("v_date %s accepted", lit)
		}
	}
}
