package estimate

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Claude calls the Anthropic Messages API with structured outputs, so the
// answer is guaranteed to parse against draftSchemaJSON.
type Claude struct {
	client anthropic.Client
	model  string
	effort anthropic.OutputConfigEffort
	price  Price
	priced bool
}

// NewClaude builds the backend. model defaults to claude-sonnet-5-5; effort
// ("low", "medium", "high") trades reasoning tokens — the bulk of the cost —
// for care in judging portions.
func NewClaude(apiKey, modelID, effort string) *Claude {
	if modelID == "" {
		modelID = "claude-sonnet-5-5"
	}
	if effort == "" {
		effort = "medium"
	}
	// The SDK also reads ANTHROPIC_BASE_URL and ANTHROPIC_CUSTOM_HEADERS from
	// the environment. A developer machine may set those for some other
	// gateway (a corporate proxy, say), and this key must never go there, so
	// both are overridden rather than inherited.
	opts := []option.RequestOption{
		option.WithBaseURL("https://api.anthropic.com/"),
		option.WithAPIKey(apiKey),
	}
	for _, line := range strings.Split(os.Getenv("ANTHROPIC_CUSTOM_HEADERS"), "\n") {
		if name, _, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(name) != "" {
			opts = append(opts, option.WithHeaderDel(strings.TrimSpace(name)))
		}
	}
	price, priced := claudePrices[modelID]
	return &Claude{
		client: anthropic.NewClient(opts...),
		model:  modelID,
		effort: anthropic.OutputConfigEffort(effort),
		price:  price,
		priced: priced,
	}
}

func (c *Claude) Name() string { return "claude:" + c.model }

func (c *Claude) Priced() bool { return c.priced }

// Estimate returns the cost alongside an error too: a refusal or a cut-off
// answer is billed like any other.
func (c *Claude) Estimate(ctx context.Context, in Input) (Result, error) {
	var content []anthropic.ContentBlockParamUnion
	for _, photo := range in.Photos {
		content = append(content, anthropic.NewImageBlockBase64(photo.MIME, base64.StdEncoding.EncodeToString(photo.Data)))
	}
	content = append(content, anthropic.NewTextBlock(userText(in)))

	response, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 16000,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(content...)},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: c.effort,
			// The SDK wants a map here, which would lose the property order;
			// the ordered schema replaces it in the request body below.
			Format: anthropic.JSONOutputFormatParam{Schema: map[string]any{"type": "object"}},
		},
		// Think before answering: judging portions is the hard part, and
		// without thinking the model fills the schema in one pass.
		Thinking: anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
	}, option.WithJSONSet("output_config.format.schema", draftSchema))
	if err != nil {
		return Result{}, fmt.Errorf("claude: %w", err)
	}
	usage := Usage{
		Input:      response.Usage.InputTokens,
		CacheWrite: response.Usage.CacheCreationInputTokens,
		CacheRead:  response.Usage.CacheReadInputTokens,
		Output:     response.Usage.OutputTokens,
	}
	result := Result{Cost: c.price.Cost(usage), Usage: usage}
	switch response.StopReason {
	case anthropic.StopReasonRefusal:
		return result, fmt.Errorf("%w: refused (%s)", ErrUnusable, response.StopDetails.Category)
	case anthropic.StopReasonMaxTokens:
		return result, fmt.Errorf("%w: answer was cut off", ErrUnusable)
	}
	for _, block := range response.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			result.Raw = text.Text
			result.Draft, err = decode(text.Text)
			return result, err
		}
	}
	return result, fmt.Errorf("%w: no text in the answer", ErrUnusable)
}
