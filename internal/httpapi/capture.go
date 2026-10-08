package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"sortwise/internal/model"
	"sortwise/internal/store"
	"sortwise/internal/xgraphql"
)

// xImportRequest carries X's own data as the extension received it: a page of
// the Bookmarks timeline, or single posts captured when you bookmark them. The
// app reads it here so a change on X's side needs only an app update.
type xImportRequest struct {
	Source   string            `json:"source"`
	Timeline json.RawMessage   `json:"timeline,omitempty"`
	Tweets   []json.RawMessage `json:"tweets,omitempty"`
	// Account is the signed-in X account's user ID, from X's own cookie.
	Account string `json:"account,omitempty"`
}

type xImportResponse struct {
	model.ImportResult
	// Received is how many posts X's data held, including unreadable ones.
	Received int `json:"received"`
	// Deleted counts posts skipped because they were deleted in Sortwise.
	Deleted int `json:"deleted"`
	// ReachedBoundary means the sync reached bookmarks from before "only from
	// now on" was chosen, and should stop.
	ReachedBoundary bool `json:"reachedBoundary"`
}

var xImportSources = map[string]bool{"sync": true, "capture": true, "scroll": true}

func (s *Server) importXData(w http.ResponseWriter, r *http.Request) {
	var request xImportRequest
	if err := decodeJSON(w, r, &request, 16<<20); err != nil {
		writeError(w, 400, "invalid_import", err.Error())
		return
	}
	if !xImportSources[request.Source] {
		writeError(w, 400, "invalid_import", "source must be sync, capture, or scroll")
		return
	}
	if len(request.Tweets) > 100 {
		writeError(w, 400, "invalid_import", "send at most 100 posts at a time")
		return
	}
	if !s.checkAccount(w, r, request.Account) {
		return
	}
	now := time.Now()
	var posts []model.ImportedBookmark
	skipped := 0
	if len(request.Timeline) > 0 {
		parsed, unreadable, err := xgraphql.ParseTimeline(request.Timeline, now)
		if err != nil {
			writeError(w, 400, "invalid_import", "The timeline data could not be read")
			return
		}
		posts, skipped = parsed, unreadable
	}
	for _, raw := range request.Tweets {
		post, err := xgraphql.ParseTweet(raw, now)
		if err != nil {
			skipped++
			continue
		}
		posts = append(posts, post)
	}

	intake, err := s.store.FilterIntake(r.Context(), request.Source, posts)
	if err != nil {
		writeError(w, 500, "storage_error", "Could not read the import settings")
		return
	}
	response := xImportResponse{Received: len(posts) + skipped, Deleted: intake.Deleted, ReachedBoundary: intake.ReachedBoundary}
	if intake.ReachedBoundary {
		response.Received = len(intake.Posts) + intake.Deleted
	}
	valid := make([]model.ImportedBookmark, 0, len(intake.Posts))
	for _, post := range intake.Posts {
		if store.ValidateImportedBookmark(post) != nil {
			skipped++
			continue
		}
		valid = append(valid, post)
	}
	for start := 0; start < len(valid); start += 100 {
		end := min(start+100, len(valid))
		result, err := s.store.Import(r.Context(), model.ImportBatch{Bookmarks: valid[start:end], Source: request.Source})
		if err != nil {
			writeError(w, 500, "storage_error", "Could not save the imported bookmarks")
			return
		}
		response.ImportID = result.ImportID
		response.Inserted += result.Inserted
		response.Updated += result.Updated
		response.Upgraded += result.Upgraded
		response.Unchanged += result.Unchanged
	}
	response.Skipped = skipped
	writeJSON(w, http.StatusCreated, response)
}

// checkAccount refuses X data from an account other than the library's own.
func (s *Server) checkAccount(w http.ResponseWriter, r *http.Request, account string) bool {
	if err := s.store.CheckAccount(r.Context(), account); err != nil {
		if errors.Is(err, store.ErrOtherAccount) {
			writeError(w, 409, "other_account", "This X account is not the one this Sortwise library belongs to. Switch back to that account on X, or use a separate Sortwise library.")
			return false
		}
		writeError(w, 500, "storage_error", "Could not check the X account")
		return false
	}
	return true
}

type scopeRequest struct {
	Scope    string          `json:"scope"`
	Timeline json.RawMessage `json:"timeline,omitempty"`
	Account  string          `json:"account,omitempty"`
}

// setImportScope records what syncs bring in. "new" needs the first page of
// the bookmarks list: its posts mark where every later sync stops.
func (s *Server) setImportScope(w http.ResponseWriter, r *http.Request) {
	var request scopeRequest
	if err := decodeJSON(w, r, &request, 16<<20); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	if !s.checkAccount(w, r, request.Account) {
		return
	}
	var first []string
	switch request.Scope {
	case store.ScopeAll:
	case store.ScopeNew:
		posts, _, err := xgraphql.ParseTimeline(request.Timeline, time.Now())
		if err != nil {
			writeError(w, 400, "invalid_request", "The bookmarks list could not be read")
			return
		}
		for _, post := range posts {
			first = append(first, post.PostID)
		}
		if len(first) == 0 {
			// An empty list on X: everything from here on is new anyway.
			first = []string{}
		}
	default:
		writeError(w, 400, "invalid_request", "scope must be all or new")
		return
	}
	if err := s.store.SetImportScope(r.Context(), request.Scope, first); err != nil {
		writeError(w, 500, "storage_error", "Could not save the import choice")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scope": request.Scope, "boundary": len(first)})
}

type removedRequest struct {
	PostIDs []string `json:"postIds"`
}

// markRemovedOnX records posts you unbookmarked on X. They stay in the library.
func (s *Server) markRemovedOnX(w http.ResponseWriter, r *http.Request) {
	var request removedRequest
	if err := decodeJSON(w, r, &request, 1<<20); err != nil || len(request.PostIDs) == 0 || len(request.PostIDs) > 500 {
		writeError(w, 400, "invalid_request", "Send between 1 and 500 post IDs")
		return
	}
	marked, err := s.store.MarkRemovedOnX(r.Context(), request.PostIDs)
	if err != nil {
		writeError(w, 500, "storage_error", "Could not update the bookmarks")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"marked": marked})
}
