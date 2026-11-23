package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"sortwise/internal/ai"
	"sortwise/internal/database"
	"sortwise/internal/model"
	"sortwise/internal/semantic"
	"sortwise/internal/store"
)

func TestLibraryMapAndSavedDaysAPI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "overview.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := store.New(db)
	now := time.Now().UTC().Format(time.RFC3339)
	post := func(id, text string) model.ImportedBookmark {
		return model.ImportedBookmark{Platform: "x", PostID: id, Author: "Ada", Username: "ada", Text: text, URL: "https://x.com/ada/status/" + id, ExtractedAt: now, ExtractorVersion: "1.0.0"}
	}
	if _, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{post("1", "My automobile engine"), post("2", "A sourdough loaf"), post("3", "The car show")}}); err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE bookmarks SET processing_status='completed'`)

	queries := 0
	meaning := semantic.New(s, testKeys{}, func(context.Context, string) (ai.Embedder, error) { return topicEmbedder{&queries}, nil })
	handler := New(db, Dependencies{Credentials: testKeys{}, Semantic: meaning})
	type mapResponse struct {
		Points  []mapPoint `json:"points"`
		Indexed int        `json:"indexed"`
		Total   int        `json:"total"`
		Notice  string     `json:"notice"`
	}
	read := func(handler http.Handler, path string, into any) {
		t.Helper()
		response := requestJSON(t, handler, http.MethodGet, path, nil, "")
		if response.Code != 200 {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
		}
		json.Unmarshal(response.Body.Bytes(), into)
	}

	// Before any post has a vector, the map is empty but knows the library's size.
	var before mapResponse
	read(handler, "/api/v1/map", &before)
	if len(before.Points) != 0 || before.Total != 3 || before.Notice != "" {
		t.Fatalf("before indexing: %+v", before)
	}

	go meaning.Run(ctx)
	deadline := time.Now().Add(10 * time.Second)
	for meaning.Status(ctx).Indexed != 3 {
		if time.Now().After(deadline) {
			t.Fatal("posts were not indexed")
		}
		time.Sleep(50 * time.Millisecond)
	}
	db.Exec(`UPDATE bookmarks SET archived_at=CURRENT_TIMESTAMP WHERE post_id='2'`)
	var after mapResponse
	read(handler, "/api/v1/map", &after)
	if after.Indexed != 2 || after.Total != 2 || len(after.Points) != 2 || queries != 0 {
		t.Fatalf("after indexing (archived post left out, no search sent): %+v, %d queries", after, queries)
	}

	// Without search by meaning, the map says why it is empty.
	var off mapResponse
	read(New(db), "/api/v1/map", &off)
	if len(off.Points) != 0 || off.Notice == "" {
		t.Fatalf("without search by meaning: %+v", off)
	}

	var days store.SavedDays
	read(handler, "/api/v1/saved-days", &days)
	if days.Total != 2 || days.BackCatalogue != nil || len(days.Days) != 1 || days.Days[0].Count != 2 {
		t.Fatalf("saved days: %+v", days)
	}
	if response := requestJSON(t, handler, http.MethodGet, "/api/v1/saved-days?since=yesterday", nil, ""); response.Code != 400 {
		t.Fatalf("bad date: %d", response.Code)
	}
}
