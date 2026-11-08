package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

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

// Совместный доступ (0.5, KIN-6): ник, гости дашборда и права на группы
// (F-03), приглашения в дашборд, регистрация и приглашения на инстанс (F-28),
// аккаунты для админа.
func (a *API) sharingRoutes() {
	a.mux.HandleFunc("PUT /api/me/nick", a.putNick)
	a.mux.HandleFunc("GET /api/register", a.registerStatus)
	a.mux.HandleFunc("POST /api/register", a.register)

	a.mux.HandleFunc("GET /api/dashboards/{d}/members", a.listMembers)
	a.mux.HandleFunc("PUT /api/dashboards/{d}/grants", a.putGrant)
	a.mux.HandleFunc("DELETE /api/dashboards/{d}/grants/{g}/{acc}", a.deleteGrant)
	a.mux.HandleFunc("DELETE /api/dashboards/{d}/members/{acc}", a.deleteMember)
	a.mux.HandleFunc("DELETE /api/dashboards/{d}/membership", a.leaveDashboard)

	a.mux.HandleFunc("GET /api/invitations", a.listInvitations)
	a.mux.HandleFunc("POST /api/invitations/{d}/accept", a.acceptInvitation)
	a.mux.HandleFunc("POST /api/invitations/{d}/decline", a.leaveDashboard)

	a.mux.HandleFunc("GET /api/admin/accounts", a.adminAccounts)
	a.mux.HandleFunc("POST /api/admin/accounts", a.adminCreateAccount)
	a.mux.HandleFunc("GET /api/invites", a.listInvites)
	a.mux.HandleFunc("POST /api/invites", a.createInvite)
	a.mux.HandleFunc("DELETE /api/invites/{id}", a.deleteInvite)
}

// viewOf — доступ смотрящего к дашборду из пути {d} на чтение (владелец или
// гость с принятым приглашением).
func (a *API) viewOf(r *http.Request, q *sqlc.Queries) (authz.Access, error) {
	sess, _ := SessionFrom(r.Context())
	id, err := pathID(r, "d")
	if err != nil {
		return authz.Access{}, err
	}
	return authz.View(r.Context(), q, sess.Account.ID, id)
}

