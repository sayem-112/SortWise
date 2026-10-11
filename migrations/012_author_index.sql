-- Author pages and the authors list look bookmarks up by handle, whatever its
-- capitalization.
CREATE INDEX idx_bookmarks_username ON bookmarks(username COLLATE NOCASE, id);
