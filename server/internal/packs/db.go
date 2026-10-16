package packs

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	packstorage "kintion/pack-storage"
	"kintion/server/internal/db/sqlc"
)

// Source — откуда читать паки языка lang: с диска (TEO_KIN_PACKS_PATH,
// раскладка pack-storage: <path>/<язык>/<пак>/…, Д-5) или встроенные.
func Source(packsPath, lang string) (fs.FS, error) {
	if packsPath == "" {
		fsys, err := packstorage.Lang(lang)
		if err != nil {
			return nil, fmt.Errorf("встроенных паков языка %q нет", lang)
		}
		return fsys, nil
	}
	dir := filepath.Join(packsPath, lang)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("TEO_KIN_PACKS_PATH: нет папки %s", dir)
	}
	return os.DirFS(dir), nil
}

// Sync отмечает в таблице pack загруженные паки и снимает отметку с остальных.
func Sync(ctx context.Context, pool *pgxpool.Pool, reg *Registry) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		ids := make([]string, 0, len(reg.Packs))
		for _, m := range reg.Packs {
			ids = append(ids, m.ID)
			if err := q.UpsertPack(ctx, sqlc.UpsertPackParams{ID: m.ID, Version: m.Version, Language: m.Language}); err != nil {
				return fmt.Errorf("pack %s: %w", m.ID, err)
			}
		}
		return q.MarkPacksUnloaded(ctx, ids)
	})
}
