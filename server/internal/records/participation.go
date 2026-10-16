package records

import (
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kintion/server/internal/audit"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/packs"
	"kintion/server/internal/partialdate"
)

// Участия (F-10): контейнер ← участник в роли шаблона, с периодом и пометкой.
// Повтор пары допустим (вышел и вернулся); циклы вложенности — нет.

// NewParticipant — участник в пакетной операции: контейнер и участник —
// id записи или временный id пакета.
type NewParticipant struct {
	Container string            `json:"container"`
	Entity    string            `json:"entity"`
	Role      string            `json:"role"`
	ValidFrom *partialdate.Date `json:"valid_from"`
	ValidTo   *partialdate.Date `json:"valid_to"`
	Note      *string           `json:"note"`
}

// AddParticipant — POST /api/records/{id}/participants (по умолчанию — в конец роли).
type AddParticipant struct {
	IfRev     *int64            `json:"if_rev"`
	Entity    uuid.UUID         `json:"entity"`
	Role      string            `json:"role"`
	ValidFrom *partialdate.Date `json:"valid_from"`
	ValidTo   *partialdate.Date `json:"valid_to"`
	Note      *string           `json:"note"`
	Position
}

// PatchParticipant — PATCH /api/records/{id}/participants/{pid}.
type PatchParticipant struct {
	IfRev     *int64                `json:"if_rev"`
	ValidFrom Opt[partialdate.Date] `json:"valid_from"`
	ValidTo   Opt[partialdate.Date] `json:"valid_to"`
	Note      Opt[string]           `json:"note"`
	Position
}

type participationSnap struct {
	ID          uuid.UUID         `json:"id"`
	ContainerID uuid.UUID         `json:"container_id"`
	EntityID    uuid.UUID         `json:"entity_id"`
	Role        string            `json:"role"`
	ValidFrom   *partialdate.Date `json:"valid_from"`
	ValidTo     *partialdate.Date `json:"valid_to"`
	Note        *string           `json:"note"`
	SortKey     string            `json:"sort_key"`
}

func snapParticipation(p sqlc.Participation) participationSnap {
	return participationSnap{ID: p.ID, ContainerID: p.ContainerID, EntityID: p.EntityID, Role: p.Role,
		ValidFrom: p.ValidFrom, ValidTo: p.ValidTo, Note: p.Note, SortKey: p.SortKey}
}

// auditParticipation — в истории контейнера.
func (t *Tx) auditParticipation(op string, before, after *sqlc.Participation) error {
	e := audit.Entry{Object: "participation", Op: op}
	if before != nil {
		s := snapParticipation(*before)
		e.Before, e.ObjectID, e.Entity = s, &before.ID, &before.ContainerID
	}
	if after != nil {
		s := snapParticipation(*after)
		e.After, e.ObjectID, e.Entity = s, &after.ID, &after.ContainerID
	}
	return t.audit(e)
}

// checkParticipant — роль есть в шаблоне контейнера, контейнер может содержать,
// участник — видимая запись допустимой сущности, вложенность без циклов.
func (t *Tx) checkParticipant(c *Rec, member sqlc.Entity, role, field string) (*packs.Role, error) {
	if !c.Entity.Container {
		return nil, invalid(field, "записи сущности %q не содержат участников", c.Entity.ID)
	}
	ro, ok := c.T.Role(role)
	if !ok {
		return nil, invalid(field, "у шаблона нет роли %q", role)
	}
	mt := t.tmpls[member.TemplateID]
	if mt == nil || !mt.Entity.Participant {
		return nil, invalid(field, "запись не может быть участником")
	}
	// вложенность — через поле-ссылку, не через участие (F-82)
	if mt.Entity.Container {
		return nil, invalid(field, "запись-контейнер не участвует в другой: вложенность задаётся полем-ссылкой")
	}
	if len(ro.Entities) > 0 && !slices.Contains(ro.Entities, member.Entity) {
		return nil, invalid(field, "в роли %q не может быть записи сущности %q", role, member.Entity)
	}
	if member.ID == c.Row.ID {
		return nil, invalid(field, "запись не может участвовать в себе")
	}
	cycle, err := t.q.ContainsEntity(t.ctx, sqlc.ContainsEntityParams{Root: member.ID, Target: c.Row.ID})
	if err != nil {
		return nil, err
	}
	if cycle {
		return nil, invalid(field, "цикл вложенности: запись уже содержит контейнер")
	}
	return ro, nil
}

