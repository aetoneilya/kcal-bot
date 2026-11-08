package estimate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

type openAITransport func(*http.Request) (*http.Response, error)

func (f openAITransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const openAIDraft = `{"items":[{"name":"рис","grams":150,"confidence":"medium","kcal100":130,"protein100":2.7,"fat100":0.3,"carbs100":28}],"title":"Рис","question":"","options":[]}`

func openAIResponse(status, blockType, text string) string {
	b, _ := json.Marshal(map[string]any{
		"status": status,
		"output": []any{
			map[string]any{"type": "reasoning"},
			map[string]any{"type": "message", "content": []any{map[string]string{"type": blockType, "text": text}}},
		},
		"usage": map[string]any{
			"input_tokens": 3000, "output_tokens": 1000,
			"input_tokens_details":  map[string]int{"cached_tokens": 1000},
			"output_tokens_details": map[string]int{"reasoning_tokens": 600},
		},
	})
	return string(b)
}

func TestOpenAIRequestAndCachedCost(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "https://unrelated.invalid")
	o := NewOpenAI("test-key", "")
	o.client.Transport = openAITransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.openai.com/v1/responses" || r.Method != http.MethodPost {
			t.Errorf("unexpected destination: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing request headers")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var request struct {
			Model        string                  `json:"model"`
			Instructions string                  `json:"instructions"`
			Store        *bool                   `json:"store"`
			MaxTokens    int                     `json:"max_output_tokens"`
			Reasoning    struct{ Effort string } `json:"reasoning"`
			Input        []struct {
				Role    string
				Content []struct {
					Type     string
					Text     string
					ImageURL string `json:"image_url"`
					Detail   string
				}
			}
			Text struct {
				Format struct {
					Type   string
					Strict bool
					Schema json.RawMessage
				}
			}
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "gpt-5.4-mini" || request.Instructions != systemPrompt || request.Store == nil || *request.Store {
			t.Fatal("wrong model, instructions or storage setting")
		}
		if request.MaxTokens != 16000 || request.Reasoning.Effort != "medium" {
			t.Error("missing reasoning budget")
		}
		if request.Text.Format.Type != "json_schema" || !request.Text.Format.Strict {
			t.Error("structured outputs are not strict")
		}
		var wantSchema bytes.Buffer
		if err := json.Compact(&wantSchema, []byte(draftSchemaJSON)); err != nil {
			t.Fatal(err)
		}
		if string(request.Text.Format.Schema) != wantSchema.String() {
			t.Error("schema changed or lost property order")
		}
		if len(request.Input) != 1 || request.Input[0].Role != "user" || len(request.Input[0].Content) != 6 {
			t.Fatal("expected five photos and one text block")
		}
		content := request.Input[0].Content
		if content[0].ImageURL != "data:image/jpeg;base64,AQID" || content[1].ImageURL != "data:image/png;base64,BAU=" || content[0].Type != "input_image" || content[0].Detail != "high" {
			t.Error("photos were not encoded correctly")
		}
		for _, part := range []string{"рис", "На масле?", "Без масла", "×4"} {
			if !strings.Contains(content[5].Text, part) {
				t.Errorf("user context lost %q", part)
			}
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(openAIResponse("completed", "output_text", openAIDraft)))}, nil
	})
	result, err := o.Estimate(context.Background(), Input{
		Photos: []Photo{
			{Data: []byte{1, 2, 3}, MIME: "image/jpeg"}, {Data: []byte{4, 5}, MIME: "image/png"},
			{Data: []byte{6}, MIME: "image/webp"}, {Data: []byte{7}, MIME: "image/jpeg"}, {Data: []byte{8}, MIME: "image/png"},
		},
		Note: "рис", Question: "На масле?", Answer: "Без масла",
		History: []Portion{{Name: "рис", Grams: 150, Kcal100: 130, Times: 4}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Draft.Items) != 1 || result.Draft.Items[0].Name != "рис" || result.Raw != openAIDraft {
		t.Fatalf("draft not decoded: %+v", result.Draft)
	}
	// Cached input must not be billed twice. Output includes reasoning tokens.
	if want := (2000*0.75 + 1000*0.075 + 1000*4.50) / 1e6; math.Abs(result.Cost-want) > 1e-12 {
		t.Fatalf("cost = %v, want %v", result.Cost, want)
	}
	if result.Usage != (Usage{Input: 2000, CacheRead: 1000, Output: 1000, Reasoning: 600}) {
		t.Fatalf("usage = %+v", result.Usage)
	}
}

func TestOpenAIFailures(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		unusable   bool
		billed     bool
	}{
		{"refusal", openAIResponse("completed", "refusal", ""), 200, true, true},
		{"incomplete", openAIResponse("incomplete", "output_text", openAIDraft), 200, true, true},
		{"failed", openAIResponse("failed", "output_text", ""), 200, true, true},
		{"empty", openAIResponse("completed", "output_text", ""), 200, true, true},
		{"invalid draft", openAIResponse("completed", "output_text", "not json"), 200, true, true},
		{"invalid response", "not json", 200, false, false},
		{"unauthorized", "secret response body", 401, false, false},
		{"rate limit", "secret response body", 429, false, false},
		{"upstream error", "secret response body", 500, false, false},
		{"redirect", "secret response body", 307, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := NewOpenAI("test-key", "")
			o.client.Transport = openAITransport(func(r *http.Request) (*http.Response, error) {
				var req struct {
					Input []struct{ Content []struct{ Type string } }
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Input) != 1 || len(req.Input[0].Content) != 1 || req.Input[0].Content[0].Type != "input_text" {
					t.Error("text-only request is malformed")
				}
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})
			result, err := o.Estimate(context.Background(), Input{Note: "рис"})
			if err == nil || errors.Is(err, ErrUnusable) != tt.unusable || (result.Cost > 0) != tt.billed {
				t.Fatalf("result=%+v, err=%v", result, err)
			}
			if strings.Contains(err.Error(), "secret response body") {
				t.Error("error leaked the API response body")
			}
		})
	}
}

