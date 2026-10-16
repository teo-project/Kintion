package api_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"kintion/server/internal/authz"
	"kintion/server/internal/db/sqlc"
)

// group создаёт группу; parents — «Входит в» (F-105).
func group(t *testing.T, c *client, dash, name string, parents ...string) string {
	t.Helper()
	r := c.do("POST", "/api/dashboards/"+dash+"/groups", map[string]any{"name": name, "parents": parents})
	expect(t, r, http.StatusCreated, "group "+name)
	return r.body["id"].(string)
}

// list — id записей списка с фильтрами.
func list(t *testing.T, c *client, dash string, q url.Values) map[string]bool {
	t.Helper()
	r := c.do("GET", "/api/dashboards/"+dash+"/records?"+q.Encode(), nil)
	expect(t, r, http.StatusOK, "list "+q.Encode())
	items, _ := r.body["items"].([]any)
	out := map[string]bool{}
	for _, it := range items {
		out[it.(map[string]any)["id"].(string)] = true
	}
	if q.Get("limit") == "" && r.body["total"] != float64(len(items)) {
		t.Fatalf("total %v, items %d", r.body["total"], len(items))
	}
	return out
}

func groupsOf(t *testing.T, c *client, dash string) map[string]map[string]any {
	t.Helper()
	list := c.doList("/api/dashboards/" + dash + "/groups")
	out := map[string]map[string]any{}
	for _, g := range list {
		out[g["id"].(string)] = g
	}
	return out
}

