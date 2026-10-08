package ai

import (
	"errors"
	"strings"
	"testing"
	"time"

	"sortwise/internal/model"
)

func TestValidateResultEnforcesControlledCategoriesAndTags(t *testing.T) {
	input := AnalysisInput{Categories: []CategoryOption{{ID: 1, Name: "AI"}}}
	result := AnalysisResult{Summary: "A useful AI paper.", Categories: []Classification{{Name: "AI", Confidence: .9}}, Tags: []Classification{{Name: "paper", Confidence: .9}, {Name: "transformer", Confidence: .8}, {Name: "research", Confidence: .7}}}
	if err := ValidateResult(input, &result); err != nil {
		t.Fatal(err)
	}
	result.Categories[0].Name = "Invented"
	if err := ValidateResult(input, &result); err != nil {
		t.Fatalf("unknown category should be dropped, got %v", err)
	}
	if len(result.Categories) != 0 {
		t.Fatalf("unknown category kept: %+v", result.Categories)
	}
}

func TestValidateResultRepairsRecoverableMistakes(t *testing.T) {
	input := AnalysisInput{Categories: []CategoryOption{{ID: 1, Name: "AI"}, {ID: 2, Name: "Backend"}, {ID: 3, Name: "Career"}}}
	tags := []Classification{}
	for _, name := range []string{"#llm", "LLM", "rag", "agents", "evals", "postgres", "queues", "retries", "latency", "tracing"} {
		tags = append(tags, Classification{Name: name, Confidence: .5})
	}
	result := AnalysisResult{
		Summary:    " A post about LLM infrastructure. ",
		Categories: []Classification{{Name: "ai", Confidence: 1.4}, {Name: "Backend", Confidence: .6}, {Name: "Career", Confidence: .2}, {Name: "AI", Confidence: .9}},
		Tags:       tags,
	}
	if err := ValidateResult(input, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Categories) != 2 || result.Categories[0].Name != "AI" || result.Categories[0].Confidence != 1 {
		t.Fatalf("categories not repaired: %+v", result.Categories)
	}
	if len(result.Tags) != maxTags || NormalizeTag(result.Tags[0].Name) != "llm" {
		t.Fatalf("tags not repaired: %+v", result.Tags)
	}
	empty := AnalysisResult{Summary: "Something", Tags: []Classification{{Name: "###"}}}
	if err := ValidateResult(input, &empty); err == nil {
		t.Fatal("expected no-usable-tags rejection")
	}
}

func TestNormalizeTag(t *testing.T) {
	if got := NormalizeTag(" Large Language Models "); got != "large-language-models" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateResultNormalizesTagKindsAndDropsGenericTags(t *testing.T) {
	input := AnalysisInput{Categories: []CategoryOption{{ID: 1, Name: "Software Engineering"}}}
	result := AnalysisResult{Summary: "Scaling Postgres writes.", Tags: []Classification{
		{Name: "Software-Engineering", Confidence: .9},
		{Name: "programming", Confidence: .9},
		{Name: "Write-Ahead_Logging", Confidence: .8, Kind: "TOPIC"},
		{Name: "  PostgreSQL ", Confidence: .8, Kind: "tool"},
		{Name: "tutorial", Confidence: .7, Kind: "format"},
		{Name: "vector databases", Confidence: .6, Kind: "unknown"},
	}}
	if err := ValidateResult(input, &result); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, tag := range result.Tags {
		got[tag.Name] = tag.Kind
	}
	want := map[string]string{"write ahead logging": "topic", "PostgreSQL": "tool", "tutorial": "format", "vector databases": "topic"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Fatalf("tag %q: got kind %q in %v", name, got[name], got)
		}
	}
	onlyGeneric := AnalysisResult{Summary: "Misc.", Tags: []Classification{{Name: "programming", Confidence: .5}}}
	if err := ValidateResult(input, &onlyGeneric); err != nil || len(onlyGeneric.Tags) != 1 {
		t.Fatalf("a lone generic tag should be kept: %v %+v", err, onlyGeneric.Tags)
	}
}

func TestObviousMergesPairsSingularAndPlural(t *testing.T) {
	merges := ObviousMerges([]model.TagStat{{Name: "llms", Count: 2}, {Name: "llm", Count: 9}, {Name: "strategies", Count: 1}, {Name: "strategy", Count: 4}, {Name: "rag", Count: 3}})
	if len(merges) != 2 {
		t.Fatalf("got %+v", merges)
	}
	for _, merge := range merges {
		if !(merge.Source == "llms" && merge.Target == "llm") && !(merge.Source == "strategies" && merge.Target == "strategy") {
			t.Fatalf("unexpected merge %+v", merge)
		}
	}
}

func TestDailyQuotaDetectionAndReset(t *testing.T) {
	daily := errors.New("Error 429, Status: RESOURCE_EXHAUSTED, Details: [quotaId:GenerateRequestsPerDayPerProjectPerModel-FreeTier]")
	perMinute := errors.New("Error 429, Status: RESOURCE_EXHAUSTED, Details: [quotaId:GenerateRequestsPerMinutePerProjectPerModel-FreeTier]")
	if !isDailyQuotaError(daily) || isDailyQuotaError(perMinute) {
		t.Fatal("daily quota detection is wrong")
	}
	now := time.Date(2026, 9, 29, 10, 15, 0, 0, time.UTC) // 03:15 Pacific
	reset := NextQuotaReset(now)
	if hours := reset.Sub(now).Hours(); hours < 20 || hours > 21 {
		t.Fatalf("expected reset at the next Pacific midnight, got %s (%.1fh away)", reset, hours)
	}
	markDailyQuotaExhausted("test-model")
	if !dailyQuotaExhausted("test-model") || dailyQuotaExhausted("other-model") {
		t.Fatal("exhaustion should be tracked per model")
	}
}

func TestNormalizeTagKeepsEveryScript(t *testing.T) {
	cases := map[string]string{"Café Culture": "café-culture", "機械学習": "機械学習", "Zone 2 Cardio": "zone-2-cardio", "  #Rust  ": "rust"}
	for input, want := range cases {
		if got := NormalizeTag(input); got != want {
			t.Fatalf("NormalizeTag(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNonEnglishTagsAreKeptAndBroadFieldsDropped(t *testing.T) {
	result := AnalysisResult{Summary: "レシピ", Tags: []Classification{{Name: "発酵", Confidence: .9, Kind: "topic"}, {Name: "fitness", Confidence: .8, Kind: "topic"}, {Name: "sourdough", Confidence: .7, Kind: "topic"}}}
	if err := ValidateResult(AnalysisInput{}, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Tags) != 2 || result.Tags[0].Name != "発酵" || result.Tags[1].Name != "sourdough" {
		t.Fatalf("got %+v", result.Tags)
	}
}

func TestPromptUsesTheChosenLanguage(t *testing.T) {
	prompt := analysisPrompt(AnalysisInput{Text: "hola", Language: "Spanish"})
	if !strings.Contains(prompt, "written in Spanish") || !strings.Contains(prompt, "summary in Spanish") || strings.Contains(prompt, "%!") {
		t.Fatalf("prompt did not use the language:\n%s", prompt)
	}
	if !strings.Contains(analysisPrompt(AnalysisInput{Text: "hi"}), "summary in English") {
		t.Fatal("English should be the default")
	}
}
