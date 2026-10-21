-- The app is open to anyone; the owner can block a person instead.
ALTER TABLE users ADD COLUMN blocked_at INTEGER; -- unix seconds; NULL = not blocked

-- What each person's model calls cost today, in millionths of a USD, so a
-- daily spend cap can stop one person from draining the API budget.
ALTER TABLE analyze_usage ADD COLUMN cost_micros INTEGER NOT NULL DEFAULT 0;
