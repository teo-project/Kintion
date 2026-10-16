package records

import (
	"errors"
	"strings"
)

// Дробный ключ ручного порядка (Д-10): строка цифр base-62 в порядке ASCII
// (колонки sort_key — COLLATE "C"). Между любыми двумя ключами есть третий,
// поэтому вставка «между» не перенумеровывает соседей. Порт midpoint из
// fractional-indexing (без целой части); ключ не кончается на '0'.
const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var errKeyOrder = errors.New("sort key: нижний ключ не меньше верхнего")

// KeyBetween — ключ строго между a и b; "" у a — начало списка, у b — конец.
func KeyBetween(a, b string) (string, error) {
	if b != "" && a >= b {
		return "", errKeyOrder
	}
	return midpoint(a, b), nil
}

func midpoint(a, b string) string {
	if b != "" {
		n := 0
		for n < len(b) && at(a, n) == b[n] {
			n++
		}
		if n > 0 {
			rest := ""
			if n < len(a) {
				rest = a[n:]
			}
			return b[:n] + midpoint(rest, b[n:])
		}
	}
	da := 0
	if a != "" {
		da = strings.IndexByte(digits, a[0])
	}
	db := len(digits)
	if b != "" {
		db = strings.IndexByte(digits, b[0])
	}
	if db-da > 1 {
		return string(digits[(da+db+1)/2])
	}
	if len(b) > 1 {
		return b[:1]
	}
	rest := ""
	if len(a) > 1 {
		rest = a[1:]
	}
	return string(digits[da]) + midpoint(rest, "")
}

func at(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return digits[0]
}
