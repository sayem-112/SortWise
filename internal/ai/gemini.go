package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // Windows may lack a zoneinfo database for Pacific time.

	"google.golang.org/genai"
)

type GeminiProvider struct {
	client     *genai.Client
	model      string
	httpClient *http.Client
}

func NewGemini(ctx context.Context, key string) (Provider, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: key, Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, err
	}
	httpClient := newMediaHTTPClient(20 * time.Second)
	return &GeminiProvider{client: client, model: ModelFor("gemini"), httpClient: httpClient}, nil
}

func newMediaHTTPClient(timeout time.Duration) *http.Client {
	httpClient := &http.Client{Timeout: timeout}
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || !allowedMediaURL(req.URL) {
			return http.ErrUseLastResponse
		}
		return nil
	}
	return httpClient
}

func allowedMediaURL(value *url.URL) bool {
	host := strings.ToLower(value.Hostname())
	return value.Scheme == "https" && (host == "pbs.twimg.com" || strings.HasSuffix(host, ".twimg.com"))
}

func fetchImage(ctx context.Context, client *http.Client, input MediaInput) ([]byte, string, error) {
	parsed, err := url.Parse(input.URL)
	if err != nil || !allowedMediaURL(parsed) {
		return nil, "", fmt.Errorf("untrusted media URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return nil, "", err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, "", fmt.Errorf("media returned %s", response.Status)
	}
	mime := strings.ToLower(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" {
		return nil, "", fmt.Errorf("unsupported media type")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 || len(data) >= 10<<20 {
		return nil, "", fmt.Errorf("media exceeds size limit")
	}
	return data, mime, nil
}

func (g *GeminiProvider) Analyze(ctx context.Context, input AnalysisInput) (AnalysisResult, error) {
	var images []*genai.Part
	attached := []MediaInput{}
	for _, media := range input.Media {
		if len(images) >= geminiMaxImages {
			break
		}
		data, mime, err := fetchImage(ctx, g.httpClient, media)
		if err == nil {
			attached = append(attached, media)
			images = append(images, genai.NewPartFromBytes(data, mime))
		}
	}
	// The prompt describes only the images the model actually receives.
	input.Media = attached
	parts := append([]*genai.Part{genai.NewPartFromText(analysisPrompt(input))}, images...)
	text, model, err := g.complete(ctx, parts, analysisSchema(input))
	if err != nil {
		return AnalysisResult{}, err
	}
	result := AnalysisResult{Model: model}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return result, fmt.Errorf("decode structured Gemini response: %w", err)
	}
	if err := ValidateResult(input, &result); err != nil {
		return result, err
	}
	return result, nil
}

// CompleteJSON runs a text-only prompt constrained to a JSON schema.
func (g *GeminiProvider) CompleteJSON(ctx context.Context, prompt, _ string, schema map[string]any) (string, error) {
	text, _, err := g.complete(ctx, []*genai.Part{genai.NewPartFromText(prompt)}, schema)
	return text, err
}

// complete tries the configured model, then the fallback model when the first is
// overloaded or out of daily quota. Models whose daily quota is spent are skipped
// until Google resets it. It returns the model that answered.
func (g *GeminiProvider) complete(ctx context.Context, parts []*genai.Part, schema map[string]any) (string, string, error) {
	models := []string{g.model}
	if fallback := GeminiFallback(); fallback != "" && fallback != g.model {
		models = append(models, fallback)
	}
	var firstErr error
	exhausted := []string{}
	for _, model := range models {
		if dailyQuotaExhausted(model) {
			exhausted = append(exhausted, model)
			continue
		}
		text, err := g.generate(ctx, model, parts, schema)
		if err == nil {
			return text, model, nil
		}
		if isDailyQuotaError(err) {
			markDailyQuotaExhausted(model)
			exhausted = append(exhausted, model)
			continue
		}
		if firstErr == nil {
			firstErr = err
		}
		if !isOverloadError(err) {
			return "", model, err
		}
	}
	if firstErr != nil {
		return "", g.model, firstErr
	}
	return "", g.model, fmt.Errorf("%w: Gemini's free-tier daily request limit is used up for %s. Processing resumes automatically after it resets at midnight Pacific time; enabling billing in Google AI Studio raises the limit", ErrDailyQuota, strings.Join(exhausted, " and "))
}

func (g *GeminiProvider) generate(ctx context.Context, model string, parts []*genai.Part, schema map[string]any) (string, error) {
	temperature := float32(.2)
	response, err := g.client.Models.GenerateContent(ctx, model, []*genai.Content{genai.NewContentFromParts(parts, genai.RoleUser)}, &genai.GenerateContentConfig{
		Temperature:        &temperature,
		MaxOutputTokens:    4096,
		ResponseMIMEType:   "application/json",
		ResponseJsonSchema: schema,
		// Gemini 3 Flash thinks at "medium" by default and thought tokens count against
		// MaxOutputTokens; classification needs little reasoning.
		ThinkingConfig: &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevelLow},
		// Medium resolution costs 560 tokens per image instead of the 1,120 default,
		// which is plenty to recognize what a post's image shows.
		MediaResolution: genai.MediaResolutionMedium,
	})
	if err != nil {
		return "", err
	}
	return response.Text(), nil
}

// ErrDailyQuota means every usable model has spent its daily request quota.
var ErrDailyQuota = errors.New("daily AI quota reached")

var (
	quotaMu        sync.Mutex
	quotaExhausted = map[string]time.Time{}
)

// isDailyQuotaError matches Gemini's per-day quota failure; its "retry in Ns" hint
// refers to the per-minute window and is wrong for a daily limit.
func isDailyQuotaError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "resource_exhausted") && strings.Contains(message, "perday")
}

func dailyQuotaExhausted(model string) bool {
	quotaMu.Lock()
	defer quotaMu.Unlock()
	until, ok := quotaExhausted[model]
	if ok && time.Now().After(until) {
		delete(quotaExhausted, model)
		return false
	}
	return ok
}

func markDailyQuotaExhausted(model string) {
	quotaMu.Lock()
	quotaExhausted[model] = NextQuotaReset(time.Now())
	quotaMu.Unlock()
}

// NextQuotaReset returns the next midnight Pacific time, when Gemini daily quotas reset.
func NextQuotaReset(now time.Time) time.Time {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		location = time.FixedZone("PT", -7*60*60)
	}
	local := now.In(location)
	return time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 30, 0, location)
}
