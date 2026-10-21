-- One weigh-in per person per local day. The day is the phone's calendar
-- date (YYYY-MM-DD), like the diary's days, so no time zone is needed here.
CREATE TABLE weights (
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    day        TEXT NOT NULL,
    weight_kg  REAL NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, day)
);
