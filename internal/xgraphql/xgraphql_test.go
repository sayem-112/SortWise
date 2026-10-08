package xgraphql

import (
	"os"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func loadTimeline(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/bookmarks.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseTimelineReadsEveryKindOfPost(t *testing.T) {
	posts, skipped, err := ParseTimeline(loadTimeline(t), now)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 {
		t.Fatalf("expected the tombstone to be skipped, got %d", skipped)
	}
	ids := []string{}
	for _, post := range posts {
		ids = append(ids, post.PostID)
	}
	if strings.Join(ids, ",") != "100,200,300,400" {
		t.Fatalf("posts out of order or missing: %v", ids)
	}

	plain := posts[0]
	if plain.Author != "Ada Lovelace" || plain.Username != "ada" || plain.URL != "https://x.com/Ada/status/100" {
		t.Fatalf("author fields: %+v", plain)
	}
	if plain.Text != "Engines & notes https://example.com/engines" {
		t.Fatalf("text should unescape HTML, expand links, and drop the media link: %q", plain.Text)
	}
	if plain.PostedAt == nil || *plain.PostedAt != "2026-09-01T10:00:00Z" {
		t.Fatalf("posted at: %v", plain.PostedAt)
	}
	if len(plain.Media) != 2 || plain.Media[0].Kind != "image" || plain.Media[0].AltText != "A diagram" || *plain.Media[0].Width != 1200 {
		t.Fatalf("photo: %+v", plain.Media)
	}
	if video := plain.Media[1]; video.Kind != "video_poster" || video.VideoURL != "https://video.twimg.com/high.mp4" {
		t.Fatalf("video should keep the highest-bitrate mp4: %+v", video)
	}
	if plain.VisibleContext.Card == nil || plain.VisibleContext.Card.URL != "https://example.com/engines" || plain.VisibleContext.Card.Title != "Analytical engines" {
		t.Fatalf("link card: %+v", plain.VisibleContext.Card)
	}
	if !plain.VisibleContext.EmbedChecked || len(plain.Raw) == 0 || plain.ExtractorVersion != Version {
		t.Fatal("parsed posts should skip the embed lookup and keep X's raw data")
	}

	long := posts[1]
	if !strings.HasPrefix(long.Text, "The full long post") || strings.Contains(long.Text, "https://t.co/") {
		t.Fatalf("long posts should use the note text with expanded links: %q", long.Text)
	}
	if long.Author != "grace" || long.Username != "grace" {
		t.Fatalf("user fields from the newer core shape: %+v", long)
	}

	quote := posts[2]
	if quote.VisibleContext.QuotedPost == nil || quote.VisibleContext.QuotedPost.PostID != "100" || quote.VisibleContext.QuotedPost.Text == "" {
		t.Fatalf("quoted post: %+v", quote.VisibleContext.QuotedPost)
	}
	if quote.Text != "Worth reading" {
		t.Fatalf("the link to the quoted post and reply mentions should be dropped: %q", quote.Text)
	}

	article := posts[3]
	if !article.VisibleContext.Article || article.VisibleContext.Card.Title != "How we rebuilt search" || article.VisibleContext.Card.URL != "https://x.com/i/article/555" {
		t.Fatalf("article: %+v", article.VisibleContext)
	}
	if article.Text != "" {
		t.Fatalf("an article's own link should not be its text: %q", article.Text)
	}
	if len(article.Media) != 1 || article.Media[0].URL != "https://pbs.twimg.com/cover.jpg" {
		t.Fatalf("article cover: %+v", article.Media)
	}
}

func TestParseTweetRejectsPlaceholders(t *testing.T) {
	if _, err := ParseTweet([]byte(`{"__typename":"TweetTombstone"}`), now); err != ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	post, err := ParseTweet([]byte(`{"__typename":"TweetWithVisibilityResults","tweet":{"rest_id":"9","core":{"user_results":{"result":{"legacy":{"name":"N","screen_name":"n"}}}},"legacy":{"full_text":"hi","created_at":"Tue Sep 01 10:00:00 +0000 2026"}}}`), now)
	if err != nil || post.PostID != "9" || post.Text != "hi" {
		t.Fatalf("visibility wrapper: %+v %v", post, err)
	}
}
