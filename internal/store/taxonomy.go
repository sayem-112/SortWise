package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"sortwise/internal/model"
)

func normalizeTaxonomy(value string) string {
	value = strings.ToLower(strings.TrimSpace(norm.NFKC.String(value)))
	var output []rune
	separator := false
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			output = append(output, char)
			separator = false
		} else if !separator && len(output) > 0 {
			output = append(output, '-')
			separator = true
		}
	}
	return strings.Trim(string(output), "-")
}

func validateName(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	normalized := normalizeTaxonomy(value)
	if normalized == "" || len([]rune(value)) > 80 {
		return "", "", errors.New("name must contain 1–80 useful characters")
	}
	return value, normalized, nil
}

func (s *Store) CreateCategory(ctx context.Context, name, description string, parentID *int64) (model.Category, error) {
	name, normalized, err := validateName(name)
	if err != nil {
		return model.Category{}, err
	}
	if parentID != nil {
		var exists int
		if err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories WHERE id=?`, *parentID).Scan(&exists); err != nil || exists != 1 {
			return model.Category{}, errors.New("parent category not found")
		}
	}
	result, err := s.DB.ExecContext(ctx, `INSERT INTO categories(name,normalized_name,parent_id,description) VALUES (?,?,?,?)`, name, normalized, parentID, strings.TrimSpace(description))
	if err != nil {
		return model.Category{}, err
	}
	id, _ := result.LastInsertId()
	return s.GetCategory(ctx, id)
}

func (s *Store) GetCategory(ctx context.Context, id int64) (model.Category, error) {
	var item model.Category
	var parent sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT c.id,c.name,c.parent_id,c.description,c.active,COUNT(CASE WHEN bc.manual_state <> 'removed' AND (bc.manual_state='added' OR bc.ai_confidence IS NOT NULL) THEN 1 END) FROM categories c LEFT JOIN bookmark_categories bc ON bc.category_id=c.id WHERE c.id=? GROUP BY c.id`, id).Scan(&item.ID, &item.Name, &parent, &item.Description, &item.Active, &item.Count)
	if parent.Valid {
		item.ParentID = &parent.Int64
	}
	return item, err
}

func (s *Store) UpdateCategory(ctx context.Context, id int64, name, description string, parentID *int64, active *bool) (model.Category, error) {
	name, normalized, err := validateName(name)
	if err != nil {
		return model.Category{}, err
	}
	if parentID != nil {
		if *parentID == id {
			return model.Category{}, errors.New("a category cannot be its own parent")
		}
		var descendant int
		err = s.DB.QueryRowContext(ctx, `WITH RECURSIVE descendants(id) AS (SELECT id FROM categories WHERE parent_id=? UNION ALL SELECT c.id FROM categories c JOIN descendants d ON c.parent_id=d.id) SELECT COUNT(*) FROM descendants WHERE id=?`, id, *parentID).Scan(&descendant)
		if err != nil {
			return model.Category{}, err
		}
		if descendant > 0 {
			return model.Category{}, errors.New("category hierarchy cannot contain a cycle")
		}
	}
	activeValue := 1
	var current bool
	if err = s.DB.QueryRowContext(ctx, `SELECT active FROM categories WHERE id=?`, id).Scan(&current); err != nil {
		return model.Category{}, err
	}
	if active == nil {
		activeValue = 0
		if current {
			activeValue = 1
		}
	} else if !*active {
		activeValue = 0
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE categories SET name=?,normalized_name=?,parent_id=?,description=?,active=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, name, normalized, parentID, strings.TrimSpace(description), activeValue, id)
	if err != nil {
		return model.Category{}, err
	}
	return s.GetCategory(ctx, id)
}

