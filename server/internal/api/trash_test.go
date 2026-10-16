package api_test

import (
	"context"
	"net/http"
	"testing"
)

func (e *env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Поведение ссылок на запись (план §4) — по строкам таблицы: корзина,
// восстановление, очистка.
func TestTrashReferences(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	r := batch(t, c, dash, map[string]any{
		"records": []any{
			map[string]any{"ref": "liza", "entity": "kin.core.person",
				"end":           map[string]any{"value": map[string]int{"y": 2070}, "link": "w"},
				"custom_fields": []any{map[string]any{"key": "mentor", "type": "ref", "name": "Наставник"}},
				"fields": map[string]any{
					"custom.mentor": []any{map[string]any{"value": "ivan"}},
					"surname": []any{map[string]any{"value": "Краник",
						"links": []any{map[string]any{"target": "w", "anchor": "none"}}}},
				}},
			map[string]any{"ref": "ivan", "entity": "kin.core.person",
				"fields": map[string]any{"name": []any{map[string]any{"value": "Иван"}}}},
			map[string]any{"ref": "w", "template": "kin.core.wedding"},
		},
		"participants": []any{
			map[string]any{"container": "w", "entity": "liza", "role": "partner"},
			map[string]any{"container": "w", "entity": "ivan", "role": "partner"},
		},
	}, http.StatusCreated)
	if errs, _ := r.body["errors"].([]any); len(errs) != 0 {
		t.Fatalf("errors %v", errs)
	}
	liza, ivan, w := created(t, r, "liza"), created(t, r, "ivan"), created(t, r, "w")

	visible := func(want bool) {
		t.Helper()
		lr := c.do("GET", "/api/records/"+liza, nil)
		rec := record(lr)
		end, _ := rec["end"].(map[string]any)
		mentor := values(rec, "custom.mentor")
		links := values(rec, "surname")[0]["links"].([]any)
		rel, _ := lr.body["related"].(map[string]any)
		_, inRelated := rel[ivan]
		wp := participants(record(c.do("GET", "/api/records/"+w, nil)), "partner")
		// привязки — к событию (F-83), оно в корзину не уходит
		if end["link"] != w || len(links) != 1 {
			t.Fatalf("event links: %v", rec)
		}
		got := []bool{mentor[0]["value"] == ivan, inRelated, len(wp) == 2}
		for i, g := range got {
			if g != want {
				t.Fatalf("visible=%v, check %d: %v", want, i, rec)
			}
		}
		if end["value"] == nil {
			t.Fatal("end date lost")
		}
	}
	visible(true)

	// в корзину: в чужих карточках и related не видна, сама — видна владельцу
	rr := c.do("DELETE", "/api/records/"+ivan, nil)
	expect(t, rr, http.StatusOK, "delete")
	if record(rr)["deleted"] != true {
		t.Fatal("not deleted")
	}
	visible(false)
	expect(t, c.do("PATCH", "/api/records/"+ivan, map[string]any{"if_rev": 0, "note": "x"}), http.StatusConflict, "edit deleted")
	expect(t, c.do("DELETE", "/api/records/"+ivan, nil), http.StatusConflict, "delete twice")
	expect(t, c.do("POST", "/api/records/"+ivan+"/restore", nil), http.StatusOK, "restore")
	visible(true)
	expect(t, c.do("POST", "/api/records/"+ivan+"/restore", nil), http.StatusConflict, "restore live")

	// очистка: только из корзины, одной операцией
	expect(t, c.do("DELETE", "/api/records/"+ivan+"/purge", nil), http.StatusConflict, "purge live")
	expect(t, c.do("DELETE", "/api/records/"+ivan, nil), http.StatusOK, "delete again")
	pr := c.do("DELETE", "/api/records/"+ivan+"/purge", nil)
	expect(t, pr, http.StatusNoContent, "purge")
	expect(t, c.do("GET", "/api/records/"+ivan, nil), http.StatusNotFound, "purged")

	if n := e.count(t, "SELECT count(*) FROM field_entry WHERE entity_id = $1 OR v_ref = $1", ivan); n != 0 {
		t.Fatalf("values left %d", n)
	}
	if n := e.count(t, "SELECT count(*) FROM participation WHERE entity_id = $1 OR container_id = $1", ivan); n != 0 {
		t.Fatalf("participations left %d", n)
	}
	// журнал очищенной записи остаётся; очистка — одна операция
	if n := e.count(t, "SELECT count(*) FROM audit_log WHERE entity_id = $1 AND op = 'create'", ivan); n == 0 {
		t.Fatal("audit of purged record lost")
	}
	if n := e.count(t, "SELECT count(DISTINCT operation_id) FROM audit_log WHERE request_id = $1", pr.header.Get("X-Request-Id")); n != 1 {
		t.Fatalf("purge operations %d", n)
	}
	// значение name, участие, ref, сама запись
	if n := e.count(t, "SELECT count(*) FROM audit_log WHERE request_id = $1", pr.header.Get("X-Request-Id")); n != 4 {
		t.Fatalf("purge audit rows %d", n)
	}

	// очистка цели привязок: привязка значения и end_link снимаются, значения остаются
	if n := e.count(t, "SELECT count(*) FROM entity WHERE id = $1 AND end_link = $2", liza, w); n != 1 {
		t.Fatal("end_link to event lost")
	}
	expect(t, c.do("DELETE", "/api/records/"+w, nil), http.StatusOK, "delete event")
	expect(t, c.do("DELETE", "/api/records/"+w+"/purge", nil), http.StatusNoContent, "purge event")
	if n := e.count(t, "SELECT count(*) FROM entry_link WHERE target_id = $1", w); n != 0 {
		t.Fatalf("links left %d", n)
	}
	if n := e.count(t, "SELECT count(*) FROM entity WHERE id = $1 AND end_link IS NULL AND NOT (end_date IS NULL)", liza); n != 1 {
		t.Fatal("end_link not cleared or end date lost")
	}
	if n := e.count(t, "SELECT count(*) FROM field_entry WHERE entity_id = $1 AND field_key = 'surname'", liza); n != 1 {
		t.Fatal("linked value removed with its link")
	}
}

// Очистка контейнера удаляет его участия; участники остаются.
func TestPurgeContainer(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	w := wedding(t, c, dash)
	expect(t, c.do("DELETE", "/api/records/"+w, nil), http.StatusOK, "delete")
	// где участвуют партнёры: свадьба в корзине не показывается
	for _, p := range participants(record(c.do("GET", "/api/records/"+w, nil)), "partner") {
		in, _ := record(c.do("GET", "/api/records/"+p["entity_id"].(string), nil))["participates_in"].([]any)
		if len(in) != 0 {
			t.Fatalf("deleted container visible: %v", in)
		}
	}
	expect(t, c.do("DELETE", "/api/records/"+w+"/purge", nil), http.StatusNoContent, "purge")
	if n := e.count(t, "SELECT count(*) FROM participation"); n != 0 {
		t.Fatalf("participations %d", n)
	}
	if n := e.count(t, "SELECT count(*) FROM entity WHERE deleted_at IS NULL"); n != 2 {
		t.Fatalf("persons %d", n)
	}
}
