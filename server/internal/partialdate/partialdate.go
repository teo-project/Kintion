// Package partialdate — частичная дата (F-14, docs/concepts/dates.md):
// год, месяц, день — каждая часть необязательна, хотя бы одна есть; год — целое
// со знаком без нуля (отрицательный — до начала эры); эра — необязательная
// текстовая надпись при годе (пустая — эра по умолчанию дашборда) или одна,
// без года, месяца и дня («мезозойская эра»).
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
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Date — частичная дата. Нулевая часть означает «неизвестна»
// (у допустимой даты частей, равных нулю, не бывает); пустая эра — наша.
type Date struct {
	Y int    // год со знаком, без 0 (int32)
	M int    // месяц 1..12
	D int    // день 1..31
	E string // эра; при годе или одна (без месяца и дня)
}

// MaxEraLen — наибольшая длина эры в символах.
const MaxEraLen = 64

// Пределы календарной даты (date в PostgreSQL): вне них Lo, Hi, Exact и
// годовщины не считаются.
const (
	minCalendarYear = -4713
	maxCalendarYear = 5874897
)

// Ошибки проверки.
var (
	ErrEmpty = errors.New("partial date: нужна хотя бы одна часть")
	ErrYear  = errors.New("partial date: год — целое со знаком, без 0")
	ErrMonth = errors.New("partial date: месяц 1..12")
	ErrDay   = errors.New("partial date: день 1..31")
	ErrNoDay = errors.New("partial date: такого дня в месяце нет")
	ErrEra   = errors.New("partial date: эра — при годе или одна, от 1 до 64 символов, без пробелов по краям")
)

// New проверяет части и возвращает дату нашей эры.
func New(y, m, d int) (Date, error) {
	pd := Date{Y: y, M: m, D: d}
	return pd, pd.Validate()
}

// IsZero — нет ни одной части.
func (p Date) IsZero() bool { return p.Y == 0 && p.M == 0 && p.D == 0 && p.E == "" }

// ValidEra — допустимый текст эры (непустой).
func ValidEra(e string) bool {
	if e == "" || !utf8.ValidString(e) || utf8.RuneCountInString(e) > MaxEraLen || strings.TrimSpace(e) != e {
		return false
	}
	return !strings.ContainsFunc(e, unicode.IsControl)
}

// Validate проверяет допустимость: части в диапазонах, день есть в месяце
// (29.02 без года — да, с годом — только в високосный), эра — при годе или
// одна (без месяца и дня).
func (p Date) Validate() error {
	switch {
	case p.IsZero():
		return ErrEmpty
	case p.Y < math.MinInt32 || p.Y > math.MaxInt32:
		return ErrYear
	case p.M < 0 || p.M > 12:
		return ErrMonth
	case p.D < 0 || p.D > 31:
		return ErrDay
	case p.E != "" && (!ValidEra(p.E) || (p.Y == 0 && (p.M != 0 || p.D != 0))):
		return ErrEra
	case p.M != 0 && p.D > DaysIn(p.Y, p.M):
		return ErrNoDay
	}
	return nil
}

// Astro — астрономический номер года: −1 → 0, −2 → −1 (у дат года 0 нет).
func Astro(y int) int {
	if y < 0 {
		return y + 1
	}
	return y
}

// DaysIn — число дней в месяце; год 0 — неизвестен (у февраля 29).
// Високосность — по астрономическому году (1 г. до н. э. — високосный).
func DaysIn(y, m int) int {
	switch m {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if y == 0 || isLeap(Astro(y)) {
			return 29
		}
		return 28
	}
	return 0
}

