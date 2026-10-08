package ai

import (
	"context"
	"errors"
	"path/filepath"
	"sortwise/internal/database"
	"sortwise/internal/model"
	"sortwise/internal/store"
	"sortwise/internal/xembed"
	"testing"
	"time"
)

type fakeCredentials struct{ key string }

func (f fakeCredentials) Get(string) (string, string, error) { return f.key, "test", nil }
func (f fakeCredentials) Set(string, string) error           { return nil }
func (f fakeCredentials) Delete(string) error                { return nil }

type fakeProvider struct{}

func (fakeProvider) Analyze(context.Context, AnalysisInput) (AnalysisResult, error) {
	return AnalysisResult{Summary: "A guide to reliable background queues.", Categories: []Classification{{Name: "Technology", Confidence: .95}}, Tags: []Classification{{Name: "queues", Confidence: .9}, {Name: "retries", Confidence: .8}, {Name: "technology", Confidence: .7}}}, nil
}

func TestWorkerCompletesImportedJob(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "worker.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := store.New(db)
	item := model.ImportedBookmark{Platform: "x", PostID: "1001", Author: "Ada", Username: "ada", Text: "Reliable queues", URL: "https://x.com/ada/status/1001", ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "1.0.0"}
	if _, err = s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(db, fakeCredentials{key: "fake"}, func(context.Context, string) (Provider, error) { return fakeProvider{}, nil })
	worker.minRequestInterval = 0
	processed, err := worker.ProcessOne(ctx)
	if err != nil || !processed {
		t.Fatalf("process: %v %v", processed, err)
	}
	var status, summary string
	if err := db.QueryRow(`SELECT b.processing_status,e.ai_summary FROM bookmarks b JOIN enrichments e ON e.bookmark_id=b.id`).Scan(&status, &summary); err != nil {
		t.Fatal(err)
	}
	if status != "completed" || summary == "" {
		t.Fatalf("got %s %q", status, summary)
	}
	var categories, tags int
	db.QueryRow(`SELECT COUNT(*) FROM bookmark_categories WHERE ai_confidence IS NOT NULL`).Scan(&categories)
	db.QueryRow(`SELECT COUNT(*) FROM bookmark_tags WHERE ai_confidence IS NOT NULL`).Scan(&tags)
	// The fake provider's "technology" tag repeats a category name and is dropped.
	if categories != 1 || tags != 2 {
		t.Fatalf("got %d categories %d tags", categories, tags)
	}
}

