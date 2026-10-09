package httpapi

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"sortwise/internal/ai"
	"sortwise/internal/buildinfo"
	"sortwise/internal/credentials"
	"sortwise/internal/model"
	"sortwise/internal/store"
	webassets "sortwise/web"
)

type Server struct {
	db          *sql.DB
	store       *store.Store
	credentials credentials.Manager
	worker      *ai.Worker
	providers   map[string]ai.Factory
}

type Dependencies struct {
	Credentials credentials.Manager
	Worker      *ai.Worker
	Provider    ai.Factory
	Providers   map[string]ai.Factory
}

func New(db *sql.DB, dependencies ...Dependencies) http.Handler {
	deps := Dependencies{Credentials: credentials.New(), Providers: ai.DefaultFactories()}
	if len(dependencies) > 0 {
		if dependencies[0].Credentials != nil {
			deps.Credentials = dependencies[0].Credentials
		}
		if dependencies[0].Provider != nil {
			deps.Providers["gemini"] = dependencies[0].Provider
		}
		if dependencies[0].Providers != nil {
			deps.Providers = dependencies[0].Providers
		}
		deps.Worker = dependencies[0].Worker
	}
	s := &Server{db: db, store: store.New(db), credentials: deps.Credentials, worker: deps.Worker, providers: deps.Providers}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, middleware.Compress(5))
	r.Use(s.security)
	r.Get("/api/v1/health", s.health)
	r.Get("/api/v1/setup", s.setup)
	r.Post("/api/v1/extension/pairing-codes", s.createPairingCode)
	r.Post("/api/v1/extension/pair", s.pairExtension)
	r.With(s.requireExtension).Delete("/api/v1/extension/pairing", s.unpairExtension)
	r.With(s.requireExtension).Post("/api/v1/imports", s.importBookmarks)
	r.With(s.requireExtension).Post("/api/v1/imports/x", s.importXData)
	r.With(s.requireExtension).Post("/api/v1/imports/x/removed", s.markRemovedOnX)
	r.With(s.requireExtension).Post("/api/v1/imports/x/scope", s.setImportScope)
	r.With(s.requireExtension).Get("/api/v1/extension/summary", s.extensionSummary)
	r.Get("/api/v1/extension/connections", s.listExtensionConnections)
	r.Delete("/api/v1/extension/connections", s.revokeExtensionConnections)
	r.Get("/api/v1/bookmarks", s.listBookmarks)
	r.Get("/api/v1/bookmarks/{id}", s.getBookmark)
	r.Patch("/api/v1/bookmarks/{id}", s.updateBookmark)
	r.Delete("/api/v1/bookmarks/{id}", s.deleteBookmark)
	r.Get("/api/v1/categories", s.listCategories)
	r.Get("/api/v1/tags", s.listTags)
	r.Post("/api/v1/categories", s.createCategory)
	r.Patch("/api/v1/categories/{id}", s.updateCategory)
	r.Delete("/api/v1/categories/{id}", s.deactivateCategory)
	r.Post("/api/v1/categories/{id}/merge", s.mergeCategory)
	r.Post("/api/v1/tags", s.createTag)
	r.Patch("/api/v1/tags/{id}", s.updateTag)
	r.Delete("/api/v1/tags/{id}", s.deleteTag)
	r.Post("/api/v1/tags/{id}/aliases", s.addTagAlias)
	r.Delete("/api/v1/tags/{id}/aliases", s.deleteTagAlias)
	r.Post("/api/v1/tags/{id}/merge", s.mergeTag)
	r.Get("/api/v1/lists", s.lists)
	r.Post("/api/v1/lists", s.createList)
	r.Patch("/api/v1/lists/{id}", s.updateList)
	r.Delete("/api/v1/lists/{id}", s.deleteList)
	r.Post("/api/v1/lists/{id}/bookmarks", s.setManyInList)
	r.Put("/api/v1/lists/{id}/bookmarks/{bookmarkId}", s.addToList)
	r.Delete("/api/v1/lists/{id}/bookmarks/{bookmarkId}", s.removeFromList)
	r.Get("/api/v1/dashboard", s.dashboard)
	r.Get("/api/v1/exports/bookmarks.json", s.exportJSON)
	r.Get("/api/v1/exports/bookmarks.csv", s.exportCSV)
	r.Post("/api/v1/backups", s.createBackup)
	r.Get("/api/v1/backups/{name}", s.downloadBackup)
	r.Get("/api/v1/settings/ai", s.getAISettings)
	r.Patch("/api/v1/settings/ai", s.updateAISettings)
	r.Put("/api/v1/settings/ai/keys/{provider}", s.saveAIKey)
	r.Delete("/api/v1/settings/ai/keys/{provider}", s.deleteAIKey)
	r.Post("/api/v1/settings/ai/test", s.testAI)
	r.Get("/api/v1/jobs", s.listJobs)
	r.Post("/api/v1/jobs/{id}/retry", s.retryJob)
	r.Post("/api/v1/bookmarks/{id}/reprocess", s.reprocessBookmark)
	r.Get("/api/v1/taxonomy/proposals", s.listProposals)
	r.Post("/api/v1/taxonomy/proposals/categories", s.proposeCategories)
	r.Post("/api/v1/taxonomy/proposals/merges", s.proposeMerges)
	r.Post("/api/v1/taxonomy/proposals/{id}/accept", s.acceptProposal)
	r.Post("/api/v1/taxonomy/proposals/{id}/dismiss", s.dismissProposal)

	dist, err := fs.Sub(webassets.Assets, "dist")
	if err == nil {
		files := http.FileServer(http.FS(dist))
		r.Handle("/assets/*", files)
		r.Handle("/logo.svg", files)
	}
	r.NotFound(s.notFound)
	return r
}

