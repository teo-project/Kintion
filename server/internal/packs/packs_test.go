package packs_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"kintion/server/internal/db"
	"kintion/server/internal/db/dbtest"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/packs"
)

func ruFS(t *testing.T) fs.FS {
	t.Helper()
	fsys, err := packs.Source("", "ru")
	if err != nil {
		t.Fatal(err)
	}
	return fsys
}

func noErrors(t *testing.T, ds []packs.Diagnostic) {
	t.Helper()
	for _, d := range ds {
		if d.Level == packs.LevelError {
			t.Errorf("unexpected: %s", d)
		}
	}
}

func TestBuiltinPacks(t *testing.T) {
	reg, ds := packs.Load(ruFS(t), "ru", []string{"kin.core", "kin.christian.orthodox"})
	noErrors(t, ds)
	if len(reg.Packs) != 2 || len(reg.Entities()) != 4 || len(reg.Templates()) != 20 {
		t.Fatalf("packs %d, entities %d, templates %d", len(reg.Packs), len(reg.Entities()), len(reg.Templates()))
	}
	if d, ok := reg.DefaultTemplate("kin.core.person"); !ok || d.ID != "kin.core.person" {
		t.Error("no default template for person")
	}
	if _, ok := reg.DefaultTemplate("kin.core.event"); ok {
		t.Error("event has no default template in kin.core")
	}
	if w := messages(ds, packs.LevelWarn); strings.Contains(w, "quick_add") {
		t.Errorf("quick_add warnings in builtin packs:\n%s", w)
	}
	tm, ok := reg.Template("kin.christian.orthodox.church_marriage")
	if !ok || tm.Pack != "kin.christian.orthodox" || tm.Entity != "kin.core.relationship" || len(tm.Raw) == 0 {
		t.Fatalf("church marriage: %+v", tm)
	}
}

func TestRequiresAddsDependencies(t *testing.T) {
	reg, ds := packs.Load(ruFS(t), "ru", []string{"kin.christian.orthodox"})
	noErrors(t, ds)
	var ids []string
	for _, m := range reg.Packs {
		ids = append(ids, m.ID)
	}
	if !slices.Equal(ids, []string{"kin.core", "kin.christian.orthodox"}) {
		t.Fatalf("packs %v", ids)
	}
	// по умолчанию православный пак не включается (Д-31)
	reg, _ = packs.Load(ruFS(t), "ru", []string{"kin.core"})
	if reg.HasPack("kin.christian.orthodox") {
		t.Fatal("orthodox pack loaded without TEO_KIN_PACKS")
	}
}

func TestSyntheticPack(t *testing.T) {
	reg, ds := packs.Load(os.DirFS("../../../testdata/packs/xx"), "xx", []string{"test.alt"})
	noErrors(t, ds)
	if len(reg.Entities()) != 3 || len(reg.Templates()) != 5 {
		t.Fatalf("entities %d, templates %d", len(reg.Entities()), len(reg.Templates()))
	}
	if _, ok := reg.DefaultTemplate("test.alt.bond"); !ok {
		t.Fatal("no default template for test.alt.bond")
	}
}

const (
	manifest = `{"format":"kintion-pack/0","id":"p","version":"1","language":"xx","name":"P","requires":[],"files":["entities/p.e.json","templates/p.e/p.e.json"]}`
	entity   = `{"format":"kintion-pack/0","source":"p","id":"p.e","participant":true}`
	tmpl     = `{"format":"kintion-pack/0","source":"p","id":"p.e","entity":"p.e","name":"E","fields":[{"key":"a","type":"text"}]}`
)

func base() fstest.MapFS {
	return fstest.MapFS{
		"p/p.pack.json":              {Data: []byte(manifest)},
		"p/entities/p.e.json":        {Data: []byte(entity)},
		"p/templates/p.e/p.e.json":   {Data: []byte(tmpl)},
		"q/q.pack.json":              {Data: []byte(`{"format":"kintion-pack/0","id":"q","version":"1","language":"xx","requires":["p"],"files":["templates/p.e/q.t.json"]}`)},
		"q/templates/p.e/q.t.json":   {Data: []byte(`{"format":"kintion-pack/0","source":"q","id":"q.t","entity":"p.e","name":"T"}`)},
		"z/z.pack.json":              {Data: []byte(`{"format":"kintion-pack/0","id":"z","version":"1","language":"xx","requires":[],"files":[]}`)},
		"z/templates/p.e/stray.json": {Data: []byte(`{}`)},
	}
}

