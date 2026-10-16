// Package packs — загрузчик и реестр паков формата v0 (docs/pack-format.md),
// порт loadPacks из прототипа. Код знает формат, но не содержимое паков:
// ключи полей, ролей и шаблонов берутся только из данных (F-52, AGENTS.md).
package packs

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"sort"
	"strings"
)

// Format — метка версии формата в файлах паков.
const Format = "kintion-pack/0"

// Reserved — системные ключи записи; ключи полей и ролей с ними не пересекаются
// (FINDINGS №2, №3). Префикс CustomPrefix — свободные поля (F-75).
var Reserved = []string{"start", "end", "stage", "title", "note", "id"}

// CustomPrefix — пространство имён свободных полей записи (F-75).
const CustomPrefix = "custom."

// SharedPrefix — пространство имён общих своих полей шаблона в дашборде
// («добавить всем записям этого шаблона», F-96).
const SharedPrefix = "shared."

// Manifest — <пак>.pack.json.
type Manifest struct {
	Format      string   `json:"format"`
	ID          string   `json:"id"`
	Version     string   `json:"version"`
	Language    string   `json:"language"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Requires    []string `json:"requires"`
	Files       []string `json:"files"`
}

// Entity — файл сущности entities/<сущность>.json. Raw — файл как есть
// (уходит во фронтенд).
type Entity struct {
	Format      string `json:"format"`
	Source      string `json:"source"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Participant bool   `json:"participant"`
	Container   bool   `json:"container"`
	// Anchor — к записям сущности привязываются start / end и значения (F-83).
	Anchor bool `json:"anchor"`
	// Links — раздел «Связи» показывает других участников записей-контейнеров
	// сущности (F-93); nil — да. Шаблон может переопределить.
	Links  *bool                      `json:"links,omitempty"`
	Period map[string]*PeriodBound    `json:"period,omitempty"`
	Stages string                     `json:"stages,omitempty"`
	System map[string]json.RawMessage `json:"system,omitempty"`
	Raw    json.RawMessage            `json:"-"`
	Pack   string                     `json:"-"`
}

// Типы полей формата v0 (pack-format.md, «Поле»). Это словарь формата, а не
// ключи паков: часть названий совпадает с ключами полей kin.core случайно.
const (
	TypeText    = "text"
	TypeNumber  = "number"
	TypeDate    = "partial_date"
	TypeBool    = "bool"
	TypeChoice  = "choice"
	TypeRef     = "ref"
	TypeImage   = "image"
	TypePlace   = "place"
	TypePhone   = "phone"
	TypeEmail   = "email"
	TypeURL     = "url"
	TypeContact = "contact"
)

// FieldTypes — все типы полей формата v0.
var FieldTypes = []string{TypeText, TypeNumber, TypeDate, TypeBool, TypeChoice, TypeRef,
	TypeImage, TypePlace, TypePhone, TypeEmail, TypeURL, TypeContact}

// StageKey — системное поле этапа (F-37); DefaultStage — этап «не выбран».
const (
	StageKey     = "stage"
	DefaultStage = "default"
)

