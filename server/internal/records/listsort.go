package records

import (
	"slices"
	"strings"

	"github.com/google/uuid"

	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/packs"
)

// sortKeys — ключи полей, по которым сортируется шаблон (F-133): sort_by пака;
// нет — поля из правила подписи (title), в его порядке.
func sortKeys(t *packs.Template) []string {
	if t == nil {
		return nil
	}
	if len(t.SortBy) > 0 {
		return t.SortBy
	}
	var keys []string
	for _, el := range t.Title {
		if el.Field == "" {
			continue
		}
		if _, ok := t.Field(el.Field); ok && !slices.Contains(keys, el.Field) {
			keys = append(keys, el.Field)
		}
	}
	return keys
}

// sortIDs упорядочивает записи по полям их шаблонов: текущее значение каждого
// поля без учёта регистра, пустое — в конец; запись с ручной подписью без
// sort_by — по ней. Дальше — время создания и id (стабильно).
func (t *Tx) sortIDs(ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) < 2 {
		return ids, nil
	}
	rows, err := t.q.ListEntitiesByIDs(t.ctx, sqlc.ListEntitiesByIDsParams{DashboardID: t.Dashboard, Ids: ids})
	if err != nil {
		return nil, err
	}
	rowOf := make(map[uuid.UUID]sqlc.Entity, len(rows))
	keysOf := map[uuid.UUID][]string{}
	var all []string
	for _, r := range rows {
		rowOf[r.ID] = r
		var ks []string
		if tm := t.tmpls[r.TemplateID]; tm != nil {
			ks = sortKeys(tm.T)
		}
		keysOf[r.ID] = ks
		for _, k := range ks {
			if !slices.Contains(all, k) {
				all = append(all, k)
			}
		}
	}
	cur := map[uuid.UUID]map[string]string{} // текущее (верхнее) значение поля
	if len(all) > 0 {
		vals, err := t.q.ListSortValues(t.ctx, sqlc.ListSortValuesParams{Ids: ids, Keys: all})
		if err != nil {
			return nil, err
		}
		for _, v := range vals {
			m := cur[v.EntityID]
			if m == nil {
				m = map[string]string{}
				cur[v.EntityID] = m
			}
			if _, seen := m[v.FieldKey]; !seen && v.VText != nil {
				m[v.FieldKey] = strings.ToLower(*v.VText)
			}
		}
	}
	tuple := make(map[uuid.UUID][]string, len(ids))
	for _, id := range ids {
		r := rowOf[id]
		var tp []string
		if r.TitleManual != nil && *r.TitleManual != "" && (tmSortBy(t, r) == nil) {
			tp = []string{strings.ToLower(*r.TitleManual)}
		} else {
			for _, k := range keysOf[id] {
				tp = append(tp, cur[id][k])
			}
		}
		tuple[id] = tp
	}
	out := slices.Clone(ids)
	slices.SortStableFunc(out, func(a, b uuid.UUID) int {
		if c := compareTuples(tuple[a], tuple[b]); c != 0 {
			return c
		}
		if c := rowOf[a].CreatedAt.Compare(rowOf[b].CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.String(), b.String())
	})
	return out, nil
}

// tmSortBy — sort_by шаблона записи (nil — нет, порядок по подписи).
func tmSortBy(t *Tx, r sqlc.Entity) []string {
	if tm := t.tmpls[r.TemplateID]; tm != nil && tm.T != nil {
		return tm.T.SortBy
	}
	return nil
}

// compareTuples сравнивает по элементам; пустой элемент — после непустого.
func compareTuples(a, b []string) int {
	for i := 0; i < max(len(a), len(b)); i++ {
		var x, y string
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x == y {
			continue
		}
		if x == "" {
			return 1
		}
		if y == "" {
			return -1
		}
		return strings.Compare(x, y)
	}
	return 0
}