func (s *Store) MergeCategory(ctx context.Context, sourceID, targetID int64) error {
	if sourceID == targetID {
		return errors.New("source and target must differ")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT bookmark_id,ai_confidence,manual_state FROM bookmark_categories WHERE category_id=?`, sourceID)
	if err != nil {
		return err
	}
	type assignment struct {
		bookmark   int64
		confidence sql.NullFloat64
		state      string
	}
	var values []assignment
	for rows.Next() {
		var value assignment
		if err := rows.Scan(&value.bookmark, &value.confidence, &value.state); err != nil {
			rows.Close()
			return err
		}
		values = append(values, value)
	}
	rows.Close()
	for _, value := range values {
		_, err = tx.ExecContext(ctx, `INSERT INTO bookmark_categories(bookmark_id,category_id,ai_confidence,manual_state) VALUES (?,?,?,?) ON CONFLICT(bookmark_id,category_id) DO UPDATE SET ai_confidence=COALESCE(MAX(bookmark_categories.ai_confidence,excluded.ai_confidence),bookmark_categories.ai_confidence,excluded.ai_confidence),manual_state=CASE WHEN bookmark_categories.manual_state='added' OR excluded.manual_state='added' THEN 'added' WHEN bookmark_categories.manual_state='removed' AND excluded.manual_state='removed' THEN 'removed' ELSE 'automatic' END,updated_at=CURRENT_TIMESTAMP`, value.bookmark, targetID, value.confidence, value.state)
		if err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE categories SET parent_id=? WHERE parent_id=?`, targetID, sourceID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM categories WHERE id=?`, sourceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateTag(ctx context.Context, name, kind string) (model.Tag, error) {
	name, normalized, err := validateName(name)
	if err != nil {
		return model.Tag{}, err
	}
	if kind, err = validateTagKind(kind); err != nil {
		return model.Tag{}, err
	}
	result, err := s.DB.ExecContext(ctx, `INSERT INTO tags(name,normalized_name,kind) VALUES (?,?,?)`, name, normalized, kind)
	if err != nil {
		return model.Tag{}, err
	}
	id, _ := result.LastInsertId()
	return s.GetTag(ctx, id)
}

func (s *Store) GetTag(ctx context.Context, id int64) (model.Tag, error) {
	var item model.Tag
	err := s.DB.QueryRowContext(ctx, `SELECT t.id,t.name,t.kind,COUNT(CASE WHEN bt.manual_state <> 'removed' AND (bt.manual_state='added' OR bt.ai_confidence IS NOT NULL) THEN 1 END) FROM tags t LEFT JOIN bookmark_tags bt ON bt.tag_id=t.id WHERE t.id=? GROUP BY t.id`, id).Scan(&item.ID, &item.Name, &item.Kind, &item.Count)
	if err != nil {
		return item, err
	}
	item.Aliases = []string{}
	rows, err := s.DB.QueryContext(ctx, `SELECT alias FROM tag_aliases WHERE tag_id=? ORDER BY alias COLLATE NOCASE`, id)
	if err != nil {
		return item, err
	}
	defer rows.Close()
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return item, err
		}
		item.Aliases = append(item.Aliases, alias)
	}
	return item, rows.Err()
}

func (s *Store) UpdateTag(ctx context.Context, id int64, name, kind string) (model.Tag, error) {
	name, normalized, err := validateName(name)
	if err != nil {
		return model.Tag{}, err
	}
	if kind != "" {
		if kind, err = validateTagKind(kind); err != nil {
			return model.Tag{}, err
		}
	}
	var oldName, oldNormalized string
	if err = s.DB.QueryRowContext(ctx, `SELECT name,normalized_name FROM tags WHERE id=?`, id).Scan(&oldName, &oldNormalized); err != nil {
		return model.Tag{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return model.Tag{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE tags SET name=?,normalized_name=?,kind=COALESCE(NULLIF(?,''),kind),updated_at=CURRENT_TIMESTAMP WHERE id=?`, name, normalized, kind, id); err != nil {
		return model.Tag{}, err
	}
	if oldNormalized != normalized {
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO tag_aliases(tag_id,alias,normalized_alias) VALUES (?,?,?)`, id, oldName, oldNormalized)
		if err != nil {
			return model.Tag{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return model.Tag{}, err
	}
	return s.GetTag(ctx, id)
}

func validateTagKind(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "topic":
		return "topic", nil
	case "tool", "entity", "format":
		return strings.ToLower(strings.TrimSpace(kind)), nil
	}
	return "", errors.New("tag kind must be topic, tool, entity, or format")
}

func (s *Store) AddTagAlias(ctx context.Context, id int64, alias string) (model.Tag, error) {
	alias, normalized, err := validateName(alias)
	if err != nil {
		return model.Tag{}, err
	}
	var canonical string
	if err = s.DB.QueryRowContext(ctx, `SELECT normalized_name FROM tags WHERE id=?`, id).Scan(&canonical); err != nil {
		return model.Tag{}, err
	}
	if canonical == normalized {
		return model.Tag{}, errors.New("alias matches the canonical tag")
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO tag_aliases(tag_id,alias,normalized_alias) VALUES (?,?,?)`, id, alias, normalized)
	if err != nil {
		return model.Tag{}, err
	}
	return s.GetTag(ctx, id)
}

func (s *Store) DeleteTagAlias(ctx context.Context, id int64, alias string) (model.Tag, error) {
	result, err := s.DB.ExecContext(ctx, `DELETE FROM tag_aliases WHERE tag_id=? AND normalized_alias=?`, id, normalizeTaxonomy(alias))
	if err != nil {
		return model.Tag{}, err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return model.Tag{}, sql.ErrNoRows
	}
	return s.GetTag(ctx, id)
}

func (s *Store) DeleteUnusedTag(ctx context.Context, id int64) error {
	result, err := s.DB.ExecContext(ctx, `DELETE FROM tags WHERE id=? AND NOT EXISTS (SELECT 1 FROM bookmark_tags WHERE tag_id=?)`, id, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("tag is in use; merge it instead")
	}
	return nil
}

