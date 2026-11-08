package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kintion/server/internal/audit"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/partialdate"
)

// Eras — эры в настройках дашборда (F-143).
type Eras struct {
	// Show — показывать эру у дат.
	Show bool `json:"show"`
	// Edit — менять эру в формах; нет — даты сохраняются с эрой по умолчанию.
	Edit bool `json:"edit"`
	// Default — эра по умолчанию; пустая — наша. Новым датам с годом и пустой
	// эрой её ставит база (триггер pd_apply_default_era, миграция 0012); при
	// непустой эре по умолчанию пустая эра — «без эры» (F-14).
	Default string `json:"default"`
	// Order — хронологический порядок эр; пустая строка — наша эра (при
	// непустой эре по умолчанию — «без эры»). Эры не из
	// списка идут первыми (новая эра — наверх), см. partialdate.EraKey.
	Order []string `json:"order"`
	// Years — необязательные минимальный и максимальный год эры (ключ — эра,
	// пустая — наша); минимальный больше максимального — счёт обратный.
	Years map[string]EraYears `json:"years"`
	// Today — текущий год мира (год и эра); nil — настоящая дата в нашей эре.
	Today *partialdate.Date `json:"today"`
	// Rev — версия настроек (F-45): правка с другой версией — конфликт.
	Rev int64 `json:"rev"`
}

// EraYears — минимальный и максимальный год эры; nil — не задан.
type EraYears struct {
	Min *int `json:"min,omitempty"`
	Max *int `json:"max,omitempty"`
}

// Reversed — счёт в эре обратный: минимальный год больше максимального
// (15000 … 1, F-143).
func (y EraYears) Reversed() bool { return y.Min != nil && y.Max != nil && *y.Min > *y.Max }

// MaxEras — наибольшее число эр в списке порядка.
const MaxEras = 200

// ErrEras — неверные настройки эр.
var ErrEras = errors.New("эры: неверные настройки")

// ErrErasConflict — эры изменены в другом окне: версия не совпала (F-45).
var ErrErasConflict = errors.New("эры изменены в другом окне")

// eraError — ошибка проверки с полем.
type eraError struct{ field, msg string }

func (e *eraError) Error() string { return e.field + ": " + e.msg }
func (e *eraError) Unwrap() error { return ErrEras }

// EraField — поле запроса, к которому относится ошибка ErrEras.
func EraField(err error) (string, string) {
	var ee *eraError
	if errors.As(err, &ee) {
		return ee.field, ee.msg
	}
	return "", err.Error()
}

func validEraOrEmpty(e string) bool { return e == "" || partialdate.ValidEra(e) }

// Validate проверяет настройки.
func (e Eras) Validate() error {
	if !validEraOrEmpty(e.Default) {
		return &eraError{"default", "эра: до 64 символов, без пробелов по краям"}
	}
	if len(e.Order) > MaxEras {
		return &eraError{"order", fmt.Sprintf("не больше %d эр", MaxEras)}
	}
	for i, o := range e.Order {
		if !validEraOrEmpty(o) || slices.Contains(e.Order[:i], o) {
			return &eraError{"order", "эры в списке — разные, до 64 символов"}
		}
	}
	if len(e.Years) > MaxEras {
		return &eraError{"years", fmt.Sprintf("не больше %d эр", MaxEras)}
	}
	for k, y := range e.Years {
		if !validEraOrEmpty(k) || !validYear(y.Min) || !validYear(y.Max) {
			return &eraError{"years", "год эры — целое со знаком, без 0"}
		}
	}
	if t := e.Today; t != nil {
		if t.Y == 0 || t.M != 0 || t.D != 0 || t.Validate() != nil {
			return &eraError{"today", "год мира — год и эра"}
		}
	}
	return nil
}

func validYear(y *int) bool {
	return y == nil || (*y != 0 && *y >= math.MinInt32 && *y <= math.MaxInt32)
}

// EraOrder — порядок эр для сравнения дат: список и эры с обратным счётом.
func (e Eras) EraOrder() partialdate.EraOrder {
	o := partialdate.EraOrder{List: e.Order}
	for k, y := range e.Years {
		if y.Reversed() {
			o.Reversed = append(o.Reversed, k)
		}
	}
	slices.Sort(o.Reversed)
	return o
}

