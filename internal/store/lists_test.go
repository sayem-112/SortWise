package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"sortwise/internal/model"
)

func importPosts(t *testing.T, s *Store, count int) []int64 {
	t.Helper()
	ctx := context.Background()
	ids := []int64{}
	for index := 0; index < count; index++ {
		item := sampleBookmark()
		item.PostID = fmt.Sprintf("%d", 5000+index)
		item.URL = "https://x.com/ada/status/" + item.PostID
		if _, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
			t.Fatal(err)
		}
		var id int64
		s.DB.QueryRow(`SELECT id FROM bookmarks WHERE post_id=?`, item.PostID).Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

func TestFavoritesIsBuiltInAndCannotBeRenamedOrDeleted(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	lists, err := s.Lists(ctx)
	if err != nil || len(lists) != 1 || lists[0].Kind != "favorites" || lists[0].Name != "Favorites" {
		t.Fatalf("lists: %+v %v", lists, err)
	}
	if _, err := s.RenameList(ctx, lists[0].ID, "Best"); !errors.Is(err, ErrFavorites) {
		t.Fatalf("rename favorites: %v", err)
	}
	if err := s.DeleteList(ctx, lists[0].ID); !errors.Is(err, ErrFavorites) {
		t.Fatalf("delete favorites: %v", err)
	}
}

func TestListsKeepBookmarksInTheOrderTheyWereAdded(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ids := importPosts(t, s, 3)
	reading, err := s.CreateList(ctx, "Read later")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateList(ctx, "read  LATER"); !errors.Is(err, ErrListExists) {
		t.Fatalf("duplicate name: %v", err)
	}
	// Added oldest bookmark last, and the first one twice.
	for _, id := range []int64{ids[2], ids[0], ids[0]} {
		if err := s.SetInList(ctx, reading.ID, id, true); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.SearchBookmarks(ctx, model.BookmarkQuery{ListID: reading.ID, Sort: "added_desc"})
	if err != nil || page.Total != 2 || page.Items[0].ID != ids[0] || page.Items[1].ID != ids[2] {
		t.Fatalf("list page: %+v %v", page, err)
	}
	if got := page.Items[0].Lists; len(got) != 1 || got[0] != reading.ID {
		t.Fatalf("bookmark lists: %v", got)
	}

	// Archived bookmarks are not counted, just as the library hides them.
	if _, err := s.SetArchived(ctx, ids[2], true); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.GetList(ctx, reading.ID); list.Count != 1 {
		t.Fatalf("count after archive: %+v", list)
	}

	if err := s.SetInList(ctx, reading.ID, ids[0], false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetInList(ctx, reading.ID, 99999, true); err == nil {
		t.Fatal("adding a missing bookmark should fail")
	}

	// Deleting a list keeps its bookmarks; deleting a bookmark leaves its lists.
	if err := s.SetInList(ctx, reading.ID, ids[1], true); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteBookmark(ctx, ids[1]); err != nil {
		t.Fatal(err)
	}
	var left int
	s.DB.QueryRow(`SELECT COUNT(*) FROM list_items WHERE bookmark_id=?`, ids[1]).Scan(&left)
	if left != 0 {
		t.Fatalf("list items left after delete: %d", left)
	}
	if err := s.DeleteList(ctx, reading.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBookmark(ctx, ids[0]); err != nil {
		t.Fatalf("bookmark gone with its list: %v", err)
	}
}
