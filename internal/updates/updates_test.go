package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"sortwise/internal/database"
	"sortwise/internal/store"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		newer           bool
	}{
		{"0.8.0", "0.9.0", true},
		{"0.8.0", "v0.8.1", true},
		{"0.9.9", "0.10.0", true}, // numbers, not text
		{"0.8.0", "0.8.0", false}, // the release you're on
		{"0.8.0", "0.7.0", false}, // a build ahead of the release
		{"1.0.0", "0.99.0", false},
		{"dev", "0.9.0", false}, // development builds never see updates
		{"0.8.0", "", false},
		{"0.8.0", "0.9.0-beta", false},
	}
	for _, item := range cases {
		if got := Newer(item.current, item.latest); got != item.newer {
			t.Fatalf("Newer(%q, %q) = %v", item.current, item.latest, got)
		}
	}
}

// github stands in for GitHub's latest-release endpoint.
func github(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var asked atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		if r.Header.Get("User-Agent") != "Sortwise/0.8.0" {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, &asked
}

func checker(t *testing.T, version, endpoint string) *Checker {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c := New(version, store.New(db))
	c.endpoint = endpoint
	return c
}

func TestANewerReleaseIsFoundOnceADay(t *testing.T) {
	ctx := context.Background()
	server, asked := github(t, 200, `{"tag_name":"v0.9.0","html_url":"https://example.com/elsewhere"}`)
	c := checker(t, "0.8.0", server.URL)
	if c.State(ctx).Available != nil {
		t.Fatal("an update before any check")
	}
	if wait := c.checkIfDue(ctx); wait != every {
		t.Fatalf("next check in %v", wait)
	}
	state := c.State(ctx)
	if state.Available == nil || *state.Available != (Release{"0.9.0", "https://github.com/sayem-112/SortWise/releases/tag/v0.9.0"}) || state.CheckedAt == nil {
		t.Fatalf("got %+v", state)
	}
	// Checked today: no second request.
	if wait := c.checkIfDue(ctx); wait <= 0 || wait > retry || asked.Load() != 1 {
		t.Fatalf("checked again (%d requests, wait %v)", asked.Load(), wait)
	}
	// Turned off: the update isn't shown, and nothing is asked.
	if err := c.SetCheck(ctx, false); err != nil {
		t.Fatal(err)
	}
	if state := c.State(ctx); state.Check || state.Available != nil {
		t.Fatalf("off, got %+v", state)
	}
}

func TestNothingIsShownWithoutANewerPlainRelease(t *testing.T) {
	ctx := context.Background()
	for _, body := range []string{`{"tag_name":"v0.8.0"}`, `{"tag_name":"v0.7.2"}`, `{"tag_name":"nightly-<script>"}`} {
		server, _ := github(t, 200, body)
		c := checker(t, "0.8.0", server.URL)
		c.checkIfDue(ctx)
		if state := c.State(ctx); state.Available != nil || state.CheckedAt == nil {
			t.Fatalf("%s: got %+v", body, state)
		}
	}
	// Nothing published yet counts as a finished check.
	server, _ := github(t, 404, `{}`)
	c := checker(t, "0.8.0", server.URL)
	if wait := c.checkIfDue(ctx); wait != every {
		t.Fatalf("a 404 was retried in %v", wait)
	}
	// A failure is tried again in an hour.
	server, _ = github(t, 500, `{}`)
	c = checker(t, "0.8.0", server.URL)
	if wait := c.checkIfDue(ctx); wait != retry || c.State(ctx).CheckedAt != nil {
		t.Fatal("a failed check was recorded")
	}
}

func TestDevelopmentBuildsNeverCheck(t *testing.T) {
	server, asked := github(t, 200, `{"tag_name":"v9.0.0"}`)
	c := checker(t, "dev", server.URL)
	done := make(chan struct{})
	go func() { c.Run(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("a development build kept running the check")
	}
	if asked.Load() != 0 || c.State(context.Background()).Released {
		t.Fatal("a development build checked")
	}
}

func TestACheckFromAClockThatWasAheadDoesNotHoldBackTheNext(t *testing.T) {
	ctx := context.Background()
	server, asked := github(t, 200, `{"tag_name":"v0.9.0"}`)
	c := checker(t, "0.8.0", server.URL)
	if err := c.store.SaveUpdateCheck(ctx, "", time.Now().Add(72*time.Hour)); err != nil {
		t.Fatal(err)
	}
	c.checkIfDue(ctx)
	if asked.Load() != 1 || c.State(ctx).Available == nil {
		t.Fatal("a future check stamp held the check back")
	}
}
