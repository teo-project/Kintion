package api_test

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

// Демо-дерево deploy/scripts/seed-demo-tree.json и Алиевы
// (seed-demo-aliev.json) принимаются API: род Лебедевых —
// одной операцией, затем пара из четырёх участников — другой.
func TestSeedDemoTree(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	raw, err := os.ReadFile("../../../deploy/scripts/seed-demo-tree.json")
	if err != nil {
		t.Fatal(err)
	}
	g := group(t, c, dash, "Лебедевы")
	var body map[string]any
	if err := json.Unmarshal([]byte(strings.ReplaceAll(string(raw), "$G_LEBEDEV", g)), &body); err != nil {
		t.Fatal(err)
	}
	tree := batch(t, c, dash, body, http.StatusCreated)
	if errs, _ := tree.body["errors"].([]any); len(errs) != 0 {
		t.Fatalf("tree errors %v", errs)
	}
	raw, err = os.ReadFile("../../../deploy/scripts/seed-demo-aliev.json")
	if err != nil {
		t.Fatal(err)
	}
	body = nil
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	r := batch(t, c, dash, body, http.StatusCreated)
	if errs, _ := r.body["errors"].([]any); len(errs) != 0 {
		t.Fatalf("errors %v", errs)
	}
	pair := record(c.do("GET", "/api/records/"+created(t, r, "u-murat"), nil))
	if ps, _ := pair["participants"].([]any); len(ps) != 4 {
		t.Fatalf("pair of four: %v", pair["participants"])
	}
	// дети от разных жён: у каждого — два родителя (Мурат и своя мать)
	for kid, mother := range map[string]string{"ruslan": "fatima", "amina": "zarina", "timur": "leila"} {
		parents := map[string]bool{}
		in, _ := record(c.do("GET", "/api/records/"+created(t, r, kid), nil))["participates_in"].([]any)
		for _, x := range in {
			p := x.(map[string]any)
			if p["role"] != "child" {
				continue
			}
			rel := record(c.do("GET", "/api/records/"+p["container_id"].(string), nil))
			for _, q := range rel["participants"].([]any) {
				if q := q.(map[string]any); q["role"] == "parent" {
					parents[q["entity_id"].(string)] = true
				}
			}
		}
		if len(parents) != 2 || !parents[created(t, r, "murat")] || !parents[created(t, r, mother)] {
			t.Fatalf("%s parents %v", kid, parents)
		}
	}
}