func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if split, _, err := net.SplitHostPort(host); err == nil {
			host = split
		}
		if host != "127.0.0.1" && !strings.EqualFold(host, "localhost") {
			writeError(w, http.StatusForbidden, "invalid_host", "Requests must use the local Sortwise address")
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" && !allowedOrigin(origin, r.Host) {
			writeError(w, http.StatusForbidden, "invalid_origin", "Request origin is not allowed")
			return
		}
		if isExtensionOrigin(origin) && !extensionPaths[r.URL.Path] {
			writeError(w, http.StatusForbidden, "invalid_origin", "Browser extensions may only import bookmarks")
			return
		}
		if strings.HasPrefix(origin, "chrome-extension://") || strings.HasPrefix(origin, "extension://") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https://pbs.twimg.com https://*.twimg.com data:; media-src https://video.twimg.com; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedOrigin(origin, requestHost string) bool {
	if strings.HasPrefix(origin, "chrome-extension://") || strings.HasPrefix(origin, "extension://") {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "http" {
		return false
	}
	return parsed.Host == requestHost && (parsed.Hostname() == "127.0.0.1" || strings.EqualFold(parsed.Hostname(), "localhost"))
}

func (s *Server) requireExtension(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(auth, "Bearer ") || !s.store.AuthenticateExtension(r.Context(), strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))) {
			writeError(w, http.StatusUnauthorized, "invalid_extension_token", "Pair the extension with this application")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": buildinfo.Version})
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var bookmarks, pending int
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM bookmarks`).Scan(&bookmarks)
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM jobs WHERE status IN ('pending','processing','blocked')`).Scan(&pending)
	_, key := s.readyProvider(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"bookmarks": bookmarks, "pendingJobs": pending, "aiConfigured": key != ""})
}

func randomDigits(length int) (string, error) {
	max := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(length)), nil)
	value, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", length, value.Int64()), nil
}

func randomToken(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func (s *Server) createPairingCode(w http.ResponseWriter, r *http.Request) {
	code, err := randomDigits(6)
	if err != nil {
		writeError(w, 500, "pairing_failed", "Could not create a pairing code")
		return
	}
	expires := time.Now().UTC().Add(5 * time.Minute)
	if err := s.store.CreatePairingCode(r.Context(), code, expires); err != nil {
		writeError(w, 500, "pairing_failed", "Could not create a pairing code")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"code": code, "expiresAt": expires.Format(time.RFC3339)})
}

func (s *Server) pairExtension(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &input, 2048); err != nil {
		writeError(w, 400, "invalid_json", err.Error())
		return
	}
	token, err := randomToken(32)
	if err != nil {
		writeError(w, 500, "pairing_failed", "Could not pair the extension")
		return
	}
	if err := s.store.PairExtension(r.Context(), strings.TrimSpace(input.Code), token); err != nil {
		writeError(w, 401, "invalid_pairing_code", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"token": token})
}

