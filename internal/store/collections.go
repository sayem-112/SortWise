package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"sortwise/internal/model"
)

// ErrCollectionExists is returned when a smart collection with the same
// name already exists.
var ErrCollectionExists = errors.New("a smart collection with this name already exists")

// ErrCollectionEmpty is returned when a smart collection has no search and
// no filter, and so would be the whole library.
var ErrCollectionEmpty = errors.New("add a search or a filter first")

var (
	collectionStatuses = []string{"", "pending", "processing", "completed", "failed", "blocked", "review"}
	collectionSorts    = []string{"", "relevance", "imported_desc", "imported_asc", "posted_desc", "posted_asc"}
	handlePattern      = regexp.MustCompile(`^[A-Za-z0-9_]{1,30}$`)
)

const (
	maxCollectionText   = 200
	maxCollectionFilter = 20
)

// positiveIDs returns the distinct IDs above zero, in order.
func positiveIDs(ids []int64) []int64 {
	result := []int64{}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

// cleanCollectionQuery checks a saved search and puts it in one form, so
// the same search is always stored the same way.
func cleanCollectionQuery(query model.CollectionQuery) (model.CollectionQuery, error) {
	query.Text = strings.Join(strings.Fields(query.Text), " ")
	if len([]rune(query.Text)) > maxCollectionText {
		return query, errors.New("the search is too long")
	}
	if query.Mode != "meaning" || query.Text == "" {
		query.Mode = ""
	}
	query.Categories = positiveIDs(query.Categories)
	query.Tags = positiveIDs(query.Tags)
	if len(query.Categories) > maxCollectionFilter || len(query.Tags) > maxCollectionFilter {
		return query, errors.New("too many categories or tags")
	}
	if !oneOf(query.Status, collectionStatuses) {
		return query, errors.New("unknown status")
	}
	if !oneOf(query.Sort, collectionSorts) {
		return query, errors.New("unknown sort order")
	}
	if query.Sort == "relevance" && query.Text == "" {
		query.Sort = ""
	}
	if query.List != "" && query.List != "none" {
		if id, err := strconv.ParseInt(query.List, 10, 64); err != nil || id < 1 {
			return query, errors.New("unknown list")
		}
	}
	query.Author = strings.TrimPrefix(strings.TrimSpace(query.Author), "@")
	if query.Author != "" && !handlePattern.MatchString(query.Author) {
		return query, errors.New("unknown author")
	}
	if query.Text == "" && len(query.Categories) == 0 && len(query.Tags) == 0 && query.Status == "" && query.List == "" && query.Author == "" {
		return query, ErrCollectionEmpty
	}
	return query, nil
}

// CollectionBookmarkQuery turns a saved search into the query the library
// runs. Posts close in meaning are added by the caller, which has the index.
func CollectionBookmarkQuery(query model.CollectionQuery) model.BookmarkQuery {
	result := model.BookmarkQuery{Text: query.Text, CategoryIDs: query.Categories, TagIDs: query.Tags, ProcessingState: query.Status, Author: query.Author, Sort: query.Sort}
	if query.List == "none" {
		result.NoList = true
	} else if id, err := strconv.ParseInt(query.List, 10, 64); err == nil {
		result.ListID = id
	}
	return result
}

func scanCollection(row rowScanner) (model.Collection, error) {
	var item model.Collection
	var query string
	if err := row.Scan(&item.ID, &item.Name, &item.Color, &query); err != nil {
		return item, err
	}
	_ = json.Unmarshal([]byte(query), &item.Query)
	if item.Query.Categories == nil {
		item.Query.Categories = []int64{}
	}
	if item.Query.Tags == nil {
		item.Query.Tags = []int64{}
	}
	return item, nil
}

// Collections returns the smart collections by name. Counts are filled in
// by the caller.
func (s *Store) Collections(ctx context.Context) ([]model.Collection, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,name,color,query_json FROM collections ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Collection{}
	for rows.Next() {
		item, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) GetCollection(ctx context.Context, id int64) (model.Collection, error) {
	return scanCollection(s.DB.QueryRowContext(ctx, `SELECT id,name,color,query_json FROM collections WHERE id=?`, id))
}

func (s *Store) CreateCollection(ctx context.Context, name string, query model.CollectionQuery) (model.Collection, error) {
	name, normalized, err := validateName(name)
	if err != nil {
		return model.Collection{}, err
	}
	if query, err = cleanCollectionQuery(query); err != nil {
		return model.Collection{}, err
	}
	// Each new collection takes the next color, so neighbours in the sidebar differ.
	var count int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM collections`).Scan(&count); err != nil {
		return model.Collection{}, err
	}
	// Starting where lists do not, so the first collection and the first list differ.
	color := ListColors[(count+1)%len(ListColors)]
	// A deleted collection's number is never handed out again, so an old
	// link can't open a different one.
	var highest int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM collections`).Scan(&highest); err != nil {
		return model.Collection{}, err
	}
	if used, _ := strconv.ParseInt(s.setting(ctx, "collection_last_id"), 10, 64); used > highest {
		highest = used
	}
	id := highest + 1
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO collections(id,name,normalized_name,color,query_json) VALUES (?,?,?,?,?)`, id, name, normalized, color, model.RawJSON(query)); err != nil {
		return model.Collection{}, collectionError(err)
	}
	if err := s.setSetting(ctx, "collection_last_id", strconv.FormatInt(id, 10)); err != nil {
		return model.Collection{}, err
	}
	return s.GetCollection(ctx, id)
}

// UpdateCollection renames a collection, recolors it, or replaces its search.
func (s *Store) UpdateCollection(ctx context.Context, id int64, input model.CollectionUpdate) (model.Collection, error) {
	current, err := s.GetCollection(ctx, id)
	if err != nil {
		return model.Collection{}, err
	}
	// Everything is checked before anything is written, so a change that is
	// partly wrong changes nothing.
	name, color, query := current.Name, current.Color, current.Query
	normalized := normalizeTaxonomy(name)
	if input.Name != nil {
		if name, normalized, err = validateName(*input.Name); err != nil {
			return model.Collection{}, err
		}
	}
	if input.Color != nil {
		if color = *input.Color; !oneOf(color, ListColors) {
			return model.Collection{}, errors.New("unknown color")
		}
	}
	if input.Query != nil {
		if query, err = cleanCollectionQuery(*input.Query); err != nil {
			return model.Collection{}, err
		}
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE collections SET name=?,normalized_name=?,color=?,query_json=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, name, normalized, color, model.RawJSON(query), id); err != nil {
		return model.Collection{}, collectionError(err)
	}
	return s.GetCollection(ctx, id)
}

func (s *Store) DeleteCollection(ctx context.Context, id int64) error {
	result, err := s.DB.ExecContext(ctx, `DELETE FROM collections WHERE id=?`, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func collectionError(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return ErrCollectionExists
	}
	return err
}

// runner is what both a transaction and the database itself offer.
type runner interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// repointCollections keeps smart collections working when a category or tag
// is merged into another: every saved search that filtered on the old one
// filters on the new one instead. kind is "category" or "tag". With to 0
// the old one is simply taken out, for a tag that is deleted: its number
// can be given to a new tag, which the saved search must not then follow.
// A collection left with no search or filter at all is deleted.
func repointCollections(ctx context.Context, tx runner, kind string, from, to int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,query_json FROM collections`)
	if err != nil {
		return err
	}
	changed := map[int64]string{}
	for rows.Next() {
		var id int64
		var body string
		if err := rows.Scan(&id, &body); err != nil {
			rows.Close()
			return err
		}
		var query model.CollectionQuery
		if json.Unmarshal([]byte(body), &query) != nil {
			continue
		}
		ids := &query.Tags
		if kind == "category" {
			ids = &query.Categories
		}
		found := false
		for index, value := range *ids {
			if value == from {
				(*ids)[index] = to
				found = true
			}
		}
		if found {
			*ids = positiveIDs(*ids)
			changed[id] = model.RawJSON(query)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for id, body := range changed {
		var query model.CollectionQuery
		_ = json.Unmarshal([]byte(body), &query)
		statement, args := `UPDATE collections SET query_json=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, []any{body, id}
		if _, err := cleanCollectionQuery(query); errors.Is(err, ErrCollectionEmpty) {
			statement, args = `DELETE FROM collections WHERE id=?`, []any{id}
		}
		if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
			return err
		}
	}
	return nil
}
