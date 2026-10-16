package partialdate_test

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"kintion/server/internal/partialdate"
)

// Cases — общие кейсы testdata/partial_dates.json.
type Cases struct {
	Valid []struct {
		Date    json.RawMessage `json:"date"`
		Lo      *string         `json:"lo"`
		Hi      *string         `json:"hi"`
		Exact   *string         `json:"exact"`
		SortKey string          `json:"sort_key"`
	} `json:"valid"`
	Invalid []struct {
		Date json.RawMessage `json:"date"`
		Why  string          `json:"why"`
	} `json:"invalid"`
	Age []struct {
		Date json.RawMessage `json:"date"`
		At   string          `json:"at"`
		Age  *int            `json:"age"`
	} `json:"age"`
	Anniversary []struct {
		Date json.RawMessage `json:"date"`
		From string          `json:"from"`
		Next *string         `json:"next"`
	} `json:"anniversary"`
	Sorted []json.RawMessage `json:"sorted"`
}

func loadCases(t *testing.T) Cases {
	t.Helper()
	data, err := os.ReadFile("../../../testdata/partial_dates.json")
	if err != nil {
		t.Fatal(err)
	}
	var c Cases
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func parse(t *testing.T, raw json.RawMessage) partialdate.Date {
	t.Helper()
	var d partialdate.Date
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	return d
}

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func fmtDate(v time.Time, ok bool) *string {
	if !ok {
		return nil
	}
	s := v.Format(time.DateOnly)
	return &s
}

func eq[T comparable](a, b *T) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func show[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestValid(t *testing.T) {
	for _, c := range loadCases(t).Valid {
		d := parse(t, c.Date)
		if got := fmtDate(d.Lo()); !eq(got, c.Lo) {
			t.Errorf("%s lo = %v, want %v", c.Date, show(got), show(c.Lo))
		}
		if got := fmtDate(d.Hi()); !eq(got, c.Hi) {
			t.Errorf("%s hi = %v, want %v", c.Date, show(got), show(c.Hi))
		}
		if got := fmtDate(d.Exact()); !eq(got, c.Exact) {
			t.Errorf("%s exact = %v, want %v", c.Date, show(got), show(c.Exact))
		}
		if got := d.SortKey(); got != c.SortKey {
			t.Errorf("%s sort key = %q, want %q", c.Date, got, c.SortKey)
		}
		// JSON туда и обратно
		out, err := json.Marshal(d)
		if err != nil || parse(t, out) != d {
			t.Errorf("%s json round trip: %s %v", c.Date, out, err)
		}
		// вид для БД туда и обратно
		v, err := d.Value()
		if err != nil {
			t.Fatalf("%s value: %v", c.Date, err)
		}
		var back partialdate.Date
		if err := back.Scan(v); err != nil || back != d {
			t.Errorf("%s db round trip: %v %v", c.Date, back, err)
		}
	}
}

func TestInvalid(t *testing.T) {
	for _, c := range loadCases(t).Invalid {
		var d partialdate.Date
		if err := json.Unmarshal(c.Date, &d); err == nil {
			t.Errorf("%s (%s) accepted", c.Date, c.Why)
		}
	}
	var d partialdate.Date
	if err := json.Unmarshal([]byte(`{"y":1985,"x":1}`), &d); err == nil {
		t.Error("unknown key accepted")
	}
	if _, err := (partialdate.Date{}).Value(); err == nil {
		t.Error("empty date written to db")
	}
	if err := d.Scan("(,,)"); err == nil {
		t.Error("empty date scanned")
	}
}

func TestAge(t *testing.T) {
	for _, c := range loadCases(t).Age {
		d := parse(t, c.Date)
		age, ok := d.AgeAt(day(t, c.At))
		var got *int
		if ok {
			got = &age
		}
		if !eq(got, c.Age) {
			t.Errorf("%s age at %s = %v, want %v", c.Date, c.At, show(got), show(c.Age))
		}
	}
}

func TestAnniversary(t *testing.T) {
	for _, c := range loadCases(t).Anniversary {
		d := parse(t, c.Date)
		if got := fmtDate(d.NextAnniversary(day(t, c.From))); !eq(got, c.Next) {
			t.Errorf("%s next from %s = %v, want %v", c.Date, c.From, show(got), show(c.Next))
		}
	}
}

func TestSort(t *testing.T) {
	var want []partialdate.Date
	for _, raw := range loadCases(t).Sorted {
		want = append(want, parse(t, raw))
	}
	got := slices.Clone(want)
	slices.Reverse(got)
	slices.SortStableFunc(got, func(a, b partialdate.Date) int {
		switch ka, kb := a.SortKey(), b.SortKey(); {
		case ka < kb:
			return -1
		case ka > kb:
			return 1
		}
		return 0
	})
	if !slices.Equal(got, want) {
		t.Fatalf("sorted %v, want %v", got, want)
	}
}
