// Package xembed reads public post details from X's embed service, the same
// endpoint embedded posts on websites use. Sortwise uses it only to fill in
// posts the extension could not read, such as X Articles, which show a title,
// preview, and cover image instead of post text.
package xembed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

var endpoint = "https://cdn.syndication.twimg.com/tweet-result"

// Post is the subset of embed data Sortwise uses.
type Post struct {
	Text    string
	Article *Article
	Photos  []Photo
}

type Article struct {
	ID          string
	Title       string
	PreviewText string
	CoverURL    string
}

type Photo struct {
	URL     string
	AltText string
	Width   int
	Height  int
}

type Client struct {
	HTTP *http.Client
}

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 15 * time.Second}}
}

var tcoOnly = regexp.MustCompile(`^(\s*https://t\.co/\w+\s*)+$`)

// OnlyLinks reports whether text is empty or nothing but t.co links, which is what
// an X Article post looks like to the extension.
func OnlyLinks(text string) bool {
	text = strings.TrimSpace(text)
	return text == "" || tcoOnly.MatchString(text)
}

// Fetch returns the public embed data for a post ID.
func (c *Client) Fetch(ctx context.Context, postID string) (Post, error) {
	id, err := strconv.ParseUint(postID, 10, 64)
	if err != nil {
		return Post{}, fmt.Errorf("invalid post id %q", postID)
	}
	url := fmt.Sprintf("%s?id=%d&lang=en&token=%s", endpoint, id, token(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Post{}, err
	}
	response, err := c.HTTP.Do(req)
	if err != nil {
		return Post{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Post{}, fmt.Errorf("X embed returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return Post{}, err
	}
	var payload struct {
		Text    string `json:"text"`
		Article *struct {
			RestID      string `json:"rest_id"`
			Title       string `json:"title"`
			PreviewText string `json:"preview_text"`
			CoverMedia  *struct {
				MediaInfo struct {
					URL string `json:"original_img_url"`
				} `json:"media_info"`
			} `json:"cover_media"`
		} `json:"article"`
		Photos []struct {
			URL             string `json:"url"`
			AccessibilityID string `json:"accessibilityLabel"`
			Width           int    `json:"width"`
			Height          int    `json:"height"`
		} `json:"photos"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return Post{}, fmt.Errorf("decode X embed: %w", err)
	}
	post := Post{Text: repairText(payload.Text)}
	if payload.Article != nil {
		post.Article = &Article{ID: payload.Article.RestID, Title: repairText(payload.Article.Title), PreviewText: repairText(payload.Article.PreviewText)}
		if payload.Article.CoverMedia != nil {
			post.Article.CoverURL = payload.Article.CoverMedia.MediaInfo.URL
		}
	}
	for _, photo := range payload.Photos {
		post.Photos = append(post.Photos, Photo{URL: photo.URL, AltText: repairText(photo.AccessibilityID), Width: photo.Width, Height: photo.Height})
	}
	return post, nil
}

// token reproduces the value X's embed widget sends with each request.
func token(id uint64) string {
	value := float64(id) / 1e15 * math.Pi
	encoded := strconv.FormatFloat(value, 'f', -1, 64)
	whole, fraction, _ := strings.Cut(encoded, ".")
	integer, _ := strconv.ParseUint(whole, 10, 64)
	result := strconv.FormatUint(integer, 36)
	// Base-36 digits of the fractional part, like JavaScript's Number#toString(36).
	frac := value - math.Floor(value)
	if fraction != "" {
		result += "."
		for i := 0; i < 12 && frac > 0; i++ {
			frac *= 36
			digit := int(frac)
			result += strconv.FormatInt(int64(digit), 36)
			frac -= float64(digit)
		}
	}
	return strings.NewReplacer("0", "", ".", "").Replace(result)
}

// repairText fixes UTF-8 text that was decoded as Windows-1252 somewhere upstream
// ("Iâ€™ve" becomes "I’ve"). Text that is already correct is returned unchanged.
func repairText(value string) string {
	if !strings.ContainsAny(value, "âÃ") {
		return value
	}
	encoded, err := charmap.Windows1252.NewEncoder().String(value)
	if err != nil || !utf8.ValidString(encoded) {
		return value
	}
	return encoded
}
