package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"kintion/server/internal/config"
)

// Ошибка в паке — приложение не стартует (план §3 п.2).
func TestLoadPacksRefusesErrors(t *testing.T) {
	dir := t.TempDir()
	pack := filepath.Join(dir, "xx", "p")
	if err := os.MkdirAll(pack, 0o750); err != nil {
		t.Fatal(err)
	}
	manifest := `{"format":"kintion-pack/0","id":"p","version":"1","language":"xx","files":["entities/missing.json"]}`
	if err := os.WriteFile(filepath.Join(pack, "p.pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Defaults()
	cfg.Language, cfg.PacksPath, cfg.Packs = "xx", dir, "p"
	if _, err := loadPacks(cfg, log); err == nil {
		t.Fatal("broken pack accepted")
	}

	// встроенные паки по умолчанию загружаются
	if _, err := loadPacks(config.Defaults(), log); err != nil {
		t.Fatal(err)
	}
}
