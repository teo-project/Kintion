package api

import (
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kintion/server/internal/authz"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/httpx"
	"kintion/server/internal/records"
)

func (a *API) groupRoutes() {
	a.mux.HandleFunc("GET /api/dashboards/{d}/records", a.listRecords)
	a.mux.HandleFunc("GET /api/dashboards/{d}/tree", a.tree)
	a.mux.HandleFunc("GET /api/dashboards/{d}/groups", a.listGroups)
	a.mux.HandleFunc("GET /api/dashboards/{d}/groups/recent", a.recentGroups)
	a.mux.HandleFunc("POST /api/dashboards/{d}/groups", a.createGroup)
	a.mux.HandleFunc("PATCH /api/dashboards/{d}/groups/{g}", a.patchGroup)
	a.mux.HandleFunc("DELETE /api/dashboards/{d}/groups/{g}", a.deleteGroup)
	a.mux.HandleFunc("PUT /api/records/{id}/groups", a.setGroups)
	a.mux.HandleFunc("POST /api/dashboards/{d}/groups/{g}/records", a.groupMembers)
}

// inDashboard — дашборд из пути {d} в транзакции; fn возвращает ответ
// (nil — 204).
func (a *API) inDashboard(w http.ResponseWriter, r *http.Request, status int, fn func(t *records.Tx) (any, error)) {
	sess, _ := SessionFrom(r.Context())
	ctx := r.Context()
	var out any
	err := pgx.BeginFunc(ctx, a.d.Pool, func(tx pgx.Tx) error {
		id, err := pathID(r, "d")
		if err != nil {
			return err
		}
		d, err := authz.Dashboard(ctx, sqlc.New(tx), sess.Account.ID, id)
		if err != nil {
			return err
		}
		t, err := a.rec.Begin(ctx, tx, operation(r, sess.Account.ID), d.ID)
		if err != nil {
			return err
		}
		out, err = fn(t)
		return err
	})
	switch {
	case err != nil:
		a.recordError(w, r, err)
	case out == nil:
		w.WriteHeader(http.StatusNoContent)
	default:
		httpx.WriteJSON(w, status, out)
	}
}

// recentGroupsLimit — сколько последних групп предлагать (F-81).
const recentGroupsLimit = 4

// recentGroups — GET /api/dashboards/{d}/groups/recent: id последних
// уникальных групп, назначенных этим пользователем (F-81), новые первыми.
func (a *API) recentGroups(w http.ResponseWriter, r *http.Request) {
	sess, _ := SessionFrom(r.Context())
	a.inDashboard(w, r, http.StatusOK, func(t *records.Tx) (any, error) {
		ids, err := t.RecentGroups(sess.Account.ID, recentGroupsLimit)
		if ids == nil {
			ids = []uuid.UUID{}
		}
		return ids, err
	})
}

// ListOut — страница списка записей.
type ListOut struct {
	Items   []*records.RecordDTO             `json:"items"`
	Total   int64                            `json:"total"`
	Related map[uuid.UUID]*records.RecordDTO `json:"related"`
}

// listRecords — GET /api/dashboards/{d}/records?entity=&group=&q=&deleted=1&limit=&offset=
func (a *API) listRecords(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	lq := records.ListQuery{Entity: qs.Get("entity"), Q: qs.Get("q"), Deleted: qs.Get("deleted") == "1"}
	lq.Limit, _ = strconv.Atoi(qs.Get("limit"))
	lq.Offset, _ = strconv.Atoi(qs.Get("offset"))
	// не-UTF-8 строка до PostgreSQL не доходит: там это ошибка кодировки (500)
	for field, v := range map[string]string{"q": lq.Q, "entity": lq.Entity} {
		if !utf8.ValidString(v) {
			httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: field, Message: "строка не в UTF-8"})
			return
		}
	}
	var ok bool
	if lq.Group, ok = groupParam(w, r); !ok {
		return
	}
	a.inDashboard(w, r, http.StatusOK, func(t *records.Tx) (any, error) {
		recs, total, err := t.List(lq)
		if err != nil {
			return nil, err
		}
		envs, err := t.Envelopes(recs)
		if err != nil {
			return nil, err
		}
		out := &ListOut{Items: make([]*records.RecordDTO, 0, len(recs)), Total: total}
		for _, rec := range recs {
			out.Items = append(out.Items, envs[rec.Row.ID])
		}
		out.Related, err = t.Related(out.Items, relatedDepth)
		return out, err
	})
}

