package api_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// withPartners — запись шаблона tmpl с двумя новыми людьми в роли partner.
func withPartners(t *testing.T, c *client, dash, tmpl string) string {
	t.Helper()
	return created(t, batch(t, c, dash, map[string]any{
		"records": []any{
			map[string]any{"ref": "w", "template": tmpl},
			map[string]any{"ref": "a", "entity": "kin.core.person"},
			map[string]any{"ref": "b", "entity": "kin.core.person"},
		},
		"participants": []any{
			map[string]any{"container": "w", "entity": "a", "role": "partner"},
			map[string]any{"container": "w", "entity": "b", "role": "partner"},
		},
	}, http.StatusCreated), "w")
}

// wedding — событие для привязок.
func wedding(t *testing.T, c *client, dash string) string {
	t.Helper()
	return withPartners(t, c, dash, "kin.core.wedding")
}

// values — значения поля key записи в порядке показа.
func values(rec map[string]any, key string) []map[string]any {
	fields, _ := rec["fields"].(map[string]any)
	f, _ := fields[key].(map[string]any)
	list, _ := f["values"].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, v := range list {
		out = append(out, v.(map[string]any))
	}
	return out
}

func fieldRev(rec map[string]any, key string) float64 {
	fields, _ := rec["fields"].(map[string]any)
	f, _ := fields[key].(map[string]any)
	v, _ := f["rev"].(float64)
	return v
}

// addValue добавляет значение и возвращает id нового (по отличию от прежних).
func addValue(t *testing.T, c *client, id, key string, body map[string]any) (string, map[string]any) {
	t.Helper()
	before := map[string]bool{}
	for _, v := range values(record(c.do("GET", "/api/records/"+id, nil)), key) {
		before[v["id"].(string)] = true
	}
	r := c.do("POST", "/api/records/"+id+"/fields/"+key+"/values", body)
	expect(t, r, http.StatusOK, "add "+key)
	for _, v := range values(record(r), key) {
		if !before[v["id"].(string)] {
			return v["id"].(string), record(r)
		}
	}
	t.Fatalf("no new value in %v", record(r))
	return "", nil
}

func order(rec map[string]any, key string, names map[string]string) string {
	s := ""
	for _, v := range values(rec, key) {
		s += names[v["id"].(string)] + " "
	}
	return s
}

// Сценарий «фамилии Лизы» (fields-and-history.md).
func TestSurnamesOfLiza(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	liza := person(t, c, dash, nil)
	first, second := wedding(t, c, dash), wedding(t, c, dash)

	e1, _ := addValue(t, c, liza, "surname", map[string]any{"if_rev": 0, "value": "Василькова",
		"links": []any{map[string]any{"target": second, "anchor": "start"}}})
	e2, _ := addValue(t, c, liza, "surname", map[string]any{"if_rev": 1, "value": "Давыдко", "position": "bottom", "note": "девичья"})
	e3, _ := addValue(t, c, liza, "surname", map[string]any{"if_rev": 2, "value": "Краник", "after_id": e1,
		"links": []any{map[string]any{"target": first, "anchor": "start"}}})
	e4, rec := addValue(t, c, liza, "surname", map[string]any{"if_rev": 3, "value": "Давыдко", "after_id": e1})
	names := map[string]string{e1: "e1", e2: "e2", e3: "e3", e4: "e4"}
	if got := order(rec, "surname", names); got != "e1 e4 e3 e2 " {
		t.Fatalf("order %s", got)
	}
	if fieldRev(rec, "surname") != 4 {
		t.Fatalf("rev %v", fieldRev(rec, "surname"))
	}
	// «сделать основным» — перенос наверх
	r := c.do("PATCH", "/api/records/"+liza+"/fields/surname/values/"+e2, map[string]any{"if_rev": 4, "position": "top"})
	expect(t, r, http.StatusOK, "make primary")
	if got := order(record(r), "surname", names); got != "e2 e1 e4 e3 " {
		t.Fatalf("order after move %s", got)
	}
	// изменение: значение, пометка, привязки
	r = c.do("PATCH", "/api/records/"+liza+"/fields/surname/values/"+e3, map[string]any{"if_rev": 5, "note": "первый брак",
		"links": []any{map[string]any{"target": first, "anchor": "start"}, map[string]any{"target": second, "anchor": "end"}}})
	expect(t, r, http.StatusOK, "patch")
	for _, v := range values(record(r), "surname") {
		if v["id"] == e3 && (v["note"] != "первый брак" || len(v["links"].([]any)) != 2 || v["value"] != "Краник") {
			t.Fatalf("patched %v", v)
		}
	}
	// удаление
	r = c.do("DELETE", "/api/records/"+liza+"/fields/surname/values/"+e2+"?if_rev=6", nil)
	expect(t, r, http.StatusOK, "delete")
	if got := order(record(r), "surname", names); got != "e1 e4 e3 " {
		t.Fatalf("order after delete %s", got)
	}
	// журнал: 4 добавления, перемещение, изменение, удаление
	var ops []string
	rows, err := e.pool.Query(context.Background(),
		"SELECT op FROM audit_log WHERE entity_id = $1 AND object = 'field_entry' ORDER BY at, id", liza)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var op string
		_ = rows.Scan(&op)
		ops = append(ops, op)
	}
	if fmt.Sprint(ops) != "[create create create create move update delete]" {
		t.Fatalf("audit %v", ops)
	}
}

