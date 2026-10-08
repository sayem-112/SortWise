package store

import (
	"context"

	"sortwise/internal/model"
)

func (s *Store) Dashboard(ctx context.Context) (model.DashboardStats, error) {
	var result model.DashboardStats
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(CASE WHEN processing_status='completed' THEN 1 END),COUNT(CASE WHEN processing_status IN ('pending','processing','blocked') THEN 1 END),COUNT(CASE WHEN processing_status='failed' THEN 1 END) FROM bookmarks WHERE archived_at IS NULL`).Scan(&result.Total, &result.Completed, &result.Pending, &result.Failed)
	if err != nil {
		return result, err
	}
	if err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM bookmarks b WHERE b.archived_at IS NULL AND `+needsReviewCondition).Scan(&result.NeedsReview); err != nil {
		return result, err
	}
	result.Categories = []model.NamedCount{}
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id,c.name,COUNT(*) FROM bookmark_categories bc JOIN categories c ON c.id=bc.category_id JOIN bookmarks b ON b.id=bc.bookmark_id WHERE b.archived_at IS NULL AND c.active=1 AND bc.manual_state <> 'removed' AND (bc.manual_state='added' OR bc.ai_confidence IS NOT NULL) GROUP BY c.id ORDER BY 3 DESC,c.name LIMIT 12`)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item model.NamedCount
		if err := rows.Scan(&item.ID, &item.Name, &item.Count); err != nil {
			rows.Close()
			return result, err
		}
		result.Categories = append(result.Categories, item)
	}
	rows.Close()
	result.TopTags = []model.NamedCount{}
	rows, err = s.DB.QueryContext(ctx, `SELECT t.id,t.name,COUNT(*) FROM bookmark_tags bt JOIN tags t ON t.id=bt.tag_id JOIN bookmarks b ON b.id=bt.bookmark_id WHERE b.archived_at IS NULL AND bt.manual_state <> 'removed' AND (bt.manual_state='added' OR bt.ai_confidence IS NOT NULL) GROUP BY t.id ORDER BY 3 DESC,t.name LIMIT 16`)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item model.NamedCount
		if err := rows.Scan(&item.ID, &item.Name, &item.Count); err != nil {
			rows.Close()
			return result, err
		}
		result.TopTags = append(result.TopTags, item)
	}
	rows.Close()
	return result, nil
}