func TestGroupsListSearch(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	family := group(t, c, dash, "Семья")
	vas := group(t, c, dash, "Васильковы", family)
	work := group(t, c, dash, "Работа")

	r := batch(t, c, dash, map[string]any{"records": []any{
		map[string]any{"ref": "liza", "entity": "kin.core.person", "groups": []any{vas},
			"fields": map[string]any{
				"name":    []any{map[string]any{"value": "Лиза"}},
				"surname": []any{map[string]any{"value": "Василькова"}, map[string]any{"value": "Давыдко"}},
			}},
		map[string]any{"ref": "ivan", "entity": "kin.core.person", "groups": []any{work}, "note": "коллега 100%"},
	}}, http.StatusCreated)
	liza, ivan := created(t, r, "liza"), created(t, r, "ivan")
	w := wedding(t, c, dash)

	// фильтр по родительской группе находит записи дочерней
	if got := list(t, c, dash, url.Values{"group": {family}}); !got[liza] || len(got) != 1 {
		t.Fatalf("family %v", got)
	}
	if got := list(t, c, dash, url.Values{"group": {work}}); !got[ivan] || len(got) != 1 {
		t.Fatalf("work %v", got)
	}
	// поиск по исторической фамилии и по примечанию; % — не шаблон
	if got := list(t, c, dash, url.Values{"q": {"давыд"}}); !got[liza] || len(got) != 1 {
		t.Fatalf("search %v", got)
	}
	if got := list(t, c, dash, url.Values{"q": {"100%"}}); !got[ivan] || len(got) != 1 {
		t.Fatalf("search note %v", got)
	}
	if got := list(t, c, dash, url.Values{"q": {"_"}}); len(got) != 0 {
		t.Fatalf("wildcard %v", got)
	}
	if got := list(t, c, dash, url.Values{"entity": {"kin.core.event"}}); !got[w] || len(got) != 1 {
		t.Fatalf("events %v", got)
	}
	if got := list(t, c, dash, url.Values{"limit": {"2"}}); len(got) != 2 {
		t.Fatalf("page %v", got)
	}
	// пустая страница за концом списка — total тот же
	if r := c.do("GET", "/api/dashboards/"+dash+"/records?offset=100", nil); r.body["total"] != float64(5) || len(r.body["items"].([]any)) != 0 {
		t.Fatalf("empty page %v", r.body)
	}
	// счётчики — только по видимому (F-07)
	if gs := groupsOf(t, c, dash); gs[vas]["count"] != float64(1) || gs[family]["count"] != float64(0) {
		t.Fatalf("counts %v", gs)
	}
	// корзина — отдельно
	expect(t, c.do("DELETE", "/api/records/"+ivan, nil), http.StatusOK, "delete")
	if got := list(t, c, dash, url.Values{"group": {work}}); len(got) != 0 {
		t.Fatalf("deleted listed %v", got)
	}
	if got := list(t, c, dash, url.Values{"deleted": {"1"}}); !got[ivan] || len(got) != 1 {
		t.Fatalf("trash %v", got)
	}
	if gs := groupsOf(t, c, dash); gs[work]["count"] != float64(0) {
		t.Fatalf("deleted counted %v", gs[work])
	}

	// метки записи — с версией поля groups
	path := "/api/records/" + liza + "/groups"
	r = c.do("PUT", path, map[string]any{"if_rev": 0, "groups": []any{vas, work}})
	expect(t, r, http.StatusOK, "set groups")
	if g := record(r)["groups"].([]any); len(g) != 2 {
		t.Fatalf("groups %v", g)
	}
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 0, "groups": []any{}}), http.StatusConflict, "stale groups")
	expect(t, c.do("PUT", path, map[string]any{"if_rev": 1, "groups": []any{w}}), http.StatusBadRequest, "not a group")

	// вложенность без циклов (F-105)
	gpath := "/api/dashboards/" + dash + "/groups/"
	expect(t, c.do("PATCH", gpath+family, map[string]any{"parents": []any{vas}}), http.StatusBadRequest, "cycle")
	expect(t, c.do("PATCH", gpath+vas, map[string]any{"children": []any{family}}), http.StatusBadRequest, "cycle via children")
	expect(t, c.do("PATCH", gpath+family, map[string]any{"parents": []any{family}}), http.StatusBadRequest, "self parent")
	r = c.do("PATCH", gpath+work, map[string]any{"name": "Коллеги", "parents": []any{family}})
	expect(t, r, http.StatusOK, "move")
	if r.body["name"] != "Коллеги" || len(r.body["parents"].([]any)) != 1 || r.body["parents"].([]any)[0] != family {
		t.Fatalf("patched %v", r.body)
	}
	if gs := groupsOf(t, c, dash); len(gs[family]["children"].([]any)) != 2 {
		t.Fatalf("children %v", gs[family])
	}
	// удаление группы: дети теряют её как родителя, записи остаются, метки снимаются
	expect(t, c.do("DELETE", gpath+family, nil), http.StatusNoContent, "delete family")
	gs := groupsOf(t, c, dash)
	if len(gs) != 2 || len(gs[vas]["parents"].([]any)) != 0 || len(gs[work]["parents"].([]any)) != 0 {
		t.Fatalf("after delete %v", gs)
	}
	expect(t, c.do("DELETE", gpath+vas, nil), http.StatusNoContent, "delete vas")
	rec := record(c.do("GET", "/api/records/"+liza, nil))
	if g := rec["groups"].([]any); len(g) != 1 || g[0] != work || revOf(rec, "groups") != 2 {
		t.Fatalf("labels after delete %v rev %v", g, revOf(rec, "groups"))
	}

	// журнал: группы (3 создания, переименование, 2 удаления) и вложенность
	// (vas→family, work→family; обе удалены с «Семьёй»)
	if n := e.count(t, "SELECT count(*) FROM audit_log WHERE object = 'grp'"); n != 3+1+2 {
		t.Fatalf("group audit %d", n)
	}
	if n := e.count(t, "SELECT count(*) FROM audit_log WHERE object = 'grp_link'"); n != 2+2 {
		t.Fatalf("group link audit %d", n)
	}
	// метки: 2 при создании, +1 PUT, −1 при удалении «Васильковых»
	if n := e.count(t, "SELECT count(*) FROM audit_log WHERE object = 'entity_group'"); n != 4 {
		t.Fatalf("label audit %d", n)
	}
	// чужой дашборд и группа — 404
	other := c.do("POST", "/api/dashboards", map[string]string{"name": "Другой"})
	expect(t, c.do("PATCH", "/api/dashboards/"+other.body["id"].(string)+"/groups/"+work, map[string]any{"name": "x"}),
		http.StatusNotFound, "foreign group")
}

