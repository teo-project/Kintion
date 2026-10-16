package records

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kintion/server/internal/audit"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/packs"
	"kintion/server/internal/partialdate"
)

// Значения полей (F-11, F-12): список значений с ручным порядком, верхнее —
// текущее; правила — из шаблона (или из custom_fields записи для custom.*).

// ValueIn — значение поля во входе.
type ValueIn struct {
	Value     json.RawMessage            `json:"value"`
	ValidFrom *partialdate.Date          `json:"valid_from"`
	ValidTo   *partialdate.Date          `json:"valid_to"`
	Note      *string                    `json:"note"`
	Meta      map[string]json.RawMessage `json:"meta"`
	Links     []LinkIn                   `json:"links"`
}

// LinkIn — привязка значения к записи (id или временный id пакета).
type LinkIn struct {
	Target string `json:"target"`
	Anchor string `json:"anchor"`
}

// Anchors — места привязки значения к записи-событию.
var Anchors = []string{"start", "end", "none"}

// Position — место значения в списке: над before_id, под after_id,
// наверх (по умолчанию — новое значение становится текущим) или вниз.
type Position struct {
	BeforeID *uuid.UUID `json:"before_id"`
	AfterID  *uuid.UUID `json:"after_id"`
	Position string     `json:"position"`
}

const (
	posTop    = "top"
	posBottom = "bottom"
)

func (p Position) set() bool { return p.BeforeID != nil || p.AfterID != nil || p.Position != "" }

// AddValue — POST .../fields/{key}/values.
type AddValue struct {
	IfRev *int64 `json:"if_rev"`
	ValueIn
	Position
}

// PatchValue — PATCH .../fields/{key}/values/{vid}: изменить и/или переместить.
type PatchValue struct {
	IfRev     *int64                          `json:"if_rev"`
	Value     Opt[json.RawMessage]            `json:"value"`
	ValidFrom Opt[partialdate.Date]           `json:"valid_from"`
	ValidTo   Opt[partialdate.Date]           `json:"valid_to"`
	Note      Opt[string]                     `json:"note"`
	Meta      Opt[map[string]json.RawMessage] `json:"meta"`
	Links     Opt[[]LinkIn]                   `json:"links"`
	Position
}

// fieldDef — описание поля записи: поле шаблона, системный этап (F-37) или
// свободное поле из custom_fields (F-75).
type fieldDef struct {
	*packs.Field
	stage bool
}

func (t *Tx) fieldDef(r *Rec, key string) (fieldDef, error) {
	switch {
	case key == packs.StageKey:
		if !r.T.HasStages() {
			return fieldDef{}, invalid(key, "у шаблона нет этапов")
		}
		return fieldDef{Field: &packs.Field{Key: key, Type: packs.TypeText, History: true}, stage: true}, nil
	case strings.HasPrefix(key, packs.CustomPrefix):
		defs, err := customDefs(r.Row.CustomFields)
		if err != nil {
			return fieldDef{}, err
		}
		for i := range defs {
			if packs.CustomPrefix+defs[i].Key == key {
				return fieldDef{Field: &defs[i]}, nil
			}
		}
		return fieldDef{}, invalid(key, "у записи нет свободного поля %q", key)
	case strings.HasPrefix(key, packs.SharedPrefix):
		// свежее описание под разделяемой блокировкой: правка общих полей
		// (PatchShared, FOR UPDATE) не разойдётся с записью значения (F-96)
		row, err := t.q.ShareLockDashboardTemplate(t.ctx, sqlc.ShareLockDashboardTemplateParams{
			DashboardID: t.Dashboard, ID: r.Row.TemplateID,
		})
		if err != nil {
			return fieldDef{}, err
		}
		defs, err := sharedDefs(row.Overrides)
		if err != nil {
			return fieldDef{}, err
		}
		for i := range defs {
			if packs.SharedPrefix+defs[i].Key == key {
				return fieldDef{Field: &defs[i]}, nil
			}
		}
		return fieldDef{}, invalid(key, "у шаблона нет общего поля %q", key)
	}
	if f, ok := r.T.Field(key); ok {
		return fieldDef{Field: f}, nil
	}
	return fieldDef{}, invalid(key, "у шаблона нет поля %q", key)
}