func isLeap(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

// date — календарная дата из частей; y — год даты (со знаком, без 0).
// time.Time считает годы астрономически.
func date(y, m, d int) time.Time {
	return time.Date(Astro(y), time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

// calendar — у даты есть календарные границы: год известен, наша эра, в пределах date.
func (p Date) calendar() bool {
	return p.Y != 0 && p.E == "" && p.Y >= minCalendarYear && p.Y <= maxCalendarYear
}

// Lo — нижняя граница (календарная, см. calendar).
func (p Date) Lo() (time.Time, bool) {
	if !p.calendar() {
		return time.Time{}, false
	}
	return date(p.Y, or(p.M, 1), or(p.D, 1)), true
}

// Hi — верхняя граница (календарная, см. calendar).
func (p Date) Hi() (time.Time, bool) {
	if !p.calendar() {
		return time.Time{}, false
	}
	m := or(p.M, 12)
	return date(p.Y, m, or(p.D, DaysIn(p.Y, m))), true
}

// Exact — точная дата, если известны все части (календарная, см. calendar).
func (p Date) Exact() (time.Time, bool) {
	if !p.calendar() || p.M == 0 || p.D == 0 {
		return time.Time{}, false
	}
	return date(p.Y, p.M, p.D), true
}

// YearOf — год календарной даты (time.Time — астрономический) как год частичной даты.
func YearOf(t time.Time) int {
	if y := t.Year(); y <= 0 {
		return y - 1
	}
	return t.Year()
}

// Today — частичная дата нашей эры из календарной.
func Today(t time.Time) Date {
	_, m, d := t.Date()
	return Date{Y: YearOf(t), M: int(m), D: d}
}

// AgeAt — полных лет на дату at той же эры, считая от верхней границы (не
// меньше стольких); у at без месяца или дня берётся нижняя граница. Без года,
// в другой эре и до нижней границы — нет значения. 29.02 в невисокосный год
// прибавляет год 28.02. reversed — счёт в эре обратный (F-143): больший год раньше.
func (p Date) AgeAt(at Date, reversed bool) (int, bool) {
	if p.Y == 0 || at.Y == 0 || p.E != at.E {
		return 0, false
	}
	// год по ходу времени: в эре с обратным счётом — со знаком минус
	fy := func(y int) int {
		if reversed {
			return -Astro(y)
		}
		return Astro(y)
	}
	am, ad := or(at.M, 1), or(at.D, 1)
	if cmp3(fy(at.Y), am, ad, fy(p.Y), or(p.M, 1), or(p.D, 1)) < 0 {
		return 0, false
	}
	hm := or(p.M, 12)
	hd := min(or(p.D, DaysIn(p.Y, hm)), DaysIn(at.Y, hm))
	age := fy(at.Y) - fy(p.Y)
	if am < hm || (am == hm && ad < hd) {
		age--
	}
	return max(age, 0), true
}

func cmp3(a1, a2, a3, b1, b2, b3 int) int {
	switch {
	case a1 != b1:
		return cmpInt(a1, b1)
	case a2 != b2:
		return cmpInt(a2, b2)
	}
	return cmpInt(a3, b3)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// NextAnniversary — ближайшая годовщина не раньше from (включительно); нужны
// месяц и день, наша эра. Если год известен — не раньше самой даты.
// 29.02 в невисокосный год — 28.02.
func (p Date) NextAnniversary(from time.Time) (time.Time, bool) {
	if p.M == 0 || p.D == 0 || p.E != "" {
		return time.Time{}, false
	}
	from = dayOf(from)
	y := YearOf(from)
	if p.Y != 0 {
		y = max(y, p.Y)
	}
	if y < minCalendarYear {
		return time.Time{}, false // вне пределов date — как в SQL
	}
	for ; y <= maxCalendarYear; y = nextYear(y) {
		c := date(y, p.M, min(p.D, DaysIn(y, p.M)))
		if !c.Before(from) {
			return c, true
		}
	}
	return time.Time{}, false
}

func nextYear(y int) int {
	if y == -1 {
		return 1
	}
	return y + 1
}

// EraOnly — дата из одной эры, без года, месяца и дня.
func (p Date) EraOnly() bool { return p.E != "" && p.Y == 0 && p.M == 0 && p.D == 0 }

// EraOrder — порядок эр дашборда для сравнения дат (F-143): List — эры по
// порядку (пустая — наша), Reversed — эры с обратным счётом (минимальный год
// больше максимального): в них больший год — раньше.
type EraOrder struct {
	List     []string
	Reversed []string
}

// EraKey — место эры в ключе сортировки (F-143): эры вне списка order —
// первыми (новая эра — наверх), между собой по тексту, пустая (наша) — раньше
// всех, с разделителем \x01; эры из списка — дальше, по месту в нём.
func EraKey(e string, order []string) string {
	for i, o := range order {
		if o == e {
			return fmt.Sprintf("1%05d", i)
		}
	}
	return "0" + e + "\x01"
}

// YearKey — год в ключе: y + 2^31 (10 цифр), в эре с обратным счётом —
// 2^31 − y (больший год — раньше).
func YearKey(y int, reversed bool) string {
	if reversed {
		return fmt.Sprintf("%010d", 1<<31-int64(y))
	}
	return fmt.Sprintf("%010d", int64(y)+1<<31)
}

// SortKey — ключ сортировки (Д-9, F-14): датированные — "0" + место эры +
// год (YearKey) + ММДД нижней и верхней границы; дата из одной эры — "0" +
// место эры (раньше всех дат эры); без года — "1" + ММДД нижней и верхней
// границы (после датированных). o — порядок эр дашборда (F-143).
// Сравнивается побайтно (COLLATE "C").
func (p Date) SortKey(o EraOrder) string {
	if p.EraOnly() {
		return "0" + EraKey(p.E, o.List)
	}
	hm := or(p.M, 12)
	md := fmt.Sprintf("%02d%02d%02d%02d", or(p.M, 1), or(p.D, 1), hm, or(p.D, DaysIn(p.Y, hm)))
	if p.Y == 0 {
		return "1" + md
	}
	return "0" + EraKey(p.E, o.List) + YearKey(p.Y, slices.Contains(o.Reversed, p.E)) + md
}

func dayOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func or(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// jsonDate — представление в JSON: {"y":1985,"m":3,"d":12,"e":"AC"}, нет части — нет ключа.
type jsonDate struct {
	Y *int    `json:"y,omitempty"`
	M *int    `json:"m,omitempty"`
	D *int    `json:"d,omitempty"`
	E *string `json:"e,omitempty"`
}

func ptr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

// MarshalJSON — {"y":…,"m":…,"d":…,"e":…} без неизвестных частей и пустой эры.
func (p Date) MarshalJSON() ([]byte, error) {
	j := jsonDate{Y: ptr(p.Y), M: ptr(p.M), D: ptr(p.D)}
	if p.E != "" {
		j.E = &p.E
	}
	return json.Marshal(j)
}

// UnmarshalJSON принимает {"y","m","d","e"} (часть может быть null или
// отсутствовать; пустая эра — наша); лишние ключи и недопустимая дата — ошибка.
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
	if j.E != nil {
		d.E = *j.E
	}
	if err := d.Validate(); err != nil {
		return err
	}
	*p = d
	return nil
}

// String — вид для журнала и отладки: ГГГГ-ММ-ДД с «?» вместо неизвестного,
// год до начала эры — со знаком минус, эра — через пробел.
func (p Date) String() string {
	part := func(v, w int) string {
		if v == 0 {
			return strings.Repeat("?", w)
		}
		return fmt.Sprintf("%0*d", w, v)
	}
	y := part(p.Y, 4)
	if p.Y < 0 {
		y = "-" + part(-p.Y, 4)
	}
	s := y + "-" + part(p.M, 2) + "-" + part(p.D, 2)
	if p.E != "" {
		s += " " + p.E
	}
	return s
}

// Value — текстовый вид составного типа PostgreSQL: (1985,3,12,), (,3,12,),
// (-500,,,"AC"). Эра всегда в кавычках.
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
	era := ""
	if p.E != "" {
		era = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(p.E) + `"`
	}
	return "(" + part(p.Y) + "," + part(p.M) + "," + part(p.D) + "," + era + ")", nil
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
	fields, err := splitRecord(s)
	if err != nil || len(fields) != 4 {
		return fmt.Errorf("partial date: неверный вид %q", s)
	}
	var vals [3]int
	for i, f := range fields[:3] {
		if f == nil {
			continue
		}
		n, err := strconv.Atoi(*f)
		if err != nil {
			return fmt.Errorf("partial date: неверный вид %q", s)
		}
		vals[i] = n
	}
	d := Date{Y: vals[0], M: vals[1], D: vals[2]}
	if fields[3] != nil {
		if *fields[3] == "" {
			return ErrEra
		}
		d.E = *fields[3]
	}
	if err := d.Validate(); err != nil {
		return err
	}
	*p = d
	return nil
}

// splitRecord разбирает текстовый вид записи PostgreSQL «(a,b,"c d")»:
// пустое поле без кавычек — NULL; в кавычках "" и \x — экранирование.
func splitRecord(s string) ([]*string, error) {
	if len(s) < 2 || s[0] != '(' || s[len(s)-1] != ')' {
		return nil, errors.New("not a record")
	}
	in := s[1 : len(s)-1]
	var out []*string
	for {
		var b strings.Builder
		quoted, null := false, true
		i := 0
	scan:
		for ; i < len(in); i++ {
			c := in[i]
			switch {
			case c == '"' && quoted && i+1 < len(in) && in[i+1] == '"':
				b.WriteByte('"')
				i++
			case c == '"':
				quoted, null = !quoted, false
			case c == '\\' && i+1 < len(in):
				b.WriteByte(in[i+1])
				null = false
				i++
			case c == ',' && !quoted:
				break scan
			default:
				b.WriteByte(c)
				null = false
			}
		}
		if quoted {
			return nil, errors.New("unterminated quote")
		}
		if null {
			out = append(out, nil)
		} else {
			v := b.String()
			out = append(out, &v)
		}
		if i >= len(in) {
			return out, nil
		}
		in = in[i+1:]
	}
}
