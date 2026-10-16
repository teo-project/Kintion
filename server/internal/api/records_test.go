package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/google/uuid"
)

// dashboardOf — первый дашборд владельца.
func dashboardOf(t *testing.T, c *client) string {
	t.Helper()
	list := c.doList("/api/dashboards")
	if len(list) == 0 {
		t.Fatal("no dashboards")
	}
	return list[0]["id"].(string)
}

// batch выполняет пакетную операцию и ждёт кода want.
func batch(t *testing.T, c *client, dash string, body any, want int) resp {
	t.Helper()
	r := c.do("POST", "/api/dashboards/"+dash+"/operations", body)
	expect(t, r, want, "operations")
	return r
}

// created — id записи пакета по временному id.
func created(t *testing.T, r resp, ref string) string {
	t.Helper()
	ids, _ := r.body["ids"].(map[string]any)
	id, _ := ids[ref].(string)
	if id == "" {
		t.Fatalf("no id for %s in %v", ref, r.body)
	}
	return id
}

func record(r resp) map[string]any {
	rec, _ := r.body["record"].(map[string]any)
	return rec
}

func revOf(rec map[string]any, key string) float64 {
	revs, _ := rec["revs"].(map[string]any)
	v, _ := revs[key].(float64)
	return v
}

// person создаёт человека (шаблон по умолчанию) и возвращает id.
func person(t *testing.T, c *client, dash string, extra map[string]any) string {
	t.Helper()
	rec := map[string]any{"ref": "p", "entity": "kin.core.person"}
	for k, v := range extra {
		rec[k] = v
	}
	return created(t, batch(t, c, dash, map[string]any{"records": []any{rec}}, http.StatusCreated), "p")
}

var uuidRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// golden сверяет JSON с testdata/api/<name>.json; UUID заменяются на <id-N>
// по порядку появления. TEO_KIN_UPDATE_GOLDEN=1 — перезаписать эталон.
func golden(t *testing.T, name string, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	raw = uuidRe.ReplaceAllFunc(raw, func(b []byte) []byte {
		s := string(b)
		if _, ok := seen[s]; !ok {
			seen[s] = "<id-" + string(rune('0'+len(seen)/10)) + string(rune('0'+len(seen)%10)) + ">"
		}
		return []byte(seen[s])
	})
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		t.Fatal(err)
	}
	buf.WriteByte('\n')
	path := filepath.Join("..", "..", "..", "testdata", "api", name+".json")
	if os.Getenv("TEO_KIN_UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s: %v (TEO_KIN_UPDATE_GOLDEN=1 — создать)", name, err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), buf.Bytes()) {
		t.Fatalf("golden %s differs:\n%s", name, buf.String())
	}
}

func TestCreateAndReadRecord(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	r := batch(t, c, dash, map[string]any{"records": []any{map[string]any{
		"ref": "p", "entity": "kin.core.person", "note": "по паспорту 1923",
		"start": map[string]any{"value": map[string]int{"y": 1985, "m": 3, "d": 12}},
	}}}, http.StatusCreated)
	id := created(t, r, "p")
	got := c.do("GET", "/api/records/"+id, nil)
	expect(t, got, http.StatusOK, "get")
	rec := record(got)
	if rec["template"] != "kin.core.person" || rec["entity"] != "kin.core.person" || rec["note"] != "по паспорту 1923" {
		t.Fatalf("record %v", rec)
	}
	golden(t, "record_person", got.body)

	// чужая и несуществующая запись — 404
	for _, p := range []string{uuid.Must(uuid.NewV7()).String(), "x"} {
		expect(t, c.do("GET", "/api/records/"+p, nil), http.StatusNotFound, "missing")
	}
	// шаблон неизвестен / неверная сущность — ошибка пакета, ничего не создано
	r = batch(t, c, dash, map[string]any{"records": []any{
		map[string]any{"ref": "a", "entity": "kin.core.person"},
		map[string]any{"ref": "b", "template": "no.such"},
	}}, http.StatusBadRequest)
	if r.body["field"] != "records[1].template" {
		t.Fatalf("error %v", r.body)
	}
	var n int
	if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM entity").Scan(&n); err != nil || n != 1 {
		t.Fatalf("entities %d %v", n, err)
	}
}