// TestGroupsManyToMany — группа в нескольких группах (F-105): «Петровы» — и в
// «Семье», и в «Гостях со стороны невесты». Доступ — через любого из родителей
// (по всем путям), без них — нет; счётчики — по видимому (F-07).
func TestGroupsManyToMany(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	family := group(t, c, dash, "Семья")
	guests := group(t, c, dash, "Гости со стороны невесты")
	petrovs := group(t, c, dash, "Петровы", family, guests)
	kids := group(t, c, dash, "Дети Петровых", petrovs)
	work := group(t, c, dash, "Работа")

	gs := groupsOf(t, c, dash)
	if p := gs[petrovs]["parents"].([]any); len(p) != 2 || p[0] != family || p[1] != guests {
		t.Fatalf("parents %v", p)
	}
	if ch := gs[family]["children"].([]any); len(ch) != 1 || ch[0] != petrovs {
		t.Fatalf("children %v", ch)
	}

	r := batch(t, c, dash, map[string]any{"records": []any{
		map[string]any{"ref": "kid", "entity": "kin.core.person", "groups": []any{kids}, "note": "Петров-младший"},
		map[string]any{"ref": "boss", "entity": "kin.core.person", "groups": []any{work}, "note": "Начальник"},
	}}, http.StatusCreated)
	kid, boss := created(t, r, "kid"), created(t, r, "boss")

	// права наследуются вниз по всем путям
	dashID := uuid.MustParse(dash)
	under := func(roots ...string) map[string]bool {
		t.Helper()
		ids := make([]uuid.UUID, 0, len(roots))
		for _, s := range roots {
			ids = append(ids, uuid.MustParse(s))
		}
		got, err := authz.GroupsUnder(context.Background(), sqlc.New(e.pool), dashID, ids)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, id := range got {
			out[id.String()] = true
		}
		return out
	}
	for _, root := range []string{family, guests} {
		if g := under(root); !g[petrovs] || !g[kids] || g[work] || len(g) != 3 {
			t.Fatalf("via %s: %v", root, g)
		}
		if got := list(t, c, dash, url.Values{"group": {root}}); !got[kid] || got[boss] || len(got) != 1 {
			t.Fatalf("list via %s: %v", root, got)
		}
	}
	if g := under(work); g[petrovs] || g[kids] {
		t.Fatalf("no path: %v", g)
	}
	if got := list(t, c, dash, url.Values{"group": {work}}); got[kid] || !got[boss] {
		t.Fatalf("list work %v", got)
	}
	// чужой дашборд: его группы через свой дашборд не достать
	other := c.do("POST", "/api/dashboards", map[string]string{"name": "Другой"})
	if g := (func() []uuid.UUID {
		got, err := authz.GroupsUnder(context.Background(), sqlc.New(e.pool), uuid.MustParse(other.body["id"].(string)), []uuid.UUID{uuid.MustParse(family)})
		if err != nil {
			t.Fatal(err)
		}
		return got
	})(); len(g) != 0 {
		t.Fatalf("foreign dashboard %v", g)
	}

	// одна из связей снята — доступ через вторую остаётся
	expect(t, c.do("PATCH", "/api/dashboards/"+dash+"/groups/"+petrovs, map[string]any{"parents": []any{guests}}), http.StatusOK, "unlink family")
	if g := under(family); g[petrovs] || g[kids] {
		t.Fatalf("after unlink family: %v", g)
	}
	if g := under(guests); !g[petrovs] || !g[kids] {
		t.Fatalf("after unlink guests: %v", g)
	}
	// цикл через несколько ступеней отвергается, состояние не меняется
	expect(t, c.do("PATCH", "/api/dashboards/"+dash+"/groups/"+guests, map[string]any{"parents": []any{kids}}), http.StatusBadRequest, "deep cycle")
	expect(t, c.do("PATCH", "/api/dashboards/"+dash+"/groups/"+kids, map[string]any{"children": []any{guests, work}}), http.StatusBadRequest, "cycle via children")
	gs = groupsOf(t, c, dash)
	if len(gs[guests]["parents"].([]any)) != 0 || len(gs[kids]["children"].([]any)) != 0 {
		t.Fatalf("cycle left links %v %v", gs[guests], gs[kids])
	}
	// «Включает в себя» задаёт ту же связь с другой стороны
	expect(t, c.do("PATCH", "/api/dashboards/"+dash+"/groups/"+family, map[string]any{"children": []any{petrovs, work}}), http.StatusOK, "children")
	gs = groupsOf(t, c, dash)
	if len(gs[petrovs]["parents"].([]any)) != 2 || len(gs[work]["parents"].([]any)) != 1 {
		t.Fatalf("children set %v %v", gs[petrovs], gs[work])
	}
	// удаление промежуточной группы: её связи разрываются, вложенные
	// к её родителям не переходят (F-105)
	expect(t, c.do("DELETE", "/api/dashboards/"+dash+"/groups/"+petrovs, nil), http.StatusNoContent, "delete petrovs")
	gs = groupsOf(t, c, dash)
	if p := gs[kids]["parents"].([]any); len(p) != 0 {
		t.Fatalf("kids lifted %v", p)
	}
	if ch := gs[family]["children"].([]any); len(ch) != 1 || ch[0] != work {
		t.Fatalf("family children %v", ch)
	}
	if g := under(family); g[kids] {
		t.Fatalf("path not broken %v", g)
	}
}