// ParseEras читает настройки из колонки; пустой объект — по умолчанию.
func ParseEras(raw []byte) (Eras, error) {
	var e Eras
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &e); err != nil {
			return Eras{}, fmt.Errorf("эры дашборда: %w", err)
		}
	}
	if e.Order == nil {
		e.Order = []string{}
	}
	if e.Years == nil {
		e.Years = map[string]EraYears{}
	}
	return e, nil
}

// ErasDTO — эры дашборда для фронтенда: настройки и список эр в
// хронологическом порядке (эры из дат не из списка порядка — наверху).
type ErasDTO struct {
	Eras
	// List — действующий порядок: эры, которые есть в датах дашборда (видимых,
	// F-07), и эра по умолчанию; наша (пустая) — только если используется или
	// выбрана по умолчанию (F-143).
	List []string `json:"list"`
}

// EffectiveOrder — действующий порядок эр used: те, которых нет в order,
// сверху по тексту (как partialdate.EraKey), затем по order; эры order, которых
// нет в used, не показываются.
func EffectiveOrder(order, used []string) []string {
	var top, rest []string
	for _, u := range used {
		if !slices.Contains(order, u) && !slices.Contains(top, u) {
			top = append(top, u)
		}
	}
	slices.Sort(top)
	for _, o := range order {
		if slices.Contains(used, o) {
			rest = append(rest, o)
		}
	}
	return append(top, rest...)
}

// LoadEras — настройки и действующий список эр дашборда.
func LoadEras(ctx context.Context, q *sqlc.Queries, dashboard uuid.UUID) (ErasDTO, error) {
	raw, err := q.GetDashboardEras(ctx, dashboard)
	if err != nil {
		return ErasDTO{}, err
	}
	e, err := ParseEras(raw)
	if err != nil {
		return ErasDTO{}, err
	}
	used, err := q.ListUsedEras(ctx, dashboard)
	if err != nil {
		return ErasDTO{}, err
	}
	used = append(used, e.Default)
	return ErasDTO{Eras: e, List: EffectiveOrder(e.Order, used)}, nil
}

// OrderOf — порядок эр дашборда для сортировки (F-143).
func OrderOf(ctx context.Context, q *sqlc.Queries, dashboard uuid.UUID) (partialdate.EraOrder, error) {
	raw, err := q.GetDashboardEras(ctx, dashboard)
	if errors.Is(err, pgx.ErrNoRows) {
		return partialdate.EraOrder{}, nil
	}
	if err != nil {
		return partialdate.EraOrder{}, err
	}
	e, err := ParseEras(raw)
	return e.EraOrder(), err
}

// erasSnapshot — эры в журнале: настройки (запись правки настроек) или замена
// эры в датах (отдельная запись на каждую замену).
type erasSnapshot struct {
	Eras    *Eras        `json:"eras,omitempty"`
	Replace *eraReplaced `json:"replace,omitempty"`
}

// eraReplaced — замена эры в датах: что на что, сколько видимых дат изменено
// (F-07) и эра по умолчанию до и после правки — по ней пустая эра в журнале
// подписывается «н. э.» или «без эры» (F-143).
type eraReplaced struct {
	From          string `json:"from"`
	To            string `json:"to"`
	Changed       int64  `json:"changed"`
	DefaultBefore string `json:"default_before"`
	DefaultAfter  string `json:"default_after"`
}

// Replace — замена эры From на To во всех датах дашборда («к уже созданным»
// при смене эры по умолчанию, переименование эры в списке).
type Replace struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func eraPtr(e string) *string {
	if e == "" {
		return nil
	}
	return &e
}

// renameInOrder — в списке порядка from заменяется на to; если to уже есть —
// from просто убирается.
func renameInOrder(order []string, from, to string) []string {
	out := make([]string, 0, len(order))
	has := slices.Contains(order, to)
	for _, o := range order {
		switch {
		case o == from && has:
		case o == from:
			out = append(out, to)
		default:
			out = append(out, o)
		}
	}
	return out
}

// applyReplace — замена в настройках: список порядка, годы эр, эра по
// умолчанию, год мира.
func applyReplace(next *Eras, from, to string) {
	next.Order = renameInOrder(next.Order, from, to)
	if y, ok := next.Years[from]; ok {
		delete(next.Years, from)
		if _, has := next.Years[to]; !has {
			next.Years[to] = y
		}
	}
	if next.Default == from {
		next.Default = to
	}
	if next.Today != nil && next.Today.E == from {
		t := *next.Today
		t.E = to
		next.Today = &t
	}
}

