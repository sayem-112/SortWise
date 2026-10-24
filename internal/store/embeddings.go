package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"sortwise/internal/model"
)

// maxEmbeddingRunes bounds the text one post's vector is made from.
const maxEmbeddingRunes = 6000

// embeddable are the posts whose text has settled: the AI has read them (or
// given up), so the summary a vector should include is there.
const embeddable = `b.processing_status IN ('completed','failed')`

// MeaningSearchEnabled reports whether search by meaning is on (it is unless
// turned off in Settings).
func (s *Store) MeaningSearchEnabled(ctx context.Context) bool {
	return s.setting(ctx, "meaning_search") != "false"
}

func (s *Store) SetMeaningSearch(ctx context.Context, on bool) error {
	return s.setSetting(ctx, "meaning_search", strconv.FormatBool(on))
}

// AIPaused reports whether background AI work is paused in Settings.
func (s *Store) AIPaused(ctx context.Context) bool {
	return s.setting(ctx, "ai_paused") == "true"
}

// EmbeddingSource is a post that may need a new vector.
type EmbeddingSource struct {
	ID int64
	// Text is what the vector describes; "" when the post has nothing to describe.
	Text string
	// Hash identifies Text and the model. Current is true when the stored
	// vector was already made from exactly that.
	Hash    string
	Current bool
}

// embeddingText joins what a post says with what the AI wrote about it.
// Tags and categories are left out, so renaming or merging them does not
// send every post through the AI service again.
func embeddingText(author, username, text, visible, summary, media string) string {
	var shown model.VisibleContext
	_ = json.Unmarshal([]byte(visible), &shown)
	parts := []string{}
	add := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, label+value)
		}
	}
	add("", text)
	if shown.QuotedPost != nil {
		add("Quoted post: ", shown.QuotedPost.Text)
	}
	if shown.Card != nil {
		add("Link: ", strings.TrimSpace(shown.Card.Title+". "+shown.Card.Description))
	}
	add("Summary: ", summary)
	add("In the images: ", media)
	if len(parts) == 0 {
		return ""
	}
	body := strings.Join(parts, "\n")
	if who := strings.TrimSpace(author + " @" + username); who != "@" {
		body = "Post by " + who + "\n" + body
	}
	if runes := []rune(body); len(runes) > maxEmbeddingRunes {
		body = string(runes[:maxEmbeddingRunes])
	}
	return body
}

func embeddingHash(modelName, text string) string {
	sum := sha256.Sum256([]byte(modelName + "\n" + text))
	return hex.EncodeToString(sum[:16])
}

// EmbeddingCandidates returns posts with no vector, a vector from another
// model, or a change since their vector was last checked. Posts with no
// vector come first.
func (s *Store) EmbeddingCandidates(ctx context.Context, modelName string, limit int) ([]EmbeddingSource, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT b.id,b.author,b.username,b.text,b.visible_context_json,COALESCE(e.manual_summary,e.ai_summary,''),COALESCE(e.media_description,''),COALESCE(m.input_hash,''),COALESCE(m.model,'')
		FROM bookmarks b LEFT JOIN enrichments e ON e.bookmark_id=b.id LEFT JOIN embeddings m ON m.bookmark_id=b.id
		WHERE `+embeddable+` AND (m.bookmark_id IS NULL OR m.model<>? OR b.updated_at>=m.checked_at OR COALESCE(e.updated_at,'')>=m.checked_at)
		ORDER BY m.bookmark_id IS NULL DESC,b.id DESC LIMIT ?`, modelName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []EmbeddingSource{}
	for rows.Next() {
		var item EmbeddingSource
		var author, username, text, visible, summary, media, storedHash, storedModel string
		if err := rows.Scan(&item.ID, &author, &username, &text, &visible, &summary, &media, &storedHash, &storedModel); err != nil {
			return nil, err
		}
		item.Text = embeddingText(author, username, text, visible, summary, media)
		item.Hash = embeddingHash(modelName, item.Text)
		item.Current = storedModel == modelName && (storedHash == item.Hash || storedHash == RefusedPrefix+item.Hash)
		result = append(result, item)
	}
	return result, rows.Err()
}

// RefusedPrefix marks, in a stored hash, a post the AI service would not
// take. It is left alone until its text changes or refusals are forgotten.
const RefusedPrefix = "refused:"

// ForgetRefusedEmbeddings makes posts the AI service refused due to be tried
// again, for when the key or the settings change.
func (s *Store) ForgetRefusedEmbeddings(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM embeddings WHERE input_hash LIKE ?`, RefusedPrefix+"%")
	return err
}

