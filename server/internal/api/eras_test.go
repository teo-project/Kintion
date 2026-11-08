package api_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
)

// erasOf — эры дашборда из схемы.
func erasOf(t *testing.T, c *client, dash string) map[string]any {
	t.Helper()
	var s struct {
		Eras map[string]any `json:"eras"`
	}
	c.getJSON("/api/dashboards/"+dash+"/schema", &s)
	return s.Eras
}

func strs(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func startOf(t *testing.T, c *client, id string) map[string]any {
	t.Helper()
	rec := record(c.do("GET", "/api/records/"+id, nil))
	start, _ := rec["start"].(map[string]any)
	v, _ := start["value"].(map[string]any)
	return v
}

// replacesIn — замены эры в последней операции журнала: «from→to» → число
// дат; второе — число записей операции.
func replacesIn(t *testing.T, c *client, dash string) (map[string]float64, int) {
	t.Helper()
	p := auditPageOf(t, c, dash, url.Values{})
	out := map[string]float64{}
	entries := p.Operations[0].Entries
	for _, e := range entries {
		if e.Object != "dashboard" {
			t.Fatalf("entry %+v", e)
		}
		if rep, ok := e.After["replace"].(map[string]any); ok {
			out[rep["from"].(string)+"→"+rep["to"].(string)] = rep["changed"].(float64)
		}
	}
	return out, len(entries)
}

// Эры дашборда (F-143): список всех эр (новые — наверху), порядок, замена
// эры во всех датах одной операцией журнала с числом изменённых дат.
func TestDashboardEras(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	date := func(v map[string]any) map[string]any { return map[string]any{"start": map[string]any{"value": v}} }
	p1 := person(t, c, dash, date(map[string]any{"y": 300, "e": "ACC"}))
	p2 := person(t, c, dash, date(map[string]any{"y": 1990}))
	p3 := person(t, c, dash, date(map[string]any{"m": 3}))
	// старая эра в двух датах одной записи — меняются обе
	p4 := person(t, c, dash, map[string]any{"start": map[string]any{"value": map[string]any{"y": 280, "e": "ACC"}},
		"end": map[string]any{"value": map[string]any{"y": 299, "e": "ACC"}}})

	er := erasOf(t, c, dash)
	if er["show"] != false || er["edit"] != false || er["default"] != "" || er["today"] != nil ||
		!slices.Equal(strs(er["order"]), []string{}) && er["order"] != nil || !slices.Equal(strs(er["list"]), []string{"", "ACC"}) {
		t.Fatalf("default eras %v", er)
	}

	path := "/api/dashboards/" + dash + "/eras"
	r := c.do("PUT", path, map[string]any{"if_rev": 0, "eras": map[string]any{"show": true, "edit": true, "order": []string{"ACC", ""},
		"today": map[string]any{"y": 305, "e": "ACC"}, "years": map[string]any{"ACC": map[string]any{"min": 1, "max": 400}}}})
	expect(t, r, http.StatusOK, "save")
	if !slices.Equal(strs(r.body["list"]), []string{"ACC", ""}) || r.body["default"] != "" {
		t.Fatalf("saved %v", r.body)
	}
	// новая эра — наверху, пока её не переставят
	person(t, c, dash, date(map[string]any{"y": 5, "e": "BC"}))
	if l := strs(erasOf(t, c, dash)["list"]); !slices.Equal(l, []string{"BC", "ACC", ""}) {
		t.Fatalf("list with new era %v", l)
	}

	// переименование ACC → AC: даты, порядок, годы эры, год мира
	r = c.do("PUT", path, map[string]any{"if_rev": 1, "eras": map[string]any{"show": true, "edit": true, "order": []string{"ACC", ""},
		"today": map[string]any{"y": 305, "e": "ACC"}, "years": map[string]any{"ACC": map[string]any{"min": 1, "max": 400}}},
		"replace": map[string]any{"from": "ACC", "to": "AC"}})
	expect(t, r, http.StatusOK, "rename")
	years, _ := r.body["years"].(map[string]any)
	if !slices.Equal(strs(r.body["order"]), []string{"AC", ""}) || r.body["today"].(map[string]any)["e"] != "AC" || years["AC"] == nil || years["ACC"] != nil {
		t.Fatalf("renamed %v", r.body)
	}
	if v := startOf(t, c, p1); v["e"] != "AC" || v["y"] != float64(300) {
		t.Fatalf("p1 %v", v)
	}
	rec4 := record(c.do("GET", "/api/records/"+p4, nil))
	if v := startOf(t, c, p4); v["e"] != "AC" {
		t.Fatalf("p4 start %v", v)
	}
	if end, _ := rec4["end"].(map[string]any)["value"].(map[string]any); end["e"] != "AC" {
		t.Fatalf("p4 end %v", rec4["end"])
	}
	if v := startOf(t, c, p2); v["e"] != nil {
		t.Fatalf("p2 %v", v)
	}
	// журнал: правка настроек и замена с числом изменённых дат
	if reps, n := replacesIn(t, c, dash); n != 2 || len(reps) != 1 || reps["ACC→AC"] != 3 {
		t.Fatalf("audit %v %d", reps, n)
	}

	// наша эра → AC «к уже созданным»: датированные без эры получают AC, без года — нет;
	// наша эра из списка уходит (её больше нет ни в датах, ни по умолчанию)
	r = c.do("PUT", path, map[string]any{"if_rev": 2, "eras": map[string]any{"default": "", "order": []string{"AC", ""}},
		"replace": map[string]any{"from": "", "to": "AC"}})
	expect(t, r, http.StatusOK, "default existing")
	if v := startOf(t, c, p2); v["e"] != "AC" {
		t.Fatalf("p2 %v", v)
	}
	if v := startOf(t, c, p3); v["e"] != nil || v["m"] != float64(3) {
		t.Fatalf("p3 %v", v)
	}
	if !slices.Equal(strs(r.body["order"]), []string{"AC"}) || r.body["default"] != "AC" {
		t.Fatalf("order %v", r.body)
	}
	if l := strs(erasOf(t, c, dash)["list"]); !slices.Equal(l, []string{"BC", "AC"}) {
		t.Fatalf("list %v", l)
	}
	var got map[string]any
	c.getJSON(path, &got)
	if !slices.Equal(strs(got["list"]), []string{"BC", "AC"}) || got["rev"] != float64(3) {
		t.Fatalf("GET eras %v", got)
	}

	// новой дате с пустой эрой база ставит эру по умолчанию (триггер)
	p5 := person(t, c, dash, date(map[string]any{"y": 10}))
	if v := startOf(t, c, p5); v["e"] != "AC" {
		t.Fatalf("p5 %v", v)
	}
	// замена на пустую при непустой эре по умолчанию — «без эры», не отказ
	bc := person(t, c, dash, date(map[string]any{"y": 7, "e": "BC"}))
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 3, "eras": map[string]any{"default": "AC"},
		"replace": map[string]any{"from": "BC", "to": ""}}), http.StatusOK, "no era while default")
	if v := startOf(t, c, bc); v["e"] != nil || v["y"] != float64(7) {
		t.Fatalf("bc %v", v)
	}

	// дата из одной эры: в списке, переименовывается вместе с эрой
	p6 := person(t, c, dash, date(map[string]any{"e": "Мезозой"}))
	if v := startOf(t, c, p6); v["e"] != "Мезозой" || v["y"] != nil {
		t.Fatalf("p6 %v", v)
	}
	if l := strs(erasOf(t, c, dash)["list"]); !slices.Contains(l, "Мезозой") || !slices.Contains(l, "") {
		t.Fatalf("list with era-only %v", l)
	}
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 4, "eras": map[string]any{"default": "AC"},
		"replace": map[string]any{"from": "Мезозой", "to": "Mz"}}), http.StatusOK, "rename era-only")
	if v := startOf(t, c, p6); v["e"] != "Mz" {
		t.Fatalf("p6 renamed %v", v)
	}

	// дата из одной эры при замене на пустую очищается (F-143)
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 5, "eras": map[string]any{"default": "AC"},
		"replace": map[string]any{"from": "Mz", "to": ""}}), http.StatusOK, "era-only to empty")
	if v := startOf(t, c, p6); v != nil {
		t.Fatalf("p6 cleared %v", v)
	}
	if reps, _ := replacesIn(t, c, dash); reps["Mz→"] != 1 {
		t.Fatalf("audit era-only %v", reps)
	}

	// AC → наша «к уже созданным»: эра по умолчанию снова наша, даты AC — без эры
	r = c.do("PUT", path, map[string]any{"if_rev": 6, "eras": map[string]any{"default": "AC"},
		"replace": map[string]any{"from": "AC", "to": ""}})
	expect(t, r, http.StatusOK, "back to ours")
	if v := startOf(t, c, p2); v["e"] != nil || r.body["default"] != "" {
		t.Fatalf("p2 back %v %v", v, r.body)
	}

	// другая версия — конфликт (F-45)
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 1, "eras": map[string]any{"show": true}}), http.StatusConflict, "stale")
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 7, "eras": map[string]any{"order": []string{"AC", "AC"}}}), http.StatusBadRequest, "dup")
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 7, "eras": map[string]any{"today": map[string]any{"y": 5, "m": 3}}}), http.StatusBadRequest, "today")
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 7, "eras": map[string]any{"default": " AC"}}), http.StatusBadRequest, "default")
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 7, "eras": map[string]any{"years": map[string]any{"AC": map[string]any{"min": 0}}}}), http.StatusBadRequest, "year 0")
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 7, "eras": map[string]any{}, "replace": "AC"}), http.StatusBadRequest, "replace")
}

