package store

import (
	"context"
	"sortwise/internal/model"
	"testing"
)

func TestCategoryHierarchyRejectsCycles(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	parent, err := s.CreateCategory(ctx, "Programming", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.CreateCategory(ctx, "Go", "", &parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateCategory(ctx, parent.ID, "Programming", "", &child.ID, nil)
	if err == nil {
		t.Fatal("expected cycle rejection")
	}
}

func TestTagRenameCreatesAliasAndMergePreservesAssignments(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	first, err := s.CreateTag(ctx, "golang", "tool")
	if err != nil {
		t.Fatal(err)
	}
	first, err = s.UpdateTag(ctx, first.ID, "go", "")
	if err != nil || len(first.Aliases) != 1 {
		t.Fatalf("rename: %+v %v", first, err)
	}
	target, err := s.CreateTag(ctx, "programming", "")
	if err != nil {
		t.Fatal(err)
	}
	bookmark := sampleBookmark()
	if _, err = s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{bookmark}}); err != nil {
		t.Fatal(err)
	}
	var bookmarkID int64
	s.DB.QueryRow(`SELECT id FROM bookmarks`).Scan(&bookmarkID)
	if _, err = s.UpdateBookmarkMetadata(ctx, bookmarkID, model.BookmarkMetadataUpdate{TagDecisions: []model.AssignmentDecision{{ID: first.ID, State: "added"}}}); err != nil {
		t.Fatal(err)
	}
	if err = s.MergeTag(ctx, first.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = s.DB.QueryRow(`SELECT manual_state FROM bookmark_tags WHERE bookmark_id=? AND tag_id=?`, bookmarkID, target.ID).Scan(&state); err != nil || state != "added" {
		t.Fatalf("assignment not preserved: %q %v", state, err)
	}
}
