package api

import (
	"context"
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
// дашборд (В-9: дашбордов несколько, первый — автоматически).
func (a *API) firstDashboard(r *http.Request) auth.SetupHook {
	return func(ctx context.Context, tx pgx.Tx, acc sqlc.Account) error {
		op := operation(r, acc.ID)
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
	rows, err := sqlc.New(a.d.Pool).ListDashboardsByOwner(r.Context(), sess.Account.ID)
	if err != nil {
		a.internal(w, r, err)
		return
	}
	out := make([]dashboards.DTO, 0, len(rows))
	for _, d := range rows {
		out = append(out, dashboards.ToDTO(d))
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
	d, ok := a.dashboard(w, r, q)
	if !ok {
		return
	}
	out, err := dashboards.Schema(r.Context(), q, a.d.Registry, d.ID)
	if err != nil {
		a.internal(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
