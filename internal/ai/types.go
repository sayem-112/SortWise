package ai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const PromptVersion = "2026-10-v5"
const (
	DefaultProvider = "gemini"
	// Groq retired qwen/qwen3.6-27b on 2026-09-14; qwen3.8-27b is its multimodal successor.
	GroqModel = "qwen/qwen3.8-27b"
	// gemini-3.5-flash is now a legacy model; 3.8 Flash is the current stable Flash.
	GeminiModel = "gemini-3.8-flash"
	// GeminiFallbackModel answers when the main model is overloaded (503); it has
	// separate capacity and is cheaper.
	GeminiFallbackModel = "gemini-3.5-flash-lite"
	DefaultModel        = GeminiModel
	// Groq accepts at most three images per request.
	groqMaxImages   = 3
	geminiMaxImages = 4
)

// ModelFor returns the model for a provider. SORTWISE_GROQ_MODEL and
// SORTWISE_GEMINI_MODEL override the defaults so a provider retiring a
// model does not require a new release.
func ModelFor(provider string) string {
	if strings.EqualFold(provider, "groq") {
		if value := env("GROQ_MODEL"); value != "" {
			return value
		}
		return GroqModel
	}
	if value := env("GEMINI_MODEL"); value != "" {
		return value
	}
	return GeminiModel
}

// env reads the SORTWISE_<name> environment variable.
func env(name string) string {
	return strings.TrimSpace(os.Getenv("SORTWISE_" + name))
}

// GeminiFallback returns the overload fallback model, overridable with
// SORTWISE_GEMINI_FALLBACK_MODEL ("none" disables it).
func GeminiFallback() string {
	value := env("GEMINI_FALLBACK_MODEL")
	if strings.EqualFold(value, "none") {
		return ""
	}
	if value != "" {
		return value
	}
	return GeminiFallbackModel
}

func ProviderName(provider string) string {
	if strings.EqualFold(provider, "groq") {
		return "Groq"
	}
	return "Gemini"
}

func DefaultFactories() map[string]Factory {
	return map[string]Factory{"gemini": NewGemini, "groq": NewGroq}
}

type CategoryOption struct {
	ID   int64
	Name string
}
type MediaInput struct{ Kind, URL, AltText string }

type AnalysisInput struct {
	BookmarkID      int64
	InputHash       string
	Text            string
	Author          string
	Username        string
	QuotedText      string
	CardTitle       string
	CardDescription string
	Media           []MediaInput
	Categories      []CategoryOption
	// ExistingTags is the library's current vocabulary, most used first, so the
	// model reuses tags instead of inventing near-duplicates.
	ExistingTags []string
	// Language is the language summaries and topic tags are written in.
	Language string
}

type Classification struct {
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
	// Kind is set for tags only: topic, tool, entity, or format.
	Kind string `json:"kind,omitempty"`
}

var TagKinds = []string{"topic", "tool", "entity", "format"}

// genericTags are too broad to help re-find anything in any field; categories
// already cover them. Kept to words that are broad for everyone, not only for
// software engineers.
var genericTags = map[string]bool{
	// Whole fields
	"tech": true, "technology": true, "software": true, "software-engineering": true, "programming": true,
	"coding": true, "code": true, "engineering": true, "development": true, "developer": true, "ai": true,
	"business": true, "science": true, "design": true, "art": true, "music": true, "sports": true, "sport": true,
	"health": true, "fitness": true, "finance": true, "money": true, "economy": true, "politics": true,
	"history": true, "education": true, "learning": true, "food": true, "travel": true, "fashion": true,
	"culture": true, "entertainment": true, "gaming": true, "games": true, "lifestyle": true, "life": true,
	"career": true, "work": true, "productivity": true, "self-improvement": true, "personal-development": true,
	// Words that describe every post
	"general": true, "misc": true, "miscellaneous": true, "other": true, "news": true, "twitter": true, "x": true,
	"tweet": true, "post": true, "social-media": true, "internet": true, "interesting": true, "tips": true,
	"advice": true, "inspiration": true, "motivation": true, "funny": true, "humor": true, "cool": true,
	"useful": true, "important": true, "thoughts": true, "update": true, "content": true, "information": true,
}

type AnalysisResult struct {
	Summary          string           `json:"summary"`
	MediaDescription string           `json:"mediaDescription"`
	Categories       []Classification `json:"categories"`
	Tags             []Classification `json:"tags"`
	// Model records which model produced the result when a provider fell back.
	Model string `json:"-"`
}

type Provider interface {
	Analyze(context.Context, AnalysisInput) (AnalysisResult, error)
}

type Factory func(context.Context, string) (Provider, error)

var ErrInvalidResult = errors.New("AI returned an invalid analysis")

const maxTags = 8