func TestReprocessingPreservesManualSummaryAndDecisions(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "manual.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := store.New(db)
	item := model.ImportedBookmark{Platform: "x", PostID: "2001", Author: "Ada", Username: "ada", Text: "Reliable queues", URL: "https://x.com/ada/status/2001", ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "1.0.0"}
	if _, err = s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(db, fakeCredentials{key: "fake"}, func(context.Context, string) (Provider, error) { return fakeProvider{}, nil })
	worker.minRequestInterval = 0
	if _, err = worker.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	bookmark, err := s.SearchBookmarks(ctx, model.BookmarkQuery{})
	if err != nil {
		t.Fatal(err)
	}
	manual := "My corrected summary"
	var backendID, researchID int64
	db.QueryRow(`SELECT id FROM categories WHERE name='Technology'`).Scan(&backendID)
	db.QueryRow(`SELECT id FROM categories WHERE name='Research'`).Scan(&researchID)
	if _, err = s.UpdateBookmarkMetadata(ctx, bookmark.Items[0].ID, model.BookmarkMetadataUpdate{Summary: &manual, CategoryDecisions: []model.AssignmentDecision{{ID: backendID, State: "removed"}, {ID: researchID, State: "added"}}}); err != nil {
		t.Fatal(err)
	}
	var hash string
	db.QueryRow(`SELECT content_hash FROM bookmarks WHERE id=?`, bookmark.Items[0].ID).Scan(&hash)
	if _, err = db.Exec(`INSERT INTO jobs(bookmark_id,input_hash) VALUES (?,?)`, bookmark.Items[0].ID, hash+":manual"); err != nil {
		t.Fatal(err)
	}
	if _, err = worker.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	updated, err := s.GetBookmark(ctx, bookmark.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Summary != manual {
		t.Fatalf("manual summary overwritten: %q", updated.Summary)
	}
	if len(updated.Categories) != 1 || updated.Categories[0].Name != "Research" || !updated.Categories[0].Manual {
		t.Fatalf("manual categories overwritten: %+v", updated.Categories)
	}
}

type rateLimitedProvider struct{}

func (rateLimitedProvider) Analyze(context.Context, AnalysisInput) (AnalysisResult, error) {
	return AnalysisResult{}, errors.New("Error 429 RESOURCE_EXHAUSTED: quota exceeded. Please retry in 12.5s")
}

func TestRateLimitDoesNotConsumeJobAttempts(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "rate-limit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := store.New(db)
	item := model.ImportedBookmark{Platform: "x", PostID: "3001", Author: "Ada", Username: "ada", Text: "Rate limits", URL: "https://x.com/ada/status/3001", ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "1.0.0"}
	if _, err = s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(db, fakeCredentials{key: "fake"}, func(context.Context, string) (Provider, error) { return rateLimitedProvider{}, nil })
	worker.minRequestInterval = 0
	processed, processErr := worker.ProcessOne(ctx)
	if !processed || !isRateLimitError(processErr) {
		t.Fatalf("expected rate limit, got processed=%v err=%v", processed, processErr)
	}
	var status string
	var attempts int
	if err := db.QueryRow(`SELECT status,attempts FROM jobs`).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempts != 0 {
		t.Fatalf("rate limit consumed job attempt: status=%s attempts=%d", status, attempts)
	}
}

func TestRetryDelayUsesProviderHint(t *testing.T) {
	delay := retryDelay(errors.New("Please retry in 8.5s"))
	if delay != 10*time.Second+500*time.Millisecond {
		t.Fatalf("unexpected retry delay: %s", delay)
	}
}

type panicProvider struct{}

func (panicProvider) Analyze(context.Context, AnalysisInput) (AnalysisResult, error) {
	panic("an empty post must not reach the provider")
}

func TestWorkerSkipsPostsWithNothingToAnalyze(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	item := model.ImportedBookmark{Platform: "x", PostID: "7001", Author: "Ada", Username: "ada", Text: "", URL: "https://x.com/ada/status/7001", ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "1.0.0"}
	if _, err = store.New(db).Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(db, fakeCredentials{key: "fake"}, func(context.Context, string) (Provider, error) { return panicProvider{}, nil })
	worker.minRequestInterval = 0
	if processed, err := worker.ProcessOne(ctx); !processed || err != nil {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	var status string
	if err := db.QueryRow(`SELECT processing_status FROM bookmarks`).Scan(&status); err != nil || status != "completed" {
		t.Fatalf("empty post should be completed without AI, got %q %v", status, err)
	}
}

type fakeFetcher struct{}

func (fakeFetcher) Fetch(context.Context, string) (xembed.Post, error) {
	return xembed.Post{Text: "https://t.co/x", Article: &xembed.Article{ID: "42", Title: "Cutting the black net", PreviewText: "Advice from months of DMs", CoverURL: "https://pbs.twimg.com/media/cover.jpg"}}, nil
}

type capturingProvider struct{ input *AnalysisInput }

func (c capturingProvider) Analyze(_ context.Context, input AnalysisInput) (AnalysisResult, error) {
	*c.input = input
	return fakeProvider{}.Analyze(context.Background(), input)
}

func TestWorkerFillsInArticlesBeforeAnalysis(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "article.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	item := model.ImportedBookmark{Platform: "x", PostID: "8001", Author: "Yacine", Username: "yacine", Text: "", URL: "https://x.com/yacine/status/8001", ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "1.0.0"}
	if _, err = store.New(db).Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
		t.Fatal(err)
	}
	var seen AnalysisInput
	worker := NewWorker(db, fakeCredentials{key: "fake"}, func(context.Context, string) (Provider, error) { return capturingProvider{input: &seen}, nil })
	worker.minRequestInterval = 0
	worker.SetPostFetcher(fakeFetcher{})
	if processed, err := worker.ProcessOne(ctx); !processed || err != nil {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if seen.CardTitle != "Cutting the black net" || seen.CardDescription == "" || len(seen.Media) != 1 {
		t.Fatalf("model did not receive the article: %+v", seen)
	}
	result, err := store.New(db).SearchBookmarks(ctx, model.BookmarkQuery{Text: "black net"})
	if err != nil || result.Total != 1 {
		t.Fatalf("article title should be searchable: %d %v", result.Total, err)
	}
}

func TestHydrationSweepFillsArticlesWhileAIIsPaused(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "sweep.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	item := model.ImportedBookmark{Platform: "x", PostID: "9001", Author: "Yacine", Username: "yacine", Text: "https://t.co/abc", URL: "https://x.com/yacine/status/9001", ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "1.0.0"}
	if _, err = store.New(db).Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES ('ai_paused','true')`); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(db, fakeCredentials{key: "fake"}, func(context.Context, string) (Provider, error) { return panicProvider{}, nil })
	worker.SetPostFetcher(fakeFetcher{})
	worker.hydratePending(ctx)
	var title string
	var checked bool
	if err := db.QueryRow(`SELECT COALESCE(json_extract(visible_context_json,'$.card.title'),''),COALESCE(json_extract(visible_context_json,'$.embedChecked'),0) FROM bookmarks`).Scan(&title, &checked); err != nil {
		t.Fatal(err)
	}
	if title != "Cutting the black net" || !checked {
		t.Fatalf("sweep did not fill in the article: title=%q checked=%v", title, checked)
	}
}
