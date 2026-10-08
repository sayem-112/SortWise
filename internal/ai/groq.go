package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

var groqChatCompletionsURL = "https://api.groq.com/openai/v1/chat/completions"

type GroqProvider struct {
	apiKey      string
	model       string
	httpClient  *http.Client
	mediaClient *http.Client
}

func NewGroq(_ context.Context, key string) (Provider, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("Groq API key is empty")
	}
	return &GroqProvider{apiKey: strings.TrimSpace(key), model: ModelFor("groq"), httpClient: &http.Client{Timeout: 90 * time.Second}, mediaClient: newMediaHTTPClient(20 * time.Second)}, nil
}

type groqContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

func (g *GroqProvider) Analyze(ctx context.Context, input AnalysisInput) (AnalysisResult, error) {
	var images []groqContentPart
	attached := []MediaInput{}
	for _, media := range input.Media {
		if len(images) >= groqMaxImages {
			break
		}
		data, mime, err := fetchImage(ctx, g.mediaClient, media)
		if err != nil {
			continue
		}
		attached = append(attached, media)
		imageURL := &struct {
			URL string `json:"url"`
		}{URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)}
		images = append(images, groqContentPart{Type: "image_url", ImageURL: imageURL})
	}
	// Keep the whole request under Groq's per-minute input budget: a request larger
	// than the limit is rejected outright (413) and can never succeed. Drop images
	// from the end until the estimate fits.
	for len(images) > 0 && groqEstimateTokens(input, attached) > groqRequestBudget {
		images = images[:len(images)-1]
		attached = attached[:len(attached)-1]
	}
	content, err := g.analyzeWith(ctx, input, attached, images)
	if err != nil && len(images) > 0 && strings.Contains(err.Error(), "413") {
		// The estimate was optimistic; the text alone always fits.
		content, err = g.analyzeWith(ctx, input, nil, nil)
	}
	if err != nil {
		return AnalysisResult{}, err
	}
	var result AnalysisResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return result, fmt.Errorf("decode structured Groq response: %w", err)
	}
	if err := ValidateResult(input, &result); err != nil {
		return result, err
	}
	return result, nil
}

const (
	// groqRequestBudget stays under the free tier's 7,000 input tokens per minute.
	groqRequestBudget = 6200
	groqImageTokens   = 2048
)

// groqEstimateTokens is a conservative estimate: about three characters per token
// for the prompt plus Groq's fixed cost per image.
func groqEstimateTokens(input AnalysisInput, attached []MediaInput) int {
	input.Media = attached
	return len(analysisPrompt(input))/3 + len(attached)*groqImageTokens + 150
}

func (g *GroqProvider) analyzeWith(ctx context.Context, input AnalysisInput, attached []MediaInput, images []groqContentPart) (string, error) {
	// The prompt describes only the images the model actually receives.
	input.Media = attached
	parts := append([]groqContentPart{{Type: "text", Text: analysisPrompt(input)}}, images...)
	return g.complete(ctx, parts, "bookmark_analysis", analysisSchema(input))
}

// CompleteJSON runs a text-only prompt constrained to a strict JSON schema.
func (g *GroqProvider) CompleteJSON(ctx context.Context, prompt, name string, schema map[string]any) (string, error) {
	return g.complete(ctx, []groqContentPart{{Type: "text", Text: prompt}}, name, schema)
}

func (g *GroqProvider) complete(ctx context.Context, parts []groqContentPart, name string, schema map[string]any) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":    g.model,
		"messages": []any{map[string]any{"role": "user", "content": parts}},
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": name, "strict": true, "schema": schema},
		},
		// Qwen 3.8 thinks by default; JSON mode requires reasoning to be parsed or hidden,
		// and reasoning tokens would otherwise eat the completion budget.
		"reasoning_format":      "hidden",
		"reasoning_effort":      "none",
		"temperature":           0.2,
		"max_completion_tokens": 4096,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, groqChatCompletionsURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := g.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		retry := response.Header.Get("Retry-After")
		if retry != "" {
			return "", fmt.Errorf("Groq returned %s; retry in %ss: %s", response.Status, retry, strings.TrimSpace(string(data)))
		}
		return "", fmt.Errorf("Groq returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Choices) == 0 {
		return "", fmt.Errorf("decode Groq response: %w", err)
	}
	content := envelope.Choices[0].Message.Content
	start := strings.IndexByte(content, '{')
	end := strings.LastIndexByte(content, '}')
	if start != -1 && end != -1 && end >= start {
		content = content[start : end+1]
	}
	return content, nil
}

// analysisSchema follows strict structured-output rules (every field required,
// no additional properties). Counts and ranges are enforced by ValidateResult.
func analysisSchema(input AnalysisInput) map[string]any {
	categoryName := map[string]any{"type": "string"}
	if len(input.Categories) > 0 {
		names := make([]string, 0, len(input.Categories))
		for _, category := range input.Categories {
			names = append(names, category.Name)
		}
		categoryName["enum"] = names
	}
	category := strictObject(map[string]any{"name": categoryName, "confidence": map[string]any{"type": "number"}})
	tag := strictObject(map[string]any{
		"name":       map[string]any{"type": "string"},
		"confidence": map[string]any{"type": "number"},
		"kind":       map[string]any{"type": "string", "enum": TagKinds},
	})
	return strictObject(map[string]any{
		"summary":          map[string]any{"type": "string"},
		"mediaDescription": map[string]any{"type": "string"},
		"categories":       map[string]any{"type": "array", "items": category},
		"tags":             map[string]any{"type": "array", "items": tag},
	})
}

// strictObject builds an object schema with every property required and no
// additional properties.
func strictObject(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for name := range properties {
		required = append(required, name)
	}
	sort.Strings(required)
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
