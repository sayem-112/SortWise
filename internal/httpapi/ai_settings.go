package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"sortwise/internal/ai"
)

// providerInfo describes one AI provider for the Settings page. Model names
// are shown only under Advanced; people choose a service, not a model.
type providerInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	Source     string `json:"source"`
	Model      string `json:"model"`
	KeyURL     string `json:"keyUrl"`
}

type aiSettingsResponse struct {
	Provider  string              `json:"provider"`
	Backup    bool                `json:"backup"`
	Paused    bool                `json:"paused"`
	Language  string              `json:"language"`
	Providers []providerInfo      `json:"providers"`
	Active    string              `json:"active"`
	Resting   []ai.ProviderStatus `json:"resting"`
}

var providerKeyURLs = map[string]string{
	"gemini": "https://aistudio.google.com/apikey",
	"groq":   "https://console.groq.com/keys",
}

func (s *Server) aiConfig(ctx context.Context) ai.Configuration {
	return ai.ReadConfiguration(ctx, s.db, func(name string) bool { return s.providers[name] != nil })
}

// readyProvider returns the provider that would organize right now: the main
// one when it has a key, otherwise the backup when backup is on.
func (s *Server) readyProvider(ctx context.Context) (string, string) {
	config := s.aiConfig(ctx)
	names := []string{config.Provider}
	if config.Backup {
		for _, name := range []string{"gemini", "groq"} {
			if name != config.Provider {
				names = append(names, name)
			}
		}
	}
	for _, name := range names {
		if key, _, err := s.credentials.Get(name); err == nil && key != "" && s.providers[name] != nil {
			return name, key
		}
	}
	return config.Provider, ""
}

func (s *Server) aiSettingsResponse(ctx context.Context) (aiSettingsResponse, error) {
	config := s.aiConfig(ctx)
	response := aiSettingsResponse{Provider: config.Provider, Backup: config.Backup, Paused: config.Paused, Language: ai.ReadLanguage(ctx, s.db), Resting: []ai.ProviderStatus{}}
	for _, id := range []string{"gemini", "groq"} {
		key, source, err := s.credentials.Get(id)
		if err != nil {
			return response, err
		}
		response.Providers = append(response.Providers, providerInfo{ID: id, Name: ai.ProviderName(id), Configured: key != "", Source: source, Model: ai.ModelFor(id), KeyURL: providerKeyURLs[id]})
	}
	if active, key := s.readyProvider(ctx); key != "" {
		response.Active = active
	}
	if s.worker != nil {
		response.Resting = s.worker.Resting()
	}
	return response, nil
}

func (s *Server) writeAISettings(w http.ResponseWriter, r *http.Request) {
	response, err := s.aiSettingsResponse(r.Context())
	if err != nil {
		writeError(w, 500, "credential_error", "Windows Credential Manager is unavailable")
		return
	}
	writeJSON(w, 200, response)
}

func (s *Server) getAISettings(w http.ResponseWriter, r *http.Request) {
	s.writeAISettings(w, r)
}

