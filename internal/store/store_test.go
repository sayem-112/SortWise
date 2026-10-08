package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"sortwise/internal/database"
	"sortwise/internal/model"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db)
}

func sampleBookmark() model.ImportedBookmark {
	return model.ImportedBookmark{Platform: "x", PostID: "1001", Author: "Ada", Username: "ada", Text: "Reliable queues", URL: "https://x.com/ada/status/1001", ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "1.0.0", Media: []model.ImportedMedia{{Kind: "image", URL: "https://pbs.twimg.com/media/demo.jpg", PreviewURL: "https://pbs.twimg.com/media/demo.jpg"}}}
}

func TestImportIsIdempotentAndQueuesChangedContent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	first, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{sampleBookmark()}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Inserted != 1 {
		t.Fatalf("expected insert, got %+v", first)
	}
	second, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{sampleBookmark()}})
	if err != nil {
		t.Fatal(err)
	}
	if second.Unchanged != 1 {
		t.Fatalf("expected unchanged, got %+v", second)
	}
	changed := sampleBookmark()
	changed.Text = "Reliable durable queues"
	third, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{changed}})
	if err != nil {
		t.Fatal(err)
	}
	if third.Updated != 1 {
		t.Fatalf("expected update, got %+v", third)
	}
	var bookmarks, jobs int
	s.DB.QueryRow(`SELECT COUNT(*) FROM bookmarks`).Scan(&bookmarks)
	s.DB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&jobs)
	if bookmarks != 1 || jobs != 2 {
		t.Fatalf("got %d bookmarks and %d jobs", bookmarks, jobs)
	}
}

func TestPairingCodeIsOneTimeAndTokensAreHashed(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.CreatePairingCode(ctx, "123456", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.PairExtension(ctx, "123456", "secret-token"); err != nil {
		t.Fatal(err)
	}
	if err := s.PairExtension(ctx, "123456", "another-token"); err == nil {
		t.Fatal("expected one-time code to fail")
	}
	if !s.AuthenticateExtension(ctx, "secret-token") {
		t.Fatal("expected valid token")
	}
	var stored string
	s.DB.QueryRow(`SELECT token_hash FROM extension_tokens`).Scan(&stored)
	if stored == "secret-token" {
		t.Fatal("token was stored in plaintext")
	}
}

func TestSearchUsesFTSAndArchiveIsReversible(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	item := sampleBookmark()
	item.Text = "A practical guide to durable background queues"
	_, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}})
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.SearchBookmarks(ctx, model.BookmarkQuery{Text: "durable queue"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Fatalf("expected search match, got %+v", page)
	}
	id := page.Items[0].ID
	bookmark, err := s.SetArchived(ctx, id, true)
	if err != nil || !bookmark.Archived {
		t.Fatalf("archive failed: %+v %v", bookmark, err)
	}
	page, _ = s.SearchBookmarks(ctx, model.BookmarkQuery{})
	if page.Total != 0 {
		t.Fatal("archived bookmark remained in default search")
	}
	bookmark, err = s.SetArchived(ctx, id, false)
	if err != nil || bookmark.Archived {
		t.Fatalf("unarchive failed: %+v %v", bookmark, err)
	}
}

func TestSearchSortsByPostedDateWithMissingDatesLast(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	oldDate, newDate := "2024-01-01T12:00:00Z", "2025-01-01T12:00:00Z"
	items := []model.ImportedBookmark{sampleBookmark(), sampleBookmark(), sampleBookmark()}
	items[0].PostID, items[0].URL, items[0].PostedAt = "1001", "https://x.com/ada/status/1001", &oldDate
	items[1].PostID, items[1].URL, items[1].PostedAt = "1002", "https://x.com/ada/status/1002", nil
	items[2].PostID, items[2].URL, items[2].PostedAt = "1003", "https://x.com/ada/status/1003", &newDate
	if _, err := s.Import(ctx, model.ImportBatch{Bookmarks: items}); err != nil {
		t.Fatal(err)
	}
	page, err := s.SearchBookmarks(ctx, model.BookmarkQuery{Sort: "posted_desc"})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{page.Items[0].PostID, page.Items[1].PostID, page.Items[2].PostID}; got[0] != "1003" || got[1] != "1001" || got[2] != "1002" {
		t.Fatalf("unexpected posted-date order: %v", got)
	}
}

func TestFTSExpressionIgnoresPunctuationAndSplitsLikeTheIndex(t *testing.T) {
	cases := map[string]string{
		"lamine":        `"lamine"*`,
		"react - hooks": `"react"* AND "hooks"*`,
		"c++":           `"c"`,
		"next.js":       `"next"* AND "js"*`,
		"...":           ``,
		"#ai @karpathy": `"ai"* AND "karpathy"*`,
	}
	for input, want := range cases {
		if got := ftsExpression(input); got != want {
			t.Errorf("ftsExpression(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSearchFindsBookmarksByTagAndCategory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{sampleBookmark()}}); err != nil {
		t.Fatal(err)
	}
	var bookmarkID int64
	s.DB.QueryRow(`SELECT id FROM bookmarks`).Scan(&bookmarkID)
	tag, err := s.CreateTag(ctx, "Lamine Yamal", "entity")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO bookmark_tags(bookmark_id,tag_id,ai_confidence) VALUES (?,?,0.9)`, bookmarkID, tag.ID); err != nil {
		t.Fatal(err)
	}
	result, err := s.SearchBookmarks(ctx, model.BookmarkQuery{Text: "lamine"})
	if err != nil || result.Total != 1 {
		t.Fatalf("search by tag: total=%d err=%v", result.Total, err)
	}
	if _, err = s.UpdateTag(ctx, tag.ID, "Yamal", ""); err != nil {
		t.Fatal(err)
	}
	// The old name becomes an alias, so both spellings still find the post.
	for _, query := range []string{"yamal", "lamine"} {
		if result, _ = s.SearchBookmarks(ctx, model.BookmarkQuery{Text: query}); result.Total != 1 {
			t.Fatalf("after rename, %q found %d", query, result.Total)
		}
	}
}