// Список отношений (F-109): у строки — видимые участники, их записи — в related.
func TestListRelationParticipants(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	rel := withPartners(t, c, dash, "kin.core.relationship")
	row := func() (map[string]any, map[string]any) {
		t.Helper()
		r := c.do("GET", "/api/dashboards/"+dash+"/records?entity=kin.core.relationship", nil)
		expect(t, r, http.StatusOK, "list")
		items, _ := r.body["items"].([]any)
		if len(items) != 1 || items[0].(map[string]any)["id"] != rel {
			t.Fatalf("items %v", items)
		}
		related, _ := r.body["related"].(map[string]any)
		return items[0].(map[string]any), related
	}
	item, related := row()
	parts := participants(item, "partner")
	if len(parts) != 2 {
		t.Fatalf("participants %v", parts)
	}
	for _, p := range parts {
		if related[p["entity_id"].(string)] == nil {
			t.Fatalf("no related %v", p)
		}
	}
	// участник в корзине — не виден ни в строке, ни в related (F-07)
	gone := parts[0]["entity_id"].(string)
	expect(t, c.do("DELETE", "/api/records/"+gone, nil), http.StatusOK, "delete")
	item, related = row()
	if parts := participants(item, "partner"); len(parts) != 1 || parts[0]["entity_id"] == gone || related[gone] != nil {
		t.Fatalf("hidden participant leaked: %v", item)
	}
}

// Не-UTF-8 поиск — 400, а не 500 от PostgreSQL.
func TestListRecordsNotUTF8(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	for _, q := range []string{"q=%CF%F0%E8", "entity=%FF"} {
		r := c.do("GET", "/api/dashboards/"+dash+"/records?"+q, nil)
		expect(t, r, http.StatusBadRequest, q)
	}
}

