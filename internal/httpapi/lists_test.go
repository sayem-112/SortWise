package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"sortwise/internal/database"
	"sortwise/internal/model"
	"sortwise/internal/store"
)

func TestListsAPI(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "lists.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	item := model.ImportedBookmark{Platform: "x", PostID: "7001", Author: "Ada", Username: "ada", Text: "Queues", URL: "https://x.com/ada/status/7001", ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "1.0.0"}
	if _, err := store.New(db).Import(context.Background(), model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
		t.Fatal(err)
	}
	var bookmarkID int64
	db.QueryRow(`SELECT id FROM bookmarks`).Scan(&bookmarkID)
	handler := New(db)

	var lists struct {
		Items []model.List `json:"items"`
	}
	json.Unmarshal(requestJSON(t, handler, http.MethodGet, "/api/v1/lists", nil, "").Body.Bytes(), &lists)
	if len(lists.Items) != 1 || lists.Items[0].Kind != "favorites" {
		t.Fatalf("lists: %+v", lists)
	}
	favorites := lists.Items[0].ID

	created := requestJSON(t, handler, http.MethodPost, "/api/v1/lists", map[string]string{"name": "Recipes"}, "")
	var recipes model.List
	json.Unmarshal(created.Body.Bytes(), &recipes)
	if created.Code != 201 || recipes.Name != "Recipes" {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	if duplicate := requestJSON(t, handler, http.MethodPost, "/api/v1/lists", map[string]string{"name": "recipes"}, ""); duplicate.Code != 409 {
		t.Fatalf("duplicate: %d", duplicate.Code)
	}
	if renamed := requestJSON(t, handler, http.MethodPatch, fmt.Sprintf("/api/v1/lists/%d", favorites), map[string]string{"name": "Stars"}, ""); renamed.Code != 409 {
		t.Fatalf("rename favorites: %d", renamed.Code)
	}

	add := requestJSON(t, handler, http.MethodPut, fmt.Sprintf("/api/v1/lists/%d/bookmarks/%d", favorites, bookmarkID), nil, "")
	if add.Code != 204 {
		t.Fatalf("add: %d %s", add.Code, add.Body.String())
	}
	var page model.BookmarkPage
	json.Unmarshal(requestJSON(t, handler, http.MethodGet, fmt.Sprintf("/api/v1/bookmarks?list=%d", favorites), nil, "").Body.Bytes(), &page)
	if page.Total != 1 || len(page.Items[0].Lists) != 1 || page.Items[0].Lists[0] != favorites {
		t.Fatalf("favorites page: %+v", page)
	}
	json.Unmarshal(requestJSON(t, handler, http.MethodGet, fmt.Sprintf("/api/v1/bookmarks?list=%d", recipes.ID), nil, "").Body.Bytes(), &page)
	if page.Total != 0 {
		t.Fatalf("recipes page: %+v", page)
	}

	if missing := requestJSON(t, handler, http.MethodPut, fmt.Sprintf("/api/v1/lists/%d/bookmarks/99999", favorites), nil, ""); missing.Code != 404 {
		t.Fatalf("missing bookmark: %d", missing.Code)
	}
	if removed := requestJSON(t, handler, http.MethodDelete, fmt.Sprintf("/api/v1/lists/%d/bookmarks/%d", favorites, bookmarkID), nil, ""); removed.Code != 204 {
		t.Fatalf("remove: %d", removed.Code)
	}
	many := requestJSON(t, handler, http.MethodPost, fmt.Sprintf("/api/v1/lists/%d/bookmarks", recipes.ID), map[string]any{"bookmarkIds": []int64{bookmarkID, 99999}, "inList": true}, "")
	if many.Code != 204 {
		t.Fatalf("add many: %d %s", many.Code, many.Body.String())
	}
	json.Unmarshal(requestJSON(t, handler, http.MethodGet, fmt.Sprintf("/api/v1/bookmarks?list=%d", recipes.ID), nil, "").Body.Bytes(), &page)
	if page.Total != 1 {
		t.Fatalf("recipes after add many: %+v", page)
	}
	json.Unmarshal(requestJSON(t, handler, http.MethodGet, "/api/v1/bookmarks?list=none", nil, "").Body.Bytes(), &page)
	if page.Total != 0 {
		t.Fatalf("in no list: %+v", page)
	}
	deleted := requestJSON(t, handler, http.MethodDelete, fmt.Sprintf("/api/v1/lists/%d", recipes.ID), nil, "")
	var removed struct {
		BookmarkIDs []int64 `json:"bookmarkIds"`
	}
	json.Unmarshal(deleted.Body.Bytes(), &removed)
	if deleted.Code != 200 || len(removed.BookmarkIDs) != 1 || removed.BookmarkIDs[0] != bookmarkID {
		t.Fatalf("delete: %d %s", deleted.Code, deleted.Body.String())
	}
	// Starred but in no list of its own still counts as in no list.
	requestJSON(t, handler, http.MethodPut, fmt.Sprintf("/api/v1/lists/%d/bookmarks/%d", favorites, bookmarkID), nil, "")
	json.Unmarshal(requestJSON(t, handler, http.MethodGet, "/api/v1/bookmarks?list=none", nil, "").Body.Bytes(), &page)
	if page.Total != 1 {
		t.Fatalf("starred only should be in no list: %+v", page)
	}
	// A deleted list's number is not reused.
	var again model.List
	json.Unmarshal(requestJSON(t, handler, http.MethodPost, "/api/v1/lists", map[string]string{"name": "Recipes"}, "").Body.Bytes(), &again)
	if again.ID == recipes.ID {
		t.Fatalf("list id %d was reused", again.ID)
	}
}
