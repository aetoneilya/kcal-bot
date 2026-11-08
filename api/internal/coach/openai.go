package coach

import (
	"context"

	"github.com/aetoneilya/kcal-bot/api/internal/estimate"
)

// OpenAI keeps weekly encouragement available when OpenAI is the selected
// backend, without requiring a second provider's key or balance.
type OpenAI struct {
	client *estimate.OpenAI
}

func NewOpenAI(apiKey, modelID string) *OpenAI {
	return &OpenAI{client: estimate.NewOpenAI(apiKey, modelID)}
}

func (c *OpenAI) Motivate(ctx context.Context, week Week) (Result, error) {
	result, err := c.client.Write(ctx, systemPrompt, describe(week))
	return Result{Text: result.Raw, Cost: result.Cost, Usage: result.Usage}, err
}