// cols — значение по колонкам field_entry (план §4, таблица типов).
type cols struct {
	text *string
	num  pgtype.Numeric
	date *partialdate.Date
	b    *bool
	ref  *uuid.UUID
	meta []byte
}

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

// resolver — id записи по строке (в пакете — и по временному id).
type resolver func(s, field string) (uuid.UUID, error)

func (t *Tx) plainResolver(s, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, invalid(field, "неверный id записи")
	}
	if _, err := t.target(id, field); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// convert проверяет значение по типу поля и раскладывает по колонкам.
func convert(d fieldDef, raw json.RawMessage, resolve resolver) (cols, error) {
	key := d.Key
	var c cols
	if len(raw) == 0 || string(raw) == "null" {
		return c, invalid(key, "нужно значение") // недостающее не храним (F-42)
	}
	str := func() (string, error) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", invalid(key, "ожидается строка")
		}
		if strings.TrimSpace(s) == "" {
			return "", invalid(key, "пустое значение")
		}
		return s, nil
	}
	switch d.Type {
	case packs.TypeText, packs.TypePlace, packs.TypePhone, packs.TypeEmail, packs.TypeURL, packs.TypeContact:
		s, err := str()
		if err != nil {
			return c, err
		}
		c.text = &s
	case packs.TypeChoice:
		s, err := str()
		if err != nil {
			return c, err
		}
		// своё значение (F-100) размечает markFree по пометке (F-104)
		if !d.Free && !slices.Contains(packs.Keys(d.Options), s) {
			return c, invalid(key, "нет варианта %q", s)
		}
		c.text = &s
	case packs.TypeImage:
		s, err := str()
		if err != nil {
			return c, err
		}
		if !sha256Re.MatchString(s) {
			return c, invalid(key, "ожидается sha256 файла")
		}
		c.text = &s
	case packs.TypeNumber:
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var n json.Number
		if err := dec.Decode(&n); err != nil {
			return c, invalid(key, "ожидается число")
		}
		if err := c.num.Scan(n.String()); err != nil {
			return c, invalid(key, "ожидается число")
		}
	case packs.TypeDate:
		var pd partialdate.Date
		if err := json.Unmarshal(raw, &pd); err != nil {
			return c, invalid(key, "неверная частичная дата: %v", err)
		}
		c.date = &pd
	case packs.TypeBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return c, invalid(key, "ожидается да или нет")
		}
		c.b = &b
	case packs.TypeRef:
		s, err := str()
		if err != nil {
			return c, err
		}
		id, err := resolve(s, key)
		if err != nil {
			return c, err
		}
		c.ref = &id
	default:
		return c, invalid(key, "неизвестный тип поля %q", d.Type)
	}
	return c, nil
}

// markFree размечает значение выбора со своим значением (F-104): пометка
// FreeMeta в meta — своё значение, текст как есть, даже если равен ключу
// варианта; без пометки — ключ варианта, а не ключ — своё значение (пометка
// ставится). Текст, совпавший с названием варианта, — ключ без пометки
// (byName; правка одной meta смысла значения не меняет — без этого шага).
// У остальных полей пометки не бывает.
func markFree(d fieldDef, c *cols, meta map[string]json.RawMessage, byName bool) (map[string]json.RawMessage, error) {
	raw, has := meta[packs.FreeMeta]
	if d.Type != packs.TypeChoice || !d.Free || c.text == nil {
		if has {
			return nil, invalid(d.Key, "пометка %s — только у выбора со своим значением", packs.FreeMeta)
		}
		return meta, nil
	}
	var marked bool
	if has && json.Unmarshal(raw, &marked) != nil {
		return nil, invalid(d.Key, "пометка %s — да или нет", packs.FreeMeta)
	}
	out := maps.Clone(meta)
	if out == nil {
		out = map[string]json.RawMessage{}
	}
	delete(out, packs.FreeMeta)
	if !marked && slices.Contains(packs.Keys(d.Options), *c.text) {
		return out, nil
	}
	if k, ok := packs.OptionByName(d.Options, *c.text); ok && byName {
		*c.text = k
		return out, nil
	}
	out[packs.FreeMeta] = json.RawMessage("true")
	return out, nil
}

