package semantic

import (
	"context"
	"errors"
	"hash/fnv"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"sortwise/internal/ai"
	"sortwise/internal/database"
	"sortwise/internal/model"
	"sortwise/internal/store"
)

type fakeCredentials struct{ key string }

func (f *fakeCredentials) Get(string) (string, string, error) { return f.key, "test", nil }
func (f *fakeCredentials) Set(string, string) error           { return nil }
func (f *fakeCredentials) Delete(string) error                { return nil }

// concepts stand in for meaning: words for the same thing share a dimension.
var concepts = map[string]int{
	"car": 0, "cars": 0, "automobile": 0, "vehicle": 0, "engine": 0,
	"bread": 1, "baking": 1, "sourdough": 1, "loaf": 1, "flour": 1,
	"queue": 2, "queues": 2, "retry": 2, "backoff": 2, "worker": 2,
}

// fakeEmbedder places a text by the concepts its words belong to, with a
// little weight on the words themselves. It records what it was asked.
type fakeEmbedder struct {
	requests [][]string
	queries  int
	fail     func(texts []string) error
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string, query bool) ([][]float32, error) {
	f.requests = append(f.requests, texts)
	if query {
		f.queries++
	}
	if f.fail != nil {
		if err := f.fail(texts); err != nil {
			return nil, err
		}
	}
	vectors := make([][]float32, len(texts))
	for index, text := range texts {
		vector := make([]float32, ai.EmbeddingDimensions)
		for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return r < 'a' || r > 'z' }) {
			if concept, ok := concepts[word]; ok {
				vector[concept] += 1
				continue
			}
			hash := fnv.New32a()
			hash.Write([]byte(word))
			vector[16+hash.Sum32()%700] += 0.05
		}
		vectors[index] = vector
	}
	return vectors, nil
}

type fixture struct {
	store    *store.Store
	service  *Service
	embedder *fakeEmbedder
	keys     *fakeCredentials
	ids      map[string]int64
}

