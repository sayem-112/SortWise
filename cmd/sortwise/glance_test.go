package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"sortwise/internal/ai"
	"sortwise/internal/database"
	"sortwise/internal/flyout"
	"sortwise/internal/model"
	"sortwise/internal/store"
)

func TestThousands(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 2547: "2,547", 1234567: "1,234,567", -1200: "-1,200"} {
		if got := thousands(n); got != want {
			t.Fatalf("%d: got %s", n, got)
		}
	}
	if count(1, "bookmark") != "1 bookmark" || count(2547, "bookmark") != "2,547 bookmarks" {
		t.Fatal("count")
	}
}

// keyring stands in for Windows Credential Manager.
type keyring map[string]string

func (k keyring) Get(name string) (string, string, error) { return k[name], "test", nil }
func (k keyring) Set(name, key string) error              { k[name] = key; return nil }
func (k keyring) Delete(name string) error                { delete(k, name); return nil }

func TestPanelStatus(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st := store.New(db)
	providers := ai.DefaultFactories()
	keys := keyring{}
	lib := &library{db: db, store: st, keys: keys, providers: providers, worker: ai.NewWorkerWithProviders(db, keys, providers)}
	check := func(headline string, tone flyout.Tone, detail string) {
		t.Helper()
		lib.keysRead = time.Time{} // read the keys again
		status := lib.status()
		if status.Headline != headline || status.Tone != tone || status.Detail != detail {
			t.Fatalf("got %+v, want %q %v %q", status, headline, tone, detail)
		}
	}

	check("Needs an AI key to organize", flyout.Warning, "0 bookmarks on this PC")
	keys["groq"] = "key" // the backup alone is enough while backup is on
	check("Up to date", flyout.Good, "0 bookmarks on this PC")

	now := time.Now().UTC().Format(time.RFC3339)
	batch := model.ImportBatch{}
	for _, id := range []string{"1", "2"} {
		batch.Bookmarks = append(batch.Bookmarks, model.ImportedBookmark{Platform: "x", PostID: id, Author: "Ada", Username: "ada", Text: "Post " + id, URL: "https://x.com/ada/status/" + id, ExtractedAt: now, ExtractorVersion: "1.0.0"})
	}
	if _, err := st.Import(ctx, batch); err != nil {
		t.Fatal(err)
	}
	check("Organizing 2 new posts", flyout.Busy, "2 bookmarks on this PC · 2 saved today")

	if _, err := db.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES ('ai_paused','true')`); err != nil {
		t.Fatal(err)
	}
	check("Paused, 2 waiting", flyout.Quiet, "2 bookmarks on this PC · 2 saved today")
	if _, err := db.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES ('ai_backup','false')`); err != nil {
		t.Fatal(err)
	}
	// Gemini is the main provider and has no key; the backup is off.
	check("Needs an AI key to organize", flyout.Warning, "2 bookmarks on this PC · 2 saved today")

	// When the library can't be read in time, the last good status stays.
	db.Close()
	if status := lib.status(); status.Headline != "Needs an AI key to organize" {
		t.Fatalf("a failed read replaced the last status: %+v", status)
	}
}
