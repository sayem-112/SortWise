package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sortwise/internal/model"
)

// JSONCompleter is implemented by providers that can answer a text prompt with
// JSON constrained to a schema.
type JSONCompleter interface {
	CompleteJSON(ctx context.Context, prompt, name string, schema map[string]any) (string, error)
}

const (
	maxCategoryProposals = 10
	maxMergeProposals    = 30
	// Merging is hard to undo, so only near-certain synonyms are offered.
	minMergeConfidence = 0.85
)

// ProposeCategories asks the model for sub-categories or new branches that fit
// the library's actual content. Proposals referencing unknown parents or tags
// are dropped; nothing is applied until the user accepts one.
func ProposeCategories(ctx context.Context, completer JSONCompleter, snapshot model.TaxonomySnapshot) ([]model.CategoryProposal, error) {
	var categories, tags strings.Builder
	for _, category := range snapshot.Categories {
		parent := ""
		if category.Parent != "" {
			parent = " (inside " + category.Parent + ")"
		}
		fmt.Fprintf(&categories, "- %s%s: %d bookmarks. %s\n", category.Name, parent, category.Count, category.Description)
	}
	for _, tag := range snapshot.Tags {
		fmt.Fprintf(&tags, "%s (%s, %d); ", tag.Name, tag.Kind, tag.Count)
	}
	prompt := fmt.Sprintf(`You organize a personal library of %d saved X posts.
Propose up to %d new categories that would make this library easier to browse.
- Prefer sub-categories that split a crowded existing category into meaningful areas (for example "Databases" inside "Software Engineering", or "LLMs" inside "AI").
- Also propose new top-level categories for recurring subjects that no existing category covers, whatever the field.
- A good category holds at least 5 bookmarks and is distinct from its siblings. Do not propose a category that duplicates an existing one.
- parent must be the exact name of an existing category, or an empty string for a new top-level category.
- tags lists existing tag names (exactly as written below) whose bookmarks belong in the new category.
- description is one short sentence; reason says briefly why this helps.

Existing categories:
%s
Existing tags (name, kind, bookmark count):
%s

Sample summaries:
- %s`, snapshot.Analyzed, maxCategoryProposals, categories.String(), tags.String(), strings.Join(snapshot.Summaries, "\n- "))

	item := strictObject(map[string]any{
		"name":        map[string]any{"type": "string"},
		"parent":      map[string]any{"type": "string"},
		"description": map[string]any{"type": "string"},
		"reason":      map[string]any{"type": "string"},
		"tags":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	})
	schema := strictObject(map[string]any{"proposals": map[string]any{"type": "array", "items": item}})
	content, err := completer.CompleteJSON(ctx, prompt, "category_proposals", schema)
	if err != nil {
		return nil, err
	}
	var response struct {
		Proposals []model.CategoryProposal `json:"proposals"`
	}
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, fmt.Errorf("decode category proposals: %w", err)
	}

	existing := map[string]string{}
	for _, category := range snapshot.Categories {
		existing[NormalizeTag(category.Name)] = category.Name
	}
	knownTags := map[string]string{}
	for _, tag := range snapshot.Tags {
		knownTags[NormalizeTag(tag.Name)] = tag.Name
	}
	result := []model.CategoryProposal{}
	seen := map[string]bool{}
	for _, proposal := range response.Proposals {
		proposal.Name = strings.TrimSpace(proposal.Name)
		key := NormalizeTag(proposal.Name)
		if key == "" || existing[key] != "" || seen[key] {
			continue
		}
		if proposal.Parent = strings.TrimSpace(proposal.Parent); proposal.Parent != "" {
			name, ok := existing[NormalizeTag(proposal.Parent)]
			if !ok {
				continue
			}
			proposal.Parent = name
		}
		matched := []string{}
		for _, tag := range proposal.Tags {
			if name, ok := knownTags[NormalizeTag(tag)]; ok {
				matched = append(matched, name)
			}
		}
		if len(matched) == 0 {
			continue
		}
		proposal.Tags = matched
		proposal.Description = strings.TrimSpace(proposal.Description)
		proposal.Reason = strings.TrimSpace(proposal.Reason)
		seen[key] = true
		result = append(result, proposal)
		if len(result) == maxCategoryProposals {
			break
		}
	}
	return result, nil
}

