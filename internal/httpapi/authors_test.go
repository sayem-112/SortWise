package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestSavedPicturesAreServedByNameOnly(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "pictures.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	dir := filepath.Join(root, "avatars")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG\r\n\x1a\n picture")
	for path, body := range map[string][]byte{
		filepath.Join(dir, "0123456789abcdef0123.png"): png,
		filepath.Join(dir, "notes.txt"):                []byte("not a picture"),
		filepath.Join(root, "secret.png"):              []byte("outside the folder"),
	} {
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(db, Dependencies{AvatarDir: dir})
	get := func(path string, headers map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Host = "127.0.0.1:8787"
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	served := get("/api/v1/avatars/0123456789abcdef0123.png", map[string]string{"Sec-Fetch-Site": "same-origin"})
	if served.Code != 200 || served.Body.String() != string(png) || served.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("picture: %d %q %q", served.Code, served.Header().Get("Content-Type"), served.Body.String())
	}
	if cache := served.Header().Get("Cache-Control"); !strings.Contains(cache, "immutable") || !strings.Contains(cache, "private") {
		t.Fatalf("cache: %q", cache)
	}
	// A picture can never be read as anything but an image, or embedded elsewhere.
	if served.Header().Get("X-Content-Type-Options") != "nosniff" || served.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" {
		t.Fatalf("headers: %v", served.Header())
	}

	// Names that are not a saved picture's are refused before the disk is
	// touched. Several of these would open the real file on Windows otherwise.
	for _, path := range []string{
		"/api/v1/avatars/notes.txt",
		"/api/v1/avatars/ffffffffffffffffffff.png",
		"/api/v1/avatars/0123456789ABCDEF0123.png",
		"/api/v1/avatars/0123456789abcdef0123.PNG",
		"/api/v1/avatars/0123456789abcdef0123.png.",
		"/api/v1/avatars/0123456789abcdef0123.png%20",
		"/api/v1/avatars/0123456789abcdef0123.png::$DATA",
		"/api/v1/avatars/..%2Fsecret.png",
		"/api/v1/avatars/%2e%2e%2fsecret.png",
		"/api/v1/avatars/0123456789abcdef0123.png%00.txt",
	} {
		if response := get(path, nil); response.Code != 404 {
			t.Errorf("%s: %d %q", path, response.Code, response.Body.String())
		}
	}

	// Another site's page cannot tell a saved picture from a missing one, so
	// it cannot learn whose posts are saved here.
	for _, site := range []string{"cross-site", "same-site"} {
		if response := get("/api/v1/avatars/0123456789abcdef0123.png", map[string]string{"Sec-Fetch-Site": site}); response.Code != 404 {
			t.Errorf("embedded from %s: %d", site, response.Code)
		}
	}
	if response := get("/api/v1/avatars/0123456789abcdef0123.png", map[string]string{"Origin": "https://evil.example"}); response.Code != 403 {
		t.Errorf("foreign origin: %d", response.Code)
	}
	// With no folder configured, nothing is served.
	request := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/0123456789abcdef0123.png", nil)
	request.Host = "127.0.0.1:8787"
	response := httptest.NewRecorder()
	New(db).ServeHTTP(response, request)
	if response.Code != 404 {
		t.Fatalf("no folder: %d", response.Code)
	}
}
