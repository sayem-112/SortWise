-- Index link-card titles and descriptions (including X Articles filled in from
-- X's embed data) and quoted posts alongside the post text.

DROP VIEW IF EXISTS bookmark_fts_source;

CREATE VIEW bookmark_fts_source AS
SELECT
    b.id AS bookmark_id,
    TRIM(
        b.text
        || ' ' || COALESCE(json_extract(b.visible_context_json, '$.card.title'), '')
        || ' ' || COALESCE(json_extract(b.visible_context_json, '$.card.description'), '')
        || ' ' || COALESCE(json_extract(b.visible_context_json, '$.quotedPost.text'), '')
    ) AS text,
    COALESCE(e.manual_summary, e.ai_summary, '') AS summary,
    b.author AS author,
    b.username AS username,
    COALESCE(e.media_description, '') AS media_description,
    TRIM(
        COALESCE((SELECT group_concat(t.name, ' ') FROM bookmark_tags bt JOIN tags t ON t.id = bt.tag_id
                  WHERE bt.bookmark_id = b.id AND bt.manual_state <> 'removed' AND (bt.manual_state = 'added' OR bt.ai_confidence IS NOT NULL)), '')
        || ' ' ||
        COALESCE((SELECT group_concat(a.alias, ' ') FROM bookmark_tags bt JOIN tag_aliases a ON a.tag_id = bt.tag_id
                  WHERE bt.bookmark_id = b.id AND bt.manual_state <> 'removed' AND (bt.manual_state = 'added' OR bt.ai_confidence IS NOT NULL)), '')
        || ' ' ||
        COALESCE((SELECT group_concat(c.name, ' ') FROM bookmark_categories bc JOIN categories c ON c.id = bc.category_id
                  WHERE bc.bookmark_id = b.id AND bc.manual_state <> 'removed' AND (bc.manual_state = 'added' OR bc.ai_confidence IS NOT NULL)), '')
    ) AS taxonomy
FROM bookmarks b
LEFT JOIN enrichments e ON e.bookmark_id = b.id;

DROP TRIGGER IF EXISTS bookmarks_fts_update;
CREATE TRIGGER bookmarks_fts_update AFTER UPDATE OF text, author, username, visible_context_json ON bookmarks BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = old.id;
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source WHERE bookmark_id = new.id;
END;

DELETE FROM bookmark_fts;
INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source;
