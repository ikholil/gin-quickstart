package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAppliesMigrationsAndEnablesForeignKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "store.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var migrations int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&migrations); err != nil {
		t.Fatal(err)
	}
	if migrations != 1 {
		t.Fatalf("got %d migrations, want 1", migrations)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("database permissions = %o, want 600", info.Mode().Perm())
	}
	var foreignKeys int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
	if _, err := db.Exec(`INSERT INTO products (id, category_id, name, price_cents, stock, created_at, updated_at)
		VALUES ('p1', 'missing', 'Product', 100, 1, 'now', 'now')`); err == nil {
		t.Fatal("expected foreign-key constraint failure")
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	for range 2 {
		db, err := Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
