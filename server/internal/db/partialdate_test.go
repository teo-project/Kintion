package db_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"kintion/server/internal/db"
	"kintion/server/internal/db/dbtest"
	"kintion/server/internal/partialdate"
)

// pdCases — общие кейсы testdata/partial_dates.json (те же, что в Go- и TS-тестах).
type pdCases struct {
	Valid []struct {
		Date    partialdate.Date `json:"date"`
		Lo      *string          `json:"lo"`
		Hi      *string          `json:"hi"`
		Exact   *string          `json:"exact"`
		SortKey string           `json:"sort_key"`
	} `json:"valid"`
	Invalid []struct {
		Date map[string]any `json:"date"`
		Why  string         `json:"why"`
	} `json:"invalid"`
	Age []struct {
		Date partialdate.Date `json:"date"`
		At   partialdate.Date `json:"at"`
		Age  *int             `json:"age"`
	} `json:"age"`
	Anniversary []struct {
		Date partialdate.Date `json:"date"`
		From string           `json:"from"`
		Next *string          `json:"next"`
	} `json:"anniversary"`
	Sorted     []partialdate.Date `json:"sorted"`
	SortedEras struct {
		Eras     []string           `json:"eras"`
		Reversed []string           `json:"reversed"`
		Dates    []partialdate.Date `json:"dates"`
	} `json:"sorted_eras"`
}

// isoSQL — вид даты как в кейсах: ±ГГГГ-ММ-ДД, год до н. э. — со знаком минус.
func isoSQL(expr string) string {
	return "CASE WHEN extract(year FROM " + expr + ") < 0 THEN '-' ELSE '' END || to_char(" + expr + ", 'YYYY-MM-DD')"
}

func migrated(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := dbtest.New(t)
	if err := db.NewMigrator(pool, "test", slog.New(slog.NewTextHandler(io.Discard, nil))).Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	return pool
}

func eqp[T comparable](a, b *T) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func TestPartialDateSQL(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/partial_dates.json")
	if err != nil {
		t.Fatal(err)
	}
	var c pdCases
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	pool := migrated(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "CREATE TABLE pd_test (n serial PRIMARY KEY, v partial_date)"); err != nil {
		t.Fatal(err)
	}

	for _, v := range c.Valid {
		var lo, hi, exact *string
		var key string
		var back partialdate.Date
		err := pool.QueryRow(ctx, `INSERT INTO pd_test (v) VALUES ($1) RETURNING v,
			`+isoSQL("pd_lo(v)")+`, `+isoSQL("pd_hi(v)")+`, `+isoSQL("pd_exact(v)")+`, pd_sort_key(v)`, v.Date).
			Scan(&back, &lo, &hi, &exact, &key)
		if err != nil {
			t.Errorf("%v: %v", v.Date, err)
			continue
		}
		if back != v.Date || !eqp(lo, v.Lo) || !eqp(hi, v.Hi) || !eqp(exact, v.Exact) || key != v.SortKey {
			t.Errorf("%v: got %v lo=%v hi=%v exact=%v key=%s", v.Date, back, lo, hi, exact, key)
		}
	}

	for _, v := range c.Invalid {
		part := func(k string) any {
			if n, ok := v.Date[k]; ok {
				if f, ok := n.(float64); ok {
					return int64(f)
				}
				return n
			}
			return nil
		}
		_, err := pool.Exec(ctx, `INSERT INTO pd_test (v) VALUES (ROW($1::bigint::integer, $2::smallint, $3::smallint, $4::text))`,
			part("y"), part("m"), part("d"), part("e"))
		if err == nil {
			t.Errorf("%v (%s) accepted by domain", v.Date, v.Why)
		}
	}

	// необязательная колонка: настоящий NULL допустим и читается в *Date как nil
	var null *partialdate.Date
	if err := pool.QueryRow(ctx, "INSERT INTO pd_test (v) VALUES (NULL) RETURNING v").Scan(&null); err != nil || null != nil {
		t.Fatalf("NULL: %v %v", null, err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO pd_test (v) VALUES ($1) RETURNING v", (*partialdate.Date)(nil)).Scan(&null); err != nil || null != nil {
		t.Fatalf("nil param: %v %v", null, err)
	}

	for _, a := range c.Age {
		if a.At.E != "" || a.At.M == 0 || a.At.D == 0 {
			continue // pd_age_at считает на календарную дату нашей эры
		}
		var got *int
		if err := pool.QueryRow(ctx, "SELECT pd_age_at($1::partial_date, make_date($2, $3, $4))", a.Date, a.At.Y, a.At.M, a.At.D).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if !eqp(got, a.Age) {
			t.Errorf("%v age at %s = %v, want %v", a.Date, a.At, got, a.Age)
		}
	}

	for _, a := range c.Anniversary {
		var got *string
		if err := pool.QueryRow(ctx, "SELECT pd_next_anniversary($1::partial_date, $2::date)::text", a.Date, a.From).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if !eqp(got, a.Next) {
			t.Errorf("%v next from %s = %v, want %v", a.Date, a.From, got, a.Next)
		}
	}

	sqlSorted(t, pool, c.Sorted, partialdate.EraOrder{})
	order := partialdate.EraOrder{List: c.SortedEras.Eras, Reversed: c.SortedEras.Reversed}
	sqlSorted(t, pool, c.SortedEras.Dates, order)

	// sort key в SQL и Go совпадает и с порядком эр
	for _, d := range c.SortedEras.Dates {
		var key string
		if err := pool.QueryRow(ctx, "SELECT pd_sort_key($1::partial_date, $2::text[], $3::text[])", d, order.List, order.Reversed).Scan(&key); err != nil {
			t.Fatal(err)
		}
		if want := d.SortKey(order); key != want {
			t.Errorf("%v: sql key %q, go %q", d, key, want)
		}
	}
}

// sqlSorted — ORDER BY pd_sort_key(v, eras) даёт порядок кейса.
func sqlSorted(t *testing.T, pool *pgxpool.Pool, want []partialdate.Date, o partialdate.EraOrder) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE pd_test"); err != nil {
		t.Fatal(err)
	}
	rev := slices.Clone(want)
	slices.Reverse(rev)
	for _, d := range rev {
		if _, err := pool.Exec(ctx, "INSERT INTO pd_test (v) VALUES ($1)", d); err != nil {
			t.Fatal(err)
		}
	}
	eras, revs := o.List, o.Reversed
	if eras == nil {
		eras = []string{}
	}
	if revs == nil {
		revs = []string{}
	}
	rows, err := pool.Query(ctx, `SELECT v FROM pd_test ORDER BY pd_sort_key(v, $1::text[], $2::text[]) COLLATE "C"`, eras, revs)
	if err != nil {
		t.Fatal(err)
	}
	var got []partialdate.Date
	for rows.Next() {
		var d partialdate.Date
		if err := rows.Scan(&d); err != nil {
			t.Fatal(err)
		}
		got = append(got, d)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ORDER BY pd_sort_key(%q): %v, want %v", eras, got, want)
	}
}
