package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"sortwise/internal/database"
)

func TestHealth(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	r.Host = "127.0.0.1:8787"
	w := httptest.NewRecorder()
	New(db).ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected security header, got %q", got)
	}
}

func TestPairAndImportRequiresToken(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := New(db)
	codeResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/extension/pairing-codes", nil, "")
	if codeResponse.Code != 201 {
		t.Fatalf("pairing code status: %d %s", codeResponse.Code, codeResponse.Body.String())
	}
	var code struct {
		Code string `json:"code"`
	}
	json.Unmarshal(codeResponse.Body.Bytes(), &code)
	pairResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/extension/pair", map[string]string{"code": code.Code}, "")
	var paired struct {
		Token string `json:"token"`
	}
	json.Unmarshal(pairResponse.Body.Bytes(), &paired)
	bookmark := map[string]any{"platform": "x", "postId": "1001", "author": "Ada", "username": "ada", "text": "Queues", "url": "https://x.com/ada/status/1001", "visibleContext": map[string]any{}, "media": []any{}, "extractedAt": "2026-07-18T12:00:00Z", "extractorVersion": "1.0.0"}
	unauthorized := requestJSON(t, handler, http.MethodPost, "/api/v1/imports", map[string]any{"bookmarks": []any{bookmark}}, "")
	if unauthorized.Code != 401 {
		t.Fatalf("expected 401, got %d", unauthorized.Code)
	}
	authorized := requestJSON(t, handler, http.MethodPost, "/api/v1/imports", map[string]any{"bookmarks": []any{bookmark}}, paired.Token)
	if authorized.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", authorized.Code, authorized.Body.String())
	}
	disconnected := requestJSON(t, handler, http.MethodDelete, "/api/v1/extension/pairing", nil, paired.Token)
	if disconnected.Code != http.StatusNoContent {
		t.Fatalf("expected disconnect to return 204, got %d: %s", disconnected.Code, disconnected.Body.String())
	}
	revoked := requestJSON(t, handler, http.MethodPost, "/api/v1/imports", map[string]any{"bookmarks": []any{bookmark}}, paired.Token)
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked token rejection, got %d", revoked.Code)
	}
}

func TestRejectsUntrustedHostAndOrigin(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := New(db)
	request := httptest.NewRequest(http.MethodGet, "http://malicious.example/api/v1/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected invalid host rejection, got %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8787/api/v1/extension/pairing-codes", bytes.NewBufferString("{}"))
	request.Host = "127.0.0.1:8787"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://malicious.example")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected invalid origin rejection, got %d", response.Code)
	}
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var buffer bytes.Buffer
	if body != nil {
		json.NewEncoder(&buffer).Encode(body)
	}
	r := httptest.NewRequest(method, path, &buffer)
	r.Host = "127.0.0.1:8787"
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestExtensionOriginIsLimitedToImportEndpoints(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "origin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := New(db, Dependencies{})
	for path, want := range map[string]int{
		"/api/v1/bookmarks":               http.StatusForbidden,
		"/api/v1/settings/ai":             http.StatusForbidden,
		"/api/v1/extension/pairing-codes": http.StatusForbidden,
		"/api/v1/health":                  http.StatusOK,
	} {
		method := http.MethodGet
		if strings.HasSuffix(path, "pairing-codes") {
			method = http.MethodPost
		}
		req := httptest.NewRequest(method, "http://127.0.0.1:8787"+path, strings.NewReader("{}"))
		req.Header.Set("Origin", "chrome-extension://someotherextension")
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != want {
			t.Errorf("%s from an extension: got %d, want %d", path, recorder.Code, want)
		}
	}
}

func TestLogoIsServed(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "logo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := httptest.NewRequest(http.MethodGet, "/logo.svg", nil)
	r.Host = "127.0.0.1:8787"
	w := httptest.NewRecorder()
	New(db).ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "image/svg+xml") {
		t.Fatalf("logo: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
}
