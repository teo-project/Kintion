package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kintion/server/internal/authz"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/httpx"
	"kintion/server/internal/records"
)

// relatedDepth — related на 2 шага (Д-12).
const relatedDepth = 2

func (a *API) recordRoutes() {
	a.mux.HandleFunc("POST /api/dashboards/{d}/operations", a.operations)
	a.mux.HandleFunc("GET /api/records/{id}", a.getRecord)
	a.mux.HandleFunc("PATCH /api/records/{id}", a.patchRecord)
	a.mux.HandleFunc("DELETE /api/records/{id}", a.deleteRecord)
	a.mux.HandleFunc("POST /api/records/{id}/restore", a.restoreRecord)
	a.mux.HandleFunc("DELETE /api/records/{id}/purge", a.purgeRecord)
	a.mux.HandleFunc("POST /api/records/{id}/fields/{key}/values", a.addValue)
	a.mux.HandleFunc("PATCH /api/records/{id}/fields/{key}/values/{vid}", a.patchValue)
	a.mux.HandleFunc("DELETE /api/records/{id}/fields/{key}/values/{vid}", a.deleteValue)
	a.mux.HandleFunc("POST /api/records/{id}/participants", a.addParticipant)
	a.mux.HandleFunc("PATCH /api/records/{id}/participants/{pid}", a.patchParticipant)
	a.mux.HandleFunc("DELETE /api/records/{id}/participants/{pid}", a.deleteParticipant)
}

// ifRev — версия поля из ?if_rev= (DELETE без тела).
func ifRev(r *http.Request) *int64 {
	v, err := strconv.ParseInt(r.URL.Query().Get("if_rev"), 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

// pathID — id из пути; неверный — как несуществующий.
func pathID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return uuid.Nil, authz.ErrNotFound
	}
	return id, nil
}

func (a *API) addValue(w http.ResponseWriter, r *http.Request) {
	var in records.AddValue
	if !decode(w, r, &in) {
		return
	}
	a.inRecord(w, r, func(t *records.Tx, rec *records.Rec) (*records.Rec, error) {
		return t.AddValue(rec, r.PathValue("key"), in)
	})
}

func (a *API) patchValue(w http.ResponseWriter, r *http.Request) {
	var in records.PatchValue
	if !decode(w, r, &in) {
		return
	}
	a.inRecord(w, r, func(t *records.Tx, rec *records.Rec) (*records.Rec, error) {
		vid, err := pathID(r, "vid")
		if err != nil {
			return nil, err
		}
		return t.PatchValue(rec, r.PathValue("key"), vid, in)
	})
}

func (a *API) deleteValue(w http.ResponseWriter, r *http.Request) {
	a.inRecord(w, r, func(t *records.Tx, rec *records.Rec) (*records.Rec, error) {
		vid, err := pathID(r, "vid")
		if err != nil {
			return nil, err
		}
		return t.DeleteValue(rec, r.PathValue("key"), vid, ifRev(r))
	})
}

// RecordOut — ответ с записью: конверт и связанные записи для подписей.
type RecordOut struct {
	Record  *records.RecordDTO               `json:"record"`
	Related map[uuid.UUID]*records.RecordDTO `json:"related"`
}

// ConflictBody — 409: поле изменил кто-то другой (F-45), Current — его текущее состояние.
type ConflictBody struct {
	Error   string `json:"error"`
	Field   string `json:"field"`
	Message string `json:"message"`
	Current any    `json:"current"`
}

func recordOut(t *records.Tx, r *records.Rec) (*RecordOut, error) {
	d, err := t.Envelope(r)
	if err != nil {
		return nil, err
	}
	rel, err := t.Related([]*records.RecordDTO{d}, relatedDepth)
	if err != nil {
		return nil, err
	}
	return &RecordOut{Record: d, Related: rel}, nil
}

// recordError — ответ на ошибку сервиса записей.
func (a *API) recordError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *records.ValidationError
	var ce *records.ConflictError
	switch {
	case errors.Is(err, authz.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "не найдено")
	case errors.Is(err, records.ErrDeleted):
		httpx.WriteError(w, http.StatusConflict, "deleted", err.Error())
	case errors.Is(err, records.ErrNotDeleted):
		httpx.WriteError(w, http.StatusConflict, "not_deleted", err.Error())
	case errors.As(err, &ve):
		httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: ve.Field, Message: ve.Message})
	case errors.As(err, &ce):
		httpx.WriteJSON(w, http.StatusConflict, ConflictBody{Error: "conflict", Field: ce.Field,
			Message: "поле уже изменено: показано текущее состояние", Current: ce.Current})
	default:
		a.internal(w, r, err)
	}
}

// recordFn — действие над записью из пути; nil-результат — ответ 204.
type recordFn func(t *records.Tx, rec *records.Rec) (*records.Rec, error)

