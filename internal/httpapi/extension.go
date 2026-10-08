package httpapi

import (
	"net/http"
	"strings"
)

// extensionPaths are the only endpoints a browser extension may call. Any
// installed extension can send requests with a chrome-extension:// origin, so the
// rest of the API (reading the library, settings, creating pairing codes) stays
// limited to the app's own pages.
var extensionPaths = map[string]bool{
	"/api/v1/health":            true,
	"/api/v1/extension/pair":    true,
	"/api/v1/extension/pairing": true,
	"/api/v1/extension/summary": true,
	"/api/v1/imports":           true,
	"/api/v1/imports/x":         true,
	"/api/v1/imports/x/removed": true,
	"/api/v1/imports/x/scope":   true,
}

func isExtensionOrigin(origin string) bool {
	return strings.HasPrefix(origin, "chrome-extension://") || strings.HasPrefix(origin, "extension://")
}

// extensionSummary lets a paired extension show the library size and the last
// import, and confirms its token is still valid.
func (s *Server) extensionSummary(w http.ResponseWriter, r *http.Request) {
	var bookmarks int
	var lastImport, lastImportInserted any
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM bookmarks`).Scan(&bookmarks)
	var completedAt string
	var inserted int
	err := s.db.QueryRowContext(r.Context(), `SELECT COALESCE(completed_at,started_at) FROM imports ORDER BY id DESC LIMIT 1`).Scan(&completedAt)
	if err == nil {
		lastImport = completedAt
		// Imports arrive in batches; sum the batches from the same session (within an hour).
		_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(inserted_count),0) FROM imports WHERE started_at >= datetime(?, '-1 hour')`, completedAt).Scan(&inserted)
		lastImportInserted = inserted
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookmarks": bookmarks, "lastImportAt": lastImport, "lastImportInserted": lastImportInserted, "appUrl": "http://127.0.0.1" + portSuffix(r.Host), "scope": s.store.ImportScope(r.Context())})
}

func portSuffix(host string) string {
	if index := strings.LastIndex(host, ":"); index != -1 {
		return host[index:]
	}
	return ""
}

type extensionConnection struct {
	ID         int64   `json:"id"`
	Label      string  `json:"label"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt *string `json:"lastUsedAt"`
}

// listExtensionConnections shows the app's Settings page which extensions are paired.
func (s *Server) listExtensionConnections(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,label,created_at,last_used_at FROM extension_tokens WHERE revoked_at IS NULL ORDER BY COALESCE(last_used_at,created_at) DESC`)
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load extension connections")
		return
	}
	defer rows.Close()
	items := []extensionConnection{}
	for rows.Next() {
		var item extensionConnection
		if err := rows.Scan(&item.ID, &item.Label, &item.CreatedAt, &item.LastUsedAt); err != nil {
			writeError(w, 500, "storage_error", "Could not load extension connections")
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// revokeExtensionConnections disconnects every paired extension; each must pair again.
func (s *Server) revokeExtensionConnections(w http.ResponseWriter, r *http.Request) {
	if _, err := s.db.ExecContext(r.Context(), `UPDATE extension_tokens SET revoked_at=CURRENT_TIMESTAMP WHERE revoked_at IS NULL`); err != nil {
		writeError(w, 500, "storage_error", "Could not disconnect extensions")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