func sameToday(a, b *partialdate.Date) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// SaveEras сохраняет настройки с версией ifRev (F-45; не та — ErrErasConflict).
// replaces — замены эры во всех датах дашборда по порядку («к уже созданным»
// при смене эры по умолчанию, переименование эры): эра From меняется на To
// (даты с другими эрами не трогаются) в датах, списке порядка, годах эр, эре
// по умолчанию и годе мира. Дата из одной эры при замене на пустую очищается
// (F-143). Журнал: запись правки настроек и по записи на каждую замену с
// числом изменённых дат.
//
// Эру по умолчанию новым датам ставит база (триггер, миграция 0012); смена
// эры по умолчанию без замены («только к новым») старые даты не меняет:
// пустые остаются пустыми («без эры»).
func SaveEras(ctx context.Context, tx pgx.Tx, op *audit.Operation, d sqlc.Dashboard, ifRev int64, next Eras, replaces []Replace) (sqlc.Dashboard, error) {
	q := sqlc.New(tx)
	raw, err := q.LockDashboardEras(ctx, d.ID)
	if err != nil {
		return sqlc.Dashboard{}, err
	}
	prev, err := ParseEras(raw)
	if err != nil {
		return sqlc.Dashboard{}, err
	}
	if ifRev != prev.Rev {
		return sqlc.Dashboard{}, ErrErasConflict
	}
	if len(replaces) > MaxEras {
		return sqlc.Dashboard{}, &eraError{"replace", fmt.Sprintf("не больше %d замен", MaxEras)}
	}
	next.Rev = prev.Rev + 1
	if next.Order == nil {
		next.Order = []string{}
	}
	next.Years = maps.Clone(next.Years)
	if next.Years == nil {
		next.Years = map[string]EraYears{}
	}
	var todo []Replace
	todayReplaced := false
	for _, r := range replaces {
		if !validEraOrEmpty(r.From) || !validEraOrEmpty(r.To) {
			return sqlc.Dashboard{}, &eraError{"to", "эра: до 64 символов, без пробелов по краям"}
		}
		if r.From == r.To {
			continue
		}
		if next.Today != nil && next.Today.E == r.From {
			todayReplaced = true
		}
		applyReplace(&next, r.From, r.To)
		todo = append(todo, r)
	}
	// новый год мира с пустой эрой — в эре по умолчанию, как новые даты
	if next.Today != nil && next.Today.E == "" && next.Default != "" && !todayReplaced && !sameToday(prev.Today, next.Today) {
		t := *next.Today
		t.E = next.Default
		next.Today = &t
	}
	if err := next.Validate(); err != nil {
		return sqlc.Dashboard{}, err
	}
	out, err := json.Marshal(next)
	if err != nil {
		return sqlc.Dashboard{}, err
	}
	nd, err := q.SetDashboardEras(ctx, sqlc.SetDashboardErasParams{ID: d.ID, Eras: out})
	if err != nil {
		return sqlc.Dashboard{}, err
	}
	if err := op.Write(ctx, q, audit.Entry{
		Dashboard: &d.ID, Object: "dashboard", ObjectID: &d.ID, Op: audit.OpUpdate,
		Before: erasSnapshot{Eras: &prev}, After: erasSnapshot{Eras: &next},
	}); err != nil {
		return sqlc.Dashboard{}, err
	}
	for _, r := range todo {
		// замена и версии изменённых полей — одним запросом (F-45)
		rep, err := q.ReplaceEra(ctx, sqlc.ReplaceEraParams{DashboardID: d.ID, OldEra: eraPtr(r.From), NewEra: eraPtr(r.To)})
		if err != nil {
			return sqlc.Dashboard{}, err
		}
		if err := q.TouchEntities(ctx, rep.Touched); err != nil {
			return sqlc.Dashboard{}, err
		}
		n := rep.Changed
		if err := op.Write(ctx, q, audit.Entry{
			Dashboard: &d.ID, Object: "dashboard", ObjectID: &d.ID, Op: audit.OpUpdate,
			After: erasSnapshot{Replace: &eraReplaced{From: r.From, To: r.To, Changed: n, DefaultBefore: prev.Default, DefaultAfter: next.Default}},
		}); err != nil {
			return sqlc.Dashboard{}, err
		}
	}
	return nd, nil
}
