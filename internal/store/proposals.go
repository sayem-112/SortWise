package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"sortwise/internal/model"
)

// ErrProposalNotFound is returned when a proposal does not exist or was already handled.
var ErrProposalNotFound = errors.New("proposal not found")

// proposalConfidence marks categories applied from an accepted proposal. It is
// above the 0.5 review threshold but below typical direct AI classifications.
const proposalConfidence = 0.6

const assignedTag = `bt.manual_state <> 'removed' AND (bt.manual_state='added' OR bt.ai_confidence IS NOT NULL)`

// TaxonomySnapshot gathers what the AI needs to propose structure.
func (s *Store) TaxonomySnapshot(ctx context.Context, tagLimit, summaryLimit int) (model.TaxonomySnapshot, error) {
	snapshot := model.TaxonomySnapshot{Categories: []model.CategoryStat{}, Tags: []model.TagStat{}, Summaries: []string{}}
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM bookmarks WHERE processing_status='completed' AND archived_at IS NULL`).Scan(&snapshot.Analyzed); err != nil {
		return snapshot, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT c.name,COALESCE(p.name,''),c.description,COUNT(CASE WHEN bc.manual_state <> 'removed' AND (bc.manual_state='added' OR bc.ai_confidence IS NOT NULL) THEN 1 END) FROM categories c LEFT JOIN categories p ON p.id=c.parent_id LEFT JOIN bookmark_categories bc ON bc.category_id=c.id WHERE c.active=1 GROUP BY c.id ORDER BY c.name`)
	if err != nil {
		return snapshot, err
	}
	for rows.Next() {
		var item model.CategoryStat
		if err := rows.Scan(&item.Name, &item.Parent, &item.Description, &item.Count); err != nil {
			rows.Close()
			return snapshot, err
		}
		snapshot.Categories = append(snapshot.Categories, item)
	}
	rows.Close()
	snapshot.Tags, err = s.tagStats(ctx, tagLimit)
	if err != nil {
		return snapshot, err
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT COALESCE(e.manual_summary,e.ai_summary) FROM enrichments e JOIN bookmarks b ON b.id=e.bookmark_id WHERE b.archived_at IS NULL AND COALESCE(e.manual_summary,e.ai_summary,'') <> '' ORDER BY random() LIMIT ?`, summaryLimit)
	if err != nil {
		return snapshot, err
	}
	defer rows.Close()
	for rows.Next() {
		var summary string
		if err := rows.Scan(&summary); err != nil {
			return snapshot, err
		}
		if runes := []rune(summary); len(runes) > 240 {
			summary = string(runes[:240]) + "…"
		}
		snapshot.Summaries = append(snapshot.Summaries, summary)
	}
	return snapshot, rows.Err()
}

func (s *Store) tagStats(ctx context.Context, limit int) ([]model.TagStat, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT t.name,t.kind,COUNT(*) FROM tags t JOIN bookmark_tags bt ON bt.tag_id=t.id WHERE `+assignedTag+` GROUP BY t.id ORDER BY 3 DESC,t.name LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.TagStat{}
	for rows.Next() {
		var item model.TagStat
		if err := rows.Scan(&item.Name, &item.Kind, &item.Count); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// ReplaceProposals stores a fresh set of proposals of one kind, dismissing any
// still-pending proposals of that kind so old suggestions do not pile up.
func (s *Store) ReplaceProposals(ctx context.Context, kind string, payloads []any) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE taxonomy_proposals SET status='dismissed',updated_at=CURRENT_TIMESTAMP WHERE kind=? AND status='pending'`, kind); err != nil {
		return err
	}
	for _, payload := range payloads {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO taxonomy_proposals(kind,payload_json) VALUES (?,?)`, kind, string(data)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListProposals returns pending proposals, each with how many bookmarks it affects.
func (s *Store) ListProposals(ctx context.Context) ([]model.TaxonomyProposal, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,kind,payload_json FROM taxonomy_proposals WHERE status='pending' ORDER BY kind,id`)
	if err != nil {
		return nil, err
	}
	result := []model.TaxonomyProposal{}
	for rows.Next() {
		var item model.TaxonomyProposal
		var payload string
		if err := rows.Scan(&item.ID, &item.Kind, &payload); err != nil {
			rows.Close()
			return nil, err
		}
		if err := decodeProposal(&item, payload); err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, item)
	}
	rows.Close()
	for index := range result {
		item := &result[index]
		var err error
		if item.Category != nil {
			item.Matches, err = s.countTaggedBookmarks(ctx, s.DB, item.Category.Tags)
		} else if item.Merge != nil {
			item.Matches, err = s.countTaggedBookmarks(ctx, s.DB, []string{item.Merge.Source})
		}
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func decodeProposal(item *model.TaxonomyProposal, payload string) error {
	switch item.Kind {
	case "category":
		item.Category = &model.CategoryProposal{}
		return json.Unmarshal([]byte(payload), item.Category)
	case "merge":
		item.Merge = &model.MergeProposal{}
		return json.Unmarshal([]byte(payload), item.Merge)
	}
	return errors.New("unknown proposal kind")
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) countTaggedBookmarks(ctx context.Context, db queryer, tags []string) (int, error) {
	if len(tags) == 0 {
		return 0, nil
	}
	placeholders, args := tagNameArgs(tags)
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT bt.bookmark_id) FROM bookmark_tags bt JOIN tags t ON t.id=bt.tag_id JOIN bookmarks b ON b.id=bt.bookmark_id WHERE b.archived_at IS NULL AND `+assignedTag+` AND lower(t.name) IN (`+placeholders+`)`, args...).Scan(&count)
	return count, err
}

func tagNameArgs(tags []string) (string, []any) {
	args := make([]any, 0, len(tags))
	for _, tag := range tags {
		args = append(args, strings.ToLower(tag))
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(tags)), ","), args
}

// AcceptProposal applies a pending proposal. A category proposal creates the
// category and files every bookmark carrying one of its tags into it; a merge
// proposal merges the source tag into the target.
func (s *Store) AcceptProposal(ctx context.Context, id int64) error {
	var item model.TaxonomyProposal
	var payload string
	err := s.DB.QueryRowContext(ctx, `SELECT id,kind,payload_json FROM taxonomy_proposals WHERE id=? AND status='pending'`, id).Scan(&item.ID, &item.Kind, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrProposalNotFound
	}
	if err != nil {
		return err
	}
	if err := decodeProposal(&item, payload); err != nil {
		return err
	}
	if item.Merge != nil {
		var sourceID, targetID int64
		if err := s.DB.QueryRowContext(ctx, `SELECT id FROM tags WHERE lower(name)=lower(?)`, item.Merge.Source).Scan(&sourceID); err != nil {
			return errors.New("the tag to merge no longer exists")
		}
		if err := s.DB.QueryRowContext(ctx, `SELECT id FROM tags WHERE lower(name)=lower(?)`, item.Merge.Target).Scan(&targetID); err != nil {
			return errors.New("the tag to keep no longer exists")
		}
		if err := s.MergeTag(ctx, sourceID, targetID); err != nil {
			return err
		}
		return s.setProposalStatus(ctx, id, "accepted")
	}

	proposal := item.Category
	var parentID *int64
	if proposal.Parent != "" {
		var value int64
		if err := s.DB.QueryRowContext(ctx, `SELECT id FROM categories WHERE lower(name)=lower(?) AND active=1`, proposal.Parent).Scan(&value); err != nil {
			return errors.New("the parent category no longer exists")
		}
		parentID = &value
	}
	category, err := s.reviveCategory(ctx, proposal.Name, proposal.Description, parentID)
	if err != nil {
		return err
	}
	if len(proposal.Tags) > 0 {
		placeholders, args := tagNameArgs(proposal.Tags)
		args = append([]any{category.ID, proposalConfidence}, args...)
		// Bookmarks where the user removed this category by hand are left alone by the
		// conflict clause, which only fills in automatic rows.
		_, err = s.DB.ExecContext(ctx, `INSERT INTO bookmark_categories(bookmark_id,category_id,ai_confidence) SELECT DISTINCT bt.bookmark_id,?,? FROM bookmark_tags bt JOIN tags t ON t.id=bt.tag_id WHERE `+assignedTag+` AND lower(t.name) IN (`+placeholders+`) ON CONFLICT(bookmark_id,category_id) DO UPDATE SET ai_confidence=COALESCE(bookmark_categories.ai_confidence,excluded.ai_confidence),updated_at=CURRENT_TIMESTAMP WHERE bookmark_categories.manual_state='automatic'`, args...)
		if err != nil {
			return err
		}
	}
	return s.setProposalStatus(ctx, id, "accepted")
}

// reviveCategory turns a turned-off category with the same name back on (for
// example a starter category a new library did not need at first), or creates
// the category.
func (s *Store) reviveCategory(ctx context.Context, name, description string, parentID *int64) (model.Category, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM categories WHERE normalized_name=? AND active=0`, normalizeTaxonomy(name)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return s.CreateCategory(ctx, name, description, parentID)
	}
	if err != nil {
		return model.Category{}, err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE categories SET active=1,parent_id=COALESCE(?,parent_id),description=CASE WHEN ?<>'' THEN ? ELSE description END,updated_at=CURRENT_TIMESTAMP WHERE id=?`, parentID, description, description, id); err != nil {
		return model.Category{}, err
	}
	return model.Category{ID: id, Name: name, ParentID: parentID, Description: description, Active: true}, nil
}

func (s *Store) DismissProposal(ctx context.Context, id int64) error {
	return s.setProposalStatus(ctx, id, "dismissed")
}

func (s *Store) setProposalStatus(ctx context.Context, id int64, status string) error {
	result, err := s.DB.ExecContext(ctx, `UPDATE taxonomy_proposals SET status=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='pending'`, status, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrProposalNotFound
	}
	return nil
}

// TagStats exposes tag usage for merge suggestions.
func (s *Store) TagStats(ctx context.Context, limit int) ([]model.TagStat, error) {
	return s.tagStats(ctx, limit)
}
