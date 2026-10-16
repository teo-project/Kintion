// Package dbtest — отдельная временная база PostgreSQL для интеграционных тестов.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnvURL — строка подключения к тестовому PostgreSQL (тулбокс задаёт её сам).
const EnvURL = "TEO_KIN_TEST_DATABASE_URL"

// EnvRequire — если задана, тесты с БД не пропускаются, а падают без EnvURL (тулбокс).
const EnvRequire = "TEO_KIN_TEST_REQUIRE_DB"

// New создаёт пустую базу и пул к ней; база удаляется после теста.
// Без TEO_KIN_TEST_DATABASE_URL тест пропускается.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv(EnvURL)
	if url == "" && os.Getenv(EnvRequire) != "" {
		t.Fatalf("%s не задан, а %s требует интеграционные тесты", EnvURL, EnvRequire)
	}
	if url == "" {
		t.Skipf("%s не задан: интеграционный тест с БД пропущен", EnvURL)
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("тестовая БД: %v", err)
	}
	var b [6]byte
	_, _ = rand.Read(b[:])
	name := "kintion_test_" + hex.EncodeToString(b[:])
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("CREATE DATABASE: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		_ = admin.Close(ctx)
	})
	return pool
}