func (s *Server) unpairExtension(w http.ResponseWriter, r *http.Request) {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if err := s.store.RevokeExtension(r.Context(), token); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_extension_token", "This extension is already disconnected")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) importBookmarks(w http.ResponseWriter, r *http.Request) {
	var batch model.ImportBatch
	if err := decodeJSON(w, r, &batch, 5<<20); err != nil {
		writeError(w, 400, "invalid_import", err.Error())
		return
	}
	result, err := s.store.Import(r.Context(), batch)
	if err != nil {
		writeError(w, 422, "invalid_import", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) listBookmarks(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	query := model.BookmarkQuery{Text: strings.TrimSpace(r.URL.Query().Get("q")), ProcessingState: r.URL.Query().Get("status"), Sort: r.URL.Query().Get("sort"), IncludeArchived: r.URL.Query().Get("archived") == "true", Page: page, PageSize: pageSize}
	query.CategoryIDs = parseIDs(r.URL.Query()["category"])
	query.TagIDs = parseIDs(r.URL.Query()["tag"])
	if r.URL.Query().Get("list") == "none" {
		query.NoList = true
	} else if ids := parseIDs(r.URL.Query()["list"]); len(ids) > 0 {
		query.ListID = ids[0]
	}
	result, err := s.store.SearchBookmarks(r.Context(), query)
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load bookmarks")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseIDs(values []string) []int64 {
	var result []int64
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if id, err := strconv.ParseInt(part, 10, 64); err == nil && id > 0 {
				result = append(result, id)
			}
		}
	}
	return result
}

func (s *Server) getBookmark(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, 400, "invalid_id", "Bookmark ID must be a number")
		return
	}
	item, err := s.store.GetBookmark(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Bookmark not found")
		return
	}
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load the bookmark")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateBookmark(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, 400, "invalid_id", "Bookmark ID must be a number")
		return
	}
	var input struct {
		Archived          *bool                      `json:"archived"`
		Summary           *string                    `json:"summary,omitempty"`
		ResetSummary      bool                       `json:"resetSummary,omitempty"`
		CategoryDecisions []model.AssignmentDecision `json:"categoryDecisions,omitempty"`
		TagDecisions      []model.AssignmentDecision `json:"tagDecisions,omitempty"`
	}
	if err := decodeJSON(w, r, &input, 64<<10); err != nil {
		writeError(w, 400, "invalid_update", err.Error())
		return
	}
	var item model.Bookmark
	if input.Archived != nil {
		item, err = s.store.SetArchived(r.Context(), id, *input.Archived)
	} else {
		item, err = s.store.UpdateBookmarkMetadata(r.Context(), id, model.BookmarkMetadataUpdate{Summary: input.Summary, ResetSummary: input.ResetSummary, CategoryDecisions: input.CategoryDecisions, TagDecisions: input.TagDecisions})
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Bookmark not found")
		return
	}
	if err != nil {
		writeError(w, 500, "storage_error", "Could not update bookmark")
		return
	}
	writeJSON(w, 200, item)
}

func (s *Server) deleteBookmark(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, 400, "invalid_id", "Bookmark ID must be a number")
		return
	}
	if r.URL.Query().Get("confirm") != "true" {
		writeError(w, 400, "confirmation_required", "Pass confirm=true to permanently delete this bookmark")
		return
	}
	err = s.store.DeleteBookmark(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Bookmark not found")
		return
	}
	if err != nil {
		writeError(w, 500, "storage_error", "Could not delete bookmark")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listCategories(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListCategories(r.Context())
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load categories")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListTags(r.Context())
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load tags")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func parseEntityID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, 400, "invalid_id", "ID must be a positive number")
		return 0, false
	}
	return id, true
}

type categoryInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ParentID    *int64 `json:"parentId"`
	Active      *bool  `json:"active,omitempty"`
}

