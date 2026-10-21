-- Water drunk per person per local day (the phone's YYYY-MM-DD, like
-- weights), as one running total in millilitres.
CREATE TABLE water (
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    day        TEXT NOT NULL,
    ml         INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, day)
);
