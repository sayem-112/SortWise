package store

import (
	"context"
	"time"
)

// DesktopSetUp reports whether Sortwise has added itself to Windows (the
// Start menu, and starting at sign-in) before, so it only does so once.
func (s *Store) DesktopSetUp(ctx context.Context) bool {
	return s.setting(ctx, "desktop_set_up") == "true"
}

func (s *Store) MarkDesktopSetUp(ctx context.Context) error {
	return s.setSetting(ctx, "desktop_set_up", "true")
}

// Glance is the library in a few numbers, for the tray panel.
type Glance struct {
	// Bookmarks counts every post on this PC, as the sidebar does.
	Bookmarks int
	// Waiting counts posts waiting for the AI to organize them.
	Waiting int
	// SavedToday counts posts saved since midnight where now is.
	SavedToday int
}

func (s *Store) Glance(ctx context.Context, now time.Time) (Glance, error) {
	var glance Glance
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM bookmarks`).Scan(&glance.Bookmarks); err != nil {
		return glance, err
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE status IN ('pending','processing','blocked')`).Scan(&glance.Waiting); err != nil {
		return glance, err
	}
	// Stamps are UTC and start with the date, so comparing text from the day
	// before midnight's UTC date narrows the rows; each is then checked.
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	rows, err := s.DB.QueryContext(ctx, `SELECT imported_at FROM bookmarks WHERE archived_at IS NULL AND imported_at >= ?`, midnight.UTC().AddDate(0, 0, -1).Format("2006-01-02"))
	if err != nil {
		return glance, err
	}
	defer rows.Close()
	for rows.Next() {
		var stamp string
		if err := rows.Scan(&stamp); err != nil {
			return glance, err
		}
		if saved, ok := parseStamp(stamp); ok && !saved.Before(midnight) {
			glance.SavedToday++
		}
	}
	return glance, rows.Err()
}