func (s *Server) createCategory(w http.ResponseWriter, r *http.Request) {
	var input categoryInput
	if err := decodeJSON(w, r, &input, 8192); err != nil {
		writeError(w, 400, "invalid_category", err.Error())
		return
	}
	item, err := s.store.CreateCategory(r.Context(), input.Name, input.Description, input.ParentID)
	if err != nil {
		writeError(w, 422, "invalid_category", err.Error())
		return
	}
	writeJSON(w, 201, item)
}
func (s *Server) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	var input categoryInput
	if err := decodeJSON(w, r, &input, 8192); err != nil {
		writeError(w, 400, "invalid_category", err.Error())
		return
	}
	item, err := s.store.UpdateCategory(r.Context(), id, input.Name, input.Description, input.ParentID, input.Active)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Category not found")
		return
	}
	if err != nil {
		writeError(w, 422, "invalid_category", err.Error())
		return
	}
	writeJSON(w, 200, item)
}
func (s *Server) deactivateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	item, err := s.store.GetCategory(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Category not found")
		return
	}
	active := false
	item, err = s.store.UpdateCategory(r.Context(), id, item.Name, item.Description, item.ParentID, &active)
	if err != nil {
		writeError(w, 422, "invalid_category", err.Error())
		return
	}
	writeJSON(w, 200, item)
}
func (s *Server) mergeCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	var input struct {
		TargetID int64 `json:"targetId"`
	}
	if err := decodeJSON(w, r, &input, 2048); err != nil {
		writeError(w, 400, "invalid_merge", err.Error())
		return
	}
	if err := s.store.MergeCategory(r.Context(), id, input.TargetID); err != nil {
		writeError(w, 422, "invalid_merge", err.Error())
		return
	}
	w.WriteHeader(204)
}

type tagInput struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