func (s *Store) MergeTag(ctx context.Context, sourceID, targetID int64) error {
	if sourceID == targetID {
		return errors.New("source and target must differ")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sourceName, sourceNormalized string
	if err = tx.QueryRowContext(ctx, `SELECT name,normalized_name FROM tags WHERE id=?`, sourceID).Scan(&sourceName, &sourceNormalized); err != nil {
		return err
	}
	var targetExists int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tags WHERE id=?`, targetID).Scan(&targetExists); err != nil || targetExists != 1 {
		return errors.New("target tag not found")
	}
	rows, err := tx.QueryContext(ctx, `SELECT bookmark_id,ai_confidence,manual_state FROM bookmark_tags WHERE tag_id=?`, sourceID)
	if err != nil {
		return err
	}
	type assignment struct {
		bookmark   int64
		confidence sql.NullFloat64
		state      string
	}
	var values []assignment
	for rows.Next() {
		var value assignment
		if err := rows.Scan(&value.bookmark, &value.confidence, &value.state); err != nil {
			rows.Close()
			return err
		}
		values = append(values, value)
	}
	rows.Close()
	for _, value := range values {
		_, err = tx.ExecContext(ctx, `INSERT INTO bookmark_tags(bookmark_id,tag_id,ai_confidence,manual_state) VALUES (?,?,?,?) ON CONFLICT(bookmark_id,tag_id) DO UPDATE SET ai_confidence=COALESCE(MAX(bookmark_tags.ai_confidence,excluded.ai_confidence),bookmark_tags.ai_confidence,excluded.ai_confidence),manual_state=CASE WHEN bookmark_tags.manual_state='added' OR excluded.manual_state='added' THEN 'added' WHEN bookmark_tags.manual_state='removed' AND excluded.manual_state='removed' THEN 'removed' ELSE 'automatic' END,updated_at=CURRENT_TIMESTAMP`, value.bookmark, targetID, value.confidence, value.state)
		if err != nil {
			return err
		}
	}
	_, _ = tx.ExecContext(ctx, `INSERT OR IGNORE INTO tag_aliases(tag_id,alias,normalized_alias) VALUES (?,?,?)`, targetID, sourceName, sourceNormalized)
	if _, err = tx.ExecContext(ctx, `UPDATE OR IGNORE tag_aliases SET tag_id=? WHERE tag_id=?`, targetID, sourceID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM tags WHERE id=?`, sourceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateBookmarkMetadata(ctx context.Context, id int64, input model.BookmarkMetadataUpdate) (model.Bookmark, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return model.Bookmark{}, err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM bookmarks WHERE id=?`, id).Scan(&exists); err != nil || exists != 1 {
		return model.Bookmark{}, sql.ErrNoRows
	}
	if input.Summary != nil || input.ResetSummary {
		var summary any = nil
		if input.Summary != nil && !input.ResetSummary {
			value := strings.TrimSpace(*input.Summary)
			if len([]rune(value)) > 800 {
				return model.Bookmark{}, errors.New("summary cannot exceed 800 characters")
			}
			summary = value
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO enrichments(bookmark_id,manual_summary) VALUES (?,?) ON CONFLICT(bookmark_id) DO UPDATE SET manual_summary=excluded.manual_summary,updated_at=CURRENT_TIMESTAMP`, id, summary)
		if err != nil {
			return model.Bookmark{}, err
		}
	}
	if err = applyDecisions(ctx, tx, "bookmark_categories", "category_id", id, input.CategoryDecisions); err != nil {
		return model.Bookmark{}, err
	}
	if err = applyDecisions(ctx, tx, "bookmark_tags", "tag_id", id, input.TagDecisions); err != nil {
		return model.Bookmark{}, err
	}
	if err = tx.Commit(); err != nil {
		return model.Bookmark{}, err
	}
	return s.GetBookmark(ctx, id)
}

func applyDecisions(ctx context.Context, tx *sql.Tx, table, column string, bookmarkID int64, decisions []model.AssignmentDecision) error {
	if table != "bookmark_categories" && table != "bookmark_tags" {
		return errors.New("invalid assignment table")
	}
	for _, decision := range decisions {
		if decision.State != "added" && decision.State != "removed" && decision.State != "automatic" {
			return fmt.Errorf("invalid manual state %q", decision.State)
		}
		query := fmt.Sprintf(`INSERT INTO %s(bookmark_id,%s,manual_state) VALUES (?,?,?) ON CONFLICT(bookmark_id,%s) DO UPDATE SET manual_state=excluded.manual_state,updated_at=CURRENT_TIMESTAMP`, table, column, column)
		if _, err := tx.ExecContext(ctx, query, bookmarkID, decision.ID, decision.State); err != nil {
			return err
		}
	}
	return nil
}
