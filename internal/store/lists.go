package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"sortwise/internal/model"
)

// ErrFavorites is returned when renaming or deleting the built-in Favorites list.
var ErrFavorites = errors.New("Favorites can't be renamed or deleted")

// ErrListExists is returned when a list with the same name already exists.
var ErrListExists = errors.New("a list with this name already exists")

// ListIcons are the icons a list can have; the web app draws each one.
var ListIcons = []string{"list", "bookmark", "book-open", "lightbulb", "code", "palette", "chef-hat", "dumbbell", "wallet", "briefcase", "graduation-cap", "flask", "music", "film", "plane", "heart", "rocket", "target", "newspaper", "sparkles"}

// ListColors are the select colors a list can have, in the order new lists
// take them.
var ListColors = []string{"purple", "blue", "green", "orange", "pink", "brown", "red", "gray", "yellow"}

func oneOf(value string, allowed []string) bool {
	for _, item := range allowed {
		if item == value {
			return true
		}
	}
	return false
}

// listCount counts the bookmarks in a list the way the library shows them:
// archived bookmarks are left out.
const listCount = `(SELECT COUNT(*) FROM list_items li JOIN bookmarks b ON b.id=li.bookmark_id WHERE li.list_id=l.id AND b.archived_at IS NULL)`

// Lists returns Favorites first, then pinned lists in the order they were
// pinned, then the rest by name.
func (s *Store) Lists(ctx context.Context) ([]model.List, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT l.id,l.name,l.kind,l.icon,l.color,l.pinned_at IS NOT NULL,`+listCount+` FROM lists l ORDER BY l.kind='favorites' DESC,l.pinned_at IS NULL,l.pinned_at,l.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.List{}
	for rows.Next() {
		var item model.List
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.Icon, &item.Color, &item.Pinned, &item.Count); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) GetList(ctx context.Context, id int64) (model.List, error) {
	var item model.List
	err := s.DB.QueryRowContext(ctx, `SELECT l.id,l.name,l.kind,l.icon,l.color,l.pinned_at IS NOT NULL,`+listCount+` FROM lists l WHERE l.id=?`, id).Scan(&item.ID, &item.Name, &item.Kind, &item.Icon, &item.Color, &item.Pinned, &item.Count)
	return item, err
}

func (s *Store) CreateList(ctx context.Context, name string) (model.List, error) {
	name, normalized, err := validateName(name)
	if err != nil {
		return model.List{}, err
	}
	// Each new list takes the next color, so neighbours in the sidebar differ.
	var lists int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM lists WHERE kind='custom'`).Scan(&lists); err != nil {
		return model.List{}, err
	}
	color := ListColors[lists%len(ListColors)]
	// A deleted list's number is never handed out again, so an old link can't
	// open a different list.
	var highest int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM lists`).Scan(&highest); err != nil {
		return model.List{}, err
	}
	if used, _ := strconv.ParseInt(s.setting(ctx, "list_last_id"), 10, 64); used > highest {
		highest = used
	}
	id := highest + 1
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO lists(id,name,normalized_name,color) VALUES (?,?,?,?)`, id, name, normalized, color); err != nil {
		return model.List{}, listError(err)
	}
	if err := s.setSetting(ctx, "list_last_id", strconv.FormatInt(id, 10)); err != nil {
		return model.List{}, err
	}
	return s.GetList(ctx, id)
}

// UpdateList renames a list or changes its icon or color.
func (s *Store) UpdateList(ctx context.Context, id int64, input model.ListUpdate) (model.List, error) {
	if err := s.customList(ctx, id); err != nil {
		return model.List{}, err
	}
	if input.Icon != "" && !oneOf(input.Icon, ListIcons) {
		return model.List{}, errors.New("unknown list icon")
	}
	if input.Color != "" && !oneOf(input.Color, ListColors) {
		return model.List{}, errors.New("unknown list color")
	}
	if input.Name != "" {
		name, normalized, err := validateName(input.Name)
		if err != nil {
			return model.List{}, err
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE lists SET name=?,normalized_name=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, name, normalized, id); err != nil {
			return model.List{}, listError(err)
		}
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE lists SET icon=COALESCE(NULLIF(?,''),icon),color=COALESCE(NULLIF(?,''),color),updated_at=CURRENT_TIMESTAMP WHERE id=?`, input.Icon, input.Color, id); err != nil {
		return model.List{}, err
	}
	if input.Pinned != nil {
		// Pinning again keeps the list's place; unpinning clears it.
		statement := `UPDATE lists SET pinned_at=NULL WHERE id=?`
		if *input.Pinned {
			statement = `UPDATE lists SET pinned_at=COALESCE(pinned_at,strftime('%Y-%m-%d %H:%M:%f','now')) WHERE id=?`
		}
		if _, err := s.DB.ExecContext(ctx, statement, id); err != nil {
			return model.List{}, err
		}
	}
	return s.GetList(ctx, id)
}

// DeleteList removes a list and returns the bookmarks that were in it, so
// the deletion can be undone. The bookmarks stay in the library.
func (s *Store) DeleteList(ctx context.Context, id int64) ([]int64, error) {
	if err := s.customList(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT bookmark_id FROM list_items WHERE list_id=? ORDER BY added_at`, id)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var bookmarkID int64
		if err := rows.Scan(&bookmarkID); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, bookmarkID)
	}
	rows.Close()
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM lists WHERE id=?`, id); err != nil {
		return nil, err
	}
	return ids, nil
}

// SetInList adds a bookmark to a list or takes it out. Doing either twice is
// harmless.
func (s *Store) SetInList(ctx context.Context, listID, bookmarkID int64, in bool) error {
	var exists int
	err := s.DB.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM lists WHERE id=?) * (SELECT COUNT(*) FROM bookmarks WHERE id=?)`, listID, bookmarkID).Scan(&exists)
	if err != nil {
		return err
	}
	if exists == 0 {
		return sql.ErrNoRows
	}
	if in {
		_, err = s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO list_items(list_id,bookmark_id) VALUES (?,?)`, listID, bookmarkID)
	} else {
		_, err = s.DB.ExecContext(ctx, `DELETE FROM list_items WHERE list_id=? AND bookmark_id=?`, listID, bookmarkID)
	}
	return err
}

// SetManyInList adds bookmarks to a list or takes them out, all at once.
// Bookmarks that no longer exist are skipped.
func (s *Store) SetManyInList(ctx context.Context, listID int64, bookmarkIDs []int64, in bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM lists WHERE id=?`, listID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return sql.ErrNoRows
	}
	statement := `DELETE FROM list_items WHERE list_id=? AND bookmark_id=?`
	if in {
		statement = `INSERT OR IGNORE INTO list_items(list_id,bookmark_id) SELECT ?,id FROM bookmarks WHERE id=?`
	}
	for _, id := range bookmarkIDs {
		if _, err := tx.ExecContext(ctx, statement, listID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// customList checks that a list exists and is not Favorites.
func (s *Store) customList(ctx context.Context, id int64) error {
	var kind string
	if err := s.DB.QueryRowContext(ctx, `SELECT kind FROM lists WHERE id=?`, id).Scan(&kind); err != nil {
		return err
	}
	if kind == "favorites" {
		return ErrFavorites
	}
	return nil
}

func listError(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrListExists
	}
	return err
}
