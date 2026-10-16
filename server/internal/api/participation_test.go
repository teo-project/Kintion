package api_test

import (
	"context"
	"net/http"
	"testing"
)

func participants(rec map[string]any, role string) []map[string]any {
	list, _ := rec["participants"].([]any)
	out := []map[string]any{}
	for _, p := range list {
		m := p.(map[string]any)
		if role == "" || m["role"] == role {
			out = append(out, m)
		}
	}
	return out
}

func TestWeddingNeedsTwoPartners(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	anna, ivan := person(t, c, dash, nil), person(t, c, dash, nil)
	count := func() int {
		var n int
		if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM entity").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := count()
	for _, parts := range [][]string{nil, {anna}} {
		ps := []any{}
		for _, p := range parts {
			ps = append(ps, map[string]any{"container": "w", "entity": p, "role": "partner"})
		}
		r := batch(t, c, dash, map[string]any{"records": []any{map[string]any{"ref": "w", "template": "kin.core.wedding"}},
			"participants": ps}, http.StatusBadRequest)
		if r.body["field"] != "records[0]" {
			t.Fatalf("error %v", r.body)
		}
	}
	if count() != before {
		t.Fatal("wedding created without partners")
	}
	r := batch(t, c, dash, map[string]any{
		"records": []any{map[string]any{"ref": "w", "template": "kin.core.wedding"}},
		"participants": []any{
			map[string]any{"container": "w", "entity": anna, "role": "partner"},
			map[string]any{"container": "w", "entity": ivan, "role": "partner"},
		},
	}, http.StatusCreated)
	w := created(t, r, "w")
	rec := record(c.do("GET", "/api/records/"+w, nil))
	ps := participants(rec, "partner")
	if len(ps) != 2 {
		t.Fatalf("participants %v", rec["participants"])
	}
	// где участвует Анна
	annaRec := record(c.do("GET", "/api/records/"+anna, nil))
	in, _ := annaRec["participates_in"].([]any)
	if len(in) != 1 || in[0].(map[string]any)["container_id"] != w {
		t.Fatalf("participates_in %v", annaRec["participates_in"])
	}
	// третий в роли с max 2 — отказ; удаление ниже min — отказ
	third := person(t, c, dash, nil)
	expect(t, c.do("POST", "/api/records/"+w+"/participants", map[string]any{"if_rev": 0, "entity": third, "role": "partner"}),
		http.StatusBadRequest, "max")
	pid := ps[0]["id"].(string)
	expect(t, c.do("DELETE", "/api/records/"+w+"/participants/"+pid+"?if_rev=0", nil), http.StatusBadRequest, "below min")
	// свидетель — можно; период и пометка участия; версия роли
	r = c.do("POST", "/api/records/"+w+"/participants", map[string]any{"if_rev": 0, "entity": third, "role": "witness"})
	expect(t, r, http.StatusOK, "witness")
	wid := participants(record(r), "witness")[0]["id"].(string)
	r = c.do("PATCH", "/api/records/"+w+"/participants/"+wid, map[string]any{"if_rev": 1, "note": "со стороны невесты",
		"valid_from": map[string]int{"y": 2021}})
	expect(t, r, http.StatusOK, "patch participant")
	if p := participants(record(r), "witness")[0]; p["note"] != "со стороны невесты" {
		t.Fatalf("participant %v", p)
	}
	expect(t, c.do("DELETE", "/api/records/"+w+"/participants/"+wid+"?if_rev=1", nil), http.StatusConflict, "stale rev")
	expect(t, c.do("DELETE", "/api/records/"+w+"/participants/"+wid+"?if_rev=2", nil), http.StatusOK, "delete witness")
	// человек не контейнер
	expect(t, c.do("POST", "/api/records/"+anna+"/participants", map[string]any{"if_rev": 0, "entity": ivan, "role": "partner"}),
		http.StatusBadRequest, "person is not a container")
	// журнал: участия пишутся
	var n int
	if err := e.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_log WHERE entity_id = $1 AND object = 'participation'", w).Scan(&n); err != nil || n != 5 {
		t.Fatalf("participation audit %d %v", n, err)
	}
}

func TestParticipantRules(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	child := person(t, c, dash, nil)
	org := created(t, batch(t, c, dash, map[string]any{"records": []any{
		map[string]any{"ref": "o", "entity": "kin.core.organization"},
	}}, http.StatusCreated), "o")
	// организация в роли parent — отказ
	r := batch(t, c, dash, map[string]any{
		"records": []any{map[string]any{"ref": "r", "template": "kin.core.parent_child"}},
		"participants": []any{
			map[string]any{"container": "r", "entity": org, "role": "parent"},
			map[string]any{"container": "r", "entity": child, "role": "child"},
		},
	}, http.StatusBadRequest)
	if r.body["field"] != "participants[0].role" {
		t.Fatalf("error %v", r.body)
	}
	// «Община»: Иван дважды (вышел и вернулся) — допустимо
	ivan := person(t, c, dash, nil)
	r = batch(t, c, dash, map[string]any{
		"records": []any{map[string]any{"ref": "k", "template": "kin.core.commune"}},
		"participants": []any{
			map[string]any{"container": "k", "entity": ivan, "role": "member", "valid_from": map[string]int{"y": 2020}, "valid_to": map[string]int{"y": 2024}},
			map[string]any{"container": "k", "entity": ivan, "role": "member", "valid_from": map[string]int{"y": 2025}},
		},
	}, http.StatusCreated)
	if ps := participants(r.body["records"].([]any)[0].(map[string]any), "member"); len(ps) != 2 {
		t.Fatalf("commune %v", ps)
	}
}

// Вложенность через участие запрещена (F-82): запись-контейнер не участник.
func TestNoNestingByParticipation(t *testing.T) {
	e := newEnvAlt(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	r := batch(t, c, dash, map[string]any{
		"records": []any{
			map[string]any{"ref": "a", "entity": "test.alt.happening"},
			map[string]any{"ref": "b", "entity": "test.alt.happening"},
		},
		"participants": []any{map[string]any{"container": "a", "entity": "b", "role": "episode"}},
	}, http.StatusBadRequest)
	if r.body["field"] != "participants[0].role" {
		t.Fatalf("error %v", r.body)
	}
	r = batch(t, c, dash, map[string]any{"records": []any{
		map[string]any{"ref": "a", "entity": "test.alt.happening"},
		map[string]any{"ref": "b", "entity": "test.alt.happening"},
	}}, http.StatusCreated)
	a, b := created(t, r, "a"), created(t, r, "b")
	expect(t, c.do("POST", "/api/records/"+a+"/participants", map[string]any{"if_rev": 0, "entity": b, "role": "episode"}),
		http.StatusBadRequest, "nest")
	// роли синтетического пака: min 2 у ally
	batch(t, c, dash, map[string]any{"records": []any{map[string]any{"ref": "u", "template": "test.alt.union_rite"}}},
		http.StatusBadRequest)
}
