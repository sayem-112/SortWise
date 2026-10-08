package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"sortwise/internal/database"
)

func pairedHandler(t *testing.T) (http.Handler, string, func()) {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "capture.db"))
	if err != nil {
		t.Fatal(err)
	}
	handler := New(db)
	var code struct {
		Code string `json:"code"`
	}
	json.Unmarshal(requestJSON(t, handler, http.MethodPost, "/api/v1/extension/pairing-codes", nil, "").Body.Bytes(), &code)
	var paired struct {
		Token string `json:"token"`
	}
	json.Unmarshal(requestJSON(t, handler, http.MethodPost, "/api/v1/extension/pair", map[string]string{"code": code.Code}, "").Body.Bytes(), &paired)
	return handler, paired.Token, func() { db.Close() }
}

func TestImportXTimelineAndCapturedPosts(t *testing.T) {
	handler, token, done := pairedHandler(t)
	defer done()
	timeline, err := os.ReadFile("../xgraphql/testdata/bookmarks.json")
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"source": "sync", "timeline": json.RawMessage(timeline)}
	if w := requestJSON(t, handler, http.MethodPost, "/api/v1/imports/x", body, ""); w.Code != 401 {
		t.Fatalf("expected 401 without a token, got %d", w.Code)
	}
	w := requestJSON(t, handler, http.MethodPost, "/api/v1/imports/x", body, token)
	if w.Code != 201 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var result xImportResponse
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Inserted != 4 || result.Skipped != 1 || result.Received != 5 {
		t.Fatalf("unexpected result: %+v", result)
	}

	// A post captured when bookmarked on X, already saved: nothing changes.
	capture := map[string]any{"source": "capture", "tweets": []json.RawMessage{
		json.RawMessage(`{"__typename":"Tweet","rest_id":"300","core":{"user_results":{"result":{"legacy":{"name":"Linus","screen_name":"linus"}}}},"legacy":{"full_text":"Different text now","created_at":"Tue Sep 01 11:00:00 +0000 2026"}}`),
		json.RawMessage(`{"__typename":"TweetTombstone"}`),
	}}
	w = requestJSON(t, handler, http.MethodPost, "/api/v1/imports/x", capture, token)
	json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != 201 || result.Updated != 1 || result.Skipped != 1 {
		t.Fatalf("capture: %d %+v", w.Code, result)
	}

	w = requestJSON(t, handler, http.MethodPost, "/api/v1/imports/x/removed", map[string]any{"postIds": []string{"300"}}, token)
	if w.Code != 200 || w.Body.String() != "{\"marked\":1}\n" {
		t.Fatalf("removed: %d %s", w.Code, w.Body.String())
	}

	if w := requestJSON(t, handler, http.MethodPost, "/api/v1/imports/x", map[string]any{"source": "other"}, token); w.Code != 400 {
		t.Fatalf("expected an unknown source to be rejected, got %d", w.Code)
	}
}

func timelineBody(t *testing.T) json.RawMessage {
	t.Helper()
	timeline, err := os.ReadFile("../xgraphql/testdata/bookmarks.json")
	if err != nil {
		t.Fatal(err)
	}
	return json.RawMessage(timeline)
}

