package records

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"

	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/partialdate"
)

// RecordDTO — конверт записи (план §5). Подписи, формы ролей и меню (+)
// считает фронтенд по реестру (Д-12).
type RecordDTO struct {
	ID             uuid.UUID            `json:"id"`
	Entity         string               `json:"entity"`
	Template       string               `json:"template"`
	TemplateID     uuid.UUID            `json:"template_id"`
	Start          *PeriodDTO           `json:"start"`
	End            *PeriodDTO           `json:"end"`
	Note           *string              `json:"note"`
	TitleManual    *string              `json:"title_manual"`
	Sensitive      bool                 `json:"sensitive"`
	Groups         []uuid.UUID          `json:"groups"`
	CustomFields   []json.RawMessage    `json:"custom_fields"`
	Fields         map[string]*FieldDTO `json:"fields"`
	Revs           map[string]int64     `json:"revs"` // версии системных полей (F-45)
	Participants   []ParticipantDTO     `json:"participants"`
	ParticipatesIn []ParticipationDTO   `json:"participates_in"`
	Deleted        bool                 `json:"deleted"`

	refs []uuid.UUID // соседи для related
}

// PeriodDTO — системная дата с привязкой к записи-событию.
type PeriodDTO struct {
	Value *partialdate.Date `json:"value"`
	Link  *uuid.UUID        `json:"link"`
}

// FieldDTO — поле: версия и значения в ручном порядке (верхнее — текущее).
type FieldDTO struct {
	Rev    int64      `json:"rev"`
	Values []ValueDTO `json:"values"`
}

// ValueDTO — значение поля. Value — по колонке: строка, число, частичная
// дата, да/нет, id записи (null — цель ссылки не видна).
type ValueDTO struct {
	ID        uuid.UUID         `json:"id"`
	Value     any               `json:"value"`
	ValidFrom *partialdate.Date `json:"valid_from"`
	ValidTo   *partialdate.Date `json:"valid_to"`
	Note      *string           `json:"note"`
	Meta      json.RawMessage   `json:"meta"`
	Links     []LinkDTO         `json:"links"`
}

// LinkDTO — привязка значения к записи: start / end / none (F-11).
type LinkDTO struct {
	Target uuid.UUID `json:"target"`
	Anchor string    `json:"anchor"`
}

// ParticipantDTO — участник контейнера.
type ParticipantDTO struct {
	ID        uuid.UUID         `json:"id"`
	EntityID  uuid.UUID         `json:"entity_id"`
	Role      string            `json:"role"`
	ValidFrom *partialdate.Date `json:"valid_from"`
	ValidTo   *partialdate.Date `json:"valid_to"`
	Note      *string           `json:"note"`
}

// ParticipationDTO — участие записи в чужом контейнере.
type ParticipationDTO struct {
	ID          uuid.UUID         `json:"id"`
	ContainerID uuid.UUID         `json:"container_id"`
	Role        string            `json:"role"`
	ValidFrom   *partialdate.Date `json:"valid_from"`
	ValidTo     *partialdate.Date `json:"valid_to"`
	Note        *string           `json:"note"`
}

// systemRevKeys — системные поля записи с версиями (без участников ролей).
var systemRevKeys = []string{RevStart, RevEnd, RevNote, RevTitle, RevSensitive, RevGroups, RevCustom}

func isSystemRev(key string) bool {
	return slices.Contains(systemRevKeys, key) || strings.HasPrefix(key, revRoleStart)
}

// Envelope — конверт одной записи.
func (t *Tx) Envelope(r *Rec) (*RecordDTO, error) {
	m, err := t.Envelopes([]*Rec{r})
	if err != nil {
		return nil, err
	}
	return m[r.Row.ID], nil
}