func (s *Server) saveSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=CURRENT_TIMESTAMP`, key, value)
	return err
}

// updateAISettings changes the main provider, backup, pause, or AI language.
func (s *Server) updateAISettings(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Provider *string `json:"provider"`
		Backup   *bool   `json:"backup"`
		Paused   *bool   `json:"paused"`
		Language *string `json:"language"`
	}
	if err := decodeJSON(w, r, &input, 4096); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	ctx := r.Context()
	if input.Provider != nil {
		name := strings.ToLower(strings.TrimSpace(*input.Provider))
		if s.providers[name] == nil {
			writeError(w, 400, "invalid_provider", "Choose Gemini or Groq")
			return
		}
		if err := s.saveSetting(ctx, "ai_provider", name); err != nil {
			writeError(w, 500, "storage_error", "Could not save the AI settings")
			return
		}
	}
	if input.Language != nil {
		language := strings.TrimSpace(*input.Language)
		if !ai.SupportedLanguage(language) {
			writeError(w, 400, "invalid_language", "Choose a supported language")
			return
		}
		if err := s.saveSetting(ctx, "ai_language", language); err != nil {
			writeError(w, 500, "storage_error", "Could not save the AI settings")
			return
		}
	}
	for key, value := range map[string]*bool{"ai_backup": input.Backup, "ai_paused": input.Paused} {
		if value == nil {
			continue
		}
		text := "false"
		if *value {
			text = "true"
		}
		if err := s.saveSetting(ctx, key, text); err != nil {
			writeError(w, 500, "storage_error", "Could not save the AI settings")
			return
		}
	}
	if s.worker != nil && (input.Provider != nil || input.Backup != nil || (input.Paused != nil && !*input.Paused)) {
		s.worker.Unblock(ctx)
	}
	s.writeAISettings(w, r)
}

// saveAIKey stores a provider's key in Windows Credential Manager. A key for a
// provider becomes the main one when the current main provider has no key.
func (s *Server) saveAIKey(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(chi.URLParam(r, "provider"))
	var input struct {
		APIKey string `json:"apiKey"`
	}
	if err := decodeJSON(w, r, &input, 4096); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	if s.providers[name] == nil {
		writeError(w, 400, "invalid_provider", "Choose Gemini or Groq")
		return
	}
	key := strings.TrimSpace(input.APIKey)
	if len(key) < 10 {
		writeError(w, 400, "invalid_api_key", "Enter a valid API key")
		return
	}
	if _, source, _ := s.credentials.Get(name); strings.HasSuffix(source, "_API_KEY") {
		writeError(w, 409, "environment_override", "This key comes from an environment variable. Remove it there first.")
		return
	}
	if err := s.credentials.Set(name, key); err != nil {
		writeError(w, 500, "credential_error", "Could not store the key in Windows Credential Manager")
		return
	}
	config := s.aiConfig(r.Context())
	if main, _, _ := s.credentials.Get(config.Provider); main == "" {
		_ = s.saveSetting(r.Context(), "ai_provider", name)
	}
	if s.worker != nil {
		s.worker.Unblock(r.Context())
	}
	s.writeAISettings(w, r)
}

func (s *Server) deleteAIKey(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(chi.URLParam(r, "provider"))
	if s.providers[name] == nil {
		writeError(w, 400, "invalid_provider", "Choose Gemini or Groq")
		return
	}
	if _, source, _ := s.credentials.Get(name); strings.HasSuffix(source, "_API_KEY") {
		writeError(w, 409, "environment_override", "This key comes from an environment variable. Remove it there.")
		return
	}
	if err := s.credentials.Delete(name); err != nil {
		writeError(w, 500, "credential_error", "Could not remove the saved key")
		return
	}
	s.writeAISettings(w, r)
}

// testAI sends one small request to a provider (the given one, or the one that
// would organize right now) to confirm the key and model work.
func (s *Server) testAI(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Provider string `json:"provider"`
	}
	_ = decodeJSON(w, r, &input, 1024)
	name, key := s.readyProvider(r.Context())
	if input.Provider != "" {
		name = strings.ToLower(input.Provider)
		key, _, _ = s.credentials.Get(name)
	}
	factory := s.providers[name]
	if factory == nil || key == "" {
		writeError(w, 400, "ai_unconfigured", "Add a "+ai.ProviderName(name)+" API key first")
		return
	}
	provider, err := factory(r.Context(), key)
	if err != nil {
		writeError(w, 502, "ai_connection_failed", "Could not create the AI client")
		return
	}
	input2 := ai.AnalysisInput{Text: "A short guide to reliable background queues.", Author: "Sortwise", Username: "sortwise", Categories: []ai.CategoryOption{{ID: 1, Name: "Technology"}}}
	if _, err = provider.Analyze(r.Context(), input2); err != nil {
		writeError(w, 502, "ai_connection_failed", ai.ProviderName(name)+" did not accept the test request: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "provider": name})
}
