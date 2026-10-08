-- Starter categories were saved with spaces in their matching key ("health
-- fitness") while names typed in the app use hyphens ("health-fitness"), so
-- typing an existing name made a duplicate. Use the app's form.
UPDATE categories
SET normalized_name = replace(normalized_name, ' ', '-')
WHERE normalized_name LIKE '% %'
  AND NOT EXISTS (SELECT 1 FROM categories other WHERE other.normalized_name = replace(categories.normalized_name, ' ', '-'));

-- New libraries start with broad categories that fit any field. The earlier
-- software-specific starters are turned off, but only in a library with no
-- bookmarks yet, so existing libraries keep what they have. Suggested
-- categories bring back the specific ones a library actually needs.
UPDATE categories
SET active = 0, updated_at = CURRENT_TIMESTAMP
WHERE normalized_name IN ('ai', 'backend', 'security', 'startup', 'research', 'software-engineering')
  AND NOT EXISTS (SELECT 1 FROM bookmarks);