// isTrue — JSON true.
func isTrue(raw json.RawMessage) bool {
	var b bool
	return json.Unmarshal(raw, &b) == nil && b
}

// metaOf проверяет ключи meta: только объявленные в поле или в этапе (F-12);
// у place и contact meta — структура значения, ключи свободны (F-63, F-65).
func metaOf(r *Rec, d fieldDef, c cols, meta map[string]json.RawMessage) ([]byte, error) {
	if len(meta) == 0 {
		return []byte("{}"), nil
	}
	if d.Type != packs.TypePlace && d.Type != packs.TypeContact {
		allowed := packs.Keys(d.Meta)
		if d.stage && c.text != nil {
			if st, ok := r.T.Stage(*c.text); ok {
				allowed = packs.Keys(st.Meta)
			}
		}
		keys := make([]string, 0, len(meta))
		for k := range meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if d.Type == packs.TypeChoice && d.Free {
			allowed = append(slices.Clone(allowed), packs.FreeMeta) // F-104
		}
		for _, k := range keys {
			if !slices.Contains(allowed, k) {
				return nil, invalid(d.Key, "неизвестный ключ meta %q", k)
			}
		}
	}
	return json.Marshal(meta)
}

// valueRow — проверенное значение, готовое к записи.
type valueRow struct {
	cols
	validFrom, validTo *partialdate.Date
	note               *string
	links              []sqlc.EntryLink
}

func (t *Tx) checkLinks(in []LinkIn, key string, resolve resolver) ([]sqlc.EntryLink, error) {
	out := make([]sqlc.EntryLink, 0, len(in))
	for _, l := range in {
		if !slices.Contains(Anchors, l.Anchor) {
			return nil, invalid(key, "привязка: место %q — только start, end или none", l.Anchor)
		}
		id, err := resolve(l.Target, key)
		if err != nil {
			return nil, err
		}
		if err := t.anchorTarget(id, key); err != nil {
			return nil, err
		}
		out = append(out, sqlc.EntryLink{TargetID: id, Anchor: l.Anchor})
	}
	return out, nil
}

func (t *Tx) checkValue(r *Rec, d fieldDef, in ValueIn, resolve resolver) (valueRow, error) {
	c, err := convert(d, in.Value, resolve)
	if err != nil {
		return valueRow{}, err
	}
	if c.ref != nil {
		if err := t.refTarget(r, d, *c.ref); err != nil {
			return valueRow{}, err
		}
	}
	meta, err := markFree(d, &c, in.Meta, true)
	if err != nil {
		return valueRow{}, err
	}
	if c.meta, err = metaOf(r, d, c, meta); err != nil {
		return valueRow{}, err
	}
	links, err := t.checkLinks(in.Links, d.Key, resolve)
	if err != nil {
		return valueRow{}, err
	}
	return valueRow{cols: c, validFrom: in.ValidFrom, validTo: in.ValidTo, note: normText(in.Note), links: links}, nil
}

// single — у поля не больше одного значения: без истории и без multiple (F-11).
func single(d fieldDef) bool { return !d.History && !d.Multiple }

// valueSnap — значение в журнале.
type valueSnap struct {
	ValueDTO
	Field   string `json:"field"`
	SortKey string `json:"sort_key"`
}

func snapValue(e sqlc.FieldEntry, links []sqlc.EntryLink) valueSnap {
	ls := make([]LinkDTO, 0, len(links))
	for _, l := range links {
		ls = append(ls, LinkDTO{Target: l.TargetID, Anchor: l.Anchor})
	}
	return valueSnap{ValueDTO: valueDTO(e, ls, true), Field: e.FieldKey, SortKey: e.SortKey}
}

