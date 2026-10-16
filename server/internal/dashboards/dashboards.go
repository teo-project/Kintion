// Package dashboards — дашборды (F-01) и их шаблоны (F-36, F-58, F-60).
package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kintion/server/internal/audit"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/packs"
)

// ErrName — неверное название дашборда.
var ErrName = errors.New("название: от 1 до 200 символов")

func newID() uuid.UUID { return uuid.Must(uuid.NewV7()) }

// NormName проверяет и очищает название.
func NormName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 200 {
		return "", ErrName
	}
	return name, nil
}

// snapshot — вид дашборда в журнале.
type snapshot struct {
	ID      uuid.UUID `json:"id"`
	OwnerID uuid.UUID `json:"owner_id"`
	Name    string    `json:"name"`
}

func snap(d sqlc.Dashboard) snapshot { return snapshot{ID: d.ID, OwnerID: d.OwnerID, Name: d.Name} }

type templateSnapshot struct {
	ID          uuid.UUID `json:"id"`
	PackID      string    `json:"pack_id"`
	TemplateKey string    `json:"template_key"`
	Entity      string    `json:"entity"`
	PackVersion string    `json:"pack_version"`
	Active      bool      `json:"active"`
}

// Create создаёт дашборд и строку dashboard_template на каждый шаблон
// включённых паков; всё — в журнал одной операцией.
func Create(ctx context.Context, tx pgx.Tx, op *audit.Operation, reg *packs.Registry, owner uuid.UUID, name string) (sqlc.Dashboard, error) {
	name, err := NormName(name)
	if err != nil {
		return sqlc.Dashboard{}, err
	}
	q := sqlc.New(tx)
	d, err := q.InsertDashboard(ctx, sqlc.InsertDashboardParams{ID: newID(), OwnerID: owner, Name: name})
	if err != nil {
		return sqlc.Dashboard{}, err
	}
	if err := op.Write(ctx, q, audit.Entry{
		Dashboard: &d.ID, Object: "dashboard", ObjectID: &d.ID, Op: audit.OpCreate, After: snap(d),
	}); err != nil {
		return sqlc.Dashboard{}, err
	}
	for _, t := range reg.Templates() {
		if err := addTemplate(ctx, q, op, d.ID, reg, t); err != nil {
			return sqlc.Dashboard{}, err
		}
	}
	return d, nil
}

func addTemplate(ctx context.Context, q *sqlc.Queries, op *audit.Operation, dashboard uuid.UUID, reg *packs.Registry, t *packs.Template) error {
	version := ""
	for _, m := range reg.Packs {
		if m.ID == t.Pack {
			version = m.Version
		}
	}
	row, err := q.InsertDashboardTemplate(ctx, sqlc.InsertDashboardTemplateParams{
		ID: newID(), DashboardID: dashboard, PackID: t.Pack, TemplateKey: t.ID, Entity: t.Entity, PackVersion: version,
	})
	if err != nil {
		return err
	}
	return op.Write(ctx, q, audit.Entry{
		Dashboard: &dashboard, Object: "dashboard_template", ObjectID: &row.ID, Op: audit.OpCreate,
		After: templateSnapshot{ID: row.ID, PackID: row.PackID, TemplateKey: row.TemplateKey,
			Entity: row.Entity, PackVersion: row.PackVersion, Active: row.Active},
	})
}

// Rename переименовывает дашборд с записью в журнал.
func Rename(ctx context.Context, tx pgx.Tx, op *audit.Operation, d sqlc.Dashboard, name string) (sqlc.Dashboard, error) {
	name, err := NormName(name)
	if err != nil {
		return sqlc.Dashboard{}, err
	}
	if name == d.Name {
		return d, nil
	}
	q := sqlc.New(tx)
	nd, err := q.RenameDashboard(ctx, sqlc.RenameDashboardParams{ID: d.ID, Name: name})
	if err != nil {
		return sqlc.Dashboard{}, err
	}
	return nd, op.Write(ctx, q, audit.Entry{
		Dashboard: &d.ID, Object: "dashboard", ObjectID: &d.ID, Op: audit.OpUpdate, Before: snap(d), After: snap(nd),
	})
}

