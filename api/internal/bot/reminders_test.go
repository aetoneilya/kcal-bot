package bot

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/coach"
	"github.com/aetoneilya/kcal-bot/api/internal/model"
	"github.com/aetoneilya/kcal-bot/api/internal/store"
	"github.com/aetoneilya/kcal-bot/api/internal/telegram"
	"github.com/aetoneilya/kcal-bot/api/internal/treats"
)

func TestRemindersGoOutOnTheirLocalClockOnce(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "kcal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	var sent []telegram.SendMessageRequest
	forbidden := map[int64]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req telegram.SendMessageRequest
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &req)
		if forbidden[req.ChatID] {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`))
			return
		}
		sent = append(sent, req)
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	t.Cleanup(srv.Close)
	r := &Reminders{Client: &telegram.Client{Token: "t", BaseURL: srv.URL}, Store: st, AppURL: "https://kcal.example"}

	msk, _ := time.LoadLocation("Europe/Moscow")
	for _, id := range []int64{1, 2} {
		if _, err := st.EnsureUser(ctx, model.User{ID: id}); err != nil {
			t.Fatal(err)
		}
		if _, err := st.SaveReminders(ctx, id, model.Reminders{TimeZone: "Europe/Moscow", SummaryAt: "21:00", Nudge: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.SaveProfile(ctx, 1, model.Profile{Sex: model.SexMale, BirthYear: 1996, HeightCm: 180, WeightKg: 80,
		GoalWeightKg: 80, Activity: model.ActivityModerate}); err != nil {
		t.Fatal(err)
	}
	meal := func(at time.Time) store.MealInput {
		return store.MealInput{EatenAt: at, Items: []model.Item{{Name: "гречка", Grams: 1000, Kcal100: 100, Protein100: 4}}}
	}
	if _, err := st.CreateMeal(ctx, 1, meal(time.Date(2026, 10, 20, 9, 0, 0, 0, msk))); err != nil {
		t.Fatal(err)
	}

	// 14:30 in Moscow, nothing logged today but yesterday: nudge user 1 only
	// (user 2 never logged anything).
	r.Tick(ctx, time.Date(2026, 10, 21, 14, 30, 0, 0, msk))
	if len(sent) != 1 || sent[0].ChatID != 1 || !strings.Contains(sent[0].Text, "ничего не записано") {
		t.Fatalf("want one nudge to user 1, got %+v", sent)
	}
	r.Tick(ctx, time.Date(2026, 10, 21, 14, 45, 0, 0, msk))
	if len(sent) != 1 {
		t.Fatalf("a nudge goes out once a day, got %d", len(sent))
	}

	if _, err := st.CreateMeal(ctx, 1, meal(time.Date(2026, 10, 21, 15, 0, 0, 0, msk))); err != nil {
		t.Fatal(err)
	}
	// User 2 logs once too, then blocks the bot.
	if _, err := st.CreateMeal(ctx, 2, meal(time.Date(2026, 10, 24, 13, 0, 0, 0, msk))); err != nil {
		t.Fatal(err)
	}
	forbidden[2] = true

	// Wednesday evening: the summary is weekly now.
	r.Tick(ctx, time.Date(2026, 10, 21, 21, 1, 0, 0, msk))
	if len(sent) != 1 {
		t.Fatalf("no summary on a weekday, got %+v", sent[1:])
	}
	r.Tick(ctx, time.Date(2026, 10, 25, 20, 59, 0, 0, msk))
	if len(sent) != 1 {
		t.Fatal("no summary before its time")
	}
	r.Tick(ctx, time.Date(2026, 10, 25, 21, 1, 0, 0, msk))
	if len(sent) != 2 || sent[1].ChatID != 1 {
		t.Fatalf("want user 1's weekly summary, got %+v", sent)
	}
	for _, want := range []string{"Итог недели", "Записано дней: 2 из 7", "В среднем 1 000 ккал в день при норме 2 760, в норме 0 дней", "Белок — 40 г"} {
		if !strings.Contains(sent[1].Text, want) {
			t.Errorf("summary lacks %q:\n%s", want, sent[1].Text)
		}
	}
	if subs, _ := st.Subscribers(ctx); len(subs) != 1 || subs[0].User.ID != 1 {
		t.Fatalf("a user who blocked the bot must be unsubscribed, got %+v", subs)
	}
	r.Tick(ctx, time.Date(2026, 10, 25, 21, 30, 0, 0, msk))
	r.Tick(ctx, time.Date(2026, 11, 1, 2, 0, 0, 0, msk))
	if len(sent) != 2 {
		t.Fatal("one summary a week, and a missed one must not wake anyone at night")
	}
}

func TestRemindersCarryAComplimentForChosenPeople(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "kcal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var sent []telegram.SendMessageRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req telegram.SendMessageRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		sent = append(sent, req)
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compliments.txt"), []byte("Ты солнце\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Reminders{Client: &telegram.Client{Token: "t", BaseURL: srv.URL}, Store: st, Treats: treats.New(dir, "@Sunny")}
	for _, user := range []model.User{{ID: 1, Username: "sunny"}, {ID: 2, Username: "other"}} {
		if _, err := st.EnsureUser(ctx, user); err != nil {
			t.Fatal(err)
		}
		if _, err := st.SaveReminders(ctx, user.ID, model.Reminders{TimeZone: "UTC", SummaryAt: "21:00"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int64{1, 2} {
		if _, err := st.CreateMeal(ctx, id, store.MealInput{EatenAt: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC),
			Items: []model.Item{{Name: "суп", Grams: 300, Kcal100: 50}}}); err != nil {
			t.Fatal(err)
		}
	}
	r.Tick(ctx, time.Date(2026, 10, 25, 21, 0, 0, 0, time.UTC))
	if len(sent) != 2 {
		t.Fatalf("want two summaries, got %+v", sent)
	}
	for _, msg := range sent {
		has := strings.HasSuffix(msg.Text, "\n\nТы солнце")
		if has != (msg.ChatID == 1) {
			t.Errorf("user %d: compliment = %v in %q", msg.ChatID, has, msg.Text)
		}
	}
}

// fixedCoach stands in for the model.
type fixedCoach struct{ got []coach.Week }

func (c *fixedCoach) Motivate(_ context.Context, w coach.Week) (string, float64, error) {
	c.got = append(c.got, w)
	return "Ты молодец!", 0.002, nil
}

func TestWeeklySummaryAsksTheCoachAndCountsItsCost(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "kcal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var sent []telegram.SendMessageRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req telegram.SendMessageRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		sent = append(sent, req)
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	t.Cleanup(srv.Close)
	c := &fixedCoach{}
	r := &Reminders{Client: &telegram.Client{Token: "t", BaseURL: srv.URL}, Store: st, Coach: c}

	if _, err := st.EnsureUser(ctx, model.User{ID: 1, FirstName: "Соня"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveReminders(ctx, 1, model.Reminders{TimeZone: "UTC", SummaryAt: "20:00"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveProfile(ctx, 1, model.Profile{Sex: model.SexFemale, BirthYear: 1998, HeightCm: 165, WeightKg: 60,
		GoalWeightKg: 58, Activity: model.ActivityModerate, PaceKgWeek: 0.25}); err != nil {
		t.Fatal(err)
	}
	for _, w := range []model.Weight{{Day: "2026-10-17", WeightKg: 60.4}, {Day: "2026-10-25", WeightKg: 59.9}} {
		if _, err := st.SaveWeight(ctx, 1, w); err != nil {
			t.Fatal(err)
		}
	}
	for d := 19; d <= 25; d++ {
		if _, err := st.CreateMeal(ctx, 1, store.MealInput{EatenAt: time.Date(2026, 10, d, 13, 0, 0, 0, time.UTC),
			Items: []model.Item{{Name: "обед", Grams: 500, Kcal100: 360, Protein100: 18}}}); err != nil {
			t.Fatal(err)
		}
	}

	r.Tick(ctx, time.Date(2026, 10, 25, 20, 5, 0, 0, time.UTC))
	if len(sent) != 1 || !strings.HasSuffix(sent[0].Text, "\n\nТы молодец!") || !strings.Contains(sent[0].Text, "Вес 59,9 кг, −0,5 за неделю") {
		t.Fatalf("got %+v", sent)
	}
	w := c.got[0]
	if w.Name != "Соня" || w.Sex != model.SexFemale || w.Logged != 7 || math.Round(w.Kcal) != 1800 || math.Round(w.Protein) != 90 || w.OnTarget != 7 {
		t.Fatalf("coach got %+v", w)
	}
	if spent, _ := st.SpentToday(ctx, 1); spent < 0.002 {
		t.Fatalf("coach cost not recorded: %v", spent)
	}
}