// ValidateResult repairs recoverable model mistakes instead of failing the whole
// bookmark: unknown or duplicate categories are dropped, extra categories and tags
// are trimmed by confidence, and confidences are clamped. Only an empty summary or
// no usable tags is treated as an invalid analysis.
func ValidateResult(input AnalysisInput, result *AnalysisResult) error {
	result.Summary = strings.TrimSpace(result.Summary)
	result.MediaDescription = strings.TrimSpace(result.MediaDescription)
	if result.Summary == "" {
		return fmt.Errorf("%w: summary is empty", ErrInvalidResult)
	}
	if runes := []rune(result.Summary); len(runes) > 800 {
		result.Summary = strings.TrimSpace(string(runes[:797])) + "…"
	}
	allowed := map[string]string{}
	for _, category := range input.Categories {
		allowed[strings.ToLower(strings.TrimSpace(category.Name))] = category.Name
	}
	categories := make([]Classification, 0, len(result.Categories))
	seenCategories := map[string]bool{}
	for _, item := range result.Categories {
		key := strings.ToLower(strings.TrimSpace(item.Name))
		name, ok := allowed[key]
		if !ok || seenCategories[key] {
			continue
		}
		seenCategories[key] = true
		categories = append(categories, Classification{Name: name, Confidence: clampConfidence(item.Confidence)})
	}
	sort.SliceStable(categories, func(i, j int) bool { return categories[i].Confidence > categories[j].Confidence })
	if len(categories) > 2 {
		categories = categories[:2]
	}
	result.Categories = categories

	categoryKeys := map[string]bool{}
	for _, category := range input.Categories {
		categoryKeys[NormalizeTag(category.Name)] = true
	}
	tags := make([]Classification, 0, len(result.Tags))
	generic := []Classification{}
	seenTags := map[string]bool{}
	for _, item := range result.Tags {
		kind := normalizeKind(item.Kind)
		name := displayTagName(item.Name, kind)
		key := NormalizeTag(name)
		if key == "" || seenTags[key] {
			continue
		}
		seenTags[key] = true
		tag := Classification{Name: name, Confidence: clampConfidence(item.Confidence), Kind: kind}
		if genericTags[key] || categoryKeys[key] {
			generic = append(generic, tag)
			continue
		}
		tags = append(tags, tag)
	}
	// Keep a broad tag only when the model offered nothing more specific.
	if len(tags) == 0 && len(generic) > 0 {
		tags = generic[:1]
	}
	sort.SliceStable(tags, func(i, j int) bool { return tags[i].Confidence > tags[j].Confidence })
	if len(tags) > maxTags {
		tags = tags[:maxTags]
	}
	if len(tags) == 0 {
		return fmt.Errorf("%w: no usable tags", ErrInvalidResult)
	}
	result.Tags = tags
	return nil
}

func normalizeKind(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, kind := range TagKinds {
		if value == kind {
			return kind
		}
	}
	return "topic"
}

// displayTagName keeps tool and entity names as written (Next.js, OpenAI) and
// writes topics and formats as lowercase words, so "Video-Editing" and
// "video editing" display the same way.
func displayTagName(value, kind string) string {
	value = strings.Trim(strings.TrimSpace(value), "#")
	if kind == "tool" || kind == "entity" {
		return strings.Join(strings.Fields(value), " ")
	}
	value = strings.NewReplacer("-", " ", "_", " ").Replace(strings.ToLower(value))
	return strings.Join(strings.Fields(value), " ")
}

// TagVariants returns normalized spellings that should resolve to the same tag
// (singular and plural), most canonical first.
func TagVariants(normalized string) []string {
	variants := []string{normalized}
	switch {
	case strings.HasSuffix(normalized, "ies") && len(normalized) > 4:
		variants = append(variants, strings.TrimSuffix(normalized, "ies")+"y")
	case strings.HasSuffix(normalized, "s") && !strings.HasSuffix(normalized, "ss") && len(normalized) > 3:
		variants = append(variants, strings.TrimSuffix(normalized, "s"))
	default:
		variants = append(variants, normalized+"s")
	}
	return variants
}

func clampConfidence(value float64) float64 {
	if value < 0 || value != value {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

// NormalizeTag gives the key tags are matched by. It keeps letters and digits
// in every script, the same way the store normalizes names typed by hand, so
// tags in Japanese or with accents work.
func NormalizeTag(value string) string {
	value = strings.ToLower(strings.TrimSpace(norm.NFKC.String(value)))
	var output []rune
	previousSeparator := false
	for _, char := range value {
		letter := unicode.IsLetter(char) || unicode.IsDigit(char) || unicode.Is(unicode.Mn, char)
		if letter {
			output = append(output, char)
			previousSeparator = false
			continue
		}
		if !previousSeparator && len(output) > 0 {
			output = append(output, '-')
			previousSeparator = true
		}
	}
	return strings.Trim(string(output), "-")
}
