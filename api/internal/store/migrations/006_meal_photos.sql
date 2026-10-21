-- A small preview of the meal's first photo (JPEG, at most 320 px a side),
-- so the diary shows what was eaten. NULL for meals entered by hand.
ALTER TABLE meals ADD COLUMN photo BLOB;