// Эра по умолчанию «только к новым»: прежние даты сохраняют свою эру, пустые
// остаются пустыми («без эры»); новые даты (INSERT или колонка была NULL)
// получают новую эру по умолчанию, существующие при правке — нет (F-143).
func TestErasDefaultNewOnly(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	date := func(v map[string]any) map[string]any { return map[string]any{"start": map[string]any{"value": v}} }
	ours := person(t, c, dash, date(map[string]any{"y": 1990}))
	undated := person(t, c, dash, nil)
	path := "/api/dashboards/" + dash + "/eras"
	// с нашей на AC только к новым: дата без эры остаётся без эры
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 0, "eras": map[string]any{"default": "AC"}}), http.StatusOK, "AC")
	if v := startOf(t, c, ours); v["e"] != nil {
		t.Fatalf("ours %v", v)
	}
	// в списке — и пустая (она используется), и эра по умолчанию
	if l := strs(erasOf(t, c, dash)["list"]); !slices.Equal(l, []string{"", "AC"}) {
		t.Fatalf("list %v", l)
	}
	// правка существующей даты эру по умолчанию не ставит
	expect(t, c.do("PATCH", "/api/records/"+ours, map[string]any{"if_rev": 0, "start": map[string]any{"value": map[string]any{"y": 1991}}}), http.StatusOK, "edit ours")
	if v := startOf(t, c, ours); v["e"] != nil || v["y"] != float64(1991) {
		t.Fatalf("ours edited %v", v)
	}
	// дата появилась в строке (колонка была NULL) — новая: эра по умолчанию
	expect(t, c.do("PATCH", "/api/records/"+undated, map[string]any{"if_rev": 0, "start": map[string]any{"value": map[string]any{"y": 5}}}), http.StatusOK, "date added")
	if v := startOf(t, c, undated); v["e"] != "AC" {
		t.Fatalf("undated %v", v)
	}
	// с AC на BC только к новым: прежние — AC, новая — BC
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 1, "eras": map[string]any{"default": "BC"}}), http.StatusOK, "BC")
	if v := startOf(t, c, undated); v["e"] != "AC" {
		t.Fatalf("undated after BC %v", v)
	}
	if v := startOf(t, c, person(t, c, dash, date(map[string]any{"y": 7}))); v["e"] != "BC" {
		t.Fatalf("new %v", v)
	}
	// новый год мира с пустой эрой — в эре по умолчанию
	r := c.do("PUT", path, map[string]any{"if_rev": 2, "eras": map[string]any{"default": "BC", "today": map[string]any{"y": 9}}})
	expect(t, r, http.StatusOK, "today")
	if r.body["today"].(map[string]any)["e"] != "BC" {
		t.Fatalf("today %v", r.body["today"])
	}
	// пустая → BC «к уже созданным»: дата без эры получает BC
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 3, "eras": map[string]any{"default": "BC", "today": map[string]any{"y": 9, "e": "BC"}},
		"replace": map[string]any{"from": "", "to": "BC"}}), http.StatusOK, "empty to BC")
	if v := startOf(t, c, ours); v["e"] != "BC" {
		t.Fatalf("ours to BC %v", v)
	}
}

