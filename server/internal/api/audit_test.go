package api_test

import (
	"net/http"
	"net/url"
	"testing"
)

type auditPage struct {
	Operations []struct {
		OperationID string `json:"operation_id"`
		ActorLogin  string `json:"actor_login"`
		Source      string `json:"source"`
		RequestID   string `json:"request_id"`
		Entries     []struct {
			EntityID string         `json:"entity_id"`
			Object   string         `json:"object"`
			Op       string         `json:"op"`
			After    map[string]any `json:"after"`
		} `json:"entries"`
	} `json:"operations"`
	Next    *string                   `json:"next"`
	Records map[string]map[string]any `json:"records"`
	Related map[string]map[string]any `json:"related"`
	Purged  map[string]map[string]any `json:"purged"`
}

func auditPageOf(t *testing.T, c *client, dash string, q url.Values) auditPage {
	t.Helper()
	var p auditPage
	c.getJSON("/api/dashboards/"+dash+"/audit?"+q.Encode(), &p)
	return p
}

func TestAuditAPI(t *testing.T) {
	e := newEnv(t, "kin.core")
	c := e.setupOwner()
	dash := dashboardOf(t, c)

	// пакет «запись + отношение + участия» — одна операция
	r := batch(t, c, dash, map[string]any{
		"records": []any{
			map[string]any{"ref": "anna", "entity": "kin.core.person",
				"fields": map[string]any{"name": []any{map[string]any{"value": "Анна"}}}},
			map[string]any{"ref": "ivan", "entity": "kin.core.person"},
			map[string]any{"ref": "pair", "template": "kin.core.relationship"},
		},
		"participants": []any{
			map[string]any{"container": "pair", "entity": "anna", "role": "partner"},
			map[string]any{"container": "pair", "entity": "ivan", "role": "partner"},
		},
	}, http.StatusCreated)
	op := r.body["operation_id"].(string)
	anna, pair := created(t, r, "anna"), created(t, r, "pair")

	p := auditPageOf(t, c, dash, url.Values{"operation": {op}})
	if len(p.Operations) != 1 || p.Operations[0].OperationID != op || len(p.Operations[0].Entries) < 4 ||
		p.Operations[0].ActorLogin != "owner" || p.Operations[0].Source != "ui" {
		t.Fatalf("operation %+v", p)
	}
	objects := map[string]int{}
	for _, en := range p.Operations[0].Entries {
		objects[en.Object]++
	}
	if objects["entity"] != 3 || objects["participation"] != 2 {
		t.Fatalf("objects %v", objects)
	}

	// изменение значения записи — отдельной операцией
	addValue(t, c, anna, "surname", map[string]any{"value": "Василькова", "if_rev": 0})

	// история записи: её значения и её участия (в чужом контейнере)
	h := auditPageOf(t, c, dash, url.Values{"entity": {anna}})
	seen := map[string]bool{}
	for _, o := range h.Operations {
		for _, en := range o.Entries {
			seen[en.Object+"/"+en.Op] = true
			if en.Object != "participation" && en.EntityID != anna {
				t.Fatalf("foreign entry in history: %+v", en)
			}
		}
	}
	if len(h.Operations) != 2 || !seen["entity/create"] || !seen["field_entry/create"] || !seen["participation/create"] {
		t.Fatalf("history %v (%d ops)", seen, len(h.Operations))
	}
	if h.Operations[0].Entries[0].Object != "field_entry" {
		t.Fatalf("newest first: %+v", h.Operations[0])
	}
	// история отношения — свои участники
	if hp := auditPageOf(t, c, dash, url.Values{"entity": {pair}}); len(hp.Operations) != 1 || len(hp.Operations[0].Entries) != 3 {
		t.Fatalf("pair history %+v", hp)
	}

	// курсор: по одной операции на страницу, без повторов
	all := auditPageOf(t, c, dash, url.Values{})
	got := []string{}
	q := url.Values{"limit": {"1"}}
	for range 20 {
		pg := auditPageOf(t, c, dash, q)
		for _, o := range pg.Operations {
			got = append(got, o.OperationID)
		}
		if pg.Next == nil {
			break
		}
		q.Set("before", *pg.Next)
	}
	if len(got) != len(all.Operations) || len(got) < 3 {
		t.Fatalf("paged %d, all %d", len(got), len(all.Operations))
	}
	for i := range got {
		if got[i] != all.Operations[i].OperationID {
			t.Fatalf("page order %d", i)
		}
	}

	// упомянутые записи — с конвертами (и участник участия), запись в корзине тоже
	if all.Records[anna] == nil || all.Records[pair] == nil {
		t.Fatalf("records %v", all.Records)
	}
	expect(t, c.do("DELETE", "/api/records/"+anna, nil), http.StatusOK, "delete")
	if hd := auditPageOf(t, c, dash, url.Values{"entity": {anna}}); hd.Records[anna] == nil || hd.Records[anna]["deleted"] != true {
		t.Fatalf("deleted record in audit %v", hd.Records[anna])
	}
	expect(t, c.do("DELETE", "/api/records/"+anna+"/purge", nil), http.StatusNoContent, "purge")
	if hp := auditPageOf(t, c, dash, url.Values{"entity": {anna}}); hp.Records[anna] != nil || len(hp.Operations) < 3 {
		t.Fatalf("purged record: %d ops, %v", len(hp.Operations), hp.Records[anna])
	}
	// F-94: последнее состояние очищенной записи — по журналу, для названия
	hp := auditPageOf(t, c, dash, url.Values{"entity": {anna}})
	if v := values(hp.Purged[anna], "name"); len(v) != 1 || v[0]["value"] != "Анна" || hp.Purged[anna]["deleted"] != true {
		t.Fatalf("purged snapshot %v", hp.Purged[anna])
	}

	// фильтры: действие и автор (system — изменения системы)
	for _, o := range auditPageOf(t, c, dash, url.Values{"op": {"purge"}}).Operations {
		for _, e := range o.Entries {
			if e.Op != "purge" {
				t.Fatalf("op filter: %s", e.Op)
			}
		}
	}
	if pg := auditPageOf(t, c, dash, url.Values{"op": {"purge"}}); len(pg.Operations) != 1 {
		t.Fatalf("purge ops %d", len(pg.Operations))
	}
	for _, o := range auditPageOf(t, c, dash, url.Values{"actor": {"system"}}).Operations {
		if o.ActorLogin != "" {
			t.Fatalf("system filter: %s", o.ActorLogin)
		}
	}
	expect(t, c.do("GET", "/api/dashboards/"+dash+"/audit?op=x", nil), http.StatusBadRequest, "bad op")

	expect(t, c.do("GET", "/api/dashboards/"+dash+"/audit?entity=x", nil), http.StatusBadRequest, "bad entity")
	expect(t, c.do("GET", "/api/dashboards/"+dash+"/audit?before=x", nil), http.StatusBadRequest, "bad cursor")
	expect(t, c.do("GET", "/api/dashboards/00000000-0000-0000-0000-000000000000/audit", nil), http.StatusNotFound, "foreign")
}
