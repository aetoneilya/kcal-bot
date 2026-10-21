package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/model"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "kcal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	for _, id := range []int64{1, 2} {
		if _, err := st.EnsureUser(context.Background(), model.User{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func sampleMeal(at time.Time) MealInput {
	return MealInput{
		EatenAt: at,
		Title:   "Гречка с курицей",
		Items: []model.Item{
			{Name: "гречка", Grams: 180, Kcal100: 110, Protein100: 4.2, Fat100: 1.1, Carbs100: 21.3, Confidence: model.ConfidenceMedium},
			{Name: "курица", Grams: 150, Kcal100: 185, Protein100: 24, Fat100: 9.8},
		},
	}
}

func TestMealRoundTripAndRange(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	day := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)

	created, err := st.CreateMeal(ctx, 1, sampleMeal(day.Add(13*time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Items) != 2 || created.Items[0].Name != "гречка" || created.Items[1].Confidence != model.ConfidenceHigh {
		t.Fatalf("items not stored in order with defaults: %+v", created.Items)
	}
	if _, err := st.CreateMeal(ctx, 1, sampleMeal(day.Add(-time.Hour))); err != nil {
		t.Fatal(err)
	}

	meals, err := st.Meals(ctx, 1, day, day.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(meals) != 1 || meals[0].ID != created.ID {
		t.Fatalf("range must include only the day's meal, got %+v", meals)
	}

	edit := sampleMeal(day.Add(14 * time.Hour))
	edit.Items = edit.Items[:1]
	edit.Items[0].Grams = 250
	updated, err := st.UpdateMeal(ctx, 1, created.ID, edit)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Items) != 1 || updated.Items[0].Grams != 250 || !updated.EatenAt.Equal(day.Add(14*time.Hour)) {
		t.Fatalf("update must replace items and time, got %+v", updated)
	}

	if err := st.DeleteMeal(ctx, 1, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Meal(ctx, 1, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted meal still readable: %v", err)
	}
}

func TestMealsAreScopedToTheirOwner(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	meal, err := st.CreateMeal(ctx, 1, sampleMeal(at))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.Meal(ctx, 2, meal.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user read the meal: %v", err)
	}
	if _, err := st.UpdateMeal(ctx, 2, meal.ID, sampleMeal(at)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user updated the meal: %v", err)
	}
	if err := st.DeleteMeal(ctx, 2, meal.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user deleted the meal: %v", err)
	}
	meals, err := st.Meals(ctx, 2, at.Add(-time.Hour), at.Add(time.Hour))
	if err != nil || len(meals) != 0 {
		t.Fatalf("another user listed the meal: %v %+v", err, meals)
	}
	if _, err := st.Meal(ctx, 1, meal.ID); err != nil {
		t.Fatalf("owner lost the meal: %v", err)
	}
}

func TestRecentItemsAreTheOwnersNewestFirst(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

	old := sampleMeal(at.Add(-90 * 24 * time.Hour))
	old.Items[0].Name = "давняя гречка"
	newer := sampleMeal(at)
	newer.Items[0].Grams = 220
	for _, meal := range []MealInput{old, sampleMeal(at.Add(-time.Hour)), newer} {
		if _, err := st.CreateMeal(ctx, 1, meal); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.CreateMeal(ctx, 2, sampleMeal(at)); err != nil {
		t.Fatal(err)
	}

	items, err := st.RecentItems(ctx, 1, at.Add(-60*24*time.Hour), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].Name != "гречка" || items[0].Grams != 220 || items[2].Grams != 180 {
		t.Fatalf("want the owner's newest three items within the window, got %+v", items)
	}
}

func TestProfileRoundTripAndValidation(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	st.now = func() time.Time { return time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC) }

	if _, err := st.Profile(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no profile yet must be ErrNotFound, got %v", err)
	}
	in := model.Profile{
		Sex: model.SexMale, BirthYear: 1995, HeightCm: 180.4, WeightKg: 82.36, GoalWeightKg: 76,
		Activity: model.ActivityModerate, PaceKgWeek: 0.5,
	}
	if _, err := st.SaveProfile(ctx, 1, in); err != nil {
		t.Fatal(err)
	}
	in.WeightKg = 81
	saved, err := st.SaveProfile(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.Profile(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != saved || got.HeightCm != 180 || got.WeightKg != 81 || got.Activity != model.ActivityModerate {
		t.Fatalf("profile must read back as saved and rounded, got %+v want %+v", got, saved)
	}
	if _, err := st.Profile(ctx, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user must not see the profile: %v", err)
	}

	bad := in
	bad.BirthYear = 2020
	if _, err := st.SaveProfile(ctx, 1, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a child's profile must be rejected, got %v", err)
	}
	bad = in
	bad.HeightCm = 1.8
	if _, err := st.SaveProfile(ctx, 1, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("height in metres must be rejected, got %v", err)
	}
}

func TestCreateMealRejectsInvalidInput(t *testing.T) {
	st := openTest(t)
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	cases := map[string]func(*MealInput){
		"no items":        func(in *MealInput) { in.Items = nil },
		"no time":         func(in *MealInput) { in.EatenAt = time.Time{} },
		"blank name":      func(in *MealInput) { in.Items[0].Name = "  " },
		"huge portion":    func(in *MealInput) { in.Items[0].Grams = 50000 },
		"denser than fat": func(in *MealInput) { in.Items[0].Kcal100 = 1200 },
		"bad confidence":  func(in *MealInput) { in.Items[0].Confidence = "sure" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := sampleMeal(at)
			mutate(&in)
			if _, err := st.CreateMeal(context.Background(), 1, in); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestDailyAnalysisAllowance(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	now := time.Date(2026, 10, 19, 23, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		if err := st.TakeAnalysis(ctx, 1, 2); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.TakeAnalysis(ctx, 1, 2); !errors.Is(err, ErrLimit) {
		t.Fatalf("third call must hit the limit, got %v", err)
	}
	if err := st.TakeAnalysis(ctx, 2, 2); err != nil {
		t.Fatalf("limits are per user: %v", err)
	}

	if err := st.RefundAnalysis(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := st.TakeAnalysis(ctx, 1, 2); err != nil {
		t.Fatalf("a refund must free one call: %v", err)
	}

	now = now.Add(2 * time.Hour) // next UTC day
	if used, err := st.AnalysesUsed(ctx, 1); err != nil || used != 0 {
		t.Fatalf("a new day starts at zero, got %d %v", used, err)
	}
}

func TestAccountsReportNewAndBlocked(t *testing.T) {
	ctx := context.Background()
	st := openTest(t) // users 1 and 2 already exist

	account, err := st.EnsureUser(ctx, model.User{ID: 3, Username: "new", FirstName: "Новый"})
	if err != nil || !account.Created || account.Blocked {
		t.Fatalf("first sight must report Created: %+v %v", account, err)
	}
	if account, _ = st.EnsureUser(ctx, model.User{ID: 3, Username: "renamed"}); account.Created {
		t.Fatalf("second sight must not report Created: %+v", account)
	}

	if err := st.SetBlocked(ctx, 3, true); err != nil {
		t.Fatal(err)
	}
	if account, _ = st.EnsureUser(ctx, model.User{ID: 3}); !account.Blocked {
		t.Fatalf("blocked user not reported: %+v", account)
	}
	if _, blocked, err := st.User(ctx, 3); err != nil || !blocked {
		t.Fatalf("User must report the block: %v %v", blocked, err)
	}
	if err := st.SetBlocked(ctx, 3, false); err != nil {
		t.Fatal(err)
	}
	if account, _ = st.EnsureUser(ctx, model.User{ID: 3}); account.Blocked {
		t.Fatalf("unblock did not stick: %+v", account)
	}
	if err := st.SetBlocked(ctx, 99, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("blocking an unknown id must be ErrNotFound, got %v", err)
	}
}

func TestSpendAddsUpPerUserPerDay(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	day := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return day }

	for _, usd := range []float64{0.0123, 0.0201} {
		if _, err := st.AddSpend(ctx, 1, usd); err != nil {
			t.Fatal(err)
		}
	}
	if spent, _ := st.SpentToday(ctx, 1); spent != 0.0324 {
		t.Fatalf("spend = %v, want 0.0324", spent)
	}
	if spent, _ := st.SpentToday(ctx, 2); spent != 0 {
		t.Fatalf("another user's spend leaked: %v", spent)
	}
	// Spend shares the row with the call count and must not reset it.
	if err := st.TakeAnalysis(ctx, 1, 5); err != nil {
		t.Fatal(err)
	}
	if used, _ := st.AnalysesUsed(ctx, 1); used != 1 {
		t.Fatalf("count = %d, want 1", used)
	}
	st.now = func() time.Time { return day.Add(24 * time.Hour) }
	if spent, _ := st.SpentToday(ctx, 1); spent != 0 {
		t.Fatalf("a new day must start at zero, got %v", spent)
	}
}