// Замена эры повышает версии изменённых полей (F-45, KIN-48): карточка,
// открытая до замены, не может молча вернуть прежнюю эру — 409. Поля без
// старой эры версии не меняют.
func TestErasReplaceBumpsRevs(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	zz := person(t, c, dash, map[string]any{"start": map[string]any{"value": map[string]any{"y": 3, "e": "ZZ"}},
		"end": map[string]any{"value": map[string]any{"y": 9}}})
	sid, _ := addValue(t, c, zz, "surname", map[string]any{"if_rev": 0, "value": "Петров", "valid_from": map[string]any{"y": 5, "e": "ZZ"}})
	// брак: партнёр с датой в ZZ; свидетель с датой в WW уйдёт в корзину
	anna, wit := person(t, c, dash, nil), person(t, c, dash, nil)
	w := created(t, batch(t, c, dash, map[string]any{
		"records": []any{map[string]any{"ref": "w", "template": "kin.core.wedding"}},
		"participants": []any{
			map[string]any{"container": "w", "entity": anna, "role": "partner", "valid_from": map[string]int{"y": 6}},
			map[string]any{"container": "w", "entity": zz, "role": "partner", "valid_from": map[string]any{"y": 6, "e": "ZZ"}},
			map[string]any{"container": "w", "entity": wit, "role": "witness", "valid_from": map[string]any{"y": 6, "e": "WW"}},
		},
	}, http.StatusCreated), "w")
	expect(t, c.do("DELETE", "/api/records/"+wit, nil), http.StatusOK, "trash witness")
	other := person(t, c, dash, map[string]any{"start": map[string]any{"value": map[string]any{"y": 4, "e": "QQ"}}})
	revs := func(id string) map[string]float64 {
		out := map[string]float64{}
		for k, v := range record(c.do("GET", "/api/records/"+id, nil))["revs"].(map[string]any) {
			out[k] = v.(float64)
		}
		return out
	}
	r0, s0, o0, w0 := revs(zz), fieldRev(record(c.do("GET", "/api/records/"+zz, nil)), "surname"), revs(other), revs(w)

	expect(t, c.do("PUT", "/api/dashboards/"+dash+"/eras", map[string]any{"if_rev": 0, "eras": map[string]any{},
		"replace": map[string]any{"from": "ZZ", "to": "YY"}}), http.StatusOK, "replace")
	r1, rec := revs(zz), record(c.do("GET", "/api/records/"+zz, nil))
	if r1["start"] != r0["start"]+1 || r1["end"] != r0["end"] || fieldRev(rec, "surname") != s0+1 {
		t.Fatalf("revs %v → %v, surname %v → %v", r0, r1, s0, fieldRev(rec, "surname"))
	}
	if o1 := revs(other); o1["start"] != o0["start"] {
		t.Fatalf("other %v → %v", o0, o1)
	}
	if w1 := revs(w); w1["participants:partner"] != w0["participants:partner"]+1 || w1["participants:witness"] != w0["participants:witness"] {
		t.Fatalf("wedding %v → %v", w0, w1)
	}
	// дата скрытого свидетеля меняется, а версия роли — нет: 409 отвечал бы по невидимому (F-07)
	expect(t, c.do("PUT", "/api/dashboards/"+dash+"/eras", map[string]any{"if_rev": 1, "eras": map[string]any{},
		"replace": map[string]any{"from": "WW", "to": "VV"}}), http.StatusOK, "replace hidden")
	if w2 := revs(w); w2["participants:witness"] != w0["participants:witness"] {
		t.Fatalf("witness rev %v → %v", w0, w2)
	}
	expect(t, c.do("PATCH", "/api/records/"+zz+"/fields/surname/values/"+sid, map[string]any{"if_rev": s0, "note": "старая карточка"}),
		http.StatusConflict, "stale value")
	expect(t, c.do("PATCH", "/api/records/"+zz, map[string]any{"if_rev": r0["start"], "start": map[string]any{"value": map[string]any{"y": 3, "e": "ZZ"}}}),
		http.StatusConflict, "stale start")
	if v := startOf(t, c, zz); v["e"] != "YY" {
		t.Fatalf("start %v", v)
	}
}

