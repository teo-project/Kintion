package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"kintion/server/internal/audit"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/httpx"
	"kintion/server/internal/records"
)

// Страница журнала: число операций по умолчанию и наибольшее.
const (
	auditLimit = 50
	// auditActorSystem — фильтр ?actor=system: изменения системы (без автора).
	auditActorSystem = "system"
	auditMaxLimit    = 200
)

func (a *API) auditRoutes() {
	a.mux.HandleFunc("GET /api/dashboards/{d}/audit", a.listAudit)
}

// AuditEntryDTO — одно изменение объекта (F-16): было → стало.
type AuditEntryDTO struct {
	ID       uuid.UUID       `json:"id"`
	At       time.Time       `json:"at"`
	EntityID *uuid.UUID      `json:"entity_id"`
	Object   string          `json:"object"`
	ObjectID *uuid.UUID      `json:"object_id"`
	Op       string          `json:"op"`
	Before   json.RawMessage `json:"before"`
	After    json.RawMessage `json:"after"`
}

// AuditOperationDTO — операция: все её записи журнала с общим operation_id.
type AuditOperationDTO struct {
	OperationID   uuid.UUID       `json:"operation_id"`
	At            time.Time       `json:"at"`
	ActorID       *uuid.UUID      `json:"actor_id"`
	ActorLogin    *string         `json:"actor_login"`
	Source        string          `json:"source"`
	RequestID     string          `json:"request_id"`
	AppVersion    string          `json:"app_version"`
	SchemaVersion int64           `json:"schema_version"`
	Entries       []AuditEntryDTO `json:"entries"`
}

// AuditOut — страница журнала, новые операции сверху. Next — курсор
// следующей страницы (?before=), null — страниц больше нет.
type AuditOut struct {
	Operations []*AuditOperationDTO `json:"operations"`
	Next       *string              `json:"next"`
	// Records — записи, о которых идёт речь на странице (и в корзине тоже):
	// подписи и шаблоны для показа «было → стало»; очищенных здесь нет.
	Records map[uuid.UUID]*records.RecordDTO `json:"records"`
	// Related — их связанные записи на шаг (подписи отношений и событий).
	Related map[uuid.UUID]*records.RecordDTO `json:"related"`
	// Purged — окончательно удалённые записи страницы, восстановленные по
	// журналу: только для их последнего названия (F-94).
	Purged map[uuid.UUID]*records.RecordDTO `json:"purged"`
}

func rawJSON(b []byte) json.RawMessage {
	if b == nil {
		return json.RawMessage("null")
	}
	return b
}

// auditCursor — «время_операция» последней операции страницы.
func auditCursor(at time.Time, op uuid.UUID) string {
	return at.UTC().Format(time.RFC3339Nano) + "_" + op.String()
}

func parseAuditCursor(s string) (time.Time, uuid.UUID, bool) {
	ts, id, ok := strings.Cut(s, "_")
	if !ok {
		return time.Time{}, uuid.Nil, false
	}
	at, err1 := time.Parse(time.RFC3339Nano, ts)
	op, err2 := uuid.Parse(id)
	return at, op, err1 == nil && err2 == nil
}

// optUUID — необязательный фильтр-uuid из строки запроса.
func optUUID(w http.ResponseWriter, qs map[string][]string, name string) (*uuid.UUID, bool) {
	v := ""
	if vs := qs[name]; len(vs) > 0 {
		v = vs[0]
	}
	if v == "" {
		return nil, true
	}
	id, err := uuid.Parse(v)
	if err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: name, Message: "неверный id"})
		return nil, false
	}
	return &id, true
}

