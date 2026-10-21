-- What a person tells about themselves so the app can work out a daily
-- calorie target. Only the inputs are stored; the target is computed from
-- them, so a formula change never leaves stale numbers behind.
CREATE TABLE profiles (
    user_id        INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    sex            TEXT NOT NULL,    -- male | female; the BMR formula differs
    birth_year     INTEGER NOT NULL, -- age changes, the birth year does not
    height_cm      REAL NOT NULL,
    weight_kg      REAL NOT NULL,
    goal_weight_kg REAL NOT NULL,
    activity       TEXT NOT NULL,    -- sedentary | light | moderate | active | very_active
    pace_kg_week   REAL NOT NULL,    -- planned change per week toward the goal
    updated_at     INTEGER NOT NULL
);
