package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"sortwise/internal/model"
)

func (s *Server) authors(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	result, err := s.store.Authors(r.Context(), model.AuthorQuery{Text: r.URL.Query().Get("q"), Sort: r.URL.Query().Get("sort"), Page: page, PageSize: pageSize})
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load authors")
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) authorProfile(w http.ResponseWriter, r *http.Request) {
	// chi hands back the path segment as it was sent; a handle may arrive encoded.
	username := chi.URLParam(r, "username")
	if decoded, err := url.PathUnescape(username); err == nil {
		username = decoded
	}
	username = strings.TrimPrefix(username, "@")
	profile, err := s.store.AuthorProfile(r.Context(), username)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "No saved posts from this author")
		return
	}
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load the author")
		return
	}
	writeJSON(w, 200, profile)
}

// quickSearch answers the finder as you type: a few best posts with excerpts,
// and matching authors.
func (s *Server) quickSearch(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	result, err := s.store.QuickSearch(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		writeError(w, 500, "storage_error", "Could not search")
		return
	}
	writeJSON(w, 200, result)
}
