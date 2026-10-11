package store

import (
	"context"
	"database/sql"
	"strings"

	"sortwise/internal/model"
)

// Authors are the people whose posts are saved, keyed by X handle. Handles are
// compared without regard to case, and an author's name and the spelling of
// their handle are the ones on their most recently saved post. Archived
// bookmarks are left out, as in the library.

const authorBase = ` FROM bookmarks b WHERE b.archived_at IS NULL AND b.username <> ''`

// latestOf picks a column from the author's most recently saved post, the
// same way everywhere, so the list and the author's page never disagree.
func latestOf(column string) string {
	return `(SELECT x.` + column + ` FROM bookmarks x WHERE x.username=b.username COLLATE NOCASE AND x.archived_at IS NULL ORDER BY x.imported_at DESC,x.id DESC LIMIT 1)`
}

// likePattern makes user text safe inside LIKE, matching it anywhere.
func likePattern(text string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text) + "%"
}

// authorMatch narrows authors to those with any post whose name or handle
// matches the text. It matches whole authors, not single posts, so someone
// who changed their display name keeps their full count. The search index
// finds words whatever their case or accents, in any script; LIKE adds
// matches from the middle of a handle or name.
func authorMatch(text string) (string, []any) {
	condition := ` AND b.username COLLATE NOCASE IN (SELECT m.username FROM bookmarks m WHERE m.username LIKE ? ESCAPE '\' OR m.author LIKE ? ESCAPE '\'`
	args := []any{likePattern(text), likePattern(text)}
	if search := ftsExpression(text); search != "" {
		condition += ` UNION SELECT username FROM bookmark_fts WHERE bookmark_fts MATCH ?`
		args = append(args, `{author username} : (`+search+`)`)
	}
	return condition + `)`, args
}

// Authors lists authors with how many of their posts are saved. Text filters
// by name or handle. Sort is "count" (default), "recent", or "name".
func (s *Store) Authors(ctx context.Context, query model.AuthorQuery) (model.AuthorPage, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 200 {
		query.PageSize = 100
	}
	result := model.AuthorPage{Items: []model.Author{}, Page: query.Page, PageSize: query.PageSize}
	where := authorBase
	args := []any{}
	if typed := strings.TrimSpace(query.Text); typed != "" {
		text := strings.TrimPrefix(typed, "@")
		if text == "" {
			// A lone "@" is the start of a handle, not a request for everyone.
			return result, nil
		}
		condition, matchArgs := authorMatch(text)
		where += condition
		args = append(args, matchArgs...)
	}
	// Grouping on the handle's own collation lets the handle index do the work.
	const group = ` GROUP BY b.username COLLATE NOCASE`
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT 1`+where+group+`)`, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	// Every order ends on the handle, so pages never repeat or skip an author.
	order := "3 DESC,4 DESC,b.username COLLATE NOCASE"
	switch query.Sort {
	case "recent":
		order = "4 DESC,3 DESC,b.username COLLATE NOCASE"
	case "name":
		order = "2 COLLATE NOCASE,b.username COLLATE NOCASE"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+latestOf("username")+`,`+latestOf("author")+`,COUNT(*),MAX(b.imported_at),`+avatarColumn+where+group+` ORDER BY `+order+` LIMIT ? OFFSET ?`,
		append(args, query.PageSize, (query.Page-1)*query.PageSize)...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item model.Author
		var avatar string
		if err := rows.Scan(&item.Username, &item.Name, &item.Count, &item.LatestSavedAt, &avatar); err != nil {
			return result, err
		}
		item.Avatar = AvatarPath(avatar)
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

// AuthorProfile describes one author for the top of their page: counts, when
// their posts were written and saved, and the categories and tags their posts
// carry most.
func (s *Store) AuthorProfile(ctx context.Context, username string) (model.AuthorProfile, error) {
	var profile model.AuthorProfile
	var latest, firstPosted, latestPosted sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*),MAX(b.imported_at),MIN(b.posted_at),MAX(b.posted_at)`+authorBase+` AND b.username=? COLLATE NOCASE`, username).
		Scan(&profile.Count, &latest, &firstPosted, &latestPosted)
	if err != nil {
		return profile, err
	}
	if profile.Count == 0 {
		return profile, sql.ErrNoRows
	}
	profile.LatestSavedAt, profile.FirstPostedAt, profile.LatestPostedAt = latest.String, firstPosted.String, latestPosted.String
	var avatar string
	if err := s.DB.QueryRowContext(ctx, `SELECT b.username,b.author,`+avatarColumn+authorBase+` AND b.username=? COLLATE NOCASE ORDER BY b.imported_at DESC,b.id DESC LIMIT 1`, username).Scan(&profile.Username, &profile.Name, &avatar); err != nil {
		return profile, err
	}
	profile.Avatar = AvatarPath(avatar)

	top := func(table, link, column string) ([]model.NamedCount, error) {
		rows, err := s.DB.QueryContext(ctx, `SELECT t.id,t.name,COUNT(*) FROM `+link+` x JOIN `+table+` t ON t.id=x.`+column+` JOIN bookmarks b ON b.id=x.bookmark_id WHERE b.archived_at IS NULL AND b.username=? COLLATE NOCASE AND x.manual_state <> 'removed' AND (x.manual_state='added' OR x.ai_confidence IS NOT NULL) GROUP BY t.id ORDER BY 3 DESC,t.name COLLATE NOCASE LIMIT 6`, username)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		items := []model.NamedCount{}
		for rows.Next() {
			var item model.NamedCount
			if err := rows.Scan(&item.ID, &item.Name, &item.Count); err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, rows.Err()
	}
	if profile.Categories, err = top("categories", "bookmark_categories", "category_id"); err != nil {
		return profile, err
	}
	if profile.TopTags, err = top("tags", "bookmark_tags", "tag_id"); err != nil {
		return profile, err
	}
	return profile, nil
}

