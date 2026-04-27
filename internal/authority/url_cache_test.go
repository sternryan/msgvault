package authority

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

// loadFixture reads a .sql file from testdata and execs it on db.
func loadFixture(t *testing.T, db *sql.DB, sqlPath string) {
	t.Helper()
	data, err := os.ReadFile(sqlPath)
	if err != nil {
		t.Fatalf("read fixture %s: %v", sqlPath, err)
	}
	if _, err := db.Exec(string(data)); err != nil {
		t.Fatalf("exec fixture %s: %v", sqlPath, err)
	}
}

// openSourcesFixture returns a sqlite3 :memory: DB pre-loaded with the
// shared sources_fixture.sql.
func openSourcesFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sources fixture: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	loadFixture(t, db, "testdata/sources_fixture.sql")
	return db
}

// openMsgvaultFixture returns a sqlite3 :memory: DB with InitSchema
// applied AND msgvault_fixture.sql loaded.
func openMsgvaultFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open msgvault fixture: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	loadFixture(t, db, "testdata/msgvault_fixture.sql")
	return db
}

func TestBuildURLHashCache_HappyPath(t *testing.T) {
	mvDB := openMsgvaultFixture(t)
	srcDB := openSourcesFixture(t)
	if err := BuildURLHashCache(context.Background(), mvDB, "testdata/manifests", srcDB); err != nil {
		t.Fatalf("BuildURLHashCache: %v", err)
	}
	var n int
	if err := mvDB.QueryRow(`SELECT COUNT(*) FROM url_hash_cache`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	// 3 fixture manifests, all with origin_url + source_hash populated.
	if n != 3 {
		t.Fatalf("url_hash_cache rows = %d, want 3", n)
	}
}

func TestBuildURLHashCache_Idempotent(t *testing.T) {
	mvDB := openMsgvaultFixture(t)
	srcDB := openSourcesFixture(t)
	ctx := context.Background()
	if err := BuildURLHashCache(ctx, mvDB, "testdata/manifests", srcDB); err != nil {
		t.Fatalf("first build: %v", err)
	}
	if err := BuildURLHashCache(ctx, mvDB, "testdata/manifests", srcDB); err != nil {
		t.Fatalf("second build: %v", err)
	}
	var n int
	mvDB.QueryRow(`SELECT COUNT(*) FROM url_hash_cache`).Scan(&n)
	if n != 3 {
		t.Fatalf("after re-run rows = %d, want 3 (idempotent)", n)
	}
}

func TestBuildURLHashCache_TolerateMalformed(t *testing.T) {
	// Use a temp manifests dir so we can drop a malformed file without
	// polluting the shared fixtures. We seed it with copies of the 3 good
	// manifests + 1 malformed manifest.
	tmp := t.TempDir()
	for _, name := range []string{"example-1", "example-2", "example-3"} {
		src, err := os.ReadFile(filepath.Join("testdata", "manifests", name, "manifest.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(tmp, name)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, "manifest.yaml"), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	brokenDir := filepath.Join(tmp, "broken")
	if err := os.MkdirAll(brokenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(brokenDir, "manifest.yaml"), []byte(":::not yaml:::\n  - [oops"), 0o644); err != nil {
		t.Fatal(err)
	}

	mvDB := openMsgvaultFixture(t)
	srcDB := openSourcesFixture(t)
	if err := BuildURLHashCache(context.Background(), mvDB, tmp, srcDB); err != nil {
		t.Fatalf("BuildURLHashCache should tolerate malformed YAML, got: %v", err)
	}
	var n int
	mvDB.QueryRow(`SELECT COUNT(*) FROM url_hash_cache`).Scan(&n)
	if n != 3 {
		t.Fatalf("rows = %d, want 3 (broken manifest skipped, others kept)", n)
	}
}

func TestBuildURLHashCache_RespectsCompiledStatus(t *testing.T) {
	mvDB := openMsgvaultFixture(t)
	srcDB := openSourcesFixture(t)
	if err := BuildURLHashCache(context.Background(), mvDB, "testdata/manifests", srcDB); err != nil {
		t.Fatalf("BuildURLHashCache: %v", err)
	}
	// example-1 + example-2 are status='compiled' in BOTH sources.db and
	// manifest YAML → compiled=1. example-3 is status='ingested' in
	// sources.db (and 'ingested' in the manifest) → compiled=0.
	cases := map[string]int{
		NormalizeURL("https://example.com/post/alpha"):       1,
		NormalizeURL("https://stanford.edu/research/beta"):   1,
		NormalizeURL("https://newsletter.com/issue/3"):       0,
	}
	for url, want := range cases {
		var got int
		err := mvDB.QueryRow(`SELECT compiled FROM url_hash_cache WHERE url_normalized = ?`, url).Scan(&got)
		if err != nil {
			t.Fatalf("query %s: %v", url, err)
		}
		if got != want {
			t.Errorf("compiled for %s = %d, want %d", url, got, want)
		}
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://Example.com/path/", "https://example.com/path"},
		{"https://Example.com/path/?q=1#frag", "https://example.com/path/?q=1"},
		{"https://example.com/", "https://example.com"},
		{"  https://EXAMPLE.com/foo  ", "https://example.com/foo"},
	}
	for _, tc := range cases {
		if got := NormalizeURL(tc.in); got != tc.want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
