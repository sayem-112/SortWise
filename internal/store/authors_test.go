package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"sortwise/internal/model"
)

func importPost(t *testing.T, s *Store, postID, author, username, text string) int64 {
	t.Helper()
	item := sampleBookmark()
	item.PostID = postID
	item.Author = author
	item.Username = username
	item.Text = text
	item.URL = "https://x.com/" + username + "/status/" + postID
	if _, err := s.Import(context.Background(), model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
		t.Fatal(err)
	}
	var id int64
	s.DB.QueryRow(`SELECT id FROM bookmarks WHERE post_id=?`, postID).Scan(&id)
	return id
}

func authors(t *testing.T, s *Store, query model.AuthorQuery) model.AuthorPage {
	t.Helper()
	page, err := s.Authors(context.Background(), query)
	if err != nil {
		t.Fatalf("authors %+v: %v", query, err)
	}
	return page
}

func TestAuthorsGroupByHandleWhateverItsCase(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	importPost(t, s, "9001", "Ada", "ada", "Engines")
	// Same person, renamed and with the handle capitalized differently.
	s.DB.Exec(`UPDATE bookmarks SET imported_at='2026-01-01 00:00:00' WHERE post_id='9001'`)
	importPost(t, s, "9002", "Ada Lovelace", "Ada", "Notes on the engine")
	archived := importPost(t, s, "9003", "Ada Lovelace", "Ada", "Archived note")
	importPost(t, s, "9004", "Grace Hopper", "grace", "Compilers")
	if _, err := s.SetArchived(ctx, archived, true); err != nil {
		t.Fatal(err)
	}

	page := authors(t, s, model.AuthorQuery{})
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("authors: %+v", page)
	}
	// Most saved first; name and handle are the ones on the latest post; archived is not counted.
	if first := page.Items[0]; first.Username != "Ada" || first.Name != "Ada Lovelace" || first.Count != 2 {
		t.Fatalf("first author: %+v", first)
	}
	if found := authors(t, s, model.AuthorQuery{Text: "@GRA"}); found.Total != 1 || found.Items[0].Username != "grace" {
		t.Fatalf("search by handle: %+v", found)
	}
	// Searching a name only some of their posts carry still counts all of them.
	if found := authors(t, s, model.AuthorQuery{Text: "lovelace"}); found.Total != 1 || found.Items[0].Count != 2 || found.Items[0].Name != "Ada Lovelace" {
		t.Fatalf("search by a newer name: %+v", found)
	}
	if found := authors(t, s, model.AuthorQuery{Text: "100%"}); found.Total != 0 {
		t.Fatalf("percent must be literal: %+v", found)
	}
	if found := authors(t, s, model.AuthorQuery{Text: "@"}); found.Total != 0 || len(found.Items) != 0 {
		t.Fatalf("a lone @ matches nobody: %+v", found)
	}
	if byName := authors(t, s, model.AuthorQuery{Sort: "name"}); byName.Items[0].Name != "Ada Lovelace" || byName.Items[1].Name != "Grace Hopper" {
		t.Fatalf("by name: %+v", byName.Items)
	}

	posts, err := s.SearchBookmarks(ctx, model.BookmarkQuery{Author: "@ADA"})
	if err != nil || posts.Total != 2 {
		t.Fatalf("posts by author: %+v %v", posts.Total, err)
	}

	profile, err := s.AuthorProfile(ctx, "ADA")
	if err != nil || profile.Count != 2 || profile.Name != "Ada Lovelace" || profile.Username != page.Items[0].Username {
		t.Fatalf("profile: %+v %v", profile, err)
	}
	if _, err := s.AuthorProfile(ctx, "nobody"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing author: %v", err)
	}
}

func TestAuthorSearchIgnoresCaseAndAccentsInAnyScript(t *testing.T) {
	s := testStore(t)
	importPost(t, s, "9051", "Émile Zola", "emilez", "J'accuse")
	importPost(t, s, "9052", "山田太郎", "yamada", "継続は力なり")
	for _, text := range []string{"émile", "EMILE", "zola", "milez"} {
		if found := authors(t, s, model.AuthorQuery{Text: text}); found.Total != 1 || found.Items[0].Username != "emilez" {
			t.Fatalf("%q: %+v", text, found)
		}
	}
	if found := authors(t, s, model.AuthorQuery{Text: "山田"}); found.Total != 1 || found.Items[0].Username != "yamada" {
		t.Fatalf("non-Latin name: %+v", found)
	}
}

