package httpapi

import (
	"context"
	"net/http"

	"sortwise/internal/buildinfo"
	"sortwise/internal/updates"
)

// Updates reports whether a newer Sortwise is out, and turns the daily
// check on or off.
type Updates interface {
	State(ctx context.Context) updates.State
	SetCheck(ctx context.Context, on bool) error
}

func (s *Server) getUpdates(w http.ResponseWriter, r *http.Request) {
	if s.updates == nil {
		writeJSON(w, 200, updates.State{Version: buildinfo.Version})
		return
	}
	writeJSON(w, 200, s.updates.State(r.Context()))
}

func (s *Server) updateUpdates(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Check *bool `json:"check"`
	}
	if err := decodeJSON(w, r, &input, 1024); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	if s.updates == nil {
		writeError(w, 409, "unavailable", "This copy of Sortwise doesn't check for updates")
		return
	}
	if input.Check != nil {
		if err := s.updates.SetCheck(r.Context(), *input.Check); err != nil {
			writeError(w, 500, "storage_error", "Could not save the update setting")
			return
		}
	}
	writeJSON(w, 200, s.updates.State(r.Context()))
}
