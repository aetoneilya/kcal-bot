package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/coach"
	"github.com/aetoneilya/kcal-bot/api/internal/model"
	"github.com/aetoneilya/kcal-bot/api/internal/store"
	"github.com/aetoneilya/kcal-bot/api/internal/telegram"
	"github.com/aetoneilya/kcal-bot/api/internal/treats"
)

// ReminderStore is the part of the store the reminder loop needs.
type ReminderStore interface {
	Subscribers(ctx context.Context) ([]store.Subscriber, error)
	Meals(ctx context.Context, userID int64, from, to time.Time) ([]model.Meal, error)
	Profile(ctx context.Context, userID int64) (model.Profile, error)
	Weights(ctx context.Context, userID int64, from, to string) ([]model.Weight, error)
	MarkSent(ctx context.Context, userID int64, kind, day string) error
	StopReminders(ctx context.Context, userID int64) error
	AddSpend(ctx context.Context, userID int64, usd float64) (float64, error)
}

// Reminders sends the messages people opted into, on their own clocks: the
// week's summary on Sunday at the time they chose, and a nudge at midday
// when nothing is logged yet. Each goes out at most once a local day.
type Reminders struct {
	Client *telegram.Client
	Store  ReminderStore
	AppURL string
	// Coach writes the warm lines under the week's numbers. Nil uses
	// coach.Fallback.
	Coach coach.Coach
	// Treats adds a compliment to the messages of people who get them.
	Treats *treats.Treats
	Log    *slog.Logger
}

// NudgeAt is the local hour of the "nothing logged yet" reminder.
const NudgeAt = 14

// SummaryDay is when the week's summary goes out, closing the week.
const SummaryDay = time.Sunday

// A summary is sent only within this long after its time, so a server that
// was down at 21:00 does not wake someone at 3 in the morning.
const summaryWindow = 3 * time.Hour

func (r *Reminders) log() *slog.Logger {
	if r.Log != nil {
		return r.Log
	}
	return slog.Default()
}

// Run checks once a minute until ctx is cancelled.
func (r *Reminders) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		r.Tick(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Tick sends whatever is due at now. Exported so tests can drive the clock.
func (r *Reminders) Tick(ctx context.Context, now time.Time) {
	subs, err := r.Store.Subscribers(ctx)
	if err != nil {
		r.log().Warn("could not list reminder subscribers", "err", err)
		return
	}
	for _, sub := range subs {
		loc, err := time.LoadLocation(sub.Reminders.TimeZone)
		if err != nil {
			continue
		}
		local := now.In(loc)
		day := local.Format(time.DateOnly)
		start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)

		if at, ok := clock(start, sub.Reminders.SummaryAt); ok && local.Weekday() == SummaryDay &&
			sub.LastSummaryDay != day && !local.Before(at) && local.Before(at.Add(summaryWindow)) {
			r.summary(ctx, sub, start, day, now)
		}
		if sub.Reminders.Nudge && sub.LastNudgeDay != day && local.Hour() == NudgeAt {
			r.nudge(ctx, sub, start, day)
		}
	}
}

func clock(day time.Time, hhmm string) (time.Time, bool) {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return time.Time{}, false
	}
	return day.Add(time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute), true
}

// summary sends the last seven local days: how many were logged, the
// average against the target, the weight trend, and a few warm lines. A week
// with nothing logged stays quiet.
func (r *Reminders) summary(ctx context.Context, sub store.Subscriber, start time.Time, day string, now time.Time) {
	from := start.AddDate(0, 0, -6)
	meals, err := r.Store.Meals(ctx, sub.User.ID, from, start.Add(24*time.Hour))
	if err != nil {
		r.log().Warn("could not read the week for a summary", "user", sub.User.ID, "err", err)
		return
	}
	week := coach.Week{Name: sub.User.FirstName}
	_, number := start.ISOWeek()
	week.Number = number
	days := map[string]*[2]float64{} // local day → kcal, protein
	for _, meal := range meals {
		key := meal.EatenAt.In(start.Location()).Format(time.DateOnly)
		if days[key] == nil {
			days[key] = &[2]float64{}
		}
		for _, item := range meal.Items {
			days[key][0] += item.Kcal()
			days[key][1] += item.Grams / 100 * item.Protein100
		}
	}
	if len(days) == 0 {
		// Mark it so an abandoned diary is not checked again every minute.
		_ = r.Store.MarkSent(ctx, sub.User.ID, "summary", day)
		return
	}
	week.Logged = len(days)
	for _, d := range days {
		week.Kcal += d[0] / float64(week.Logged)
		week.Protein += d[1] / float64(week.Logged)
	}
	if profile, err := r.Store.Profile(ctx, sub.User.ID); err == nil {
		week.Sex = profile.Sex
		week.Target = model.DailyTarget(profile, now)
		for _, d := range days {
			if math.Abs(d[0]-week.Target) <= week.Target*0.1 {
				week.OnTarget++
			}
		}
	}
	// The newest weigh-in against the last one at least a week older.
	weights, err := r.Store.Weights(ctx, sub.User.ID, start.AddDate(0, 0, -21).Format(time.DateOnly), day)
	if err == nil && len(weights) > 0 {
		latest := weights[len(weights)-1]
		week.Weight = &latest.WeightKg
		cut := start.AddDate(0, 0, -7).Format(time.DateOnly)
		for i := len(weights) - 1; i >= 0; i-- {
			if weights[i].Day <= cut {
				change := latest.WeightKg - weights[i].WeightKg
				week.Change = &change
				break
			}
		}
	}

	text := "Итог недели 🐱\n\n" + weekNumbers(week) + "\n\n" + r.motivate(ctx, sub.User, week)
	r.send(ctx, sub, "summary", day, text)
}