func messages(ds []packs.Diagnostic, level packs.Level) string {
	var b strings.Builder
	for _, d := range ds {
		if d.Level == level {
			b.WriteString(d.String() + "\n")
		}
	}
	return b.String()
}

func TestBrokenPacks(t *testing.T) {
	if _, ds := packs.Load(base(), "xx", []string{"q"}); packs.HasErrors(ds) {
		t.Fatalf("base must load: %v", ds)
	}
	cases := []struct {
		name    string
		mutate  func(fstest.MapFS)
		enabled []string
		want    string
	}{
		{"missing listed file", func(m fstest.MapFS) { delete(m, "p/entities/p.e.json") }, []string{"p"}, "отсутствует"},
		{"foreign source", func(m fstest.MapFS) {
			m["q/templates/p.e/q.t.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"p","id":"q.t","entity":"p.e"}`)}
		}, []string{"q"}, `source = "p"`},
		{"missing requires", func(m fstest.MapFS) { delete(m, "p/p.pack.json") }, []string{"q"}, "требуется пак p"},
		{"template without entity", func(m fstest.MapFS) {
			m["q/templates/p.e/q.t.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"q","id":"q.t","entity":"p.none"}`)}
		}, []string{"q"}, "сущность p.none не найдена"},
		{"duplicate template id", func(m fstest.MapFS) {
			m["q/templates/p.e/q.t.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"q","id":"p.e","entity":"p.e"}`)}
		}, []string{"q"}, "шаблон p.e уже объявлен"},
		{"unknown enabled pack", func(fstest.MapFS) {}, []string{"nope"}, "включён пак nope"},
		{"bad json", func(m fstest.MapFS) { m["p/entities/p.e.json"] = &fstest.MapFile{Data: []byte(`{`)} }, []string{"p"}, "неверный JSON"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fsys := base()
			c.mutate(fsys)
			_, ds := packs.Load(fsys, "xx", c.enabled)
			if errs := messages(ds, packs.LevelError); !strings.Contains(errs, c.want) {
				t.Fatalf("want error %q, got:\n%s", c.want, errs)
			}
		})
	}
}

func TestWarnings(t *testing.T) {
	fsys := base()
	// Д-7: пересечение ключей — warn, не error
	fsys["p/templates/p.e/p.e.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"p","id":"p.e","entity":"p.e",
		"fields":[{"key":"a"},{"key":"note"},{"key":"custom.x"}],"roles":[{"key":"a","forms":{"default":"A"}}]}`)}
	_, ds := packs.Load(fsys, "xx", []string{"p", "z"})
	if packs.HasErrors(ds) {
		t.Fatalf("errors: %v", ds)
	}
	warns := messages(ds, packs.LevelWarn)
	for _, w := range []string{`ключ "a" — и поле, и роль`, `"note" совпадает с системным`, `"custom.x" в пространстве`, "stray.json: файл не перечислен"} {
		if !strings.Contains(warns, w) {
			t.Errorf("no warning %q in:\n%s", w, warns)
		}
	}
}

// F-102: пункты quick_add проверяются при загрузке — роли, этап, ключи
// relation; ошибки — warn, пак загружается. Объект-константа (частичная
// дата) — не ошибка.
func TestQuickAddWarnings(t *testing.T) {
	fsys := base()
	fsys["q/templates/p.e/q.t.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"q","id":"q.t","entity":"p.e","name":"T",
		"period":{"end":null},"fields":[{"key":"f"}],"roles":[{"key":"a","forms":{"default":"A"}},{"key":"b","forms":{"default":"B"}}],
		"ui":{"quick_add":[
			{"name":"ok","role":"a","new_role":"b","new":{"g":"x","s":{"field":"s[-1]"},"d":{"y":2000}},"relation":{"start":{"y":2000,"m":5},"f":"c",".note":{"string":"n"}}},
			{"name":"bad","role":"a","new_role":"x","stage":"st","relation":{"end":{"field":"start"},"zz":1}},
			{"name":"broken","relation":[]}
		]}}`)}
	_, ds := packs.Load(fsys, "xx", []string{"q"})
	if packs.HasErrors(ds) {
		t.Fatalf("errors: %v", ds)
	}
	warns := messages(ds, packs.LevelWarn)
	if strings.Contains(warns, `"ok"`) {
		t.Errorf("warnings for a valid item:\n%s", warns)
	}
	for _, w := range []string{`"bad": нет роли "x"`, `"bad": нет этапа "st"`, `relation.end — у отношения нет этой даты`,
		`relation.zz — нет такого поля`, `ui.quick_add[2] не разобран`} {
		if !strings.Contains(warns, w) {
			t.Errorf("no warning %q in:\n%s", w, warns)
		}
	}
}

