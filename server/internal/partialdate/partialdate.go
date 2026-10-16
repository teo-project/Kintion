// Package partialdate — частичная дата (F-14, docs/concepts/dates.md):
// год, месяц, день — каждая часть необязательна, хотя бы одна есть.
//
// Те же правила — в PostgreSQL (домен partial_date и функции pd_*) и во
// фронтенде (web/src/core/dates.ts); общие кейсы — testdata/partial_dates.json.
// Операторов «раньше / возможно раньше» нет (Q-07, В-13).
package partialdate

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Date — частичная дата. Нулевая часть означает «неизвестна»
// (у допустимой даты частей, равных нулю, не бывает).
type Date struct {
	Y int // год 1..9999
	M int // месяц 1..12
	D int // день 1..31
}

// Ошибки проверки.
var (
	ErrEmpty = errors.New("partial date: нужна хотя бы одна часть")
	ErrYear  = errors.New("partial date: год 1..9999")
	ErrMonth = errors.New("partial date: месяц 1..12")
	ErrDay   = errors.New("partial date: день 1..31")
	ErrNoDay = errors.New("partial date: такого дня в месяце нет")
)

// New проверяет части и возвращает дату.
func New(y, m, d int) (Date, error) {
	pd := Date{Y: y, M: m, D: d}
	return pd, pd.Validate()
}

// IsZero — нет ни одной части.
func (p Date) IsZero() bool { return p.Y == 0 && p.M == 0 && p.D == 0 }

// Validate проверяет допустимость: части в диапазонах, день есть в месяце
// (29.02 без года — да, с годом — только в високосный).
func (p Date) Validate() error {
	switch {
	case p.IsZero():
		return ErrEmpty
	case p.Y < 0 || p.Y > 9999:
		return ErrYear
	case p.M < 0 || p.M > 12:
		return ErrMonth
	case p.D < 0 || p.D > 31:
		return ErrDay
	case p.M != 0 && p.D > DaysIn(p.Y, p.M):
		return ErrNoDay
	}
	return nil
}

// DaysIn — число дней в месяце; год 0 — неизвестен (у февраля 29).
func DaysIn(y, m int) int {
	switch m {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if y == 0 || isLeap(y) {
			return 29
		}
		return 28
	}
	return 0
}

func isLeap(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

func date(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC) }

// Lo — нижняя граница; только при известном годе.
func (p Date) Lo() (time.Time, bool) {
	if p.Y == 0 {
		return time.Time{}, false
	}
	return date(p.Y, or(p.M, 1), or(p.D, 1)), true
}

// Hi — верхняя граница; только при известном годе.
func (p Date) Hi() (time.Time, bool) {
	if p.Y == 0 {
		return time.Time{}, false
	}
	m := or(p.M, 12)
	return date(p.Y, m, or(p.D, DaysIn(p.Y, m))), true
}

// Exact — точная дата, если известны все части.
func (p Date) Exact() (time.Time, bool) {
	if p.Y == 0 || p.M == 0 || p.D == 0 {
		return time.Time{}, false
	}
	return date(p.Y, p.M, p.D), true
}

// AgeAt — полных лет на дату at, считая от верхней границы (не меньше
// стольких). Без года и до нижней границы — нет значения. 29.02 в
// невисокосный год прибавляет год 28.02.
func (p Date) AgeAt(at time.Time) (int, bool) {
	lo, ok := p.Lo()
	at = dayOf(at)
	if !ok || at.Before(lo) {
		return 0, false
	}
	hi, _ := p.Hi()
	ay, am, ad := at.Date()
	hm, hd := int(hi.Month()), min(hi.Day(), DaysIn(ay, int(hi.Month())))
	age := ay - hi.Year()
	if int(am) < hm || (int(am) == hm && ad < hd) {
		age--
	}
	return max(age, 0), true
}

