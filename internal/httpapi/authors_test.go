package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"sortwise/internal/database"
	"sortwise/internal/model"
	"sortwise/internal/store"
)

func TestAuthorsAPI(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "authors.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	post := func(id, author, username, text string) model.ImportedBookmark {
		return model.ImportedBookmark{Platform: "x", PostID: id, Author: author, Username: username, Text: text, URL: "https://x.com/" + username + "/status/" + id, ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "1.0.0"}
	}
	batch := model.ImportBatch{Bookmarks: []model.ImportedBookmark{
		post("8001", "Ada Lovelace", "Ada", "The analytical engine"),
		post("8002", "Ada Lovelace", "Ada", "Notes"),
		post("8003", "Grace Hopper", "grace", "A ship in port"),
	}}
	if _, err := store.New(db).Import(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	handler := New(db)
	get := func(path string, into any) int {
		t.Helper()
		response := requestJSON(t, handler, http.MethodGet, path, nil, "")
		if into != nil {
			json.Unmarshal(response.Body.Bytes(), into)
		}
		return response.Code
	}

	var list model.AuthorPage
	if code := get("/api/v1/authors?sort=count", &list); code != 200 || list.Total != 2 || list.Items[0].Username != "Ada" || list.Items[0].Count != 2 {
		t.Fatalf("list: %d %+v", code, list)
	}
	if get("/api/v1/authors?q=hopper", &list); list.Total != 1 || list.Items[0].Username != "grace" {
		t.Fatalf("filtered list: %+v", list)
	}

	// A handle is found whatever its case, with or without @, encoded or not.
	for _, path := range []string{"/api/v1/authors/ada", "/api/v1/authors/ADA", "/api/v1/authors/@ada", "/api/v1/authors/%40ada"} {
		var profile model.AuthorProfile
		if code := get(path, &profile); code != 200 || profile.Username != "Ada" || profile.Count != 2 {
			t.Fatalf("%s: %d %+v", path, code, profile)
		}
	}
	if code := get("/api/v1/authors/nobody", nil); code != 404 {
		t.Fatalf("missing author: %d", code)
	}

	var page model.BookmarkPage
	if get("/api/v1/bookmarks?author=ADA", &page); page.Total != 2 {
		t.Fatalf("posts by author: %+v", page.Total)
	}

	var quick model.QuickResults
	if code := get("/api/v1/search/quick?q=engine", &quick); code != 200 || quick.Total != 1 || len(quick.Bookmarks) != 1 || quick.Bookmarks[0].Username != "Ada" {
		t.Fatalf("quick search: %d %+v", code, quick)
	}
	if get("/api/v1/search/quick?q=grace", &quick); len(quick.Authors) != 1 || quick.Authors[0].Username != "grace" {
		t.Fatalf("quick search authors: %+v", quick)
	}
	if code := get("/api/v1/search/quick?q=", &quick); code != 200 || len(quick.Bookmarks) != 0 {
		t.Fatalf("empty quick search: %d %+v", code, quick)
	}
}
