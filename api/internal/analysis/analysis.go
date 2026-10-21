// Package analysis runs one model estimate for a person within their limits:
// the daily count, their own spend and everyone's spend together. The Mini
// App's upload and photos sent to the bot both go through it, so the limits
// hold however a meal arrives.
package analysis

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/estimate"
	"github.com/aetoneilya/kcal-bot/api/internal/model"
	"github.com/aetoneilya/kcal-bot/api/internal/store"
)

// Store is the part of the store an estimate needs.
type Store interface {
	SpentToday(ctx context.Context, userID int64) (float64, error)
	TotalSpentToday(ctx context.Context) (float64, error)
	AddSpend(ctx context.Context, userID int64, usd float64) (float64, error)
	TakeAnalysis(ctx context.Context, userID int64, limit int) error
	RefundAnalysis(ctx context.Context, userID int64) error
	RecentItems(ctx context.Context, userID int64, since time.Time, limit int) ([]model.Item, error)
}

// Notifier hears when a spend limit is reached. Implementations must return
// quickly.
type Notifier interface {
	SpendLimitReached(ctx context.Context, user model.User, spent, limit float64)
	TotalSpendLimitReached(ctx context.Context, spent, limit float64)
}

type Analyzer struct {
	Store     Store
	Estimator estimate.Estimator
	// DailyLimit caps model calls per user per UTC day; 0 means no cap.
	DailyLimit int
	// DailySpendLimit caps one user's model spend per UTC day, USD; 0 means
	// no cap.
	DailySpendLimit float64
	// TotalDailySpendLimit caps everyone's spend together per UTC day, USD;
	// 0 means no cap.
	TotalDailySpendLimit float64
	// Notifier may be nil.
	Notifier Notifier
	Log      *slog.Logger
}

var (
	// ErrSpendLimit: this person has spent their day's budget.
	ErrSpendLimit = errors.New("daily spend limit reached")
	// ErrPaused: everyone together has spent the day's budget.
	ErrPaused = errors.New("analysis paused for today")
	// ErrModel: the model failed or timed out; the call was refunded.
	ErrModel = errors.New("the model did not answer")
)

// Portion history sent to the model: recent enough to reflect current habits,
// small enough to keep each call cheap (40 lines is ~600 input tokens).
const (
	historyWindow   = 60 * 24 * time.Hour
	historyScan     = 500
	historyPortions = 40
)

// Analyze estimates one meal. Besides the sentinels above it returns
// store.ErrLimit when the day's count is used up and estimate.ErrUnusable
// when the model's answer could not be read; neither costs the person a call.
func (a *Analyzer) Analyze(ctx context.Context, user model.User, in estimate.Input) (model.Draft, error) {
	log := a.Log
	if log == nil {
		log = slog.Default()
	}
	spent, err := a.Store.SpentToday(ctx, user.ID)
	if err != nil {
		return model.Draft{}, err
	}
	if a.DailySpendLimit > 0 && spent >= a.DailySpendLimit {
		return model.Draft{}, ErrSpendLimit
	}
	totalBefore, err := a.Store.TotalSpentToday(ctx)
	if err != nil {
		return model.Draft{}, err
	}
	if a.TotalDailySpendLimit > 0 && totalBefore >= a.TotalDailySpendLimit {
		return model.Draft{}, ErrPaused
	}
	if err := a.Store.TakeAnalysis(ctx, user.ID, a.DailyLimit); err != nil {
		return model.Draft{}, err
	}

	// The person's own corrected portions anchor the model's weight guesses.
	// Without them the estimate is still useful, so a failure only logs.
	recent, err := a.Store.RecentItems(ctx, user.ID, time.Now().Add(-historyWindow), historyScan)
	if err != nil {
		log.Warn("could not load portion history", "user", user.ID, "err", err)
	}
	in.History = estimate.Portions(recent, historyPortions)

	// Detach from the caller so a closed Mini App does not waste a call we
	// already paid for halfway, but still bound it.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
	defer cancel()
	started := time.Now()
	result, err := a.Estimator.Estimate(ctx, in)
	a.recordSpend(ctx, log, user, spent, totalBefore, result.Cost)
	if err != nil {
		// A failure on our side (or the model's) should not eat the allowance.
		if refundErr := a.Store.RefundAnalysis(ctx, user.ID); refundErr != nil {
			log.Error("could not refund analysis", "user", user.ID, "err", refundErr)
		}
		log.Error("estimate failed", "estimator", a.Estimator.Name(), "user", user.ID, "ms", time.Since(started).Milliseconds(), "err", err)
		if errors.Is(err, estimate.ErrUnusable) {
			return model.Draft{}, err
		}
		return model.Draft{}, fmt.Errorf("%w: %v", ErrModel, err)
	}
	draft := result.Draft
	log.Info("estimated", "estimator", a.Estimator.Name(), "user", user.ID,
		"items", len(draft.Items), "photos", len(in.Photos), "ms", time.Since(started).Milliseconds(),
		"usd", fmt.Sprintf("%.4f", result.Cost), "tokens_in", result.Usage.Input, "tokens_out", result.Usage.Output,
		"history", len(in.History))
	if len(draft.Items) == 0 {
		// An empty plate is either a non-food photo or a model misfire; the
		// answer text tells which. It holds no personal data beyond the meal.
		log.Warn("estimate found no food", "user", user.ID, "raw", truncate(result.Raw, 500))
	}
	return draft, nil
}

func (a *Analyzer) recordSpend(ctx context.Context, log *slog.Logger, user model.User, before, totalBefore, cost float64) {
	if cost <= 0 {
		return
	}
	total, err := a.Store.AddSpend(ctx, user.ID, cost)
	if err != nil {
		log.Error("could not record spend", "user", user.ID, "usd", cost, "err", err)
		return
	}
	if a.Notifier == nil {
		return
	}
	if limit := a.DailySpendLimit; limit > 0 && before < limit && total >= limit {
		a.Notifier.SpendLimitReached(ctx, user, total, limit)
	}
	// Concurrent calls can each see the old total; the one that pushes it
	// over announces it, and a rare duplicate notice is harmless.
	if limit := a.TotalDailySpendLimit; limit > 0 && totalBefore < limit && totalBefore+cost >= limit {
		a.Notifier.TotalSpendLimitReached(ctx, totalBefore+cost, limit)
	}
}

func truncate(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// Compile-time check that the real store fits.
var _ Store = (*store.Store)(nil)