// NextAnniversary — ближайшая годовщина не раньше from (включительно);
// нужны месяц и день. Если год известен — не раньше самой даты.
// 29.02 в невисокосный год — 28.02.
func (p Date) NextAnniversary(from time.Time) (time.Time, bool) {
	if p.M == 0 || p.D == 0 {
		return time.Time{}, false
	}
	from = dayOf(from)
	for y := max(from.Year(), p.Y); ; y++ {
		c := date(y, p.M, min(p.D, DaysIn(y, p.M)))
		if !c.Before(from) {
			return c, true
		}
	}
}

// SortKey — ключ сортировки (Д-9): датированные — "0" + lo + hi (ГГГГММДД),
// без года — "1" + ММДД нижней + ММДД верхней границы (после датированных).
// Сравнивается побайтно (COLLATE "C").
func (p Date) SortKey() string {
	if lo, ok := p.Lo(); ok {
		hi, _ := p.Hi()
		return "0" + lo.Format("20060102") + hi.Format("20060102")
	}
	hm := or(p.M, 12)
	return fmt.Sprintf("1%02d%02d%02d%02d", or(p.M, 1), or(p.D, 1), hm, or(p.D, DaysIn(0, hm)))
}

func dayOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return date(y, int(m), d)
}

func or(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// jsonDate — представление в JSON: {"y":1985,"m":3,"d":12}, нет части — нет ключа.
type jsonDate struct {
	Y *int `json:"y,omitempty"`
	M *int `json:"m,omitempty"`
	D *int `json:"d,omitempty"`
}

func ptr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

// MarshalJSON — {"y":…,"m":…,"d":…} без неизвестных частей.
func (p Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(jsonDate{Y: ptr(p.Y), M: ptr(p.M), D: ptr(p.D)})
}

// UnmarshalJSON принимает {"y","m","d"} (часть может быть null или
// отсутствовать); лишние ключи и недопустимая дата — ошибка.
func (p *Date) UnmarshalJSON(b []byte) error {
	var j jsonDate
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&j); err != nil {
		return fmt.Errorf("partial date: %w", err)
	}
	var d Date
	for _, part := range []struct {
		src *int
		dst *int
		err error
	}{{j.Y, &d.Y, ErrYear}, {j.M, &d.M, ErrMonth}, {j.D, &d.D, ErrDay}} {
		if part.src != nil {
			if *part.src == 0 {
				return part.err // явный ноль — не «неизвестно»
			}
			*part.dst = *part.src
		}
	}
	if err := d.Validate(); err != nil {
		return err
	}
	*p = d
	return nil
}

// String — вид для журнала и отладки: ГГГГ-ММ-ДД с «?» вместо неизвестного.
func (p Date) String() string {
	part := func(v, w int) string {
		if v == 0 {
			return strings.Repeat("?", w)
		}
		return fmt.Sprintf("%0*d", w, v)
	}
	return part(p.Y, 4) + "-" + part(p.M, 2) + "-" + part(p.D, 2)
}

// Value — текстовый вид составного типа PostgreSQL: (1985,3,12), (,3,12).
func (p Date) Value() (driver.Value, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	part := func(v int) string {
		if v == 0 {
			return ""
		}
		return strconv.Itoa(v)
	}
	return "(" + part(p.Y) + "," + part(p.M) + "," + part(p.D) + ")", nil
}

// Scan читает текстовый вид составного типа. NULL сюда не приходит —
// для необязательных колонок используется *Date.
func (p *Date) Scan(src any) error {
	var s string
	switch v := src.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return fmt.Errorf("partial date: неожиданный тип %T", src)
	}
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") {
		return fmt.Errorf("partial date: неверный вид %q", s)
	}
	inner := s[1 : len(s)-1]
	parts := strings.Split(inner, ",")
	if len(parts) != 3 {
		return fmt.Errorf("partial date: неверный вид %q", s)
	}
	var vals [3]int
	for i, part := range parts {
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("partial date: неверный вид %q", s)
		}
		vals[i] = n
	}
	d := Date{Y: vals[0], M: vals[1], D: vals[2]}
	if err := d.Validate(); err != nil {
		return err
	}
	*p = d
	return nil
}
