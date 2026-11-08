// Package authz — единственное место проверки доступа. Изменять дашборд может
// владелец (Dashboard, Entity); читать — владелец и гость с правами на группы
// (View, ViewEntity; 0.5, F-03, F-05).
package authz

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kintion/server/internal/db/sqlc"
)

// ErrNotFound — объекта нет или он не виден: чужое неотличимо от
// несуществующего (404, не 403 — F-07).
var ErrNotFound = errors.New("не найдено")

// Dashboard возвращает дашборд, если account может его изменять: владелец.
func Dashboard(ctx context.Context, q *sqlc.Queries, account, dashboard uuid.UUID) (sqlc.Dashboard, error) {
	d, err := q.GetDashboard(ctx, dashboard)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && d.OwnerID != account) {
		return sqlc.Dashboard{}, ErrNotFound
	}
	return d, err
}

// Entity возвращает запись, если account может с ней работать: в 0.1 — запись
// дашборда, которым он владеет. Удалённые записи (корзина) тоже возвращаются —
// их видит владелец; видимость шаблона (F-60) проверяет вызывающий.
func Entity(ctx context.Context, q *sqlc.Queries, account, id uuid.UUID) (sqlc.Entity, error) {
	e, err := q.GetEntityByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.Entity{}, ErrNotFound
	}
	if err != nil {
		return sqlc.Entity{}, err
	}
	if _, err := Dashboard(ctx, q, account, e.DashboardID); err != nil {
		return sqlc.Entity{}, err
	}
	return e, nil
}

// GroupsUnder — группы, доступные через право на группы roots дашборда: сами
// roots и все вложенные в них по всем путям (F-105) — доступ к любой из
// родительских групп даёт доступ к вложенной. Права на группы появятся в 0.5;
// тот же обход фильтрует список записей по группе.
func GroupsUnder(ctx context.Context, q *sqlc.Queries, dashboard uuid.UUID, roots []uuid.UUID) ([]uuid.UUID, error) {
	return q.GroupDescendants(ctx, sqlc.GroupDescendantsParams{DashboardID: dashboard, Roots: roots})
}

// Access — доступ смотрящего к дашборду: владелец видит всё, гость (принявший
// приглашение, F-03) — записи из групп с выдачей и вложенных в них (F-05, F-105).
type Access struct {
	Dashboard sqlc.Dashboard
	Account   uuid.UUID
	Owner     bool
	// Groups — группы, видимые гостю, и есть ли на них право редактирования;
	// nil у владельца.
	Groups map[uuid.UUID]bool
	// Own — техническая группа «Созданные <ник>» этого гостя (F-149): даёт
	// видимость и правку, но самому гостю не показывается.
	Own map[uuid.UUID]bool
	// Create — у гостя прямое право правки на свою техническую группу: может
	// создавать записи (F-149); правка, унаследованная от родительской группы,
	// его не даёт — переключатель владельца действует всегда.
	Create bool
}

// Guest — смотрящий не владелец.
func (a Access) Guest() bool { return !a.Owner }

// CanCreate — смотрящий может создавать записи: владелец или гость с прямым
// правом правки на свою техническую группу (F-149).
func (a Access) CanCreate() bool { return a.Owner || a.Create }

// CanUpload — смотрящий может загружать файлы: может создавать записи или
// править хоть какие-то (фото и файлы к редактируемой записи, F-148).
func (a Access) CanUpload() bool {
	if a.CanCreate() {
		return true
	}
	for _, edit := range a.Groups {
		if edit {
			return true
		}
	}
	return false
}

// View — доступ account к дашборду на чтение: владелец или гость с принятым
// приглашением. Ждущее подтверждения приглашение доступа не даёт.
func View(ctx context.Context, q *sqlc.Queries, account, dashboard uuid.UUID) (Access, error) {
	d, err := q.GetDashboard(ctx, dashboard)
	if errors.Is(err, pgx.ErrNoRows) {
		return Access{}, ErrNotFound
	} else if err != nil {
		return Access{}, err
	}
	if d.OwnerID == account {
		return Access{Dashboard: d, Account: account, Owner: true}, nil
	}
	m, err := q.GetMember(ctx, sqlc.GetMemberParams{DashboardID: dashboard, AccountID: account})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && m.AcceptedAt == nil) {
		return Access{}, ErrNotFound
	} else if err != nil {
		return Access{}, err
	}
	rows, err := q.GuestGroups(ctx, sqlc.GuestGroupsParams{DashboardID: dashboard, AccountID: account})
	if err != nil {
		return Access{}, err
	}
	acc := Access{Dashboard: d, Account: account, Groups: make(map[uuid.UUID]bool, len(rows)), Own: map[uuid.UUID]bool{}}
	for _, r := range rows {
		acc.Groups[r.ID] = r.CanEdit
		if r.Own {
			acc.Own[r.ID] = true
		}
	}
	grants, err := q.ListMemberGrants(ctx, sqlc.ListMemberGrantsParams{DashboardID: dashboard, AccountID: account})
	if err != nil {
		return Access{}, err
	}
	for _, g := range grants {
		acc.Create = acc.Create || (g.CanEdit && acc.Own[g.GroupID])
	}
	return acc, nil
}

// ViewEntity — запись и доступ к её дашборду на чтение. Видимость самой
// записи гостю проверяет records (по группам).
func ViewEntity(ctx context.Context, q *sqlc.Queries, account, id uuid.UUID) (sqlc.Entity, Access, error) {
	e, err := q.GetEntityByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.Entity{}, Access{}, ErrNotFound
	}
	if err != nil {
		return sqlc.Entity{}, Access{}, err
	}
	acc, err := View(ctx, q, account, e.DashboardID)
	if err != nil {
		return sqlc.Entity{}, Access{}, err
	}
	return e, acc, nil
}
