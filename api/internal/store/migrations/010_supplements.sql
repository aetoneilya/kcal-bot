-- Supplements a person takes, each with a free-text dose ("2000 МЕ, 1 капс"),
-- and the local days (the phone's YYYY-MM-DD, like water) they took one.
-- Deleting a supplement drops its days with it.
CREATE TABLE supplements (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    dose       TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX supplements_user ON supplements (user_id);

CREATE TABLE supplement_days (
    supplement_id INTEGER NOT NULL REFERENCES supplements (id) ON DELETE CASCADE,
    day           TEXT NOT NULL,
    taken_at      INTEGER NOT NULL,
    PRIMARY KEY (supplement_id, day)
);
