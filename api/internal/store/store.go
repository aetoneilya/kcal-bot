// Package store owns kcal-bot's SQLite persistence.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/model"
	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = model.ErrInvalid
	// ErrLimit means the user spent today's model-call allowance.
	ErrLimit = errors.New("daily analysis limit reached")
)

type Store struct {
	db *sql.DB
	// now is swapped in tests; the daily allowance depends on the date.
	now func() time.Time
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite settings such as foreign_keys are connection-local. One writer
	// connection is plenty for a handful of users and keeps them stable.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = 5000`,
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	if err := migrate(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, now: time.Now}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Ping proves the database file is still reachable; the health check calls it
// so a lost volume surfaces as a failing probe, not a 500 later.
func (s *Store) Ping(ctx context.Context) error {
	var one int
	if err := s.db.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	return nil
}

// Backup writes a consistent snapshot of the live database with VACUUM INTO.
// Safe while serving; refuses to overwrite an existing file.
func (s *Store) Backup(ctx context.Context, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%w: backup path is required", ErrInvalid)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("backup to %s: %w", path, err)
	}
	return nil
}

// Account is what the store knows about a person beyond their Telegram name.
type Account struct {
	// Created is true only on the request that first saw this person.
	Created bool
	Blocked bool
}

// EnsureUser records the person (refreshing their name) and reports whether
// they are new and whether they are blocked.
func (s *Store) EnsureUser(ctx context.Context, user model.User) (Account, error) {
	if user.ID == 0 {
		return Account{}, fmt.Errorf("%w: user id is required", ErrInvalid)
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, username, first_name, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING`,
		user.ID, user.Username, user.FirstName, s.now().Unix())
	if err != nil {
		return Account{}, fmt.Errorf("ensure user: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 1 {
		return Account{Created: true}, nil
	}
	var blockedAt sql.NullInt64
	err = s.db.QueryRowContext(ctx, `
		UPDATE users SET username = ?, first_name = ? WHERE id = ? RETURNING blocked_at`,
		user.Username, user.FirstName, user.ID).Scan(&blockedAt)
	if err != nil {
		return Account{}, fmt.Errorf("ensure user: %w", err)
	}
	return Account{Blocked: blockedAt.Valid}, nil
}

// User returns a known person and whether they are blocked.
func (s *Store) User(ctx context.Context, id int64) (model.User, bool, error) {
	user := model.User{ID: id}
	var blockedAt sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT username, first_name, blocked_at FROM users WHERE id = ?`, id).
		Scan(&user.Username, &user.FirstName, &blockedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.User{}, false, ErrNotFound
	}
	if err != nil {
		return model.User{}, false, fmt.Errorf("query user: %w", err)
	}
	return user, blockedAt.Valid, nil
}

