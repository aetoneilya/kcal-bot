CREATE TABLE users (
    id         INTEGER PRIMARY KEY, -- Telegram user id
    username   TEXT NOT NULL DEFAULT '',
    first_name TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

CREATE TABLE meals (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    eaten_at   INTEGER NOT NULL, -- unix seconds; the client decides what "today" is
    title      TEXT NOT NULL DEFAULT '',
    note       TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX meals_by_user_time ON meals (user_id, eaten_at);

-- Density per 100 g, not line totals: a corrected weight recomputes the line
-- without another model call, and the original density stays inspectable.
CREATE TABLE meal_items (
    meal_id     INTEGER NOT NULL REFERENCES meals (id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    name        TEXT NOT NULL,
    grams       REAL NOT NULL,
    kcal_100    REAL NOT NULL,
    protein_100 REAL NOT NULL,
    fat_100     REAL NOT NULL,
    carbs_100   REAL NOT NULL,
    confidence  TEXT NOT NULL DEFAULT 'low',
    PRIMARY KEY (meal_id, position)
);

-- Model calls cost money, so every user gets a daily allowance.
CREATE TABLE analyze_usage (
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    day     TEXT NOT NULL, -- UTC date, YYYY-MM-DD
    count   INTEGER NOT NULL,
    PRIMARY KEY (user_id, day)
);