func TestBatchOneOperation(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	r := batch(t, c, dash, map[string]any{
		"records": []any{
			map[string]any{"ref": "a", "entity": "kin.core.person"},
			map[string]any{"ref": "b", "entity": "kin.core.person", "start": map[string]any{"link": "w"}},
			map[string]any{"ref": "w", "template": "kin.core.birth"},
		},
		"participants": []any{map[string]any{"container": "w", "entity": "b", "role": "child"}},
	}, http.StatusCreated)
	op, err := uuid.Parse(r.body["operation_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	rows := auditOf(t, e, op)
	entities := 0
	for _, row := range rows {
		if row.Object == "entity" && row.Op == "create" {
			entities++
		}
	}
	if entities != 3 {
		t.Fatalf("audit: %d entity rows of %d", entities, len(rows))
	}
	b := created(t, r, "b")
	rec := record(c.do("GET", "/api/records/"+b, nil))
	start, _ := rec["start"].(map[string]any)
	if start["link"] != created(t, r, "w") {
		t.Fatalf("start link %v", rec["start"])
	}
}

func TestSystemFields(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	id := person(t, c, dash, nil)
	path := "/api/records/" + id

	// две «вкладки» видят rev 0 у всех полей
	r := c.do("PATCH", path, map[string]any{"if_rev": 0, "note": "первая"})
	expect(t, r, http.StatusOK, "note")
	if revOf(record(r), "note") != 1 || record(r)["note"] != "первая" {
		t.Fatalf("note %v", record(r))
	}
	// другая вкладка правит другое поле со старой версией — без конфликта
	r = c.do("PATCH", path, map[string]any{"if_rev": 0, "start": map[string]any{"value": map[string]int{"y": 1990}}})
	expect(t, r, http.StatusOK, "start")
	// то же поле со старой версией — 409 с текущим значением
	r = c.do("PATCH", path, map[string]any{"if_rev": 0, "note": "вторая"})
	expect(t, r, http.StatusConflict, "note conflict")
	cur, _ := r.body["current"].(map[string]any)
	if r.body["field"] != "note" || cur["value"] != "первая" || cur["rev"] != float64(1) {
		t.Fatalf("conflict %v", r.body)
	}
	// без версии, два поля сразу, ручная подпись без freeedit — ошибки проверки
	expect(t, c.do("PATCH", path, map[string]any{"note": "x"}), http.StatusBadRequest, "no if_rev")
	expect(t, c.do("PATCH", path, map[string]any{"if_rev": 1, "note": "x", "sensitive": true}), http.StatusBadRequest, "two fields")
	expect(t, c.do("PATCH", path, map[string]any{"if_rev": 0, "title_manual": "Тётя"}), http.StatusBadRequest, "no freeedit")
	r = c.do("PATCH", path, map[string]any{"if_rev": 0, "sensitive": true})
	expect(t, r, http.StatusOK, "sensitive")
	if record(r)["sensitive"] != true {
		t.Fatal("sensitive not set")
	}

	// событие: ручная подпись разрешена (freeedit), конца нет ("end": null)
	w := wedding(t, c, dash)
	wp := "/api/records/" + w
	r = c.do("PATCH", wp, map[string]any{"if_rev": 0, "title_manual": "Свадьба в Пензе"})
	expect(t, r, http.StatusOK, "title")
	expect(t, c.do("PATCH", wp, map[string]any{"if_rev": 0, "end": map[string]any{"value": map[string]int{"y": 2020}}}),
		http.StatusBadRequest, "event end")
	// привязка start к событию того же дашборда; к себе и к несуществующей — нет
	r = c.do("PATCH", path, map[string]any{"if_rev": 1, "start": map[string]any{"value": map[string]int{"y": 1990}, "link": w}})
	expect(t, r, http.StatusOK, "start link")
	expect(t, c.do("PATCH", path, map[string]any{"if_rev": 2, "start": map[string]any{"link": id}}), http.StatusBadRequest, "self link")
	expect(t, c.do("PATCH", path, map[string]any{"if_rev": 2, "start": map[string]any{"link": uuid.Must(uuid.NewV7()).String()}}),
		http.StatusBadRequest, "missing link")
	rel, _ := r.body["related"].(map[string]any)
	if _, ok := rel[w]; !ok {
		t.Fatalf("related %v", rel)
	}

	// каждое изменение — в журнале
	var n int
	if err := e.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_log WHERE entity_id = $1 AND object = 'entity' AND op = 'update'", id).Scan(&n); err != nil || n != 4 {
		t.Fatalf("updates in audit %d %v", n, err)
	}
}

func TestInactiveTemplateHidden(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	id := person(t, c, dash, nil)
	if _, err := e.pool.Exec(context.Background(),
		"UPDATE dashboard_template SET active = false WHERE id = (SELECT template_id FROM entity WHERE id = $1)", id); err != nil {
		t.Fatal(err)
	}
	expect(t, c.do("GET", "/api/records/"+id, nil), http.StatusNotFound, "inactive template")
	batch(t, c, dash, map[string]any{"records": []any{map[string]any{"entity": "kin.core.person"}}}, http.StatusBadRequest)
}
