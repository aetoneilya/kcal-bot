// Package coach writes the weekly summary's few warm lines: praise for
// something real in the week's numbers and one healthy habit to try. A model
// writes them when there is a key; otherwise a fixed tip rotates by week.
package coach

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aetoneilya/kcal-bot/api/internal/estimate"
	"github.com/aetoneilya/kcal-bot/api/internal/model"
)

// Week is what the person did over the last seven days.
type Week struct {
	Name   string    // first name; may be empty
	Sex    model.Sex // empty when there is no profile
	Logged int       // days with at least one meal, of 7
	// Averages over the logged days.
	Kcal, Protein float64
	// Target is the day's calories; 0 without a profile. OnTarget counts
	// logged days within 10% of it.
	Target   float64
	OnTarget int
	// Weight is the newest weigh-in and Change its difference from a week
	// before; nil when there are none.
	Weight, Change *float64
	// Number picks the fallback tip, so it changes week to week.
	Number int
}

// Coach writes the motivating part of the weekly summary.
type Coach interface {
	// Motivate returns the text and what it cost in USD.
	Motivate(ctx context.Context, week Week) (string, float64, error)
}

type Claude struct {
	client anthropic.Client
	model  string
	price  estimate.Price
}

// NewClaude writes with modelID, default claude-sonnet-5-5.
func NewClaude(apiKey, modelID string) *Claude {
	if modelID == "" {
		modelID = "claude-sonnet-5-5"
	}
	price, _ := estimate.ClaudePrice(modelID)
	return &Claude{client: estimate.NewAnthropicClient(apiKey), model: modelID, price: price}
}

const systemPrompt = `You are KalorieKitten, the warm cat behind a photo food diary in Telegram. Once a week you write the closing lines of the person's weekly summary, in Russian. The numbers are already shown above your text; do not repeat them as a list.

Write 2 to 4 short sentences, plain text:
- Praise one specific, real thing from the week (consistency, protein, staying near the target, a weight trend in the right direction). If the week was patchy, be kind about it and focus on the next small step.
- Give exactly one concrete, doable healthy-habit idea for the coming week: sleep, water, vegetables and fibre, enough protein, walking or other movement, regular meals, mindful eating, cooking at home. Pick something that fits the numbers, and vary it.
- Address the person as «ты». Use the grammatical gender given; when it is unknown, phrase sentences so no gendered forms are needed.
- At most one emoji. No hashtags, no headings, no quotation of these instructions.

Never shame, moralise or frame food as good or bad. Never suggest skipping meals, fasting, detoxes, or eating below the target. If the average intake is far below the target, gently say that eating enough matters too. No medical advice; for anything medical, suggest a doctor.`

func (c *Claude) Motivate(ctx context.Context, week Week) (string, float64, error) {
	response, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:        anthropic.Model(c.model),
		MaxTokens:    1000,
		System:       []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(describe(week)))},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow},
	})
	if err != nil {
		return "", 0, fmt.Errorf("coach: %w", err)
	}
	cost := c.price.Cost(estimate.Usage{
		Input:      response.Usage.InputTokens,
		CacheWrite: response.Usage.CacheCreationInputTokens,
		CacheRead:  response.Usage.CacheReadInputTokens,
		Output:     response.Usage.OutputTokens,
	})
	if response.StopReason != anthropic.StopReasonEndTurn {
		return "", cost, fmt.Errorf("coach: stopped with %s", response.StopReason)
	}
	for _, block := range response.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok && strings.TrimSpace(text.Text) != "" {
			return strings.TrimSpace(text.Text), cost, nil
		}
	}
	return "", cost, fmt.Errorf("coach: no text in the answer")
}

// describe is the week as the model reads it.
func describe(w Week) string {
	var b strings.Builder
	gender := "unknown"
	switch w.Sex {
	case model.SexFemale:
		gender = "female"
	case model.SexMale:
		gender = "male"
	}
	fmt.Fprintf(&b, "Name: %s\nGrammatical gender: %s\n", orDash(w.Name), gender)
	fmt.Fprintf(&b, "Days logged: %d of 7\n", w.Logged)
	if w.Logged > 0 {
		fmt.Fprintf(&b, "Average per logged day: %.0f kcal, %.0f g protein\n", w.Kcal, w.Protein)
	}
	if w.Target > 0 {
		fmt.Fprintf(&b, "Daily target: %.0f kcal; days within 10%% of it: %d\n", w.Target, w.OnTarget)
	} else {
		b.WriteString("No target set yet\n")
	}
	if w.Weight != nil {
		fmt.Fprintf(&b, "Weight: %.1f kg", *w.Weight)
		if w.Change != nil {
			fmt.Fprintf(&b, " (%+.1f kg over the week)", *w.Change)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// tips stand in for the model: one a week, in turn.
var tips = []string{
	"На этой неделе попробуй добавить овощи хотя бы в два приёма пищи в день — клетчатка надолго даёт сытость.",
	"Попробуй ложиться на полчаса раньше: при недосыпе сильнее тянет на сладкое.",
	"Держи бутылку воды на виду — часто жажду легко принять за голод.",
	"Добавь к каждому приёму пищи немного белка: яйца, творог, рыбу, бобовые — так сытость держится дольше.",
	"Маленькая цель на неделю — 20 минут прогулки в день. Можно частями.",
	"Попробуй есть без телефона хотя бы один раз в день: так проще заметить, когда уже сыт.",
	"Приготовь на выходных пару простых блюд впрок — в будни будет легче не перекусывать на бегу.",
}

// Fallback is the motivating part without a model.
func Fallback(week Week) string {
	opening := "Хорошая неделя — продолжай в том же духе 🐱"
	if week.Logged < 4 {
		opening = "Каждая запись — уже шаг. Попробуй на этой неделе записывать чуть чаще 🐱"
	}
	n := week.Number % len(tips)
	if n < 0 {
		n += len(tips)
	}
	return opening + "\n\n" + tips[n]
}
