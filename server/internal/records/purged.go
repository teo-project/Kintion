package records

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"

	"kintion/server/internal/audit"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/packs"
)

// Purged восстанавливает по журналу последнее состояние окончательно
// удалённых записей ids (F-94): только для названия в журнале — сама запись,
// её значения и связи из базы уже удалены. Читаются только записи журнала
// очистки: в них — состояние каждой строки на момент удаления. Запись
// неактивного шаблона (F-60) не восстанавливается: показываются сырые ключи.
// Ссылки на другие записи пусты (цель могла стать невидимой, F-07).
func (t *Tx) Purged(ids []uuid.UUID) (map[uuid.UUID]*RecordDTO, error) {
	out := map[uuid.UUID]*RecordDTO{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := t.q.ListAuditSnapshots(t.ctx, sqlc.ListAuditSnapshotsParams{DashboardID: &t.Dashboard, Ids: ids})
	if err != nil {
		return nil, err
	}
	ents := map[uuid.UUID]*entitySnap{}
	vals := map[uuid.UUID]map[uuid.UUID]valueSnap{} // запись → значение
	for _, r := range rows {
		if r.EntityID == nil {
			continue
		}
		raw := r.After
		if len(raw) == 0 || r.Op == audit.OpPurge {
			raw = r.Before
		}
		switch r.Object {
		case "entity":
			var s entitySnap
			if json.Unmarshal(raw, &s) == nil && s.ID == *r.EntityID {
				ents[s.ID] = &s
			}
		case "field_entry":
			var v valueSnap
			if json.Unmarshal(raw, &v) != nil {
				continue
			}
			m := vals[*r.EntityID]
			if m == nil {
				m = map[uuid.UUID]valueSnap{}
				vals[*r.EntityID] = m
			}
			if r.Op == audit.OpDelete && len(r.After) == 0 {
				delete(m, v.ID)
			} else {
				m[v.ID] = v
			}
		}
	}
	for id, s := range ents {
		tm := t.tmpls[s.TemplateID]
		if tm == nil {
			continue
		}
		d := &RecordDTO{
			ID: id, Entity: s.Entity, Template: tm.T.ID, TemplateID: s.TemplateID,
			Start: period(s.StartDate, nil), End: period(s.EndDate, nil),
			TitleManual: s.TitleManual, Groups: []uuid.UUID{}, CustomFields: []json.RawMessage{},
			Fields: map[string]*FieldDTO{}, Revs: map[string]int64{}, Participants: []ParticipantDTO{},
			ParticipatesIn: []ParticipationDTO{}, Deleted: true,
		}
		if len(s.CustomFields) > 0 {
			_ = json.Unmarshal(s.CustomFields, &d.CustomFields)
		}
		custom, _ := customDefs(s.CustomFields)
		// тип поля — по шаблону, своим полям записи и общим полям шаблона
		isRef := func(key string) bool {
			var f *packs.Field
			switch {
			case strings.HasPrefix(key, packs.CustomPrefix):
				f = findField(custom, strings.TrimPrefix(key, packs.CustomPrefix))
			case strings.HasPrefix(key, packs.SharedPrefix):
				f = findField(tm.Shared, strings.TrimPrefix(key, packs.SharedPrefix))
			default:
				f, _ = tm.T.Field(key)
			}
			return f != nil && f.Type == packs.TypeRef
		}
		list := make([]valueSnap, 0, len(vals[id]))
		for _, v := range vals[id] {
			list = append(list, v)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].SortKey < list[j].SortKey })
		for _, v := range list {
			f := d.Fields[v.Field]
			if f == nil {
				f = &FieldDTO{Values: []ValueDTO{}}
				d.Fields[v.Field] = f
			}
			val := v.ValueDTO
			val.Links = []LinkDTO{}
			if isRef(v.Field) {
				val.Value = nil
			}
			f.Values = append(f.Values, val)
		}
		out[id] = d
	}
	return out, nil
}

func findField(list []packs.Field, key string) *packs.Field {
	for i := range list {
		if list[i].Key == key {
			return &list[i]
		}
	}
	return nil
}
