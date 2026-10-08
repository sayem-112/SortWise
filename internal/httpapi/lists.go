package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"sortwise/internal/store"
)

type listInput struct {
	Name string `json:"name"`
}

func (s *Server) lists(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Lists(r.Context())
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load lists")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) createList(w http.ResponseWriter, r *http.Request) {
	var input listInput
	if err := decodeJSON(w, r, &input, 4096); err != nil {
		writeError(w, 400, "invalid_list", err.Error())
		return
	}
	item, err := s.store.CreateList(r.Context(), input.Name)
	if err != nil {
		writeListError(w, err)
		return
	}
	writeJSON(w, 201, item)
}

func (s *Server) renameList(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	var input listInput
	if err := decodeJSON(w, r, &input, 4096); err != nil {
		writeError(w, 400, "invalid_list", err.Error())
		return
	}
	item, err := s.store.RenameList(r.Context(), id, input.Name)
	if err != nil {
		writeListError(w, err)
		return
	}
	writeJSON(w, 200, item)
}

func (s *Server) deleteList(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteList(r.Context(), id); err != nil {
		writeListError(w, err)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) addToList(w http.ResponseWriter, r *http.Request)      { s.setInList(w, r, true) }
func (s *Server) removeFromList(w http.ResponseWriter, r *http.Request) { s.setInList(w, r, false) }

func (s *Server) setInList(w http.ResponseWriter, r *http.Request, in bool) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	bookmarkID, err := strconv.ParseInt(chi.URLParam(r, "bookmarkId"), 10, 64)
	if err != nil || bookmarkID < 1 {
		writeError(w, 400, "invalid_id", "Bookmark ID must be a positive number")
		return
	}
	if err := s.store.SetInList(r.Context(), id, bookmarkID, in); err != nil {
		writeListError(w, err)
		return
	}
	w.WriteHeader(204)
}

func writeListError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, 404, "not_found", "List or bookmark not found")
	case errors.Is(err, store.ErrFavorites):
		writeError(w, 409, "favorites", err.Error())
	case errors.Is(err, store.ErrListExists):
		writeError(w, 409, "list_exists", "You already have a list with this name")
	default:
		writeError(w, 422, "invalid_list", err.Error())
	}
}
