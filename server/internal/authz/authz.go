// Package authz — единственное место проверки доступа. В 0.1 смотрящий —
// владелец дашборда; в 0.5 сюда добавятся гости и права на группы.
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

// Dashboard возвращает дашборд, если account может с ним работать.
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
