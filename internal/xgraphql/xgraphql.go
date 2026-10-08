// Package xgraphql reads posts from the data X's web app loads for itself: the
// Bookmarks timeline and the individual posts the extension sees when you
// bookmark something. This data is complete where reading the page is not: it
// has the full text of long posts, X Articles, quoted posts, video files, and
// link cards, and it does not change when X redesigns its pages.
package xgraphql

import (
	"encoding/json"
	"errors"
	"html"
	"sort"
	"strings"
	"time"

	"sortwise/internal/model"
)

// Version identifies bookmarks read by this parser.
const Version = "x-graphql-1"

// ErrUnavailable means X returned a placeholder instead of a post (deleted,
// withheld, or from an account you can no longer see).
var ErrUnavailable = errors.New("post is unavailable")

type object = map[string]any

// ParseTimeline returns the posts in a timeline response, newest first as X
// listed them, and how many entries could not be read.
func ParseTimeline(data []byte, now time.Time) ([]model.ImportedBookmark, int, error) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, 0, err
	}
	var results []object
	collectTimelineTweets(root, &results)
	posts := make([]model.ImportedBookmark, 0, len(results))
	skipped := 0
	seen := map[string]bool{}
	for _, result := range results {
		post, err := parseResult(result, now)
		if err != nil || seen[post.PostID] {
			if err != nil {
				skipped++
			}
			continue
		}
		seen[post.PostID] = true
		posts = append(posts, post)
	}
	return posts, skipped, nil
}

// ParseTweet reads a single post result (tweet_results.result).
func ParseTweet(data []byte, now time.Time) (model.ImportedBookmark, error) {
	var result object
	if err := json.Unmarshal(data, &result); err != nil {
		return model.ImportedBookmark{}, err
	}
	return parseResult(result, now)
}

// collectTimelineTweets finds timeline items holding a post. Promoted posts
// (ads) are skipped; they never appear in bookmarks but can in other timelines.
func collectTimelineTweets(node any, out *[]object) {
	switch value := node.(type) {
	case object:
		if value["__typename"] == "TimelineTweet" || value["itemType"] == "TimelineTweet" {
			if value["promotedMetadata"] == nil {
				if result, ok := dig(value, "tweet_results", "result").(object); ok {
					*out = append(*out, result)
				}
			}
			return
		}
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		// Entries live in arrays, which keep X's order; sorting the keys only
		// makes the walk deterministic.
		sort.Strings(keys)
		for _, key := range keys {
			collectTimelineTweets(value[key], out)
		}
	case []any:
		for _, item := range value {
			collectTimelineTweets(item, out)
		}
	}
}

func parseResult(result object, now time.Time) (model.ImportedBookmark, error) {
	tweet := unwrap(result)
	if tweet == nil {
		return model.ImportedBookmark{}, ErrUnavailable
	}
	legacy, _ := tweet["legacy"].(object)
	id := str(tweet["rest_id"])
	if id == "" {
		id = str(legacy["id_str"])
	}
	name, screen := user(tweet)
	if id == "" || legacy == nil || screen == "" {
		return model.ImportedBookmark{}, ErrUnavailable
	}

	quoted := quotedPost(tweet, legacy)
	post := model.ImportedBookmark{
		Platform:         "x",
		PostID:           id,
		Author:           name,
		Username:         strings.ToLower(screen),
		Text:             text(tweet, legacy, quoted),
		URL:              "https://x.com/" + screen + "/status/" + id,
		PostedAt:         postedAt(legacy),
		Language:         optional(str(legacy["lang"])),
		Media:            media(legacy),
		ExtractedAt:      now.UTC().Format(time.RFC3339),
		ExtractorVersion: Version,
	}
	if post.Author == "" {
		post.Author = screen
	}
	post.VisibleContext.QuotedPost = quoted
	// Everything here came from X directly, so the embed lookup is not needed.
	post.VisibleContext.EmbedChecked = true
	if article := articleCard(tweet); article != nil {
		post.VisibleContext.Card = &article.card
		post.VisibleContext.Article = true
		// The post text of an Article is only a link to it, which the card shows.
		if article.card.URL != "" {
			path := strings.TrimPrefix(article.card.URL, "https://")
			for _, prefix := range []string{"https://", "http://"} {
				post.Text = strings.TrimSpace(strings.ReplaceAll(post.Text, prefix+path, ""))
			}
		}
		if article.cover != "" && len(post.Media) < 4 {
			cover := model.ImportedMedia{Kind: "image", URL: article.cover, PreviewURL: article.cover, AltText: "Article cover: " + article.card.Title}
			post.Media = append([]model.ImportedMedia{cover}, post.Media...)
		}
	} else {
		post.VisibleContext.Card = linkCard(tweet, legacy)
	}
	raw, err := json.Marshal(result)
	if err == nil {
		post.Raw = raw
	}
	return post, nil
}