// inView — чтение дашборда из пути {d} глазами смотрящего (гость видит только
// записи и группы по своим правам, F-05).
func (a *API) inView(w http.ResponseWriter, r *http.Request, fn func(t *records.Tx) (any, error)) {
	ctx := r.Context()
	var out any
	err := pgx.BeginTxFunc(ctx, a.d.Pool, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead}, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		acc, err := a.viewOf(r, q)
		if err != nil {
			return err
		}
		t, err := a.rec.ReadAs(ctx, q, acc)
		if err != nil {
			return err
		}
		out, err = fn(t)
		return err
	})
	if err != nil {
		a.recordError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// guestGroupName — название технической группы гостя «Созданные <ник>»
// (F-149) на языке инстанса.
func (a *API) guestGroupName(acc sqlc.Account) string {
	name := i18n.Lookup(a.d.Lang, "sharing.guestGroup")
	if name == "" {
		name = "{nick}"
	}
	return strings.ReplaceAll(name, "{nick}", nickOrLogin(acc))
}

// nickOrLogin — ник аккаунта, без ника — логин.
func nickOrLogin(acc sqlc.Account) string {
	if acc.Nick != nil {
		return *acc.Nick
	}
	return acc.Login
}

// ensureGuestGroup — техническая группа принявшего приглашение гостя (F-149);
// accept — при самом принятии (право на прежнюю группу выдаётся заново).
func (a *API) ensureGuestGroup(ctx context.Context, tx pgx.Tx, op *audit.Operation, dashboard uuid.UUID, guest sqlc.Account, accept bool) error {
	t, err := a.rec.Begin(ctx, tx, op, dashboard)
	if err != nil {
		return err
	}
	return t.EnsureGuestGroup(guest.ID, a.guestGroupName(guest), accept)
}

// ---- ник ----

func accountDTO(acc sqlc.Account) AccountDTO {
	return AccountDTO{ID: acc.ID, Login: acc.Login, Nick: acc.Nick, IsAdmin: acc.IsAdmin}
}

// putNick — PUT /api/me/nick {nick}: свой ник — адрес для приглашений
// (F-03); пустой — без ника. В названиях технических групп гостя старый ник
// заменяется новым (F-149).
func (a *API) putNick(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nick string `json:"nick"`
	}
	if !decode(w, r, &in) {
		return
	}
	sess, _ := SessionFrom(r.Context())
	op := operation(r, sess.Account.ID)
	acc, err := a.d.Auth.SetNick(r.Context(), sess.Account.ID, in.Nick, func(ctx context.Context, tx pgx.Tx, old, acc sqlc.Account) error {
		// без ника в названии — логин (как в guestGroupName)
		from, to := nickOrLogin(old), nickOrLogin(acc)
		if from == to {
			return nil
		}
		groups, err := sqlc.New(tx).ListGuestGroupsOf(ctx, &acc.ID)
		if err != nil {
			return err
		}
		for _, g := range groups {
			t, err := a.rec.Begin(ctx, tx, op, g.DashboardID)
			if err != nil {
				return err
			}
			if err := t.RenameGuestGroup(g, from, to); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		a.authError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, accountDTO(acc))
}

// ---- регистрация (F-28) ----

// RegisterStatus — режим регистрации и годность приглашения из ?token=.
type RegisterStatus struct {
	Mode        string `json:"mode"`
	InviteValid bool   `json:"invite_valid"`
}

func (a *API) registerStatus(w http.ResponseWriter, r *http.Request) {
	out := RegisterStatus{Mode: a.d.Registration}
	if tok := r.URL.Query().Get("token"); tok != "" && auth.ByInvite(a.d.Registration) {
		ok, err := a.d.Auth.InviteValid(r.Context(), tok)
		if err != nil {
			a.internal(w, r, err)
			return
		}
		out.InviteValid = ok
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Login    string `json:"login"`
		Password string `json:"password"`
		Nick     string `json:"nick"`
	}
	if !decode(w, r, &in) {
		return
	}
	sess, err := a.d.Auth.Register(r.Context(), a.d.Registration, in.Token,
		auth.NewAccount{Login: in.Login, Password: in.Password, Nick: in.Nick}, r.UserAgent(), a.firstDashboard(r))
	switch {
	case errors.Is(err, auth.ErrRegistrationClosed):
		httpx.WriteError(w, http.StatusForbidden, "registration_closed", err.Error())
		return
	case errors.Is(err, auth.ErrBadInvite):
		httpx.WriteError(w, http.StatusForbidden, "bad_invite", err.Error())
		return
	case err != nil:
		a.authError(w, r, err)
		return
	}
	a.setCookie(w, sess)
	httpx.WriteJSON(w, http.StatusCreated, meOf(sess))
}

// ---- гости и права (владелец) ----

// GrantDTO — право гостя на группу.
type GrantDTO struct {
	GroupID uuid.UUID `json:"group_id"`
	CanEdit bool      `json:"can_edit"`
}

// MemberDTO — гость дашборда: ник (без ника — логин), принял ли приглашение,
// права на группы; OwnGroup — его техническая группа (F-149): право правки на
// неё — право создавать записи.
type MemberDTO struct {
	AccountID uuid.UUID  `json:"account_id"`
	Nick      *string    `json:"nick"`
	Login     string     `json:"login"`
	InvitedAt time.Time  `json:"invited_at"`
	Accepted  bool       `json:"accepted"`
	Grants    []GrantDTO `json:"grants"`
	OwnGroup  *uuid.UUID `json:"own_group"`
}

// grantSnap — право в журнале (F-16).
type grantSnap struct {
	AccountID uuid.UUID `json:"account_id"`
	Nick      *string   `json:"nick"`
	GroupID   uuid.UUID `json:"group_id"`
	CanEdit   bool      `json:"can_edit"`
}

// ownerTx — изменение доступа к дашборду {d}: только владелец (остальным 404).
func (a *API) ownerTx(w http.ResponseWriter, r *http.Request, status int, fn func(tx pgx.Tx, d sqlc.Dashboard, op *audit.Operation) (any, error)) {
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
		out, err = fn(tx, d, operation(r, sess.Account.ID))
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

func members(r *http.Request, q *sqlc.Queries, dashboard uuid.UUID) ([]MemberDTO, error) {
	rows, err := q.ListMembers(r.Context(), dashboard)
	if err != nil {
		return nil, err
	}
	grants, err := q.ListGrants(r.Context(), dashboard)
	if err != nil {
		return nil, err
	}
	by := map[uuid.UUID][]GrantDTO{}
	for _, g := range grants {
		by[g.AccountID] = append(by[g.AccountID], GrantDTO{GroupID: g.GroupID, CanEdit: g.CanEdit})
	}
	groups, err := q.ListGroups(r.Context(), dashboard)
	if err != nil {
		return nil, err
	}
	own := map[uuid.UUID]*uuid.UUID{}
	for _, g := range groups {
		if g.CreatedFor != nil {
			own[*g.CreatedFor] = &g.ID
		}
	}
	out := make([]MemberDTO, 0, len(rows))
	for _, m := range rows {
		gs := by[m.AccountID]
		if gs == nil {
			gs = []GrantDTO{}
		}
		out = append(out, MemberDTO{AccountID: m.AccountID, Nick: m.Nick, Login: m.Login, InvitedAt: m.CreatedAt, Accepted: m.AcceptedAt != nil, Grants: gs, OwnGroup: own[m.AccountID]})
	}
	return out, nil
}

// listMembers — GET /api/dashboards/{d}/members: гости и их права (владелец).
func (a *API) listMembers(w http.ResponseWriter, r *http.Request) {
	a.ownerTx(w, r, http.StatusOK, func(tx pgx.Tx, d sqlc.Dashboard, _ *audit.Operation) (any, error) {
		return members(r, sqlc.New(tx), d.ID)
	})
}

// putGrant — PUT /api/dashboards/{d}/grants {nick | account_id, group_id,
// can_edit}: выдать или изменить право на группу (F-03). Новый гость
// приглашается по нику; гостю дашборда право меняется по аккаунту — ник для
// этого не нужен. Первая выдача в дашборд — приглашение, его гость
// подтверждает; дальнейшие действуют сразу. Ответ — гости дашборда.
func (a *API) putGrant(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nick      string     `json:"nick"`
		AccountID *uuid.UUID `json:"account_id"`
		GroupID   uuid.UUID  `json:"group_id"`
		CanEdit   bool       `json:"can_edit"`
	}
	if !decode(w, r, &in) {
		return
	}
	sess, _ := SessionFrom(r.Context())
	a.ownerTx(w, r, http.StatusOK, func(tx pgx.Tx, d sqlc.Dashboard, op *audit.Operation) (any, error) {
		ctx, q := r.Context(), sqlc.New(tx)
		guest, err := grantee(ctx, q, d.ID, in.Nick, in.AccountID)
		if err != nil {
			return nil, err
		}
		if guest.ID == d.OwnerID {
			return nil, &records.ValidationError{Field: "nick", Message: "владелец и так видит весь дашборд"}
		}
		if _, err := q.GetGroup(ctx, sqlc.GetGroupParams{DashboardID: d.ID, ID: in.GroupID}); errors.Is(err, pgx.ErrNoRows) {
			return nil, &records.ValidationError{Field: "group_id", Message: "нет такой группы"}
		} else if err != nil {
			return nil, err
		}
		invited, err := q.InsertMember(ctx, sqlc.InsertMemberParams{DashboardID: d.ID, AccountID: guest.ID, InvitedBy: &sess.Account.ID})
		if err != nil {
			return nil, err
		}
		if invited > 0 { // первая выдача — приглашение в дашборд (F-03)
			if err := op.Write(ctx, q, audit.Entry{Dashboard: &d.ID, Object: "dashboard_member", ObjectID: &guest.ID, Op: audit.OpCreate,
				After: memberSnap{AccountID: guest.ID, Nick: guest.Nick}}); err != nil {
				return nil, err
			}
		}
		old, err := q.GetGrant(ctx, sqlc.GetGrantParams{DashboardID: d.ID, GroupID: in.GroupID, AccountID: guest.ID})
		existed := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		g, err := q.UpsertGrant(ctx, sqlc.UpsertGrantParams{DashboardID: d.ID, GroupID: in.GroupID, AccountID: guest.ID, CanEdit: in.CanEdit})
		if err != nil {
			return nil, err
		}
		e := audit.Entry{Dashboard: &d.ID, Object: "grp_grant", ObjectID: &g.GroupID, Op: audit.OpCreate,
			After: grantSnap{AccountID: guest.ID, Nick: guest.Nick, GroupID: g.GroupID, CanEdit: g.CanEdit}}
		if existed {
			e.Op = audit.OpUpdate
			e.Before = grantSnap{AccountID: guest.ID, Nick: guest.Nick, GroupID: old.GroupID, CanEdit: old.CanEdit}
		}
		if !existed || old.CanEdit != g.CanEdit {
			if err := op.Write(ctx, q, e); err != nil {
				return nil, err
			}
		}
		// гость принял приглашение до появления технической группы при принятии
		// (F-149) — группа появляется при любой выдаче права, и повторной
		if m, err := q.GetMember(ctx, sqlc.GetMemberParams{DashboardID: d.ID, AccountID: guest.ID}); err != nil {
			return nil, err
		} else if m.AcceptedAt != nil {
			if err := a.ensureGuestGroup(ctx, tx, op, d.ID, guest, false); err != nil {
				return nil, err
			}
		}
		return members(r, q, d.ID)
	})
}

// grantee — кому выдаётся право: гость дашборда по аккаунту или аккаунт по
// нику. Аккаунты не ищутся: нет такого ника — нет (F-03); по id — только
// гости этого дашборда.
func grantee(ctx context.Context, q *sqlc.Queries, dashboard uuid.UUID, nickIn string, account *uuid.UUID) (sqlc.Account, error) {
	if account != nil {
		if _, err := q.GetMember(ctx, sqlc.GetMemberParams{DashboardID: dashboard, AccountID: *account}); errors.Is(err, pgx.ErrNoRows) {
			return sqlc.Account{}, &records.ValidationError{Field: "account_id", Message: "нет такого гостя"}
		} else if err != nil {
			return sqlc.Account{}, err
		}
		return q.GetAccount(ctx, *account)
	}
	nick := auth.NormNick(nickIn)
	if nick == nil {
		return sqlc.Account{}, &records.ValidationError{Field: "nick", Message: "нужен ник"}
	}
	acc, err := q.GetAccountByNick(ctx, *nick)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.Account{}, &records.ValidationError{Field: "nick", Message: "нет аккаунта с таким ником"}
	}
	return acc, err
}

// deleteGrant — DELETE /api/dashboards/{d}/grants/{g}/{acc}: отозвать право.
func (a *API) deleteGrant(w http.ResponseWriter, r *http.Request) {
	a.ownerTx(w, r, http.StatusOK, func(tx pgx.Tx, d sqlc.Dashboard, op *audit.Operation) (any, error) {
		ctx, q := r.Context(), sqlc.New(tx)
		gid, err := pathID(r, "g")
		if err != nil {
			return nil, err
		}
		acc, err := pathID(r, "acc")
		if err != nil {
			return nil, err
		}
		g, err := q.GetGrant(ctx, sqlc.GetGrantParams{DashboardID: d.ID, GroupID: gid, AccountID: acc})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, authz.ErrNotFound
		} else if err != nil {
			return nil, err
		}
		if err := q.DeleteGrant(ctx, sqlc.DeleteGrantParams{DashboardID: d.ID, GroupID: gid, AccountID: acc}); err != nil {
			return nil, err
		}
		guest, err := q.GetAccount(ctx, acc)
		if err != nil {
			return nil, err
		}
		if err := op.Write(ctx, q, audit.Entry{Dashboard: &d.ID, Object: "grp_grant", ObjectID: &gid, Op: audit.OpDelete,
			Before: grantSnap{AccountID: acc, Nick: guest.Nick, GroupID: gid, CanEdit: g.CanEdit}}); err != nil {
			return nil, err
		}
		return members(r, q, d.ID)
	})
}