// SavedEmbedding is a post's new vector. Vector is empty for a post with
// nothing to describe.
type SavedEmbedding struct {
	ID     int64
	Hash   string
	Vector []byte
}

// SaveEmbeddings stores new vectors. checkedAt is when the posts were read,
// so a change made while their vectors were being computed is seen next time.
// Posts deleted meanwhile are skipped.
func (s *Store) SaveEmbeddings(ctx context.Context, modelName, checkedAt string, items []SavedEmbedding) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var value string
	_ = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='embedding_version'`).Scan(&value)
	version, _ := strconv.ParseInt(value, 10, 64)
	version++
	for _, item := range items {
		vector := item.Vector
		if vector == nil {
			vector = []byte{}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO embeddings(bookmark_id,model,input_hash,vector,checked_at,version)
			SELECT ?,?,?,?,?,? WHERE EXISTS (SELECT 1 FROM bookmarks WHERE id=?)
			ON CONFLICT(bookmark_id) DO UPDATE SET model=excluded.model,input_hash=excluded.input_hash,vector=excluded.vector,checked_at=excluded.checked_at,version=excluded.version`,
			item.ID, modelName, item.Hash, vector, checkedAt, version, item.ID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES ('embedding_version',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=CURRENT_TIMESTAMP`, strconv.FormatInt(version, 10)); err != nil {
		return err
	}
	return tx.Commit()
}

// TouchEmbeddings records that these posts were compared with their vectors
// and nothing had changed.
func (s *Store) TouchEmbeddings(ctx context.Context, ids []int64, checkedAt string) error {
	for start := 0; start < len(ids); start += 400 {
		chunk := ids[start:min(start+400, len(ids))]
		args := []any{checkedAt}
		for _, id := range chunk {
			args = append(args, id)
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE embeddings SET checked_at=? WHERE bookmark_id IN (?`+strings.Repeat(",?", len(chunk)-1)+`)`, args...); err != nil {
			return err
		}
	}
	return nil
}

// EmbeddingSignature changes whenever the stored vectors do: a write raises
// the version, a deleted post lowers the count.
func (s *Store) EmbeddingSignature(ctx context.Context, modelName string) (count int, version int64, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MAX(version),0) FROM embeddings WHERE model=? AND length(vector)>0`, modelName).Scan(&count, &version)
	return count, version, err
}

// LoadEmbeddings calls visit with every stored vector for the model.
func (s *Store) LoadEmbeddings(ctx context.Context, modelName string, visit func(id int64, vector []byte)) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT bookmark_id,vector FROM embeddings WHERE model=? AND length(vector)>0 ORDER BY bookmark_id`, modelName)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var vector []byte
		if err := rows.Scan(&id, &vector); err != nil {
			return err
		}
		visit(id, vector)
	}
	return rows.Err()
}

