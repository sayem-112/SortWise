package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"sortwise/internal/avatars"
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

var avatarTypes = map[string]string{".jpg": "image/jpeg", ".png": "image/png", ".webp": "image/webp", ".gif": "image/gif"}

// avatar serves a saved profile picture. The name is checked against the one
// shape saved pictures have, so nothing else in the folder, or outside it,
// can be asked for. A picture's name changes when the picture does, so
// browsers may keep it for good.
func (s *Server) avatar(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "file")
	// Another site's page asking, to learn whose posts are saved here, gets
	// the same answer as for a picture that does not exist.
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		writeError(w, 404, "not_found", "No such picture")
		return
	}
	if s.avatarDir == "" || !avatars.ValidFile(name) {
		writeError(w, 404, "not_found", "No such picture")
		return
	}
	file, err := os.Open(filepath.Join(s.avatarDir, name))
	if err != nil {
		writeError(w, 404, "not_found", "No such picture")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		writeError(w, 404, "not_found", "No such picture")
		return
	}
	// The type comes from the name, which was chosen from the picture's own
	// bytes when it was saved, never from this computer's file associations.
	w.Header().Set("Content-Type", avatarTypes[filepath.Ext(name)])
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, name, info.ModTime(), file)
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
