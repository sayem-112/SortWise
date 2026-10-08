package store

import (
	"context"
	"fmt"
	"sortwise/internal/model"
	"testing"
)

func TestDashboardUsesEffectiveAssignments(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tag, err := s.CreateTag(ctx, "mcp", "")
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 10; index++ {
		item := sampleBookmark()
		item.PostID = fmt.Sprintf("%d", 2000+index)
		item.URL = "https://x.com/ada/status/" + item.PostID
		if _, err = s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
			t.Fatal(err)
		}
		var id int64
		s.DB.QueryRow(`SELECT id FROM bookmarks WHERE post_id=?`, item.PostID).Scan(&id)
		if _, err = s.UpdateBookmarkMetadata(ctx, id, model.BookmarkMetadataUpdate{TagDecisions: []model.AssignmentDecision{{ID: tag.ID, State: "added"}}}); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := s.Dashboard(ctx)
	if err != nil || stats.Total != 10 || len(stats.TopTags) != 1 {
		t.Fatalf("stats: %+v %v", stats, err)
	}
}
