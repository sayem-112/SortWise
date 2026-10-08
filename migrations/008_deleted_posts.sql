-- Posts deleted in Sortwise. A sync must not bring them back just because they
-- are still bookmarked on X; bookmarking a post again on X does.
CREATE TABLE deleted_posts (
    platform TEXT NOT NULL DEFAULT 'x',
    post_id TEXT NOT NULL,
    deleted_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (platform, post_id)
);