// inRecord — запись из пути {id} в транзакции: доступ через authz, видимость
// шаблона (F-60), действие, ответ с конвертом.
func (a *API) inRecord(w http.ResponseWriter, r *http.Request, fn recordFn) {
	sess, _ := SessionFrom(r.Context())
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		a.recordError(w, r, authz.ErrNotFound)
		return
	}
	ctx := r.Context()
	var out *RecordOut
	err = pgx.BeginFunc(ctx, a.d.Pool, func(tx pgx.Tx) error {
		row, err := authz.Entity(ctx, sqlc.New(tx), sess.Account.ID, id)
		if err != nil {
			return err
		}
		t, err := a.rec.Begin(ctx, tx, operation(r, sess.Account.ID), row.DashboardID)
		if err != nil {
			return err
		}
		rec, err := t.Record(row)
		if err != nil {
			return err
		}
		res, err := fn(t, rec)
		if err != nil || res == nil {
			return err
		}
		out, err = recordOut(t, res)
		return err
	})
	switch {
	case err != nil:
		a.recordError(w, r, err)
	case out == nil:
		w.WriteHeader(http.StatusNoContent)
	default:
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

func (a *API) getRecord(w http.ResponseWriter, r *http.Request) {
	a.inRecord(w, r, func(_ *records.Tx, rec *records.Rec) (*records.Rec, error) { return rec, nil })
}

func (a *API) patchRecord(w http.ResponseWriter, r *http.Request) {
	var in records.SystemPatch
	if !decode(w, r, &in) {
		return
	}
	a.inRecord(w, r, func(t *records.Tx, rec *records.Rec) (*records.Rec, error) { return t.PatchSystem(rec, in) })
}

// OperationOut — итог пакетной операции.
type OperationOut struct {
	OperationID uuid.UUID                        `json:"operation_id"`
	IDs         map[string]uuid.UUID             `json:"ids"`
	Records     []*records.RecordDTO             `json:"records"`
	Related     map[uuid.UUID]*records.RecordDTO `json:"related"`
	Errors      []records.FieldError             `json:"errors"`
}

// operations — пакет записей, значений и участников одной операцией.
func (a *API) operations(w http.ResponseWriter, r *http.Request) {
	var in records.Batch
	if !decode(w, r, &in) {
		return
	}
	sess, _ := SessionFrom(r.Context())
	ctx := r.Context()
	var out *OperationOut
	err := pgx.BeginFunc(ctx, a.d.Pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		dashID, err := uuid.Parse(r.PathValue("d"))
		if err != nil {
			return authz.ErrNotFound
		}
		d, err := authz.Dashboard(ctx, q, sess.Account.ID, dashID)
		if err != nil {
			return err
		}
		op := operation(r, sess.Account.ID)
		t, err := a.rec.Begin(ctx, tx, op, d.ID)
		if err != nil {
			return err
		}
		res, err := t.Create(in, sess.Account.ID)
		if err != nil {
			return err
		}
		envs, err := t.Envelopes(res.Records)
		if err != nil {
			return err
		}
		out = &OperationOut{OperationID: op.ID, IDs: res.IDs, Errors: res.Errors}
		if out.Errors == nil {
			out.Errors = []records.FieldError{}
		}
		for _, rec := range res.Records {
			out.Records = append(out.Records, envs[rec.Row.ID])
		}
		out.Related, err = t.Related(out.Records, relatedDepth)
		return err
	})
	if err != nil {
		a.recordError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (a *API) addParticipant(w http.ResponseWriter, r *http.Request) {
	var in records.AddParticipant
	if !decode(w, r, &in) {
		return
	}
	a.inRecord(w, r, func(t *records.Tx, rec *records.Rec) (*records.Rec, error) { return t.AddParticipant(rec, in) })
}

func (a *API) patchParticipant(w http.ResponseWriter, r *http.Request) {
	var in records.PatchParticipant
	if !decode(w, r, &in) {
		return
	}
	a.inRecord(w, r, func(t *records.Tx, rec *records.Rec) (*records.Rec, error) {
		pid, err := pathID(r, "pid")
		if err != nil {
			return nil, err
		}
		return t.PatchParticipant(rec, pid, in)
	})
}

func (a *API) deleteParticipant(w http.ResponseWriter, r *http.Request) {
	a.inRecord(w, r, func(t *records.Tx, rec *records.Rec) (*records.Rec, error) {
		pid, err := pathID(r, "pid")
		if err != nil {
			return nil, err
		}
		return t.DeleteParticipant(rec, pid, ifRev(r))
	})
}

// deleteRecord — в корзину (F-44); в 0.1 удаляет владелец (authz).
func (a *API) deleteRecord(w http.ResponseWriter, r *http.Request) {
	sess, _ := SessionFrom(r.Context())
	a.inRecord(w, r, func(t *records.Tx, rec *records.Rec) (*records.Rec, error) { return t.Delete(rec, sess.Account.ID) })
}

func (a *API) restoreRecord(w http.ResponseWriter, r *http.Request) {
	a.inRecord(w, r, func(t *records.Tx, rec *records.Rec) (*records.Rec, error) { return t.Restore(rec) })
}

// purgeRecord — окончательная очистка записи из корзины; ответ 204.
func (a *API) purgeRecord(w http.ResponseWriter, r *http.Request) {
	a.inRecord(w, r, func(t *records.Tx, rec *records.Rec) (*records.Rec, error) { return nil, t.Purge(rec) })
}