// Несколько замен в одном запросе (смена эры по умолчанию «к уже созданным» и
// переименование другой эры) — по записи журнала на каждую с числом дат.
func TestErasSeveralReplaces(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	date := func(v map[string]any) map[string]any { return map[string]any{"start": map[string]any{"value": v}} }
	ours := person(t, c, dash, date(map[string]any{"y": 1990}))
	zz := person(t, c, dash, date(map[string]any{"y": 3, "e": "ZZ"}))
	person(t, c, dash, date(map[string]any{"y": 4, "e": "ZZ"}))
	r := c.do("PUT", "/api/dashboards/"+dash+"/eras", map[string]any{"if_rev": 0, "eras": map[string]any{"order": []string{"ZZ", ""}},
		"replace": []any{map[string]any{"from": "", "to": "AC"}, map[string]any{"from": "ZZ", "to": "YY"}}})
	expect(t, r, http.StatusOK, "two replaces")
	if r.body["default"] != "AC" || !slices.Equal(strs(r.body["order"]), []string{"YY", "AC"}) {
		t.Fatalf("eras %v", r.body)
	}
	if v := startOf(t, c, ours); v["e"] != "AC" {
		t.Fatalf("ours %v", v)
	}
	if v := startOf(t, c, zz); v["e"] != "YY" {
		t.Fatalf("zz %v", v)
	}
	if reps, n := replacesIn(t, c, dash); n != 3 || reps["→AC"] != 1 || reps["ZZ→YY"] != 2 {
		t.Fatalf("audit %v %d", reps, n)
	}
}