// memberSnap — гость в журнале.
type memberSnap struct {
	AccountID uuid.UUID `json:"account_id"`
	Nick      *string   `json:"nick"`
	Accepted  bool      `json:"accepted"`
}

// deleteMember — DELETE /api/dashboards/{d}/members/{acc}: владелец убирает
// гостя со всеми его правами.
func (a *API) deleteMember(w http.ResponseWriter, r *http.Request) {
	a.ownerTx(w, r, http.StatusOK, func(tx pgx.Tx, d sqlc.Dashboard, op *audit.Operation) (any, error) {
		q := sqlc.New(tx)
		acc, err := pathID(r, "acc")
		if err != nil {
			return nil, err
		}
		if err := removeMember(r, q, op, d.ID, acc); err != nil {
			return nil, err
		}
		return members(r, q, d.ID)
	})
}

// removeMember — гость уходит из дашборда (сам или его убирает владелец):
// права удаляются вместе с ним; в журнале — каждое право и сам гость.
func removeMember(r *http.Request, q *sqlc.Queries, op *audit.Operation, dashboard, account uuid.UUID) error {
	ctx := r.Context()
	m, err := q.LockMember(ctx, sqlc.LockMemberParams{DashboardID: dashboard, AccountID: account})
	if errors.Is(err, pgx.ErrNoRows) {
		return authz.ErrNotFound
	} else if err != nil {
		return err
	}
	guest, err := q.GetAccount(ctx, account)
	if err != nil {
		return err
	}
	grants, err := q.ListMemberGrants(ctx, sqlc.ListMemberGrantsParams{DashboardID: dashboard, AccountID: account})
	if err != nil {
		return err
	}
	if err := q.DeleteMember(ctx, sqlc.DeleteMemberParams{DashboardID: dashboard, AccountID: account}); err != nil {
		return err
	}
	for _, g := range grants {
		if err := op.Write(ctx, q, audit.Entry{Dashboard: &dashboard, Object: "grp_grant", ObjectID: &g.GroupID, Op: audit.OpDelete,
			Before: grantSnap{AccountID: account, Nick: guest.Nick, GroupID: g.GroupID, CanEdit: g.CanEdit}}); err != nil {
			return err
		}
	}
	return op.Write(ctx, q, audit.Entry{Dashboard: &dashboard, Object: "dashboard_member", ObjectID: &account, Op: audit.OpDelete,
		Before: memberSnap{AccountID: account, Nick: guest.Nick, Accepted: m.AcceptedAt != nil}})
}