// TestGroupMembersBulk — массовые метки группы (F-131): журнал и версия у
// каждой записи, одна операция; невидимое — 404, как у одиночной (F-07).
func TestGroupMembersBulk(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	g := group(t, c, dash, "Родня")
	a := person(t, c, dash, map[string]any{"note": "А"})
	b := person(t, c, dash, map[string]any{"note": "Б"})
	path := "/api/dashboards/" + dash + "/groups/" + g + "/records"

	r := c.do("POST", path, map[string]any{"add": []string{a, b}})
	expect(t, r, http.StatusOK, "add")
	if r.body["changed"] != float64(2) {
		t.Fatalf("changed %v", r.body)
	}
	if got := list(t, c, dash, url.Values{"group": {g}}); !got[a] || !got[b] || len(got) != 2 {
		t.Fatalf("group %v", got)
	}
	if rev := revOf(record(c.do("GET", "/api/records/"+a, nil)), "groups"); rev != 1 {
		t.Fatalf("rev groups %v", rev)
	}
	p := auditPageOf(t, c, dash, url.Values{})
	labels := 0
	for _, e := range p.Operations[0].Entries {
		if e.Object == "entity_group" && e.Op == "create" {
			labels++
		}
	}
	if labels != 2 {
		t.Fatalf("audit: %d labels in the last operation", labels)
	}

	// уже стоящая метка не считается; снятие — одной операцией
	r = c.do("POST", path, map[string]any{"add": []string{a}, "remove": []string{b}})
	expect(t, r, http.StatusOK, "add+remove")
	if r.body["changed"] != float64(1) {
		t.Fatalf("changed %v", r.body)
	}
	if got := list(t, c, dash, url.Values{"group": {g}}); !got[a] || got[b] {
		t.Fatalf("group %v", got)
	}

	expect(t, c.do("POST", path, map[string]any{"add": []string{a}, "remove": []string{a}}), http.StatusBadRequest, "both")
	expect(t, c.do("POST", path, map[string]any{}), http.StatusBadRequest, "empty")
	expect(t, c.do("POST", path, map[string]any{"add": []string{uuid.NewString()}}), http.StatusNotFound, "unknown record")
	expect(t, c.do("POST", "/api/dashboards/"+dash+"/groups/"+uuid.NewString()+"/records", map[string]any{"add": []string{a}}), http.StatusNotFound, "unknown group")
	expect(t, c.do("DELETE", "/api/records/"+b, nil), http.StatusOK, "trash")
	expect(t, c.do("POST", path, map[string]any{"add": []string{b}}), http.StatusNotFound, "trashed record")
}

// F-133: список сортируется по sort_by шаблона (текущие значения, без регистра,
// пустое — в конец); у шаблона без sort_by — по полям подписи; страницы стабильны.
func TestListSortedByPack(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	txt := func(v string) []any { return []any{map[string]any{"value": v}} }
	p := func(ref, surname, name string) map[string]any {
		f := map[string]any{"name": txt(name)}
		if surname != "" {
			f["surname"] = txt(surname)
		}
		return map[string]any{"ref": ref, "entity": "kin.core.person", "fields": f}
	}
	r := batch(t, c, dash, map[string]any{"records": []any{
		p("c", "Яковлев", "Антон"), p("a2", "абрамов", "Борис"), p("a1", "Абрамов", "Аркадий"), p("none", "", "Яна"),
	}}, http.StatusCreated)
	order := func(q string) []string {
		resp := c.do("GET", "/api/dashboards/"+dash+"/records?"+q, nil)
		expect(t, resp, http.StatusOK, "list")
		var ids []string
		for _, it := range resp.body["items"].([]any) {
			ids = append(ids, it.(map[string]any)["id"].(string))
		}
		return ids
	}
	want := []string{created(t, r, "a1"), created(t, r, "a2"), created(t, r, "c"), created(t, r, "none")}
	got := order("entity=kin.core.person")
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order %v, want %v", got, want)
		}
	}
	if page := order("entity=kin.core.person&limit=2&offset=1"); len(page) != 2 || page[0] != want[1] || page[1] != want[2] {
		t.Fatalf("page %v, want %v", page, want[1:3])
	}
}