// counts — число видимых участников контейнера: по ролям и всего (Д-22, F-07).
func (t *Tx) counts(container uuid.UUID) (map[string]int, int, error) {
	parts, err := t.q.ListParticipantsByContainers(t.ctx, []uuid.UUID{container})
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, 0, len(parts))
	for _, p := range parts {
		ids = append(ids, p.EntityID)
	}
	vis, err := t.visibleRows(ids)
	if err != nil {
		return nil, 0, err
	}
	byRole, total := map[string]int{}, 0
	for _, p := range parts {
		if _, ok := vis[p.EntityID]; ok {
			byRole[p.Role]++
			total++
		}
	}
	return byRole, total, nil
}

// checkMax — после добавления не больше max в роли и в шаблоне.
func (t *Tx) checkMax(c *Rec, field string) error {
	byRole, total, err := t.counts(c.Row.ID)
	if err != nil {
		return err
	}
	for _, ro := range c.T.Roles {
		if ro.Max != nil && byRole[ro.Key] > *ro.Max {
			return invalid(field, "в роли %q не больше %d", ro.Key, *ro.Max)
		}
	}
	if p := c.T.Participants; p != nil && p.Max != nil && total > *p.Max {
		return invalid(field, "участников не больше %d", *p.Max)
	}
	return nil
}

// checkMin — участников не меньше min в каждой роли и в шаблоне.
func (t *Tx) checkMin(c *Rec, field string) error {
	byRole, total, err := t.counts(c.Row.ID)
	if err != nil {
		return err
	}
	for _, ro := range c.T.Roles {
		if ro.Min != nil && byRole[ro.Key] < *ro.Min {
			return invalid(field, "в роли %q нужно не меньше %d", ro.Key, *ro.Min)
		}
	}
	if p := c.T.Participants; p != nil && p.Min != nil && total < *p.Min {
		return invalid(field, "нужно не меньше %d участников", *p.Min)
	}
	return nil
}

func (t *Tx) insertParticipation(c *Rec, member uuid.UUID, role string, from, to *partialdate.Date, note *string, pos Position, field string) (sqlc.Participation, error) {
	list, err := t.q.ListParticipantsByContainerRole(t.ctx, sqlc.ListParticipantsByContainerRoleParams{ContainerID: c.Row.ID, Role: role})
	if err != nil {
		return sqlc.Participation{}, err
	}
	if !pos.set() {
		pos.Position = posBottom
	}
	sk, err := participantKey(list, pos, field)
	if err != nil {
		return sqlc.Participation{}, err
	}
	p, err := t.q.InsertParticipation(t.ctx, sqlc.InsertParticipationParams{
		ID: newID(), DashboardID: t.Dashboard, ContainerID: c.Row.ID, EntityID: member, Role: role,
		ValidFrom: from, ValidTo: to, Note: normText(note), SortKey: sk,
	})
	if err != nil {
		return p, err
	}
	return p, t.auditParticipation(audit.OpCreate, nil, &p)
}

func participantKey(list []sqlc.Participation, pos Position, field string) (string, error) {
	keys := make([]string, len(list))
	ids := make([]uuid.UUID, len(list))
	for i, p := range list {
		keys[i], ids[i] = p.SortKey, p.ID
	}
	return between(keys, ids, pos, field)
}

// participantsCurrent — участники роли (ответ 409).
func (t *Tx) participantsCurrent(r *Rec, role string) func() (any, error) {
	return func() (any, error) {
		d, err := t.Envelope(r)
		if err != nil {
			return nil, err
		}
		list := []ParticipantDTO{}
		for _, p := range d.Participants {
			if p.Role == role {
				list = append(list, p)
			}
		}
		return struct {
			Rev    int64            `json:"rev"`
			Values []ParticipantDTO `json:"values"`
		}{d.Revs[RevParticipants(role)], list}, nil
	}
}

// AddParticipant добавляет участника в контейнер r.
func (t *Tx) AddParticipant(r *Rec, in AddParticipant) (*Rec, error) {
	if err := live(r); err != nil {
		return nil, err
	}
	if _, ok := r.T.Role(in.Role); !ok {
		return nil, invalid("role", "у шаблона нет роли %q", in.Role)
	}
	key := RevParticipants(in.Role)
	if err := t.lockRev(r.Row.ID, key, in.IfRev, t.participantsCurrent(r, in.Role)); err != nil {
		return nil, err
	}
	member, err := t.target(in.Entity, "entity")
	if err != nil {
		return nil, err
	}
	if _, err := t.checkParticipant(r, member, in.Role, "entity"); err != nil {
		return nil, err
	}
	if _, err := t.insertParticipation(r, member.ID, in.Role, in.ValidFrom, in.ValidTo, in.Note, in.Position, "entity"); err != nil {
		return nil, err
	}
	if err := t.checkMax(r, "role"); err != nil {
		return nil, err
	}
	return t.bumped(r, key)
}

