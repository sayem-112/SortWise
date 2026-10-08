package store

import (
	"context"
	"testing"

	"sortwise/internal/model"
)

func TestAcceptedCategoryProposalFilesTaggedBookmarks(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{sampleBookmark()}}); err != nil {
		t.Fatal(err)
	}
	var bookmarkID int64
	s.DB.QueryRow(`SELECT id FROM bookmarks`).Scan(&bookmarkID)
	tag, err := s.CreateTag(ctx, "postgres", "tool")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO bookmark_tags(bookmark_id,tag_id,ai_confidence) VALUES (?,?,0.9)`, bookmarkID, tag.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE bookmarks SET processing_status='completed'`); err != nil {
		t.Fatal(err)
	}

	stats, err := s.Dashboard(ctx)
	if err != nil || stats.NeedsReview != 1 {
		t.Fatalf("an analyzed bookmark without a category needs review: %d %v", stats.NeedsReview, err)
	}
	review, err := s.SearchBookmarks(ctx, model.BookmarkQuery{ProcessingState: "review"})
	if err != nil || review.Total != 1 {
		t.Fatalf("review filter: %d %v", review.Total, err)
	}

	proposal := model.CategoryProposal{Name: "Databases", Description: "Database engines", Tags: []string{"postgres"}}
	if err = s.ReplaceProposals(ctx, "category", []any{proposal}); err != nil {
		t.Fatal(err)
	}
	items, err := s.ListProposals(ctx)
	if err != nil || len(items) != 1 || items[0].Matches != 1 {
		t.Fatalf("list: %+v %v", items, err)
	}
	if err = s.AcceptProposal(ctx, items[0].ID); err != nil {
		t.Fatal(err)
	}
	var assigned int
	s.DB.QueryRow(`SELECT COUNT(*) FROM bookmark_categories bc JOIN categories c ON c.id=bc.category_id WHERE c.name='Databases' AND bc.ai_confidence IS NOT NULL`).Scan(&assigned)
	if assigned != 1 {
		t.Fatalf("expected the tagged bookmark to be filed, got %d", assigned)
	}
	stats, _ = s.Dashboard(ctx)
	if stats.NeedsReview != 0 {
		t.Fatalf("filed bookmark should leave review, got %d", stats.NeedsReview)
	}
	if err = s.AcceptProposal(ctx, items[0].ID); err != ErrProposalNotFound {
		t.Fatalf("accepting twice should fail, got %v", err)
	}
}

func TestAcceptedMergeProposalMergesTags(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.CreateTag(ctx, "llms", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTag(ctx, "llm", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceProposals(ctx, "merge", []any{model.MergeProposal{Source: "llms", Target: "llm"}}); err != nil {
		t.Fatal(err)
	}
	items, _ := s.ListProposals(ctx)
	if err := s.AcceptProposal(ctx, items[0].ID); err != nil {
		t.Fatal(err)
	}
	var remaining int
	s.DB.QueryRow(`SELECT COUNT(*) FROM tags`).Scan(&remaining)
	if remaining != 1 {
		t.Fatalf("expected one tag after merge, got %d", remaining)
	}
}

func TestAcceptingASuggestionRevivesATurnedOffStarter(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	category, err := s.reviveCategory(ctx, "Software Engineering", "Code and systems", nil)
	if err != nil {
		t.Fatal(err)
	}
	var active, total int
	s.DB.QueryRow(`SELECT active FROM categories WHERE id=?`, category.ID).Scan(&active)
	s.DB.QueryRow(`SELECT COUNT(*) FROM categories WHERE lower(name)='software engineering'`).Scan(&total)
	if active != 1 || total != 1 {
		t.Fatalf("expected the existing starter to be turned back on, got active=%d rows=%d", active, total)
	}
}
