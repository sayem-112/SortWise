package store

import (
	"context"
	"strings"
	"testing"

	"sortwise/internal/model"
)

func graphqlVersion(item model.ImportedBookmark) model.ImportedBookmark {
	item.ExtractorVersion = "x-graphql-1"
	item.Raw = []byte(`{"rest_id":"1001"}`)
	return item
}

func jobCount(t *testing.T, s *Store) int {
	t.Helper()
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestUpgradeFromPageReadingKeepsAnalysisUnlessContentGrew(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{sampleBookmark()}}); err != nil {
		t.Fatal(err)
	}
	s.DB.Exec(`UPDATE bookmarks SET processing_status='completed'`)

	// Same post, small differences only: better data, no new AI request.
	same := graphqlVersion(sampleBookmark())
	same.Text = "Reliable  queues"
	same.Media[0].AltText = "diagram"
	result, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{same}, Source: "sync"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Upgraded != 1 || result.Updated != 0 || jobCount(t, s) != 1 {
		t.Fatalf("expected a quiet upgrade, got %+v with %d jobs", result, jobCount(t, s))
	}
	var status, raw, source string
	s.DB.QueryRow(`SELECT processing_status, raw_payload_json FROM bookmarks`).Scan(&status, &raw)
	s.DB.QueryRow(`SELECT source FROM imports ORDER BY id DESC LIMIT 1`).Scan(&source)
	if status != "completed" || raw != `{"rest_id":"1001"}` || source != "sync" {
		t.Fatalf("status %q raw %q source %q", status, raw, source)
	}

	// Once read from X's data, later changes are normal updates.
	longer := graphqlVersion(sampleBookmark())
	longer.Text = "Reliable queues " + strings.Repeat("and more detail ", 10)
	result, err = s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{longer}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 || jobCount(t, s) != 2 {
		t.Fatalf("expected an update with a new job, got %+v", result)
	}
}

func TestUpgradeRequeuesWhenTheLongPostWasCutOff(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{sampleBookmark()}})
	full := graphqlVersion(sampleBookmark())
	full.Text = "Reliable queues " + strings.Repeat("the rest of a long post ", 6)
	result, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{full}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 || jobCount(t, s) != 2 {
		t.Fatalf("expected the AI to read the full post, got %+v", result)
	}
}

func TestRemovedOnXIsMarkedAndClearedWhenSeenAgain(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	item := sampleBookmark()
	item.Media[0].Kind = "video_poster"
	item.Media[0].VideoURL = "https://video.twimg.com/demo.mp4"
	if _, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
		t.Fatal(err)
	}
	var id int64
	s.DB.QueryRow(`SELECT id FROM bookmarks WHERE post_id='1001'`).Scan(&id)
	marked, err := s.MarkRemovedOnX(ctx, []string{"1001", "not-a-number", "999"})
	if err != nil || marked != 1 {
		t.Fatalf("marked %d, %v", marked, err)
	}
	saved, err := s.GetBookmark(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RemovedOnXAt == nil || saved.Media[0].VideoURL != "https://video.twimg.com/demo.mp4" {
		t.Fatalf("expected removal mark and video URL, got %+v", saved)
	}
	s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}})
	saved, _ = s.GetBookmark(ctx, saved.ID)
	if saved.RemovedOnXAt != nil {
		t.Fatal("bookmarking the post again should clear the mark")
	}
}

func TestVideoURLMustBeHostedByX(t *testing.T) {
	item := sampleBookmark()
	item.Media[0].VideoURL = "https://evil.example/video.mp4"
	if err := ValidateImportedBookmark(item); err == nil {
		t.Fatal("expected a non-X video URL to be rejected")
	}
}