// Markers around the matched words in a search excerpt; the web app turns
// them into highlights.
const (
	MatchStart = "\x02"
	MatchEnd   = "\x03"
)

// marked keeps an excerpt only when the match is actually in it.
func marked(excerpt string) string {
	if strings.Contains(excerpt, MatchStart) {
		return excerpt
	}
	return ""
}

// QuickSearch is the finder's search: the best-matching posts with a short
// excerpt of where each one matched, and the authors whose name or handle
// matches. It reads far less per post than the library search.
func (s *Store) QuickSearch(ctx context.Context, text string, limit int) (model.QuickResults, error) {
	result := model.QuickResults{Bookmarks: []model.QuickBookmark{}, Authors: []model.Author{}}
	if limit < 1 || limit > 20 {
		limit = 8
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return result, nil
	}
	if search := ftsExpression(text); search != "" {
		const from = ` FROM bookmark_fts JOIN bookmarks b ON b.id=bookmark_fts.bookmark_id WHERE bookmark_fts MATCH ? AND b.archived_at IS NULL`
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*)`+from, search).Scan(&result.Total); err != nil {
			return result, err
		}
		// Index columns: 1 post text, 2 summary, 3 author name, 4 handle,
		// 5 what is in the images, 6 tags and categories.
		snippet := func(column, words string) string {
			return `snippet(bookmark_fts,` + column + `,'` + MatchStart + `','` + MatchEnd + `','…',` + words + `)`
		}
		rows, err := s.DB.QueryContext(ctx, `SELECT b.id,b.author,b.username,`+avatarColumn+`,b.posted_at,b.imported_at,`+
			snippet("1", "16")+`,`+snippet("2", "14")+`,`+snippet("5", "14")+`,`+snippet("6", "8")+`,`+snippet("3", "8")+`,`+snippet("4", "4")+
			from+` ORDER BY bm25(bookmark_fts),b.imported_at DESC,b.id DESC LIMIT ?`, search, limit)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var item model.QuickBookmark
			var posted sql.NullString
			var avatar string
			if err := rows.Scan(&item.ID, &item.Author, &item.Username, &avatar, &posted, &item.ImportedAt, &item.Text, &item.Summary, &item.Media, &item.Taxonomy, &item.AuthorMatch, &item.UsernameMatch); err != nil {
				rows.Close()
				return result, err
			}
			if posted.Valid {
				item.PostedAt = &posted.String
			}
			item.Avatar = AvatarPath(avatar)
			// The post text always shows; the others only when they hold the match.
			item.Summary, item.Media, item.Taxonomy = marked(item.Summary), marked(item.Media), marked(item.Taxonomy)
			item.AuthorMatch, item.UsernameMatch = marked(item.AuthorMatch), marked(item.UsernameMatch)
			result.Bookmarks = append(result.Bookmarks, item)
		}
		// Finish reading before the next query: the database has one connection.
		if err := rows.Close(); err != nil {
			return result, err
		}
		if err := rows.Err(); err != nil {
			return result, err
		}
	}
	authors, err := s.Authors(ctx, model.AuthorQuery{Text: text, PageSize: 4})
	if err != nil {
		return result, err
	}
	result.Authors = authors.Items
	return result, nil
}
