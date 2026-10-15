-- Smart collections: a saved search and filters. Unlike a list, nothing is
-- put in by hand; a collection shows whatever matches right now.
CREATE TABLE collections (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL UNIQUE,
    color TEXT NOT NULL DEFAULT 'blue',
    -- The search and filters, as JSON.
    query_json TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