func TestFieldConflicts(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	id := person(t, c, dash, nil)
	// две вкладки, у обеих rev 0: разные поля — без конфликта
	addValue(t, c, id, "name", map[string]any{"if_rev": 0, "value": "Лиза"})
	addValue(t, c, id, "surname", map[string]any{"if_rev": 0, "value": "Василькова"})
	// одно поле — 409 и текущее состояние поля
	r := c.do("POST", "/api/records/"+id+"/fields/name/values", map[string]any{"if_rev": 0, "value": "Елизавета"})
	expect(t, r, http.StatusConflict, "same field")
	cur, _ := r.body["current"].(map[string]any)
	list, _ := cur["values"].([]any)
	if r.body["field"] != "name" || cur["rev"] != float64(1) || len(list) != 1 || list[0].(map[string]any)["value"] != "Лиза" {
		t.Fatalf("conflict %v", r.body)
	}
	expect(t, c.do("POST", "/api/records/"+id+"/fields/name/values", map[string]any{"value": "x"}), http.StatusBadRequest, "no if_rev")
}

func TestFieldRules(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	// неизвестный ключ meta — ошибка поля, запись и остальные поля сохранены (F-15)
	r := batch(t, c, dash, map[string]any{"records": []any{map[string]any{
		"ref": "p", "entity": "kin.core.person",
		"fields": map[string]any{
			"name":     []any{map[string]any{"value": "Лиза", "meta": map[string]any{"bColor": "#e57373"}}},
			"surname":  []any{map[string]any{"value": "Василькова", "meta": map[string]any{"unknown": 1}}},
			"nickname": []any{map[string]any{"value": "Лиз"}, map[string]any{"value": "Лиса"}},
			"gender":   []any{map[string]any{"value": "female"}},
		},
	}}}, http.StatusCreated)
	errs, _ := r.body["errors"].([]any)
	if len(errs) != 1 || errs[0].(map[string]any)["field"] != "surname" {
		t.Fatalf("errors %v", r.body["errors"])
	}
	id := created(t, r, "p")
	rec := record(c.do("GET", "/api/records/"+id, nil))
	if len(values(rec, "name")) != 1 || len(values(rec, "surname")) != 0 || len(values(rec, "nickname")) != 2 {
		t.Fatalf("fields %v", rec["fields"])
	}
	golden(t, "record_fields", map[string]any{"record": rec})

	path := "/api/records/" + id + "/fields/"
	for _, tc := range []struct {
		key  string
		body map[string]any
	}{
		{"gender", map[string]any{"if_rev": 0, "value": "unknown"}},                            // нет варианта
		{"name", map[string]any{"if_rev": 0, "value": "  "}},                                   // пустое
		{"name", map[string]any{"if_rev": 0, "value": nil}},                                    // нет значения (F-42)
		{"photo", map[string]any{"if_rev": 0, "value": "not-a-hash"}},                          // не sha256
		{"no_such", map[string]any{"if_rev": 0, "value": "x"}},                                 // нет поля
		{"stage", map[string]any{"if_rev": 0, "value": "marriage"}},                            // у человека нет этапов
		{"name", map[string]any{"if_rev": 0, "value": "x", "meta": map[string]any{"crop": 1}}}, // чужой meta
		{"name", map[string]any{"if_rev": 0, "value": "x", "links": []any{map[string]any{"target": id, "anchor": "middle"}}}},
	} {
		r := c.do("POST", path+tc.key+"/values", tc.body)
		expect(t, r, http.StatusBadRequest, tc.key)
		if r.body["field"] != tc.key {
			t.Fatalf("%s: %v", tc.key, r.body)
		}
	}
	// поле без истории и multiple — одно значение
	w := wedding(t, c, dash)
	addValue(t, c, w, "place", map[string]any{"if_rev": 0, "value": "Пенза", "meta": map[string]any{"city": "Пенза"}})
	expect(t, c.do("POST", "/api/records/"+w+"/fields/place/values", map[string]any{"if_rev": 1, "value": "Тула"}),
		http.StatusBadRequest, "single value")
}

