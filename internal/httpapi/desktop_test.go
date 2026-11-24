package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"sortwise/internal/database"
	"sortwise/internal/desktop"
)

// fakeDesktop stands in for Windows.
type fakeDesktop struct {
	state desktop.State
	fail  bool
}

func (f *fakeDesktop) State() desktop.State { return f.state }
func (f *fakeDesktop) SetStartMenu(on bool) error {
	if f.fail {
		return errors.New("refused")
	}
	f.state.StartMenu = on
	return nil
}
func (f *fakeDesktop) SetStartup(on bool) error {
	f.state.Startup = on
	return nil
}

func TestDesktopSettings(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	read := func(body []byte) desktop.State {
		var state desktop.State
		if err := json.Unmarshal(body, &state); err != nil {
			t.Fatal(err)
		}
		return state
	}

	// Without Windows to set up, the settings say so and can't be changed.
	plain := New(db)
	if w := requestJSON(t, plain, "GET", "/api/v1/settings/desktop", nil, ""); w.Code != 200 || read(w.Body.Bytes()).Available {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
	if w := requestJSON(t, plain, "PATCH", "/api/v1/settings/desktop", map[string]bool{"startup": true}, ""); w.Code != 409 {
		t.Fatalf("changed settings with nothing to change: %d", w.Code)
	}

	windows := &fakeDesktop{state: desktop.State{Available: true, StartMenu: true, Startup: true}}
	handler := New(db, Dependencies{Desktop: windows})
	w := requestJSON(t, handler, "PATCH", "/api/v1/settings/desktop", map[string]bool{"startup": false}, "")
	if state := read(w.Body.Bytes()); w.Code != 200 || state.Startup || !state.StartMenu {
		t.Fatalf("got %d %+v", w.Code, state)
	}
	windows.fail = true
	if w := requestJSON(t, handler, "PATCH", "/api/v1/settings/desktop", map[string]bool{"startMenu": false}, ""); w.Code != 500 {
		t.Fatalf("a refused change reported %d", w.Code)
	}
	if w := requestJSON(t, handler, "PATCH", "/api/v1/settings/desktop", map[string]any{"startup": "yes"}, ""); w.Code != 400 {
		t.Fatalf("a malformed change reported %d", w.Code)
	}
}
