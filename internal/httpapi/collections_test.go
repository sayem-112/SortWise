package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sortwise/internal/ai"
	"sortwise/internal/database"
	"sortwise/internal/model"
	"sortwise/internal/semantic"
	"sortwise/internal/store"
)

type testKeys struct{}

func (testKeys) Get(string) (string, string, error) { return "key", "test", nil }
func (testKeys) Set(string, string) error           { return nil }
func (testKeys) Delete(string) error                { return nil }

// topicEmbedder places a text by which of two topics its words belong to.
type topicEmbedder struct{ queries *int }

func (e topicEmbedder) Embed(_ context.Context, texts []string, query bool) ([][]float32, error) {
	if query {
		*e.queries++
	}
	vectors := make([][]float32, len(texts))
	for index, text := range texts {
		vector := make([]float32, ai.EmbeddingDimensions)
		vector[2] = 0.01
		for _, word := range strings.Fields(strings.ToLower(text)) {
			switch strings.Trim(word, ".,:@") {
			case "car", "automobile", "engine":
				vector[0]++
			case "bread", "sourdough", "loaf":
				vector[1]++
			}
		}
		vectors[index] = vector
	}
	return vectors, nil
}

func TestSearchByMeaningAndCollectionsAPI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "meaning.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := store.New(db)
	post := func(id, text string) model.ImportedBookmark {
		return model.ImportedBookmark{Platform: "x", PostID: id, Author: "Ada", Username: "ada", Text: text, URL: "https://x.com/ada/status/" + id, ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "1.0.0"}
	}
	if _, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{post("1", "My automobile needs an engine"), post("2", "A sourdough loaf"), post("3", "The car show")}}); err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE bookmarks SET processing_status='completed'`)
	id := func(postID string) (value int64) {
		db.QueryRow(`SELECT id FROM bookmarks WHERE post_id=?`, postID).Scan(&value)
		return value
	}

	queries := 0
	meaning := semantic.New(s, testKeys{}, func(context.Context, string) (ai.Embedder, error) { return topicEmbedder{&queries}, nil })
	get := func(handler http.Handler, path string, into any) int {
		t.Helper()
		response := requestJSON(t, handler, http.MethodGet, path, nil, "")
		if into != nil {
			json.Unmarshal(response.Body.Bytes(), into)
		}
		return response.Code
	}

	// Without the index, a search by meaning is the word search plus a reason.
	var page model.BookmarkPage
	if code := get(New(db), "/api/v1/bookmarks?q=car&mode=meaning", &page); code != 200 || page.Total != 1 || page.Search == nil || page.Search.Meaning || page.Search.Notice == "" {
		t.Fatalf("without search by meaning: %d %+v %+v", code, page, page.Search)
	}

	handler := New(db, Dependencies{Credentials: testKeys{}, Semantic: meaning})
	page = model.BookmarkPage{}
	if get(handler, "/api/v1/bookmarks?q=car&mode=meaning", &page); page.Search == nil || page.Search.Meaning || !strings.Contains(page.Search.Notice, "prepared") {
		t.Fatalf("before indexing: %+v", page.Search)
	}

	go meaning.Run(ctx)
	deadline := time.Now().Add(10 * time.Second)
	for meaning.Status(ctx).Indexed != 3 {
		if time.Now().After(deadline) {
			t.Fatal("posts were not indexed")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// "car" is in one post; by meaning the automobile post is found too.
	page = model.BookmarkPage{}
	get(handler, "/api/v1/bookmarks?q=car&mode=meaning", &page)
	if page.Search == nil || !page.Search.Meaning || page.Total != 2 || page.Items[0].ID != id("3") || page.Items[1].Match != "meaning" {
		t.Fatalf("by meaning: %+v %+v", page.Search, page.Items)
	}
	page = model.BookmarkPage{}
	if get(handler, "/api/v1/bookmarks?q=car", &page); page.Total != 1 || page.Search != nil {
		t.Fatalf("by words: %d %+v", page.Total, page.Search)
	}

	var related struct{ Items []model.RelatedBookmark }
	if code := get(handler, "/api/v1/bookmarks/"+itoa(id("1"))+"/related", &related); code != 200 || len(related.Items) == 0 || related.Items[0].ID != id("3") || related.Items[0].Text != "The car show" {
		t.Fatalf("related: %d %+v", code, related)
	}

	// A collection saved from that search counts the same posts without asking again.
	asked := queries
	created := requestJSON(t, handler, http.MethodPost, "/api/v1/collections", map[string]any{"name": "Cars", "query": map[string]any{"q": "car", "mode": "meaning"}}, "")
	var collection model.Collection
	json.Unmarshal(created.Body.Bytes(), &collection)
	if created.Code != 201 || collection.Count != 2 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	// One whose search was never run counts its word matches until it is opened.
	requestJSON(t, handler, http.MethodPost, "/api/v1/collections", map[string]any{"name": "Bread", "query": map[string]any{"q": "bread", "mode": "meaning"}}, "")
	var list struct{ Items []model.Collection }
	if get(handler, "/api/v1/collections", &list); len(list.Items) != 2 || list.Items[0].Name != "Bread" || list.Items[0].Count != 0 || list.Items[1].Count != 2 || queries != asked {
		t.Fatalf("list: %+v, asked %d more times", list.Items, queries-asked)
	}

	if code := requestJSON(t, handler, http.MethodPost, "/api/v1/collections", map[string]any{"name": "cars", "query": map[string]any{"q": "x"}}, "").Code; code != 409 {
		t.Fatalf("duplicate name: %d", code)
	}
	if code := requestJSON(t, handler, http.MethodPost, "/api/v1/collections", map[string]any{"name": "All", "query": map[string]any{}}, "").Code; code != 422 {
		t.Fatalf("empty collection: %d", code)
	}
	renamed := requestJSON(t, handler, http.MethodPatch, "/api/v1/collections/"+itoa(collection.ID), map[string]any{"name": "Motoring"}, "")
	json.Unmarshal(renamed.Body.Bytes(), &collection)
	if renamed.Code != 200 || collection.Name != "Motoring" || collection.Query.Text != "car" {
		t.Fatalf("rename: %d %+v", renamed.Code, collection)
	}
	if code := requestJSON(t, handler, http.MethodDelete, "/api/v1/collections/"+itoa(collection.ID), nil, "").Code; code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code := requestJSON(t, handler, http.MethodDelete, "/api/v1/collections/"+itoa(collection.ID), nil, "").Code; code != 404 {
		t.Fatalf("delete again: %d", code)
	}

	// Turning it off in Settings stops the search and says so.
	off := requestJSON(t, handler, http.MethodPatch, "/api/v1/settings/ai", map[string]any{"meaning": false}, "")
	var settings struct{ Meaning semantic.Status }
	json.Unmarshal(off.Body.Bytes(), &settings)
	if off.Code != 200 || settings.Meaning.Enabled || settings.Meaning.Indexed != 3 {
		t.Fatalf("settings: %d %+v", off.Code, settings)
	}
	page = model.BookmarkPage{}
	if get(handler, "/api/v1/bookmarks?q=car&mode=meaning", &page); page.Total != 1 || !strings.Contains(page.Search.Notice, "turned off") {
		t.Fatalf("when off: %d %+v", page.Total, page.Search)
	}
}

func itoa(value int64) string {
	body, _ := json.Marshal(value)
	return string(body)
}
