package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// UpdateRecord is what the update check has stored: whether it is on, and
// the newest release it found and when.
type UpdateRecord struct {
	On bool
	// Latest is the newest release's version, like "0.9.0"; "" until found.
	Latest    string
	CheckedAt time.Time
}

// UpdateRecord reads the record. The check counts as off when its setting
// can't be read, so a busy or broken library never checks against your
// choice.
func (s *Store) UpdateRecord(ctx context.Context) UpdateRecord {
	var choice string
	err := s.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='update_check'`).Scan(&choice)
	on := errors.Is(err, sql.ErrNoRows) || (err == nil && choice != "false")
	record := UpdateRecord{On: on, Latest: s.setting(ctx, "update_latest")}
	record.CheckedAt, _ = time.Parse(time.RFC3339, s.setting(ctx, "update_checked_at"))
	return record
}

// SetUpdateCheck turns the daily update check on or off.
func (s *Store) SetUpdateCheck(ctx context.Context, on bool) error {
	value := "true"
	if !on {
		value = "false"
	}
	return s.setSetting(ctx, "update_check", value)
}

// SaveUpdateCheck records a finished check: the newest release, or "" for
// none, found at checked.
func (s *Store) SaveUpdateCheck(ctx context.Context, latest string, checked time.Time) error {
	if err := s.setSetting(ctx, "update_latest", latest); err != nil {
		return err
	}
	return s.setSetting(ctx, "update_checked_at", checked.UTC().Format(time.RFC3339))
}
