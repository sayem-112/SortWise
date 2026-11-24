package httpapi

import (
	"net/http"

	"sortwise/internal/desktop"
)

// Desktop sets Sortwise up in Windows: the Start menu shortcut that Windows
// search finds, and starting at sign-in.
type Desktop interface {
	State() desktop.State
	SetStartMenu(on bool) error
	SetStartup(on bool) error
}

func (s *Server) desktopState() desktop.State {
	if s.desktop == nil {
		return desktop.State{}
	}
	return s.desktop.State()
}

func (s *Server) getDesktop(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.desktopState())
}

func (s *Server) updateDesktop(w http.ResponseWriter, r *http.Request) {
	var input struct {
		StartMenu *bool `json:"startMenu"`
		Startup   *bool `json:"startup"`
	}
	if err := decodeJSON(w, r, &input, 1024); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	if s.desktop == nil {
		writeError(w, 409, "unavailable", "This copy of Sortwise can't change Windows settings")
		return
	}
	if input.StartMenu != nil {
		if err := s.desktop.SetStartMenu(*input.StartMenu); err != nil {
			writeError(w, 500, "windows_error", "Windows did not accept the change to the Start menu")
			return
		}
	}
	if input.Startup != nil {
		if err := s.desktop.SetStartup(*input.Startup); err != nil {
			writeError(w, 500, "windows_error", "Windows did not accept the change to startup apps")
			return
		}
	}
	writeJSON(w, 200, s.desktop.State())
}
