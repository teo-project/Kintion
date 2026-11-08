package records

import "testing"

// F-149: ник в названии технической группы заменяется только целым словом.
func TestReplaceNick(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Созданные mama", "Созданные mom"},
		{"Записи mama (mamas)", "Записи mom (mamas)"},
		{"mama и mama.x", "mom и mama.x"},
		{"Без ника", "Без ника"},
		{"mamamama", "mamamama"},
		{"(mama)(mama)", "(mom)(mom)"},
	} {
		if got := replaceNick(c.in, "mama", "mom"); got != c.want {
			t.Errorf("replaceNick(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
