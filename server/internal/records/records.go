// Package records — сервис записей: записи и их системные поля, значения
// полей, участия, группы, корзина. Все правила берутся из шаблона пака;
// ключей kin.core код не знает (F-52, AGENTS.md).
//
// Работа идёт в транзакции одного дашборда (Tx): доступ проверяет
// вызывающий через internal/authz, журнал пишется в той же транзакции.
package records

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kintion/server/internal/audit"
	"kintion/server/internal/authz"
	"kintion/server/internal/dashboards"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/packs"
	"kintion/server/internal/partialdate"
)

// Ошибки сервиса.
var (
	// ErrNotFound — записи нет или она не видна (чужая, шаблон неактивен — F-60).
	ErrNotFound = authz.ErrNotFound
	// ErrDeleted — запись в корзине: менять нельзя, только восстановить или очистить.
	ErrDeleted = errors.New("запись в корзине")
	// ErrNotDeleted — восстановить или очистить можно только запись из корзины.
	ErrNotDeleted = errors.New("запись не в корзине")
)

// ValidationError — ошибка проверки одного поля запроса (F-15). Field — путь
// к полю в запросе или ключ поля записи.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// ConflictError — поле изменил кто-то другой (F-45): if_rev не совпал.
// Current — текущее состояние поля.
type ConflictError struct {
	Field   string
	Current any
}

func (e *ConflictError) Error() string { return "конфликт версии поля " + e.Field }

// Ключи версий системных полей записи в field_rev (план §4).
const (
	RevStart     = "start"
	RevEnd       = "end"
	RevNote      = "note"
	RevTitle     = "title"
	RevSensitive = "sensitive"
	RevGroups    = "groups"
	RevCustom    = "custom_fields"
	revRoleStart = "participants:"
)

// RevParticipants — ключ версии участников роли.
func RevParticipants(role string) string { return revRoleStart + role }

func newID() uuid.UUID { return uuid.Must(uuid.NewV7()) }

// Service — сервис записей поверх реестра паков.
type Service struct {
	Reg *packs.Registry
}

// New создаёт сервис.
func New(reg *packs.Registry) *Service { return &Service{Reg: reg} }

// Tmpl — действующий шаблон дашборда: строка dashboard_template, шаблон пака
// с правками (F-36, F-58) и его сущность.
type Tmpl struct {
	Row    sqlc.DashboardTemplate
	T      *packs.Template
	Entity *packs.Entity
	// Shared — общие свои поля шаблона в дашборде (F-96): overrides.custom_fields.
	Shared []packs.Field
}

// Rec — видимая запись с её шаблоном.
type Rec struct {
	Row sqlc.Entity
	*Tmpl
}

// Tx — работа с записями одного дашборда в одной транзакции.
type Tx struct {
	s         *Service
	ctx       context.Context
	q         *sqlc.Queries
	op        *audit.Operation
	Dashboard uuid.UUID
	tmpls     map[uuid.UUID]*Tmpl  // nil — шаблон неактивен или не загружен (F-60)
	eraOrder  partialdate.EraOrder // порядок эр дашборда для сортировки по датам (F-143)
	// эры дашборда для проверки пределов года (checkEraYear), читаются по требованию
	eras       dashboards.Eras
	erasLoaded bool
	// scope — записи, видимые гостю (F-05), groups — видимые гостю группы
	// (выдачи и вложенные, F-105, без его технической группы); nil — смотрит владелец
	scope  map[uuid.UUID]bool
	groups map[uuid.UUID]bool
	// гость-редактор (F-04): editScope — записи с меткой группы, где у гостя
	// право правки; editGroups — показанные гостю группы с правом правки;
	// guest — сам гость, ownGroup — его техническая группа (F-149), найденная в
	// этой транзакции (ownGroupID)
	editScope  map[uuid.UUID]bool
	editGroups map[uuid.UUID]bool
	guest      uuid.UUID
	ownGroup   *uuid.UUID
}

