package ai

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"sortwise/internal/xembed"
)

// PostFetcher looks up public details for a post the extension could not read.
type PostFetcher interface {
	Fetch(ctx context.Context, postID string) (xembed.Post, error)
}

// SetPostFetcher enables filling in X Articles and other posts that arrive with
// no readable text or media.
func (w *Worker) SetPostFetcher(fetcher PostFetcher) {
	w.fetcher = fetcher
}

func needsHydration(input AnalysisInput) bool {
	return xembed.OnlyLinks(input.Text) && strings.TrimSpace(input.QuotedText) == "" &&
		strings.TrimSpace(input.CardTitle) == "" && len(input.Media) == 0
}

// hydrate fills a bookmark from X's public embed data and reports whether it
// added anything. An X Article becomes a link card (title and opening lines) plus
// its cover image, so both the AI and search can see its content. Failures are
// quiet: the bookmark simply stays as imported.
func (w *Worker) hydrate(ctx context.Context, bookmarkID int64) bool {
	if w.fetcher == nil {
		return false
	}
	var postID, visible string
	if err := w.db.QueryRowContext(ctx, `SELECT post_id,visible_context_json FROM bookmarks WHERE id=?`, bookmarkID).Scan(&postID, &visible); err != nil {
		return false
	}
	post, err := w.fetcher.Fetch(ctx, postID)
	if err != nil {
		return false
	}
	visibleContext := map[string]any{}
	_ = json.Unmarshal([]byte(visible), &visibleContext)
	// Remember the lookup so the background sweep never asks X about this post again.
	visibleContext["embedChecked"] = true
	changed := false
	if article := post.Article; article != nil && strings.TrimSpace(article.Title) != "" {
		url := "https://x.com/i/article/" + article.ID
		if article.ID == "" {
			url = ""
		}
		visibleContext["card"] = map[string]string{"url": url, "title": article.Title, "description": article.PreviewText}
		visibleContext["article"] = true
		changed = true
		if article.CoverURL != "" {
			_, _ = w.db.ExecContext(ctx, `INSERT OR IGNORE INTO media(bookmark_id,kind,url,preview_url,alt_text,position) VALUES (?,?,?,?,?,0)`, bookmarkID, "image", article.CoverURL, article.CoverURL, "Article cover: "+article.Title)
		}
	}
	for index, photo := range post.Photos {
		if index >= 4 || photo.URL == "" {
			break
		}
		result, err := w.db.ExecContext(ctx, `INSERT OR IGNORE INTO media(bookmark_id,kind,url,preview_url,alt_text,width,height,position) VALUES (?,?,?,?,?,?,?,?)`, bookmarkID, "image", photo.URL, photo.URL, photo.AltText, photo.Width, photo.Height, index+1)
		if err == nil {
			if count, _ := result.RowsAffected(); count > 0 {
				changed = true
			}
		}
	}
	data, err := json.Marshal(visibleContext)
	if err != nil {
		return false
	}
	_, err = w.db.ExecContext(ctx, `UPDATE bookmarks SET visible_context_json=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, string(data), bookmarkID)
	return err == nil && changed
}

const hydrationSweepInterval = time.Minute

// hydratePending fills in recently imported posts the extension could not read,
// independent of whether AI is configured, paused, or out of quota.
func (w *Worker) hydratePending(ctx context.Context) {
	if w.fetcher == nil || time.Since(w.lastHydrationSweep) < hydrationSweepInterval {
		return
	}
	w.lastHydrationSweep = time.Now()
	rows, err := w.db.QueryContext(ctx, `SELECT b.id FROM bookmarks b WHERE json_extract(b.visible_context_json,'$.embedChecked') IS NULL AND json_extract(b.visible_context_json,'$.card') IS NULL AND json_extract(b.visible_context_json,'$.quotedPost') IS NULL AND (trim(b.text)='' OR b.text LIKE 'https://t.co/%') AND NOT EXISTS (SELECT 1 FROM media m WHERE m.bookmark_id=b.id) ORDER BY b.id DESC LIMIT 20`)
	if err != nil {
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		var text string
		if w.db.QueryRowContext(ctx, `SELECT text FROM bookmarks WHERE id=?`, id).Scan(&text) != nil || !xembed.OnlyLinks(text) {
			continue
		}
		w.hydrate(ctx, id)
	}
}
