package estimate

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// Gemini calls the Gemini API with a response schema. In 2026 benchmarks the
// Flash models had the smallest portion error and the lowest price, so this
// is the default backend.
type Gemini struct {
	client *genai.Client
	model  string
}

// NewGemini builds the backend; model defaults to gemini-3.5-flash.
func NewGemini(ctx context.Context, apiKey, modelID string) (*Gemini, error) {
	if modelID == "" {
		modelID = "gemini-3.5-flash"
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, fmt.Errorf("gemini client: %w", err)
	}
	return &Gemini{client: client, model: modelID}, nil
}

func (g *Gemini) Name() string { return "gemini:" + g.model }

// Priced is false: no Gemini price table is kept, so spend limits do not
// apply to this backend; the per-day call limit still does.
func (g *Gemini) Priced() bool { return false }

func (g *Gemini) Estimate(ctx context.Context, in Input) (Result, error) {
	var parts []*genai.Part
	if len(in.Photo) > 0 {
		parts = append(parts, genai.NewPartFromBytes(in.Photo, in.PhotoMIME))
	}
	parts = append(parts, genai.NewPartFromText(userText(in)))

	response, err := g.client.Models.GenerateContent(ctx, g.model,
		[]*genai.Content{{Role: genai.RoleUser, Parts: parts}},
		&genai.GenerateContentConfig{
			SystemInstruction:  &genai.Content{Parts: []*genai.Part{genai.NewPartFromText(systemPrompt)}},
			ResponseMIMEType:   "application/json",
			ResponseJsonSchema: draftSchema,
		})
	if err != nil {
		return Result{}, fmt.Errorf("gemini: %w", err)
	}
	text := response.Text()
	if text == "" {
		return Result{}, fmt.Errorf("%w: empty answer", ErrUnusable)
	}
	draft, err := decode(text)
	return Result{Draft: draft}, err
}