func (s *Server) createTag(w http.ResponseWriter, r *http.Request) {
	var input tagInput
	if err := decodeJSON(w, r, &input, 4096); err != nil {
		writeError(w, 400, "invalid_tag", err.Error())
		return
	}
	item, err := s.store.CreateTag(r.Context(), input.Name, input.Kind)
	if err != nil {
		writeError(w, 422, "invalid_tag", err.Error())
		return
	}
	writeJSON(w, 201, item)
}
func (s *Server) updateTag(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	var input tagInput
	if err := decodeJSON(w, r, &input, 4096); err != nil {
		writeError(w, 400, "invalid_tag", err.Error())
		return
	}
	item, err := s.store.UpdateTag(r.Context(), id, input.Name, input.Kind)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Tag not found")
		return
	}
	if err != nil {
		writeError(w, 422, "invalid_tag", err.Error())
		return
	}
	writeJSON(w, 200, item)
}
func (s *Server) deleteTag(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteUnusedTag(r.Context(), id); err != nil {
		writeError(w, 409, "tag_in_use", err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) addTagAlias(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	var input struct {
		Alias string `json:"alias"`
	}
	if err := decodeJSON(w, r, &input, 4096); err != nil {
		writeError(w, 400, "invalid_alias", err.Error())
		return
	}
	item, err := s.store.AddTagAlias(r.Context(), id, input.Alias)
	if err != nil {
		writeError(w, 422, "invalid_alias", err.Error())
		return
	}
	writeJSON(w, 201, item)
}
func (s *Server) deleteTagAlias(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	item, err := s.store.DeleteTagAlias(r.Context(), id, r.URL.Query().Get("alias"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Alias not found")
		return
	}
	if err != nil {
		writeError(w, 422, "invalid_alias", err.Error())
		return
	}
	writeJSON(w, 200, item)
}
func (s *Server) mergeTag(w http.ResponseWriter, r *http.Request) {
	id, ok := parseEntityID(w, r)
	if !ok {
		return
	}
	var input struct {
		TargetID int64 `json:"targetId"`
	}
	if err := decodeJSON(w, r, &input, 2048); err != nil {
		writeError(w, 400, "invalid_merge", err.Error())
		return
	}
	if err := s.store.MergeTag(r.Context(), id, input.TargetID); err != nil {
		writeError(w, 422, "invalid_merge", err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Dashboard(r.Context())
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load dashboard statistics")
		return
	}
	writeJSON(w, 200, stats)
}

func (s *Server) exportJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="sortwise-export.json"`)
	if err := s.store.ExportJSON(r.Context(), w); err != nil {
		slog.Error("JSON export failed", "error", err)
	}
}
func (s *Server) exportCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="sortwise-bookmarks.csv"`)
	if err := s.store.ExportCSV(r.Context(), w); err != nil {
		slog.Error("CSV export failed", "error", err)
	}
}
func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	name, _, err := s.store.CreateBackup(r.Context())
	if err != nil {
		writeError(w, 500, "backup_failed", "Could not create a consistent database backup")
		return
	}
	writeJSON(w, 201, map[string]any{"name": name, "downloadUrl": "/api/v1/backups/" + url.PathEscape(name)})
}
func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	path, err := s.store.BackupPath(r.Context(), name)
	if err != nil {
		writeError(w, 404, "not_found", "Backup not found")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	http.ServeFile(w, r, path)
}

type jobResponse struct {
	ID          int64  `json:"id"`
	BookmarkID  int64  `json:"bookmarkId"`
	PostID      string `json:"postId"`
	Author      string `json:"author"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	MaxAttempts int    `json:"maxAttempts"`
	LastError   string `json:"lastError"`
	UpdatedAt   string `json:"updatedAt"`
}

const latestJobCondition = `j.id=(SELECT MAX(latest.id) FROM jobs latest WHERE latest.bookmark_id=j.bookmark_id)`

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	// status accepts a comma-separated list, e.g. "pending,processing".
	// Reprocessing adds a new job for the same bookmark; only each bookmark's latest
	// job is shown and counted, so totals match the number of bookmarks.
	query := `SELECT j.id,j.bookmark_id,b.post_id,b.author,j.status,j.attempts,j.max_attempts,j.last_error,j.updated_at FROM jobs j JOIN bookmarks b ON b.id=j.bookmark_id WHERE ` + latestJobCondition
	args := []any{}
	statuses := []string{}
	for _, value := range strings.Split(r.URL.Query().Get("status"), ",") {
		if value = strings.TrimSpace(value); value != "" {
			statuses = append(statuses, value)
			args = append(args, value)
		}
	}
	if len(statuses) > 0 {
		query += ` AND j.status IN (` + strings.TrimSuffix(strings.Repeat("?,", len(statuses)), ",") + `)`
	}
	query += ` ORDER BY j.updated_at DESC,j.id DESC LIMIT 200`
	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, 500, "storage_error", "Could not load jobs")
		return
	}
	defer rows.Close()
	items := []jobResponse{}
	for rows.Next() {
		var item jobResponse
		if err := rows.Scan(&item.ID, &item.BookmarkID, &item.PostID, &item.Author, &item.Status, &item.Attempts, &item.MaxAttempts, &item.LastError, &item.UpdatedAt); err != nil {
			writeError(w, 500, "storage_error", "Could not load jobs")
			return
		}
		items = append(items, item)
	}
	// The list is capped at 200; counts cover every job so the page can show true totals.
	counts := map[string]int{"pending": 0, "processing": 0, "completed": 0, "failed": 0, "blocked": 0}
	countRows, err := s.db.QueryContext(r.Context(), `SELECT j.status,COUNT(*) FROM jobs j WHERE `+latestJobCondition+` GROUP BY j.status`)
	if err != nil {
		writeError(w, 500, "storage_error", "Could not count jobs")
		return
	}
	defer countRows.Close()
	total := 0
	for countRows.Next() {
		var status string
		var count int
		if err := countRows.Scan(&status, &count); err != nil {
			writeError(w, 500, "storage_error", "Could not count jobs")
			return
		}
		counts[status] = count
		total += count
	}
	counts["total"] = total
	writeJSON(w, 200, map[string]any{"items": items, "counts": counts})
}

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request) {
	if s.worker == nil {
		writeError(w, 503, "worker_unavailable", "The AI worker is unavailable")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, 400, "invalid_id", "Job ID must be a number")
		return
	}
	if err = s.worker.RetryFailed(r.Context(), id); errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Failed job not found")
		return
	} else if err != nil {
		writeError(w, 500, "storage_error", "Could not retry the job")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reprocessBookmark(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, 400, "invalid_id", "Bookmark ID must be a number")
		return
	}
	var contentHash string
	err = s.db.QueryRowContext(r.Context(), `SELECT content_hash FROM bookmarks WHERE id=?`, id).Scan(&contentHash)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "Bookmark not found")
		return
	}
	inputHash := fmt.Sprintf("%s:manual:%d", contentHash, time.Now().UnixNano())
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO jobs(bookmark_id,input_hash) VALUES (?,?)`, id, inputHash)
	if err == nil {
		_, err = s.db.ExecContext(r.Context(), `UPDATE bookmarks SET processing_status='pending' WHERE id=?`, id)
	}
	if err != nil {
		writeError(w, 500, "storage_error", "Could not queue reprocessing")
		return
	}
	writeJSON(w, 202, map[string]any{"status": "pending"})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any, maxBytes int64) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return errors.New("Content-Type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON value")
	}
	return nil
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, 404, "not_found", "API route not found")
		return
	}
	body, err := webassets.Assets.ReadFile("dist/index.html")
	if err != nil {
		http.Error(w, "Web application has not been built", 503)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