// ErrForbidden — запись видна, но изменять её смотрящему нельзя (гость без
// права правки на её группы, F-04). Невидимое — по-прежнему ErrNotFound.
var ErrForbidden = errors.New("нет права на изменение")

// Begin начинает работу с дашбордом в транзакции tx. op может быть nil для чтения.
func (s *Service) Begin(ctx context.Context, tx pgx.Tx, op *audit.Operation, dashboard uuid.UUID) (*Tx, error) {
	return s.begin(ctx, sqlc.New(tx), op, dashboard)
}

// Read — только чтение (без транзакции).
func (s *Service) Read(ctx context.Context, q *sqlc.Queries, dashboard uuid.UUID) (*Tx, error) {
	return s.begin(ctx, q, nil, dashboard)
}

// ReadAs — чтение глазами смотрящего: гостю видны только записи из групп с
// выдачей и вложенных в них (F-05); всё остальное ведёт себя как несуществующее (F-07).
func (s *Service) ReadAs(ctx context.Context, q *sqlc.Queries, acc authz.Access) (*Tx, error) {
	return s.as(ctx, q, nil, acc)
}

// BeginAs — изменение глазами смотрящего в транзакции tx: гость видит и
// меняет только своё по правам (F-04, F-05).
func (s *Service) BeginAs(ctx context.Context, tx pgx.Tx, op *audit.Operation, acc authz.Access) (*Tx, error) {
	return s.as(ctx, sqlc.New(tx), op, acc)
}

func (s *Service) as(ctx context.Context, q *sqlc.Queries, op *audit.Operation, acc authz.Access) (*Tx, error) {
	t, err := s.begin(ctx, q, op, acc.Dashboard.ID)
	if err != nil || acc.Owner {
		return t, err
	}
	t.guest = acc.Account
	t.groups, t.editGroups = map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
	var all, edit []uuid.UUID
	for id, canEdit := range acc.Groups {
		all = append(all, id)
		if canEdit {
			edit = append(edit, id)
		}
		if acc.Own[id] {
			continue // своя техническая группа гостю не показывается (F-149)
		}
		t.groups[id] = true
		if canEdit {
			t.editGroups[id] = true
		}
	}
	t.scope, t.editScope = map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
	for _, x := range []struct {
		ids []uuid.UUID
		out map[uuid.UUID]bool
	}{{all, t.scope}, {edit, t.editScope}} {
		if len(x.ids) == 0 {
			continue
		}
		ents, err := q.EntitiesInGroups(ctx, sqlc.EntitiesInGroupsParams{DashboardID: acc.Dashboard.ID, GroupIds: x.ids})
		if err != nil {
			return nil, err
		}
		for _, id := range ents {
			x.out[id] = true
		}
	}
	return t, nil
}

// CanEdit — смотрящий может изменять запись: владелец — любую, гость — с меткой
// группы, где у него право правки (F-04).
func (t *Tx) CanEdit(id uuid.UUID) bool { return t.editScope == nil || t.editScope[id] }

// GroupEditable — смотрящий может ставить и снимать метку группы.
func (t *Tx) GroupEditable(id uuid.UUID) bool { return t.editGroups == nil || t.editGroups[id] }

// Guest — смотрит гость: видит часть дашборда.
func (t *Tx) Guest() bool { return t.scope != nil }

// inScope — запись видна смотрящему по группам (владельцу — любая).
func (t *Tx) inScope(id uuid.UUID) bool { return t.scope == nil || t.scope[id] }

// GroupVisible — группа видна смотрящему (владельцу — любая).
func (t *Tx) GroupVisible(id uuid.UUID) bool { return t.groups == nil || t.groups[id] }

