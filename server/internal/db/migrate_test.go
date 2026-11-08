package db_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"kintion/server/internal/db"
	"kintion/server/internal/db/dbtest"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/partialdate"
)

func TestSchemaVersion(t *testing.T) {
	if db.SchemaVersion() < 1 {
		t.Fatalf("no embedded migrations: %d", db.SchemaVersion())
	}
}

func TestMigrateUpDown(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := db.NewMigrator(pool, "test-1", log)
	q := sqlc.New(pool)

	if err := m.Up(ctx); err != nil {
		t.Fatal(err)
	}
	info, err := q.GetSchemaInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != db.SchemaVersion() || info.AppVersion != "test-1" {
		t.Fatalf("schema_info %+v", info)
	}

	// повторный запуск — без изменений
	if err := db.NewMigrator(pool, "test-2", log).Up(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := q.GetSchemaInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again != info {
		t.Fatalf("second Up changed schema_info: %+v → %+v", info, again)
	}

	st, err := m.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range st {
		if !s.Applied {
			t.Fatalf("not applied: %+v", s)
		}
	}

	// откат до пустой схемы
	for range db.SchemaVersion() {
		if err := m.Down(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('schema_info') IS NOT NULL").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("schema_info must be dropped after full down")
	}
	if err := m.Down(ctx); err != nil {
		t.Fatalf("down on empty schema: %v", err)
	}

	// и снова вверх
	if err := m.Up(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateRefusesNewerSchema(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	m := db.NewMigrator(pool, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := m.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		"INSERT INTO goose_db_version (version_id, is_applied) VALUES ($1, true)", db.SchemaVersion()+1000); err != nil {
		t.Fatal(err)
	}
	if err := m.Up(ctx); err == nil {
		t.Fatal("Up must refuse a schema newer than the app")
	}
}

// 0009 (F-14): даты из старого типа (y, m, d smallint) переходят в новый
// (y integer, m, d, e text) как есть, эра — пустая; откат возвращает их.
func TestMigratePartialDateEra(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := db.NewMigrator(pool, "test", log)
	if err := m.Up(ctx); err != nil {
		t.Fatal(err)
	}
	for range db.SchemaVersion() - 8 {
		if err := m.Down(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO account (id, login, password_hash) VALUES ('00000000-0000-7000-8000-000000000001', 'a', 'x');
		INSERT INTO dashboard (id, owner_id, name) VALUES ('00000000-0000-7000-8000-000000000002', '00000000-0000-7000-8000-000000000001', 'd');
		INSERT INTO dashboard_template (id, dashboard_id, pack_id, template_key, entity, pack_version)
			VALUES ('00000000-0000-7000-8000-000000000003', '00000000-0000-7000-8000-000000000002', 'p', 't', 'e', '1');
		INSERT INTO entity (id, dashboard_id, entity, template_id, start_date, end_date)
			VALUES ('00000000-0000-7000-8000-000000000004', '00000000-0000-7000-8000-000000000002', 'e',
			        '00000000-0000-7000-8000-000000000003', '(1985,3,12)', NULL);
		INSERT INTO field_entry (id, dashboard_id, entity_id, field_key, sort_key, v_date, valid_from)
			VALUES ('00000000-0000-7000-8000-000000000005', '00000000-0000-7000-8000-000000000002',
			        '00000000-0000-7000-8000-000000000004', 'k', 'a', '(,2,29)', '(2000,,)')`); err != nil {
		t.Fatal(err)
	}
	if err := m.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var start, vdate, from partialdate.Date
	var end *partialdate.Date
	if err := pool.QueryRow(ctx, `SELECT e.start_date, e.end_date, f.v_date, f.valid_from
		FROM entity e JOIN field_entry f ON f.entity_id = e.id`).Scan(&start, &end, &vdate, &from); err != nil {
		t.Fatal(err)
	}
	if start != (partialdate.Date{Y: 1985, M: 3, D: 12}) || end != nil || vdate != (partialdate.Date{M: 2, D: 29}) || from != (partialdate.Date{Y: 2000}) {
		t.Fatalf("after 0009: %v %v %v %v", start, end, vdate, from)
	}
	// дата с эрой не даёт откатить 0009 без потерь
	if _, err := pool.Exec(ctx, `UPDATE entity SET end_date = '(-500,,,AC)'`); err != nil {
		t.Fatal(err)
	}
	for range db.SchemaVersion() - 9 {
		if err := m.Down(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Down(ctx); err == nil {
		t.Fatal("down of 0009 with an era succeeded")
	}
}
