package db_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"kintion/server/internal/db"
	"kintion/server/internal/db/dbtest"
	"kintion/server/internal/db/sqlc"
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