func (t *Tx) auditValue(op string, entity uuid.UUID, before, after *valueSnap) error {
	e := audit.Entry{Object: "field_entry", Op: op, Entity: &entity}
	if before != nil {
		e.ObjectID, e.Before = &before.ID, before
	}
	if after != nil {
		e.ObjectID, e.After = &after.ID, after
	}
	return t.audit(e)
}

func (t *Tx) linksOf(entry uuid.UUID) ([]sqlc.EntryLink, error) {
	return t.q.ListEntryLinksByEntries(t.ctx, []uuid.UUID{entry})
}

func (t *Tx) insertLinks(entry uuid.UUID, links []sqlc.EntryLink) ([]sqlc.EntryLink, error) {
	out := make([]sqlc.EntryLink, 0, len(links))
	for _, l := range links {
		row, err := t.q.InsertEntryLink(t.ctx, sqlc.InsertEntryLinkParams{
			ID: newID(), DashboardID: t.Dashboard, EntryID: entry, TargetID: l.TargetID, Anchor: l.Anchor,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

// sortKey — ключ для места pos в списке list (без перемещаемого значения).
func sortKey(list []sqlc.FieldEntry, pos Position, key string) (string, error) {
	keys := make([]string, len(list))
	ids := make([]uuid.UUID, len(list))
	for i, e := range list {
		keys[i], ids[i] = e.SortKey, e.ID
	}
	return between(keys, ids, pos, key)
}

// between — ключ порядка по соседям (общий для значений, участников, групп).
func between(keys []string, ids []uuid.UUID, pos Position, field string) (string, error) {
	n := len(keys)
	idx := func(id uuid.UUID) (int, error) {
		i := slices.Index(ids, id)
		if i < 0 {
			return 0, invalid(field, "соседа %s в списке нет", id)
		}
		return i, nil
	}
	var at int // вставить перед позицией at
	switch {
	case pos.BeforeID != nil:
		i, err := idx(*pos.BeforeID)
		if err != nil {
			return "", err
		}
		at = i
	case pos.AfterID != nil:
		i, err := idx(*pos.AfterID)
		if err != nil {
			return "", err
		}
		at = i + 1
	case pos.Position == posBottom:
		at = n
	case pos.Position == "" || pos.Position == posTop:
		at = 0
	default:
		return "", invalid(field, "position — top или bottom")
	}
	lo, hi := "", ""
	if at > 0 {
		lo = keys[at-1]
	}
	if at < n {
		hi = keys[at]
	}
	return KeyBetween(lo, hi)
}

// fieldCurrent — текущее состояние поля (ответ 409).
func (t *Tx) fieldCurrent(r *Rec, key string) func() (any, error) {
	return func() (any, error) {
		d, err := t.Envelope(r)
		if err != nil {
			return nil, err
		}
		if f, ok := d.Fields[key]; ok {
			return f, nil
		}
		return &FieldDTO{Values: []ValueDTO{}}, nil
	}
}

// AddValue добавляет значение поля.
func (t *Tx) AddValue(r *Rec, key string, in AddValue) (*Rec, error) {
	if err := live(r); err != nil {
		return nil, err
	}
	d, err := t.fieldDef(r, key)
	if err != nil {
		return nil, err
	}
	if err := t.lockRev(r.Row.ID, key, in.IfRev, t.fieldCurrent(r, key)); err != nil {
		return nil, err
	}
	list, err := t.q.ListFieldEntriesByKey(t.ctx, sqlc.ListFieldEntriesByKeyParams{EntityID: r.Row.ID, FieldKey: key})
	if err != nil {
		return nil, err
	}
	if single(d) && len(list) > 0 {
		return nil, invalid(key, "у поля одно значение — измените его")
	}
	v, err := t.checkValue(r, d, in.ValueIn, t.plainResolver)
	if err != nil {
		return nil, err
	}
	sk, err := sortKey(list, in.Position, key)
	if err != nil {
		return nil, err
	}
	if _, err := t.insertValue(r.Row.ID, key, sk, v); err != nil {
		return nil, err
	}
	return t.bumped(r, key)
}

func (t *Tx) insertValue(entity uuid.UUID, key, sk string, v valueRow) (sqlc.FieldEntry, error) {
	e, err := t.q.InsertFieldEntry(t.ctx, sqlc.InsertFieldEntryParams{
		ID: newID(), DashboardID: t.Dashboard, EntityID: entity, FieldKey: key, SortKey: sk,
		VText: v.text, VNum: v.num, VDate: v.date, VBool: v.b, VRef: v.ref, Meta: v.meta,
		ValidFrom: v.validFrom, ValidTo: v.validTo, Note: v.note,
	})
	if err != nil {
		return e, err
	}
	links, err := t.insertLinks(e.ID, v.links)
	if err != nil {
		return e, err
	}
	s := snapValue(e, links)
	return e, t.auditValue(audit.OpCreate, entity, nil, &s)
}

// bumped — новая версия поля и свежая строка записи.
func (t *Tx) bumped(r *Rec, key string) (*Rec, error) {
	if _, err := t.bumpRev(r.Row.ID, key); err != nil {
		return nil, err
	}
	row, err := t.q.GetEntityByID(t.ctx, r.Row.ID)
	if err != nil {
		return nil, err
	}
	out := &Rec{Row: row, Tmpl: r.Tmpl}
	if slices.Contains(surnameFields(r.T), key) {
		if err := t.autoGroups(out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// entry — значение vid поля key записи r.
func (t *Tx) entry(r *Rec, key string, vid uuid.UUID) (sqlc.FieldEntry, error) {
	e, err := t.q.GetFieldEntry(t.ctx, sqlc.GetFieldEntryParams{DashboardID: t.Dashboard, ID: vid})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (e.EntityID != r.Row.ID || e.FieldKey != key)) {
		return e, ErrNotFound
	}
	return e, err
}

// PatchValue меняет значение и/или его место; «сделать основным» — position: top.
func (t *Tx) PatchValue(r *Rec, key string, vid uuid.UUID, in PatchValue) (*Rec, error) {
	if err := live(r); err != nil {
		return nil, err
	}
	d, err := t.fieldDef(r, key)
	if err != nil {
		return nil, err
	}
	if err := t.lockRev(r.Row.ID, key, in.IfRev, t.fieldCurrent(r, key)); err != nil {
		return nil, err
	}
	old, err := t.entry(r, key, vid)
	if err != nil {
		return nil, err
	}
	oldLinks, err := t.linksOf(old.ID)
	if err != nil {
		return nil, err
	}
	before := snapValue(old, oldLinks)

	// новое значение = старое + изменённые ключи
	merged := ValueIn{ValidFrom: old.ValidFrom, ValidTo: old.ValidTo, Note: old.Note}
	if in.Value.Set {
		if in.Value.V == nil {
			return nil, invalid(key, "нужно значение: удалите его, если оно не нужно")
		}
		merged.Value = *in.Value.V
	}
	if in.ValidFrom.Set {
		merged.ValidFrom = in.ValidFrom.V
	}
	if in.ValidTo.Set {
		merged.ValidTo = in.ValidTo.V
	}
	if in.Note.Set {
		merged.Note = in.Note.V
	}
	v := valueRow{
		cols:      cols{text: old.VText, num: old.VNum, date: old.VDate, b: old.VBool, ref: old.VRef, meta: old.Meta},
		validFrom: merged.ValidFrom, validTo: merged.ValidTo, note: normText(merged.Note),
	}
	if in.Value.Set {
		c, err := convert(d, merged.Value, t.plainResolver)
		if err != nil {
			return nil, err
		}
		if c.ref != nil {
			if err := t.refTarget(r, d, *c.ref); err != nil {
				return nil, err
			}
		}
		c.meta = old.Meta
		v.cols = c
	}
	if in.Meta.Set || in.Value.Set {
		meta := map[string]json.RawMessage{}
		oldMeta := map[string]json.RawMessage{}
		if err := json.Unmarshal(old.Meta, &oldMeta); err != nil {
			return nil, err
		}
		if in.Meta.Set {
			if in.Meta.V != nil {
				meta = maps.Clone(*in.Meta.V)
			}
		} else {
			meta = oldMeta
		}
		// пометка своего значения (F-104) — о самом значении: новое значение
		// без неё размечается заново, правка одной meta её не меняет
		switch {
		case in.Value.Set && !in.Meta.Set:
			delete(meta, packs.FreeMeta)
		case !in.Value.Set:
			// присланная пометка проверяется, но смысла значения не меняет
			if _, err := markFree(d, &v.cols, meta, false); err != nil {
				return nil, err
			}
			delete(meta, packs.FreeMeta)
			if f, ok := oldMeta[packs.FreeMeta]; ok {
				meta[packs.FreeMeta] = f
			}
		}
		// своё значение, присланное заново тем же текстом с пометкой (правка
		// примечания, дат и т. п.), не становится вариантом с таким же
		// названием; явно снятая пометка — сопоставляется
		byName := in.Value.Set
		if isTrue(oldMeta[packs.FreeMeta]) && isTrue(meta[packs.FreeMeta]) && old.VText != nil && v.text != nil && *old.VText == *v.text {
			byName = false
		}
		if meta, err = markFree(d, &v.cols, meta, byName); err != nil {
			return nil, err
		}
		if v.meta, err = metaOf(r, d, v.cols, meta); err != nil {
			return nil, err
		}
	}
	links := oldLinks
	if in.Links.Set {
		var ls []LinkIn
		if in.Links.V != nil {
			ls = *in.Links.V
		}
		if links, err = t.checkLinks(ls, key, t.plainResolver); err != nil {
			return nil, err
		}
	}
	sk := old.SortKey
	if in.set() {
		list, err := t.q.ListFieldEntriesByKey(t.ctx, sqlc.ListFieldEntriesByKeyParams{EntityID: r.Row.ID, FieldKey: key})
		if err != nil {
			return nil, err
		}
		list = slices.DeleteFunc(list, func(e sqlc.FieldEntry) bool { return e.ID == old.ID })
		if sk, err = sortKey(list, in.Position, key); err != nil {
			return nil, err
		}
	}
	ne, err := t.q.UpdateFieldEntry(t.ctx, sqlc.UpdateFieldEntryParams{
		ID: old.ID, SortKey: sk, VText: v.text, VNum: v.num, VDate: v.date, VBool: v.b, VRef: v.ref, Meta: v.meta,
		ValidFrom: v.validFrom, ValidTo: v.validTo, Note: v.note,
	})
	if err != nil {
		return nil, err
	}
	if in.Links.Set {
		if err := t.q.DeleteEntryLinksByEntry(t.ctx, old.ID); err != nil {
			return nil, err
		}
		if links, err = t.insertLinks(old.ID, links); err != nil {
			return nil, err
		}
	}
	after := snapValue(ne, links)
	op := audit.OpUpdate
	if sk != old.SortKey && !in.Value.Set && !in.ValidFrom.Set && !in.ValidTo.Set && !in.Note.Set && !in.Meta.Set && !in.Links.Set {
		op = audit.OpMove
	}
	if err := t.auditValue(op, r.Row.ID, &before, &after); err != nil {
		return nil, err
	}
	return t.bumped(r, key)
}

// DeleteValue удаляет значение поля.
func (t *Tx) DeleteValue(r *Rec, key string, vid uuid.UUID, ifRev *int64) (*Rec, error) {
	if err := live(r); err != nil {
		return nil, err
	}
	if _, err := t.fieldDef(r, key); err != nil {
		return nil, err
	}
	if err := t.lockRev(r.Row.ID, key, ifRev, t.fieldCurrent(r, key)); err != nil {
		return nil, err
	}
	old, err := t.entry(r, key, vid)
	if err != nil {
		return nil, err
	}
	links, err := t.linksOf(old.ID)
	if err != nil {
		return nil, err
	}
	if err := t.q.DeleteFieldEntry(t.ctx, old.ID); err != nil {
		return nil, err
	}
	before := snapValue(old, links)
	if err := t.auditValue(audit.OpDelete, r.Row.ID, &before, nil); err != nil {
		return nil, err
	}
	return t.bumped(r, key)
}

// createValues — значения новых записей пакета: по порядку списка, сверху вниз.
// Ошибка значения не отменяет пакет (F-15): значение пропускается; в строгом
// пакете (Strict, F-88) — отменяет.
func (t *Tx) createValues(b Batch, res *BatchResult, refs *batchRefs) error {
	for i, n := range b.Records {
		r := res.Records[i]
		keys := make([]string, 0, len(n.Fields))
		for k := range n.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fail := func(err error) error {
				var ve *ValidationError
				if !errors.As(err, &ve) || b.Strict {
					return err
				}
				res.Errors = append(res.Errors, FieldError{Ref: n.Ref, Field: key, Message: ve.Message})
				return nil
			}
			d, err := t.fieldDef(r, key)
			if err != nil {
				if err := fail(err); err != nil {
					return err
				}
				continue
			}
			last := ""
			for j, in := range n.Fields[key] {
				if j > 0 && single(d) {
					if err := fail(invalid(key, "у поля одно значение")); err != nil {
						return err
					}
					break
				}
				v, err := t.checkValue(r, d, in, refs.resolve)
				if err != nil {
					if err := fail(err); err != nil {
						return err
					}
					continue
				}
				sk, err := KeyBetween(last, "")
				if err != nil {
					return err
				}
				if _, err := t.insertValue(r.Row.ID, key, sk, v); err != nil {
					return err
				}
				last = sk
			}
		}
	}
	return nil
}

// customTypes — типы своих полей (F-96): все типы формата, кроме «контакта»
// (это частный случай ссылки на запись).
func customType(typ string) bool {
	return typ != packs.TypeContact && slices.Contains(packs.FieldTypes, typ)
}

// checkCustomList — описания своих полей (записи или общих для шаблона, F-96):
// ключ, тип, название; у «выбора» — варианты с ключом и названием; у ссылки —
// ограничение сущностями установленных паков.
func (t *Tx) checkCustomList(field string, list []json.RawMessage) (map[string]packs.Field, error) {
	seen := map[string]packs.Field{}
	for _, raw := range list {
		var f packs.Field
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, invalid(field, "неверное описание поля: %v", err)
		}
		switch {
		case !customKeyRe.MatchString(f.Key):
			return nil, invalid(field, "ключ %q: латиница в нижнем регистре, цифры, _", f.Key)
		case !customType(f.Type):
			return nil, invalid(field, "поле %q: тип %q недоступен для своего поля", f.Key, f.Type)
		case strings.TrimSpace(f.Name) == "":
			return nil, invalid(field, "поле %q: нужно название", f.Key)
		}
		if _, dup := seen[f.Key]; dup {
			return nil, invalid(field, "ключ %q повторяется", f.Key)
		}
		if err := checkOptions(field, f); err != nil {
			return nil, err
		}
		if f.Free && f.Type != packs.TypeChoice {
			return nil, invalid(field, "поле %q: своё значение — только у выбора", f.Key)
		}
		if len(f.Entities) > 0 && f.Type != packs.TypeRef {
			return nil, invalid(field, "поле %q: ограничение сущностью — только у ссылки", f.Key)
		}
		for _, e := range f.Entities {
			if _, ok := t.s.Reg.Entity(e); !ok {
				return nil, invalid(field, "поле %q: неизвестная сущность %q", f.Key, e)
			}
		}
		seen[f.Key] = f
	}
	return seen, nil
}

// checkOptions — варианты «выбора»: хотя бы один, ключи и названия непустые,
// ключи не повторяются. У других типов вариантов нет.
func checkOptions(field string, f packs.Field) error {
	if f.Type != packs.TypeChoice {
		if len(f.Options) > 0 {
			return invalid(field, "поле %q: варианты — только у выбора", f.Key)
		}
		return nil
	}
	if len(f.Options) == 0 {
		return invalid(field, "поле %q: у выбора нужны варианты", f.Key)
	}
	keys := map[string]bool{}
	for _, raw := range f.Options {
		var o struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &o); err != nil || !customKeyRe.MatchString(o.Key) || strings.TrimSpace(o.Name) == "" {
			return invalid(field, "поле %q: у варианта нужны ключ (латиница, цифры, _) и название", f.Key)
		}
		if keys[o.Key] {
			return invalid(field, "поле %q: вариант %q повторяется", f.Key, o.Key)
		}
		keys[o.Key] = true
	}
	return nil
}

// usedValue — значение выбора в базе: текст и пометка своего (F-104).
type usedValue struct {
	Text string
	Free bool
}

// usedOptions — варианты выбора, которые хранят значения, нельзя убрать:
// значения остались бы без подписи. Свои значения различаются по пометке
// (F-104): они не варианты и не становятся вариантом с тем же ключом;
// пока они есть, выбор нельзя сделать строгим.
func usedOptions(field string, f packs.Field, used []usedValue) error {
	keys := packs.Keys(f.Options)
	for _, v := range used {
		switch {
		case v.Free && !f.Free:
			return invalid(field, "поле %q: есть свои значения (%q) — сначала измените их", f.Key, v.Text)
		case !v.Free && !slices.Contains(keys, v.Text):
			return invalid(field, "поле %q: вариант %q используется в значениях — сначала измените их", f.Key, v.Text)
		}
	}
	return nil
}

// customKeyRe — ключ свободного поля (без префикса custom.).
var customKeyRe = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

func customDefs(raw []byte) ([]packs.Field, error) {
	var defs []packs.Field
	if len(raw) == 0 {
		return defs, nil
	}
	if err := json.Unmarshal(raw, &defs); err != nil {
		return nil, err
	}
	return defs, nil
}

// customFields проверяет описания свободных полей записи (F-61, F-75): та же
// форма, что поле шаблона; ключ — без префикса custom.; поле со значениями
// нельзя убрать или сменить ему тип.
func (t *Tx) customFields(r *Rec, list []json.RawMessage) ([]byte, error) {
	const field = "custom_fields"
	seen, err := t.checkCustomList(field, list)
	if err != nil {
		return nil, err
	}
	old, err := customDefs(r.Row.CustomFields)
	if err != nil {
		return nil, err
	}
	used, err := t.q.ListFieldKeysByEntity(t.ctx, r.Row.ID)
	if err != nil {
		return nil, err
	}
	for _, o := range old {
		if !slices.Contains(used, packs.CustomPrefix+o.Key) {
			continue
		}
		nf, ok := seen[o.Key]
		if !ok || nf.Type != o.Type {
			return nil, invalid(field, "у поля %q есть значения: сначала удалите их", o.Key)
		}
		if nf.Type == packs.TypeChoice {
			vals, err := t.q.ListFieldTextsByEntity(t.ctx, sqlc.ListFieldTextsByEntityParams{
				EntityID: r.Row.ID, FieldKey: packs.CustomPrefix + o.Key,
			})
			if err != nil {
				return nil, err
			}
			used := make([]usedValue, len(vals))
			for i, v := range vals {
				used[i] = usedValue{Text: v.Text, Free: v.Free}
			}
			if err := usedOptions(field, nf, used); err != nil {
				return nil, err
			}
		}
	}
	if list == nil {
		list = []json.RawMessage{}
	}
	return json.Marshal(list)
}