func (t *Tx) participation(r *Rec, pid uuid.UUID) (sqlc.Participation, error) {
	p, err := t.q.GetParticipation(t.ctx, sqlc.GetParticipationParams{DashboardID: t.Dashboard, ID: pid})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.ContainerID != r.Row.ID) {
		return p, ErrNotFound
	}
	return p, err
}

// PatchParticipant меняет период, пометку или место участника в роли.
func (t *Tx) PatchParticipant(r *Rec, pid uuid.UUID, in PatchParticipant) (*Rec, error) {
	if err := live(r); err != nil {
		return nil, err
	}
	old, err := t.participation(r, pid)
	if err != nil {
		return nil, err
	}
	key := RevParticipants(old.Role)
	if err := t.lockRev(r.Row.ID, key, in.IfRev, t.participantsCurrent(r, old.Role)); err != nil {
		return nil, err
	}
	p := old
	if in.ValidFrom.Set {
		p.ValidFrom = in.ValidFrom.V
	}
	if in.ValidTo.Set {
		p.ValidTo = in.ValidTo.V
	}
	if in.Note.Set {
		p.Note = normText(in.Note.V)
	}
	if in.set() {
		list, err := t.q.ListParticipantsByContainerRole(t.ctx, sqlc.ListParticipantsByContainerRoleParams{ContainerID: r.Row.ID, Role: old.Role})
		if err != nil {
			return nil, err
		}
		list = slices.DeleteFunc(list, func(x sqlc.Participation) bool { return x.ID == old.ID })
		if p.SortKey, err = participantKey(list, in.Position, "position"); err != nil {
			return nil, err
		}
	}
	np, err := t.q.UpdateParticipation(t.ctx, sqlc.UpdateParticipationParams{
		ID: p.ID, ValidFrom: p.ValidFrom, ValidTo: p.ValidTo, Note: p.Note, SortKey: p.SortKey,
	})
	if err != nil {
		return nil, err
	}
	if err := t.auditParticipation(audit.OpUpdate, &old, &np); err != nil {
		return nil, err
	}
	return t.bumped(r, key)
}

// DeleteParticipant убирает участника; ниже min — отказ (Д-22).
func (t *Tx) DeleteParticipant(r *Rec, pid uuid.UUID, ifRev *int64) (*Rec, error) {
	if err := live(r); err != nil {
		return nil, err
	}
	old, err := t.participation(r, pid)
	if err != nil {
		return nil, err
	}
	key := RevParticipants(old.Role)
	if err := t.lockRev(r.Row.ID, key, ifRev, t.participantsCurrent(r, old.Role)); err != nil {
		return nil, err
	}
	if err := t.q.DeleteParticipation(t.ctx, old.ID); err != nil {
		return nil, err
	}
	if err := t.checkMin(r, "role"); err != nil {
		return nil, err
	}
	if err := t.auditParticipation(audit.OpDelete, &old, nil); err != nil {
		return nil, err
	}
	return t.bumped(r, key)
}

// createParticipants — участники пакета; затем min/max у всех новых записей
// (Свадьба без двух участников не создаётся — Д-22).
func (t *Tx) createParticipants(b Batch, res *BatchResult, refs *batchRefs) error {
	inBatch := map[uuid.UUID]*Rec{}
	for _, r := range res.Records {
		inBatch[r.Row.ID] = r
	}
	touched := map[uuid.UUID]*Rec{}
	for i, n := range b.Participants {
		field := fmt.Sprintf("participants[%d]", i)
		cid, err := refs.resolve(n.Container, field+".container")
		if err != nil {
			return err
		}
		c, ok := inBatch[cid]
		if !ok {
			row, err := t.target(cid, field+".container")
			if err != nil {
				return err
			}
			if c, err = t.Record(row); err != nil {
				return err
			}
		}
		mid, err := refs.resolve(n.Entity, field+".entity")
		if err != nil {
			return err
		}
		member, err := t.target(mid, field+".entity")
		if err != nil {
			return err
		}
		if _, err := t.checkParticipant(c, member, n.Role, field+".role"); err != nil {
			return err
		}
		if _, err := t.insertParticipation(c, mid, n.Role, n.ValidFrom, n.ValidTo, n.Note, Position{}, field); err != nil {
			return err
		}
		if !ok {
			if _, err := t.bumpRev(cid, RevParticipants(n.Role)); err != nil {
				return err
			}
		}
		touched[cid] = c
	}
	for _, c := range touched {
		if err := t.checkMax(c, "participants"); err != nil {
			return err
		}
	}
	for i, r := range res.Records {
		if err := t.checkMin(r, fmt.Sprintf("records[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}
