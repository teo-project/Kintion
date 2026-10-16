package records

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kintion/server/internal/audit"
	"kintion/server/internal/db/sqlc"
)

// QuickFieldsKey — ключ набора полей быстрой формы в overrides шаблона
// дашборда (F-124): значение заменяет `quick_fields` шаблона пака.
const QuickFieldsKey = "quick_fields"

// systemAddrs — адреса системных полей записи в quick_fields (pack-format.md).
var systemAddrs = []string{"start", "end", ".note"}

const (
	maxQuickFields = 50
	maxQuickAddr   = 100
)

// QuickPatch — набор полей быстрой формы шаблона в дашборде целиком (F-124);
// пусто или null — вернуть к умолчанию пака. Версия — rev шаблона (F-45).
type QuickPatch struct {
	IfRev       *int64   `json:"if_rev"`
	QuickFields []string `json:"quick_fields"`
}

type quickSnapshot struct {
	QuickFields []string `json:"quick_fields"`
}

func quickOf(overrides []byte) []string {
	var ov map[string]json.RawMessage
	_ = json.Unmarshal(overrides, &ov)
	var out []string
	_ = json.Unmarshal(ov[QuickFieldsKey], &out)
	return out
}

// PatchQuickFields меняет набор полей быстрой формы шаблона в дашборде.
// Адреса — как в `quick_fields` пака; форма сама отбрасывает неизвестные.
func (t *Tx) PatchQuickFields(tmplID uuid.UUID, p QuickPatch) (sqlc.DashboardTemplate, error) {
	const field = QuickFieldsKey
	if t.tmpls[tmplID] == nil {
		return sqlc.DashboardTemplate{}, ErrNotFound
	}
	if p.IfRev == nil {
		return sqlc.DashboardTemplate{}, invalid("if_rev", "нужна версия шаблона")
	}
	if len(p.QuickFields) > maxQuickFields {
		return sqlc.DashboardTemplate{}, invalid(field, "не больше %d полей", maxQuickFields)
	}
	tm := t.tmpls[tmplID].T
	seen := map[string]bool{}
	for _, a := range p.QuickFields {
		if strings.TrimSpace(a) == "" || utf8.RuneCountInString(a) > maxQuickAddr {
			return sqlc.DashboardTemplate{}, invalid(field, "неверный адрес поля")
		}
		if _, ok := tm.Field(a); !ok && !slices.Contains(systemAddrs, a) {
			return sqlc.DashboardTemplate{}, invalid(field, "нет такого поля: %q", a)
		}
		if seen[a] {
			return sqlc.DashboardTemplate{}, invalid(field, "поле %q указано дважды", a)
		}
		seen[a] = true
	}
	row, err := t.q.LockDashboardTemplate(t.ctx, sqlc.LockDashboardTemplateParams{DashboardID: t.Dashboard, ID: tmplID})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.DashboardTemplate{}, ErrNotFound
	}
	if err != nil {
		return sqlc.DashboardTemplate{}, err
	}
	if row.Rev != *p.IfRev {
		return sqlc.DashboardTemplate{}, &ConflictError{Field: field, Current: map[string]any{"rev": row.Rev, field: quickOf(row.Overrides)}}
	}
	ov := map[string]json.RawMessage{}
	if len(row.Overrides) > 0 {
		if err := json.Unmarshal(row.Overrides, &ov); err != nil {
			return sqlc.DashboardTemplate{}, err
		}
	}
	before := quickSnapshot{QuickFields: quickOf(row.Overrides)}
	if len(p.QuickFields) == 0 {
		delete(ov, QuickFieldsKey)
	} else {
		b, err := json.Marshal(p.QuickFields)
		if err != nil {
			return sqlc.DashboardTemplate{}, err
		}
		ov[QuickFieldsKey] = b
	}
	b, err := json.Marshal(ov)
	if err != nil {
		return sqlc.DashboardTemplate{}, err
	}
	nrow, err := t.q.UpdateDashboardTemplateOverrides(t.ctx, sqlc.UpdateDashboardTemplateOverridesParams{
		DashboardID: t.Dashboard, ID: tmplID, Overrides: b,
	})
	if err != nil {
		return sqlc.DashboardTemplate{}, err
	}
	ntm, err := t.s.resolve(nrow)
	if err != nil {
		return sqlc.DashboardTemplate{}, err
	}
	t.tmpls[tmplID] = ntm
	return nrow, t.op.Write(t.ctx, t.q, audit.Entry{
		Dashboard: &t.Dashboard, Object: "dashboard_template", ObjectID: &tmplID, Op: audit.OpUpdate,
		Before: before, After: quickSnapshot{QuickFields: p.QuickFields},
	})
}
