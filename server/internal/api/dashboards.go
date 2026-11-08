package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kintion/i18n"
	"kintion/server/internal/audit"
	"kintion/server/internal/auth"
	"kintion/server/internal/authz"
	"kintion/server/internal/dashboards"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/httpx"
	"kintion/server/internal/records"
)

func (a *API) dashboardRoutes() {
	a.mux.HandleFunc("GET /api/dashboards", a.listDashboards)
	a.mux.HandleFunc("POST /api/dashboards", a.createDashboard)
	a.mux.HandleFunc("PATCH /api/dashboards/{d}", a.renameDashboard)
	a.mux.HandleFunc("GET /api/dashboards/{d}/schema", a.dashboardSchema)
	a.mux.HandleFunc("PUT /api/dashboards/{d}/templates/{t}/custom_fields", a.putSharedFields)
	a.mux.HandleFunc("PUT /api/dashboards/{d}/templates/{t}/quick_fields", a.putQuickFields)
	a.mux.HandleFunc("GET /api/dashboards/{d}/eras", a.getEras)
	a.mux.HandleFunc("PUT /api/dashboards/{d}/eras", a.putEras)
}

// getEras — эры дашборда с действующим списком (F-143).
func (a *API) getEras(w http.ResponseWriter, r *http.Request) {
	q := sqlc.New(a.d.Pool)
	acc, err := a.viewOf(r, q)
	if err != nil {
		a.recordError(w, r, err)
		return
	}
	out, err := dashboards.LoadEras(r.Context(), q, acc.Dashboard.ID)
	if err != nil {
		a.internal(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// erasIn — тело правки эр дашборда (F-143): настройки целиком и, при смене
// эры по умолчанию «к уже созданным» или переименовании эры, замена — одна
// ({from, to}) или список замен по порядку.
type erasIn struct {
	IfRev   int64           `json:"if_rev"`
	Eras    dashboards.Eras `json:"eras"`
	Replace json.RawMessage `json:"replace"`
}

// replaces — замены из тела: объект, список или ничего (null).
func (in erasIn) replaces() ([]dashboards.Replace, error) {
	raw := bytes.TrimSpace(in.Replace)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] == '[' {
		var list []dashboards.Replace
		err := json.Unmarshal(raw, &list)
		return list, err
	}
	var one dashboards.Replace
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil, err
	}
	return []dashboards.Replace{one}, nil
}

// putEras — эры дашборда; ответ — эры с действующим списком.
func (a *API) putEras(w http.ResponseWriter, r *http.Request) {
	var in erasIn
	if !decode(w, r, &in) {
		return
	}
	reps, err := in.replaces()
	if err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: "replace", Message: "замена — {from, to} или список таких"})
		return
	}
	sess, _ := SessionFrom(r.Context())
	ctx := r.Context()
	var out dashboards.ErasDTO
	found := true
	err = pgx.BeginFunc(ctx, a.d.Pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		d, ok := a.dashboard(w, r, q)
		if !ok {
			found = false
			return nil
		}
		if _, err := dashboards.SaveEras(ctx, tx, operation(r, sess.Account.ID), d, in.IfRev, in.Eras, reps); err != nil {
			return err
		}
		var err error
		out, err = dashboards.LoadEras(ctx, q, d.ID)
		return err
	})
	if !found {
		return
	}
	if errors.Is(err, dashboards.ErrErasConflict) {
		httpx.WriteError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	if errors.Is(err, dashboards.ErrEras) {
		field, msg := dashboards.EraField(err)
		httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: field, Message: msg})
		return
	}
	if err != nil {
		a.internal(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// putSharedFields — общие свои поля шаблона в дашборде (F-96) целиком, с
// версией шаблона (F-45); ответ — действующий шаблон.
func (a *API) putSharedFields(w http.ResponseWriter, r *http.Request) {
	var in records.SharedPatch
	if !decode(w, r, &in) {
		return
	}
	a.editTemplate(w, r, func(t *records.Tx, id uuid.UUID) (sqlc.DashboardTemplate, error) { return t.PatchShared(id, in) })
}

// putQuickFields — набор полей быстрой формы шаблона в дашборде (F-124).
func (a *API) putQuickFields(w http.ResponseWriter, r *http.Request) {
	var in records.QuickPatch
	if !decode(w, r, &in) {
		return
	}
	a.editTemplate(w, r, func(t *records.Tx, id uuid.UUID) (sqlc.DashboardTemplate, error) { return t.PatchQuickFields(id, in) })
}

// editTemplate — правка шаблона дашборда в транзакции; ответ — действующий шаблон.
func (a *API) editTemplate(w http.ResponseWriter, r *http.Request, apply func(*records.Tx, uuid.UUID) (sqlc.DashboardTemplate, error)) {
	sess, _ := SessionFrom(r.Context())
	ctx := r.Context()
	var out dashboards.TemplateDTO
	err := pgx.BeginFunc(ctx, a.d.Pool, func(tx pgx.Tx) error {
		dashID, err := uuid.Parse(r.PathValue("d"))
		if err != nil {
			return authz.ErrNotFound
		}
		tmplID, err := uuid.Parse(r.PathValue("t"))
		if err != nil {
			return authz.ErrNotFound
		}
		d, err := authz.Dashboard(ctx, sqlc.New(tx), sess.Account.ID, dashID)
		if err != nil {
			return err
		}
		t, err := a.rec.Begin(ctx, tx, operation(r, sess.Account.ID), d.ID)
		if err != nil {
			return err
		}
		row, err := apply(t, tmplID)
		if err != nil {
			return err
		}
		dto, ok, err := dashboards.TemplateOf(a.d.Registry, row)
		if err == nil && !ok {
			err = authz.ErrNotFound
		}
		out = dto
		return err
	})
	if err != nil {
		a.recordError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// operation — операция журнала для запроса интерфейса.
func operation(r *http.Request, actor uuid.UUID) *audit.Operation {
	return audit.New(&actor, audit.SourceUI, httpx.RequestIDFrom(r.Context()))
}

// firstDashboard — в транзакции настройки: журнал создания владельца и первый
// дашборд (В-9: дашбордов несколько, первый — автоматически). Автор в журнале —
// сам новый аккаунт (настройка, регистрация).
func (a *API) firstDashboard(r *http.Request) auth.SetupHook {
	return a.firstDashboardBy(r, nil)
}

// firstDashboardBy — то же; by — автор в журнале, если аккаунт заводит другой
// (админ, F-28); nil — сам новый аккаунт.
func (a *API) firstDashboardBy(r *http.Request, by *uuid.UUID) auth.SetupHook {
	return func(ctx context.Context, tx pgx.Tx, acc sqlc.Account) error {
		actor := acc.ID
		if by != nil {
			actor = *by
		}
		op := operation(r, actor)
		if err := op.Write(ctx, sqlc.New(tx), audit.Entry{
			Object: "account", ObjectID: &acc.ID, Op: audit.OpCreate,
			After: AccountDTO{ID: acc.ID, Login: acc.Login, IsAdmin: acc.IsAdmin},
		}); err != nil {
			return err
		}
		name := i18n.Lookup(a.d.Lang, "dashboard.defaultName")
		if name == "" {
			name = "Kintion"
		}
		_, err := dashboards.Create(ctx, tx, op, a.d.Registry, acc.ID, name)
		return err
	}
}

func (a *API) listDashboards(w http.ResponseWriter, r *http.Request) {
	sess, _ := SessionFrom(r.Context())
	q := sqlc.New(a.d.Pool)
	rows, err := q.ListDashboardsByOwner(r.Context(), sess.Account.ID)
	if err != nil {
		a.internal(w, r, err)
		return
	}
	shared, err := q.ListSharedDashboards(r.Context(), sess.Account.ID)
	if err != nil {
		a.internal(w, r, err)
		return
	}
	// свои, затем чужие, где приглашение принято (F-03)
	out := make([]SharedDashboardDTO, 0, len(rows)+len(shared))
	for _, d := range rows {
		out = append(out, SharedDashboardDTO{DTO: dashboards.ToDTO(d), CanEdit: true})
	}
	for _, d := range shared {
		if d.AcceptedAt == nil {
			continue
		}
		// может ли гость создавать записи — как при создании (F-149)
		acc, err := authz.View(r.Context(), q, sess.Account.ID, d.ID)
		if err != nil {
			a.internal(w, r, err)
			return
		}
		out = append(out, SharedDashboardDTO{DTO: dashboards.DTO{ID: d.ID, Name: d.Name, CreatedAt: d.CreatedAt}, Guest: true, OwnerNick: d.OwnerNick, CanEdit: acc.CanCreate()})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// dashboardIn — тело создания и переименования дашборда.
type dashboardIn struct {
	Name string `json:"name"`
}

// jsonKey — ключ JSON поля структуры (по тегу): ключи тел запросов не
// дублируются строками в коде.
func jsonKey(v any, field string) string {
	f, ok := reflect.TypeOf(v).FieldByName(field)
	if !ok {
		return field
	}
	key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	return key
}

func (a *API) nameError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, dashboards.ErrName) {
		httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: jsonKey(dashboardIn{}, "Name"), Message: err.Error()})
		return
	}
	a.internal(w, r, err)
}

func (a *API) createDashboard(w http.ResponseWriter, r *http.Request) {
	var in dashboardIn
	if !decode(w, r, &in) {
		return
	}
	sess, _ := SessionFrom(r.Context())
	var d sqlc.Dashboard
	err := pgx.BeginFunc(r.Context(), a.d.Pool, func(tx pgx.Tx) error {
		var err error
		d, err = dashboards.Create(r.Context(), tx, operation(r, sess.Account.ID), a.d.Registry, sess.Account.ID, in.Name)
		return err
	})
	if err != nil {
		a.nameError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, dashboards.ToDTO(d))
}

// dashboard — дашборд из пути, если смотрящему он доступен; иначе ответ 404.
func (a *API) dashboard(w http.ResponseWriter, r *http.Request, q *sqlc.Queries) (sqlc.Dashboard, bool) {
	sess, _ := SessionFrom(r.Context())
	id, err := uuid.Parse(r.PathValue("d"))
	if err == nil {
		var d sqlc.Dashboard
		if d, err = authz.Dashboard(r.Context(), q, sess.Account.ID, id); err == nil {
			return d, true
		}
		if !errors.Is(err, authz.ErrNotFound) {
			a.internal(w, r, err)
			return sqlc.Dashboard{}, false
		}
	}
	httpx.WriteError(w, http.StatusNotFound, "not_found", "дашборд не найден")
	return sqlc.Dashboard{}, false
}

func (a *API) renameDashboard(w http.ResponseWriter, r *http.Request) {
	var in dashboardIn
	if !decode(w, r, &in) {
		return
	}
	sess, _ := SessionFrom(r.Context())
	var out sqlc.Dashboard
	found := true
	err := pgx.BeginFunc(r.Context(), a.d.Pool, func(tx pgx.Tx) error {
		d, ok := a.dashboard(w, r, sqlc.New(tx))
		if !ok {
			found = false
			return nil
		}
		var err error
		out, err = dashboards.Rename(r.Context(), tx, operation(r, sess.Account.ID), d, in.Name)
		return err
	})
	if !found {
		return
	}
	if err != nil {
		a.nameError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, dashboards.ToDTO(out))
}

func (a *API) dashboardSchema(w http.ResponseWriter, r *http.Request) {
	q := sqlc.New(a.d.Pool)
	acc, err := a.viewOf(r, q)
	if err != nil {
		a.recordError(w, r, err)
		return
	}
	out, err := dashboards.Schema(r.Context(), q, a.d.Registry, acc.Dashboard.ID)
	if err != nil {
		a.internal(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
