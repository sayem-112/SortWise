package ai

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"sortwise/internal/database"
	"sortwise/internal/model"
	"sortwise/internal/store"
)

type quotaProvider struct{}

func (quotaProvider) Analyze(context.Context, AnalysisInput) (AnalysisResult, error) {
	return AnalysisResult{}, fmt.Errorf("%w: resets at midnight", ErrDailyQuota)
}

type keysFor map[string]string

func (k keysFor) Get(name string) (string, string, error) { return k[name], "credential_manager", nil }
func (k keysFor) Set(string, string) error                { return nil }
func (k keysFor) Delete(string) error                     { return nil }

func failoverWorker(t *testing.T, keys keysFor) (*Worker, func()) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "failover.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"1", "2"} {
		item := model.ImportedBookmark{Platform: "x", PostID: id, Author: "Ada", Username: "ada", Text: "Reliable queues " + id, URL: "https://x.com/ada/status/" + id, ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "1.0.0"}
		if _, err := store.New(db).Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
			t.Fatal(err)
		}
	}
	worker := NewWorkerWithProviders(db, keys, map[string]Factory{
		"gemini": func(context.Context, string) (Provider, error) { return quotaProvider{}, nil },
		"groq":   func(context.Context, string) (Provider, error) { return fakeProvider{}, nil },
	})
	worker.minRequestInterval = 0
	return worker, func() { db.Close() }
}

func TestBackupProviderTakesOverWhenTheMainOneRests(t *testing.T) {
	worker, done := failoverWorker(t, keysFor{"gemini": "g", "groq": "q"})
	defer done()
	ctx := context.Background()

	// Gemini (the default) reports its daily quota; the job goes straight back.
	if processed, _ := worker.ProcessOne(ctx); !processed {
		t.Fatal("expected the first job to be tried")
	}
	resting := worker.Resting()
	if len(resting) != 1 || resting[0].Provider != "gemini" || !resting[0].RestingUntil.After(time.Now()) {
		t.Fatalf("expected Gemini to rest, got %+v", resting)
	}
	for i := 0; i < 2; i++ {
		if processed, err := worker.ProcessOne(ctx); !processed || err != nil {
			t.Fatalf("expected Groq to organize, got %v %v", processed, err)
		}
	}
	var completed int
	worker.db.QueryRow(`SELECT COUNT(*) FROM bookmarks WHERE processing_status='completed'`).Scan(&completed)
	var provider string
	worker.db.QueryRow(`SELECT provider FROM enrichments LIMIT 1`).Scan(&provider)
	if completed != 2 || provider != "groq" {
		t.Fatalf("completed %d, provider %q", completed, provider)
	}
}

func TestNoBackupWhenTurnedOffOrWithoutAKey(t *testing.T) {
	worker, done := failoverWorker(t, keysFor{"gemini": "g"})
	defer done()
	ctx := context.Background()
	worker.ProcessOne(ctx)
	if processed, _ := worker.ProcessOne(ctx); processed {
		t.Fatal("with no Groq key, the queue should wait for Gemini")
	}

	worker2, done2 := failoverWorker(t, keysFor{"gemini": "g", "groq": "q"})
	defer done2()
	worker2.db.Exec(`INSERT INTO settings(key,value) VALUES ('ai_backup','false')`)
	worker2.ProcessOne(ctx)
	if processed, _ := worker2.ProcessOne(ctx); processed {
		t.Fatal("with backup off, the queue should wait for Gemini")
	}
	worker2.Unblock(ctx)
	if len(worker2.Resting()) != 0 {
		t.Fatal("Unblock should clear every provider's rest")
	}
}

func TestMissingKeyBlocksWithAClearMessage(t *testing.T) {
	worker, done := failoverWorker(t, keysFor{})
	defer done()
	worker.ProcessOne(context.Background())
	var message string
	worker.db.QueryRow(`SELECT last_error FROM jobs WHERE status='blocked'`).Scan(&message)
	if message != "Add a Gemini API key in Settings to organize bookmarks" {
		t.Fatalf("got %q", message)
	}
}