func (s *Service) begin(ctx context.Context, q *sqlc.Queries, op *audit.Operation, dashboard uuid.UUID) (*Tx, error) {
	t := &Tx{s: s, ctx: ctx, q: q, op: op, Dashboard: dashboard, tmpls: map[uuid.UUID]*Tmpl{}}
	rows, err := q.ListDashboardTemplates(ctx, dashboard)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		tm, err := s.resolve(r)
		if err != nil {
			return nil, err
		}
		t.tmpls[r.ID] = tm
	}
	if t.eraOrder, err = dashboards.OrderOf(ctx, q, dashboard); err != nil {
		return nil, err
	}
	return t, nil
}

// resolve — действующее определение шаблона дашборда или nil, если шаблон
// неактивен, не загружен или его сущность не загружена (F-60).
func (s *Service) resolve(r sqlc.DashboardTemplate) (*Tmpl, error) {
	t, ok := s.Reg.Template(r.TemplateKey)
	if !r.Active || !ok || t.Pack != r.PackID {
		return nil, nil
	}
	e, ok := s.Reg.Entity(t.Entity)
	if !ok {
		return nil, nil
	}
	shared, err := sharedDefs(r.Overrides)
	if err != nil {
		return nil, fmt.Errorf("шаблон %s: %w", r.TemplateKey, err)
	}
	if len(r.Overrides) > 0 && string(r.Overrides) != "{}" {
		def, err := dashboards.Effective(t.Raw, r.Overrides)
		if err != nil {
			return nil, fmt.Errorf("шаблон %s: %w", r.TemplateKey, err)
		}
		var eff packs.Template
		if err := json.Unmarshal(def, &eff); err != nil {
			return nil, fmt.Errorf("шаблон %s: %w", r.TemplateKey, err)
		}
		eff.Raw, eff.Pack = def, t.Pack
		t = &eff
	}
	return &Tmpl{Row: r, T: t, Entity: e, Shared: shared}, nil
}

// visible — запись видна: не в корзине и на действующем шаблоне.
func (t *Tx) visible(row sqlc.Entity) bool {
	return row.DeletedAt == nil && t.tmpls[row.TemplateID] != nil && t.inScope(row.ID)
}

// Record — запись с шаблоном; запись в корзине тоже (её видит владелец),
// запись неактивного шаблона или невидимая гостю — ErrNotFound (F-60, F-05).
func (t *Tx) Record(row sqlc.Entity) (*Rec, error) {
	if row.DashboardID != t.Dashboard || !t.inScope(row.ID) || (t.Guest() && row.DeletedAt != nil) {
		return nil, ErrNotFound
	}
	tm := t.tmpls[row.TemplateID]
	if tm == nil {
		return nil, ErrNotFound
	}
	return &Rec{Row: row, Tmpl: tm}, nil
}

// live — запись для изменения: не в корзине.
func live(r *Rec) error {
	if r.Row.DeletedAt != nil {
		return ErrDeleted
	}
	return nil
}