func TestStageAndCustomFields(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	pair := withPartners(t, c, dash, "kin.core.relationship")
	// этап — ключ этапа шаблона с его meta или свободный текст без meta (F-37, F-54)
	addValue(t, c, pair, "stage", map[string]any{"if_rev": 0, "value": "dating"})
	_, rec := addValue(t, c, pair, "stage", map[string]any{"if_rev": 1, "value": "marriage", "meta": map[string]any{"registration_place": "ЗАГС"}})
	if v := values(rec, "stage"); len(v) != 2 || v[0]["value"] != "marriage" {
		t.Fatalf("stages %v", v)
	}
	expect(t, c.do("POST", "/api/records/"+pair+"/fields/stage/values",
		map[string]any{"if_rev": 2, "value": "dating", "meta": map[string]any{"registration_place": "x"}}), http.StatusBadRequest, "stage meta")
	addValue(t, c, pair, "stage", map[string]any{"if_rev": 2, "value": "Дружба"})

	// свободное поле: описание в записи, значения — custom.<key> (F-75)
	id := person(t, c, dash, nil)
	r := c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 0, "custom_fields": []any{
		map[string]any{"key": "army_unit", "type": "text", "name": "Воинская часть", "history": true, "section": "additional"},
		map[string]any{"key": "mentor", "type": "ref", "name": "Наставник"},
	}})
	expect(t, r, http.StatusOK, "custom fields")
	addValue(t, c, id, "custom.army_unit", map[string]any{"if_rev": 0, "value": "в/ч 1234"})
	// ref — видимая запись того же дашборда
	mentor := person(t, c, dash, nil)
	_, rec = addValue(t, c, id, "custom.mentor", map[string]any{"if_rev": 0, "value": mentor})
	if v := values(rec, "custom.mentor"); len(v) != 1 || v[0]["value"] != mentor {
		t.Fatalf("ref %v", v)
	}
	expect(t, c.do("POST", "/api/records/"+id+"/fields/custom.mentor/values", map[string]any{"if_rev": 1, "value": pair + "0"}),
		http.StatusBadRequest, "bad ref")
	expect(t, c.do("POST", "/api/records/"+id+"/fields/custom.other/values", map[string]any{"if_rev": 0, "value": "x"}),
		http.StatusBadRequest, "unknown custom")
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 1, "custom_fields": []any{
		map[string]any{"key": "mentor", "type": "ref", "name": "Наставник"},
	}}),
		http.StatusBadRequest, "remove used custom field")
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 1, "custom_fields": []any{
		map[string]any{"key": "Bad Key", "type": "text", "name": "x"},
	}}), http.StatusBadRequest, "bad key")
}

