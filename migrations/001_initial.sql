CREATE TABLE app_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE bookmarks (
    id INTEGER PRIMARY KEY,
    platform TEXT NOT NULL DEFAULT 'x',
    post_id TEXT NOT NULL,
    author TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL,
    posted_at TEXT,
    bookmarked_at TEXT,
    imported_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    language TEXT,
    visible_context_json TEXT NOT NULL DEFAULT '{}',
    raw_payload_json TEXT NOT NULL,
    extractor_version TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    processing_status TEXT NOT NULL DEFAULT 'pending' CHECK (processing_status IN ('pending','processing','completed','failed','blocked')),
    archived_at TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(platform, post_id)
);

CREATE INDEX idx_bookmarks_imported ON bookmarks(imported_at DESC, id DESC);
CREATE INDEX idx_bookmarks_processing ON bookmarks(processing_status, id);

CREATE TABLE media (
    id INTEGER PRIMARY KEY,
    bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('image','video_poster')),
    url TEXT NOT NULL,
    preview_url TEXT NOT NULL DEFAULT '',
    alt_text TEXT NOT NULL DEFAULT '',
    width INTEGER,
    height INTEGER,
    position INTEGER NOT NULL DEFAULT 0,
    UNIQUE(bookmark_id, url)
);

CREATE TABLE imports (
    id INTEGER PRIMARY KEY,
    source TEXT NOT NULL DEFAULT 'extension',
    started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TEXT,
    received_count INTEGER NOT NULL DEFAULT 0,
    inserted_count INTEGER NOT NULL DEFAULT 0,
    updated_count INTEGER NOT NULL DEFAULT 0,
    unchanged_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE enrichments (
    bookmark_id INTEGER PRIMARY KEY REFERENCES bookmarks(id) ON DELETE CASCADE,
    ai_summary TEXT NOT NULL DEFAULT '',
    manual_summary TEXT,
    media_description TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    prompt_version TEXT NOT NULL DEFAULT '',
    input_hash TEXT NOT NULL DEFAULT '',
    analyzed_at TEXT,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE categories (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL UNIQUE,
    parent_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    description TEXT NOT NULL DEFAULT '',
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0,1)),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (parent_id IS NULL OR parent_id <> id)
);

CREATE TABLE tags (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tag_aliases (
    id INTEGER PRIMARY KEY,
    tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    alias TEXT NOT NULL,
    normalized_alias TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE bookmark_categories (
    bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
    category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    ai_confidence REAL,
    manual_state TEXT NOT NULL DEFAULT 'automatic' CHECK (manual_state IN ('automatic','added','removed')),
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(bookmark_id, category_id)
);

CREATE TABLE bookmark_tags (
    bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
    tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    ai_confidence REAL,
    manual_state TEXT NOT NULL DEFAULT 'automatic' CHECK (manual_state IN ('automatic','added','removed')),
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(bookmark_id, tag_id)
);

CREATE TABLE jobs (
    id INTEGER PRIMARY KEY,
    bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
    job_type TEXT NOT NULL DEFAULT 'analyze',
    input_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','completed','failed','blocked')),
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    available_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    lease_until TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(bookmark_id, job_type, input_hash)
);

CREATE INDEX idx_jobs_claim ON jobs(status, available_at, id);

CREATE TABLE extension_tokens (
    id INTEGER PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL DEFAULT 'Browser extension',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TEXT,
    revoked_at TEXT
);

CREATE TABLE pairing_codes (
    id INTEGER PRIMARY KEY,
    code_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    used_at TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE category_suggestions (
    id INTEGER PRIMARY KEY,
    tag_id INTEGER NOT NULL UNIQUE REFERENCES tags(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','dismissed')),
    suggested_parent_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE VIRTUAL TABLE bookmark_fts USING fts5(
    bookmark_id UNINDEXED,
    text,
    summary,
    author,
    username,
    media_description,
    tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER bookmarks_fts_insert AFTER INSERT ON bookmarks BEGIN
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description)
    VALUES (new.id, new.text, '', new.author, new.username, '');
END;

CREATE TRIGGER bookmarks_fts_update AFTER UPDATE OF text, author, username ON bookmarks BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = old.id;
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description)
    SELECT new.id, new.text, COALESCE(e.manual_summary, e.ai_summary, ''), new.author, new.username, COALESCE(e.media_description, '')
    FROM bookmarks b LEFT JOIN enrichments e ON e.bookmark_id = b.id WHERE b.id = new.id;
END;

CREATE TRIGGER bookmarks_fts_delete AFTER DELETE ON bookmarks BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = old.id;
END;

CREATE TRIGGER enrichments_fts_insert AFTER INSERT ON enrichments BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = new.bookmark_id;
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description)
    SELECT b.id, b.text, COALESCE(new.manual_summary, new.ai_summary, ''), b.author, b.username, new.media_description
    FROM bookmarks b WHERE b.id = new.bookmark_id;
END;

CREATE TRIGGER enrichments_fts_update AFTER UPDATE OF ai_summary, manual_summary, media_description ON enrichments BEGIN
    DELETE FROM bookmark_fts WHERE bookmark_id = new.bookmark_id;
    INSERT INTO bookmark_fts(bookmark_id, text, summary, author, username, media_description)
    SELECT b.id, b.text, COALESCE(new.manual_summary, new.ai_summary, ''), b.author, b.username, new.media_description
    FROM bookmarks b WHERE b.id = new.bookmark_id;
END;

INSERT INTO categories(name, normalized_name, description) VALUES
    ('AI', 'ai', 'Artificial intelligence and machine learning'),
    ('Backend', 'backend', 'Server-side engineering and infrastructure'),
    ('Research', 'research', 'Papers, experiments, and scientific work'),
    ('Career', 'career', 'Professional growth and work'),
    ('Startup', 'startup', 'Startups, products, and entrepreneurship'),
    ('Funny', 'funny', 'Humor and entertaining posts'),
    ('Security', 'security', 'Security, privacy, and vulnerabilities'),
    ('Uncategorized', 'uncategorized', 'Bookmarks awaiting or outside classification');

INSERT INTO app_metadata(key, value) VALUES ('schema_initialized', CURRENT_TIMESTAMP);
