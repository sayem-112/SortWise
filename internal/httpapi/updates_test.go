package httpapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"sortwise/internal/database"
	"sortwise/internal/store"
	"sortwise/internal/updates"
)

func TestUpdateSettings(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	read := func(body []byte) updates.State {
		var state updates.State
		if err := json.Unmarshal(body, &state); err != nil {
			t.Fatal(err)
		}
		return state
	}

	// Without a checker, as in tests and the console: a build that doesn't check.
	plain := New(db)
	if w := requestJSON(t, plain, "GET", "/api/v1/settings/updates", nil, ""); w.Code != 200 || read(w.Body.Bytes()).Released {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
	if w := requestJSON(t, plain, "PATCH", "/api/v1/settings/updates", map[string]bool{"check": false}, ""); w.Code != 409 {
		t.Fatalf("got %d", w.Code)
	}

	handler := New(db, Dependencies{Updates: updates.New("0.8.0", store.New(db))})
	if state := read(requestJSON(t, handler, "GET", "/api/v1/settings/updates", nil, "").Body.Bytes()); !state.Released || !state.Check || state.Version != "0.8.0" {
		t.Fatalf("got %+v", state)
	}
	w := requestJSON(t, handler, "PATCH", "/api/v1/settings/updates", map[string]bool{"check": false}, "")
	if state := read(w.Body.Bytes()); w.Code != 200 || state.Check {
		t.Fatalf("got %d %+v", w.Code, state)
	}
	if w := requestJSON(t, handler, "PATCH", "/api/v1/settings/updates", map[string]string{"check": "no"}, ""); w.Code != 400 {
		t.Fatalf("a malformed change reported %d", w.Code)
	}
}
