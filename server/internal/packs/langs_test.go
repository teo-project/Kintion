package packs_test

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"kintion/server/internal/packs"
)

// textKeys — ключи, значения которых человекочитаемы и переводятся.
var textKeys = map[string]bool{"name": true, "description": true, "string": true, "language": true}

// skeleton заменяет переводимые строки пустыми: остаются ключи, id, структура.
func skeleton(v any, parent string) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if _, ok := val.(string); ok && (textKeys[k] || (parent == "forms" || parent == "forms:") && k != "by" || parent == "period") {
				out[k] = ""
				continue
			}
			child := k
			if k == "forms" || parent == "forms:" {
				child = "forms:" // внутри forms все строки, кроме by, — слова
			}
			out[k] = skeleton(val, child)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = skeleton(val, parent)
		}
		return out
	}
	return v
}

func readSkeletons(t *testing.T, lang string) map[string]any {
	t.Helper()
	fsys, err := packs.Source("", lang)
	if err != nil {
		t.Fatal(err)
	}
	res := map[string]any{}
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".json" {
			return err
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Errorf("%s/%s: %v", lang, p, err)
		}
		res[p] = skeleton(dropLocalized(v), "")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// Языковые версии паков совпадают по ключам и структуре (F-54, F-57): различаются только слова.
func TestPackLanguagesMatch(t *testing.T) {
	enabled := []string{"kin.core", "kin.christian.orthodox"}
	for _, lang := range []string{"ru", "en"} {
		fsys, err := packs.Source("", lang)
		if err != nil {
			t.Fatal(err)
		}
		reg, ds := packs.Load(fsys, lang, enabled)
		if reg == nil {
			t.Fatalf("%s: пак не загружен", lang)
		}
		for _, d := range ds {
			if d.Level != packs.LevelInfo {
				t.Errorf("%s: %v", lang, d)
			}
		}
	}
	ru, en := readSkeletons(t, "ru"), readSkeletons(t, "en")
	if len(ru) == 0 {
		t.Fatal("нет файлов ru")
	}
	for p, a := range ru {
		b, ok := en[p]
		if !ok {
			t.Errorf("en: нет файла %s", p)
			continue
		}
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: структура ru и en различается", p)
		}
	}
	for p := range en {
		if _, ok := ru[p]; !ok {
			t.Errorf("en: лишний файл %s", p)
		}
	}
}

// dropLocalized убирает у полей с localized_options список вариантов, а у шаблона
// с таким полем — правило подписи (остаются ссылки на поля): форма юрлица в языках стоит по-разному.
func dropLocalized(v any) any {
	tm, ok := v.(map[string]any)
	if !ok {
		return v
	}
	fields, _ := tm["fields"].([]any)
	// поля только части языков (localized_field) не сравниваются
	kept := fields[:0:0]
	for _, f := range fields {
		if fm, ok := f.(map[string]any); ok && fm["localized_field"] == true {
			continue
		}
		kept = append(kept, f)
	}
	if fields != nil {
		tm["fields"] = kept
	}
	fields = kept
	found := false
	for _, f := range fields {
		if fm, ok := f.(map[string]any); ok && fm["localized_options"] == true {
			delete(fm, "options")
			found = true
		}
	}
	if found {
		// подпись: сравниваются только используемые поля, порядок и разделители — нет
		var refs []string
		title, _ := tm["title"].([]any)
		for _, e := range title {
			if em, ok := e.(map[string]any); ok {
				if f, ok := em["field"].(string); ok {
					refs = append(refs, f)
				}
			}
		}
		sort.Strings(refs)
		tm["title"] = refs
	}
	return v
}