func TestOpenAITransportCancellation(t *testing.T) {
	o := NewOpenAI("test-key", "")
	o.client.Transport = openAITransport(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := o.Estimate(ctx, Input{Note: "рис"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("context error lost: %v", err)
	}
}

func TestOpenAIModels(t *testing.T) {
	for _, tt := range []struct {
		model  string
		priced bool
	}{
		{"", true}, {"gpt-5.4-mini", true}, {"gpt-5.4-mini-2026-03-17", true}, {"unknown-model", false},
	} {
		o := NewOpenAI("test-key", tt.model)
		if o.Priced() != tt.priced || !strings.HasPrefix(o.Name(), "openai:") {
			t.Fatalf("model=%q name=%q priced=%v", tt.model, o.Name(), o.Priced())
		}
	}
}

func TestOpenAIWrite(t *testing.T) {
	for _, tt := range []struct {
		name, block, text string
		unusable          bool
	}{
		{"plain text", "output_text", "  Продолжай записывать приёмы пищи.  ", false},
		{"refusal", "refusal", "", true},
		{"blank", "output_text", "  \n ", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := NewOpenAI("test-key", "")
			o.client.Transport = openAITransport(func(r *http.Request) (*http.Response, error) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if _, ok := body["text"]; ok {
					t.Error("weekly coach must not receive the meal JSON schema")
				}
				if body["instructions"] != "coach instructions" || body["store"] != false {
					t.Error("wrong coach instructions or storage setting")
				}
				if body["reasoning"].(map[string]any)["effort"] != "low" {
					t.Error("weekly coach should use low reasoning")
				}
				input := body["input"].([]any)[0].(map[string]any)["content"].([]any)
				if len(input) != 1 || input[0].(map[string]any)["text"] != "test week" {
					t.Error("weekly context was lost")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(openAIResponse("completed", tt.block, tt.text)))}, nil
			})
			result, err := o.Write(context.Background(), "coach instructions", "test week")
			if errors.Is(err, ErrUnusable) != tt.unusable || (!tt.unusable && err != nil) || result.Cost <= 0 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if !tt.unusable && result.Raw != strings.TrimSpace(tt.text) {
				t.Fatalf("text=%q", result.Raw)
			}
			if result.Usage.CacheRead != 1000 || result.Usage.Reasoning != 600 {
				t.Fatalf("coach billing details lost: %+v", result.Usage)
			}
		})
	}
}
