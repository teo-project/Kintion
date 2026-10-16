// Package i18n — языковые файлы интерфейса (В-8, Д-16): по одному JSON на
// язык, общий набор для сервера и фронтенда. Язык — на инстанс (F-38).
// systemFields — запасные подписи системных полей, если пака или шаблона нет:
// у всего, что есть в таблицах БД, должно быть название в каждом языке.
package i18n

import (
	"embed"
	"encoding/json"
	"io/fs"
	"slices"
	"strings"
)

//go:embed *.json
var files embed.FS

// Languages возвращает коды языков, для которых есть файл.
func Languages() []string {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		panic(err) // встроенная ФС
	}
	var out []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".json"); ok {
			out = append(out, name)
		}
	}
	return out
}

// Has сообщает, есть ли языковой файл для lang.
func Has(lang string) bool {
	return slices.Contains(Languages(), lang)
}

// File возвращает содержимое языкового файла.
func File(lang string) ([]byte, error) {
	return files.ReadFile(lang + ".json")
}

// Lookup — строка языкового файла по пути через точку ("dashboard.defaultName").
// Нет строки — пустая строка.
func Lookup(lang, path string) string {
	data, err := File(lang)
	if err != nil {
		return ""
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return ""
	}
	for part := range strings.SplitSeq(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		v = m[part]
	}
	s, _ := v.(string)
	return s
}