func TestAuthorsPageWithoutRepeatsWhenCountsTie(t *testing.T) {
	s := testStore(t)
	for index, handle := range []string{"delta", "alpha", "charlie", "bravo", "echo"} {
		importPost(t, s, "906"+string(rune('0'+index)), strings.ToUpper(handle), handle, "post")
	}
	// All five have one post saved at the same moment.
	s.DB.Exec(`UPDATE bookmarks SET imported_at='2026-02-02 00:00:00'`)
	seen := map[string]bool{}
	for page := 1; page <= 3; page++ {
		for _, item := range authors(t, s, model.AuthorQuery{Page: page, PageSize: 2}).Items {
			if seen[item.Username] {
				t.Fatalf("%s listed twice", item.Username)
			}
			seen[item.Username] = true
		}
	}
	if len(seen) != 5 {
		t.Fatalf("saw %d of 5 authors: %v", len(seen), seen)
	}
}

func TestAuthorProfileCountsTheirTopTags(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tag, err := s.CreateTag(ctx, "compilers", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, postID := range []string{"9101", "9102"} {
		id := importPost(t, s, postID, "Grace Hopper", "grace", "COBOL "+postID)
		if _, err := s.UpdateBookmarkMetadata(ctx, id, model.BookmarkMetadataUpdate{TagDecisions: []model.AssignmentDecision{{ID: tag.ID, State: "added"}}}); err != nil {
			t.Fatal(err)
		}
	}
	s.DB.Exec(`UPDATE bookmarks SET posted_at='2025-03-01T10:00:00Z' WHERE post_id='9101'`)
	s.DB.Exec(`UPDATE bookmarks SET posted_at='2026-06-01T10:00:00Z' WHERE post_id='9102'`)
	profile, err := s.AuthorProfile(ctx, "grace")
	if err != nil || len(profile.TopTags) != 1 || profile.TopTags[0].Name != "compilers" || profile.TopTags[0].Count != 2 {
		t.Fatalf("top tags: %+v %v", profile, err)
	}
	if profile.FirstPostedAt != "2025-03-01T10:00:00Z" || profile.LatestPostedAt != "2026-06-01T10:00:00Z" {
		t.Fatalf("posted range: %+v", profile)
	}
}

func TestQuickSearchMarksWhereEachPostMatched(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	importPost(t, s, "9201", "Ada Lovelace", "ada", "The analytical engine weaves algebraic patterns")
	tagged := importPost(t, s, "9202", "Grace Hopper", "grace", "A ship in port is safe")
	hidden := importPost(t, s, "9203", "Charles Babbage", "babbage", "The difference engine, archived")
	tag, _ := s.CreateTag(ctx, "engine", "")
	if _, err := s.UpdateBookmarkMetadata(ctx, tagged, model.BookmarkMetadataUpdate{TagDecisions: []model.AssignmentDecision{{ID: tag.ID, State: "added"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetArchived(ctx, hidden, true); err != nil {
		t.Fatal(err)
	}

	result, err := s.QuickSearch(ctx, "engine", 0)
	if err != nil || result.Total != 2 || len(result.Bookmarks) != 2 {
		t.Fatalf("quick search (archived posts stay out): %+v %v", result, err)
	}
	byHandle := map[string]model.QuickBookmark{}
	for _, item := range result.Bookmarks {
		byHandle[item.Username] = item
	}
	if item := byHandle["ada"]; !strings.Contains(item.Text, MatchStart+"engine"+MatchEnd) || item.Taxonomy != "" || item.Summary != "" {
		t.Fatalf("text match: %+v", item)
	}
	// Matched by its tag: the post text carries no mark, the tag excerpt does.
	if item := byHandle["grace"]; item.Text == "" || strings.Contains(item.Text, MatchStart) || !strings.Contains(item.Taxonomy, MatchStart+"engine"+MatchEnd) {
		t.Fatalf("tag match: %+v", item)
	}

	// A name finds the author, and marks the name on their posts.
	byName, err := s.QuickSearch(ctx, "grace", 0)
	if err != nil || len(byName.Authors) != 1 || byName.Authors[0].Username != "grace" {
		t.Fatalf("authors: %+v %v", byName, err)
	}
	if len(byName.Bookmarks) != 1 || !strings.Contains(byName.Bookmarks[0].AuthorMatch, MatchStart+"Grace"+MatchEnd) {
		t.Fatalf("name match on the post: %+v", byName.Bookmarks)
	}
	// Nothing typed, or only punctuation, finds nothing and does not fail.
	if empty, err := s.QuickSearch(ctx, "  ", 0); err != nil || len(empty.Bookmarks) != 0 || len(empty.Authors) != 0 {
		t.Fatalf("empty: %+v %v", empty, err)
	}
	if odd, err := s.QuickSearch(ctx, `"(*`, 0); err != nil || len(odd.Bookmarks) != 0 {
		t.Fatalf("punctuation: %+v %v", odd, err)
	}
}
