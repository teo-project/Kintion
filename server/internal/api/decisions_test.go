package api_test

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

// F-83: start / end и значения привязываются только к записям сущности с
// "anchor": true (в kin.core — «Событие»).
func TestAnchorOnlyToAnchorEntities(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	w := wedding(t, c, dash)
	ivan := person(t, c, dash, nil)
	liza := person(t, c, dash, nil)

	expect(t, c.do("PATCH", "/api/records/"+liza, map[string]any{"if_rev": 0, "start": map[string]any{"value": nil, "link": ivan}}),
		http.StatusBadRequest, "link to person")
	expect(t, c.do("PATCH", "/api/records/"+liza, map[string]any{"if_rev": 0, "start": map[string]any{"value": nil, "link": w}}),
		http.StatusOK, "link to event")
	expect(t, c.do("POST", "/api/records/"+liza+"/fields/surname/values", map[string]any{
		"if_rev": 0, "value": "Петрова", "links": []any{map[string]any{"target": ivan, "anchor": "start"}},
	}), http.StatusBadRequest, "value link to person")
	batch(t, c, dash, map[string]any{"records": []any{map[string]any{"ref": "p", "entity": "kin.core.person",
		"end": map[string]any{"value": nil, "link": ivan}}}}, http.StatusBadRequest)
}

// F-82: вложенное событие — значение поля-ссылки; `contains` шаблона
// ограничивает цели-контейнеры, прочие записи — без ограничений.
func TestRefFieldContains(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	w := wedding(t, c, dash)
	reg := withPartners(t, c, dash, "kin.core.registration")
	div := withPartners(t, c, dash, "kin.core.divorce")
	ivan := person(t, c, dash, nil)
	expect(t, c.do("PATCH", "/api/records/"+w, map[string]any{"if_rev": 0, "custom_fields": []any{
		map[string]any{"key": "part", "type": "ref", "name": "Часть", "multiple": true},
	}}), http.StatusOK, "custom field")
	add := func(target string, want int, what string) {
		t.Helper()
		rev := fieldRev(record(c.do("GET", "/api/records/"+w, nil)), "custom.part")
		expect(t, c.do("POST", "/api/records/"+w+"/fields/custom.part/values", map[string]any{"if_rev": rev, "value": target}), want, what)
	}
	add(reg, http.StatusOK, "registration in wedding")
	add(div, http.StatusBadRequest, "divorce not in contains")
	add(ivan, http.StatusOK, "person ref")
}

// F-80: настройки пользователя на сервере.
func TestPrefs(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	var p map[string]any
	c.getJSON("/api/me/prefs", &p)
	if p["history_edit"] != "" {
		t.Fatalf("default %v", p)
	}
	expect(t, c.do("PUT", "/api/me/prefs", map[string]any{"history_edit": "add"}), http.StatusOK, "set")
	c.getJSON("/api/me/prefs", &p)
	if p["history_edit"] != "add" {
		t.Fatalf("stored %v", p)
	}
	expect(t, c.do("PUT", "/api/me/prefs", map[string]any{"history_edit": "maybe"}), http.StatusBadRequest, "bad")
	expect(t, c.do("PUT", "/api/me/prefs", map[string]any{"history_edit": ""}), http.StatusOK, "reset")
}

// F-108: режим карточек — у аккаунта на сервере, по умолчанию «Чтение».
func TestPrefsCardMode(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	var p map[string]any
	c.getJSON("/api/me/prefs", &p)
	if p["card_mode"] != "read" {
		t.Fatalf("default %v", p)
	}
	for _, m := range []string{"inline", "form", "read"} {
		expect(t, c.do("PUT", "/api/me/prefs", map[string]any{"history_edit": "add", "card_mode": m}), http.StatusOK, m)
		c.getJSON("/api/me/prefs", &p)
		if p["card_mode"] != m || p["history_edit"] != "add" {
			t.Fatalf("stored %s: %v", m, p)
		}
	}
	expect(t, c.do("PUT", "/api/me/prefs", map[string]any{"history_edit": "", "card_mode": "edit"}), http.StatusBadRequest, "bad mode")
	// клиент без режима (до F-108) — «Чтение»
	expect(t, c.do("PUT", "/api/me/prefs", map[string]any{"history_edit": ""}), http.StatusOK, "no mode")
	c.getJSON("/api/me/prefs", &p)
	if p["card_mode"] != "read" {
		t.Fatalf("no mode %v", p)
	}
}

// F-81: последние уникальные группы, назначенные этим пользователем.
func TestRecentGroups(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	a, b := group(t, c, dash, "А"), group(t, c, dash, "Б")
	var ids []string
	c.getJSON("/api/dashboards/"+dash+"/groups/recent", &ids)
	if len(ids) != 0 {
		t.Fatalf("empty %v", ids)
	}
	person(t, c, dash, map[string]any{"groups": []string{a}})
	p := person(t, c, dash, nil)
	expect(t, c.do("PUT", "/api/records/"+p+"/groups", map[string]any{"if_rev": 0, "groups": []string{b}}), http.StatusOK, "label")
	person(t, c, dash, map[string]any{"groups": []string{a}})
	c.getJSON("/api/dashboards/"+dash+"/groups/recent", &ids)
	if len(ids) != 2 || ids[0] != a || ids[1] != b {
		t.Fatalf("recent %v (a=%s b=%s)", ids, a, b)
	}
}

// F-88: запись из черновика создаётся только вместе со значением — ошибка
// значения в строгом пакете отменяет всё.
func TestStrictBatch(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	bad := map[string]any{"strict": true, "records": []any{map[string]any{"ref": "p", "entity": "kin.core.person",
		"fields": map[string]any{"gender": []any{map[string]any{"value": "nobody"}}}}}}
	r := batch(t, c, dash, bad, http.StatusBadRequest)
	if r.body["field"] != "gender" {
		t.Fatalf("error %v", r.body)
	}
	if n := e.count(t, "SELECT count(*) FROM entity"); n != 0 {
		t.Fatalf("records %d", n)
	}
	r = batch(t, c, dash, map[string]any{"strict": true, "records": []any{map[string]any{"ref": "p", "entity": "kin.core.person",
		"fields": map[string]any{"name": []any{map[string]any{"value": "Лиза"}}}}}}, http.StatusCreated)
	if values(record(c.do("GET", "/api/records/"+created(t, r, "p"), nil)), "name")[0]["value"] != "Лиза" {
		t.Fatal("value lost")
	}
}

// Тело не в UTF-8 — 400, а не внутренняя ошибка базы.
func TestBodyNotUTF8(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	raw := []byte(`{"records":[{"ref":"p","entity":"kin.core.person","fields":{"name":[{"value":"` + "\xcb\xfe" + `"}]}}]}`)
	r := c.do("POST", "/api/dashboards/"+dash+"/operations", nil, func(req *http.Request) {
		req.Body = io.NopCloser(bytes.NewReader(raw))
		req.ContentLength = int64(len(raw))
		req.Header.Set("Content-Type", "application/json")
	})
	expect(t, r, http.StatusBadRequest, "not utf-8")
}
