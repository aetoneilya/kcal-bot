-- A larger copy of the meal's first photo (JPEG, at most 1080 px a side,
-- ~100 KB) for the top of the meal's screen. NULL for older meals; the
-- small preview stands in.
ALTER TABLE meals ADD COLUMN photo_large BLOB;