// library saves the given posts (post ID to text) as already analyzed.
func library(t *testing.T, posts map[string]string) *fixture {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "semantic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := store.New(db)
	batch := model.ImportBatch{}
	for id, text := range posts {
		batch.Bookmarks = append(batch.Bookmarks, model.ImportedBookmark{Platform: "x", PostID: id, Author: "Ada", Username: "ada", Text: text, URL: "https://x.com/ada/status/" + id, ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "1.0.0"})
	}
	if len(batch.Bookmarks) > 0 {
		if _, err := s.Import(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE bookmarks SET processing_status='completed'`); err != nil {
		t.Fatal(err)
	}
	f := &fixture{store: s, embedder: &fakeEmbedder{}, keys: &fakeCredentials{key: "key"}, ids: map[string]int64{}}
	rows, err := db.Query(`SELECT post_id,id FROM bookmarks`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var post string
		var id int64
		rows.Scan(&post, &id)
		f.ids[post] = id
	}
	f.service = New(s, f.keys, func(context.Context, string) (ai.Embedder, error) { return f.embedder, nil })
	return f
}

// index runs the worker until it has nothing left to do.
func (f *fixture) index(t *testing.T) {
	t.Helper()
	for round := 0; round < 50; round++ {
		busy, err := f.service.step(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !busy {
			return
		}
	}
	t.Fatal("indexing never finished")
}

func TestSearchFindsPostsByMeaningNotWords(t *testing.T) {
	f := library(t, map[string]string{
		"1": "Why my automobile needs a new engine",
		"2": "Sourdough loaf with rye flour",
		"3": "Retry with backoff in a worker queue",
	})
	f.index(t)
	ctx := context.Background()

	// "car" appears in no post; the post about an automobile is the closest.
	ids, err := f.service.Search(ctx, "  Car ")
	if err != nil || len(ids) == 0 || ids[0] != f.ids["1"] {
		t.Fatalf("search: %v %v, want post %d first", ids, err, f.ids["1"])
	}
	for _, id := range ids {
		if id == f.ids["2"] || id == f.ids["3"] {
			t.Fatalf("a search for cars returned an unrelated post: %v", ids)
		}
	}

	// The same search again, however it is typed, asks the AI service nothing.
	if _, err := f.service.Search(ctx, "car"); err != nil || f.embedder.queries != 1 {
		t.Fatalf("repeat search asked again: %d queries, %v", f.embedder.queries, err)
	}
	if ids, err := f.service.Recall(ctx, "CAR"); err != nil || len(ids) == 0 {
		t.Fatalf("recall of a known search: %v %v", ids, err)
	}
	if _, err := f.service.Recall(ctx, "baking"); !errors.Is(err, ErrUnknownQuery) || f.embedder.queries != 1 {
		t.Fatalf("recall of a new search must not ask: %v, %d queries", err, f.embedder.queries)
	}
}

func TestRelatedLeavesOutThePostItself(t *testing.T) {
	f := library(t, map[string]string{
		"1": "Baking sourdough at home",
		"2": "A loaf of bread from good flour",
		"3": "Retry with backoff",
	})
	f.index(t)
	ids, err := f.service.Related(context.Background(), f.ids["1"], 5)
	if err != nil || len(ids) != 1 || ids[0] != f.ids["2"] {
		t.Fatalf("related: %v %v, want only post %d", ids, err, f.ids["2"])
	}
	if ids, _ := f.service.Related(context.Background(), 999, 5); len(ids) != 0 {
		t.Fatalf("a post with no vector has no related posts, got %v", ids)
	}
}

func TestIndexingOnlyAsksForPostsThatChanged(t *testing.T) {
	f := library(t, map[string]string{"1": "Baking bread", "2": "Car engine"})
	f.index(t)
	if len(f.embedder.requests) != 1 || len(f.embedder.requests[0]) != 2 {
		t.Fatalf("first index: %v", f.embedder.requests)
	}
	ctx := context.Background()

	// A later sync touches every post without changing it: nothing is asked.
	time.Sleep(1100 * time.Millisecond)
	f.store.DB.Exec(`UPDATE bookmarks SET updated_at=CURRENT_TIMESTAMP`)
	f.index(t)
	if len(f.embedder.requests) != 1 {
		t.Fatalf("unchanged posts were sent again: %v", f.embedder.requests)
	}

	// A new summary changes what the post is about: only that post is sent.
	f.store.DB.Exec(`INSERT INTO enrichments(bookmark_id,ai_summary) VALUES (?,?)`, f.ids["2"], "A guide to worker queues")
	f.store.DB.Exec(`UPDATE bookmarks SET updated_at=CURRENT_TIMESTAMP WHERE id=?`, f.ids["2"])
	f.index(t)
	if len(f.embedder.requests) != 2 || len(f.embedder.requests[1]) != 1 || !strings.Contains(f.embedder.requests[1][0], "worker queues") {
		t.Fatalf("changed post: %v", f.embedder.requests)
	}
	if ids, err := f.service.Search(ctx, "queue"); err != nil || len(ids) == 0 || ids[0] != f.ids["2"] {
		t.Fatalf("the new vector is not searched: %v %v", ids, err)
	}

	// A deleted post leaves the index.
	if err := f.store.DeleteBookmark(ctx, f.ids["2"]); err != nil {
		t.Fatal(err)
	}
	if ids, _ := f.service.Search(ctx, "queue"); len(ids) > 0 && ids[0] == f.ids["2"] {
		t.Fatalf("a deleted post is still found: %v", ids)
	}
}

func TestIndexingWaitsWhenItShould(t *testing.T) {
	f := library(t, map[string]string{"1": "Baking bread"})
	ctx := context.Background()
	asked := func() int { return len(f.embedder.requests) }

	f.keys.key = ""
	if busy, err := f.service.step(ctx); busy || err != nil || asked() != 0 {
		t.Fatalf("without a key: %v %v %d", busy, err, asked())
	}
	if _, err := f.service.Search(ctx, "bread"); !errors.Is(err, ErrNoKey) {
		t.Fatalf("search without a key or an index: %v", err)
	}
	f.keys.key = "key"

	f.store.DB.Exec(`INSERT INTO settings(key,value) VALUES ('ai_paused','true')`)
	if f.service.step(ctx); asked() != 0 {
		t.Fatal("indexed while AI is paused")
	}
	f.store.DB.Exec(`DELETE FROM settings WHERE key='ai_paused'`)

	f.store.SetMeaningSearch(ctx, false)
	if f.service.step(ctx); asked() != 0 {
		t.Fatal("indexed while search by meaning is off")
	}
	if _, err := f.service.Search(ctx, "bread"); !errors.Is(err, ErrOff) {
		t.Fatalf("search while off: %v", err)
	}
	f.store.SetMeaningSearch(ctx, true)

	// A rate limit rests everything; no post is marked against.
	f.embedder.fail = func([]string) error { return errors.New("Error 429, Status: RESOURCE_EXHAUSTED, retry in 30s") }
	if _, err := f.service.step(ctx); err == nil {
		t.Fatal("a rate limit should be reported")
	}
	if f.service.Status(ctx).RestingUntil == nil {
		t.Fatal("a rate limit should rest the worker")
	}
	before := asked()
	if f.service.step(ctx); asked() != before {
		t.Fatal("asked again while resting")
	}
	if status := f.service.Status(ctx); status.Indexed != 0 || status.Total != 1 {
		t.Fatalf("status while resting: %+v", status)
	}

	// A key the service rejects is not held against the posts either.
	f.service.Wake(ctx)
	f.embedder.fail = func([]string) error {
		return errors.New("Error 400, Message: API key not valid, Status: INVALID_ARGUMENT")
	}
	f.service.step(ctx)
	if status := f.service.Status(ctx); status.Indexed != 0 {
		t.Fatalf("a bad key marked posts as done: %+v", status)
	}

	// An expired key is answered like refused content; it must not be taken for it.
	f.service.Wake(ctx)
	f.embedder.fail = func([]string) error {
		return errors.New("Error 400, Message: API key expired. Please renew the API key., Status: INVALID_ARGUMENT")
	}
	f.service.step(ctx)
	if status := f.service.Status(ctx); status.Indexed != 0 || status.RestingUntil == nil {
		t.Fatalf("an expired key marked posts as done: %+v", status)
	}

	// While AI is paused a new search is not sent; one made before still works.
	f.service.Wake(ctx)
	f.embedder.fail = nil
	f.index(t)
	if _, err := f.service.Search(ctx, "bread"); err != nil {
		t.Fatal(err)
	}
	f.store.DB.Exec(`INSERT INTO settings(key,value) VALUES ('ai_paused','true')`)
	sent := f.embedder.queries
	if _, err := f.service.Search(ctx, "flour"); !errors.Is(err, ErrPaused) || f.embedder.queries != sent {
		t.Fatalf("a new search while paused: %v, %d sent", err, f.embedder.queries-sent)
	}
	if ids, err := f.service.Search(ctx, "bread"); err != nil || len(ids) != 1 {
		t.Fatalf("a known search while paused: %v %v", ids, err)
	}
	f.store.DB.Exec(`DELETE FROM settings WHERE key='ai_paused'`)
	if status := f.service.Status(ctx); status.Indexed != 1 || !status.Enabled || !status.Available {
		t.Fatalf("status once indexed: %+v", status)
	}
}

func TestOnePostTheServiceRefusesDoesNotBlockTheRest(t *testing.T) {
	f := library(t, map[string]string{"1": "Baking bread", "2": "FORBIDDEN text", "3": "Car engine"})
	f.embedder.fail = func(texts []string) error {
		for _, text := range texts {
			if strings.Contains(text, "FORBIDDEN") {
				return errors.New("Error 400, Message: content not supported, Status: INVALID_ARGUMENT")
			}
		}
		return nil
	}
	f.index(t)
	status := f.service.Status(context.Background())
	if status.Indexed != 3 || status.RestingUntil != nil {
		t.Fatalf("every post should be settled, with no rest: %+v", status)
	}
	if ids, err := f.service.Search(context.Background(), "bread"); err != nil || len(ids) == 0 || ids[0] != f.ids["1"] {
		t.Fatalf("the other posts should be searchable: %v %v", ids, err)
	}
	if ids, _ := f.service.Related(context.Background(), f.ids["2"], 5); len(ids) != 0 {
		t.Fatalf("the refused post has no vector: %v", ids)
	}
	// It is left alone until something changes, then tried again.
	asked := len(f.embedder.requests)
	f.index(t)
	if len(f.embedder.requests) != asked {
		t.Fatal("a refused post was sent again with nothing changed")
	}
	f.embedder.fail = nil
	f.service.Wake(context.Background())
	f.index(t)
	if ids, _ := f.service.Related(context.Background(), f.ids["2"], 5); len(f.embedder.requests) != asked+1 || ids == nil && f.service.Status(context.Background()).Indexed != 3 {
		t.Fatalf("after a change the refused post should be tried again: %d requests", len(f.embedder.requests)-asked)
	}
}

func TestQuantizeKeepsDirection(t *testing.T) {
	if Quantize([]float32{0, 0, 0}) != nil {
		t.Fatal("a vector with no direction has no packed form")
	}
	a := signed(Quantize([]float32{0.2, -0.1, 0.05, 0}))
	b := signed(Quantize([]float32{0.4, -0.2, 0.1, 0}))
	if string(Quantize([]float32{0.2, -0.1, 0.05, 0})) != string(Quantize([]float32{0.4, -0.2, 0.1, 0})) || length(a) != length(b) {
		t.Fatalf("vectors pointing the same way should pack the same: %v %v", a, b)
	}
	if a[0] != 127 || a[1] > -60 {
		t.Fatalf("unexpected packing: %v", a)
	}
}

func TestNothingStandsOutNothingIsFound(t *testing.T) {
	posts := map[string]string{}
	for index, text := range []string{"alpha beta", "gamma delta", "epsilon zeta", "eta theta", "iota kappa", "lambda omicron", "sigma upsilon", "phi chi", "psi omega", "aleph beth", "bread baking", "sourdough loaf"} {
		posts[strconv.Itoa(101+index)] = text
	}
	f := library(t, posts)
	f.index(t)
	ctx := context.Background()
	// The two baking posts stand clear of the rest for a baking search.
	if ids, err := f.service.Search(ctx, "flour"); err != nil || len(ids) != 2 {
		t.Fatalf("a search the library has posts on: %v %v", ids, err)
	}
	// Nothing here is about cars: no post is offered just for being the least far.
	if ids, err := f.service.Search(ctx, "car"); err != nil || len(ids) != 0 {
		t.Fatalf("a search the library has nothing on: %v %v", ids, err)
	}
	if ids, _ := f.service.Related(ctx, f.ids["101"], 5); len(ids) != 0 {
		t.Fatalf("a post unlike any other has no related posts: %v", ids)
	}
	if ids, _ := f.service.Related(ctx, f.ids["111"], 5); len(ids) != 1 || ids[0] != f.ids["112"] {
		t.Fatalf("related baking post: %v", ids)
	}
}