// Синтетический пак: те же механизмы на других ключах (план §3 п.7).
func TestFieldsOnSyntheticPack(t *testing.T) {
	e := newEnvAlt(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	r := batch(t, c, dash, map[string]any{"records": []any{map[string]any{
		"ref": "b", "entity": "test.alt.being",
		"fields": map[string]any{
			"given":  []any{map[string]any{"value": "Луна"}},
			"clan":   []any{map[string]any{"value": "Серебряные"}, map[string]any{"value": "Тёмные"}},
			"kind":   []any{map[string]any{"value": "moon"}},
			"signal": []any{map[string]any{"value": "+7"}},
		},
	}}}, http.StatusCreated)
	if errs, _ := r.body["errors"].([]any); len(errs) != 0 {
		t.Fatalf("errors %v", errs)
	}
	id := created(t, r, "b")
	rec := record(c.do("GET", "/api/records/"+id, nil))
	if v := values(rec, "clan"); len(v) != 2 || v[0]["value"] != "Серебряные" {
		t.Fatalf("clan %v", v)
	}
	expect(t, c.do("POST", "/api/records/"+id+"/fields/kind/values", map[string]any{"if_rev": 0, "value": "star"}),
		http.StatusBadRequest, "choice")
}

// F-96: свои поля — «выбор» с вариантами, «изображение», ссылка с
// ограничением сущностью; «контакта» нет; общее поле всем записям шаблона.
func TestCustomFieldTypesAndShared(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	id := person(t, c, dash, nil)
	other := person(t, c, dash, nil)
	pair := withPartners(t, c, dash, "kin.core.relationship")
	personEntity, _ := record(c.do("GET", "/api/records/"+id, nil))["entity"].(string)

	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 0, "custom_fields": []any{
		map[string]any{"key": "faith", "type": "choice", "name": "Вера",
			"options": []any{map[string]any{"key": "a", "name": "А"}, map[string]any{"key": "b", "name": "Б"}}},
		map[string]any{"key": "pic", "type": "image", "name": "Картинка",
			"meta": []any{map[string]any{"key": "crop", "name": "Обрезка", "editor": "crop", "places": []any{"card"}}}},
		map[string]any{"key": "friend", "type": "ref", "name": "Друг", "entities": []any{personEntity}},
	}}), http.StatusOK, "choice, image, ref")
	addValue(t, c, id, "custom.faith", map[string]any{"if_rev": 0, "value": "b"})
	// используемый вариант не убрать, неиспользуемый — можно, переименовать — можно
	faith := func(opts ...any) map[string]any {
		return map[string]any{"key": "faith", "type": "choice", "name": "Вера", "options": opts}
	}
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 1, "custom_fields": []any{
		faith(map[string]any{"key": "a", "name": "А"}),
	}}), http.StatusBadRequest, "remove used option")
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 1, "custom_fields": []any{
		faith(map[string]any{"key": "b", "name": "Бэ"}),
		map[string]any{"key": "pic", "type": "image", "name": "Картинка",
			"meta": []any{map[string]any{"key": "crop", "name": "Обрезка", "editor": "crop", "places": []any{"card"}}}},
		map[string]any{"key": "friend", "type": "ref", "name": "Друг", "entities": []any{personEntity}},
	}}), http.StatusOK, "drop unused option, rename used")
	expect(t, c.do("POST", "/api/records/"+id+"/fields/custom.faith/values", map[string]any{"if_rev": 1, "value": "z"}),
		http.StatusBadRequest, "unknown option")
	addValue(t, c, id, "custom.pic", map[string]any{"if_rev": 0, "value": strings.Repeat("a", 64),
		"meta": map[string]any{"crop": map[string]any{"card": map[string]any{"x": 0, "y": 0, "w": 10, "h": 5, "shape": "rect"}}}})
	addValue(t, c, id, "custom.friend", map[string]any{"if_rev": 0, "value": other})
	expect(t, c.do("POST", "/api/records/"+id+"/fields/custom.friend/values", map[string]any{"if_rev": 1, "value": pair}),
		http.StatusBadRequest, "ref limited to entity")

	bad := []map[string]any{
		{"key": "c", "type": "contact", "name": "Контакт"},
		{"key": "c", "type": "choice", "name": "Без вариантов"},
		{"key": "c", "type": "choice", "name": "Повтор", "options": []any{map[string]any{"key": "a", "name": "А"}, map[string]any{"key": "a", "name": "Б"}}},
		{"key": "c", "type": "text", "name": "Варианты не у выбора", "options": []any{map[string]any{"key": "a", "name": "А"}}},
		{"key": "c", "type": "ref", "name": "Нет сущности", "entities": []any{"no.such"}},
	}
	for _, b := range bad {
		expect(t, c.do("PATCH", "/api/records/"+other, map[string]any{"if_rev": 0, "custom_fields": []any{b}}),
			http.StatusBadRequest, b["name"].(string))
	}

	// общее поле шаблона: в схеме дашборда, значения — shared.<key> у любой записи шаблона
	tmpl, _ := record(c.do("GET", "/api/records/"+id, nil))["template_id"].(string)
	url := "/api/dashboards/" + dash + "/templates/" + tmpl + "/custom_fields"
	religion := map[string]any{"key": "religion", "type": "text", "name": "Религия"}
	r := c.do("PUT", url, map[string]any{"if_rev": 0, "custom_fields": []any{religion}})
	expect(t, r, http.StatusOK, "shared field")
	if def, _ := r.body["definition"].(map[string]any); r.body["rev"] != float64(1) || def["custom_fields"] == nil {
		t.Fatalf("template %v", r.body)
	}
	expect(t, c.do("PUT", url, map[string]any{"if_rev": 0, "custom_fields": []any{}}), http.StatusConflict, "stale rev")
	addValue(t, c, other, "shared.religion", map[string]any{"if_rev": 0, "value": "—"})
	addValue(t, c, id, "shared.religion", map[string]any{"if_rev": 0, "value": "x"})
	expect(t, c.do("POST", "/api/records/"+pair+"/fields/shared.religion/values", map[string]any{"if_rev": 0, "value": "x"}),
		http.StatusBadRequest, "other template")
	expect(t, c.do("PUT", url, map[string]any{"if_rev": 1, "custom_fields": []any{}}), http.StatusBadRequest, "remove used shared")
	religion["type"] = "number"
	expect(t, c.do("PUT", url, map[string]any{"if_rev": 1, "custom_fields": []any{religion}}), http.StatusBadRequest, "retype used shared")
	religion["type"], religion["name"] = "text", "Вероисповедание"
	expect(t, c.do("PUT", url, map[string]any{"if_rev": 1, "custom_fields": []any{religion}}), http.StatusOK, "rename shared")
	expect(t, c.do("PUT", "/api/dashboards/"+dash+"/templates/"+id+"/custom_fields", map[string]any{"if_rev": 0, "custom_fields": []any{}}),
		http.StatusNotFound, "unknown template")

	// журнал: правка шаблона дашборда с «было → стало»
	found := false
	r = c.do("GET", "/api/dashboards/"+dash+"/audit", nil)
	ops, _ := r.body["operations"].([]any)
	for _, o := range ops {
		entries, _ := o.(map[string]any)["entries"].([]any)
		for _, en := range entries {
			if m := en.(map[string]any); m["object"] == "dashboard_template" && m["op"] == "update" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("no audit entry for shared fields")
	}
}