// Envelopes собирает конверты записей пачкой. Ссылки на невидимые записи
// (корзина, неактивный шаблон) не показываются: участники и участия
// опускаются, привязки и start/end-ссылки — тоже, значение ref — null.
func (t *Tx) Envelopes(recs []*Rec) (map[uuid.UUID]*RecordDTO, error) {
	out := map[uuid.UUID]*RecordDTO{}
	if len(recs) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, 0, len(recs))
	for _, r := range recs {
		ids = append(ids, r.Row.ID)
	}
	ctx, q := t.ctx, t.q
	entries, err := q.ListFieldEntriesByEntities(ctx, ids)
	if err != nil {
		return nil, err
	}
	entryIDs := make([]uuid.UUID, 0, len(entries))
	for _, e := range entries {
		entryIDs = append(entryIDs, e.ID)
	}
	links, err := q.ListEntryLinksByEntries(ctx, entryIDs)
	if err != nil {
		return nil, err
	}
	revs, err := q.ListFieldRevsByEntities(ctx, ids)
	if err != nil {
		return nil, err
	}
	parts, err := q.ListParticipantsByContainers(ctx, ids)
	if err != nil {
		return nil, err
	}
	partsIn, err := q.ListParticipationsByEntities(ctx, ids)
	if err != nil {
		return nil, err
	}
	groups, err := q.ListGroupsByEntities(ctx, ids)
	if err != nil {
		return nil, err
	}

	// видимость всех упомянутых записей
	var refIDs []uuid.UUID
	for _, r := range recs {
		for _, l := range []*uuid.UUID{r.Row.StartLink, r.Row.EndLink} {
			if l != nil {
				refIDs = append(refIDs, *l)
			}
		}
	}
	for _, e := range entries {
		if e.VRef != nil {
			refIDs = append(refIDs, *e.VRef)
		}
	}
	for _, l := range links {
		refIDs = append(refIDs, l.TargetID)
	}
	for _, p := range parts {
		refIDs = append(refIDs, p.EntityID)
	}
	for _, p := range partsIn {
		refIDs = append(refIDs, p.ContainerID)
	}
	vis, err := t.visibleRows(refIDs)
	if err != nil {
		return nil, err
	}
	seen := func(id *uuid.UUID) *uuid.UUID {
		if id == nil {
			return nil
		}
		if _, ok := vis[*id]; !ok {
			return nil
		}
		return id
	}

	for _, r := range recs {
		row := r.Row
		d := &RecordDTO{
			ID: row.ID, Entity: row.Entity, Template: r.T.ID, TemplateID: row.TemplateID,
			Note: row.Note, TitleManual: row.TitleManual, Sensitive: row.Sensitive,
			Groups: []uuid.UUID{}, CustomFields: []json.RawMessage{}, Fields: map[string]*FieldDTO{},
			Revs: map[string]int64{}, Participants: []ParticipantDTO{}, ParticipatesIn: []ParticipationDTO{},
			Deleted: row.DeletedAt != nil,
		}
		d.Start = period(row.StartDate, seen(row.StartLink))
		d.End = period(row.EndDate, seen(row.EndLink))
		if l := seen(row.StartLink); l != nil {
			d.refs = append(d.refs, *l)
		}
		if l := seen(row.EndLink); l != nil {
			d.refs = append(d.refs, *l)
		}
		if len(row.CustomFields) > 0 {
			_ = json.Unmarshal(row.CustomFields, &d.CustomFields)
		}
		for _, k := range systemRevKeys {
			d.Revs[k] = 0
		}
		for _, role := range r.T.Roles {
			d.Revs[RevParticipants(role.Key)] = 0
		}
		out[row.ID] = d
	}
	for _, rv := range revs {
		d := out[rv.EntityID]
		if isSystemRev(rv.FieldKey) {
			d.Revs[rv.FieldKey] = rv.Rev
			continue
		}
		d.field(rv.FieldKey).Rev = rv.Rev
	}
	linksOf := map[uuid.UUID][]LinkDTO{}
	for _, l := range links {
		if seen(&l.TargetID) != nil {
			linksOf[l.EntryID] = append(linksOf[l.EntryID], LinkDTO{Target: l.TargetID, Anchor: l.Anchor})
		}
	}
	for _, e := range entries {
		d := out[e.EntityID]
		v := valueDTO(e, linksOf[e.ID], seen(e.VRef) != nil)
		if e.VRef != nil && seen(e.VRef) != nil {
			d.refs = append(d.refs, *e.VRef)
		}
		for _, l := range v.Links {
			d.refs = append(d.refs, l.Target)
		}
		f := d.field(e.FieldKey)
		f.Values = append(f.Values, v)
	}
	for _, p := range parts {
		if seen(&p.EntityID) == nil {
			continue
		}
		d := out[p.ContainerID]
		d.Participants = append(d.Participants, ParticipantDTO{
			ID: p.ID, EntityID: p.EntityID, Role: p.Role, ValidFrom: p.ValidFrom, ValidTo: p.ValidTo, Note: p.Note,
		})
		d.refs = append(d.refs, p.EntityID)
	}
	// участия: по началу контейнера (Д-9), без даты — в конце по времени создания (Д-27)
	slices.SortStableFunc(partsIn, func(a, b sqlc.Participation) int {
		return strings.Compare(startKey(vis[a.ContainerID]), startKey(vis[b.ContainerID]))
	})
	for _, p := range partsIn {
		if seen(&p.ContainerID) == nil {
			continue
		}
		d := out[p.EntityID]
		d.ParticipatesIn = append(d.ParticipatesIn, ParticipationDTO{
			ID: p.ID, ContainerID: p.ContainerID, Role: p.Role, ValidFrom: p.ValidFrom, ValidTo: p.ValidTo, Note: p.Note,
		})
		d.refs = append(d.refs, p.ContainerID)
	}
	for _, g := range groups {
		d := out[g.EntityID]
		d.Groups = append(d.Groups, g.GroupID)
	}
	return out, nil
}

