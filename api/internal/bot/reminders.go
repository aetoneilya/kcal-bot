package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

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
	MarkSent(ctx context.Context, userID int64, kind, day string) error
	StopReminders(ctx context.Context, userID int64) error
}

// Reminders sends the messages people opted into, on their own clocks: the
// evening total at the time they chose, and a nudge at midday when nothing
// is logged yet. Each goes out at most once a local day.
type Reminders struct {
	Client *telegram.Client
	Store  ReminderStore
	AppURL string
	// Treats adds a compliment to the messages of people who get them.
	Treats *treats.Treats
	Log    *slog.Logger
}

// NudgeAt is the local hour of the "nothing logged yet" reminder.
const NudgeAt = 14

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

		if at, ok := clock(start, sub.Reminders.SummaryAt); ok && sub.LastSummaryDay != day &&
			!local.Before(at) && local.Before(at.Add(summaryWindow)) {
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

func (r *Reminders) summary(ctx context.Context, sub store.Subscriber, start time.Time, day string, now time.Time) {
	meals, err := r.Store.Meals(ctx, sub.User.ID, start, start.Add(24*time.Hour))
	if err != nil {
		r.log().Warn("could not read the day for a summary", "user", sub.User.ID, "err", err)
		return
	}
	var kcal, protein, fat, carbs float64
	for _, meal := range meals {
		for _, item := range meal.Items {
			k := item.Grams / 100
			kcal += k * item.Kcal100
			protein += k * item.Protein100
			fat += k * item.Fat100
			carbs += k * item.Carbs100
		}
	}
	var text string
	if len(meals) == 0 {
		text = "Сегодня в дневнике пусто. Если что-то ел — добавь, пока помнишь."
	} else {
		text = fmt.Sprintf("Итог дня: %s ккал", thousands(kcal))
		if profile, err := r.Store.Profile(ctx, sub.User.ID); err == nil {
			target := model.DailyTarget(profile, now)
			left := target - kcal
			text = fmt.Sprintf("Итог дня: %s из %s ккал", thousands(kcal), thousands(target))
			switch {
			case left >= 0:
				text += fmt.Sprintf(" — осталось %s", thousands(left))
			default:
				text += fmt.Sprintf(" — сверх нормы %s", thousands(-left))
			}
		}
		text += fmt.Sprintf(".\nБ %.0f · Ж %.0f · У %.0f г", protein, fat, carbs)
	}
	r.send(ctx, sub, "summary", day, text)
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