// EmbeddingProgress returns how many posts have been indexed for search by
// meaning, out of how many can be.
func (s *Store) EmbeddingProgress(ctx context.Context, modelName string) (indexed, total int, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(m.bookmark_id),COUNT(*) FROM bookmarks b LEFT JOIN embeddings m ON m.bookmark_id=b.id AND m.model=? WHERE `+embeddable, modelName).Scan(&indexed, &total)
	return indexed, total, err
}

// QueryVector returns the saved vector for a search made before. used marks
// it as used just now, which keeps it from being dropped for age.
func (s *Store) QueryVector(ctx context.Context, hash string, used bool) ([]byte, bool) {
	var vector []byte
	if err := s.DB.QueryRowContext(ctx, `SELECT vector FROM query_vectors WHERE query_hash=?`, hash).Scan(&vector); err != nil || len(vector) == 0 {
		return nil, false
	}
	if used {
		_, _ = s.DB.ExecContext(ctx, `UPDATE query_vectors SET used_at=CURRENT_TIMESTAMP WHERE query_hash=?`, hash)
	}
	return vector, true
}

// maxQueryVectors is how many past searches keep their vector.
const maxQueryVectors = 500

// SaveQueryVector remembers a search's vector, dropping the ones used
// longest ago once there are too many.
func (s *Store) SaveQueryVector(ctx context.Context, hash string, vector []byte) error {
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO query_vectors(query_hash,vector) VALUES (?,?) ON CONFLICT(query_hash) DO UPDATE SET vector=excluded.vector,used_at=CURRENT_TIMESTAMP`, hash, vector); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM query_vectors WHERE query_hash NOT IN (SELECT query_hash FROM query_vectors ORDER BY used_at DESC,rowid DESC LIMIT ?)`, maxQueryVectors)
	return err
}

// RelatedBookmarks returns the given posts, in the order given, leaving out
// archived and deleted ones.
func (s *Store) RelatedBookmarks(ctx context.Context, ids []int64, limit int) ([]model.RelatedBookmark, error) {
	if len(ids) == 0 {
		return []model.RelatedBookmark{}, nil
	}
	args := make([]any, len(ids))
	for index, id := range ids {
		args[index] = id
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT b.id,b.author,b.username,b.posted_at,b.text,b.visible_context_json,COALESCE(e.manual_summary,e.ai_summary,''),COALESCE(e.media_description,''),`+avatarColumn+`
		FROM bookmarks b LEFT JOIN enrichments e ON e.bookmark_id=b.id WHERE b.archived_at IS NULL AND b.id IN (?`+strings.Repeat(",?", len(ids)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := map[int64]model.RelatedBookmark{}
	for rows.Next() {
		var item model.RelatedBookmark
		var posted *string
		var text, visible, summary, media, avatar string
		if err := rows.Scan(&item.ID, &item.Author, &item.Username, &posted, &text, &visible, &summary, &media, &avatar); err != nil {
			return nil, err
		}
		item.PostedAt = posted
		item.Avatar = AvatarPath(avatar)
		item.Text = postPreview(text, visible, summary, media)
		found[item.ID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := []model.RelatedBookmark{}
	for _, id := range ids {
		if item, ok := found[id]; ok && len(result) < limit {
			result = append(result, item)
		}
	}
	return result, nil
}

// postPreview picks the best one-line description of a post: its text, or,
// for articles and link-only posts, the card title, the quoted text, what is
// in the images, then the summary.
func postPreview(text, visible, summary, media string) string {
	var shown model.VisibleContext
	_ = json.Unmarshal([]byte(visible), &shown)
	linkOnly := true
	for _, word := range strings.Fields(text) {
		if !strings.HasPrefix(word, "https://t.co/") {
			linkOnly = false
			break
		}
	}
	candidates := []string{}
	if !linkOnly {
		candidates = append(candidates, text)
	}
	if shown.Card != nil {
		candidates = append(candidates, shown.Card.Title)
	}
	if shown.QuotedPost != nil {
		candidates = append(candidates, shown.QuotedPost.Text)
	}
	candidates = append(candidates, media, summary, text)
	for _, value := range candidates {
		if value = strings.Join(strings.Fields(value), " "); value != "" {
			if runes := []rune(value); len(runes) > 240 {
				return string(runes[:239]) + "…"
			}
			return value
		}
	}
	return "Media bookmark"
}