// SyncTemplates при старте: шаблоны незагруженных паков — active=false, вернувшиеся —
// снова true (F-60); шаблоны новых включённых паков добавляются в существующие
// дашборды (F-36). Добавления пишутся в журнал операцией системы.
func SyncTemplates(ctx context.Context, pool *pgxpool.Pool, reg *packs.Registry) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		loaded := make([]string, 0)
		for _, t := range reg.Templates() {
			loaded = append(loaded, t.Pack+"/"+t.ID)
		}
		if err := q.SetTemplatesActive(ctx, loaded); err != nil {
			return err
		}
		ids, err := q.ListLiveDashboardIDs(ctx)
		if err != nil {
			return err
		}
		op := audit.New(nil, audit.SourceSystem, "")
		for _, id := range ids {
			have, err := q.ListDashboardTemplates(ctx, id)
			if err != nil {
				return err
			}
			known := map[string]bool{}
			for _, h := range have {
				known[h.PackID+"/"+h.TemplateKey] = true
			}
			for _, t := range reg.Templates() {
				if !known[t.Pack+"/"+t.ID] {
					if err := addTemplate(ctx, q, op, id, reg, t); err != nil {
						return fmt.Errorf("dashboard %s: %w", id, err)
					}
				}
			}
		}
		return nil
	})
}

// Effective — действующее определение шаблона: шаблон пака + ручные правки
// overrides (F-36, F-58). В 0.1 правок нет; слияние — верхний уровень ключей.
func Effective(packTemplate, overrides json.RawMessage) (json.RawMessage, error) {
	var ov map[string]json.RawMessage
	if len(overrides) > 0 {
		if err := json.Unmarshal(overrides, &ov); err != nil {
			return nil, fmt.Errorf("overrides: %w", err)
		}
	}
	if len(ov) == 0 {
		return packTemplate, nil
	}
	var base map[string]json.RawMessage
	if err := json.Unmarshal(packTemplate, &base); err != nil {
		return nil, err
	}
	for k, v := range ov {
		base[k] = v
	}
	return json.Marshal(base)
}

// DTO — дашборд в ответах API.
type DTO struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ToDTO — дашборд для ответа.
func ToDTO(d sqlc.Dashboard) DTO { return DTO{ID: d.ID, Name: d.Name, CreatedAt: d.CreatedAt} }

// TemplateDTO — действующий шаблон дашборда.
type TemplateDTO struct {
	ID          uuid.UUID `json:"id"`
	Key         string    `json:"key"`
	Entity      string    `json:"entity"`
	Pack        string    `json:"pack"`
	PackVersion string    `json:"pack_version"`
	Default     bool      `json:"default"`
	// Rev — версия правок шаблона в дашборде (общие свои поля, F-96, F-45).
	Rev        int64           `json:"rev"`
	Definition json.RawMessage `json:"definition"`
}

// TemplateOf — действующий шаблон дашборда для ответа; ok=false — шаблон
// неактивен, не загружен или его сущность не загружена (F-60).
func TemplateOf(reg *packs.Registry, r sqlc.DashboardTemplate) (TemplateDTO, bool, error) {
	t, ok := reg.Template(r.TemplateKey)
	if !r.Active || !ok || t.Pack != r.PackID {
		return TemplateDTO{}, false, nil
	}
	if _, ok := reg.Entity(t.Entity); !ok {
		return TemplateDTO{}, false, nil
	}
	def, err := Effective(t.Raw, r.Overrides)
	if err != nil {
		return TemplateDTO{}, false, fmt.Errorf("шаблон %s: %w", r.TemplateKey, err)
	}
	return TemplateDTO{
		ID: r.ID, Key: r.TemplateKey, Entity: r.Entity, Pack: r.PackID, PackVersion: r.PackVersion,
		Default: r.TemplateKey == r.Entity, Rev: r.Rev, Definition: def,
	}, true, nil
}

// SchemaDTO — реестр дашборда для фронтенда: сущности и действующие шаблоны.
type SchemaDTO struct {
	Language  string            `json:"language"`
	Entities  []json.RawMessage `json:"entities"`
	Templates []TemplateDTO     `json:"templates"`
}

// Schema собирает реестр дашборда: только активные шаблоны, чья сущность загружена.
func Schema(ctx context.Context, q *sqlc.Queries, reg *packs.Registry, dashboard uuid.UUID) (SchemaDTO, error) {
	out := SchemaDTO{Language: reg.Language, Entities: []json.RawMessage{}, Templates: []TemplateDTO{}}
	for _, e := range reg.Entities() {
		out.Entities = append(out.Entities, e.Raw)
	}
	rows, err := q.ListDashboardTemplates(ctx, dashboard)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		dto, ok, err := TemplateOf(reg, r)
		if err != nil {
			return out, err
		}
		if ok {
			out.Templates = append(out.Templates, dto)
		}
	}
	return out, nil
}
