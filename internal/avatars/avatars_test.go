package avatars

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sortwise/internal/database"
	"sortwise/internal/model"
	"sortwise/internal/store"
)

// A 1x1 PNG.
var png = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xcf\xc0\x00\x00\x03\x01\x01\x00\x18\xdd\x8d\xb0\x00\x00\x00\x00IEND\xaeB`\x82")

// recorder notes every request the worker sends anywhere, and fails the test
// if one is to a host other than the stand-in image server.
type recorder struct {
	t    *testing.T
	host string
	seen []string
	next http.RoundTripper
}

func (r *recorder) RoundTrip(request *http.Request) (*http.Response, error) {
	r.seen = append(r.seen, request.URL.Path)
	if request.URL.Host != r.host {
		r.t.Errorf("request sent to %s", request.URL)
		return nil, http.ErrAbortHandler
	}
	if request.Header.Get("Cookie") != "" || request.Header.Get("Authorization") != "" || request.Header.Get("Referer") != "" {
		r.t.Errorf("picture requests must carry no login or referrer: %v", request.Header)
	}
	return r.next.RoundTrip(request)
}

func setup(t *testing.T, handler http.HandlerFunc) (*Worker, *store.Store, *httptest.Server, *recorder) {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "avatars.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	s := store.New(db)
	w := New(s, filepath.Join(t.TempDir(), "avatars"))
	// Stand in for X's image server; every other rule stays as in real use.
	w.scheme, w.host, w.pause = "http", strings.TrimPrefix(server.URL, "http://"), 0
	record := &recorder{t: t, host: w.host, next: http.DefaultTransport}
	w.client.Transport = record
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return w, s, server, record
}

var nextPost = 1000

func importFrom(t *testing.T, s *store.Store, username, avatarURL string) {
	t.Helper()
	nextPost++
	postID := fmt.Sprint(nextPost)
	item := model.ImportedBookmark{Platform: "x", PostID: postID, Author: username, Username: username, Text: "post " + postID, URL: "https://x.com/" + username + "/status/" + postID, ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "x-graphql-1", AvatarURL: avatarURL}
	if _, err := s.Import(context.Background(), model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
		t.Fatal(err)
	}
}

func avatarOf(t *testing.T, s *store.Store, username string) string {
	t.Helper()
	page, err := s.SearchBookmarks(context.Background(), model.BookmarkQuery{Author: username})
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("posts by %s: %v", username, err)
	}
	return page.Items[0].AuthorAvatar
}

func files(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func step(t *testing.T, w *Worker) {
	t.Helper()
	if _, err := w.step(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPicturesAreSavedOnceAndReplacedWhenTheyChange(t *testing.T) {
	w, s, server, record := setup(t, func(response http.ResponseWriter, request *http.Request) { response.Write(png) })
	importFrom(t, s, "ada", server.URL+"/profile_images/1/ada_normal.jpg")

	step(t, w)
	// The sharper size is asked for, and the file is named by what it is.
	if len(record.seen) != 1 || record.seen[0] != "/profile_images/1/ada_200x200.jpg" {
		t.Fatalf("requests: %v", record.seen)
	}
	first := avatarOf(t, s, "ada")
	saved := files(t, w.dir)
	if len(saved) != 1 || !ValidFile(saved[0]) || !strings.HasSuffix(saved[0], ".png") || first != "/api/v1/avatars/"+saved[0] {
		t.Fatalf("saved %v, served as %q", saved, first)
	}

	// Nothing more to fetch until the picture changes.
	step(t, w)
	if len(record.seen) != 1 {
		t.Fatalf("fetched again: %v", record.seen)
	}
	importFrom(t, s, "ada", server.URL+"/profile_images/1/ada2_normal.jpg")
	step(t, w)
	second := avatarOf(t, s, "ada")
	if saved := files(t, w.dir); len(saved) != 1 || second == first || second != "/api/v1/avatars/"+saved[0] {
		t.Fatalf("after a new picture: %v, %q then %q", saved, first, second)
	}
}

func TestTheNameCannotBeWorkedOutFromPublicFacts(t *testing.T) {
	// The same author and the same picture address, in two separate libraries.
	const address = "https://pbs.twimg.com/profile_images/1/ada_normal.jpg"
	names := []string{}
	for range 2 {
		w, s, _, _ := setup(t, func(response http.ResponseWriter, request *http.Request) { response.Write(png) })
		// Send the real address to the stand-in server instead of to X.
		real := w.client.Transport
		w.scheme, w.host = "https", "pbs.twimg.com"
		w.client.Transport = roundTrip(func(request *http.Request) (*http.Response, error) {
			request.URL.Scheme, request.URL.Host = "http", real.(*recorder).host
			return real.RoundTrip(request)
		})
		importFrom(t, s, "ada", address)
		step(t, w)
		saved := files(t, w.dir)
		if len(saved) != 1 {
			t.Fatalf("saved: %v", saved)
		}
		names = append(names, saved[0])
	}
	if names[0] == names[1] {
		t.Fatalf("two libraries gave the same file name %q", names[0])
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestOnlyRealPicturesFromTheImageServerAreSaved(t *testing.T) {
	w, s, server, record := setup(t, func(response http.ResponseWriter, request *http.Request) {
		switch {
		case strings.Contains(request.URL.Path, "/11/"):
			response.Header().Set("Content-Type", "image/png")
			response.Write([]byte("<html><script>alert(1)</script></html>"))
		case strings.Contains(request.URL.Path, "/12/"):
			response.Write(append(append([]byte{}, png...), make([]byte, maxBytes)...))
		case strings.Contains(request.URL.Path, "/13/"):
			http.NotFound(response, request)
		case strings.Contains(request.URL.Path, "/14/"):
			http.Redirect(response, request, "/elsewhere/secret.png", http.StatusFound)
		case strings.Contains(request.URL.Path, "/15/"):
			response.Header().Set("Content-Type", "image/svg+xml")
			response.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
		default:
			response.Write(png)
		}
	})
	base := server.URL + "/profile_images/"
	refused := map[string]string{
		// Fetched, but not kept.
		"html":   base + "11/a_normal.png",
		"huge":   base + "12/a_normal.png",
		"gone":   base + "13/a_normal.png",
		"away":   base + "14/a_normal.png",
		"vector": base + "15/a_normal.png",
		// Never requested at all.
		"elsewhere": "https://example.com/profile_images/1/a_normal.png",
		"file":      "file:///profile_images/1/a_normal.png",
		"local":     server.URL + "/other/a.png",
		"climb":     base + "1/../../media/a.png",
		"encoded":   base + "1/..%2f..%2fmedia%2fa.png",
		"query":     base + "1/a_normal.png?redirect=http://127.0.0.1/",
		"login":     strings.Replace(base, "http://", "http://user:pass@", 1) + "1/a_normal.png",
		"plain":     "https://abs.twimg.com/sticky/default_profile_images/default_profile_normal.png",
		"nonsense":  "::not a url::",
	}
	for username, address := range refused {
		importFrom(t, s, username, address)
	}
	// More authors than one round handles, so nothing is skipped by luck.
	for round := 0; round < 4; round++ {
		step(t, w)
	}
	if saved := files(t, w.dir); len(saved) != 0 {
		t.Fatalf("nothing should be saved: %v", saved)
	}
	for username := range refused {
		if got := avatarOf(t, s, username); got != "" {
			t.Fatalf("%s has a picture: %q", username, got)
		}
	}
	// Only the five on the image server were ever requested (the recorder
	// fails the test for any other host); the dead one twice, sharp then plain,
	// and the redirect is not followed off the picture's path.
	if len(record.seen) != 6 {
		t.Fatalf("expected 6 requests, saw %d: %v", len(record.seen), record.seen)
	}
	for _, path := range record.seen {
		if !picturePath.MatchString(path) {
			t.Fatalf("requested something that is not a picture: %s", path)
		}
	}
	// Each is settled: nothing is asked for again.
	asked := len(record.seen)
	step(t, w)
	if len(record.seen) != asked {
		t.Fatalf("asked again: %v", record.seen[asked:])
	}
}

func TestASavedPictureOutlivesItsAuthorRemovingTheirs(t *testing.T) {
	w, s, server, _ := setup(t, func(response http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/2/") {
			http.NotFound(response, request)
			return
		}
		response.Write(png)
	})
	importFrom(t, s, "ada", server.URL+"/profile_images/1/ada_normal.jpg")
	step(t, w)
	saved := avatarOf(t, s, "ada")
	if saved == "" {
		t.Fatal("no picture saved")
	}
	for _, address := range []string{
		"https://abs.twimg.com/sticky/default_profile_images/default_profile_normal.png", // went back to X's default
		server.URL + "/profile_images/2/dead_normal.jpg",                                 // an address with nothing behind it
		"https://new-image-host.example/profile_images/3/ada_normal.jpg",                 // X moved its pictures
	} {
		importFrom(t, s, "ada", address)
		step(t, w)
		if got := avatarOf(t, s, "ada"); got != saved || len(files(t, w.dir)) != 1 {
			t.Fatalf("after %s: picture %q, files %v", address, got, files(t, w.dir))
		}
	}
}

func TestTroubleReachingTheServerMarksNobodyAndIsRetried(t *testing.T) {
	down := true
	w, s, server, record := setup(t, func(response http.ResponseWriter, request *http.Request) {
		if down {
			http.Error(response, "slow down", http.StatusTooManyRequests)
			return
		}
		response.Write(png)
	})
	importFrom(t, s, "ada", server.URL+"/profile_images/1/ada_normal.jpg")
	importFrom(t, s, "grace", server.URL+"/profile_images/2/grace_normal.jpg")

	// The round stops at the first refusal instead of working through everyone.
	if _, err := w.step(context.Background()); err == nil {
		t.Fatal("a refusal should stop the round")
	}
	if len(record.seen) != 1 {
		t.Fatalf("kept asking while refused: %v", record.seen)
	}
	// The one refused moved to the back, so the other is first next time.
	if _, err := w.step(context.Background()); err == nil || len(record.seen) != 2 || record.seen[0] == record.seen[1] {
		t.Fatalf("second round: %v", record.seen)
	}
	down = false
	step(t, w)
	if avatarOf(t, s, "ada") == "" || avatarOf(t, s, "grace") == "" {
		t.Fatal("pictures were not saved once the server answered again")
	}
}

func TestMissingFilesAreFetchedAgainAndStrayOnesCleared(t *testing.T) {
	w, s, server, record := setup(t, func(response http.ResponseWriter, request *http.Request) { response.Write(png) })
	importFrom(t, s, "ada", server.URL+"/profile_images/1/ada_normal.jpg")
	step(t, w)
	saved := files(t, w.dir)[0]

	// A backup restored without the folder, plus leftovers from an older library.
	os.Remove(filepath.Join(w.dir, saved))
	os.WriteFile(filepath.Join(w.dir, "0123456789abcdef0123.jpg"), png, 0o600)
	os.WriteFile(filepath.Join(w.dir, "incoming-123"), []byte("half"), 0o600)
	os.WriteFile(filepath.Join(w.dir, "notes.txt"), []byte("the user's own file"), 0o600)
	w.nextUpkeep = w.nextUpkeep.AddDate(-1, 0, 0)
	step(t, w)

	if len(record.seen) != 2 || avatarOf(t, s, "ada") != "/api/v1/avatars/"+saved {
		t.Fatalf("not fetched again: %v, %q", record.seen, avatarOf(t, s, "ada"))
	}
	left := strings.Join(files(t, w.dir), " ")
	if !strings.Contains(left, saved) || strings.Contains(left, "0123456789abcdef0123") || strings.Contains(left, "incoming-") || !strings.Contains(left, "notes.txt") {
		t.Fatalf("folder after tidy-up: %s", left)
	}
}

func TestOlderPostsGetTheirPictureFromStoredData(t *testing.T) {
	w, s, server, _ := setup(t, func(response http.ResponseWriter, request *http.Request) { response.Write(png) })
	ctx := context.Background()
	// Saved before pictures were kept: the address is only in the stored X data.
	raw, _ := json.Marshal(map[string]any{
		"__typename": "Tweet", "rest_id": "9",
		"core":   map[string]any{"user_results": map[string]any{"result": map[string]any{"legacy": map[string]any{"name": "Grace", "screen_name": "grace", "profile_image_url_https": server.URL + "/profile_images/9/grace_normal.jpg"}}}},
		"legacy": map[string]any{"full_text": "A ship in port"},
	})
	item := model.ImportedBookmark{Platform: "x", PostID: "9", Author: "Grace", Username: "grace", Text: "A ship in port", URL: "https://x.com/grace/status/9", ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "x-graphql-1", Raw: raw}
	if _, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
		t.Fatal(err)
	}
	// A newer post of hers that was only read from the page carries no address;
	// the older one with X's data is the one to read.
	newer := model.ImportedBookmark{Platform: "x", PostID: "10", Author: "Grace", Username: "grace", Text: "Read from the page", URL: "https://x.com/grace/status/10", ExtractedAt: "2026-07-19T12:00:00Z", ExtractorVersion: "1.0.0"}
	if _, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{newer}}); err != nil {
		t.Fatal(err)
	}
	importFrom(t, s, "nodata", "")

	if busy, err := w.step(ctx); err != nil || !busy {
		t.Fatalf("first round: %v %v", busy, err)
	}
	if got := avatarOf(t, s, "grace"); got == "" {
		t.Fatal("grace's picture was not found in her stored post")
	}
	if got := avatarOf(t, s, "nodata"); got != "" {
		t.Fatalf("no address, no picture: %q", got)
	}

	// An author with no posts left loses their picture at the next tidy-up.
	rows, err := s.DB.Query(`SELECT id FROM bookmarks WHERE username='grace'`)
	if err != nil {
		t.Fatal(err)
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := s.DeleteBookmark(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	w.nextUpkeep = w.nextUpkeep.AddDate(-1, 0, 0)
	step(t, w)
	if saved := files(t, w.dir); len(saved) != 0 {
		t.Fatalf("picture left behind: %v", saved)
	}
}

func TestOnlySavedPictureNamesAreValid(t *testing.T) {
	for name, want := range map[string]bool{
		"0123456789abcdef0123.jpg":        true,
		"0123456789abcdef0123.webp":       true,
		"0123456789ABCDEF0123.jpg":        false,
		"0123456789abcdef0123.svg":        false,
		"../0123456789abcdef0123.jpg":     false,
		"0123456789abcdef0123.jpg/..":     false,
		"0123456789abcdef0123.jpg.":       false,
		"0123456789abcdef0123.jpg ":       false,
		"0123456789abcdef0123.jpg::$DATA": false,
		"0123456789abcdef0123.jpg\n":      false,
		"incoming-123":                    false,
		"":                                false,
		"0123456789abcdef0123.jpg\x00.x":  false,
	} {
		if ValidFile(name) != want {
			t.Errorf("ValidFile(%q) = %v", name, !want)
		}
	}
}

func TestOnlyProfilePicturesOnXsImageServerAreAllowed(t *testing.T) {
	w := New(nil, t.TempDir())
	for address, want := range map[string]bool{
		"https://pbs.twimg.com/profile_images/1683325380441128960/yRsRRjGO_normal.jpg": true,
		"https://pbs.twimg.com/profile_images/1/a-b_200x200.png":                       true,
		"http://pbs.twimg.com/profile_images/1/a_normal.jpg":                           false,
		"https://pbs.twimg.com.evil.example/profile_images/1/a.jpg":                    false,
		"https://evil.example/pbs.twimg.com/profile_images/1/a.jpg":                    false,
		"https://PBS.twimg.com/profile_images/1/a.jpg":                                 false,
		"https://pbs.twimg.com./profile_images/1/a.jpg":                                false,
		"https://pbs.twimg.com:8443/profile_images/1/a.jpg":                            false,
		"https://user@pbs.twimg.com/profile_images/1/a.jpg":                            false,
		"https://pbs.twimg.com/media/1.jpg":                                            false,
		"https://pbs.twimg.com/profile_images/1/../../media/a.jpg":                     false,
		"https://pbs.twimg.com/profile_images/1/..%2f..%2fmedia%2fa.jpg":               false,
		"https://pbs.twimg.com/profile_images/1/a.jpg?x=1":                             false,
		"https://pbs.twimg.com/profile_images/1/a.svg":                                 false,
		"https://pbs.twimg.com/profile_images/abc/a.jpg":                               false,
		"https://127.0.0.1/profile_images/1/a.jpg":                                     false,
		"file:///profile_images/1/a.jpg":                                               false,
	} {
		parsed, err := url.Parse(address)
		if err != nil {
			t.Fatal(err)
		}
		if w.allowed(parsed) != want {
			t.Errorf("allowed(%q) = %v", address, !want)
		}
	}
}