func weekNumbers(w coach.Week) string {
	lines := []string{fmt.Sprintf("Записано дней: %d из 7", w.Logged)}
	avg := fmt.Sprintf("В среднем %s ккал в день", thousands(w.Kcal))
	if w.Target > 0 {
		avg += fmt.Sprintf(" при норме %s, в норме %s", thousands(w.Target), plural(w.OnTarget, "день", "дня", "дней"))
	}
	lines = append(lines, avg, fmt.Sprintf("Белок — %.0f г в день", w.Protein))
	if w.Weight != nil {
		weight := fmt.Sprintf("Вес %s кг", kg(*w.Weight))
		if w.Change != nil {
			sign := "+"
			if *w.Change < 0 {
				sign = "−"
			}
			if math.Abs(*w.Change) < 0.05 {
				weight += ", без изменений за неделю"
			} else {
				weight += fmt.Sprintf(", %s%s за неделю", sign, kg(math.Abs(*w.Change)))
			}
		}
		lines = append(lines, weight)
	}
	return strings.Join(lines, "\n")
}

// motivate asks the coach, falling back to a fixed tip; its cost counts
// toward the person's spend like an estimate.
func (r *Reminders) motivate(ctx context.Context, user model.User, week coach.Week) string {
	if r.Coach == nil {
		return coach.Fallback(week)
	}
	text, cost, err := r.Coach.Motivate(ctx, week)
	if cost > 0 {
		if _, spendErr := r.Store.AddSpend(ctx, user.ID, cost); spendErr != nil {
			r.log().Warn("could not record coach spend", "user", user.ID, "err", spendErr)
		}
	}
	if err != nil {
		r.log().Warn("coach failed, using a fixed tip", "user", user.ID, "err", err)
		return coach.Fallback(week)
	}
	r.log().Info("coached", "user", user.ID, "usd", fmt.Sprintf("%.4f", cost))
	return text
}

func kg(v float64) string {
	return strings.Replace(strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64), ".", ",", 1)
}

// plural writes n with the Russian form for one, few or many.
func plural(n int, one, few, many string) string {
	form := many
	switch {
	case n%10 == 1 && n%100 != 11:
		form = one
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		form = few
	}
	return fmt.Sprintf("%d %s", n, form)
}

func (r *Reminders) nudge(ctx context.Context, sub store.Subscriber, start time.Time, day string) {
	today, err := r.Store.Meals(ctx, sub.User.ID, start, start.Add(24*time.Hour))
	if err != nil || len(today) > 0 {
		return
	}
	// Only nudge someone who has been using the diary; an abandoned one
	// should stay quiet rather than nag.
	recent, err := r.Store.Meals(ctx, sub.User.ID, start.AddDate(0, 0, -3), start)
	if err != nil || len(recent) == 0 {
		return
	}
	r.send(ctx, sub, "nudge", day, "Сегодня ещё ничего не записано. Сфотографируй, что ел — займёт полминуты.")
}

func (r *Reminders) send(ctx context.Context, sub store.Subscriber, kind, day, text string) {
	if r.Treats.For(sub.User) {
		if compliment := r.Treats.Compliment(); compliment != "" {
			text += "\n\n" + compliment
		}
	}
	request := telegram.SendMessageRequest{ChatID: sub.User.ID, Text: text}
	if r.AppURL != "" {
		request.ReplyMarkup = telegram.OpenAppKeyboard(buttonText, r.AppURL)
	}
	err := r.Client.SendMessage(ctx, request)
	var apiErr *telegram.APIError
	switch {
	case errors.As(err, &apiErr) && !apiErr.Retryable():
		// The person blocked the bot or never started it: stop for good.
		r.log().Info("reminders stopped", "user", sub.User.ID, "err", err)
		_ = r.Store.StopReminders(ctx, sub.User.ID)
		return
	case err != nil:
		r.log().Warn("could not send a reminder", "user", sub.User.ID, "kind", kind, "err", err)
		return // retried next minute
	}
	if err := r.Store.MarkSent(ctx, sub.User.ID, kind, day); err != nil {
		r.log().Warn("could not record a sent reminder", "user", sub.User.ID, "err", err)
	}
}

// thousands prints whole kcal with a space between thousands, as in
// "1 850".
func thousands(v float64) string {
	n := int(v + 0.5)
	if n < 1000 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("%d %03d", n/1000, n%1000)
}
