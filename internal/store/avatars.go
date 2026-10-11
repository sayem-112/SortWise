package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"regexp"
	"strings"
)

// avatarFile is the only shape a saved picture's name can have.
var avatarFile = regexp.MustCompile(`^[0-9a-f]{20}\.(jpg|png|webp|gif)$`)

// ValidAvatarFile reports whether name is a saved picture's file name.
// Everything that reads, serves, or deletes a picture checks it first.
func ValidAvatarFile(name string) bool { return avatarFile.MatchString(name) }

// AvatarPath is where the web app loads a saved profile picture from. The
// file name changes whenever the picture does, so the address can be cached
// for good. Anything that is not a saved picture's name gives no path.
func AvatarPath(file string) string {
	if !ValidAvatarFile(file) {
		return ""
	}
	return "/api/v1/avatars/" + file
}

// avatarColumn selects the saved picture's file name for the author of
// bookmark row b (” when there is none).
const avatarColumn = `COALESCE((SELECT a.avatar_file FROM authors a WHERE a.username=b.username),'')`

// noteAvatar records where X serves an author's picture. A new address makes
// the picture due for download again; the saved copy stays until then.
func noteAvatar(ctx context.Context, tx *sql.Tx, username, avatarURL string) error {
	if username == "" || avatarURL == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO authors(username,avatar_url) VALUES (?,?)
		ON CONFLICT(username) DO UPDATE SET avatar_url=excluded.avatar_url,updated_at=CURRENT_TIMESTAMP
		WHERE authors.avatar_url <> excluded.avatar_url`, username, avatarURL)
	return err
}

// AvatarSalt returns this library's private value mixed into picture file
// names, creating it the first time. Without it a name could be worked out
// from an author's public handle and picture address.
func (s *Store) AvatarSalt(ctx context.Context) (string, error) {
	if salt := s.setting(ctx, "avatar_salt"); salt != "" {
		return salt, nil
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	salt := hex.EncodeToString(random)
	return salt, s.setSetting(ctx, "avatar_salt", salt)
}

// PendingAvatar is a picture that has not been saved yet, or has changed.
type PendingAvatar struct {
	Username string
	URL      string
	// OldFile is the copy being replaced, to delete once the new one is in.
	OldFile string
}

// PendingAvatars returns pictures due for download, oldest request first.
func (s *Store) PendingAvatars(ctx context.Context, limit int) ([]PendingAvatar, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT username,avatar_url,avatar_file FROM authors
		WHERE avatar_url <> '' AND avatar_source <> avatar_url ORDER BY updated_at,username LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PendingAvatar{}
	for rows.Next() {
		var item PendingAvatar
		if err := rows.Scan(&item.Username, &item.URL, &item.OldFile); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// AvatarSaved records a downloaded picture. It reports false when the
// author's picture changed again meanwhile, or the author is gone, so the
// caller can throw the file away.
func (s *Store) AvatarSaved(ctx context.Context, username, source, file string) (bool, error) {
	result, err := s.DB.ExecContext(ctx, `UPDATE authors SET avatar_file=?,avatar_source=?,updated_at=CURRENT_TIMESTAMP WHERE username=? AND avatar_url=?`, file, source, username, source)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	return count == 1, nil
}

// AvatarSettled marks an address as dealt with when there is nothing to
// download from it: the author uses X's default picture, or the address is
// dead or not a picture. Any copy saved earlier is kept, and nothing is
// asked for again until the address changes.
func (s *Store) AvatarSettled(ctx context.Context, username, source string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE authors SET avatar_source=?,updated_at=CURRENT_TIMESTAMP WHERE username=? AND avatar_url=?`, source, username, source)
	return err
}

// AvatarDeferred moves an author to the back of the download line after a
// failure that may pass (the image server unreachable or refusing).
func (s *Store) AvatarDeferred(ctx context.Context, username string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE authors SET updated_at=strftime('%Y-%m-%d %H:%M:%f','now') WHERE username=?`, username)
	return err
}

// BackfillAvatars finds the picture address for authors saved before
// pictures were kept, by re-reading one of their posts' stored X data with
// extract. Every author it looks at gets a row, with or without an address,
// so the work is done once. It returns how many authors it looked at.
func (s *Store) BackfillAvatars(ctx context.Context, extract func(raw []byte) string, limit int) (int, error) {
	// With one MAX(), SQLite reads the other columns from that same row. The
	// row wanted is the author's newest post that holds X's own data, so those
	// are ranked above posts read from the page, which carry no address.
	rows, err := s.DB.QueryContext(ctx, `SELECT b.username,b.raw_payload_json,
			MAX(b.id + CASE WHEN b.extractor_version LIKE 'x-graphql%' THEN 4611686018427387904 ELSE 0 END)
		FROM bookmarks b
		WHERE b.username <> '' AND NOT EXISTS (SELECT 1 FROM authors a WHERE a.username=b.username)
		GROUP BY b.username COLLATE NOCASE LIMIT ?`, limit)
	if err != nil {
		return 0, err
	}
	type found struct{ username, url string }
	batch := []found{}
	for rows.Next() {
		var username string
		var raw []byte
		var rank int64
		if err := rows.Scan(&username, &raw, &rank); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, found{username, extract(raw)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, item := range batch {
		if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO authors(username,avatar_url) VALUES (?,?)`, item.username, item.url); err != nil {
			return 0, err
		}
	}
	return len(batch), nil
}

// DropOrphanAvatars removes authors who no longer have any bookmark, in one
// statement so an import arriving at the same moment cannot fall between
// "found" and "removed". Their picture files are then cleared by the next
// reconcile.
func (s *Store) DropOrphanAvatars(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM authors WHERE NOT EXISTS (SELECT 1 FROM bookmarks b WHERE b.username=authors.username COLLATE NOCASE)`)
	return err
}

// ReconcileAvatars brings the records in line with the files actually in the
// pictures folder. present lists the file names found there. Authors whose
// picture file is missing (the folder was cleared, or a backup was restored
// without it) are made due for download again. It returns the files no
// author uses, for the caller to delete.
func (s *Store) ReconcileAvatars(ctx context.Context, present []string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT avatar_file FROM authors WHERE avatar_file <> ''`)
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for rows.Next() {
		var file string
		if err := rows.Scan(&file); err != nil {
			rows.Close()
			return nil, err
		}
		used[file] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	onDisk := map[string]bool{}
	unused := []string{}
	for _, file := range present {
		onDisk[file] = true
		if !used[file] {
			unused = append(unused, file)
		}
	}
	missing := []string{}
	for file := range used {
		if !onDisk[file] {
			missing = append(missing, file)
		}
	}
	for start := 0; start < len(missing); start += 200 {
		chunk := missing[start:min(start+200, len(missing))]
		args := make([]any, len(chunk))
		for index, file := range chunk {
			args[index] = file
		}
		query := `UPDATE authors SET avatar_file='',avatar_source='',updated_at=CURRENT_TIMESTAMP WHERE avatar_file IN (?` + strings.Repeat(",?", len(chunk)-1) + `)`
		if _, err := s.DB.ExecContext(ctx, query, args...); err != nil {
			return nil, err
		}
	}
	return unused, nil
}
