package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"sortwise/internal/model"
	"sortwise/internal/semantic"
	"sortwise/internal/store"
)

// addMeaning widens a word search with the posts closest in meaning, and
// says whether it could. remote allows asking the AI service for a search
// not made before; without it only searches made before are widened.
func (s *Server) addMeaning(ctx context.Context, query *model.BookmarkQuery, remote bool) *model.SearchInfo {
	if s.semantic == nil {
		return &model.SearchInfo{Notice: meaningNotice(semantic.ErrOff)}
	}
	search := s.semantic.Recall
	if remote {
		search = s.semantic.Search
	}
	ids, err := search(ctx, query.Text)
	if err != nil {
		return &model.SearchInfo{Notice: meaningNotice(err)}
	}
	query.Hybrid, query.Semantic = true, ids
	return &model.SearchInfo{Meaning: true}
}

// meaningNotice says, for the person searching, why only word matches are shown.
func meaningNotice(err error) string {
	switch {
	case errors.Is(err, semantic.ErrOff):
		return "Search by meaning is turned off in Settings."
	case errors.Is(err, semantic.ErrNoKey):
		return "Search by meaning needs a Gemini API key. Add one in Settings."
	case errors.Is(err, semantic.ErrNotReady):
		return "Your posts are still being prepared for search by meaning."
	case errors.Is(err, semantic.ErrResting):
		return "Gemini is limiting requests right now. Try again in a little while."
	case errors.Is(err, semantic.ErrPaused):
		return "AI is paused in Settings, so a new search by meaning is not sent."
	case errors.Is(err, semantic.ErrUnknownQuery):
		return "Open this search once to find posts by meaning."
	}
	return "Gemini could not be reached."
}

// relatedBookmarks returns the posts closest in meaning to one post. It is
// worked out from what is already saved; nothing is sent anywhere.
func (s *Server) relatedBookmarks(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	items := []model.RelatedBookmark{}
	if s.semantic != nil {
		// Twice what is shown, since archived posts are left out afterwards.
		ids, err := s.semantic.Related(r.Context(), id, 2*relatedShown)
		if err == nil {
			items, err = s.store.RelatedBookmarks(r.Context(), ids, relatedShown)
		}
		if err != nil {
			writeError(w, 500, "storage_error", "Could not load related posts")
			return
		}
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

// relatedShown is how many related posts a post shows.
const relatedShown = 5

type collectionInput struct {
	Name  string                `json:"name"`
	Query model.CollectionQuery `json:"query"`
}

// countCollection fills in how many posts a collection holds right now. A
// search by meaning is counted with its saved vector, so listing collections
// never asks the AI service anything.
func (s *Server) countCollection(ctx context.Context, item *model.Collection) {
	query := store.CollectionBookmarkQuery(item.Query)
	if item.Query.Mode == "meaning" {
		s.addMeaning(ctx, &query, false)
	}
	item.Count, _ = s.store.CountBookmarks(ctx, query)
}

func (s *Server) collections(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Collections(r.Context())
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load smart collections")
		return
	}
	for index := range items {
		s.countCollection(r.Context(), &items[index])
	}
	writeJSON(w, 200, map[string]any{"items": items, "colors": store.ListColors})
}

func (s *Server) createCollection(w http.ResponseWriter, r *http.Request) {
	var input collectionInput
	if err := decodeJSON(w, r, &input, 16384); err != nil {
		writeError(w, 400, "invalid_collection", err.Error())
		return
	}
	item, err := s.store.CreateCollection(r.Context(), input.Name, input.Query)
	if err != nil {
		writeCollectionError(w, err)
		return
	}
	s.countCollection(r.Context(), &item)
	writeJSON(w, 201, item)
}

func (s *Server) updateCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	var input model.CollectionUpdate
	if err := decodeJSON(w, r, &input, 16384); err != nil {
		writeError(w, 400, "invalid_collection", err.Error())
		return
	}
	item, err := s.store.UpdateCollection(r.Context(), id, input)
	if err != nil {
		writeCollectionError(w, err)
		return
	}
	s.countCollection(r.Context(), &item)
	writeJSON(w, 200, item)
}

func (s *Server) deleteCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteCollection(r.Context(), id); err != nil {
		writeCollectionError(w, err)
		return
	}
	w.WriteHeader(204)
}

func writeCollectionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, 404, "not_found", "Smart collection not found")
	case errors.Is(err, store.ErrCollectionExists):
		writeError(w, 409, "collection_exists", "You already have a smart collection with this name")
	default:
		writeError(w, 422, "invalid_collection", err.Error())
	}
}
