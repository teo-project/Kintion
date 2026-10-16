package records

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"kintion/server/internal/audit"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/packs"
)

// Автодобавление в группу по фамилии (F-122). Какое поле считать фамилией —
// решает пак (packs.Field.Surname), код имён полей не знает. Сравниваются все
// значения поля из истории, без учёта регистра. Из группы автоматически не
// убирают; метка ставится так же, как вручную (журнал, версия groups).

const (
	maxSurnames   = 200
	maxSurnameLen = 200
)

// surnameKey — ключ сравнения фамилии: без пробелов по краям и регистра.
func surnameKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// cleanSurnames — список фамилий группы: обрезка, без пустых и повторов (по ключу сравнения).
func cleanSurnames(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[surnameKey(s)] {
			continue
		}
		if utf8.RuneCountInString(s) > maxSurnameLen {
			return nil, invalid(keySurnames, "фамилия: не длиннее %d символов", maxSurnameLen)
		}
		seen[surnameKey(s)] = true
		out = append(out, s)
	}
	if len(out) > maxSurnames {
		return nil, invalid(keySurnames, "не больше %d фамилий", maxSurnames)
	}
	return out, nil
}

// surnameFields — ключи полей-фамилий шаблона.
func surnameFields(tm *packs.Template) []string {
	if tm == nil {
		return nil
	}
	var keys []string
	for _, f := range tm.Fields {
		if f.Surname {
			keys = append(keys, f.Key)
		}
	}
	return keys
}

func matchesSurnames(g sqlc.Grp, have map[string]bool) bool {
	return slices.ContainsFunc(g.Surnames, func(s string) bool { return have[surnameKey(s)] })
}

// labelIfMissing ставит метку группы записи, если её нет: как SetGroupMembers —
// версия groups под блокировкой, запись в журнал, новая версия.
func (t *Tx) labelIfMissing(entity, group uuid.UUID) error {
	if _, err := t.q.LockFieldRev(t.ctx, sqlc.LockFieldRevParams{EntityID: entity, FieldKey: RevGroups}); err != nil {
		return err
	}
	cur, err := t.q.ListGroupsByEntities(t.ctx, []uuid.UUID{entity})
	if err != nil {
		return err
	}
	if slices.ContainsFunc(cur, func(c sqlc.ListGroupsByEntitiesRow) bool { return c.GroupID == group }) {
		return nil
	}
	if err := t.q.InsertEntityGroup(t.ctx, sqlc.InsertEntityGroupParams{DashboardID: t.Dashboard, EntityID: entity, GroupID: group, CreatedBy: t.actor()}); err != nil {
		return err
	}
	if err := t.auditLabel(audit.OpCreate, entity, group); err != nil {
		return err
	}
	_, err = t.bumpRev(entity, RevGroups)
	return err
}

// autoGroups — при сохранении записи: добавляет её в группы, чей список фамилий
// совпал с одной из её фамилий.
func (t *Tx) autoGroups(r *Rec) error {
	keys := surnameFields(r.T)
	if len(keys) == 0 || r.Row.DeletedAt != nil {
		return nil
	}
	groups, err := t.q.ListAutoSurnameGroups(t.ctx, t.Dashboard)
	if err != nil || len(groups) == 0 {
		return err
	}
	have := map[string]bool{}
	for _, k := range keys {
		list, err := t.q.ListFieldEntriesByKey(t.ctx, sqlc.ListFieldEntriesByKeyParams{EntityID: r.Row.ID, FieldKey: k})
		if err != nil {
			return err
		}
		for _, e := range list {
			if e.VText != nil {
				have[surnameKey(*e.VText)] = true
			}
		}
	}
	for _, g := range groups {
		if matchesSurnames(g, have) {
			if err := t.labelIfMissing(r.Row.ID, g.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// autoGroupExisting — при включении автодобавления и правке списка фамилий:
// в группу добавляются и уже существующие записи с такой фамилией.
func (t *Tx) autoGroupExisting(g sqlc.Grp) error {
	if !g.AutoSurname || len(g.Surnames) == 0 {
		return nil
	}
	have := map[uuid.UUID]map[string]bool{}
	for tid, tm := range t.tmpls {
		if tm == nil {
			continue
		}
		keys := surnameFields(tm.T)
		if len(keys) == 0 {
			continue
		}
		rows, err := t.q.ListSurnameValues(t.ctx, sqlc.ListSurnameValuesParams{DashboardID: t.Dashboard, TemplateID: tid, Keys: keys})
		if err != nil {
			return err
		}
		for _, v := range rows {
			if have[v.EntityID] == nil {
				have[v.EntityID] = map[string]bool{}
			}
			have[v.EntityID][surnameKey(*v.VText)] = true
		}
	}
	ids := make([]uuid.UUID, 0, len(have))
	for id, h := range have {
		if matchesSurnames(g, h) {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	for _, id := range ids {
		if err := t.labelIfMissing(id, g.ID); err != nil {
			return err
		}
	}
	return nil
}