// Серверная сортировка участий по началу контейнера — по паре «место эры, год» (F-143).
func TestErasServerSort(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	anna, ivan := person(t, c, dash, nil), person(t, c, dash, nil)
	wedding := func(start map[string]any) string {
		r := batch(t, c, dash, map[string]any{
			"records": []any{map[string]any{"ref": "w", "template": "kin.core.wedding", "start": map[string]any{"value": start}}},
			"participants": []any{
				map[string]any{"container": "w", "entity": anna, "role": "partner"},
				map[string]any{"container": "w", "entity": ivan, "role": "partner"},
			},
		}, http.StatusCreated)
		return created(t, r, "w")
	}
	ours := wedding(map[string]any{"y": 1990})
	ac := wedding(map[string]any{"y": 300, "e": "AC"})
	order := func() []string {
		in, _ := record(c.do("GET", "/api/records/"+anna, nil))["participates_in"].([]any)
		var ids []string
		for _, x := range in {
			ids = append(ids, x.(map[string]any)["container_id"].(string))
		}
		return ids
	}
	bc5 := wedding(map[string]any{"y": 5, "e": "BC"})
	bc500 := wedding(map[string]any{"y": 500, "e": "BC"})
	// по умолчанию: наша эра раньше остальных, остальные — по тексту
	if got := order(); !slices.Equal(got, []string{ours, ac, bc5, bc500}) {
		t.Fatalf("default order %v", got)
	}
	// порядок эр и обратный счёт в BC (15000 … 1): больший год — раньше
	expect(t, c.do("PUT", "/api/dashboards/"+dash+"/eras", map[string]any{"if_rev": 0, "eras": map[string]any{"order": []string{"BC", "AC", ""},
		"years": map[string]any{"BC": map[string]any{"min": 15000, "max": 1}}}}), http.StatusOK, "order")
	if got := order(); !slices.Equal(got, []string{bc500, bc5, ac, ours}) {
		t.Fatalf("era order %v", got)
	}
}

// Эра года мира без дат в ней в список эр не попадает (F-143).
func TestErasListTodayEra(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	expect(t, c.do("PUT", "/api/dashboards/"+dash+"/eras", map[string]any{"if_rev": 0, "eras": map[string]any{"today": map[string]any{"y": 300, "e": "AC"}}}), http.StatusOK, "today")
	if l := strs(erasOf(t, c, dash)["list"]); !slices.Equal(l, []string{""}) {
		t.Fatalf("list %v", l)
	}
}

