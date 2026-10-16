package records

import (
	"errors"
	"slices"

	"github.com/google/uuid"

	"kintion/server/internal/db/sqlc"
)

// TreeOut — данные ракурса «Дерево» (F-110, F-111): отношения шаблонов с
// tree_link и их участники. Только видимое (F-07): участники в корзине и на
// неактивных шаблонах в конверты не попадают; раскладку строит интерфейс.
type TreeOut struct {
	Relations []*RecordDTO `json:"relations"`
	Records   []*RecordDTO `json:"records"`
}

// Tree — отношения с tree_link и их видимые участники; group — только
// участники из группы (с подгруппами), отношения — где таких двое и больше
// разных, записи — только участники оставшихся отношений.
// Запросов — постоянное число, без N+1.
func (t *Tx) Tree(group *uuid.UUID) (*TreeOut, error) {
	if group != nil {
		if _, err := t.Group(*group); err != nil {
			return nil, invalid(KeyGroup, "нет такой группы")
		}
	}
	var tmplIDs []uuid.UUID
	for id, tm := range t.tmpls {
		if tm != nil && tm.Entity.Container && tm.T.InTree() {
			tmplIDs = append(tmplIDs, id)
		}
	}
	out := &TreeOut{Relations: []*RecordDTO{}, Records: []*RecordDTO{}}
	if len(tmplIDs) == 0 {
		return out, nil
	}
	slices.SortFunc(tmplIDs, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	rows, err := t.q.ListLiveEntitiesByTemplates(t.ctx, sqlc.ListLiveEntitiesByTemplatesParams{DashboardID: t.Dashboard, TemplateIds: tmplIDs})
	if err != nil {
		return nil, err
	}
	rels, err := t.recs(rows)
	if err != nil {
		return nil, err
	}
	relEnv, err := t.Envelopes(rels)
	if err != nil {
		return nil, err
	}
	// участники: в конвертах — только видимые
	seen := map[uuid.UUID]bool{}
	var ids []uuid.UUID
	for _, r := range rels {
		for _, p := range relEnv[r.Row.ID].Participants {
			if !seen[p.EntityID] {
				seen[p.EntityID] = true
				ids = append(ids, p.EntityID)
			}
		}
	}
	if group != nil && len(ids) > 0 {
		ids, err = t.q.FilterEntitiesByGroup(t.ctx, sqlc.FilterEntitiesByGroupParams{DashboardID: t.Dashboard, GroupID: *group, Ids: ids})
		if err != nil {
			return nil, err
		}
	}
	vis, err := t.visibleRows(ids)
	if err != nil {
		return nil, err
	}
	keep := map[uuid.UUID]bool{}
	for _, id := range ids {
		if _, ok := vis[id]; ok {
			keep[id] = true
		}
	}
	// отношения — с двумя и более разными оставшимися участниками; участники
	// вне фильтра из конверта убираются; записи — только участники оставшихся
	used := map[uuid.UUID]bool{}
	for _, r := range rels {
		d := relEnv[r.Row.ID]
		parts := d.Participants[:0:0]
		distinct := map[uuid.UUID]bool{}
		for _, p := range d.Participants {
			if keep[p.EntityID] {
				parts = append(parts, p)
				distinct[p.EntityID] = true
			}
		}
		if len(distinct) < 2 {
			continue
		}
		d.Participants = parts
		out.Relations = append(out.Relations, d)
		for id := range distinct {
			used[id] = true
		}
	}
	people := make([]sqlc.Entity, 0, len(used))
	for _, id := range ids {
		if used[id] {
			people = append(people, vis[id])
		}
	}
	recs, err := t.recs(people)
	if err != nil {
		return nil, err
	}
	env, err := t.Envelopes(recs)
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		out.Records = append(out.Records, env[r.Row.ID])
	}
	return out, nil
}

// recs — записи по строкам; строки неактивных шаблонов пропускаются (F-60).
func (t *Tx) recs(rows []sqlc.Entity) ([]*Rec, error) {
	out := make([]*Rec, 0, len(rows))
	for _, row := range rows {
		r, err := t.Record(row)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