// F-103: links_rank — число у шаблона контейнера; иначе warn, пак загружается.
// F-111, F-118: tree_link — true или объект; as, line_by, stages, tree_line
// и формы вариантов проверяются при загрузке (warn).
func TestTreeLink(t *testing.T) {
	fsys := base()
	fsys["p/entities/p.e.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"p","id":"p.e","participant":true,"container":true}`)}
	fsys["q/q.pack.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","id":"q","version":"1","language":"xx","requires":["p"],
		"files":["templates/p.e/q.ok.json","templates/p.e/q.yes.json","templates/p.e/q.no.json","templates/p.e/q.bad.json","templates/p.e/q.as.json"]}`)}
	delete(fsys, "q/templates/p.e/q.t.json")
	tpl := func(id, body string) {
		fsys["q/templates/p.e/"+id+".json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"q","id":"` + id + `","entity":"p.e","name":"T",` + body + `}`)}
	}
	tpl("q.ok", `"fields":[{"key":"k","type":"choice","options":[{"key":"a","name":"A","tree_line":"dashed","forms":{"p":{"default":"P"}}}]}],
		"roles":[{"key":"p","forms":{"default":"P"}},{"key":"c","forms":{"default":"C"}}],"tree_link":{"line_by":"k"}`)
	tpl("q.yes", `"roles":[{"key":"p","forms":{"default":"P"}}],"tree_link":true`)
	tpl("q.no", `"roles":[{"key":"p","forms":{"default":"P"}}],"tree_link":false`)
	tpl("q.bad", `"fields":[{"key":"k","type":"text"},{"key":"m","type":"choice","options":[{"key":"a","name":"A","tree_line":"dotted","forms":{"zz":{"default":"Z"}}}]}],
		"roles":[{"key":"p","forms":{"default":"P"}}],"tree_link":{"line_by":"k","as":"descent","stages":["st"]}`)
	tpl("q.as", `"roles":[{"key":"p","forms":{"default":"P"}}],"tree_link":{"as":"web"}`)
	reg, ds := packs.Load(fsys, "xx", []string{"q"})
	if packs.HasErrors(ds) {
		t.Fatalf("errors: %v", ds)
	}
	for id, want := range map[string]bool{"q.ok": true, "q.yes": true, "q.no": false, "q.bad": true} {
		if tm, _ := reg.Template(id); tm == nil || tm.InTree() != want {
			t.Errorf("%s: InTree != %v", id, want)
		}
	}
	if tm, _ := reg.Template("q.ok"); tm.TreeLink.LineBy != "k" {
		t.Errorf("line_by: %+v", tm.TreeLink)
	}
	warns := messages(ds, packs.LevelWarn)
	for _, id := range []string{"q.ok", "q.yes", "q.no"} {
		if strings.Contains(warns, "шаблон "+id+":") {
			t.Errorf("warnings for %s:\n%s", id, warns)
		}
	}
	for _, w := range []string{`tree_line = "dotted"`, `формы для неизвестной роли "zz"`, `as = descent — нужны две роли`,
		`line_by = "k" — нет такого поля-выбора`, `нет этапа "st"`, `as = "web"`} {
		if !strings.Contains(warns, w) {
			t.Errorf("no warning %q in:\n%s", w, warns)
		}
	}
}

