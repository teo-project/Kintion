package records

import (
	"github.com/google/uuid"

	"kintion/server/internal/audit"
	"kintion/server/internal/db/sqlc"
)

// Корзина (F-44): удаление мягкое, восстановление, окончательная очистка
// владельцем. Поведение ссылок — план §4, «Поведение ссылок на запись».
// Удаление никогда не падает из-за ссылок (F-07).

// Delete переносит запись в корзину. Ссылки на неё остаются, но в чужих
// карточках и related запись не показывается (см. Envelopes).
func (t *Tx) Delete(r *Rec, account uuid.UUID) (*Rec, error) {
	if err := live(r); err != nil {
		return nil, err
	}
	row, err := t.q.SetEntityDeleted(t.ctx, sqlc.SetEntityDeletedParams{ID: r.Row.ID, DeletedBy: &account})
	if err != nil {
		return nil, err
	}
	if err := t.auditEntity(audit.OpDelete, &r.Row, &row); err != nil {
		return nil, err
	}
	return &Rec{Row: row, Tmpl: r.Tmpl}, nil
}

// Restore возвращает запись из корзины.
func (t *Tx) Restore(r *Rec) (*Rec, error) {
	if r.Row.DeletedAt == nil {
		return nil, ErrNotDeleted
	}
	row, err := t.q.RestoreEntity(t.ctx, r.Row.ID)
	if err != nil {
		return nil, err
	}
	if err := t.auditEntity(audit.OpRestore, &r.Row, &row); err != nil {
		return nil, err
	}
	out := &Rec{Row: row, Tmpl: r.Tmpl}
	// правило группы могли включить, пока запись лежала в корзине (F-122)
	return out, t.autoGroups(out)
}

type linkSnap struct {
	ID       uuid.UUID `json:"id"`
	EntryID  uuid.UUID `json:"entry_id"`
	Field    string    `json:"field"`
	TargetID uuid.UUID `json:"target_id"`
	Anchor   string    `json:"anchor"`
}

type labelSnap struct {
	EntityID uuid.UUID `json:"entity_id"`
	GroupID  uuid.UUID `json:"group_id"`
}

// Purge окончательно удаляет запись из корзины одной операцией: каждое
// удалённое или изменённое из-за неё — отдельная запись журнала с общим
// operation_id. Журнал самой записи остаётся (audit_log без FK).
func (t *Tx) Purge(r *Rec) error {
	if r.Row.DeletedAt == nil {
		return ErrNotDeleted
	}
	id := r.Row.ID
	ids := []uuid.UUID{id}

	// свои значения (с привязками)
	entries, err := t.q.ListFieldEntriesByEntities(t.ctx, ids)
	if err != nil {
		return err
	}
	if err := t.purgeEntries(entries); err != nil {
		return err
	}
	// участия: свои участники и участие в чужих контейнерах
	parts, err := t.q.ListParticipationsOf(t.ctx, id)
	if err != nil {
		return err
	}
	for _, p := range parts {
		if err := t.q.DeleteParticipation(t.ctx, p.ID); err != nil {
			return err
		}
		if err := t.auditParticipation(audit.OpPurge, &p, nil); err != nil {
			return err
		}
		if p.ContainerID != id {
			if _, err := t.bumpRev(p.ContainerID, RevParticipants(p.Role)); err != nil {
				return err
			}
		}
	}
	// значения ref других записей на неё (Д-32)
	refs, err := t.q.ListRefEntriesTo(t.ctx, id)
	if err != nil {
		return err
	}
	if err := t.purgeEntries(refs); err != nil {
		return err
	}
	// привязки чужих значений к ней
	links, err := t.q.ListLinksTo(t.ctx, id)
	if err != nil {
		return err
	}
	for _, l := range links {
		if err := t.q.DeleteEntryLink(t.ctx, l.ID); err != nil {
			return err
		}
		if err := t.audit(audit.Entry{Object: "entry_link", ObjectID: &l.ID, Entity: &l.EntityID, Op: audit.OpPurge,
			Before: linkSnap{ID: l.ID, EntryID: l.EntryID, Field: l.FieldKey, TargetID: l.TargetID, Anchor: l.Anchor}}); err != nil {
			return err
		}
		if _, err := t.bumpRev(l.EntityID, l.FieldKey); err != nil {
			return err
		}
	}
	// start/end-ссылки чужих записей: ссылка снимается, дата остаётся
	linking, err := t.q.ListEntitiesLinkingTo(t.ctx, id)
	if err != nil {
		return err
	}
	for _, e := range linking {
		row := e
		var keys []string
		if row.StartLink != nil && *row.StartLink == id {
			row.StartLink = nil
			keys = append(keys, RevStart)
		}
		if row.EndLink != nil && *row.EndLink == id {
			row.EndLink = nil
			keys = append(keys, RevEnd)
		}
		nr, err := t.q.UpdateEntitySystem(t.ctx, sqlc.UpdateEntitySystemParams{
			ID: row.ID, StartDate: row.StartDate, StartLink: row.StartLink, EndDate: row.EndDate, EndLink: row.EndLink,
			Note: row.Note, TitleManual: row.TitleManual, Sensitive: row.Sensitive, CustomFields: row.CustomFields,
		})
		if err != nil {
			return err
		}
		if err := t.auditEntity(audit.OpUpdate, &e, &nr); err != nil {
			return err
		}
		for _, k := range keys {
			if _, err := t.bumpRev(row.ID, k); err != nil {
				return err
			}
		}
	}
	// метки групп
	labels, err := t.q.ListGroupsByEntities(t.ctx, ids)
	if err != nil {
		return err
	}
	for _, l := range labels {
		if err := t.audit(audit.Entry{Object: "entity_group", ObjectID: &l.GroupID, Entity: &id, Op: audit.OpPurge,
			Before: labelSnap{EntityID: l.EntityID, GroupID: l.GroupID}}); err != nil {
			return err
		}
	}
	// сама запись: остальное (метки, версии полей) уходит каскадом
	if err := t.auditEntity(audit.OpPurge, &r.Row, nil); err != nil {
		return err
	}
	return t.q.DeleteEntity(t.ctx, id)
}

// purgeEntries удаляет значения с журналом; у чужих записей — новая версия поля.
func (t *Tx) purgeEntries(entries []sqlc.FieldEntry) error {
	ids := make([]uuid.UUID, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	links, err := t.q.ListEntryLinksByEntries(t.ctx, ids)
	if err != nil {
		return err
	}
	byEntry := map[uuid.UUID][]sqlc.EntryLink{}
	for _, l := range links {
		byEntry[l.EntryID] = append(byEntry[l.EntryID], l)
	}
	for _, e := range entries {
		if err := t.q.DeleteFieldEntry(t.ctx, e.ID); err != nil {
			return err
		}
		s := snapValue(e, byEntry[e.ID])
		if err := t.auditValue(audit.OpPurge, e.EntityID, &s, nil); err != nil {
			return err
		}
		if _, err := t.bumpRev(e.EntityID, e.FieldKey); err != nil {
			return err
		}
	}
	return nil
}
