package api_test

import (
	"net/http"
	"testing"
)

// F-139: своё поле — в любом блоке карточки; свои блоки записи и общие блоки
// шаблона в дашборде; блок с полями не удаляется.
func TestCustomSections(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	id := person(t, c, dash, nil)
	other := person(t, c, dash, nil)
	patch := func(rid string, rev int, body map[string]any, status int, what string) {
		t.Helper()
		body["if_rev"] = rev
		expect(t, c.do("PATCH", "/api/records/"+rid, body), status, what)
	}
	field := func(key, section string) map[string]any {
		f := map[string]any{"key": key, "type": "text", "name": key}
		if section != "" {
			f["section"] = section
		}
		return f
	}
	block := func(key, name string) map[string]any { return map[string]any{"key": key, "name": name} }

	// поле в блоке шаблона и в своём блоке — одним запросом с новым блоком
	patch(id, 0, map[string]any{
		"custom_sections": []any{block("war", "Война")},
		"custom_fields":   []any{field("a", "additional"), field("b", "main"), field("c", "war")},
	}, http.StatusOK, "fields in template and own sections")
	rec := record(c.do("GET", "/api/records/"+id, nil))
	if s, _ := rec["custom_sections"].([]any); len(s) != 1 {
		t.Fatalf("custom_sections %v", rec["custom_sections"])
	}
	patch(id, 1, map[string]any{"custom_fields": []any{field("d", "nope")}}, http.StatusBadRequest, "unknown section")
	patch(id, 1, map[string]any{"custom_sections": []any{}}, http.StatusBadRequest, "remove section with fields")
	patch(id, 1, map[string]any{"custom_sections": []any{block("main", "X")}}, http.StatusBadRequest, "reserved key")
	patch(id, 1, map[string]any{"custom_sections": []any{block("war", "")}}, http.StatusBadRequest, "no name")
	patch(id, 1, map[string]any{"custom_sections": []any{block("war", "Война"), block("war", "Ещё")}}, http.StatusBadRequest, "dup key")
	// перенести поле и удалить пустой блок; переименовать блок
	patch(id, 1, map[string]any{
		"custom_sections": []any{block("war", "Служба"), block("empty", "Пустой")},
		"custom_fields":   []any{field("a", "additional"), field("b", "main"), field("c", "")},
	}, http.StatusOK, "move field out, rename, add empty")
	patch(id, 2, map[string]any{"custom_sections": []any{}}, http.StatusOK, "remove empty sections")
	// блок другой записи не виден этой
	patch(other, 0, map[string]any{"custom_fields": []any{field("x", "war")}}, http.StatusBadRequest, "other record's section")

	// общие блоки шаблона
	tmpl, _ := rec["template_id"].(string)
	url := "/api/dashboards/" + dash + "/templates/" + tmpl + "/custom_fields"
	put := func(rev int, body map[string]any, status int, what string) {
		t.Helper()
		body["if_rev"] = rev
		expect(t, c.do("PUT", url, body), status, what)
	}
	put(0, map[string]any{"custom_fields": []any{field("s", "fam")}}, http.StatusBadRequest, "shared field in unknown section")
	put(0, map[string]any{"custom_sections": []any{block("fam", "Семья")}, "custom_fields": []any{field("s", "fam")}},
		http.StatusOK, "shared section")
	// своё поле записи — в общем блоке; ключ общего блока у записи занят
	patch(other, 0, map[string]any{"custom_fields": []any{field("y", "fam")}}, http.StatusOK, "record field in shared section")
	patch(other, 1, map[string]any{"custom_sections": []any{block("fam", "Дубль")}}, http.StatusBadRequest, "shared key taken")
	// и наоборот: общий блок не занимает ключ блока записи
	patch(id, 3, map[string]any{"custom_sections": []any{block("mine", "Моё")}}, http.StatusOK, "record section")
	put(1, map[string]any{"custom_sections": []any{block("fam", "Семья"), block("mine", "Дубль")}, "custom_fields": []any{field("s", "fam")}},
		http.StatusBadRequest, "record key taken")
	// конфликт версии при правке блоков — текущие поля и блоки
	r := c.do("PATCH", "/api/records/"+id, map[string]any{"if_rev": 0, "custom_sections": []any{}})
	expect(t, r, http.StatusConflict, "stale rev")
	cur, _ := r.body["current"].(map[string]any)
	if v, _ := cur["value"].(map[string]any); v == nil || v["custom_sections"] == nil || v["custom_fields"] == nil {
		t.Fatalf("conflict body %v", r.body)
	}
	// общий блок с полем записи не удаляется, даже когда общие поля ушли
	put(1, map[string]any{"custom_fields": []any{}}, http.StatusOK, "keep sections when key absent")
	put(2, map[string]any{"custom_sections": []any{}, "custom_fields": []any{}}, http.StatusBadRequest, "shared section used by record")
	patch(other, 1, map[string]any{"custom_fields": []any{}}, http.StatusOK, "drop record field")
	put(2, map[string]any{"custom_sections": []any{}, "custom_fields": []any{}}, http.StatusOK, "remove unused shared section")
}