// Эра только у записи в корзине в список эр не попадает (F-07).
func TestErasListVisibleOnly(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	p := person(t, c, dash, map[string]any{"start": map[string]any{"value": map[string]any{"y": 3, "e": "ZZ"}}})
	if l := strs(erasOf(t, c, dash)["list"]); !slices.Equal(l, []string{"", "ZZ"}) {
		t.Fatalf("list %v", l)
	}
	expect(t, c.do("DELETE", "/api/records/"+p, nil), http.StatusOK, "trash")
	if l := strs(erasOf(t, c, dash)["list"]); !slices.Equal(l, []string{""}) {
		t.Fatalf("list after trash %v", l)
	}
	// замена меняет и дату в корзине, но в число изменённых она не входит
	expect(t, c.do("PUT", "/api/dashboards/"+dash+"/eras", map[string]any{"if_rev": 0, "eras": map[string]any{},
		"replace": map[string]any{"from": "ZZ", "to": "YY"}}), http.StatusOK, "replace")
	if reps, _ := replacesIn(t, c, dash); reps["ZZ→YY"] != 0 {
		t.Fatalf("audit %v", reps)
	}
	expect(t, c.do("POST", "/api/records/"+p+"/restore", nil), http.StatusOK, "restore")
	if v := startOf(t, c, p); v["e"] != "YY" {
		t.Fatalf("restored %v", v)
	}
}

// Дата из одной эры в корзине при замене на пустую тоже очищается (F-143);
// в число изменённых не входит (F-07).
func TestErasEraOnlyInTrash(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	p := person(t, c, dash, map[string]any{"start": map[string]any{"value": map[string]any{"e": "Mz"}}})
	expect(t, c.do("DELETE", "/api/records/"+p, nil), http.StatusOK, "trash")
	expect(t, c.do("PUT", "/api/dashboards/"+dash+"/eras", map[string]any{"if_rev": 0, "eras": map[string]any{},
		"replace": map[string]any{"from": "Mz", "to": ""}}), http.StatusOK, "replace")
	if reps, _ := replacesIn(t, c, dash); reps["Mz→"] != 0 {
		t.Fatalf("audit %v", reps)
	}
	expect(t, c.do("POST", "/api/records/"+p+"/restore", nil), http.StatusOK, "restore")
	if v := startOf(t, c, p); v != nil {
		t.Fatalf("restored %v", v)
	}
}

// Пределы года эры (F-142, F-143): новая или изменённая дата вне пределов
// своей эры — 400; новая с пустой эрой — по эре по умолчанию; правка другой
// части записи со старой датой вне пределов проходит.
func TestErasYearBounds(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	old := person(t, c, dash, map[string]any{"start": map[string]any{"value": map[string]any{"y": 900, "e": "AC"}}})
	expect(t, c.do("PUT", "/api/dashboards/"+dash+"/eras", map[string]any{"if_rev": 0, "eras": map[string]any{
		"default": "AC", "years": map[string]any{"AC": map[string]any{"min": 1, "max": 400}, "BC": map[string]any{"min": 5000, "max": 1}}}}),
		http.StatusOK, "years")
	start := func(v map[string]any) map[string]any { return map[string]any{"start": map[string]any{"value": v}} }
	r := batch(t, c, dash, map[string]any{"records": []any{map[string]any{"ref": "p", "template": "kin.core.person", "start": start(map[string]any{"y": 500})["start"]}}}, http.StatusBadRequest)
	if m, _ := r.body["message"].(string); !strings.Contains(m, "вне пределов эры") {
		t.Fatalf("batch %v", r.body)
	}
	person(t, c, dash, start(map[string]any{"y": 300}))
	// обратный счёт: 4000 BC — в пределах 1 … 5000
	person(t, c, dash, start(map[string]any{"y": 4000, "e": "BC"}))
	expect(t, c.do("PATCH", "/api/records/"+old, map[string]any{"if_rev": 0, "note": "старая дата вне пределов"}), http.StatusOK, "note")
	// та же дата, присланная повторно, — не изменение
	sr := revOf(record(c.do("GET", "/api/records/"+old, nil)), "start")
	expect(t, c.do("PATCH", "/api/records/"+old, map[string]any{"if_rev": sr, "start": map[string]any{"value": map[string]any{"y": 900, "e": "AC"}}}),
		http.StatusOK, "same date")
	expect(t, c.do("PATCH", "/api/records/"+old, map[string]any{"if_rev": sr + 1, "start": map[string]any{"value": map[string]any{"y": 901, "e": "AC"}}}),
		http.StatusBadRequest, "changed date out of range")
}
