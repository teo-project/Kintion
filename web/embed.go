// Package web — собранный фронтенд (web/dist), встроенный в бинарник.
package web

import (
	"embed"
	"io/fs"
)

// dist содержит результат `npm run build`. Без сборки в нём только .gitkeep,
// и сервер отвечает, что интерфейс не собран.
//
//go:embed all:dist
var dist embed.FS

// licenses — тексты лицензий сторонних библиотек (F-91). Не в dist: файл
// отдаётся только вошедшим (GET /api/licenses), а не публичной статикой.
//
//go:embed licenses/THIRD_PARTY_LICENSES.txt
var licenses []byte

// Licenses возвращает тексты лицензий сторонних библиотек.
func Licenses() []byte { return licenses }

// Dist возвращает содержимое web/dist.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // путь задан константой
	}
	return sub
}
