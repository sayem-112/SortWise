package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"sortwise/internal/model"
)

// importAt saves count posts as received at the given time.
func importAt(t *testing.T, s *Store, first int, count int, at string) {
	t.Helper()
	batch := model.ImportBatch{}
	for index := 0; index < count; index++ {
		item := sampleBookmark()
		item.PostID = fmt.Sprint(first + index)
		item.URL = "https://x.com/ada/status/" + item.PostID
		item.ExtractedAt = at
		batch.Bookmarks = append(batch.Bookmarks, item)
	}
	if _, err := s.Import(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
}

func TestSavedByDaySetsTheBackCatalogueApart(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	importAt(t, s, 5000, 40, "2026-03-01T10:00:00Z") // the first sync: years of bookmarks
	importAt(t, s, 6000, 3, "2026-05-02T09:00:00Z")
	importAt(t, s, 7000, 1, "2026-05-02T23:30:00Z") // still 2 May in UTC, 3 May in UTC+2
	importAt(t, s, 8000, 2, "2025-01-10T12:00:00Z") // before the window

	days, err := s.SavedByDay(ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if days.Total != 46 || days.BackCatalogue == nil || *days.BackCatalogue != (DayCount{Date: "2026-03-01", Count: 40}) {
		t.Fatalf("back catalogue: %+v total %d", days.BackCatalogue, days.Total)
	}
	if len(days.Days) != 1 || days.Days[0] != (DayCount{Date: "2026-05-02", Count: 4}) {
		t.Fatalf("days: %+v", days.Days)
	}

	// Days fall in the person's own time zone.
	east := time.FixedZone("UTC+2", 2*60*60)
	days, err = s.SavedByDay(ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, east), east)
	if err != nil {
		t.Fatal(err)
	}
	if len(days.Days) != 2 || days.Days[0] != (DayCount{Date: "2026-05-02", Count: 3}) || days.Days[1] != (DayCount{Date: "2026-05-03", Count: 1}) {
		t.Fatalf("local days: %+v", days.Days)
	}
}

func TestSavedByDayWithoutABackCatalogue(t *testing.T) {
	s := testStore(t)
	importAt(t, s, 5000, 5, "2026-06-01T10:00:00Z")
	importAt(t, s, 6000, 4, "2026-06-02T10:00:00Z")
	days, err := s.SavedByDay(context.Background(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	// Too few posts on the busiest day to call it a back catalogue.
	if days.BackCatalogue != nil || len(days.Days) != 2 {
		t.Fatalf("%+v", days)
	}
}

func TestMainCategoryPrefersThePersonsChoice(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id := importPost(t, s, "9001", "Ada", "ada", "Engines")
	other := importPost(t, s, "9002", "Ada", "ada", "Looms")
	ai, _ := s.CreateCategory(ctx, "Machines", "", nil)
	mine, _ := s.CreateCategory(ctx, "History", "", nil)
	s.DB.Exec(`INSERT INTO bookmark_categories(bookmark_id,category_id,ai_confidence) VALUES (?,?,0.95)`, id, ai.ID)
	if _, err := s.UpdateBookmarkMetadata(ctx, id, model.BookmarkMetadataUpdate{CategoryDecisions: []model.AssignmentDecision{{ID: mine.ID, State: "added"}}}); err != nil {
		t.Fatal(err)
	}
	s.DB.Exec(`UPDATE bookmarks SET archived_at=CURRENT_TIMESTAMP WHERE id=?`, other)

	main, err := s.MainCategories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(main) != 1 || main[id] != mine.ID {
		t.Fatalf("got %v, want post %d in %d and the archived post left out", main, id, mine.ID)
	}
}
