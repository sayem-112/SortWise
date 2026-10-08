package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"sortwise/internal/model"
)

// ErrFavorites is returned when renaming or deleting the built-in Favorites list.
var ErrFavorites = errors.New("Favorites can't be renamed or deleted")

// ErrListExists is returned when a list with the same name already exists.
var ErrListExists = errors.New("a list with this name already exists")

// listCount counts the bookmarks in a list the way the library shows them:
// archived bookmarks are left out.
const listCount = `(SELECT COUNT(*) FROM list_items li JOIN bookmarks b ON b.id=li.bookmark_id WHERE li.list_id=l.id AND b.archived_at IS NULL)`

// Lists returns Favorites first, then the user's lists by name.
func (s *Store) Lists(ctx context.Context) ([]model.List, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT l.id,l.name,l.kind,`+listCount+` FROM lists l ORDER BY l.kind='favorites' DESC,l.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.List{}
	for rows.Next() {
		var item model.List
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.Count); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) GetList(ctx context.Context, id int64) (model.List, error) {
	var item model.List
	err := s.DB.QueryRowContext(ctx, `SELECT l.id,l.name,l.kind,`+listCount+` FROM lists l WHERE l.id=?`, id).Scan(&item.ID, &item.Name, &item.Kind, &item.Count)
	return item, err
}

func (s *Store) CreateList(ctx context.Context, name string) (model.List, error) {
	name, normalized, err := validateName(name)
	if err != nil {
		return model.List{}, err
	}
	result, err := s.DB.ExecContext(ctx, `INSERT INTO lists(name,normalized_name) VALUES (?,?)`, name, normalized)
	if err != nil {
		return model.List{}, listError(err)
	}
	id, _ := result.LastInsertId()
	return s.GetList(ctx, id)
}

func (s *Store) RenameList(ctx context.Context, id int64, name string) (model.List, error) {
	name, normalized, err := validateName(name)
	if err != nil {
		return model.List{}, err
	}
	if err := s.customList(ctx, id); err != nil {
		return model.List{}, err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE lists SET name=?,normalized_name=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, name, normalized, id); err != nil {
		return model.List{}, listError(err)
	}
	return s.GetList(ctx, id)
}

// DeleteList removes a list. The bookmarks in it stay in the library.
func (s *Store) DeleteList(ctx context.Context, id int64) error {
	if err := s.customList(ctx, id); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM lists WHERE id=?`, id)
	return err
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