// F-100, F-104: выбор со своим значением — вариант хранится ключом без
// пометки, своё — текстом с пометкой meta.free; своё, совпавшее с названием
// варианта, — ключом варианта; текст, равный ключу, с пометкой — своё.
func TestCustomFreeChoice(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	id := person(t, c, dash, nil)
	mood := func(free bool, opts ...any) map[string]any {
		return map[string]any{"key": "mood", "type": "choice", "name": "Настроение", "history": true, "free": free, "options": opts}
	}
	fun := map[string]any{"key": "fun", "name": "Весело"}
	calm := map[string]any{"key": "calm", "name": "Спокойно"}
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 0, "custom_fields": []any{mood(true, fun)}}),
		http.StatusOK, "free choice")
	stored := func(vid string, rec map[string]any) (any, bool) {
		for _, v := range values(rec, "custom.mood") {
			if v["id"] == vid {
				meta, _ := v["meta"].(map[string]any)
				return v["value"], meta["free"] == true
			}
		}
		return nil, false
	}
	check := func(what string, wantV any, wantFree bool) func(string, map[string]any) {
		return func(vid string, rec map[string]any) {
			t.Helper()
			if v, free := stored(vid, rec); v != wantV || free != wantFree {
				t.Fatalf("%s: value %v free %v, want %v %v", what, v, free, wantV, wantFree)
			}
		}
	}
	check("own text", "Грустно", true)(addValue(t, c, id, "custom.mood", map[string]any{"if_rev": 0, "value": "Грустно"}))
	check("option name", "fun", false)(addValue(t, c, id, "custom.mood", map[string]any{"if_rev": 1, "value": " весело "}))
	check("option key", "fun", false)(addValue(t, c, id, "custom.mood", map[string]any{"if_rev": 2, "value": "fun"}))
	// вписан текст, равный ключу варианта: с пометкой — своё, не вариант
	vid, rec := addValue(t, c, id, "custom.mood", map[string]any{"if_rev": 3, "value": "fun", "meta": map[string]any{"free": true}})
	check("own text equal to key", "fun", true)(vid, rec)
	// с пометкой, но совпало с названием варианта — ключ варианта без пометки
	check("marked option name", "fun", false)(addValue(t, c, id, "custom.mood",
		map[string]any{"if_rev": 4, "value": "Весело", "meta": map[string]any{"free": true}}))
	// правка одной meta пометку сохраняет; новое значение без пометки — размечается заново
	path := "/api/records/" + id + "/fields/custom.mood/values/" + vid
	r := c.do("PATCH", path, map[string]any{"if_rev": 5, "meta": map[string]any{}})
	expect(t, r, http.StatusOK, "patch meta")
	check("meta-only patch", "fun", true)(vid, record(r))
	r = c.do("PATCH", path, map[string]any{"if_rev": 6, "value": "fun"})
	expect(t, r, http.StatusOK, "patch to option")
	check("patched to option", "fun", false)(vid, record(r))
	r = c.do("PATCH", path, map[string]any{"if_rev": 7, "value": "fun", "meta": map[string]any{"free": true}})
	expect(t, r, http.StatusOK, "patch to own")
	check("patched to own", "fun", true)(vid, record(r))

	// свои значения есть — выбор не сделать строгим; используемый вариант не убрать
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 1, "custom_fields": []any{mood(false, fun)}}),
		http.StatusBadRequest, "free values in use")
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 1, "custom_fields": []any{
		mood(true, map[string]any{"key": "sad", "name": "Грустно"})}}),
		http.StatusBadRequest, "used option")
	// новый вариант с ключом, равным своему значению, — своё им не становится
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 1, "custom_fields": []any{
		mood(true, fun, calm, map[string]any{"key": "zen", "name": "Дзен"})}}),
		http.StatusOK, "add options")
	check("own stays own", "fun", true)(vid, record(c.do("GET", "/api/records/"+id, nil)))
	// вариант с названием, равным своему значению: правка одной meta смысла не меняет
	named := map[string]any{"key": "named", "name": "fun"}
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 2, "custom_fields": []any{mood(true, fun, calm, named)}}),
		http.StatusOK, "option named as own value")
	r = c.do("PATCH", path, map[string]any{"if_rev": 8, "meta": map[string]any{"free": false}})
	expect(t, r, http.StatusOK, "meta-only patch after new option")
	check("meta-only keeps own", "fun", true)(vid, record(r))
	// то же значение заново с пометкой (правка примечания из формы) — своё остаётся своим
	r = c.do("PATCH", path, map[string]any{"if_rev": 9, "value": "fun", "meta": map[string]any{"free": true}, "note": "n"})
	expect(t, r, http.StatusOK, "resubmit own value")
	check("resubmitted own stays own", "fun", true)(vid, record(r))
	// явно снятая пометка — текст без пометки: ключ варианта fun
	r = c.do("PATCH", path, map[string]any{"if_rev": 10, "value": "fun", "meta": map[string]any{"free": false}})
	expect(t, r, http.StatusOK, "unmark own value")
	check("unmarked is option key", "fun", false)(vid, record(r))
	// своё снова; потом вариант с таким названием: правка с пометкой — своё, без неё — вариант
	r = c.do("PATCH", path, map[string]any{"if_rev": 11, "value": "Своё", "meta": map[string]any{"free": true}})
	expect(t, r, http.StatusOK, "own again")
	check("own again", "Своё", true)(vid, record(r))
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 3, "custom_fields": []any{
		mood(true, fun, calm, named, map[string]any{"key": "own", "name": "Своё"})}}), http.StatusOK, "option named as own")
	r = c.do("PATCH", path, map[string]any{"if_rev": 12, "value": "Своё", "meta": map[string]any{"free": true}, "note": "m"})
	expect(t, r, http.StatusOK, "resubmit own")
	check("resubmit keeps own", "Своё", true)(vid, record(r))
	r = c.do("PATCH", path, map[string]any{"if_rev": 13, "value": "Своё", "meta": map[string]any{}})
	expect(t, r, http.StatusOK, "unmark")
	check("unmarked matches option name", "own", false)(vid, record(r))
	// пометка — только у выбора со своим значением
	expect(t, c.do("POST", "/api/records/"+id+"/fields/name/values",
		map[string]any{"if_rev": 0, "value": "Анна", "meta": map[string]any{"free": true}}), http.StatusBadRequest, "free mark on text")
	nameID, _ := addValue(t, c, id, "name", map[string]any{"if_rev": 0, "value": "Анна"})
	expect(t, c.do("PATCH", "/api/records/"+id+"/fields/name/values/"+nameID,
		map[string]any{"if_rev": 1, "meta": map[string]any{"free": true}}), http.StatusBadRequest, "free mark on text, meta only")
	// своё значение — только у выбора
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 4, "custom_fields": []any{
		mood(true, fun, calm, named, map[string]any{"key": "own", "name": "Своё"}),
		map[string]any{"key": "t", "type": "text", "name": "Текст", "free": true},
	}}), http.StatusBadRequest, "free on text")
}

