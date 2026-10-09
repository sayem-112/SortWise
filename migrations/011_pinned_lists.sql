-- Pinned lists sort to the top of the sidebar, under Favorites, in the order
-- they were pinned.
ALTER TABLE lists ADD COLUMN pinned_at TEXT;
