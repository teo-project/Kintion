package api_test

import (
	"net/http"
	"slices"
	"testing"
)

// treeIDs — id отношений и записей ответа /tree.
func treeIDs(t *testing.T, c *client, path string) (rels, recs []string) {
	t.Helper()
	r := c.do("GET", path, nil)
	expect(t, r, http.StatusOK, "tree")
	for key, out := range map[string]*[]string{"relations": &rels, "records": &recs} {
		list, _ := r.body[key].([]any)
		for _, it := range list {
			*out = append(*out, it.(map[string]any)["id"].(string))
		}
		slices.Sort(*out)
	}
	return rels, recs
}

func sorted(ids ...string) []string {
	slices.Sort(ids)
	return ids
}

// F-110, F-111: дерево — отношения с tree_link и их видимые участники; фильтр
// по группе — только участники группы; участник в корзине не виден (F-07).
func TestTree(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	r := batch(t, c, dash, map[string]any{
		"records": []any{
			map[string]any{"ref": "a", "entity": "kin.core.person"},
			map[string]any{"ref": "b", "entity": "kin.core.person"},
			map[string]any{"ref": "k", "entity": "kin.core.person"},
			map[string]any{"ref": "pair", "template": "kin.core.relationship"},
			map[string]any{"ref": "pc", "template": "kin.core.parent_child"},
			map[string]any{"ref": "fr", "template": "kin.core.friend"},
		},
		"participants": []any{
			map[string]any{"container": "pair", "entity": "a", "role": "partner"},
			map[string]any{"container": "pair", "entity": "b", "role": "partner"},
			map[string]any{"container": "pc", "entity": "a", "role": "parent"},
			map[string]any{"container": "pc", "entity": "k", "role": "child"},
			map[string]any{"container": "fr", "entity": "b", "role": "friend"},
			map[string]any{"container": "fr", "entity": "k", "role": "friend"},
		},
	}, http.StatusCreated)
	id := func(ref string) string { return created(t, r, ref) }
	base := "/api/dashboards/" + dash + "/tree"

	rels, recs := treeIDs(t, c, base)
	if !slices.Equal(rels, sorted(id("pair"), id("pc"))) || !slices.Equal(recs, sorted(id("a"), id("b"), id("k"))) {
		t.Fatalf("tree: rels %v recs %v", rels, recs)
	}

	g := group(t, c, dash, "Пара")
	for _, ref := range []string{"a", "b"} {
		expect(t, c.do("PUT", "/api/records/"+id(ref)+"/groups", map[string]any{"if_rev": 0, "groups": []string{g}}), http.StatusOK, "label")
	}
	rels, recs = treeIDs(t, c, base+"?group="+g)
	if !slices.Equal(rels, []string{id("pair")}) || !slices.Equal(recs, sorted(id("a"), id("b"))) {
		t.Fatalf("group: rels %v recs %v", rels, recs)
	}

	// третий участник пары вне группы — в ответе его нет и в конверте отношения
	expect(t, c.do("POST", "/api/records/"+id("pair")+"/participants", map[string]any{"if_rev": 0, "entity": id("k"), "role": "partner"}),
		http.StatusOK, "third partner")
	resp := c.do("GET", base+"?group="+g, nil)
	expect(t, resp, http.StatusOK, "group tree")
	pair := resp.body["relations"].([]any)[0].(map[string]any)
	if n := len(pair["participants"].([]any)); n != 2 {
		t.Fatalf("participants outside the group: %v", pair["participants"])
	}

	expect(t, c.do("DELETE", "/api/records/"+id("k"), nil), http.StatusOK, "delete")
	rels, recs = treeIDs(t, c, base)
	if !slices.Equal(rels, []string{id("pair")}) || !slices.Equal(recs, sorted(id("a"), id("b"))) {
		t.Fatalf("hidden participant leaked: rels %v recs %v", rels, recs)
	}

	expect(t, c.do("GET", base+"?group=nope", nil), http.StatusBadRequest, "bad group")
	expect(t, c.do("GET", base+"?group=00000000-0000-0000-0000-000000000000", nil), http.StatusBadRequest, "unknown group")
}

// F-119: настройки ракурса «Дерево» — у аккаунта; лента по умолчанию включена.
func TestPrefsTree(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	var p map[string]any
	c.getJSON("/api/me/prefs", &p)
	if p["tree_ribbon"] != true || p["tree_hide_dead"] != false {
		t.Fatalf("default %v", p)
	}
	expect(t, c.do("PUT", "/api/me/prefs", map[string]any{"tree_ribbon": false, "tree_hide_dead": true}), http.StatusOK, "set")
	c.getJSON("/api/me/prefs", &p)
	if p["tree_ribbon"] != false || p["tree_hide_dead"] != true || p["card_mode"] != "read" {
		t.Fatalf("stored %v", p)
	}
}