// ---- приглашения в дашборд (гость) ----

// InvitationDTO — дашборд, куда аккаунт приглашён или уже гость.
type InvitationDTO struct {
	DashboardID uuid.UUID `json:"dashboard_id"`
	Name        string    `json:"name"`
	OwnerNick   *string   `json:"owner_nick"`
	InvitedAt   time.Time `json:"invited_at"`
}

// listInvitations — GET /api/invitations: приглашения, ждущие ответа.
func (a *API) listInvitations(w http.ResponseWriter, r *http.Request) {
	sess, _ := SessionFrom(r.Context())
	rows, err := sqlc.New(a.d.Pool).ListSharedDashboards(r.Context(), sess.Account.ID)
	if err != nil {
		a.internal(w, r, err)
		return
	}
	out := []InvitationDTO{}
	for _, d := range rows {
		if d.AcceptedAt == nil {
			out = append(out, InvitationDTO{DashboardID: d.ID, Name: d.Name, OwnerNick: d.OwnerNick, InvitedAt: d.InvitedAt})
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// guestTx — действие гостя над своим членством в дашборде {d}.
func (a *API) guestTx(w http.ResponseWriter, r *http.Request, fn func(tx pgx.Tx, op *audit.Operation, dashboard, account uuid.UUID) error) {
	sess, _ := SessionFrom(r.Context())
	err := pgx.BeginFunc(r.Context(), a.d.Pool, func(tx pgx.Tx) error {
		id, err := pathID(r, "d")
		if err != nil {
			return err
		}
		return fn(tx, operation(r, sess.Account.ID), id, sess.Account.ID)
	})
	if err != nil {
		a.recordError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// acceptInvitation — POST /api/invitations/{d}/accept: гость в дашборде,
// у него — техническая группа (F-149).
func (a *API) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	a.guestTx(w, r, func(tx pgx.Tx, op *audit.Operation, dashboard, account uuid.UUID) error {
		ctx, q := r.Context(), sqlc.New(tx)
		m, err := q.LockMember(ctx, sqlc.LockMemberParams{DashboardID: dashboard, AccountID: account})
		if errors.Is(err, pgx.ErrNoRows) {
			return authz.ErrNotFound
		} else if err != nil || m.AcceptedAt != nil {
			return err
		}
		if err := q.AcceptMember(ctx, sqlc.AcceptMemberParams{DashboardID: dashboard, AccountID: account}); err != nil {
			return err
		}
		// ник под блокировкой: смена ника не разойдётся с названием группы
		acc, err := q.LockAccount(ctx, account)
		if err != nil {
			return err
		}
		if err := op.Write(ctx, q, audit.Entry{Dashboard: &dashboard, Object: "dashboard_member", ObjectID: &account, Op: audit.OpUpdate,
			Before: memberSnap{AccountID: account, Nick: acc.Nick}, After: memberSnap{AccountID: account, Nick: acc.Nick, Accepted: true}}); err != nil {
			return err
		}
		return a.ensureGuestGroup(ctx, tx, op, dashboard, acc, true)
	})
}

// leaveDashboard — отказ от приглашения или уход гостя из дашборда.
func (a *API) leaveDashboard(w http.ResponseWriter, r *http.Request) {
	a.guestTx(w, r, func(tx pgx.Tx, op *audit.Operation, dashboard, account uuid.UUID) error {
		return removeMember(r, sqlc.New(tx), op, dashboard, account)
	})
}

// SharedDashboardDTO — дашборд в списке: свой или чужой (гость). CanEdit —
// гость может создавать записи (право правки на свою техническую группу, F-149).
type SharedDashboardDTO struct {
	dashboards.DTO
	Guest     bool    `json:"guest"`
	OwnerNick *string `json:"owner_nick,omitempty"`
	CanEdit   bool    `json:"can_edit"`
}

// ---- админ: аккаунты и приглашения на инстанс ----

// adminOnly — только админ инстанса; остальным маршрута как нет (404).
func adminOnly(w http.ResponseWriter, r *http.Request) (auth.Session, bool) {
	sess, _ := SessionFrom(r.Context())
	if !sess.Account.IsAdmin {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "нет такого метода API")
		return sess, false
	}
	return sess, true
}

// AdminAccountDTO — аккаунт в списке админа.
type AdminAccountDTO struct {
	AccountDTO
	CreatedAt time.Time `json:"created_at"`
}

func (a *API) adminAccounts(w http.ResponseWriter, r *http.Request) {
	if _, ok := adminOnly(w, r); !ok {
		return
	}
	rows, err := sqlc.New(a.d.Pool).ListAccounts(r.Context())
	if err != nil {
		a.internal(w, r, err)
		return
	}
	out := make([]AdminAccountDTO, 0, len(rows))
	for _, x := range rows {
		out = append(out, AdminAccountDTO{AccountDTO: AccountDTO{ID: x.ID, Login: x.Login, Nick: x.Nick, IsAdmin: x.IsAdmin}, CreatedAt: x.CreatedAt})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// adminCreateAccount — POST /api/admin/accounts {login, password, nick}.
func (a *API) adminCreateAccount(w http.ResponseWriter, r *http.Request) {
	sess, ok := adminOnly(w, r)
	if !ok {
		return
	}
	var in struct {
		Login    string `json:"login"`
		Password string `json:"password"`
		Nick     string `json:"nick"`
	}
	if !decode(w, r, &in) {
		return
	}
	acc, err := a.d.Auth.CreateAccount(r.Context(), auth.NewAccount{Login: in.Login, Password: in.Password, Nick: in.Nick}, a.firstDashboardBy(r, &sess.Account.ID))
	if err != nil {
		a.authError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, AdminAccountDTO{AccountDTO: accountDTO(acc), CreatedAt: acc.CreatedAt})
}

// InviteDTO — приглашение на инстанс; Token — только в ответе на создание.
type InviteDTO struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Token     string    `json:"token,omitempty"`
}

// inviter — кто может создавать ссылки-приглашения (F-28): в invite — админ,
// в invite-all — любой; остальным маршрута как нет (404). by — чьи
// приглашения видны и удаляются: nil у админа (все), иначе — свои.
func (a *API) inviter(w http.ResponseWriter, r *http.Request) (auth.Session, *uuid.UUID, bool) {
	sess, _ := SessionFrom(r.Context())
	if !auth.CanInvite(a.d.Registration, sess.Account.IsAdmin) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "нет такого метода API")
		return sess, nil, false
	}
	if sess.Account.IsAdmin {
		return sess, nil, true
	}
	return sess, &sess.Account.ID, true
}

// listInvites — GET /api/invites: действующие приглашения (админу — все, остальным — свои).
func (a *API) listInvites(w http.ResponseWriter, r *http.Request) {
	_, by, ok := a.inviter(w, r)
	if !ok {
		return
	}
	rows, err := sqlc.New(a.d.Pool).ListInvites(r.Context(), by)
	if err != nil {
		a.internal(w, r, err)
		return
	}
	out := make([]InviteDTO, 0, len(rows))
	for _, x := range rows {
		out = append(out, InviteDTO{ID: x.ID, CreatedAt: x.CreatedAt, ExpiresAt: x.ExpiresAt})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// createInvite — POST /api/invites: одноразовая ссылка на регистрацию
// (действует InviteTTL); токен показывается один раз.
func (a *API) createInvite(w http.ResponseWriter, r *http.Request) {
	sess, _, ok := a.inviter(w, r)
	if !ok {
		return
	}
	inv, token, err := a.d.Auth.CreateInvite(r.Context(), sess.Account.ID)
	if err != nil {
		a.internal(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, InviteDTO{ID: inv.ID, CreatedAt: inv.CreatedAt, ExpiresAt: inv.ExpiresAt, Token: token})
}

func (a *API) deleteInvite(w http.ResponseWriter, r *http.Request) {
	_, by, ok := a.inviter(w, r)
	if !ok {
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		a.recordError(w, r, err)
		return
	}
	if err := sqlc.New(a.d.Pool).DeleteInvite(r.Context(), sqlc.DeleteInviteParams{ID: id, By: by}); err != nil {
		a.internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
