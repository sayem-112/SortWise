-- Make tags and categories searchable. A post that says "Lamin Yamal" but is tagged
-- "Lamine Yamal" must be found by searching "lamine". The index gains a taxonomy
-- column that triggers keep in sync with tag and category assignments and renames.

DROP TRIGGER IF EXISTS bookmarks_fts_insert;
DROP TRIGGER IF EXISTS bookmarks_fts_update;
DROP TRIGGER IF EXISTS bookmarks_fts_delete;
DROP TRIGGER IF EXISTS enrichments_fts_insert;
DROP TRIGGER IF EXISTS enrichments_fts_update;
DROP TABLE IF EXISTS bookmark_fts;

CREATE VIRTUAL TABLE bookmark_fts USING fts5(
    bookmark_id UNINDEXED,
    text,
    summary,
    author,
    username,
    media_description,
    taxonomy,
    tokenize = 'unicode61 remove_diacritics 2'
);

-- One view computes a bookmark's full index row, so every trigger stays identical.
CREATE VIEW bookmark_fts_source AS
SELECT
    b.id AS bookmark_id,
    b.text AS text,
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

INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source;

CREATE TRIGGER bookmarks_fts_insert AFTER INSERT ON bookmarks BEGIN
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source WHERE bookmark_id = new.id;
END;

CREATE TRIGGER bookmarks_fts_update AFTER UPDATE OF text, author, username ON bookmarks BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = old.id;
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source WHERE bookmark_id = new.id;
END;

CREATE TRIGGER bookmarks_fts_delete AFTER DELETE ON bookmarks BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = old.id;
END;

CREATE TRIGGER enrichments_fts_insert AFTER INSERT ON enrichments BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = new.bookmark_id;
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source WHERE bookmark_id = new.bookmark_id;
END;

CREATE TRIGGER enrichments_fts_update AFTER UPDATE OF ai_summary, manual_summary, media_description ON enrichments BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = new.bookmark_id;
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source WHERE bookmark_id = new.bookmark_id;
END;

-- Tag and category assignments (AI results, manual corrections, merges).
CREATE TRIGGER bookmark_tags_fts_insert AFTER INSERT ON bookmark_tags BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = new.bookmark_id;
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source WHERE bookmark_id = new.bookmark_id;
END;

CREATE TRIGGER bookmark_tags_fts_update AFTER UPDATE ON bookmark_tags BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = new.bookmark_id;
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source WHERE bookmark_id = new.bookmark_id;
END;

CREATE TRIGGER bookmark_tags_fts_delete AFTER DELETE ON bookmark_tags BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = old.bookmark_id;
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source WHERE bookmark_id = old.bookmark_id;
END;

CREATE TRIGGER bookmark_categories_fts_insert AFTER INSERT ON bookmark_categories BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = new.bookmark_id;
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source WHERE bookmark_id = new.bookmark_id;
END;

CREATE TRIGGER bookmark_categories_fts_update AFTER UPDATE ON bookmark_categories BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = new.bookmark_id;
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source WHERE bookmark_id = new.bookmark_id;
END;

CREATE TRIGGER bookmark_categories_fts_delete AFTER DELETE ON bookmark_categories BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = old.bookmark_id;
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source WHERE bookmark_id = old.bookmark_id;
END;

-- Renames and new aliases change what every affected bookmark should match.
CREATE TRIGGER tags_fts_rename AFTER UPDATE OF name ON tags BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id IN (SELECT bookmark_id FROM bookmark_tags WHERE tag_id = new.id);
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source
    WHERE bookmark_id IN (SELECT bookmark_id FROM bookmark_tags WHERE tag_id = new.id);
END;

CREATE TRIGGER tag_aliases_fts_insert AFTER INSERT ON tag_aliases BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id IN (SELECT bookmark_id FROM bookmark_tags WHERE tag_id = new.tag_id);
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source
    WHERE bookmark_id IN (SELECT bookmark_id FROM bookmark_tags WHERE tag_id = new.tag_id);
END;

CREATE TRIGGER categories_fts_rename AFTER UPDATE OF name ON categories BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id IN (SELECT bookmark_id FROM bookmark_categories WHERE category_id = new.id);
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description, taxonomy)
    SELECT bookmark_id, text, summary, author, username, media_description, taxonomy FROM bookmark_fts_source
    WHERE bookmark_id IN (SELECT bookmark_id FROM bookmark_categories WHERE category_id = new.id);
END;
