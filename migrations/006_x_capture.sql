-- Direct capture from X's own data: video files for posts with video, and a
-- record of posts that were unbookmarked on X after being saved here.

ALTER TABLE media ADD COLUMN video_url TEXT NOT NULL DEFAULT '';
ALTER TABLE bookmarks ADD COLUMN removed_on_x_at TEXT;