// Field — поле шаблона (основные ключи; остальное — в Raw шаблона).
type Field struct {
	Key     string `json:"key"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	History bool   `json:"history"`
	// Surname — поле «фамилия» записи: по его значениям (все из истории) группы
	// с автодобавлением по фамилии (F-122); движок поле по имени не знает.
	Surname   bool              `json:"surname,omitempty"`
	Multiple  bool              `json:"multiple"`
	Section   string            `json:"section"`
	Show      string            `json:"show"`
	Sensitive bool              `json:"sensitive"`
	Options   []json.RawMessage `json:"options"`
	// Free — у выбора: можно вписать своё значение (F-100); хранится текстом
	// с пометкой FreeMeta в meta значения (F-104).
	Free bool `json:"free"`
	// LocalizedOptions — варианты выбора разные в языковых версиях пака (правовые
	// формы стран); проверка совпадения языковых версий их не сравнивает.
	LocalizedOptions bool              `json:"localized_options,omitempty"`
	Meta             []json.RawMessage `json:"meta"`
	// Entities — допустимые сущности цели поля-ссылки; пусто — любые.
	Entities []string `json:"entities"`
}

// Role — роль участников шаблона.
type Role struct {
	Key      string          `json:"key"`
	Entities []string        `json:"entities"`
	Min      *int            `json:"min"`
	Max      *int            `json:"max"`
	Forms    json.RawMessage `json:"forms"`
}

// Stage — этап шаблона (F-37).
type Stage struct {
	Key  string            `json:"key"`
	Name string            `json:"name"`
	Meta []json.RawMessage `json:"meta"`
}

// Template — шаблон templates/<сущность>/<шаблон>.json.
type Template struct {
	Format    string         `json:"format"`
	Source    string         `json:"source"`
	ID        string         `json:"id"`
	Entity    string         `json:"entity"`
	Name      string         `json:"name"`
	Title     []TitleElement `json:"title"`
	Fields    []Field        `json:"fields"`
	Roles     []Role         `json:"roles"`
	Stages    []Stage        `json:"stages"`
	Sensitive bool           `json:"sensitive"`
	// Contains — шаблоны записей-контейнеров, на которые могут ссылаться
	// поля-ссылки записи (вложенные события, F-82); nil — любые.
	Contains []string `json:"contains"`
	// Participants — общее min / max участников шаблона.
	Participants *struct {
		Min *int `json:"min"`
		Max *int `json:"max"`
	} `json:"participants"`
	// SortBy — ключи полей порядка списка (F-133): по текущему значению каждого;
	// нет — по подписи записи.
	SortBy []string `json:"sort_by,omitempty"`
	// EndFlag — ключ поля bool: истинное значение значит «запись завершена»
	// наравне с заполненным end (F-126); движок поле по имени не знает.
	EndFlag string `json:"end_flag,omitempty"`
	// Links — переопределение links сущности (F-93); nil — как у сущности.
	Links *bool `json:"links,omitempty"`
	// LinksRank — вес записей шаблона в «Связях» (F-103): число, меньше —
	// выше; порядок строит интерфейс, загрузчик только проверяет.
	LinksRank json.RawMessage `json:"links_rank,omitempty"`
	// Period — переопределение подписей start / end сущности; null — даты нет.
	Period map[string]*PeriodBound `json:"period,omitempty"`
	// TreeLink — участвует ли шаблон в дереве (F-111, F-118): nil — нет.
	TreeLink *TreeLink       `json:"tree_link,omitempty"`
	Raw      json.RawMessage `json:"-"`
	Pack     string          `json:"-"`
}

// Как шаблон с tree_link рисуется в дереве.
const (
	TreeAsCouple   = "couple"   // участники рядом, дети — под узлом отношения
	TreeAsSiblings = "siblings" // узел над участниками
	TreeAsDescent  = "descent"  // первая роль — выше второй (родитель — ребёнок)
	// TreeLineDashed — вид линии варианта выбора (`tree_line`, F-118).
	TreeLineDashed = "dashed"
)

// TreeLink — `tree_link` шаблона: `true` или объект { line_by, as, stages }.
// `as` по умолчанию — по ролям: одна роль — couple, две — descent.
type TreeLink struct {
	LineBy string   `json:"line_by,omitempty"`
	As     string   `json:"as,omitempty"`
	Stages []string `json:"stages,omitempty"`
	off    bool     // `tree_link: false`
}

// UnmarshalJSON — `true` / `false` или объект.
func (tl *TreeLink) UnmarshalJSON(data []byte) error {
	var b bool
	if json.Unmarshal(data, &b) == nil {
		*tl = TreeLink{off: !b}
		return nil
	}
	type plain TreeLink
	return json.Unmarshal(data, (*plain)(tl))
}

// InTree — шаблон участвует в дереве (F-111).
func (t *Template) InTree() bool { return t.TreeLink != nil && !t.TreeLink.off }

// PeriodBound — начало или конец в period сущности / шаблона: строка-подпись
// или объект { "name": …, "show": "filled" } (F-99). null в period — даты нет
// (nil в карте).
type PeriodBound struct {
	Name string `json:"name"`
	// Show — как show у поля: always (по умолчанию) · filled.
	Show string `json:"show,omitempty"`
}

// UnmarshalJSON принимает строку-подпись или объект.
func (b *PeriodBound) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		*b = PeriodBound{Name: name}
		return nil
	}
	type plain PeriodBound
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*b = PeriodBound(p)
	return nil
}

// TitleElement — элемент правила подписи; серверу нужны freeedit и ключи полей
// (запасная сортировка списка по подписи, F-133).
type TitleElement struct {
	FreeEdit bool   `json:"freeedit"`
	Field    string `json:"field"`
}

// FreeEdit — разрешена ли ручная подпись (элемент freeedit в правиле title, F-71).
func (t *Template) FreeEdit() bool {
	return slices.ContainsFunc(t.Title, func(e TitleElement) bool { return e.FreeEdit })
}

// Field возвращает поле шаблона по ключу.
func (t *Template) Field(key string) (*Field, bool) {
	for i := range t.Fields {
		if t.Fields[i].Key == key {
			return &t.Fields[i], true
		}
	}
	return nil, false
}

// Role возвращает роль шаблона по ключу.
func (t *Template) Role(key string) (*Role, bool) {
	for i := range t.Roles {
		if t.Roles[i].Key == key {
			return &t.Roles[i], true
		}
	}
	return nil, false
}

// Stage возвращает этап шаблона по ключу (кроме default — он не значение).
func (t *Template) Stage(key string) (*Stage, bool) {
	for i := range t.Stages {
		if t.Stages[i].Key == key && key != DefaultStage {
			return &t.Stages[i], true
		}
	}
	return nil, false
}

// HasStages — вводит ли шаблон этапы (кроме одного default).
func (t *Template) HasStages() bool {
	return slices.ContainsFunc(t.Stages, func(s Stage) bool { return s.Key != DefaultStage })
}

// Keys — ключи элементов вида [{ "key": … }] (options, meta).
func Keys(items []json.RawMessage) []string {
	out := make([]string, 0, len(items))
	for _, raw := range items {
		var k struct {
			Key string `json:"key"`
		}
		if json.Unmarshal(raw, &k) == nil && k.Key != "" {
			out = append(out, k.Key)
		}
	}
	return out
}

// FreeMeta — пометка в meta значения выбора: своё значение, а не ключ
// варианта (F-104).
const FreeMeta = "free"

// OptionByName — ключ варианта выбора по названию (без учёта регистра и
// пробелов по краям): своё значение, совпавшее с вариантом, хранится его
// ключом без пометки (F-100, F-104).
func OptionByName(items []json.RawMessage, name string) (string, bool) {
	name = strings.TrimSpace(name)
	for _, raw := range items {
		var o struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &o) == nil && o.Key != "" && strings.EqualFold(strings.TrimSpace(o.Name), name) {
			return o.Key, true
		}
	}
	return "", false
}

// PeriodLabel — подпись системной даты (start / end) у записи на шаблоне t:
// шаблон переопределяет сущность. ok=false — даты у записи нет ("end": null).
func PeriodLabel(e *Entity, t *Template, key string) (label string, ok bool) {
	ok = true
	if e != nil {
		if v, has := e.Period[key]; has {
			ok = v != nil
			if v != nil {
				label = v.Name
			}
		}
	}
	if t != nil {
		if v, has := t.Period[key]; has {
			ok = v != nil
			label = ""
			if v != nil {
				label = v.Name
			}
		}
	}
	return label, ok
}

// ShowsLinks — показывает ли раздел «Связи» других участников записи шаблона t
// (F-93): links шаблона, иначе сущности, по умолчанию — да.
func ShowsLinks(e *Entity, t *Template) bool {
	if t != nil && t.Links != nil {
		return *t.Links
	}
	if e != nil && e.Links != nil {
		return *e.Links
	}
	return true
}

// Level — серьёзность диагностики.
type Level string

// Уровни: error — приложение не стартует; warn, info — в лог.
const (
	LevelError Level = "error"
	LevelWarn  Level = "warn"
	LevelInfo  Level = "info"
)

// Diagnostic — замечание загрузчика.
type Diagnostic struct {
	Level   Level
	File    string
	Message string
}

func (d Diagnostic) String() string {
	if d.File == "" {
		return fmt.Sprintf("%s: %s", d.Level, d.Message)
	}
	return fmt.Sprintf("%s: %s: %s", d.Level, d.File, d.Message)
}

// Registry — загруженные паки; после загрузки не меняется.
type Registry struct {
	Language string
	// Packs — включённые паки в порядке загрузки (зависимости раньше).
	Packs    []Manifest
	entities map[string]*Entity
	tmpls    map[string]*Template
	// порядок — как в files манифестов
	entityOrder, tmplOrder []string
}

// Entity возвращает сущность по полному id.
func (r *Registry) Entity(id string) (*Entity, bool) { e, ok := r.entities[id]; return e, ok }

// Template возвращает шаблон по id.
func (r *Registry) Template(id string) (*Template, bool) { t, ok := r.tmpls[id]; return t, ok }

// Entities — все сущности в порядке объявления.
func (r *Registry) Entities() []*Entity {
	out := make([]*Entity, 0, len(r.entityOrder))
	for _, id := range r.entityOrder {
		out = append(out, r.entities[id])
	}
	return out
}

// Templates — все шаблоны в порядке объявления.
func (r *Registry) Templates() []*Template {
	out := make([]*Template, 0, len(r.tmplOrder))
	for _, id := range r.tmplOrder {
		out = append(out, r.tmpls[id])
	}
	return out
}

// DefaultTemplate — шаблон с тем же id, что и сущность (F-62).
func (r *Registry) DefaultTemplate(entity string) (*Template, bool) {
	t, ok := r.tmpls[entity]
	if !ok || t.Entity != entity {
		return nil, false
	}
	return t, true
}

// HasPack сообщает, загружен ли пак.
func (r *Registry) HasPack(id string) bool {
	return slices.ContainsFunc(r.Packs, func(m Manifest) bool { return m.ID == id })
}

// HasErrors — есть ли диагностики уровня error.
func HasErrors(ds []Diagnostic) bool {
	return slices.ContainsFunc(ds, func(d Diagnostic) bool { return d.Level == LevelError })
}

type loader struct {
	fsys  fs.FS
	diags []Diagnostic
}

func (l *loader) add(level Level, file, format string, args ...any) {
	l.diags = append(l.diags, Diagnostic{Level: level, File: file, Message: fmt.Sprintf(format, args...)})
}

func (l *loader) readJSON(name string, v any) (json.RawMessage, bool) {
	data, err := fs.ReadFile(l.fsys, name)
	if err != nil {
		return nil, false
	}
	if err := json.Unmarshal(data, v); err != nil {
		l.add(LevelError, name, "неверный JSON: %v", err)
		return nil, false
	}
	return data, true
}

// Load читает паки из fsys (корень — папки паков одного языка: <пак>/<пак>.pack.json)
// и включает enabled вместе с зависимостями из requires. Паки, не попавшие
// в замыкание enabled, не загружаются.
func Load(fsys fs.FS, lang string, enabled []string) (*Registry, []Diagnostic) {
	l := &loader{fsys: fsys}
	reg := &Registry{Language: lang, entities: map[string]*Entity{}, tmpls: map[string]*Template{}}

	// 1. Манифесты всех паков.
	all := map[string]Manifest{}
	dirs, err := fs.ReadDir(fsys, ".")
	if err != nil {
		l.add(LevelError, "", "папка паков: %v", err)
		return reg, l.diags
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		mpath := d.Name() + "/" + d.Name() + ".pack.json"
		var m Manifest
		if _, ok := l.readJSON(mpath, &m); !ok {
			if _, err := fs.Stat(fsys, mpath); err != nil {
				l.add(LevelWarn, d.Name(), "нет манифеста %s.pack.json — папка пропущена", d.Name())
			}
			continue
		}
		if m.Format != Format {
			l.add(LevelWarn, mpath, "format = %q, ожидался %q", m.Format, Format)
		}
		if m.ID != d.Name() {
			l.add(LevelError, mpath, "id пака %q не совпадает с папкой %q", m.ID, d.Name())
			continue
		}
		if m.Language != lang {
			l.add(LevelWarn, mpath, "язык пака %q, а язык инстанса %q", m.Language, lang)
		}
		all[m.ID] = m
	}

	// 2. Замыкание enabled по requires; порядок — зависимости раньше.
	var order []string
	state := map[string]int{} // 1 — в обходе, 2 — готов
	var visit func(id, from string)
	visit = func(id, from string) {
		switch state[id] {
		case 1:
			l.add(LevelError, from, "циклическая зависимость паков через %s", id)
			return
		case 2:
			return
		}
		m, ok := all[id]
		if !ok {
			if from == "" {
				l.add(LevelError, "", "включён пак %s, он не найден", id)
			} else {
				l.add(LevelError, from, "требуется пак %s, он не найден", id)
			}
			return
		}
		state[id] = 1
		for _, req := range m.Requires {
			visit(req, id+"/"+id+".pack.json")
		}
		state[id] = 2
		order = append(order, id)
	}
	for _, id := range enabled {
		visit(id, "")
	}

	// 3. Файлы включённых паков.
	for _, id := range order {
		m := all[id]
		reg.Packs = append(reg.Packs, m)
		l.loadFiles(reg, m)
	}

	// 4. Целостность.
	for _, id := range reg.tmplOrder {
		t := reg.tmpls[id]
		if e, ok := reg.entities[t.Entity]; !ok {
			l.add(LevelError, t.Pack, "шаблон %s: сущность %s не найдена", t.ID, t.Entity)
		} else {
			if t.Links != nil && !e.Container {
				l.add(LevelWarn, t.Pack, "шаблон %s: links у шаблона сущности %s, которая не контейнер, — ни на что не влияет", t.ID, e.ID)
			}
			if t.LinksRank != nil && !e.Container {
				l.add(LevelWarn, t.Pack, "шаблон %s: links_rank у шаблона сущности %s, которая не контейнер, — ни на что не влияет", t.ID, e.ID)
			}
		}
		if t.LinksRank != nil {
			// null тоже не число: интерфейс считает такой шаблон без веса
			var n any
			_ = json.Unmarshal(t.LinksRank, &n)
			if _, ok := n.(float64); !ok {
				l.add(LevelWarn, t.Pack, "шаблон %s: links_rank = %s — не число, вес не учитывается", t.ID, t.LinksRank)
			}
		}
		l.checkKeys(t)
		l.checkQuickAdd(reg.entities[t.Entity], t)
		l.checkTree(reg.entities[t.Entity], t)
		for _, c := range t.Contains {
			if _, ok := reg.tmpls[c]; !ok {
				l.add(LevelWarn, t.Pack, "шаблон %s: в contains неизвестный шаблон %s", t.ID, c)
			}
		}
	}
	for _, id := range reg.entityOrder {
		if e := reg.entities[id]; e.Links != nil && !e.Container {
			l.add(LevelWarn, e.Pack, "сущность %s: links у сущности, которая не контейнер, — ни на что не влияет", id)
		}
		if _, ok := reg.DefaultTemplate(id); !ok {
			l.add(LevelInfo, reg.entities[id].Pack, "у сущности %s нет шаблона по умолчанию (шаблона с тем же id)", id)
		}
	}
	return reg, l.diags
}

func (l *loader) loadFiles(reg *Registry, m Manifest) {
	dir := m.ID + "/"
	listed := map[string]bool{}
	for _, rel := range m.Files {
		full := dir + path.Clean(rel)
		if listed[full] {
			l.add(LevelWarn, full, "файл перечислен в files дважды")
			continue
		}
		listed[full] = true
		var head struct {
			Format string `json:"format"`
			Source string `json:"source"`
		}
		raw, ok := l.readJSON(full, &head)
		if !ok {
			if _, err := fs.Stat(l.fsys, full); err != nil {
				l.add(LevelError, full, "файл из списка files пака %s отсутствует", m.ID)
			}
			continue
		}
		if head.Format != Format {
			l.add(LevelWarn, full, "format = %q, ожидался %q", head.Format, Format)
		}
		if head.Source != m.ID {
			l.add(LevelError, full, "source = %q, а файл заявлен паком %q", head.Source, m.ID)
			continue
		}
		// сущность или шаблон — по папке (раскладка pack-format.md)
		parts := strings.Split(path.Clean(rel), "/")
		switch {
		case len(parts) == 2 && parts[0] == "entities":
			var e Entity
			if _, ok := l.readJSON(full, &e); !ok {
				continue
			}
			e.Raw, e.Pack = raw, m.ID
			l.checkPeriod(full, e.Period)
			if parts[1] != e.ID+".json" {
				l.add(LevelWarn, full, "имя файла не равно id сущности %s", e.ID)
			}
			if _, dup := reg.entities[e.ID]; dup {
				l.add(LevelError, full, "сущность %s уже объявлена", e.ID)
				continue
			}
			reg.entities[e.ID] = &e
			reg.entityOrder = append(reg.entityOrder, e.ID)
		case len(parts) == 3 && parts[0] == "templates":
			var t Template
			if _, ok := l.readJSON(full, &t); !ok {
				continue
			}
			t.Raw, t.Pack = raw, m.ID
			l.checkPeriod(full, t.Period)
			if parts[1] != t.Entity {
				l.add(LevelWarn, full, "шаблон лежит в папке %s, а entity = %s", parts[1], t.Entity)
			}
			if parts[2] != t.ID+".json" {
				l.add(LevelWarn, full, "имя файла не равно id шаблона %s", t.ID)
			}
			if prev, dup := reg.tmpls[t.ID]; dup {
				l.add(LevelError, full, "шаблон %s уже объявлен паком %s", t.ID, prev.Pack)
				continue
			}
			reg.tmpls[t.ID] = &t
			reg.tmplOrder = append(reg.tmplOrder, t.ID)
		default:
			l.add(LevelWarn, full, "файл не в entities/ и не в templates/<сущность>/ — пропущен")
		}
	}
	// файлы пака, не перечисленные в files
	_ = fs.WalkDir(l.fsys, m.ID, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == dir+m.ID+".pack.json" || !strings.HasSuffix(p, ".json") {
			return nil
		}
		if !listed[p] {
			l.add(LevelWarn, p, "файл не перечислен в files пака — пропущен")
		}
		return nil
	})
}

// checkPeriod — ключи period: start / end, show — always или filled (warn).
func (l *loader) checkPeriod(file string, p map[string]*PeriodBound) {
	for _, k := range slices.Sorted(maps.Keys(p)) {
		if k != "start" && k != "end" {
			l.add(LevelWarn, file, "period: неизвестный ключ %q", k)
		}
		if b := p[k]; b != nil && b.Show != "" && b.Show != "always" && b.Show != "filled" {
			l.add(LevelWarn, file, "period.%s: show = %q, ожидалось always или filled", k, b.Show)
		}
	}
}

// checkKeys — Д-7: ключи полей, ролей и зарезервированные не пересекаются (warn).
func (l *loader) checkKeys(t *Template) {
	seen := map[string]string{}
	note := func(key, kind string) {
		switch {
		case slices.Contains(Reserved, key):
			l.add(LevelWarn, t.Pack, "шаблон %s: %s %q совпадает с системным ключом", t.ID, kind, key)
		case strings.HasPrefix(key, CustomPrefix), strings.HasPrefix(key, SharedPrefix):
			l.add(LevelWarn, t.Pack, "шаблон %s: %s %q в пространстве своих полей", t.ID, kind, key)
		}
		if prev, ok := seen[key]; ok {
			l.add(LevelWarn, t.Pack, "шаблон %s: ключ %q — и %s, и %s", t.ID, key, prev, kind)
		}
		seen[key] = kind
	}
	for _, f := range t.Fields {
		note(f.Key, "поле")
		if f.LocalizedOptions && f.Type != TypeChoice {
			l.add(LevelWarn, t.Pack, "шаблон %s: поле %q: localized_options — только у выбора", t.ID, f.Key)
		}
		if f.Free && f.Type != TypeChoice {
			l.add(LevelWarn, t.Pack, "шаблон %s: поле %q: free — только у выбора", t.ID, f.Key)
		}
	}
	for _, r := range t.Roles {
		note(r.Key, "роль")
	}
	for _, k := range t.SortBy {
		if f, ok := t.Field(k); !ok {
			l.add(LevelWarn, t.Pack, "шаблон %s: sort_by: нет поля %q", t.ID, k)
		} else if f.Type != TypeText && f.Type != TypeChoice {
			l.add(LevelWarn, t.Pack, "шаблон %s: sort_by: поле %q — сортировка только по тексту и выбору", t.ID, k)
		}
	}
	if t.EndFlag != "" {
		if f, ok := t.Field(t.EndFlag); !ok || f.Type != TypeBool {
			l.add(LevelWarn, t.Pack, "шаблон %s: end_flag = %q — нет такого поля да/нет", t.ID, t.EndFlag)
		}
	}
}

// choiceOption — вариант выбора: вид линии в дереве и формы ролей (F-118).
type choiceOption struct {
	Key      string                     `json:"key"`
	TreeLine string                     `json:"tree_line"`
	Forms    map[string]json.RawMessage `json:"forms"`
}

// checkTree — tree_link (F-111, F-118), tree_line и forms вариантов выбора
// (warn): неверное не ломает загрузку, дерево его просто не учитывает.
func (l *loader) checkTree(e *Entity, t *Template) {
	for _, f := range t.Fields {
		for i, raw := range f.Options {
			var o choiceOption
			if json.Unmarshal(raw, &o) != nil {
				continue
			}
			if o.TreeLine != "" && o.TreeLine != TreeLineDashed {
				l.add(LevelWarn, t.Pack, "шаблон %s: поле %q, вариант %d: tree_line = %q, ожидалось dashed", t.ID, f.Key, i, o.TreeLine)
			}
			for _, role := range slices.Sorted(maps.Keys(o.Forms)) {
				if _, ok := t.Role(role); !ok {
					l.add(LevelWarn, t.Pack, "шаблон %s: поле %q, вариант %q: формы для неизвестной роли %q", t.ID, f.Key, o.Key, role)
				}
			}
		}
	}
	if !t.InTree() {
		return
	}
	tl := t.TreeLink
	where := fmt.Sprintf("шаблон %s: tree_link", t.ID)
	if e != nil && !e.Container {
		l.add(LevelWarn, t.Pack, "%s — только у отношения (контейнера)", where)
	}
	switch tl.As {
	case "":
		if len(t.Roles) != 1 && len(t.Roles) != 2 {
			l.add(LevelWarn, t.Pack, "%s: as не задан, а ролей %d (ожидалась одна или две)", where, len(t.Roles))
		}
	case TreeAsCouple, TreeAsSiblings:
		if len(t.Roles) != 1 {
			l.add(LevelWarn, t.Pack, "%s: as = %q — нужна одна роль", where, tl.As)
		}
	case TreeAsDescent:
		if len(t.Roles) != 2 {
			l.add(LevelWarn, t.Pack, "%s: as = descent — нужны две роли", where)
		}
	default:
		l.add(LevelWarn, t.Pack, "%s: as = %q, ожидалось couple, siblings или descent", where, tl.As)
	}
	if tl.LineBy != "" {
		if f, ok := t.Field(tl.LineBy); !ok || f.Type != TypeChoice {
			l.add(LevelWarn, t.Pack, "%s: line_by = %q — нет такого поля-выбора", where, tl.LineBy)
		}
	}
	for _, s := range tl.Stages {
		if _, ok := t.Stage(s); !ok {
			l.add(LevelWarn, t.Pack, "%s: нет этапа %q", where, s)
		}
	}
}

// QuickAdd — пункт меню (+) из ui.quick_add шаблона (F-73, F-102). Сервер его
// не исполняет — только проверяет при загрузке; меню строит интерфейс.
type QuickAdd struct {
	Name    string `json:"name"`
	Role    string `json:"role"`
	NewRole string `json:"new_role"`
	// New, Relation — предзаполнение новой записи и создаваемого отношения:
	// константа или элемент грамматики от записи, у которой нажали.
	New      map[string]json.RawMessage `json:"new"`
	Relation map[string]json.RawMessage `json:"relation"`
	Stage    string                     `json:"stage"`
	Single   bool                       `json:"single"`
}

// quickAdds — пункты ui.quick_add шаблона; bad — номера неразборчивых.
func (t *Template) quickAdds() (items []QuickAdd, bad []int) {
	var doc struct {
		UI struct {
			QuickAdd []json.RawMessage `json:"quick_add"`
		} `json:"ui"`
	}
	if json.Unmarshal(t.Raw, &doc) != nil {
		return nil, nil
	}
	for i, raw := range doc.UI.QuickAdd {
		var q QuickAdd
		if json.Unmarshal(raw, &q) != nil {
			bad = append(bad, i)
			continue
		}
		items = append(items, q)
	}
	return items, bad
}

// checkQuickAdd — пункты ui.quick_add: роли и этап шаблона, ключи relation
// (F-102) — поле шаблона, start / end (если дата есть) или .note (warn).
// Значения new / relation не проверяются: константой может быть и объект
// (частичная дата), его не отличить от опечатки в элементе грамматики.
func (l *loader) checkQuickAdd(e *Entity, t *Template) {
	items, bad := t.quickAdds()
	for _, i := range bad {
		l.add(LevelWarn, t.Pack, "шаблон %s: ui.quick_add[%d] не разобран — пропущен", t.ID, i)
	}
	for _, q := range items {
		where := fmt.Sprintf("шаблон %s: quick_add %q", t.ID, q.Name)
		for _, r := range []string{q.Role, q.NewRole} {
			if _, ok := t.Role(r); !ok {
				l.add(LevelWarn, t.Pack, "%s: нет роли %q", where, r)
			}
		}
		if q.Stage != "" {
			if _, ok := t.Stage(q.Stage); !ok {
				l.add(LevelWarn, t.Pack, "%s: нет этапа %q", where, q.Stage)
			}
		}
		for _, k := range slices.Sorted(maps.Keys(q.Relation)) {
			switch k {
			case ".note":
			case "start", "end":
				if _, ok := PeriodLabel(e, t, k); !ok {
					l.add(LevelWarn, t.Pack, "%s: relation.%s — у отношения нет этой даты", where, k)
				}
			default:
				if _, ok := t.Field(k); !ok {
					l.add(LevelWarn, t.Pack, "%s: relation.%s — нет такого поля у шаблона", where, k)
				}
			}
		}
	}
}

// Keys — все ключи полей, ролей, этапов и шаблонов реестра (для проверки
// «пакам — данные»: таких строк не должно быть в коде).
func (r *Registry) Keys() []string {
	set := map[string]bool{}
	for _, t := range r.tmpls {
		set[t.ID] = true
		for _, f := range t.Fields {
			set[f.Key] = true
		}
		for _, ro := range t.Roles {
			set[ro.Key] = true
		}
		for _, s := range t.Stages {
			set[s.Key] = true
		}
	}
	for id := range r.entities {
		set[id] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
