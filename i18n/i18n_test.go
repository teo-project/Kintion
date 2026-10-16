package i18n

import (
	"encoding/json"
	"slices"
	"testing"
)

func keys(prefix string, v any, out *[]string) {
	m, ok := v.(map[string]any)
	if !ok {
		*out = append(*out, prefix)
		return
	}
	for k, sub := range m {
		keys(prefix+"."+k, sub, out)
	}
}

func load(t *testing.T, lang string) []string {
	t.Helper()
	data, err := File(lang)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("%s.json: %v", lang, err)
	}
	var out []string
	keys("", v, &out)
	slices.Sort(out)
	return out
}

func TestLanguages(t *testing.T) {
	for _, l := range []string{"ru", "en"} {
		if !Has(l) {
			t.Fatalf("no %s.json", l)
		}
	}
	if Has("xx") {
		t.Fatal("unexpected language")
	}
}

func TestSameKeys(t *testing.T) {
	ref := load(t, "ru")
	for _, l := range Languages() {
		if got := load(t, l); !slices.Equal(got, ref) {
			t.Errorf("%s.json keys differ from ru.json:\n%v\n%v", l, got, ref)
		}
	}
}

func TestLookup(t *testing.T) {
	if Lookup("ru", "dashboard.defaultName") == "" || Lookup("ru", "dashboard.nope") != "" || Lookup("xx", "appName") != "" {
		t.Fatal("lookup")
	}
}