func TestDeletedPostsStayDeletedUntilBookmarkedAgain(t *testing.T) {
	handler, token, done := pairedHandler(t)
	defer done()
	sync := map[string]any{"source": "sync", "timeline": timelineBody(t)}
	requestJSON(t, handler, http.MethodPost, "/api/v1/imports/x", sync, token)

	var page struct {
		Items []struct {
			ID     int64  `json:"id"`
			PostID string `json:"postId"`
		} `json:"items"`
	}
	json.Unmarshal(requestJSON(t, handler, http.MethodGet, "/api/v1/bookmarks?pageSize=10", nil, "").Body.Bytes(), &page)
	var deleted int64
	for _, item := range page.Items {
		if item.PostID == "200" {
			deleted = item.ID
		}
	}
	if w := requestJSON(t, handler, http.MethodDelete, "/api/v1/bookmarks/"+strconv.FormatInt(deleted, 10)+"?confirm=true", nil, ""); w.Code != 204 {
		t.Fatalf("delete: %d", w.Code)
	}

	var result xImportResponse
	json.Unmarshal(requestJSON(t, handler, http.MethodPost, "/api/v1/imports/x", sync, token).Body.Bytes(), &result)
	if result.Deleted != 1 || result.Inserted != 0 {
		t.Fatalf("a sync should skip the deleted post: %+v", result)
	}

	capture := map[string]any{"source": "capture", "tweets": []json.RawMessage{json.RawMessage(`{"__typename":"Tweet","rest_id":"200","core":{"user_results":{"result":{"legacy":{"name":"G","screen_name":"grace"}}}},"legacy":{"full_text":"back","created_at":"Tue Sep 01 09:00:00 +0000 2026"}}`)}}
	json.Unmarshal(requestJSON(t, handler, http.MethodPost, "/api/v1/imports/x", capture, token).Body.Bytes(), &result)
	if result.Inserted != 1 {
		t.Fatalf("bookmarking it again on X should bring it back: %+v", result)
	}
}

func TestOnlyFromNowOnStopsAtTheBoundary(t *testing.T) {
	handler, token, done := pairedHandler(t)
	defer done()
	// The list at the moment of choosing: posts 100 to 400 are "before".
	scope := map[string]any{"scope": "new", "timeline": timelineBody(t)}
	if w := requestJSON(t, handler, http.MethodPost, "/api/v1/imports/x/scope", scope, token); w.Code != 200 {
		t.Fatalf("scope: %d %s", w.Code, w.Body.String())
	}
	// Later the list has one new post on top, then the old ones.
	newer := `{"data":{"bookmark_timeline_v2":{"timeline":{"instructions":[{"entries":[{"content":{"itemContent":{"__typename":"TimelineTweet","tweet_results":{"result":{"__typename":"Tweet","rest_id":"900","core":{"user_results":{"result":{"legacy":{"name":"N","screen_name":"newbie"}}}},"legacy":{"full_text":"fresh","created_at":"Tue Sep 08 09:00:00 +0000 2026"}}}}}},{"content":{"itemContent":{"__typename":"TimelineTweet","tweet_results":{"result":{"__typename":"Tweet","rest_id":"100","core":{"user_results":{"result":{"legacy":{"name":"Ada","screen_name":"Ada"}}}},"legacy":{"full_text":"old","created_at":"Tue Sep 01 10:00:00 +0000 2026"}}}}}}]}]}}}}`
	var result xImportResponse
	json.Unmarshal(requestJSON(t, handler, http.MethodPost, "/api/v1/imports/x", map[string]any{"source": "sync", "timeline": json.RawMessage(newer)}, token).Body.Bytes(), &result)
	if result.Inserted != 1 || !result.ReachedBoundary {
		t.Fatalf("only the new post should be saved and the sync told to stop: %+v", result)
	}
	var summary struct {
		Scope     string `json:"scope"`
		Bookmarks int    `json:"bookmarks"`
	}
	json.Unmarshal(requestJSON(t, handler, http.MethodGet, "/api/v1/extension/summary", nil, token).Body.Bytes(), &summary)
	if summary.Scope != "new" || summary.Bookmarks != 1 {
		t.Fatalf("summary: %+v", summary)
	}
}

func TestOtherXAccountIsRefused(t *testing.T) {
	handler, token, done := pairedHandler(t)
	defer done()
	body := map[string]any{"source": "sync", "timeline": timelineBody(t), "account": "111"}
	if w := requestJSON(t, handler, http.MethodPost, "/api/v1/imports/x", body, token); w.Code != 201 {
		t.Fatalf("first account: %d", w.Code)
	}
	body["account"] = "222"
	if w := requestJSON(t, handler, http.MethodPost, "/api/v1/imports/x", body, token); w.Code != 409 {
		t.Fatalf("expected the second account to be refused, got %d", w.Code)
	}
}
