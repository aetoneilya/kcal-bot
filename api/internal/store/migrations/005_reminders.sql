-- Messages the bot may send a person on their own clock. Off unless they
-- turn them on; the last_* days keep each message to once a local day.
CREATE TABLE reminders (
    user_id          INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    time_zone        TEXT NOT NULL,          -- IANA name from the phone, e.g. Europe/Moscow
    summary_at       TEXT NOT NULL DEFAULT '', -- HH:MM for the evening total; '' = off
    nudge            INTEGER NOT NULL DEFAULT 0, -- 1 = remind when nothing is logged by midday
    last_summary_day TEXT NOT NULL DEFAULT '',
    last_nudge_day   TEXT NOT NULL DEFAULT ''
);