// F-104: «выбор» → «выбор или своё» — всегда; обратно — когда своих значений
// (по пометке) нет, даже если текст своего значения равен ключу варианта.
func TestFreeChoiceToggle(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	id := person(t, c, dash, nil)
	mood := func(free bool) map[string]any {
		return map[string]any{"key": "mood", "type": "choice", "name": "Настроение", "free": free,
			"options": []any{map[string]any{"key": "fun", "name": "Весело"}}}
	}
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 0, "custom_fields": []any{mood(false)}}), http.StatusOK, "strict")
	addValue(t, c, id, "custom.mood", map[string]any{"if_rev": 0, "value": "fun"})
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 1, "custom_fields": []any{mood(true)}}), http.StatusOK, "to free")
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 2, "custom_fields": []any{mood(false)}}), http.StatusOK, "back: no own")
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 3, "custom_fields": []any{mood(true)}}), http.StatusOK, "to free again")
	vals := values(record(c.do("GET", "/api/records/"+id, nil)), "custom.mood")
	r := c.do("PATCH", "/api/records/"+id+"/fields/custom.mood/values/"+vals[0]["id"].(string),
		map[string]any{"if_rev": 1, "value": "fun", "meta": map[string]any{"free": true}})
	expect(t, r, http.StatusOK, "own equal to key")
	expect(t, c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 4, "custom_fields": []any{mood(false)}}),
		http.StatusBadRequest, "back with own value")
}
