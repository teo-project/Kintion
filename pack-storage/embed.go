// Package packstorage — встроенные паки (pack-storage/<язык>/<пак>/…), F-58.
package packstorage

import (
	"embed"
	"io/fs"
)

//go:embed ru en
var files embed.FS

// Lang возвращает паки одного языка: корень — папки паков.
func Lang(lang string) (fs.FS, error) {
	if _, err := fs.Stat(files, lang); err != nil {
		return nil, err
	}
	return fs.Sub(files, lang)
}

// DefaultPacks — включённые паки по умолчанию (TEO_KIN_PACKS, Д-31).
const DefaultPacks = "kin.core"
