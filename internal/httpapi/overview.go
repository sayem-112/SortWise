package httpapi

import (
	"errors"
	"net/http"
	"time"

	"sortwise/internal/semantic"
)

// mapPoint is one post on the library map: where it sits and the category
// it is colored by (0 for none).
type mapPoint struct {
	ID       int64   `json:"id"`
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	Category int64   `json:"category"`
}

// libraryMap draws the library as an archipelago: an island per main
// category, related topics near each other, and posts about the same thing
// together within an island. Only posts with a meaning vector can be placed;
// indexed says how many of total that is. When the map can't be drawn at
// all, notice says why.
func (s *Server) libraryMap(w http.ResponseWriter, r *http.Request) {
	categories, err := s.store.MainCategories(r.Context())
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load the library map")
		return
	}
	response := struct {
		Points  []mapPoint        `json:"points"`
		Islands []semantic.Island `json:"islands"`
		Aspect  float32           `json:"aspect"`
		Indexed int               `json:"indexed"`
		Total   int               `json:"total"`
		Notice  string            `json:"notice,omitempty"`
	}{Points: []mapPoint{}, Islands: []semantic.Island{}, Total: len(categories)}
	if s.semantic == nil {
		response.Notice = mapNotice(semantic.ErrOff)
		writeJSON(w, 200, response)
		return
	}
	// Archived posts are not in categories, so they stay off the map.
	drawn, err := s.semantic.Layout(r.Context(), categories)
	if errors.Is(err, semantic.ErrOff) {
		response.Notice = mapNotice(err)
		writeJSON(w, 200, response)
		return
	}
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load the library map")
		return
	}
	for _, point := range drawn.Points {
		response.Points = append(response.Points, mapPoint{ID: point.ID, X: point.X, Y: point.Y, Category: categories[point.ID]})
	}
	response.Islands, response.Aspect = drawn.Islands, drawn.Aspect
	response.Indexed = len(response.Points)
	writeJSON(w, 200, response)
}

func mapNotice(err error) string {
	if errors.Is(err, semantic.ErrOff) {
		return "The map uses search by meaning, which is turned off in Settings."
	}
	return ""
}

// savedDays counts the library's posts by the day they were saved, for the
// past year: ?since=YYYY-MM-DD starts elsewhere.
func (s *Server) savedDays(w http.ResponseWriter, r *http.Request) {
	since := time.Now().AddDate(-1, 0, 1)
	if value := r.URL.Query().Get("since"); value != "" {
		parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
		if err != nil {
			writeError(w, 400, "invalid_date", "since must be a date like 2026-01-31")
			return
		}
		since = parsed
	}
	days, err := s.store.SavedByDay(r.Context(), since, time.Local)
	if err != nil {
		writeError(w, 500, "storage_error", "Could not count saved posts")
		return
	}
	writeJSON(w, 200, days)
}
