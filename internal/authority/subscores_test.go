package authority

import (
	"database/sql"
	"math"
	"testing"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

// floatEq returns true when |a-b| <= eps. Shared by all authority tests.
func floatEq(a, b, eps float64) bool {
	return math.Abs(a-b) <= eps
}

func TestVolumeNorm(t *testing.T) {
	cases := []struct {
		name string
		v    int
		max  int
		want float64
	}{
		{"zero v", 0, 100, 0.0},
		{"v == max", 100, 100, 1.0},
		{"log compression mid", 10, 100, math.Log10(11) / math.Log10(101)},
		{"max zero guard", 50, 0, 0.0},
		{"max negative guard", 50, -1, 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := VolumeNorm(tc.v, tc.max)
			if !floatEq(got, tc.want, 0.001) {
				t.Fatalf("VolumeNorm(%d,%d) = %v, want %v", tc.v, tc.max, got, tc.want)
			}
		})
	}
}

func TestResponseRate7d(t *testing.T) {
	if got := ResponseRate7d(10, 4); !floatEq(got, 0.4, 0.001) {
		t.Fatalf("ResponseRate7d(10,4)=%v, want 0.4", got)
	}
	if got := ResponseRate7d(0, 0); got != 0.0 {
		t.Fatalf("ResponseRate7d(0,0)=%v, want 0.0", got)
	}
	if got := ResponseRate7d(5, 5); !floatEq(got, 1.0, 0.001) {
		t.Fatalf("ResponseRate7d(5,5)=%v, want 1.0", got)
	}
}

func TestLinkQuality(t *testing.T) {
	if got := LinkQuality(0, 0); got != 0.0 {
		t.Fatalf("LinkQuality(0,0)=%v, want 0.0 (D-03 literal)", got)
	}
	if got := LinkQuality(3, 10); !floatEq(got, 0.3, 0.001) {
		t.Fatalf("LinkQuality(3,10)=%v, want 0.3", got)
	}
	if got := LinkQuality(5, 5); !floatEq(got, 1.0, 0.001) {
		t.Fatalf("LinkQuality(5,5)=%v, want 1.0", got)
	}
}

func TestComposite(t *testing.T) {
	if got := Composite(1.0, 1.0, 1.0); !floatEq(got, 1.0, 0.001) {
		t.Fatalf("Composite(1,1,1)=%v, want 1.0", got)
	}
	// 0.2*0.5 + 0.4*0.25 + 0.4*0.25 = 0.1 + 0.1 + 0.1 = 0.3
	if got := Composite(0.5, 0.25, 0.25); !floatEq(got, 0.3, 0.001) {
		t.Fatalf("Composite(0.5,0.25,0.25)=%v, want 0.3", got)
	}
	if got := Composite(0, 0, 0); got != 0.0 {
		t.Fatalf("Composite(0,0,0)=%v, want 0.0", got)
	}
}

func TestInitSchema(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open mem db: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("first InitSchema: %v", err)
	}
	// Idempotency: re-run must not error and table count must not grow.
	countTables := func() int {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table'
			 AND name IN ('authority_scores','authority_state','url_hash_cache')`,
		).Scan(&n); err != nil {
			t.Fatalf("count tables: %v", err)
		}
		return n
	}
	if got := countTables(); got != 3 {
		t.Fatalf("after first init: got %d tables, want 3", got)
	}
	if err := InitSchema(db); err != nil {
		t.Fatalf("second InitSchema (idempotency): %v", err)
	}
	if got := countTables(); got != 3 {
		t.Fatalf("after second init: got %d tables, want 3 (not idempotent)", got)
	}
	// authority_state seed row exists exactly once.
	var seedCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM authority_state WHERE id=1`).Scan(&seedCount); err != nil {
		t.Fatalf("count state seed: %v", err)
	}
	if seedCount != 1 {
		t.Fatalf("authority_state seed rows = %d, want 1", seedCount)
	}
}
