-- Each list has its own icon and color, so lists are easy to tell apart.
-- Favorites keeps its star and yellow.
ALTER TABLE lists ADD COLUMN icon TEXT NOT NULL DEFAULT 'list';
ALTER TABLE lists ADD COLUMN color TEXT NOT NULL DEFAULT 'purple';

UPDATE lists SET icon = 'star', color = 'yellow' WHERE kind = 'favorites';

-- Spread existing lists across the palette instead of all purple.
UPDATE lists
SET color = CASE id % 8
    WHEN 0 THEN 'purple' WHEN 1 THEN 'blue' WHEN 2 THEN 'green' WHEN 3 THEN 'orange'
    WHEN 4 THEN 'pink' WHEN 5 THEN 'brown' WHEN 6 THEN 'red' ELSE 'gray' END
WHERE kind = 'custom';