// SetBlocked blocks or unblocks a known person. Their diary is kept either way.
func (s *Store) SetBlocked(ctx context.Context, id int64, blocked bool) error {
	var at any
	if blocked {
		at = s.now().Unix()
	}
	result, err := s.db.ExecContext(ctx, `UPDATE users SET blocked_at = ? WHERE id = ?`, at, id)
	if err != nil {
		return fmt.Errorf("set blocked: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- reminders ---------------------------------------------------------------

// Reminders returns the person's reminder settings; all off before they set any.
func (s *Store) Reminders(ctx context.Context, userID int64) (model.Reminders, error) {
	var r model.Reminders
	err := s.db.QueryRowContext(ctx, `SELECT time_zone, summary_at, nudge FROM reminders WHERE user_id = ?`, userID).
		Scan(&r.TimeZone, &r.SummaryAt, &r.Nudge)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Reminders{}, nil
	}
	return r, err
}

func (s *Store) SaveReminders(ctx context.Context, userID int64, r model.Reminders) (model.Reminders, error) {
	if err := r.Validate(); err != nil {
		return model.Reminders{}, err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO reminders (user_id, time_zone, summary_at, nudge) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			time_zone = excluded.time_zone, summary_at = excluded.summary_at, nudge = excluded.nudge`,
		userID, r.TimeZone, r.SummaryAt, r.Nudge)
	if err != nil {
		return model.Reminders{}, fmt.Errorf("save reminders: %w", err)
	}
	return r, nil
}

// Subscriber is one person with at least one reminder on.
type Subscriber struct {
	User           model.User
	Reminders      model.Reminders
	LastSummaryDay string
	LastNudgeDay   string
}

// Subscribers lists everyone with a reminder on who is not blocked.
func (s *Store) Subscribers(ctx context.Context) ([]Subscriber, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.username, u.first_name, r.time_zone, r.summary_at, r.nudge, r.last_summary_day, r.last_nudge_day
		FROM reminders r JOIN users u ON u.id = r.user_id
		WHERE (r.summary_at != '' OR r.nudge = 1) AND u.blocked_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("query subscribers: %w", err)
	}
	defer rows.Close()
	var subs []Subscriber
	for rows.Next() {
		var sub Subscriber
		if err := rows.Scan(&sub.User.ID, &sub.User.Username, &sub.User.FirstName, &sub.Reminders.TimeZone,
			&sub.Reminders.SummaryAt, &sub.Reminders.Nudge, &sub.LastSummaryDay, &sub.LastNudgeDay); err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// MarkSent records that a reminder went out on a local day. kind is
// "summary" or "nudge".
func (s *Store) MarkSent(ctx context.Context, userID int64, kind, day string) error {
	column := "last_summary_day"
	if kind == "nudge" {
		column = "last_nudge_day"
	}
	_, err := s.db.ExecContext(ctx, `UPDATE reminders SET `+column+` = ? WHERE user_id = ?`, day, userID)
	return err
}

// StopReminders turns everything off, e.g. after the person blocked the bot.
func (s *Store) StopReminders(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reminders SET summary_at = '', nudge = 0 WHERE user_id = ?`, userID)
	return err
}

// --- weight log --------------------------------------------------------------

// SaveWeight records the day's weigh-in, replacing an earlier one that day.
// When it is the newest weigh-in it also becomes the profile's current
// weight, so the daily target follows the scale without another form.
func (s *Store) SaveWeight(ctx context.Context, userID int64, w model.Weight) (model.Weight, error) {
	w.WeightKg = math.Round(w.WeightKg*10) / 10
	if err := model.ValidateWeight(w); err != nil {
		return model.Weight{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Weight{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO weights (user_id, day, weight_kg, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, day) DO UPDATE SET weight_kg = excluded.weight_kg, updated_at = excluded.updated_at`,
		userID, w.Day, w.WeightKg, s.now().Unix()); err != nil {
		return model.Weight{}, fmt.Errorf("save weight: %w", err)
	}
	if err := syncProfileWeight(ctx, tx, userID); err != nil {
		return model.Weight{}, err
	}
	return w, tx.Commit()
}

// DeleteWeight removes a weigh-in; the profile falls back to the newest left.
// SaveWater sets a day's water total.
func (s *Store) SaveWater(ctx context.Context, userID int64, w model.Water) (model.Water, error) {
	if err := model.ValidateWater(w); err != nil {
		return model.Water{}, err
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO water (user_id, day, ml, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, day) DO UPDATE SET ml = excluded.ml, updated_at = excluded.updated_at`,
		userID, w.Day, w.Ml, s.now().Unix()); err != nil {
		return model.Water{}, fmt.Errorf("save water: %w", err)
	}
	return w, nil
}

// Water returns the days in [from, to] with any water, oldest first.
func (s *Store) Water(ctx context.Context, userID int64, from, to string) ([]model.Water, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT day, ml FROM water WHERE user_id = ? AND day >= ? AND day <= ? AND ml > 0 ORDER BY day`,
		userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query water: %w", err)
	}
	defer rows.Close()
	var days []model.Water
	for rows.Next() {
		var w model.Water
		if err := rows.Scan(&w.Day, &w.Ml); err != nil {
			return nil, err
		}
		days = append(days, w)
	}
	return days, rows.Err()
}

func (s *Store) DeleteWeight(ctx context.Context, userID int64, day string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM weights WHERE user_id = ? AND day = ?`, userID, day)
	if err != nil {
		return fmt.Errorf("delete weight: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if err := syncProfileWeight(ctx, tx, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// syncProfileWeight copies the newest weigh-in into the profile, if both exist.
func syncProfileWeight(ctx context.Context, tx *sql.Tx, userID int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE profiles SET weight_kg = (
			SELECT weight_kg FROM weights WHERE user_id = ? ORDER BY day DESC LIMIT 1
		), updated_at = ?
		WHERE user_id = ? AND EXISTS (SELECT 1 FROM weights WHERE user_id = ?)`,
		userID, time.Now().Unix(), userID, userID)
	if err != nil {
		return fmt.Errorf("sync profile weight: %w", err)
	}
	return nil
}

// Weights returns the weigh-ins on days in [from, to], oldest first. Days
// compare as text, which is date order for YYYY-MM-DD.
func (s *Store) Weights(ctx context.Context, userID int64, from, to string) ([]model.Weight, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT day, weight_kg FROM weights WHERE user_id = ? AND day >= ? AND day <= ? ORDER BY day`,
		userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query weights: %w", err)
	}
	defer rows.Close()
	weights := []model.Weight{}
	for rows.Next() {
		var w model.Weight
		if err := rows.Scan(&w.Day, &w.WeightKg); err != nil {
			return nil, err
		}
		weights = append(weights, w)
	}
	return weights, rows.Err()
}

// --- daily allowance ------------------------------------------------------

func (s *Store) today() string { return s.now().UTC().Format(time.DateOnly) }

// AddSpend adds one call's cost to the person's spend for today and returns
// the new total, both in USD. Stored in micro-dollars so sums stay exact.
func (s *Store) AddSpend(ctx context.Context, userID int64, usd float64) (float64, error) {
	var total int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO analyze_usage (user_id, day, count, cost_micros) VALUES (?, ?, 0, ?)
		ON CONFLICT (user_id, day) DO UPDATE SET cost_micros = cost_micros + excluded.cost_micros
		RETURNING cost_micros`,
		userID, s.today(), int64(math.Round(usd*1e6))).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("add spend: %w", err)
	}
	return float64(total) / 1e6, nil
}

// TotalSpentToday is everyone's model spend so far today, in USD.
func (s *Store) TotalSpentToday(ctx context.Context) (float64, error) {
	var micros int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost_micros), 0) FROM analyze_usage WHERE day = ?`, s.today()).Scan(&micros)
	return float64(micros) / 1e6, err
}

// SpentToday is the person's model spend so far today, in USD.
func (s *Store) SpentToday(ctx context.Context, userID int64) (float64, error) {
	var micros int64
	err := s.db.QueryRowContext(ctx,
		`SELECT cost_micros FROM analyze_usage WHERE user_id = ? AND day = ?`, userID, s.today()).Scan(&micros)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return float64(micros) / 1e6, err
}

// TakeAnalysis spends one model call from today's allowance, or returns
// ErrLimit. It is taken before the call so concurrent uploads can't overrun
// it; RefundAnalysis gives it back when the call fails on our side.
func (s *Store) TakeAnalysis(ctx context.Context, userID int64, limit int) error {
	if limit <= 0 {
		return nil
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO analyze_usage (user_id, day, count) VALUES (?, ?, 1)
		ON CONFLICT (user_id, day) DO UPDATE SET count = count + 1 WHERE count < ?`,
		userID, s.today(), limit)
	if err != nil {
		return fmt.Errorf("take analysis: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrLimit
	}
	return nil
}

func (s *Store) RefundAnalysis(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE analyze_usage SET count = count - 1 WHERE user_id = ? AND day = ? AND count > 0`,
		userID, s.today())
	return err
}

func (s *Store) AnalysesUsed(ctx context.Context, userID int64) (int, error) {
	var used int
	err := s.db.QueryRowContext(ctx,
		`SELECT count FROM analyze_usage WHERE user_id = ? AND day = ?`, userID, s.today()).Scan(&used)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return used, err
}

// --- profile ---------------------------------------------------------------

// Profile returns the user's profile, or ErrNotFound before they fill one in.
func (s *Store) Profile(ctx context.Context, userID int64) (model.Profile, error) {
	var (
		p        model.Profile
		sex, act string
		updated  int64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT sex, birth_year, height_cm, weight_kg, goal_weight_kg, activity, pace_kg_week, updated_at
		FROM profiles WHERE user_id = ?`, userID).
		Scan(&sex, &p.BirthYear, &p.HeightCm, &p.WeightKg, &p.GoalWeightKg, &act, &p.PaceKgWeek, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Profile{}, ErrNotFound
	}
	if err != nil {
		return model.Profile{}, fmt.Errorf("query profile: %w", err)
	}
	p.Sex, p.Activity = model.Sex(sex), model.Activity(act)
	p.UpdatedAt = time.Unix(updated, 0).UTC()
	return p, nil
}

// SaveProfile validates and stores the whole profile, replacing any earlier one.
func (s *Store) SaveProfile(ctx context.Context, userID int64, p model.Profile) (model.Profile, error) {
	now := s.now()
	p = p.Normalize()
	if err := p.Validate(now); err != nil {
		return model.Profile{}, err
	}
	p.UpdatedAt = time.Unix(now.Unix(), 0).UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO profiles (user_id, sex, birth_year, height_cm, weight_kg, goal_weight_kg, activity, pace_kg_week, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			sex = excluded.sex, birth_year = excluded.birth_year, height_cm = excluded.height_cm,
			weight_kg = excluded.weight_kg, goal_weight_kg = excluded.goal_weight_kg,
			activity = excluded.activity, pace_kg_week = excluded.pace_kg_week, updated_at = excluded.updated_at`,
		userID, p.Sex, p.BirthYear, p.HeightCm, p.WeightKg, p.GoalWeightKg, p.Activity, p.PaceKgWeek, p.UpdatedAt.Unix())
	if err != nil {
		return model.Profile{}, fmt.Errorf("save profile: %w", err)
	}
	return p, nil
}

// --- meals -----------------------------------------------------------------

// MealInput is what a person submits: the edited draft plus when they ate it.
type MealInput struct {
	EatenAt time.Time
	Title   string
	Note    string
	Items   []model.Item
	// Photo is an optional preview, set when the meal is created; an update
	// keeps the one there is, so a cached preview never goes stale.
	Photo []byte
	// PhotoLarge is the same photo bigger, for the meal's own screen; same
	// rules as Photo.
	PhotoLarge []byte
}

// MaxPhoto caps a meal's preview; a 320 px JPEG is about 20 KB.
// MaxPhotoLarge caps the big one; 1080 px is about 100 KB.
const (
	MaxPhoto      = 64 << 10
	MaxPhotoLarge = 400 << 10
)

func (in MealInput) validate() error {
	if in.EatenAt.IsZero() {
		return fmt.Errorf("%w: eatenAt is required", ErrInvalid)
	}
	if err := model.ValidateText("title", in.Title, model.MaxTitleRune); err != nil {
		return err
	}
	if err := model.ValidateText("note", in.Note, model.MaxNoteRunes); err != nil {
		return err
	}
	for _, photo := range []struct {
		data []byte
		max  int
	}{{in.Photo, MaxPhoto}, {in.PhotoLarge, MaxPhotoLarge}} {
		if photo.data == nil {
			continue
		}
		if len(photo.data) > photo.max {
			return fmt.Errorf("%w: photo is larger than %d KB", ErrInvalid, photo.max>>10)
		}
		switch http.DetectContentType(photo.data) {
		case "image/jpeg", "image/webp", "image/png":
		default:
			return fmt.Errorf("%w: photo must be JPEG, WebP or PNG", ErrInvalid)
		}
	}
	return model.ValidateItems(in.Items)
}

func (s *Store) CreateMeal(ctx context.Context, userID int64, in MealInput) (model.Meal, error) {
	if err := in.validate(); err != nil {
		return model.Meal{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Meal{}, err
	}
	defer tx.Rollback()

	now := s.now().Unix()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO meals (user_id, eaten_at, title, note, photo, photo_large, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, in.EatenAt.Unix(), strings.TrimSpace(in.Title), strings.TrimSpace(in.Note), in.Photo, in.PhotoLarge, now, now)
	if err != nil {
		return model.Meal{}, fmt.Errorf("insert meal: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.Meal{}, err
	}
	if err := insertItems(ctx, tx, id, in.Items); err != nil {
		return model.Meal{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Meal{}, err
	}
	return s.Meal(ctx, userID, id)
}

// UpdateMeal replaces a meal's fields and items wholesale: the editor always
// sends the complete meal, so there is nothing to merge.
func (s *Store) UpdateMeal(ctx context.Context, userID, id int64, in MealInput) (model.Meal, error) {
	if err := in.validate(); err != nil {
		return model.Meal{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Meal{}, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE meals SET eaten_at = ?, title = ?, note = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		in.EatenAt.Unix(), strings.TrimSpace(in.Title), strings.TrimSpace(in.Note), s.now().Unix(), id, userID)
	if err != nil {
		return model.Meal{}, fmt.Errorf("update meal: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return model.Meal{}, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM meal_items WHERE meal_id = ?`, id); err != nil {
		return model.Meal{}, err
	}
	if err := insertItems(ctx, tx, id, in.Items); err != nil {
		return model.Meal{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Meal{}, err
	}
	return s.Meal(ctx, userID, id)
}

func (s *Store) DeleteMeal(ctx context.Context, userID, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM meals WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete meal: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MealPhoto returns a meal's preview, or ErrNotFound when it has none. With
// large it returns the big copy, falling back to the preview for meals saved
// before there was one.
func (s *Store) MealPhoto(ctx context.Context, userID, id int64, large bool) ([]byte, error) {
	column := "photo"
	if large {
		column = "COALESCE(photo_large, photo)"
	}
	var photo []byte
	err := s.db.QueryRowContext(ctx, `SELECT `+column+` FROM meals WHERE id = ? AND user_id = ?`, id, userID).Scan(&photo)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && photo == nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query meal photo: %w", err)
	}
	return photo, nil
}

func (s *Store) Meal(ctx context.Context, userID, id int64) (model.Meal, error) {
	meals, err := s.queryMeals(ctx, `WHERE m.user_id = ? AND m.id = ?`, userID, id)
	if err != nil {
		return model.Meal{}, err
	}
	if len(meals) == 0 {
		return model.Meal{}, ErrNotFound
	}
	return meals[0], nil
}

// Meals returns the user's meals eaten in [from, to), oldest first. The
// client passes its own local-day bounds, so the server needs no time zone.
func (s *Store) Meals(ctx context.Context, userID int64, from, to time.Time) ([]model.Meal, error) {
	if !to.After(from) {
		return nil, fmt.Errorf("%w: to must be after from", ErrInvalid)
	}
	if to.Sub(from) > 62*24*time.Hour {
		return nil, fmt.Errorf("%w: range is limited to 62 days", ErrInvalid)
	}
	return s.queryMeals(ctx, `WHERE m.user_id = ? AND m.eaten_at >= ? AND m.eaten_at < ?`,
		userID, from.Unix(), to.Unix())
}

// Regular is a meal the person keeps coming back to: its latest instance
// and how many times a meal with the same foods was saved.
type Regular struct {
	Meal  model.Meal `json:"meal"`
	Times int        `json:"times"`
}

// RegularMeals returns up to limit of the user's meals since the given time,
// one per distinct set of foods, most often eaten first and most recent among
// equals. They are what "eat this again" offers.
func (s *Store) RegularMeals(ctx context.Context, userID int64, since time.Time, limit int) ([]Regular, error) {
	meals, err := s.queryMeals(ctx, `WHERE m.user_id = ? AND m.eaten_at >= ?`, userID, since.Unix())
	if err != nil {
		return nil, err
	}
	byFoods := map[string]*Regular{}
	var order []string
	for _, meal := range meals { // oldest first, so the last one seen is the latest
		key := foodsKey(meal.Items)
		if r, ok := byFoods[key]; ok {
			r.Meal, r.Times = meal, r.Times+1
			continue
		}
		byFoods[key] = &Regular{Meal: meal, Times: 1}
		order = append(order, key)
	}
	regulars := make([]Regular, 0, len(order))
	for _, key := range order {
		regulars = append(regulars, *byFoods[key])
	}
	sort.SliceStable(regulars, func(i, j int) bool {
		if regulars[i].Times != regulars[j].Times {
			return regulars[i].Times > regulars[j].Times
		}
		return regulars[i].Meal.EatenAt.After(regulars[j].Meal.EatenAt)
	})
	if len(regulars) > limit {
		regulars = regulars[:limit]
	}
	return regulars, nil
}

// foodsKey names a meal by its foods alone, so the same breakfast with a
// slightly different weight still counts as the same breakfast.
func foodsKey(items []model.Item) string {
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = strings.ToLower(strings.TrimSpace(item.Name))
	}
	sort.Strings(names)
	return strings.Join(names, "\x00")
}

// RecentItems returns up to limit items the user saved in meals eaten since
// the given time, newest first. Saved items are the ones a person checked and
// corrected, so they are the best evidence of that person's real portions.
func (s *Store) RecentItems(ctx context.Context, userID int64, since time.Time, limit int) ([]model.Item, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.name, i.grams, i.kcal_100, i.protein_100, i.fat_100, i.carbs_100
		FROM meals m JOIN meal_items i ON i.meal_id = m.id
		WHERE m.user_id = ? AND m.eaten_at >= ?
		ORDER BY m.eaten_at DESC, m.id DESC, i.position
		LIMIT ?`, userID, since.Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("query recent items: %w", err)
	}
	defer rows.Close()

	var items []model.Item
	for rows.Next() {
		var item model.Item
		if err := rows.Scan(&item.Name, &item.Grams, &item.Kcal100, &item.Protein100, &item.Fat100, &item.Carbs100); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) queryMeals(ctx context.Context, where string, args ...any) ([]model.Meal, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.eaten_at, m.title, m.note, m.photo IS NOT NULL, m.created_at, m.updated_at,
		       i.name, i.grams, i.kcal_100, i.protein_100, i.fat_100, i.carbs_100, i.confidence
		FROM meals m JOIN meal_items i ON i.meal_id = m.id
		`+where+`
		ORDER BY m.eaten_at, m.id, i.position`, args...)
	if err != nil {
		return nil, fmt.Errorf("query meals: %w", err)
	}
	defer rows.Close()

	var meals []model.Meal
	for rows.Next() {
		var (
			id                        int64
			eatenAt, created, updated int64
			title, note               string
			hasPhoto                  bool
			item                      model.Item
			confidence                string
		)
		if err := rows.Scan(&id, &eatenAt, &title, &note, &hasPhoto, &created, &updated,
			&item.Name, &item.Grams, &item.Kcal100, &item.Protein100, &item.Fat100, &item.Carbs100, &confidence); err != nil {
			return nil, err
		}
		item.Confidence = model.Confidence(confidence)
		if len(meals) == 0 || meals[len(meals)-1].ID != id {
			meals = append(meals, model.Meal{
				ID:        id,
				EatenAt:   time.Unix(eatenAt, 0).UTC(),
				Title:     title,
				Note:      note,
				HasPhoto:  hasPhoto,
				CreatedAt: time.Unix(created, 0).UTC(),
				UpdatedAt: time.Unix(updated, 0).UTC(),
			})
		}
		last := &meals[len(meals)-1]
		last.Items = append(last.Items, item)
	}
	return meals, rows.Err()
}

func insertItems(ctx context.Context, tx *sql.Tx, mealID int64, items []model.Item) error {
	for position, item := range items {
		// Typed in by hand, or saved after the owner saw the model's "low"
		// (unsure, please check): either way the weight stands checked.
		confidence := item.Confidence
		if confidence == "" || confidence == model.ConfidenceLow {
			confidence = model.ConfidenceHigh
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO meal_items (meal_id, position, name, grams, kcal_100, protein_100, fat_100, carbs_100, confidence)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			mealID, position, strings.TrimSpace(item.Name), item.Grams, item.Kcal100,
			item.Protein100, item.Fat100, item.Carbs100, string(confidence)); err != nil {
			return fmt.Errorf("insert meal item: %w", err)
		}
	}
	return nil
}
