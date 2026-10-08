package ai

import (
	"fmt"
	"strings"
)

const maxVocabularyTags = 200

func analysisPrompt(input AnalysisInput) string {
	names := make([]string, 0, len(input.Categories))
	for _, category := range input.Categories {
		names = append(names, category.Name)
	}
	vocabulary := input.ExistingTags
	if len(vocabulary) > maxVocabularyTags {
		vocabulary = vocabulary[:maxVocabularyTags]
	}
	language := input.Language
	if language == "" {
		language = DefaultLanguage
	}
	existing := "(none yet)"
	if len(vocabulary) > 0 {
		existing = strings.Join(vocabulary, ", ")
	}
	return fmt.Sprintf(`Analyze this saved X post for a personal bookmark library.
Return only one valid JSON object with keys summary, mediaDescription, categories, and tags. Do not use markdown formatting or json blocks.

The library is written in %[1]s. The post may be in any language; understand it, then write in %[1]s.

Summary: write a factual one- or two-sentence summary in %[1]s. Do not claim details that are not visible.
mediaDescription: describe meaningful attached images in one sentence in %[1]s, or use an empty string when no image is attached. A video poster is a single still frame; describe only what the frame shows and never guess what happens in the video.

Categories: choose zero to two categories, only from the available categories below. Pick the most specific category that fits; choose none rather than a poor fit.

Tags: give three to six tags that would help someone find this exact post again. The library can hold posts from any field (software, medicine, cooking, law, sports, art, finance, parenting, and so on); tag each post in the vocabulary of its own field.
- Reuse an existing tag whenever one fits the same concept, even if the wording differs. Create a new tag only for a concept the existing tags do not cover.
- Be specific: name the concrete subject, the way an expert in that field would. For example "postgres" or "system design" rather than "programming"; "sourdough" or "fermentation" rather than "food"; "strength training" or "zone 2 cardio" rather than "fitness"; "index funds" rather than "finance"; "watercolor" rather than "art". Never use a whole field ("tech", "health", "business", "science", "ai") as a tag.
- Never repeat a category name as a tag.
- Give each tag a kind:
  topic: a subject or concept, in lowercase singular words separated by spaces, written in %[1]s (for example "vector database", "sleep", "pricing strategy").
  tool: a product, library, language, or service, written with its official name (for example "PostgreSQL", "Figma", "Kubernetes").
  entity: a person, company, or organization, written with its proper name (for example "OpenAI", "Paul Graham").
  format: what kind of post it is, exactly one of these English words: "tutorial", "thread", "opinion", "announcement", "resource list", "case study", "research paper", "job post", "question", "meme", "news", "recipe", "review".
- Add at most one format tag, and only when the post clearly is that kind of post. Use "opinion" only for an argued take on a debatable question; ordinary remarks, reactions, and personal updates get no format tag.
- Avoid vague life-advice topics such as "ambition", "career growth", "motivation", or "success" unless the post is specifically about that subject; name the concrete subject instead.
Each category and tag is an object with name and confidence (0 to 1); tags also include kind.

Author: %[2]s (@%[3]s)
Post: %[4]s
Quoted post: %[5]s
Link card: %[6]s — %[7]s
Attached media: %[8]s
Available categories: %[9]s
Existing tags (most used first): %[10]s`, language, input.Author, input.Username, input.Text, input.QuotedText, input.CardTitle, input.CardDescription, describeMedia(input.Media), strings.Join(names, ", "), existing)
}

func describeMedia(media []MediaInput) string {
	if len(media) == 0 {
		return "none"
	}
	values := []string{}
	for index, item := range media {
		label := fmt.Sprintf("image %d", index+1)
		if item.Kind == "video_poster" {
			label = fmt.Sprintf("image %d is the poster frame of a video (the video itself is not available)", index+1)
		}
		if text := strings.TrimSpace(item.AltText); text != "" {
			label += fmt.Sprintf(", alt text: %q", text)
		}
		values = append(values, label)
	}
	return strings.Join(values, "; ")
}