// listAudit — GET /api/dashboards/{d}/audit?entity=&operation=&actor=&op=&before=&limit=
// Журнал дашборда сгруппирован по операциям; entity — история записи: её
// изменения и её участия в других записях; actor — автор (uuid или system —
// изменения системы); op — действие (create, update, …). В 0.1 читает
// владелец (F-08).
func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	entity, ok := optUUID(w, qs, "entity")
	if !ok {
		return
	}
	opFilter, ok := optUUID(w, qs, "operation")
	if !ok {
		return
	}
	systemOnly := qs.Get("actor") == auditActorSystem
	var actor *uuid.UUID
	if !systemOnly {
		if actor, ok = optUUID(w, qs, "actor"); !ok {
			return
		}
	}
	var opKind *string
	if s := qs.Get("op"); s != "" {
		if !slices.Contains(audit.Ops, s) {
			httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: "op", Message: "неизвестное действие"})
			return
		}
		opKind = &s
	}
	limit := auditLimit
	if s := qs.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: "limit", Message: "ожидается целое > 0"})
			return
		}
		limit = min(n, auditMaxLimit)
	}
	var beforeAt *time.Time
	var beforeOp uuid.UUID
	if s := qs.Get("before"); s != "" {
		at, op, ok := parseAuditCursor(s)
		if !ok {
			httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: "before", Message: "неверный курсор"})
			return
		}
		beforeAt, beforeOp = &at, op
	}

	d, err := a.dashboardOf(r)
	if err != nil {
		a.recordError(w, r, err)
		return
	}
	ctx := r.Context()
	q := sqlc.New(a.d.Pool)
	ops, err := q.ListAuditOperations(ctx, sqlc.ListAuditOperationsParams{
		DashboardID: d.ID, Entity: entity, Operation: opFilter, Actor: actor, SystemOnly: systemOnly, Op: opKind,
		BeforeAt: beforeAt, BeforeOp: beforeOp, Lim: int32(limit + 1), //nolint:gosec // ≤ auditMaxLimit+1
	})
	if err != nil {
		a.internal(w, r, err)
		return
	}
	out := &AuditOut{Operations: []*AuditOperationDTO{}, Records: map[uuid.UUID]*records.RecordDTO{},
		Related: map[uuid.UUID]*records.RecordDTO{}, Purged: map[uuid.UUID]*records.RecordDTO{}}
	if len(ops) > limit {
		ops = ops[:limit]
		next := auditCursor(ops[limit-1].LastAt, ops[limit-1].OperationID)
		out.Next = &next
	}
	if len(ops) == 0 {
		httpx.WriteJSON(w, http.StatusOK, out)
		return
	}
	ids := make([]uuid.UUID, len(ops))
	byID := make(map[uuid.UUID]*AuditOperationDTO, len(ops))
	for i, o := range ops {
		ids[i] = o.OperationID
		dto := &AuditOperationDTO{OperationID: o.OperationID, At: o.LastAt, Entries: []AuditEntryDTO{}}
		byID[o.OperationID] = dto
		out.Operations = append(out.Operations, dto)
	}
	rows, err := q.ListAuditEntries(ctx, sqlc.ListAuditEntriesParams{
		DashboardID: d.ID, Operations: ids, Entity: entity, Actor: actor, SystemOnly: systemOnly, Op: opKind,
	})
	if err != nil {
		a.internal(w, r, err)
		return
	}
	for _, e := range rows {
		o := byID[e.OperationID]
		if len(o.Entries) == 0 {
			o.ActorID, o.ActorLogin, o.Source, o.RequestID = e.ActorID, e.ActorLogin, e.Source, e.RequestID
			o.AppVersion, o.SchemaVersion = e.AppVersion, e.SchemaVersion
		}
		o.Entries = append(o.Entries, AuditEntryDTO{ID: e.ID, At: e.At, EntityID: e.EntityID, Object: e.Object,
			ObjectID: e.ObjectID, Op: e.Op, Before: rawJSON(e.Before), After: rawJSON(e.After)})
	}
	if err := a.auditRecords(r, d.ID, q, out); err != nil {
		a.internal(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// auditRefs — записи, упомянутые в записи журнала: сама запись истории и
// участник участия (он не совпадает с контейнером).
type auditRefs struct {
	EntityID    *uuid.UUID `json:"entity_id"`
	ContainerID *uuid.UUID `json:"container_id"`
}

// auditRecords добавляет в страницу конверты упомянутых записей и их related.
func (a *API) auditRecords(r *http.Request, dashboard uuid.UUID, q *sqlc.Queries, out *AuditOut) error {
	seen := map[uuid.UUID]bool{}
	var ids []uuid.UUID
	add := func(id *uuid.UUID) {
		if id != nil && !seen[*id] {
			seen[*id] = true
			ids = append(ids, *id)
		}
	}
	for _, o := range out.Operations {
		for _, e := range o.Entries {
			add(e.EntityID)
			for _, raw := range []json.RawMessage{e.Before, e.After} {
				var refs auditRefs
				if e.Object == "participation" && json.Unmarshal(raw, &refs) == nil {
					add(refs.EntityID)
					add(refs.ContainerID)
				}
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	t, err := a.rec.Read(r.Context(), q, dashboard)
	if err != nil {
		return err
	}
	recs, err := t.ByIDs(ids)
	if err != nil {
		return err
	}
	var gone []uuid.UUID
	for _, id := range ids {
		if _, ok := recs[id]; !ok {
			gone = append(gone, id)
		}
	}
	if out.Purged, err = t.Purged(gone); err != nil {
		return err
	}
	roots := make([]*records.RecordDTO, 0, len(recs))
	for id, d := range recs {
		out.Records[id] = d
		roots = append(roots, d)
	}
	rel, err := t.Related(roots, 1)
	if err != nil {
		return err
	}
	out.Related = rel
	return nil
}
