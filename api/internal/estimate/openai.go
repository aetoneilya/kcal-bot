package estimate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAI uses the Responses API with the same ordered schema as the other
// backends. Photos stay inline, and responses are not stored by the API.
type OpenAI struct {
	client *http.Client
	apiKey string
	model  string
	price  Price
	priced bool
}

// NewOpenAI builds the backend; model defaults to gpt-5.4-mini.
func NewOpenAI(apiKey, modelID string) *OpenAI {
	if modelID == "" {
		modelID = "gpt-5.4-mini"
	}
	price, priced := OpenAIPrice(modelID)
	return &OpenAI{
		client: &http.Client{
			Timeout: 90 * time.Second,
			// A redirect must never carry a meal or the key to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		apiKey: apiKey,
		model:  modelID,
		price:  price,
		priced: priced,
	}
}

func (o *OpenAI) Name() string { return "openai:" + o.model }

func (o *OpenAI) Priced() bool { return o.priced }

func (o *OpenAI) Estimate(ctx context.Context, in Input) (Result, error) {
	content := make([]map[string]string, 0, len(in.Photos)+1)
	for _, photo := range in.Photos {
		content = append(content, map[string]string{
			"type": "input_image", "detail": "high",
			"image_url": "data:" + photo.MIME + ";base64," + base64.StdEncoding.EncodeToString(photo.Data),
		})
	}
	content = append(content, map[string]string{"type": "input_text", "text": userText(in)})
	result, err := o.respond(ctx, systemPrompt, content, 16000, "medium", true)
	if err != nil {
		return result, err
	}
	result.Draft, err = decode(result.Raw)
	return result, err
}

// Write uses the same model and billing for the weekly coach's plain text.
func (o *OpenAI) Write(ctx context.Context, instructions, text string) (Result, error) {
	result, err := o.respond(ctx, instructions, []map[string]string{{"type": "input_text", "text": text}}, 4000, "low", false)
	result.Raw = strings.TrimSpace(result.Raw)
	return result, err
}

func (o *OpenAI) respond(ctx context.Context, instructions string, content []map[string]string, maxTokens int, effort string, structured bool) (Result, error) {
	request := map[string]any{
		"model": o.model, "instructions": instructions,
		"input":             []any{map[string]any{"role": "user", "content": content}},
		"max_output_tokens": maxTokens,
		"reasoning":         map[string]string{"effort": effort},
		"store":             false,
		"service_tier":      "default",
	}
	if structured {
		request["text"] = map[string]any{"format": map[string]any{
			"type": "json_schema", "name": "meal", "strict": true, "schema": json.RawMessage(draftSchemaJSON),
		}}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return Result{}, fmt.Errorf("openai request: %w", err)
	}
	// Deliberately ignore OPENAI_BASE_URL and other SDK environment defaults:
	// this personal key must only be sent to the official API.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("openai request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("openai: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// API error bodies can echo input; neither meals nor credentials
		// belong in a transport error that the caller will log.
		return Result{}, fmt.Errorf("openai: HTTP %d", resp.StatusCode)
	}
	var response struct {
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
			InputDetails struct {
				CachedTokens int64 `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			OutputDetails struct {
				ReasoningTokens int64 `json:"reasoning_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&response); err != nil {
		return Result{}, fmt.Errorf("openai response: %w", err)
	}
	// Responses includes cached tokens in input_tokens, unlike Anthropic.
	usage := Usage{
		Input:     response.Usage.InputTokens - response.Usage.InputDetails.CachedTokens,
		CacheRead: response.Usage.InputDetails.CachedTokens,
		Output:    response.Usage.OutputTokens,
		Reasoning: response.Usage.OutputDetails.ReasoningTokens,
	}
	result := Result{Usage: usage, Cost: o.price.Cost(usage)}
	if response.Status != "completed" {
		return result, fmt.Errorf("%w: openai response status %q", ErrUnusable, response.Status)
	}
	var text strings.Builder
	for _, output := range response.Output {
		if output.Type != "message" {
			continue
		}
		for _, block := range output.Content {
			switch block.Type {
			case "refusal":
				return result, fmt.Errorf("%w: openai refused", ErrUnusable)
			case "output_text":
				text.WriteString(block.Text)
			}
		}
	}
	result.Raw = text.String()
	if strings.TrimSpace(result.Raw) == "" {
		return result, fmt.Errorf("%w: no text in openai answer", ErrUnusable)
	}
	return result, nil
}