// startKey — ключ сортировки по началу записи; без даты — после датированных.
func startKey(row sqlc.Entity) string {
	if row.StartDate == nil {
		return "~"
	}
	return row.StartDate.SortKey()
}

func (d *RecordDTO) field(key string) *FieldDTO {
	f, ok := d.Fields[key]
	if !ok {
		f = &FieldDTO{Values: []ValueDTO{}}
		d.Fields[key] = f
	}
	return f
}

func period(v *partialdate.Date, link *uuid.UUID) *PeriodDTO {
	if v == nil && link == nil {
		return nil
	}
	return &PeriodDTO{Value: v, Link: link}
}

// valueDTO — значение из строки: тип читается по заполненной колонке (F-12).
func valueDTO(e sqlc.FieldEntry, links []LinkDTO, refVisible bool) ValueDTO {
	v := ValueDTO{ID: e.ID, ValidFrom: e.ValidFrom, ValidTo: e.ValidTo, Note: e.Note, Meta: e.Meta, Links: links}
	if v.Links == nil {
		v.Links = []LinkDTO{}
	}
	if len(v.Meta) == 0 {
		v.Meta = json.RawMessage("{}")
	}
	switch {
	case e.VText != nil:
		v.Value = *e.VText
	case e.VNum.Valid:
		if b, err := e.VNum.MarshalJSON(); err == nil {
			v.Value = json.RawMessage(b)
		}
	case e.VDate != nil:
		v.Value = *e.VDate
	case e.VBool != nil:
		v.Value = *e.VBool
	case e.VRef != nil && refVisible:
		v.Value = *e.VRef
	}
	return v
}

// Related — записи вокруг корней на depth шагов (Д-12): участники,
// контейнеры, цели ref и привязок, start/end-ссылки. Корни в ответ не входят.
func (t *Tx) Related(roots []*RecordDTO, depth int) (map[uuid.UUID]*RecordDTO, error) {
	out := map[uuid.UUID]*RecordDTO{}
	seen := map[uuid.UUID]bool{}
	for _, r := range roots {
		seen[r.ID] = true
	}
	front := roots
	for range depth {
		var next []uuid.UUID
		for _, d := range front {
			for _, id := range d.refs {
				if !seen[id] {
					seen[id] = true
					next = append(next, id)
				}
			}
		}
		if len(next) == 0 {
			break
		}
		rows, err := t.visibleRows(next)
		if err != nil {
			return nil, err
		}
		recs := make([]*Rec, 0, len(rows))
		for _, id := range next {
			if row, ok := rows[id]; ok {
				r, err := t.Record(row)
				if err != nil {
					return nil, err
				}
				recs = append(recs, r)
			}
		}
		m, err := t.Envelopes(recs)
		if err != nil {
			return nil, err
		}
		front = front[:0:0]
		for _, r := range recs {
			out[r.Row.ID] = m[r.Row.ID]
			front = append(front, m[r.Row.ID])
		}
	}
	return out, nil
}

// ByIDs — конверты записей дашборда по id, включая корзину (её видит
// владелец, F-44); очищенные и записи неактивных шаблонов (F-60) пропускаются.
// Нужны журналу: подписи записей, о которых в нём речь.
func (t *Tx) ByIDs(ids []uuid.UUID) (map[uuid.UUID]*RecordDTO, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]*RecordDTO{}, nil
	}
	rows, err := t.q.ListEntitiesByIDs(t.ctx, sqlc.ListEntitiesByIDsParams{DashboardID: t.Dashboard, Ids: ids})
	if err != nil {
		return nil, err
	}
	recs := make([]*Rec, 0, len(rows))
	for _, row := range rows {
		r, err := t.Record(row)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return t.Envelopes(recs)
}