// F-122: автодобавление в группу по фамилии — при включении (существующие),
// при создании записи и при правке фамилии; сравнение по всей истории, без
// регистра; автоматически не убирается.
func TestGroupAutoSurname(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	txt := func(vs ...string) []any {
		out := []any{}
		for _, v := range vs {
			out = append(out, map[string]any{"value": v})
		}
		return out
	}
	mk := func(ref string, surnames ...string) map[string]any {
		return map[string]any{"ref": ref, "entity": "kin.core.person", "fields": map[string]any{"name": txt(ref), "surname": txt(surnames...)}}
	}
	r := batch(t, c, dash, map[string]any{"records": []any{mk("old", "Иванова", "ПЕТРОВА"), mk("other", "Сидоров"), mk("nosurname")}}, http.StatusCreated)
	old, other := created(t, r, "old"), created(t, r, "other")

	g := group(t, c, dash, "Петровы")
	in := func(rec string) bool { return list(t, c, dash, url.Values{"group": {g}})[rec] }
	if in(old) {
		t.Fatal("group must start empty")
	}
	pr := c.do("PATCH", "/api/dashboards/"+dash+"/groups/"+g, map[string]any{"auto_surname": true, "surnames": []any{" петрова ", "Петрова", "Кузнецов"}})
	expect(t, pr, http.StatusOK, "enable auto")
	if sn, _ := pr.body["surnames"].([]any); len(sn) != 2 || pr.body["auto_surname"] != true {
		t.Fatalf("group %v", pr.body)
	}
	if !in(old) || in(other) {
		t.Fatal("existing records: old (history surname) must be added, other not")
	}

	// новая запись и правка фамилии
	nr := batch(t, c, dash, map[string]any{"records": []any{mk("new", "кузнецов")}}, http.StatusCreated)
	if !in(created(t, nr, "new")) {
		t.Fatal("new record must be added at creation")
	}
	expect(t, c.do("POST", "/api/records/"+other+"/fields/surname/values", map[string]any{"if_rev": 0, "value": "Кузнецов"}), http.StatusOK, "add surname")
	if !in(other) {
		t.Fatal("record with a new surname must be added on save")
	}
	// из группы автоматически не убирается: поле «Название» ни при чём, фамилии сменили
	if !in(old) {
		t.Fatal("old record must stay")
	}
	// убрали вручную, переименовали группу — обратно не возвращается
	expect(t, c.do("POST", "/api/dashboards/"+dash+"/groups/"+g+"/records", map[string]any{"remove": []any{old}}), http.StatusOK, "remove")
	expect(t, c.do("PATCH", "/api/dashboards/"+dash+"/groups/"+g, map[string]any{"name": "Петровы и Ко"}), http.StatusOK, "rename")
	if in(old) {
		t.Fatal("rename must not re-add a manually removed record")
	}
	// выключенная галочка не добавляет
	expect(t, c.do("PATCH", "/api/dashboards/"+dash+"/groups/"+g, map[string]any{"auto_surname": false}), http.StatusOK, "disable")
	nr2 := batch(t, c, dash, map[string]any{"records": []any{mk("late", "Петрова")}}, http.StatusCreated)
	if in(created(t, nr2, "late")) {
		t.Fatal("auto off: must not add")
	}
}

// F-124, F-132: набор полей быстрой формы шаблона — переопределение в дашборде;
// пусто — вернуть умолчание пака; версия шаблона сверяется.
func TestQuickFieldsOverride(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	id := person(t, c, dash, nil)
	tmpl, _ := record(c.do("GET", "/api/records/"+id, nil))["template_id"].(string)
	url := "/api/dashboards/" + dash + "/templates/" + tmpl + "/quick_fields"
	def := func(r resp) []any {
		d, _ := r.body["definition"].(map[string]any)
		q, _ := d["quick_fields"].([]any)
		return q
	}
	r := c.do("PUT", url, map[string]any{"if_rev": 0, "quick_fields": []any{"name", "patronymic", "surname"}})
	expect(t, r, http.StatusOK, "override")
	if q := def(r); len(q) != 3 || q[1] != "patronymic" || r.body["rev"] != float64(1) {
		t.Fatalf("override: %v rev %v", q, r.body["rev"])
	}
	expect(t, c.do("PUT", url, map[string]any{"if_rev": 0, "quick_fields": []any{"name"}}), http.StatusConflict, "old rev")
	expect(t, c.do("PUT", url, map[string]any{"if_rev": 1, "quick_fields": []any{"name", "name"}}), http.StatusBadRequest, "duplicate")
	expect(t, c.do("PUT", url, map[string]any{"if_rev": 1, "quick_fields": []any{"nope"}}), http.StatusBadRequest, "unknown field")
	r = c.do("PUT", url, map[string]any{"if_rev": 1, "quick_fields": []any{}})
	expect(t, r, http.StatusOK, "reset")
	if q := def(r); len(q) != 5 { // умолчание пака
		t.Fatalf("reset: %v", q)
	}
}
