-- Search by meaning. Each post gets one vector describing what it is about,
-- so posts can be found by topic rather than exact words, and posts close to
-- one another can be shown as related.
CREATE TABLE embeddings (
    bookmark_id INTEGER PRIMARY KEY REFERENCES bookmarks(id) ON DELETE CASCADE,
    model TEXT NOT NULL,
    -- Hash of the text the vector was made from; a different hash means the
    -- post changed and needs a new vector.
    input_hash TEXT NOT NULL,
    -- One signed byte per dimension. Empty when the post has no text to describe.
    vector BLOB NOT NULL,
    -- When the post was last compared with its vector.
    checked_at TEXT NOT NULL,
    -- Grows with every write, so readers can tell when to load again.
    version INTEGER NOT NULL
);

-- Vectors for searches already made, so repeating one (or opening a smart
-- collection built on it) asks the AI service nothing. Only a hash of the
-- words is kept.
CREATE TABLE query_vectors (
    query_hash TEXT PRIMARY KEY,
    vector BLOB NOT NULL,
    used_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