// ObviousMerges finds tags that are the same concept spelled differently
// (singular and plural) without calling a model.
func ObviousMerges(tags []model.TagStat) []model.MergeProposal {
	byKey := map[string]model.TagStat{}
	for _, tag := range tags {
		byKey[NormalizeTag(tag.Name)] = tag
	}
	result := []model.MergeProposal{}
	used := map[string]bool{}
	for _, tag := range tags {
		key := NormalizeTag(tag.Name)
		if used[key] {
			continue
		}
		for _, variant := range TagVariants(key)[1:] {
			other, ok := byKey[variant]
			if !ok || used[variant] {
				continue
			}
			source, target := tag, other
			if other.Count < tag.Count || (other.Count == tag.Count && len(variant) > len(key)) {
				source, target = other, tag
			}
			used[NormalizeTag(source.Name)] = true
			result = append(result, model.MergeProposal{Source: source.Name, Target: target.Name, Reason: "Singular and plural spellings of the same tag."})
		}
	}
	return result
}

// ProposeMerges asks the model for tags that name the same concept (synonyms,
// abbreviations, spelling variants) and should be merged into one.
func ProposeMerges(ctx context.Context, completer JSONCompleter, tags []model.TagStat) ([]model.MergeProposal, error) {
	var list strings.Builder
	for _, tag := range tags {
		fmt.Fprintf(&list, "%s (%d); ", tag.Name, tag.Count)
	}
	prompt := fmt.Sprintf(`These are the tags in a personal bookmark library, with how many bookmarks use each.
Find tags that name exactly the same thing and should be merged: synonyms, abbreviations and their expansions ("llm" and "large language model"), and spelling or formatting variants ("hair care" and "haircare").
The test: someone searching for one tag would always want every post of the other. If a tag is broader, narrower, or merely related, do not merge it. For example, never merge "gym" into "fitness", "react" into "next.js", "postgres" into "database", or "fitness goals" into "learning goals".
For each merge, source is the tag to remove and target is the tag to keep; keep the clearer, more commonly used name. Use names exactly as written. confidence is how sure you are the two are the same thing (0 to 1); leave out anything you are not sure of. Return at most %d merges; an empty list is a good answer when nothing qualifies.

Tags:
%s`, maxMergeProposals, list.String())
	item := strictObject(map[string]any{
		"source":     map[string]any{"type": "string"},
		"target":     map[string]any{"type": "string"},
		"reason":     map[string]any{"type": "string"},
		"confidence": map[string]any{"type": "number"},
	})
	schema := strictObject(map[string]any{"merges": map[string]any{"type": "array", "items": item}})
	content, err := completer.CompleteJSON(ctx, prompt, "tag_merges", schema)
	if err != nil {
		return nil, err
	}
	var response struct {
		Merges []struct {
			model.MergeProposal
			Confidence float64 `json:"confidence"`
		} `json:"merges"`
	}
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, fmt.Errorf("decode merge proposals: %w", err)
	}
	known := map[string]string{}
	for _, tag := range tags {
		known[NormalizeTag(tag.Name)] = tag.Name
	}
	result := []model.MergeProposal{}
	usedSources := map[string]bool{}
	for _, merge := range response.Merges {
		source, sourceOK := known[NormalizeTag(merge.Source)]
		target, targetOK := known[NormalizeTag(merge.Target)]
		if !sourceOK || !targetOK || source == target || usedSources[source] || merge.Confidence < minMergeConfidence {
			continue
		}
		usedSources[source] = true
		result = append(result, model.MergeProposal{Source: source, Target: target, Reason: strings.TrimSpace(merge.Reason)})
		if len(result) == maxMergeProposals {
			break
		}
	}
	return result, nil
}