func TestLinksRank(t *testing.T) {
	fsys := base()
	fsys["q/templates/p.e/q.t.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"q","id":"q.t","entity":"p.e","name":"T","links_rank":"high"}`)}
	fsys["p/templates/p.e/p.e.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"p","id":"p.e","entity":"p.e","name":"E","links_rank":null}`)}
	_, ds := packs.Load(fsys, "xx", []string{"q"})
	if packs.HasErrors(ds) {
		t.Fatalf("errors: %v", ds)
	}
	warns := messages(ds, packs.LevelWarn)
	for _, w := range []string{`links_rank = "high" — не число`, `p.e: links_rank = null — не число`, "links_rank у шаблона сущности p.e, которая не контейнер"} {
		if !strings.Contains(warns, w) {
			t.Errorf("no warning %q in:\n%s", w, warns)
		}
	}
	reg, ds := packs.Load(ruFS(t), "ru", []string{"kin.core"})
	if w := messages(ds, packs.LevelWarn); strings.Contains(w, "links_rank") {
		t.Errorf("links_rank warnings in kin.core:\n%s", w)
	}
	if tm, _ := reg.Template("kin.core.parent_child"); string(tm.LinksRank) == "" {
		t.Error("kin.core.parent_child: no links_rank")
	}
}

// F-93: links — данные пака; шаблон переопределяет сущность, по умолчанию — да.
func TestShowsLinks(t *testing.T) {
	reg, ds := packs.Load(ruFS(t), "ru", []string{"kin.core", "kin.christian.orthodox"})
	noErrors(t, ds)
	for _, tm := range reg.Templates() {
		e, _ := reg.Entity(tm.Entity)
		if !e.Container {
			continue
		}
		want := !e.Anchor // события (anchor) — без людей
		if got := packs.ShowsLinks(e, tm); got != want {
			t.Errorf("%s: links = %v, want %v", tm.ID, got, want)
		}
	}

	yes, no := true, false
	e := &packs.Entity{Container: true}
	if !packs.ShowsLinks(e, &packs.Template{}) || !packs.ShowsLinks(nil, nil) {
		t.Error("default must be true")
	}
	e.Links = &no
	if packs.ShowsLinks(e, &packs.Template{}) || !packs.ShowsLinks(e, &packs.Template{Links: &yes}) {
		t.Error("template must override entity")
	}
	if packs.ShowsLinks(&packs.Entity{}, &packs.Template{Links: &no}) {
		t.Error("template links=false")
	}

	// links вне контейнера — предупреждение
	fsys := base()
	fsys["p/entities/p.e.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"p","id":"p.e","participant":true,"links":false}`)}
	fsys["q/templates/p.e/q.t.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"q","id":"q.t","entity":"p.e","name":"T","links":true}`)}
	_, ds = packs.Load(fsys, "xx", []string{"q"})
	warns := messages(ds, packs.LevelWarn)
	for _, w := range []string{"сущность p.e: links", "шаблон q.t: links"} {
		if !strings.Contains(warns, w) {
			t.Errorf("no warning %q in:\n%s", w, warns)
		}
	}
}

// F-99: начало / конец period — строка-подпись или объект с show; null — даты нет.
func TestPeriodBound(t *testing.T) {
	fsys := base()
	fsys["p/entities/p.e.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"p","id":"p.e","participant":true,
		"period":{"start":"Начало","end":{"name":"Конец","show":"filled"}}}`)}
	fsys["q/templates/p.e/q.t.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"q","id":"q.t","entity":"p.e","name":"T",
		"period":{"end":null,"start":{"name":"С","show":"sometimes"},"middle":"M"}}`)}
	reg, ds := packs.Load(fsys, "xx", []string{"q"})
	if packs.HasErrors(ds) {
		t.Fatalf("errors: %v", ds)
	}
	e, _ := reg.Entity("p.e")
	if b := e.Period["end"]; b == nil || b.Name != "Конец" || b.Show != "filled" {
		t.Errorf("end = %+v", b)
	}
	if b := e.Period["start"]; b == nil || b.Name != "Начало" || b.Show != "" {
		t.Errorf("start = %+v", b)
	}
	if l, ok := packs.PeriodLabel(e, nil, "end"); !ok || l != "Конец" {
		t.Errorf("label = %q, %v", l, ok)
	}
	tm, _ := reg.Template("q.t")
	if _, ok := packs.PeriodLabel(e, tm, "end"); ok {
		t.Error("template end: null must remove the date")
	}
	if l, _ := packs.PeriodLabel(e, tm, "start"); l != "С" {
		t.Errorf("template start = %q", l)
	}
	warns := messages(ds, packs.LevelWarn)
	for _, w := range []string{`period.start: show = "sometimes"`, `period: неизвестный ключ "middle"`} {
		if !strings.Contains(warns, w) {
			t.Errorf("no warning %q in:\n%s", w, warns)
		}
	}
}

// Проверка «пакам — данные» (план §3 п.7): в рабочем коде server/ нет строк
// с id и ключами паков — ни kin.*, ни ключей полей, ролей, этапов и шаблонов.
// Разрешены только системные ключи.
func TestNoPackKeysInServerCode(t *testing.T) {
	reg, ds := packs.Load(ruFS(t), "ru", []string{"kin.core", "kin.christian.orthodox"})
	noErrors(t, ds)
	allowed := []string{"start", "end", "stage", "title", "note", "id", "default", "photo"}
	forbidden := map[string]bool{}
	for _, k := range reg.Keys() {
		if !slices.Contains(allowed, k) {
			forbidden[k] = true
		}
	}
	if !forbidden["surname"] || !forbidden["partner"] || !forbidden["gender"] {
		t.Fatal("pack keys not collected")
	}

	root := "../.."
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "testdata" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		tags := map[*ast.BasicLit]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			if f, ok := n.(*ast.Field); ok && f.Tag != nil {
				tags[f.Tag] = true
			}
			return true
		})
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if strings.Contains(s, "kin.") {
				t.Errorf("%s: pack id in code: %s", p, lit.Value)
			}
			// типы полей формата (place, phone, email) — словарь формата, не ключи паков
			if !tags[lit] && forbidden[s] && !slices.Contains(packs.FieldTypes, s) {
				t.Errorf("%s: pack key in code: %s", p, lit.Value)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSync(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if err := db.NewMigrator(pool, "test", slog.New(slog.NewTextHandler(io.Discard, nil))).Up(ctx); err != nil {
		t.Fatal(err)
	}
	both, _ := packs.Load(ruFS(t), "ru", []string{"kin.christian.orthodox"})
	if err := packs.Sync(ctx, pool, both); err != nil {
		t.Fatal(err)
	}
	core, _ := packs.Load(ruFS(t), "ru", []string{"kin.core"})
	if err := packs.Sync(ctx, pool, core); err != nil {
		t.Fatal(err)
	}
	list, err := sqlc.New(pool).ListPacks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, p := range list {
		got[p.ID] = p.Loaded
	}
	if len(got) != 2 || !got["kin.core"] || got["kin.christian.orthodox"] {
		t.Fatalf("pack table: %v", got)
	}
}

// F-126: end_flag — ключ поля bool; иначе warn, пак загружается.
func TestEndFlag(t *testing.T) {
	reg, ds := packs.Load(ruFS(t), "ru", []string{"kin.core"})
	noErrors(t, ds)
	if tm, _ := reg.Template("kin.core.person"); tm == nil || tm.EndFlag != "deceased" {
		t.Fatalf("person end_flag: %+v", tm)
	} else if f, ok := tm.Field("deceased"); !ok || f.Type != packs.TypeBool || f.Show != "filled" || f.Section != "main" {
		t.Errorf("deceased field: %+v", f)
	}
	if w := messages(ds, packs.LevelWarn); strings.Contains(w, "end_flag") {
		t.Errorf("end_flag warnings in builtin packs:\n%s", w)
	}

	fsys := base()
	fsys["q/templates/p.e/q.t.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"q","id":"q.t","entity":"p.e","name":"T",
		"end_flag":"dead","fields":[{"key":"dead","type":"text"}]}`)}
	_, ds = packs.Load(fsys, "xx", []string{"q"})
	if packs.HasErrors(ds) {
		t.Fatalf("errors: %v", ds)
	}
	if w := messages(ds, packs.LevelWarn); !strings.Contains(w, `end_flag = "dead"`) {
		t.Errorf("no end_flag warning:\n%s", w)
	}
}

// F-133: sort_by — ключи полей шаблона; неизвестный ключ — warn.
func TestSortBy(t *testing.T) {
	reg, ds := packs.Load(ruFS(t), "ru", []string{"kin.core"})
	noErrors(t, ds)
	if w := messages(ds, packs.LevelWarn); strings.Contains(w, "sort_by") {
		t.Errorf("sort_by warnings in builtin packs:\n%s", w)
	}
	if tm, _ := reg.Template("kin.core.person"); tm == nil || !slices.Equal(tm.SortBy, []string{"surname", "name", "patronymic"}) {
		t.Errorf("person sort_by: %+v", tm)
	}
	fsys := base()
	fsys["q/templates/p.e/q.t.json"] = &fstest.MapFile{Data: []byte(`{"format":"kintion-pack/0","source":"q","id":"q.t","entity":"p.e","name":"T",
		"sort_by":["zz"],"fields":[{"key":"a","type":"text"}]}`)}
	_, ds = packs.Load(fsys, "xx", []string{"q"})
	if w := messages(ds, packs.LevelWarn); !strings.Contains(w, `sort_by: нет поля "zz"`) {
		t.Errorf("no sort_by warning:\n%s", w)
	}
}
