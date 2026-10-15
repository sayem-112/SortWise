package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"
)

const (
	// GeminiEmbeddingModel turns text into a vector describing its meaning.
	GeminiEmbeddingModel = "gemini-embedding-001"
	// EmbeddingDimensions is the vector size asked for. The model's full
	// 3072 adds little for short posts and would take four times the room.
	EmbeddingDimensions = 768
	// MaxEmbeddingBatch is how many texts one request may carry.
	MaxEmbeddingBatch = 100
)

// EmbeddingModel returns the model vectors are made with.
// SORTWISE_EMBEDDING_MODEL overrides it; vectors from another model are
// made again.
func EmbeddingModel() string {
	if value := env("EMBEDDING_MODEL"); value != "" {
		return value
	}
	return GeminiEmbeddingModel
}

// Embedder turns texts into vectors, one per text and in the same order.
// query is true for words being searched for, false for a post's content;
// the model places the two so that a question lands near its answers.
type Embedder interface {
	Embed(ctx context.Context, texts []string, query bool) ([][]float32, error)
}

type EmbedderFactory func(context.Context, string) (Embedder, error)

type geminiEmbedder struct {
	client *genai.Client
	model  string
}

func NewGeminiEmbedder(ctx context.Context, key string) (Embedder, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: key, Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, err
	}
	return &geminiEmbedder{client: client, model: EmbeddingModel()}, nil
}

func (g *geminiEmbedder) Embed(ctx context.Context, texts []string, query bool) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if len(texts) > MaxEmbeddingBatch {
		return nil, fmt.Errorf("at most %d texts per request", MaxEmbeddingBatch)
	}
	contents := make([]*genai.Content, len(texts))
	for index, text := range texts {
		contents[index] = genai.NewContentFromText(text, genai.RoleUser)
	}
	task := "RETRIEVAL_DOCUMENT"
	if query {
		task = "RETRIEVAL_QUERY"
	}
	dimensions := int32(EmbeddingDimensions)
	response, err := g.client.Models.EmbedContent(ctx, g.model, contents, &genai.EmbedContentConfig{TaskType: task, OutputDimensionality: &dimensions})
	if err != nil {
		return nil, err
	}
	if len(response.Embeddings) != len(texts) {
		return nil, fmt.Errorf("asked for %d vectors, got %d", len(texts), len(response.Embeddings))
	}
	vectors := make([][]float32, len(texts))
	for index, embedding := range response.Embeddings {
		if embedding == nil || len(embedding.Values) == 0 {
			return nil, errors.New("the AI service returned an empty vector")
		}
		vectors[index] = embedding.Values
	}
	return vectors, nil
}

// EmbeddingRest says how long to stop asking for vectors after a failed
// request: until the daily quota resets, for as long as a rate limit asks,
// ten minutes for a bad key or retired model, and for an outage 30 seconds
// that doubles with each failure in a row (streak) up to ten minutes.
func EmbeddingRest(err error, streak int) time.Duration {
	switch {
	case isDailyQuotaError(err):
		return time.Until(NextQuotaReset(time.Now()))
	case isRateLimitError(err):
		return retryDelay(err)
	case isConfigurationError(err):
		return configurationCooldown
	}
	return min(30*time.Second<<min(streak, 5), 10*time.Minute)
}

// EmbeddingRefused reports whether the AI service rejected the text itself,
// as opposed to being busy, unreachable, or unhappy with the key or the
// account (an expired key is also answered with "invalid argument").
func EmbeddingRefused(err error) bool {
	if err == nil || isConfigurationError(err) || isRateLimitError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, about := range []string{"api key", "api_key", "apikey", "billing", "location", "permission"} {
		if strings.Contains(message, about) {
			return false
		}
	}
	return strings.Contains(message, "invalid_argument")
}
