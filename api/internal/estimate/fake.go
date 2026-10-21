package estimate

import (
	"context"
	"strings"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/model"
)

// Fake answers without any API key, so the whole app can be exercised locally.
// It ignores the photo and returns a fixed plate, asking about the oil unless
// the note or an answer has already settled it.
type Fake struct {
	// Delay imitates model latency so the loading state is visible.
	Delay time.Duration
	// Cost is reported for every call, to exercise spend limits locally.
	Cost float64
}

func (Fake) Name() string { return "fake" }

func (Fake) Priced() bool { return true }

func (f Fake) Estimate(ctx context.Context, in Input) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-time.After(f.Delay):
	}
	draft := model.Draft{
		Title: "Гречка с курицей",
		Items: []model.Item{
			{Name: "гречка отварная", Grams: 180, Kcal100: 110, Protein100: 4.2, Fat100: 1.1, Carbs100: 21.3, Confidence: model.ConfidenceMedium},
			{Name: "куриное бедро запечённое без кожи", Grams: 150, Kcal100: 185, Protein100: 24, Fat100: 9.8, Carbs100: 0, Confidence: model.ConfidenceMedium},
			{Name: "огурец", Grams: 80, Kcal100: 15, Protein100: 0.8, Fat100: 0.1, Carbs100: 2.8, Confidence: model.ConfidenceHigh},
		},
	}
	said := strings.ToLower(in.Note + " " + in.Answer)
	switch {
	case strings.Contains(said, "без масл"):
		// Settled: no oil.
	case strings.Contains(said, "масл"):
		draft.Items = append(draft.Items, model.Item{
			Name: "масло подсолнечное", Grams: 10, Kcal100: 899, Fat100: 99.9, Confidence: model.ConfidenceMedium,
		})
	default:
		draft.Items = append(draft.Items, model.Item{
			Name: "масло подсолнечное", Grams: 10, Kcal100: 899, Fat100: 99.9, Confidence: model.ConfidenceLow,
		})
		draft.Question = "Курицу готовили на масле?"
		draft.Options = []string{"Да, на масле", "Без масла"}
	}
	return Result{Draft: draft.Normalize(), Cost: f.Cost}, nil
}
