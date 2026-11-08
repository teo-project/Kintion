package records

import (
	"strconv"

	"kintion/server/internal/dashboards"
	"kintion/server/internal/partialdate"
)

// checkEraYear — год новой или изменённой даты в пределах своей эры из
// настроек дашборда (F-142, F-143: минимальный и максимальный год эры;
// min > max — обратный счёт, пределы те же). Дата без изменений не
// проверяется: правка другой части записи не зависит от старых дат вне
// пределов. Пустая эра — по пределам эры по умолчанию (новой дате её
// ставит триггер базы; так же считает и форма — одно простое правило).
func (t *Tx) checkEraYear(field string, d, old *partialdate.Date) error {
	if d == nil || d.Y == 0 || (old != nil && *old == *d) {
		return nil
	}
	if !t.erasLoaded {
		raw, err := t.q.GetDashboardEras(t.ctx, t.Dashboard)
		if err != nil {
			return err
		}
		if t.eras, err = dashboards.ParseEras(raw); err != nil {
			return err
		}
		t.erasLoaded = true
	}
	era := d.E
	if era == "" {
		era = t.eras.Default
	}
	y, ok := t.eras.Years[era]
	if !ok {
		return nil
	}
	lo, hi := y.Min, y.Max
	if lo != nil && hi != nil && *lo > *hi {
		lo, hi = hi, lo
	}
	if (lo != nil && d.Y < *lo) || (hi != nil && d.Y > *hi) {
		bound := func(p *int) string {
			if p == nil {
				return "…"
			}
			return strconv.Itoa(*p)
		}
		return invalid(field, "год %d вне пределов эры: %s … %s", d.Y, bound(lo), bound(hi))
	}
	return nil
}
