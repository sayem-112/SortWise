package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenAppliesMigrations(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "wizard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 9 {
		t.Fatalf("expected nine migrations, got %d", count)
	}
	var categories int
	if err := db.QueryRow(`SELECT COUNT(*) FROM categories WHERE active=1`).Scan(&categories); err != nil {
		t.Fatal(err)
	}
	// A new library starts with broad categories that fit any field.
	if categories != 15 {
		t.Fatalf("expected 15 broad starter categories, got %d", categories)
	}
	var specific int
	db.QueryRow(`SELECT COUNT(*) FROM categories WHERE active=1 AND normalized_name IN ('ai','backend','software-engineering')`).Scan(&specific)
	if specific != 0 {
		t.Fatalf("software-specific starters should be off in a new library, got %d", specific)
	}
}