// visibleRows — видимые записи дашборда из ids.
func (t *Tx) visibleRows(ids []uuid.UUID) (map[uuid.UUID]sqlc.Entity, error) {
	out := map[uuid.UUID]sqlc.Entity{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := t.q.ListEntitiesByIDs(t.ctx, sqlc.ListEntitiesByIDsParams{DashboardID: t.Dashboard, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if t.visible(r) {
			out[r.ID] = r
		}
	}
	return out, nil
}

// FileVisible — файл sha — значение поля типа image или file видимой
// смотрящему записи (F-30, F-05): текст, совпавший с хешем, доступа не даёт.
func (t *Tx) FileVisible(sha string) (bool, error) {
	used, err := t.q.FileUsedBy(t.ctx, sqlc.FileUsedByParams{DashboardID: t.Dashboard, VText: &sha})
	if err != nil {
		return false, err
	}
	ids := make([]uuid.UUID, 0, len(used))
	for _, u := range used {
		ids = append(ids, u.EntityID)
	}
	rows, err := t.visibleRows(ids)
	if err != nil {
		return false, err
	}
	for _, u := range used {
		row, ok := rows[u.EntityID]
		if !ok {
			continue
		}
		r, err := t.Record(row)
		if err != nil {
			continue
		}
		d, err := t.fieldDef(r, u.FieldKey)
		if err == nil && (d.Type == packs.TypeImage || d.Type == packs.TypeFile) {
			return true, nil
		}
	}
	return false, nil
}

// target — видимая запись дашборда по id (цель ссылки, привязки, участник).
func (t *Tx) target(id uuid.UUID, field string) (sqlc.Entity, error) {
	rows, err := t.visibleRows([]uuid.UUID{id})
	if err != nil {
		return sqlc.Entity{}, err
	}
	row, ok := rows[id]
	if !ok {
		return sqlc.Entity{}, invalid(field, "запись не найдена")
	}
	return row, nil
}

// anchorTarget — цель привязки даты или значения: запись сущности с
// `"anchor": true` (F-83; какая это сущность, знает только пак).
func (t *Tx) anchorTarget(id uuid.UUID, field string) error {
	row, err := t.target(id, field)
	if err != nil {
		return err
	}
	if tm := t.tmpls[row.TemplateID]; tm == nil || !tm.Entity.Anchor {
		return invalid(field, "к этой записи нельзя привязать дату")
	}
	return nil
}

// refTarget — цель поля-ссылки (F-82): сущность из `entities` поля, а
// запись-контейнер — только шаблона из `contains` шаблона записи.
func (t *Tx) refTarget(r *Rec, d fieldDef, id uuid.UUID) error {
	row, err := t.target(id, d.Key)
	if err != nil {
		return err
	}
	tm := t.tmpls[row.TemplateID]
	if tm == nil {
		return invalid(d.Key, "запись не найдена")
	}
	if len(d.Entities) > 0 && !slices.Contains(d.Entities, tm.Entity.ID) {
		return invalid(d.Key, "ссылка на запись этой сущности недопустима")
	}
	if tm.Entity.Container && r.T.Contains != nil && !slices.Contains(r.T.Contains, tm.T.ID) {
		return invalid(d.Key, "шаблон записи не может содержать %q", tm.T.Name)
	}
	return nil
}

// lockRev блокирует версию поля и сверяет её с ifRev (F-45). current
// вызывается при несовпадении — текущее состояние поля для ответа 409.
func (t *Tx) lockRev(entity uuid.UUID, key string, ifRev *int64, current func() (any, error)) error {
	if ifRev == nil {
		return invalid("if_rev", "нужна версия поля")
	}
	rev, err := t.q.LockFieldRev(t.ctx, sqlc.LockFieldRevParams{EntityID: entity, FieldKey: key})
	if err != nil {
		return err
	}
	if rev != *ifRev {
		cur, err := current()
		if err != nil {
			return err
		}
		return &ConflictError{Field: key, Current: cur}
	}
	return nil
}

func (t *Tx) bumpRev(entity uuid.UUID, key string) (int64, error) {
	if err := t.q.TouchEntity(t.ctx, entity); err != nil {
		return 0, err
	}
	return t.q.BumpFieldRev(t.ctx, sqlc.BumpFieldRevParams{EntityID: entity, FieldKey: key})
}

func (t *Tx) rev(entity uuid.UUID, key string) (int64, error) {
	revs, err := t.q.ListFieldRevsByEntities(t.ctx, []uuid.UUID{entity})
	if err != nil {
		return 0, err
	}
	for _, r := range revs {
		if r.FieldKey == key {
			return r.Rev, nil
		}
	}
	return 0, nil
}

func (t *Tx) audit(e audit.Entry) error {
	e.Dashboard = &t.Dashboard
	return t.op.Write(t.ctx, t.q, e)
}

// normText — пустой текст не хранится (F-42): nil вместо "".
func normText(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	v := *s
	return &v
}
