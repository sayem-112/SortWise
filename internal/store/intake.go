package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"sortwise/internal/model"
)

// Import scopes: what a sync brings in from the X bookmarks list.
const (
	// ScopeAll imports every bookmark on X.
	ScopeAll = "all"
	// ScopeNew imports only bookmarks made after the scope was chosen; the posts
	// at the top of the list at that moment mark where syncs stop.
	ScopeNew = "new"
)

// boundarySize is how many posts from the top of the list mark the "from now
// on" boundary, so unbookmarking one or two of them does not lose it.
const boundarySize = 20

// ErrOtherAccount means X data arrived from a different X account than the one
// this library belongs to.
var ErrOtherAccount = errors.New("this library belongs to a different X account")

func (s *Store) setting(ctx context.Context, key string) string {
	var value string
	_ = s.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&value)
	return value
}

func (s *Store) setSetting(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=CURRENT_TIMESTAMP`, key, value)
	return err
}

// ImportScope returns the chosen scope ("" until the user chooses).
func (s *Store) ImportScope(ctx context.Context) string {
	return s.setting(ctx, "import_scope")
}

// SetImportScope records the scope. For ScopeNew, firstPosts are the post IDs at
// the top of the bookmarks list right now, newest first.
func (s *Store) SetImportScope(ctx context.Context, scope string, firstPosts []string) error {
	switch scope {
	case ScopeAll:
		if err := s.setSetting(ctx, "import_boundary", "[]"); err != nil {
			return err
		}
	case ScopeNew:
		if len(firstPosts) > boundarySize {
			firstPosts = firstPosts[:boundarySize]
		}
		body, _ := json.Marshal(firstPosts)
		if err := s.setSetting(ctx, "import_boundary", string(body)); err != nil {
			return err
		}
	default:
		return errors.New("scope must be all or new")
	}
	return s.setSetting(ctx, "import_scope", scope)
}

// CheckAccount ties the library to the first X account that sends it data and
// rejects data from any other. An empty account is not checked.
func (s *Store) CheckAccount(ctx context.Context, account string) error {
	if account == "" {
		return nil
	}
	owner := s.setting(ctx, "x_account")
	if owner == "" {
		return s.setSetting(ctx, "x_account", account)
	}
	if owner != account {
		return ErrOtherAccount
	}
	return nil
}

// Intake is what remains of a batch after deleted posts and the "from now on"
// boundary are applied.
type Intake struct {
	Posts []model.ImportedBookmark
	// Deleted counts posts skipped because they were deleted in Sortwise.
	Deleted int
	// ReachedBoundary is true when the batch reached posts from before the
	// "from now on" choice; the sync should stop there.
	ReachedBoundary bool
}

// FilterIntake applies the user's choices to posts arriving from X. Syncs skip
// posts deleted in Sortwise and stop at the "from now on" boundary; a post
// bookmarked just now (capture) is always kept and forgets an earlier deletion.
func (s *Store) FilterIntake(ctx context.Context, source string, posts []model.ImportedBookmark) (Intake, error) {
	result := Intake{Posts: make([]model.ImportedBookmark, 0, len(posts))}
	if source == "capture" {
		for _, post := range posts {
			if _, err := s.DB.ExecContext(ctx, `DELETE FROM deleted_posts WHERE platform=? AND post_id=?`, post.Platform, post.PostID); err != nil {
				return result, err
			}
		}
		result.Posts = posts
		return result, nil
	}
	boundary := map[string]bool{}
	if s.ImportScope(ctx) == ScopeNew {
		var ids []string
		_ = json.Unmarshal([]byte(s.setting(ctx, "import_boundary")), &ids)
		for _, id := range ids {
			boundary[id] = true
		}
	}
	for _, post := range posts {
		// The list is newest first: everything from the boundary on is older.
		if boundary[post.PostID] {
			result.ReachedBoundary = true
			break
		}
		var deleted int
		err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM deleted_posts WHERE platform=? AND post_id=?`, post.Platform, post.PostID).Scan(&deleted)
		if err == nil {
			result.Deleted++
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
		result.Posts = append(result.Posts, post)
	}
	return result, nil
}
