-- Lists the user keeps bookmarks in by hand. Every library has Favorites,
-- which cannot be renamed or deleted; other lists are the user's own.
CREATE TABLE lists (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL DEFAULT 'custom' CHECK (kind IN ('favorites', 'custom')),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE list_items (
    list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
    -- Milliseconds, so a list sorts by the order bookmarks were added.
    added_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    PRIMARY KEY (list_id, bookmark_id)
);

CREATE INDEX list_items_bookmark ON list_items(bookmark_id);

INSERT INTO lists(name, normalized_name, kind) VALUES ('Favorites', 'favorites', 'favorites');