// unwrap returns the post inside a result, or nil for a placeholder.
func unwrap(result object) object {
	switch str(result["__typename"]) {
	case "TweetWithVisibilityResults":
		inner, _ := result["tweet"].(object)
		return inner
	case "Tweet", "":
		if result["legacy"] == nil {
			return nil
		}
		return result
	default:
		return nil
	}
}

// user reads the author's display name and handle. X has moved these fields
// between user.legacy and user.core, so both are checked.
func user(tweet object) (string, string) {
	result, _ := dig(tweet, "core", "user_results", "result").(object)
	if result == nil {
		return "", ""
	}
	name := first(str(dig(result, "core", "name")), str(dig(result, "legacy", "name")))
	screen := first(str(dig(result, "core", "screen_name")), str(dig(result, "legacy", "screen_name")))
	return name, screen
}

// text returns the full post text with t.co links expanded, the trailing media
// link removed, and reply mentions X hides dropped. Long posts ("note tweets")
// carry their full text separately from the truncated legacy text.
func text(tweet, legacy object, quoted *model.QuotedPost) string {
	var body string
	var urls []any
	if note, ok := dig(tweet, "note_tweet", "note_tweet_results", "result").(object); ok && str(note["text"]) != "" {
		body = str(note["text"])
		urls, _ = dig(note, "entity_set", "urls").([]any)
	} else {
		body = str(legacy["full_text"])
		urls, _ = dig(legacy, "entities", "urls").([]any)
		if bounds, ok := legacy["display_text_range"].([]any); ok && len(bounds) == 2 {
			if start, ok := bounds[0].(float64); ok && start > 0 {
				runes := []rune(body)
				if int(start) < len(runes) {
					body = string(runes[int(start):])
				}
			}
		}
	}
	for _, entry := range urls {
		link, _ := entry.(object)
		short, expanded := str(link["url"]), str(link["expanded_url"])
		if short == "" {
			continue
		}
		// The link to a quoted post duplicates the quote shown below the text.
		if quoted != nil && strings.Contains(expanded, "/status/"+quoted.PostID) {
			expanded = ""
		}
		body = strings.ReplaceAll(body, short, expanded)
	}
	if mediaLinks, ok := dig(legacy, "entities", "media").([]any); ok {
		for _, entry := range mediaLinks {
			if short := str(dig(entry, "url")); short != "" {
				body = strings.ReplaceAll(body, short, "")
			}
		}
	}
	return strings.TrimSpace(html.UnescapeString(body))
}

func postedAt(legacy object) *string {
	parsed, err := time.Parse("Mon Jan 02 15:04:05 -0700 2006", str(legacy["created_at"]))
	if err != nil {
		return nil
	}
	value := parsed.UTC().Format(time.RFC3339)
	return &value
}

func media(legacy object) []model.ImportedMedia {
	items, ok := dig(legacy, "extended_entities", "media").([]any)
	if !ok {
		items, _ = dig(legacy, "entities", "media").([]any)
	}
	output := []model.ImportedMedia{}
	for _, entry := range items {
		item, _ := entry.(object)
		url := str(item["media_url_https"])
		if url == "" || len(output) == 4 {
			continue
		}
		media := model.ImportedMedia{Kind: "image", URL: url, PreviewURL: url, AltText: str(item["ext_alt_text"])}
		if width, ok := dig(item, "original_info", "width").(float64); ok && width > 0 {
			value := int(width)
			media.Width = &value
		}
		if height, ok := dig(item, "original_info", "height").(float64); ok && height > 0 {
			value := int(height)
			media.Height = &value
		}
		if kind := str(item["type"]); kind == "video" || kind == "animated_gif" {
			media.Kind = "video_poster"
			media.VideoURL = bestVideo(item)
		}
		output = append(output, media)
	}
	return output
}

