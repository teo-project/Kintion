// Package audit — журнал изменений (F-08, F-16): операция с operation_id,
// request_id, автором, источником и версиями; записи «было → стало» пишутся
// в той же транзакции, что и само изменение. Пишутся только успешные
// изменения (Д-23); снимок «что видел автор» в 0.1 = before/after (Д-24).
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"kintion/server/internal/db"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/version"
)

// Источники изменений.
const (
	SourceUI     = "ui"     // запрос интерфейса (API)
	SourceSystem = "system" // само приложение (старт, синхронизация паков)
)

// Действия.
const (
	OpCreate  = "create"
	OpUpdate  = "update"
	OpDelete  = "delete"
	OpRestore = "restore"
	OpPurge   = "purge"
	OpMove    = "move"
)

// Ops — все действия журнала (фильтр ?op=).
var Ops = []string{OpCreate, OpUpdate, OpDelete, OpRestore, OpPurge, OpMove}

// Operation — одна операция пользователя или системы; все её записи журнала
// несут один operation_id.
type Operation struct {
	ID        uuid.UUID
	Actor     *uuid.UUID
	Source    string
	RequestID string
}

// New начинает операцию.
func New(actor *uuid.UUID, source, requestID string) *Operation {
	return &Operation{ID: uuid.Must(uuid.NewV7()), Actor: actor, Source: source, RequestID: requestID}
}

// Entry — одно изменение объекта.
type Entry struct {
	Dashboard *uuid.UUID
	Entity    *uuid.UUID // запись, к истории которой относится изменение
	Object    string     // 'dashboard', 'dashboard_template', 'entity', 'field_entry', …
	ObjectID  *uuid.UUID
	Op        string
	Before    any // nil — не было
	After     any // nil — не стало
}

func raw(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("audit: %w", err)
	}
	return b, nil
}

// Write пишет запись журнала через q — запросы той же транзакции, что и изменение.
func (o *Operation) Write(ctx context.Context, q *sqlc.Queries, e Entry) error {
	before, err := raw(e.Before)
	if err != nil {
		return err
	}
	after, err := raw(e.After)
	if err != nil {
		return err
	}
	return q.InsertAuditLog(ctx, sqlc.InsertAuditLogParams{
		ID:            uuid.Must(uuid.NewV7()),
		DashboardID:   e.Dashboard,
		ActorID:       o.Actor,
		OperationID:   o.ID,
		Source:        o.Source,
		RequestID:     o.RequestID,
		AppVersion:    version.Version,
		SchemaVersion: db.SchemaVersion(),
		EntityID:      e.Entity,
		Object:        e.Object,
		ObjectID:      e.ObjectID,
		Op:            e.Op,
		Before:        before,
		After:         after,
	})
}
