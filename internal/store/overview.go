package store

import (
	"context"
	"sort"
	"time"
)

// MainCategories returns every post in the library (archived ones left out)
// with the category that best describes it, or 0 for none: one the person
// chose themselves first, then the AI's most confident. The library map
// colors each post by it.
func (s *Store) MainCategories(ctx context.Context) (map[int64]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT b.id,COALESCE((SELECT bc.category_id FROM bookmark_categories bc
		WHERE bc.bookmark_id=b.id AND bc.manual_state <> 'removed' AND (bc.manual_state='added' OR bc.ai_confidence IS NOT NULL)
		ORDER BY bc.manual_state='added' DESC,bc.ai_confidence DESC,bc.category_id LIMIT 1),0)
		FROM bookmarks b WHERE b.archived_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[int64]int64{}
	for rows.Next() {
		var id, category int64
		if err := rows.Scan(&id, &category); err != nil {
			return nil, err
		}
		result[id] = category
	}
	return result, rows.Err()
}

// DayCount is how many posts were saved on one day (YYYY-MM-DD, local time).
type DayCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// SavedDays is the library's saving activity, day by day.
type SavedDays struct {
	// Days with at least one save from since onward, oldest first. The back
	// catalogue's day is left out.
	Days []DayCount `json:"days"`
	// BackCatalogue is the day a large share of the library arrived at once,
	// usually the first sync, which brings in years of bookmarks. It would
	// dwarf every other day, so it is reported on its own. Nil when there is
	// no such day.
	BackCatalogue *DayCount `json:"backCatalogue"`
	Total         int       `json:"total"`
}

// A day counts as the back catalogue when it holds at least this many posts
// and at least this share of the whole library.
const (
	backCatalogueMinimum = 20
	backCatalogueShare   = 3 // one in three
)

// SavedByDay counts the posts in the library (archived ones left out) by the
// day they were saved, in loc. X doesn't say when something was bookmarked,
// so a post's save date is when Sortwise first received it.
func (s *Store) SavedByDay(ctx context.Context, since time.Time, loc *time.Location) (SavedDays, error) {
	result := SavedDays{Days: []DayCount{}}
	rows, err := s.DB.QueryContext(ctx, `SELECT imported_at FROM bookmarks WHERE archived_at IS NULL`)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var stamp string
		if err := rows.Scan(&stamp); err != nil {
			return result, err
		}
		saved, ok := parseStamp(stamp)
		if !ok {
			continue
		}
		counts[saved.In(loc).Format("2006-01-02")]++
		result.Total++
	}
	if err := rows.Err(); err != nil {
		return result, err
	}

	var biggest DayCount
	for date, count := range counts {
		if count > biggest.Count || (count == biggest.Count && date < biggest.Date) {
			biggest = DayCount{Date: date, Count: count}
		}
	}
	if biggest.Count >= backCatalogueMinimum && biggest.Count*backCatalogueShare >= result.Total {
		result.BackCatalogue = &biggest
		delete(counts, biggest.Date)
	}
	from := since.In(loc).Format("2006-01-02")
	for date, count := range counts {
		if date >= from {
			result.Days = append(result.Days, DayCount{Date: date, Count: count})
		}
	}
	sort.Slice(result.Days, func(i, j int) bool { return result.Days[i].Date < result.Days[j].Date })
	return result, nil
}

// parseStamp reads a time as Sortwise stores them: SQLite's own
// "2006-01-02 15:04:05" (UTC) or RFC 3339 from the extension.
func parseStamp(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
