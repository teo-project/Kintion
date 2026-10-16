package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"kintion/server/internal/api"
	"kintion/server/internal/files"
	"kintion/server/internal/httpx"
	"kintion/server/internal/packs"
)

// F-135: значение поля, которого нет в шаблоне текущего языка, «спит»: переживает правки
// других полей и снова видно, когда поле есть.
func TestLanguageFieldSleeps(t *testing.T) {
	e := newEnv(t) // ru: у организации есть ИНН
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	id := created(t, batch(t, c, dash, map[string]any{"records": []any{map[string]any{
		"ref": "o", "entity": "kin.core.organization",
		"fields": map[string]any{
			"name": []any{map[string]any{"value": "Ромашка"}},
			"inn":  []any{map[string]any{"value": "7700000000"}},
		},
	}}}, http.StatusCreated), "o")
	if v := values(record(c.do("GET", "/api/records/"+id, nil)), "inn"); len(v) != 1 {
		t.Fatalf("ru inn %v", v)
	}

	// тот же сервер и БД, но пак en: поля inn в шаблоне нет
	src, err := packs.Source("", "en")
	if err != nil {
		t.Fatal(err)
	}
	reg, ds := packs.Load(src, "en", []string{"kin.core", "kin.christian.orthodox"})
	if packs.HasErrors(ds) {
		t.Fatal(ds)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := api.New(api.Deps{Log: log, Auth: e.auth, Pool: e.pool, Registry: reg, Lang: "en",
		Files: &files.Store{Root: t.TempDir(), MaxBytes: testUploadMax}, Licenses: []byte(testLicenses)})
	srv := httptest.NewServer(httpx.NewHandler(httpx.Deps{Log: log, DB: e.pool, API: h, Static: fstest.MapFS{}}))
	t.Cleanup(srv.Close)
	e2 := *e
	e2.srv, e2.reg = srv, reg
	c2 := &client{e: &e2, cookie: c.cookie, csrf: c.csrf}

	rec := record(c2.do("GET", "/api/records/"+id, nil))
	// сервер отдаёт хранимое как есть; показывает только поля шаблона текущего языка клиент
	if v := values(rec, "inn"); len(v) != 1 {
		t.Fatalf("en inn must be kept: %v", v)
	}
	// правка другого поля в en
	expect(t, c2.do("POST", "/api/records/"+id+"/fields/name/values", map[string]any{"if_rev": fieldRev(rec, "name"), "value": "Лютик"}),
		http.StatusOK, "add name in en")
	expect(t, c2.do("POST", "/api/records/"+id+"/fields/inn/values", map[string]any{"if_rev": 0, "value": "1"}),
		http.StatusBadRequest, "no such field in en")

	// вернули язык — значение на месте
	if v := values(record(c.do("GET", "/api/records/"+id, nil)), "inn"); len(v) != 1 || v[0]["value"] != "7700000000" {
		t.Fatalf("ru inn after en edits %v", v)
	}
}
