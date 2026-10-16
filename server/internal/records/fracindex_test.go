package records

import (
	"math/rand/v2"
	"slices"
	"testing"
)

func TestKeyBetween(t *testing.T) {
	keys := []string{}
	r := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		i := r.IntN(len(keys) + 1)
		a, b := "", ""
		if i > 0 {
			a = keys[i-1]
		}
		if i < len(keys) {
			b = keys[i]
		}
		k, err := KeyBetween(a, b)
		if err != nil {
			t.Fatal(err)
		}
		if k <= a || (b != "" && k >= b) || k[len(k)-1] == '0' {
			t.Fatalf("between %q and %q: %q", a, b, k)
		}
		keys = slices.Insert(keys, i, k)
	}
	if !slices.IsSorted(keys) {
		t.Fatal("not sorted")
	}
	// постоянная вставка наверх и вниз
	top, bottom := "V", "V"
	for range 300 {
		top, _ = KeyBetween("", top)
		bottom, _ = KeyBetween(bottom, "")
	}
	if len(top) > 70 || len(bottom) > 70 { // ~6 вставок на символ
		t.Fatalf("keys grow too fast: %d %d", len(top), len(bottom))
	}
	if _, err := KeyBetween("b", "a"); err == nil {
		t.Fatal("wrong order accepted")
	}
}