// groupParam — необязательный фильтр ?group=<id>; неверный id — 400.
func groupParam(w http.ResponseWriter, r *http.Request) (*uuid.UUID, bool) {
	g := r.URL.Query().Get(records.KeyGroup)
	if g == "" {
		return nil, true
	}
	id, err := uuid.Parse(g)
	if err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: records.KeyGroup, Message: "неверный id группы"})
		return nil, false
	}
	return &id, true
}

// tree — GET /api/dashboards/{d}/tree[?group=]: отношения с tree_link и их
// видимые участники (F-110, F-111).
func (a *API) tree(w http.ResponseWriter, r *http.Request) {
	group, ok := groupParam(w, r)
	if !ok {
		return
	}
	a.inDashboard(w, r, http.StatusOK, func(t *records.Tx) (any, error) {
		return t.Tree(group)
	})
}

func (a *API) listGroups(w http.ResponseWriter, r *http.Request) {
	a.inDashboard(w, r, http.StatusOK, func(t *records.Tx) (any, error) { return t.Groups() })
}

func (a *API) createGroup(w http.ResponseWriter, r *http.Request) {
	var in records.NewGroup
	if !decode(w, r, &in) {
		return
	}
	a.inDashboard(w, r, http.StatusCreated, func(t *records.Tx) (any, error) {
		g, err := t.CreateGroup(in)
		if err != nil {
			return nil, err
		}
		return t.GroupOut(g)
	})
}

func (a *API) patchGroup(w http.ResponseWriter, r *http.Request) {
	var in records.PatchGroup
	if !decode(w, r, &in) {
		return
	}
	a.inDashboard(w, r, http.StatusOK, func(t *records.Tx) (any, error) {
		id, err := pathID(r, "g")
		if err != nil {
			return nil, err
		}
		old, err := t.Group(id)
		if err != nil {
			return nil, err
		}
		g, err := t.PatchGroup(old, in)
		if err != nil {
			return nil, err
		}
		return t.GroupOut(g)
	})
}

func (a *API) deleteGroup(w http.ResponseWriter, r *http.Request) {
	a.inDashboard(w, r, http.StatusOK, func(t *records.Tx) (any, error) {
		id, err := pathID(r, "g")
		if err != nil {
			return nil, err
		}
		g, err := t.Group(id)
		if err != nil {
			return nil, err
		}
		return nil, t.DeleteGroup(g)
	})
}

func (a *API) setGroups(w http.ResponseWriter, r *http.Request) {
	var in records.SetGroups
	if !decode(w, r, &in) {
		return
	}
	a.inRecord(w, r, func(t *records.Tx, rec *records.Rec) (*records.Rec, error) { return t.SetGroups(rec, in) })
}

// groupMembers — POST /api/dashboards/{d}/groups/{g}/records: массово
// поставить и снять метку группы (F-131); ответ — сколько меток изменилось.
func (a *API) groupMembers(w http.ResponseWriter, r *http.Request) {
	var in records.GroupMembers
	if !decode(w, r, &in) {
		return
	}
	a.inDashboard(w, r, http.StatusOK, func(t *records.Tx) (any, error) {
		id, err := pathID(r, "g")
		if err != nil {
			return nil, err
		}
		return t.SetGroupMembers(id, in)
	})
}
