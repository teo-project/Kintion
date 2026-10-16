package records

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kintion/server/internal/audit"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/packs"
)

// SharedKey — ключ общих своих полей в overrides шаблона дашборда (F-96).
const SharedKey = "custom_fields"

// sharedDefs — общие свои поля из overrides шаблона дашборда.
func sharedDefs(overrides []byte) ([]packs.Field, error) {
	var ov map[string]json.RawMessage
	if len(overrides) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(overrides, &ov); err != nil {
		return nil, err
	}
	return customDefs(ov[SharedKey])
}

// SharedPatch — описания общих своих полей шаблона дашборда целиком (F-96):
// «добавить всем записям этого шаблона». Версия — rev шаблона (F-45).
type SharedPatch struct {
	IfRev        *int64            `json:"if_rev"`
	CustomFields []json.RawMessage `json:"custom_fields"`
}

// sharedSnapshot — общие поля шаблона в журнале.
type sharedSnapshot struct {
	CustomFields []json.RawMessage `json:"custom_fields"`
}

// PatchShared заменяет общие свои поля шаблона дашборда. Те же правила, что
// у своих полей записи: поле со значениями (у любой записи шаблона в дашборде,
// в том числе в корзине) нельзя убрать и сменить ему тип. В 0.1 дашборд и все
// его записи видит только владелец — ответ не зависит от невидимого (F-07).
func (t *Tx) PatchShared(tmplID uuid.UUID, p SharedPatch) (sqlc.DashboardTemplate, error) {
	const field = SharedKey
	tm := t.tmpls[tmplID]
	if tm == nil {
		return sqlc.DashboardTemplate{}, ErrNotFound
	}
	if p.IfRev == nil {
		return sqlc.DashboardTemplate{}, invalid("if_rev", "нужна версия шаблона")
	}
	row, err := t.q.LockDashboardTemplate(t.ctx, sqlc.LockDashboardTemplateParams{DashboardID: t.Dashboard, ID: tmplID})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.DashboardTemplate{}, ErrNotFound
	}
	if err != nil {
		return sqlc.DashboardTemplate{}, err
	}
	old, err := sharedDefs(row.Overrides)
	if err != nil {
		return sqlc.DashboardTemplate{}, err
	}
	if row.Rev != *p.IfRev {
		return sqlc.DashboardTemplate{}, &ConflictError{Field: field, Current: map[string]any{"rev": row.Rev, SharedKey: old}}
	}
	seen, err := t.checkCustomList(field, p.CustomFields)
	if err != nil {
		return sqlc.DashboardTemplate{}, err
	}
	used, err := t.q.ListFieldKeysByTemplate(t.ctx, sqlc.ListFieldKeysByTemplateParams{
		DashboardID: t.Dashboard, TemplateID: tmplID, Prefix: packs.SharedPrefix,
	})
	if err != nil {
		return sqlc.DashboardTemplate{}, err
	}
	for _, o := range old {
		if !slices.Contains(used, packs.SharedPrefix+o.Key) {
			continue
		}
		nf, ok := seen[o.Key]
		if !ok || nf.Type != o.Type {
			return sqlc.DashboardTemplate{}, invalid(field, "у поля %q есть значения: сначала удалите их", o.Key)
		}
		if nf.Type == packs.TypeChoice {
			vals, err := t.q.ListFieldTextsByTemplate(t.ctx, sqlc.ListFieldTextsByTemplateParams{
				DashboardID: t.Dashboard, TemplateID: tmplID, FieldKey: packs.SharedPrefix + o.Key,
			})
			if err != nil {
				return sqlc.DashboardTemplate{}, err
			}
			used := make([]usedValue, len(vals))
			for i, v := range vals {
				used[i] = usedValue{Text: v.Text, Free: v.Free}
			}
			if err := usedOptions(field, nf, used); err != nil {
				return sqlc.DashboardTemplate{}, err
			}
		}
	}

	ov := map[string]json.RawMessage{}
	if len(row.Overrides) > 0 {
		if err := json.Unmarshal(row.Overrides, &ov); err != nil {
			return sqlc.DashboardTemplate{}, err
		}
	}
	before := sharedSnapshot{CustomFields: rawList(ov[SharedKey])}
	list := p.CustomFields
	if list == nil {
		list = []json.RawMessage{}
	}
	if len(list) == 0 {
		delete(ov, SharedKey)
	} else {
		b, err := json.Marshal(list)
		if err != nil {
			return sqlc.DashboardTemplate{}, err
		}
		ov[SharedKey] = b
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
		Before: before, After: sharedSnapshot{CustomFields: list},
	})
}

func rawList(raw json.RawMessage) []json.RawMessage {
	out := []json.RawMessage{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}
