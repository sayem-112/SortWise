package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"sortwise/internal/model"
)

var postIDPattern = regexp.MustCompile(`^[0-9]+$`)

type Store struct{ DB *sql.DB }

func New(db *sql.DB) *Store { return &Store{DB: db} }

func ValidateImportedBookmark(item model.ImportedBookmark) error {
	if item.Platform != "x" || !postIDPattern.MatchString(item.PostID) {
		return errors.New("invalid X post identifier")
	}
	parsed, err := url.Parse(item.URL)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "x.com") {
		return errors.New("post URL must use https://x.com")
	}
	if item.ExtractorVersion == "" || len(item.Text) > 100_000 || len(item.Media) > 4 {
		return errors.New("invalid bookmark payload")
	}
	for _, media := range item.Media {
		if media.Kind != "image" && media.Kind != "video_poster" {
			return errors.New("invalid media kind")
		}
		if !twimgURL(media.URL) || (media.VideoURL != "" && !twimgURL(media.VideoURL)) {
			return errors.New("media URL must be hosted by twimg.com")
		}
	}
	return nil
}

func twimgURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && strings.HasSuffix(strings.ToLower(u.Hostname()), "twimg.com")
}

func contentHash(item model.ImportedBookmark) string {
	stable := struct {
		Author         string
		Username       string
		Text           string
		URL            string
		PostedAt       *string
		Language       *string
		VisibleContext model.VisibleContext
		Media          []model.ImportedMedia
	}{item.Author, item.Username, item.Text, item.URL, item.PostedAt, item.Language, item.VisibleContext, item.Media}
	body, _ := json.Marshal(stable)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (s *Store) Import(ctx context.Context, batch model.ImportBatch) (model.ImportResult, error) {
	if len(batch.Bookmarks) == 0 || len(batch.Bookmarks) > 100 {
		return model.ImportResult{}, errors.New("an import batch must contain between 1 and 100 bookmarks")
	}
	source := batch.Source
	if source == "" {
		source = "extension"
	}
	for _, item := range batch.Bookmarks {
		if err := ValidateImportedBookmark(item); err != nil {
			return model.ImportResult{}, fmt.Errorf("post %s: %w", item.PostID, err)
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return model.ImportResult{}, err
	}
	defer tx.Rollback()
	result := model.ImportResult{}
	created, err := tx.ExecContext(ctx, `INSERT INTO imports(source,received_count) VALUES (?,?)`, source, len(batch.Bookmarks))
	if err != nil {
		return result, err
	}
	result.ImportID, _ = created.LastInsertId()
	for _, item := range batch.Bookmarks {
		change, bookmarkID, hash, err := upsertBookmark(ctx, tx, item)
		if err != nil {
			return result, err
		}
		if err := noteAvatar(ctx, tx, item.Username, item.AvatarURL); err != nil {
			return result, err
		}
		switch change {
		case "inserted":
			result.Inserted++
		case "updated":
			result.Updated++
		case "upgraded":
			result.Upgraded++
		default:
			result.Unchanged++
		}
		if change != "unchanged" {
			if err := replaceMedia(ctx, tx, bookmarkID, item.Media); err != nil {
				return result, err
			}
		}
		if change == "inserted" || change == "updated" {
			_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO jobs(bookmark_id, input_hash) VALUES (?, ?)`, bookmarkID, hash)
			if err != nil {
				return result, err
			}
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE imports SET completed_at=CURRENT_TIMESTAMP, inserted_count=?, updated_count=?, unchanged_count=?, failed_count=? WHERE id=?`, result.Inserted, result.Updated, result.Unchanged, result.Failed, result.ImportID)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

// upsertBookmark saves one post and reports "inserted", "updated" (the AI should
// look again), "upgraded" (better data for a post the AI already read), or
// "unchanged". Seeing a post again also clears any "removed on X" mark.
func upsertBookmark(ctx context.Context, tx *sql.Tx, item model.ImportedBookmark) (string, int64, string, error) {
	hash := contentHash(item)
	var id int64
	var existing, version, oldText, oldVisible string
	var oldMedia int
	err := tx.QueryRowContext(ctx, `SELECT id, content_hash, extractor_version, text, visible_context_json, (SELECT COUNT(*) FROM media m WHERE m.bookmark_id=b.id) FROM bookmarks b WHERE platform=? AND post_id=?`, item.Platform, item.PostID).Scan(&id, &existing, &version, &oldText, &oldVisible, &oldMedia)
	visible := model.RawJSON(item.VisibleContext)
	raw := model.RawJSON(item)
	if len(item.Raw) > 0 {
		raw = string(item.Raw)
	}
	if errors.Is(err, sql.ErrNoRows) {
		result, err := tx.ExecContext(ctx, `INSERT INTO bookmarks(platform,post_id,author,username,text,url,posted_at,bookmarked_at,imported_at,last_seen_at,language,visible_context_json,raw_payload_json,extractor_version,content_hash) VALUES (?,?,?,?,?,?,?,?,COALESCE(NULLIF(?,''),CURRENT_TIMESTAMP),COALESCE(NULLIF(?,''),CURRENT_TIMESTAMP),?,?,?,?,?)`, item.Platform, item.PostID, item.Author, item.Username, item.Text, item.URL, item.PostedAt, item.BookmarkedAt, item.ExtractedAt, item.ExtractedAt, item.Language, visible, raw, item.ExtractorVersion, hash)
		if err != nil {
			return "", 0, "", err
		}
		id, _ = result.LastInsertId()
		return "inserted", id, hash, nil
	}
	if err != nil {
		return "", 0, "", err
	}
	if existing == hash {
		_, err = tx.ExecContext(ctx, `UPDATE bookmarks SET last_seen_at=COALESCE(NULLIF(?,''),CURRENT_TIMESTAMP), raw_payload_json=?, extractor_version=?, removed_on_x_at=NULL, updated_at=CURRENT_TIMESTAMP WHERE id=?`, item.ExtractedAt, raw, item.ExtractorVersion, id)
		return "unchanged", id, hash, err
	}
	// A post first read from the page, now read from X's own data, differs in
	// small ways (whitespace, expanded links) for every post. Only ask the AI
	// again when the new data holds real content the page reading missed.
	if strings.HasPrefix(item.ExtractorVersion, "x-graphql") && !strings.HasPrefix(version, "x-graphql") && !richer(item, oldText, oldVisible, oldMedia) {
		_, err = tx.ExecContext(ctx, `UPDATE bookmarks SET author=?,username=?,text=?,url=?,posted_at=COALESCE(?,posted_at),last_seen_at=COALESCE(NULLIF(?,''),CURRENT_TIMESTAMP),language=COALESCE(?,language),visible_context_json=?,raw_payload_json=?,extractor_version=?,content_hash=?,removed_on_x_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=?`, item.Author, item.Username, item.Text, item.URL, item.PostedAt, item.ExtractedAt, item.Language, visible, raw, item.ExtractorVersion, hash, id)
		return "upgraded", id, hash, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE bookmarks SET author=?,username=?,text=?,url=?,posted_at=?,bookmarked_at=?,last_seen_at=COALESCE(NULLIF(?,''),CURRENT_TIMESTAMP),language=?,visible_context_json=?,raw_payload_json=?,extractor_version=?,content_hash=?,processing_status='pending',removed_on_x_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=?`, item.Author, item.Username, item.Text, item.URL, item.PostedAt, item.BookmarkedAt, item.ExtractedAt, item.Language, visible, raw, item.ExtractorVersion, hash, id)
	return "updated", id, hash, err
}

// richer reports whether X's own data adds content the page reading missed: the
// rest of a long post, more media, or a link card or quote that was not seen.
func richer(item model.ImportedBookmark, oldText, oldVisible string, oldMedia int) bool {
	var old model.VisibleContext
	_ = json.Unmarshal([]byte(oldVisible), &old)
	if len([]rune(item.Text)) > len([]rune(oldText))+80 || len(item.Media) > oldMedia {
		return true
	}
	if item.VisibleContext.Card != nil && old.Card == nil {
		return true
	}
	quote := item.VisibleContext.QuotedPost
	return quote != nil && quote.Text != "" && (old.QuotedPost == nil || old.QuotedPost.Text == "")
}

// MarkRemovedOnX records posts that were unbookmarked on X. They stay in the
// library; importing them again clears the mark.
func (s *Store) MarkRemovedOnX(ctx context.Context, postIDs []string) (int, error) {
	marked := 0
	for _, id := range postIDs {
		if !postIDPattern.MatchString(id) {
			continue
		}
		result, err := s.DB.ExecContext(ctx, `UPDATE bookmarks SET removed_on_x_at=CURRENT_TIMESTAMP WHERE platform='x' AND post_id=? AND removed_on_x_at IS NULL`, id)
		if err != nil {
			return marked, err
		}
		count, _ := result.RowsAffected()
		marked += int(count)
	}
	return marked, nil
}

func replaceMedia(ctx context.Context, tx *sql.Tx, bookmarkID int64, items []model.ImportedMedia) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM media WHERE bookmark_id=?`, bookmarkID); err != nil {
		return err
	}
	for position, item := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO media(bookmark_id,kind,url,preview_url,alt_text,width,height,position,video_url) VALUES (?,?,?,?,?,?,?,?,?)`, bookmarkID, item.Kind, item.URL, item.PreviewURL, item.AltText, item.Width, item.Height, position, item.VideoURL); err != nil {
			return err
		}
	}
	return nil
}

func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreatePairingCode(ctx context.Context, code string, expires time.Time) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO pairing_codes(code_hash,expires_at) VALUES (?,?)`, HashSecret(code), expires.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) PairExtension(ctx context.Context, code, token string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE pairing_codes SET used_at=CURRENT_TIMESTAMP WHERE code_hash=? AND used_at IS NULL AND expires_at > ?`, HashSecret(code), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("pairing code is invalid or expired")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO extension_tokens(token_hash) VALUES (?)`, HashSecret(token)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AuthenticateExtension(ctx context.Context, token string) bool {
	if token == "" {
		return false
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE extension_tokens SET last_used_at=CURRENT_TIMESTAMP WHERE token_hash=? AND revoked_at IS NULL`, HashSecret(token))
	if err != nil {
		return false
	}
	count, _ := result.RowsAffected()
	return count == 1
}

func (s *Store) RevokeExtension(ctx context.Context, token string) error {
	if token == "" {
		return errors.New("extension token is required")
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE extension_tokens SET revoked_at=CURRENT_TIMESTAMP WHERE token_hash=? AND revoked_at IS NULL`, HashSecret(token))
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// ftsExpression turns free text into an FTS5 query where every word must match.
// Words are split the way the index tokenizes them, so "c++" becomes "c" and
// "next.js" becomes "next" + "js"; punctuation-only input is ignored instead of
// making the whole search match nothing. Single characters match exactly rather
// than as a prefix of every word.
func ftsExpression(input string) string {
	parts := []string{}
	var word strings.Builder
	flush := func() {
		if word.Len() == 0 {
			return
		}
		token := word.String()
		word.Reset()
		if len([]rune(token)) == 1 {
			parts = append(parts, `"`+token+`"`)
			return
		}
		parts = append(parts, `"`+token+`"*`)
	}
	for _, char := range input {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || unicode.Is(unicode.Mn, char) {
			word.WriteRune(char)
			continue
		}
		flush()
	}
	flush()
	return strings.Join(parts, " AND ")
}

// needsReviewCondition matches analyzed bookmarks with no category the AI was at
// least 50% sure of and none added by hand.
const needsReviewCondition = `b.processing_status='completed' AND NOT EXISTS (SELECT 1 FROM bookmark_categories bc WHERE bc.bookmark_id=b.id AND bc.manual_state <> 'removed' AND (bc.manual_state='added' OR bc.ai_confidence >= 0.5))`

// bookmarkColumns is what scanBookmark reads, from bookmarks b joined to
// enrichments e.
const bookmarkColumns = `b.id,b.post_id,b.author,b.username,b.text,b.url,b.posted_at,b.imported_at,b.language,b.visible_context_json,COALESCE(e.manual_summary,e.ai_summary,''),COALESCE(e.media_description,''),b.processing_status,b.archived_at,b.removed_on_x_at,` + avatarColumn

// bookmarkFilter is a BookmarkQuery as SQL: what to join, the conditions,
// and their arguments in order.
type bookmarkFilter struct {
	joins      string
	conditions []string
	args       []any
	// search is the full-text expression, or "" when no words are searched for.
	search string
}

// filterFor builds the filter. With words false the text search is left out,
// which is how posts found by meaning are checked against the other filters.
func filterFor(query model.BookmarkQuery, words bool) bookmarkFilter {
	filter := bookmarkFilter{joins: " LEFT JOIN enrichments e ON e.bookmark_id=b.id"}
	if query.ListID > 0 {
		filter.joins += " JOIN list_items li ON li.bookmark_id=b.id AND li.list_id=?"
		filter.args = append(filter.args, query.ListID)
	}
	add := func(condition string, args ...any) {
		filter.conditions = append(filter.conditions, condition)
		filter.args = append(filter.args, args...)
	}
	if author := strings.TrimPrefix(strings.TrimSpace(query.Author), "@"); author != "" {
		add("b.username=? COLLATE NOCASE", author)
	}
	if query.NoList {
		// Starring is not filing: a post only in Favorites still counts as in no list.
		add("NOT EXISTS (SELECT 1 FROM list_items nl JOIN lists l ON l.id=nl.list_id WHERE nl.bookmark_id=b.id AND l.kind='custom')")
	}
	if words {
		if filter.search = ftsExpression(query.Text); filter.search != "" {
			filter.joins += " JOIN bookmark_fts f ON f.bookmark_id=b.id"
			add("bookmark_fts MATCH ?", filter.search)
		}
	}
	if !query.IncludeArchived {
		add("b.archived_at IS NULL")
	}
	if query.ProcessingState == "review" {
		add(needsReviewCondition)
	} else if query.ProcessingState != "" {
		add("b.processing_status=?", query.ProcessingState)
	}
	for _, id := range query.CategoryIDs {
		add(`EXISTS (SELECT 1 FROM bookmark_categories bc WHERE bc.bookmark_id=b.id AND bc.category_id=? AND bc.manual_state <> 'removed' AND (bc.manual_state='added' OR bc.ai_confidence IS NOT NULL))`, id)
	}
	for _, id := range query.TagIDs {
		add(`EXISTS (SELECT 1 FROM bookmark_tags bt WHERE bt.bookmark_id=b.id AND bt.tag_id=? AND bt.manual_state <> 'removed' AND (bt.manual_state='added' OR bt.ai_confidence IS NOT NULL))`, id)
	}
	return filter
}

func (f bookmarkFilter) where() string {
	if len(f.conditions) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(f.conditions, " AND ")
}

// among narrows the filter to the given posts.
func (f bookmarkFilter) among(ids []int64) bookmarkFilter {
	f.conditions = append(append([]string{}, f.conditions...), "b.id IN (?"+strings.Repeat(",?", len(ids)-1)+")")
	f.args = append([]any{}, f.args...)
	for _, id := range ids {
		f.args = append(f.args, id)
	}
	return f
}

// sortOrder is the ORDER BY for a query; ranked says whether the words
// searched for can order the results.
func sortOrder(query model.BookmarkQuery, ranked bool) string {
	switch query.Sort {
	case "posted_desc":
		return "b.posted_at IS NULL,b.posted_at DESC,b.id DESC"
	case "posted_asc":
		return "b.posted_at IS NULL,b.posted_at ASC,b.id ASC"
	case "imported_asc":
		return "b.imported_at ASC,b.id ASC"
	case "imported_desc":
		return "b.imported_at DESC,b.id DESC"
	case "added_desc":
		if query.ListID > 0 {
			return "li.added_at DESC,b.id DESC"
		}
		return "b.imported_at DESC,b.id DESC"
	}
	if ranked {
		return "bm25(bookmark_fts),b.imported_at DESC"
	}
	return "b.imported_at DESC,b.id DESC"
}

func normalizePaging(query *model.BookmarkQuery) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 24
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}
}

func (s *Store) SearchBookmarks(ctx context.Context, query model.BookmarkQuery) (model.BookmarkPage, error) {
	normalizePaging(&query)
	if query.Hybrid {
		return s.searchHybrid(ctx, query)
	}
	filter := filterFor(query, true)
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT b.id) FROM bookmarks b`+filter.joins+filter.where(), filter.args...).Scan(&total); err != nil {
		return model.BookmarkPage{}, err
	}
	args := append(append([]any{}, filter.args...), query.PageSize, (query.Page-1)*query.PageSize)
	items, err := s.bookmarkRows(ctx, `SELECT `+bookmarkColumns+` FROM bookmarks b`+filter.joins+filter.where()+` ORDER BY `+sortOrder(query, filter.search != "")+` LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return model.BookmarkPage{}, err
	}
	return model.BookmarkPage{Items: items, Page: query.Page, PageSize: query.PageSize, Total: total}, nil
}

// CountBookmarks returns how many posts a query matches.
func (s *Store) CountBookmarks(ctx context.Context, query model.BookmarkQuery) (int, error) {
	if query.Hybrid {
		ids, _, err := s.hybridIDs(ctx, query)
		return len(ids), err
	}
	filter := filterFor(query, true)
	var total int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT b.id) FROM bookmarks b`+filter.joins+filter.where(), filter.args...).Scan(&total)
	return total, err
}

// bookmarkRows runs a query selecting bookmarkColumns and loads each post's
// media, lists, categories, and tags.
func (s *Store) bookmarkRows(ctx context.Context, statement string, args ...any) ([]model.Bookmark, error) {
	rows, err := s.DB.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	items := []model.Bookmark{}
	for rows.Next() {
		item, err := scanBookmark(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for index := range items {
		if err := s.loadBookmarkRelations(ctx, &items[index]); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// rankConstant is the usual constant for reciprocal rank fusion: it keeps
// the top of one ranking from drowning out the other.
const rankConstant = 60

// hybridIDs returns the posts a search by meaning finds, best first: those
// containing the words and those close in meaning, each ranked in its own
// way and then merged by reciprocal rank, so a post high in both comes
// first. byMeaning holds the posts that do not contain the words.
func (s *Store) hybridIDs(ctx context.Context, query model.BookmarkQuery) (ids []int64, byMeaning map[int64]bool, err error) {
	score := map[int64]float64{}
	byMeaning = map[int64]bool{}
	filter := filterFor(query, true)
	if filter.search != "" {
		found, err := s.idRows(ctx, `SELECT b.id FROM bookmarks b`+filter.joins+filter.where()+` ORDER BY bm25(bookmark_fts),b.imported_at DESC`, filter.args...)
		if err != nil {
			return nil, nil, err
		}
		for index, id := range found {
			score[id] += 1 / float64(rankConstant+index+1)
		}
	}
	if len(query.Semantic) > 0 {
		// Only the posts that pass the other filters count, ranked among themselves.
		within := filterFor(query, false).among(query.Semantic)
		found, err := s.idRows(ctx, `SELECT b.id FROM bookmarks b`+within.joins+within.where(), within.args...)
		if err != nil {
			return nil, nil, err
		}
		allowed := map[int64]bool{}
		for _, id := range found {
			allowed[id] = true
		}
		rank := 0
		for _, id := range query.Semantic {
			if !allowed[id] {
				continue
			}
			allowed[id] = false
			rank++
			if _, words := score[id]; !words {
				byMeaning[id] = true
			}
			score[id] += 1 / float64(rankConstant+rank)
		}
	}
	ids = make([]int64, 0, len(score))
	for id := range score {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if score[ids[i]] != score[ids[j]] {
			return score[ids[i]] > score[ids[j]]
		}
		return ids[i] > ids[j]
	})
	return ids, byMeaning, nil
}

func (s *Store) idRows(ctx context.Context, statement string, args ...any) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) searchHybrid(ctx context.Context, query model.BookmarkQuery) (model.BookmarkPage, error) {
	result := model.BookmarkPage{Items: []model.Bookmark{}, Page: query.Page, PageSize: query.PageSize}
	ids, byMeaning, err := s.hybridIDs(ctx, query)
	if err != nil {
		return result, err
	}
	result.Total = len(ids)
	offset := (query.Page - 1) * query.PageSize
	if offset >= len(ids) {
		return result, nil
	}
	filter := filterFor(query, false)
	if query.Sort == "" || query.Sort == "relevance" {
		// Best matches first: fetch this page's posts and put them in rank order.
		page := ids[offset:min(offset+query.PageSize, len(ids))]
		within := filter.among(page)
		items, err := s.bookmarkRows(ctx, `SELECT `+bookmarkColumns+` FROM bookmarks b`+within.joins+within.where(), within.args...)
		if err != nil {
			return result, err
		}
		position := map[int64]int{}
		for index, id := range page {
			position[id] = index
		}
		sort.Slice(items, func(i, j int) bool { return position[items[i].ID] < position[items[j].ID] })
		result.Items = items
	} else {
		// Another order: the same posts, sorted by the database. They are
		// named as "has the words, or is one of those found by meaning",
		// which stays short however many posts have the words.
		within := filter
		meaningOnly := make([]int64, 0, len(byMeaning))
		for id := range byMeaning {
			meaningOnly = append(meaningOnly, id)
		}
		search := ftsExpression(query.Text)
		switch {
		case search != "" && len(meaningOnly) > 0:
			found := filter.among(meaningOnly)
			condition := found.conditions[len(found.conditions)-1]
			within.conditions = append(append([]string{}, filter.conditions...), "(b.id IN (SELECT bookmark_id FROM bookmark_fts WHERE bookmark_fts MATCH ?) OR "+condition+")")
			within.args = append(append(append([]any{}, filter.args...), search), found.args[len(filter.args):]...)
		case search != "":
			within.conditions = append(append([]string{}, filter.conditions...), "b.id IN (SELECT bookmark_id FROM bookmark_fts WHERE bookmark_fts MATCH ?)")
			within.args = append(append([]any{}, filter.args...), search)
		default:
			within = filter.among(ids)
		}
		args := append(append([]any{}, within.args...), query.PageSize, offset)
		if result.Items, err = s.bookmarkRows(ctx, `SELECT `+bookmarkColumns+` FROM bookmarks b`+within.joins+within.where()+` ORDER BY `+sortOrder(query, false)+` LIMIT ? OFFSET ?`, args...); err != nil {
			return result, err
		}
	}
	for index := range result.Items {
		if byMeaning[result.Items[index].ID] {
			result.Items[index].Match = "meaning"
		}
	}
	return result, nil
}

type rowScanner interface{ Scan(...any) error }

func scanBookmark(row rowScanner) (model.Bookmark, error) {
	var item model.Bookmark
	var posted, language, archived, removed sql.NullString
	var visible, avatar string
	err := row.Scan(&item.ID, &item.PostID, &item.Author, &item.Username, &item.Text, &item.URL, &posted, &item.ImportedAt, &language, &visible, &item.Summary, &item.MediaDescription, &item.ProcessingStatus, &archived, &removed, &avatar)
	if err != nil {
		return item, err
	}
	item.AuthorAvatar = AvatarPath(avatar)
	if posted.Valid {
		item.PostedAt = &posted.String
	}
	if language.Valid {
		item.Language = &language.String
	}
	item.Archived = archived.Valid
	if removed.Valid {
		item.RemovedOnXAt = &removed.String
	}
	_ = json.Unmarshal([]byte(visible), &item.VisibleContext)
	item.Media = []model.ImportedMedia{}
	item.Categories = []model.TaxonomyItem{}
	item.Tags = []model.TaxonomyItem{}
	item.Lists = []int64{}
	return item, nil
}

func (s *Store) loadBookmarkRelations(ctx context.Context, item *model.Bookmark) error {
	mediaRows, err := s.DB.QueryContext(ctx, `SELECT kind,url,preview_url,alt_text,width,height,video_url FROM media WHERE bookmark_id=? ORDER BY position`, item.ID)
	if err != nil {
		return err
	}
	for mediaRows.Next() {
		var media model.ImportedMedia
		var width, height sql.NullInt64
		if err := mediaRows.Scan(&media.Kind, &media.URL, &media.PreviewURL, &media.AltText, &width, &height, &media.VideoURL); err != nil {
			mediaRows.Close()
			return err
		}
		if width.Valid {
			value := int(width.Int64)
			media.Width = &value
		}
		if height.Valid {
			value := int(height.Int64)
			media.Height = &value
		}
		item.Media = append(item.Media, media)
	}
	mediaRows.Close()
	listRows, err := s.DB.QueryContext(ctx, `SELECT list_id FROM list_items WHERE bookmark_id=? ORDER BY list_id`, item.ID)
	if err != nil {
		return err
	}
	for listRows.Next() {
		var id int64
		if err := listRows.Scan(&id); err != nil {
			listRows.Close()
			return err
		}
		item.Lists = append(item.Lists, id)
	}
	listRows.Close()
	taxonomyRows, err := s.DB.QueryContext(ctx, `SELECT 'category',c.id,c.name,bc.ai_confidence,bc.manual_state FROM bookmark_categories bc JOIN categories c ON c.id=bc.category_id WHERE bc.bookmark_id=? AND bc.manual_state <> 'removed' AND (bc.manual_state='added' OR bc.ai_confidence IS NOT NULL) UNION ALL SELECT 'tag',t.id,t.name,bt.ai_confidence,bt.manual_state FROM bookmark_tags bt JOIN tags t ON t.id=bt.tag_id WHERE bt.bookmark_id=? AND bt.manual_state <> 'removed' AND (bt.manual_state='added' OR bt.ai_confidence IS NOT NULL) ORDER BY 1,3`, item.ID, item.ID)
	if err != nil {
		return err
	}
	defer taxonomyRows.Close()
	for taxonomyRows.Next() {
		var kind, state string
		var entry model.TaxonomyItem
		var confidence sql.NullFloat64
		if err := taxonomyRows.Scan(&kind, &entry.ID, &entry.Name, &confidence, &state); err != nil {
			return err
		}
		if confidence.Valid {
			entry.Confidence = &confidence.Float64
		}
		entry.Manual = state != "automatic"
		if kind == "category" {
			item.Categories = append(item.Categories, entry)
		} else {
			item.Tags = append(item.Tags, entry)
		}
	}
	return taxonomyRows.Err()
}

func (s *Store) GetBookmark(ctx context.Context, id int64) (model.Bookmark, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+bookmarkColumns+` FROM bookmarks b LEFT JOIN enrichments e ON e.bookmark_id=b.id WHERE b.id=?`, id)
	item, err := scanBookmark(row)
	if err != nil {
		return item, err
	}
	err = s.loadBookmarkRelations(ctx, &item)
	return item, err
}

func (s *Store) SetArchived(ctx context.Context, id int64, archived bool) (model.Bookmark, error) {
	value := any(nil)
	if archived {
		value = time.Now().UTC().Format(time.RFC3339)
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE bookmarks SET archived_at=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, value, id)
	if err != nil {
		return model.Bookmark{}, err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return model.Bookmark{}, sql.ErrNoRows
	}
	return s.GetBookmark(ctx, id)
}

// DeleteBookmark removes a bookmark and remembers its post, so a later sync
// does not bring it back while it is still bookmarked on X.
func (s *Store) DeleteBookmark(ctx context.Context, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO deleted_posts(platform,post_id) SELECT platform,post_id FROM bookmarks WHERE id=?`, id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM bookmarks WHERE id=?`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (s *Store) ListCategories(ctx context.Context) ([]model.Category, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id,c.name,c.parent_id,c.description,c.active,COUNT(CASE WHEN bc.manual_state <> 'removed' AND (bc.manual_state='added' OR bc.ai_confidence IS NOT NULL) THEN 1 END) FROM categories c LEFT JOIN bookmark_categories bc ON bc.category_id=c.id GROUP BY c.id ORDER BY c.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Category{}
	for rows.Next() {
		var item model.Category
		var parent sql.NullInt64
		if err := rows.Scan(&item.ID, &item.Name, &parent, &item.Description, &item.Active, &item.Count); err != nil {
			return nil, err
		}
		if parent.Valid {
			item.ParentID = &parent.Int64
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ListTags(ctx context.Context) ([]model.Tag, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT t.id,t.name,t.kind,COUNT(CASE WHEN bt.manual_state <> 'removed' AND (bt.manual_state='added' OR bt.ai_confidence IS NOT NULL) THEN 1 END) FROM tags t LEFT JOIN bookmark_tags bt ON bt.tag_id=t.id GROUP BY t.id ORDER BY 4 DESC,t.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	result := []model.Tag{}
	for rows.Next() {
		var item model.Tag
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.Count); err != nil {
			return nil, err
		}
		item.Aliases = []string{}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for index := range result {
		aliasRows, err := s.DB.QueryContext(ctx, `SELECT alias FROM tag_aliases WHERE tag_id=? ORDER BY alias COLLATE NOCASE`, result[index].ID)
		if err != nil {
			return nil, err
		}
		for aliasRows.Next() {
			var alias string
			if err := aliasRows.Scan(&alias); err != nil {
				aliasRows.Close()
				return nil, err
			}
			result[index].Aliases = append(result[index].Aliases, alias)
		}
		aliasRows.Close()
	}
	return result, nil
}