// bestVideo picks the highest-bitrate MP4 variant.
func bestVideo(item object) string {
	variants, _ := dig(item, "video_info", "variants").([]any)
	best, bestRate := "", -1.0
	for _, entry := range variants {
		variant, _ := entry.(object)
		if str(variant["content_type"]) != "video/mp4" {
			continue
		}
		rate, _ := variant["bitrate"].(float64)
		if url := str(variant["url"]); url != "" && rate > bestRate {
			best, bestRate = url, rate
		}
	}
	return best
}

func quotedPost(tweet, legacy object) *model.QuotedPost {
	if result, ok := dig(tweet, "quoted_status_result", "result").(object); ok {
		if inner := unwrap(result); inner != nil {
			innerLegacy, _ := inner["legacy"].(object)
			_, screen := user(inner)
			id := first(str(inner["rest_id"]), str(innerLegacy["id_str"]))
			if id != "" && screen != "" {
				return &model.QuotedPost{
					PostID:   id,
					Username: strings.ToLower(screen),
					URL:      "https://x.com/" + screen + "/status/" + id,
					Text:     text(inner, innerLegacy, nil),
				}
			}
		}
	}
	// The quoted post is unavailable, but its link still identifies it.
	id := str(legacy["quoted_status_id_str"])
	permalink := str(dig(legacy, "quoted_status_permalink", "expanded"))
	if id == "" || permalink == "" {
		return nil
	}
	username := ""
	if parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(permalink, "https://"), "x.com/"), "/"); len(parts) > 2 && parts[1] == "status" {
		username = strings.ToLower(parts[0])
	}
	return &model.QuotedPost{PostID: id, Username: username, URL: strings.Replace(permalink, "twitter.com", "x.com", 1)}
}

type article struct {
	card  model.LinkCard
	cover string
}

func articleCard(tweet object) *article {
	result, ok := dig(tweet, "article", "article_results", "result").(object)
	if !ok || strings.TrimSpace(str(result["title"])) == "" {
		return nil
	}
	url := ""
	if id := str(result["rest_id"]); id != "" {
		url = "https://x.com/i/article/" + id
	}
	return &article{
		card:  model.LinkCard{URL: url, Title: strings.TrimSpace(str(result["title"])), Description: strings.TrimSpace(str(result["preview_text"]))},
		cover: str(dig(result, "cover_media", "media_info", "original_img_url")),
	}
}

// linkCard reads a website preview card. Polls and app cards have no title and
// are left out.
func linkCard(tweet, legacy object) *model.LinkCard {
	card, ok := dig(tweet, "card", "legacy").(object)
	if !ok {
		return nil
	}
	values := map[string]string{}
	bindings, _ := card["binding_values"].([]any)
	for _, entry := range bindings {
		binding, _ := entry.(object)
		if value := str(dig(binding, "value", "string_value")); value != "" {
			values[str(binding["key"])] = value
		}
	}
	title := strings.TrimSpace(values["title"])
	if title == "" {
		return nil
	}
	url := first(values["card_url"], str(card["url"]))
	if urls, ok := dig(legacy, "entities", "urls").([]any); ok {
		for _, entry := range urls {
			if str(dig(entry, "url")) == url {
				url = str(dig(entry, "expanded_url"))
			}
		}
	}
	return &model.LinkCard{URL: url, Title: title, Description: strings.TrimSpace(values["description"])}
}

func dig(node any, path ...string) any {
	for _, key := range path {
		value, ok := node.(object)
		if !ok {
			return nil
		}
		node = value[key]
	}
	return node
}

func str(value any) string {
	text, _ := value.(string)
	return text
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
