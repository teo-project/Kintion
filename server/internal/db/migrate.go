package db

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"kintion/server/internal/db/sqlc"
)

// Миграции (Д-3): SQL-файлы goose, встроены в бинарник, применяются при старте.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

func migrations() fs.FS {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		panic(err) // путь задан константой
	}
	return sub
}

// SchemaVersion — номер последней миграции, известной приложению.
func SchemaVersion() int64 {
	names, err := fs.Glob(migrations(), "*.sql")
	if err != nil {
		panic(err) // шаблон задан константой
	}
	var last int64
	for _, n := range names {
		v, err := goose.NumericComponent(n)
		if err != nil {
			panic(fmt.Sprintf("миграция %s: %v", n, err))
		}
		last = max(last, v)
	}
	return last
}

// Migrator применяет и откатывает миграции.
type Migrator struct {
	pool       *pgxpool.Pool
	appVersion string
	log        *slog.Logger
}

// NewMigrator создаёт мигратор. appVersion пишется в schema_info.
func NewMigrator(pool *pgxpool.Pool, appVersion string, log *slog.Logger) *Migrator {
	return &Migrator{pool: pool, appVersion: appVersion, log: log}
}

// MigrationStatus — состояние одной миграции.
type MigrationStatus struct {
	Version int64
	Name    string
	Applied bool
}

func (m *Migrator) withProvider(ctx context.Context, fn func(*goose.Provider) error) error {
	sqlDB := stdlib.OpenDBFromPool(m.pool)
	defer func() { _ = sqlDB.Close() }()

	// advisory lock: два экземпляра не применяют миграции одновременно
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations(), goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("миграции: %w", err)
	}
	return fn(p)
}

// Up применяет все новые миграции. Если схема в БД новее приложения — ошибка:
// старое приложение не должно работать с новой схемой.
func (m *Migrator) Up(ctx context.Context) error {
	return m.withProvider(ctx, func(p *goose.Provider) error {
		current, err := p.GetDBVersion(ctx)
		if err != nil {
			return fmt.Errorf("версия схемы: %w", err)
		}
		if latest := SchemaVersion(); current > latest {
			return fmt.Errorf("схема БД (%d) новее приложения (%d): обновите приложение", current, latest)
		}
		results, err := p.Up(ctx)
		if err != nil {
			return fmt.Errorf("миграции: %w", err)
		}
		if len(results) == 0 {
			m.log.InfoContext(ctx, "схема актуальна", slog.Int64("schema_version", current))
			return nil
		}
		for _, r := range results {
			m.log.InfoContext(ctx, "миграция применена",
				slog.Int64("version", r.Source.Version), slog.Duration("duration", r.Duration))
		}
		return m.recordVersion(ctx, p)
	})
}

// Down откатывает последнюю применённую миграцию.
func (m *Migrator) Down(ctx context.Context) error {
	return m.withProvider(ctx, func(p *goose.Provider) error {
		r, err := p.Down(ctx)
		if err != nil {
			if errors.Is(err, goose.ErrNoNextVersion) {
				m.log.InfoContext(ctx, "нечего откатывать")
				return nil
			}
			return fmt.Errorf("откат: %w", err)
		}
		m.log.InfoContext(ctx, "миграция откачена", slog.Int64("version", r.Source.Version))
		return m.recordVersion(ctx, p)
	})
}

// Status возвращает состояние всех известных миграций.
func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	var out []MigrationStatus
	err := m.withProvider(ctx, func(p *goose.Provider) error {
		st, err := p.Status(ctx)
		if err != nil {
			return err
		}
		for _, s := range st {
			out = append(out, MigrationStatus{
				Version: s.Source.Version,
				Name:    s.Source.Path,
				Applied: s.State == goose.StateApplied,
			})
		}
		return nil
	})
	return out, err
}

// recordVersion пишет текущую версию в schema_info (если таблица ещё есть).
func (m *Migrator) recordVersion(ctx context.Context, p *goose.Provider) error {
	v, err := p.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("версия схемы: %w", err)
	}
	if v == 0 {
		return nil // откат до пустой схемы: schema_info удалена
	}
	return sqlc.New(m.pool).UpsertSchemaInfo(ctx, sqlc.UpsertSchemaInfoParams{
		Version:    v,
		AppVersion: m.appVersion,
	})
}
