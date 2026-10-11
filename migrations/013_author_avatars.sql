-- Authors' profile pictures, saved on this computer so they still show after
-- the author changes or removes theirs.
-- avatar_url is where X serves the picture. avatar_file is the saved copy in
-- the data folder's avatars directory ('' when there is none). avatar_source
-- is the address last dealt with: a picture is due for download while it
-- differs from avatar_url.
CREATE TABLE authors (
    username TEXT PRIMARY KEY COLLATE NOCASE,
    avatar_url TEXT NOT NULL DEFAULT '',
    avatar_file TEXT NOT NULL DEFAULT '',
    avatar_source TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
