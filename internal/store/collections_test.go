package store

import (
	"context"
	"errors"
	"testing"

	"sortwise/internal/model"
)

// posts saves one post per text and returns their IDs in the same order.
func posts(t *testing.T, s *Store, texts ...string) []int64 {
	t.Helper()
	batch := model.ImportBatch{}
	for index, text := range texts {
		item := sampleBookmark()
		item.PostID = string(rune('1'+index)) + "000"
		item.URL = "https://x.com/ada/status/" + item.PostID
		item.Text = text
		batch.Bookmarks = append(batch.Bookmarks, item)
	}
	if _, err := s.Import(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, len(texts))
	for index := range texts {
		if err := s.DB.QueryRow(`SELECT id FROM bookmarks WHERE post_id=?`, batch.Bookmarks[index].PostID).Scan(&ids[index]); err != nil {
			t.Fatal(err)
		}
	}
	return ids
}

func order(page model.BookmarkPage) []int64 {
	ids := []int64{}
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	return ids
}

func same(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func TestSearchByMeaningMergesWordAndMeaningMatches(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ids := posts(t, s, "Rust ownership explained", "Borrow checker tips", "Sourdough starter", "Rust in the kernel")
	rust, borrow, bread, kernel := ids[0], ids[1], ids[2], ids[3]

	// Words alone find the two posts that say "rust".
	words, err := s.SearchBookmarks(ctx, model.BookmarkQuery{Text: "rust"})
	if err != nil || words.Total != 2 {
		t.Fatalf("word search: %+v %v", order(words), err)
	}

	// By meaning, the borrow-checker post joins them, and the post that is
	// high in both rankings comes first.
	query := model.BookmarkQuery{Text: "rust", Hybrid: true, Semantic: []int64{kernel, borrow, 999}}
	page, err := s.SearchBookmarks(ctx, query)
	if err != nil || page.Total != 3 || page.Items[0].ID != kernel {
		t.Fatalf("hybrid search: %v total %d %v", order(page), page.Total, err)
	}
	for _, item := range page.Items {
		if want := map[int64]string{borrow: "meaning"}[item.ID]; item.Match != want {
			t.Fatalf("post %d match %q, want %q", item.ID, item.Match, want)
		}
		if item.ID == bread {
			t.Fatal("an unrelated post was returned")
		}
	}
	if count, _ := s.CountBookmarks(ctx, query); count != 3 {
		t.Fatalf("count: %d", count)
	}

	// Pages follow the same ranking and do not repeat a post.
	query.PageSize = 2
	first, _ := s.SearchBookmarks(ctx, query)
	query.Page = 2
	second, _ := s.SearchBookmarks(ctx, query)
	if got := append(order(first), order(second)...); !same(got, order(page)) {
		t.Fatalf("pages %v, want %v", got, order(page))
	}
	query.Page, query.PageSize = 0, 0

	// Another sort order keeps the same posts and reorders them.
	query.Sort = "imported_asc"
	if sorted, _ := s.SearchBookmarks(ctx, query); sorted.Total != 3 || !same(order(sorted), []int64{rust, borrow, kernel}) {
		t.Fatalf("sorted: %v", order(sorted))
	}
	query.Sort = ""

	// The other filters apply to posts found by meaning too.
	if _, err := s.SetArchived(ctx, borrow, true); err != nil {
		t.Fatal(err)
	}
	if filtered, _ := s.SearchBookmarks(ctx, query); filtered.Total != 2 {
		t.Fatalf("an archived post found by meaning was returned: %v", order(filtered))
	}

	// Sorted another way with no post found by meaning, and with only those.
	query.Sort = "imported_asc"
	query.Semantic = nil
	if sorted, _ := s.SearchBookmarks(ctx, query); !same(order(sorted), []int64{rust, kernel}) {
		t.Fatalf("sorted words only: %v", order(sorted))
	}
	query.Text, query.Semantic = "", []int64{bread, rust}
	if sorted, _ := s.SearchBookmarks(ctx, query); !same(order(sorted), []int64{rust, bread}) {
		t.Fatalf("sorted meaning only: %v", order(sorted))
	}

	// With no post close in meaning, it is the word search.
	if plain, _ := s.SearchBookmarks(ctx, model.BookmarkQuery{Text: "rust", Hybrid: true}); plain.Total != 2 {
		t.Fatalf("hybrid with no meaning matches: %v", order(plain))
	}
}

func TestCollections(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if _, err := s.CreateCollection(ctx, "Everything", model.CollectionQuery{}); !errors.Is(err, ErrCollectionEmpty) {
		t.Fatalf("a collection with no search or filter: %v", err)
	}
	for _, bad := range []model.CollectionQuery{{Text: "x", Status: "nope"}, {Text: "x", Sort: "nope"}, {List: "abc"}, {Author: "not a handle"}} {
		if _, err := s.CreateCollection(ctx, "Bad", bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}

	created, err := s.CreateCollection(ctx, " Rust reading ", model.CollectionQuery{Text: "  rust   async ", Mode: "meaning", Tags: []int64{7, 7, -1}, Author: "@ada", Sort: "relevance"})
	if err != nil {
		t.Fatal(err)
	}
	want := model.CollectionQuery{Text: "rust async", Mode: "meaning", Categories: []int64{}, Tags: []int64{7}, Author: "ada", Sort: "relevance"}
	if created.Name != "Rust reading" || created.Query.Text != want.Text || created.Query.Mode != "meaning" || !same(created.Query.Tags, want.Tags) || created.Query.Author != "ada" || created.Color == "" {
		t.Fatalf("created: %+v", created)
	}
	if _, err := s.CreateCollection(ctx, "rust READING", model.CollectionQuery{Text: "x"}); !errors.Is(err, ErrCollectionExists) {
		t.Fatalf("duplicate name: %v", err)
	}
	// Meaning needs words to compare; without them it is an ordinary filter.
	filterOnly, err := s.CreateCollection(ctx, "Needs review", model.CollectionQuery{Mode: "meaning", Status: "review", Sort: "relevance"})
	if err != nil || filterOnly.Query.Mode != "" || filterOnly.Query.Sort != "" || filterOnly.Color == created.Color {
		t.Fatalf("filter-only collection: %+v %v", filterOnly, err)
	}

	name := "Systems"
	updated, err := s.UpdateCollection(ctx, created.ID, model.CollectionUpdate{Name: &name, Query: &model.CollectionQuery{List: "none"}})
	if err != nil || updated.Name != "Systems" || updated.Query.List != "none" || updated.Query.Text != "" {
		t.Fatalf("updated: %+v %v", updated, err)
	}
	if query := CollectionBookmarkQuery(updated.Query); !query.NoList || query.ListID != 0 {
		t.Fatalf("query for 'in no list': %+v", query)
	}
	if query := CollectionBookmarkQuery(model.CollectionQuery{List: "4", Categories: []int64{2}}); query.ListID != 4 || len(query.CategoryIDs) != 1 {
		t.Fatalf("query for a list: %+v", query)
	}

	all, err := s.Collections(ctx)
	if err != nil || len(all) != 2 || all[0].Name != "Needs review" {
		t.Fatalf("list: %+v %v", all, err)
	}

	// A deleted collection's number is not reused.
	if err := s.DeleteCollection(ctx, filterOnly.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCollection(ctx, filterOnly.ID); err == nil {
		t.Fatal("deleting twice should report it is gone")
	}
	again, _ := s.CreateCollection(ctx, "Again", model.CollectionQuery{Text: "x"})
	if again.ID <= filterOnly.ID {
		t.Fatalf("reused collection number %d", again.ID)
	}
}

func TestMergingATagKeepsCollectionsWorking(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	llm, _ := s.CreateTag(ctx, "llm", "topic")
	llms, _ := s.CreateTag(ctx, "large language models", "topic")
	other, _ := s.CreateTag(ctx, "rust", "topic")
	saved, err := s.CreateCollection(ctx, "AI", model.CollectionQuery{Tags: []int64{llms.ID, llm.ID, other.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MergeTag(ctx, llms.ID, llm.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetCollection(ctx, saved.ID)
	if !same(after.Query.Tags, []int64{llm.ID, other.ID}) {
		t.Fatalf("after merging: %v, want %v", after.Query.Tags, []int64{llm.ID, other.ID})
	}
	// A deleted tag's number can go to a new tag, so it leaves saved searches;
	// a search with nothing else in it goes with it.
	only, _ := s.CreateCollection(ctx, "Just rust", model.CollectionQuery{Tags: []int64{other.ID}})
	if err := s.DeleteUnusedTag(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	if after, _ = s.GetCollection(ctx, saved.ID); !same(after.Query.Tags, []int64{llm.ID}) {
		t.Fatalf("after deleting a tag: %v", after.Query.Tags)
	}
	if _, err := s.GetCollection(ctx, only.ID); err == nil {
		t.Fatal("a collection left with no filter should be removed")
	}
	// A change that is partly wrong changes nothing.
	name, bad := "Renamed", "no-such-color"
	if _, err := s.UpdateCollection(ctx, saved.ID, model.CollectionUpdate{Name: &name, Color: &bad}); err == nil {
		t.Fatal("accepted an unknown color")
	}
	if after, _ = s.GetCollection(ctx, saved.ID); after.Name != "AI" {
		t.Fatalf("a rejected update renamed the collection to %q", after.Name)
	}
}

func TestEmbeddingCandidates(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ids := posts(t, s, "Reliable queues", "")
	// Posts the AI has not read yet wait, so their summary is part of the vector.
	if waiting, _ := s.EmbeddingCandidates(ctx, "m1", 10); len(waiting) != 0 {
		t.Fatalf("unanalyzed posts offered: %+v", waiting)
	}
	s.DB.Exec(`UPDATE bookmarks SET processing_status='completed'`)
	s.DB.Exec(`DELETE FROM media`)
	candidates, err := s.EmbeddingCandidates(ctx, "m1", 10)
	if err != nil || len(candidates) != 2 {
		t.Fatalf("candidates: %+v %v", candidates, err)
	}
	for _, candidate := range candidates {
		if candidate.Current || (candidate.ID == ids[1]) != (candidate.Text == "") {
			t.Fatalf("candidate: %+v", candidate)
		}
	}
	saved := []SavedEmbedding{{ID: candidates[0].ID, Hash: candidates[0].Hash, Vector: []byte{1, 2}}, {ID: candidates[1].ID, Hash: candidates[1].Hash}, {ID: 999, Hash: "gone"}}
	if err := s.SaveEmbeddings(ctx, "m1", "2000-01-01 00:00:00", saved); err != nil {
		t.Fatal(err)
	}
	// Only the post with something to describe is searchable; both are done.
	if count, version, _ := s.EmbeddingSignature(ctx, "m1"); count != 1 || version != 1 {
		t.Fatalf("signature: %d %d", count, version)
	}
	if indexed, total, _ := s.EmbeddingProgress(ctx, "m1"); indexed != 2 || total != 2 {
		t.Fatalf("progress: %d of %d", indexed, total)
	}
	// Checked long ago, so both are looked at again, and found unchanged.
	again, _ := s.EmbeddingCandidates(ctx, "m1", 10)
	if len(again) != 2 || !again[0].Current || !again[1].Current {
		t.Fatalf("recheck: %+v", again)
	}
	if err := s.TouchEmbeddings(ctx, []int64{ids[0], ids[1]}, "2999-01-01 00:00:00"); err != nil {
		t.Fatal(err)
	}
	if settled, _ := s.EmbeddingCandidates(ctx, "m1", 10); len(settled) != 0 {
		t.Fatalf("settled posts offered again: %+v", settled)
	}
	// Another model starts over.
	if other, _ := s.EmbeddingCandidates(ctx, "m2", 10); len(other) != 2 || other[0].Current {
		t.Fatalf("another model: %+v", other)
	}
}
