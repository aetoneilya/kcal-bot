package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestEstimatorSelection(t *testing.T) {
	for _, tt := range []struct {
		name, kind, token, apiKey, claudeKey, model, want, wantErr string
		dev                                                        bool
	}{
		{name: "OpenAI token", token: "test-token", want: "openai:gpt-5.4-mini"},
		{name: "OpenAI standard key", apiKey: "test-key", want: "openai:gpt-5.4-mini"},
		{name: "explicit OpenAI", kind: "openai", token: "test-token", claudeKey: "test-key", model: "custom-model", want: "openai:custom-model"},
		{name: "existing auto priority", token: "test-token", claudeKey: "test-key", want: "claude:claude-sonnet-5-5"},
		{name: "missing OpenAI key", kind: "openai", wantErr: "needs OPENAI_TOKEN"},
		{name: "missing production keys", wantErr: "no model API key"},
		{name: "unknown kind", kind: "bad", wantErr: "unknown ESTIMATOR"},
		{name: "explicit fake", kind: "fake", token: "test-token", want: "fake"},
		{name: "development fallback", dev: true, want: "fake"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range map[string]string{
				"ESTIMATOR": tt.kind, "OPENAI_TOKEN": tt.token, "OPENAI_API_KEY": tt.apiKey,
				"OPENAI_MODEL": tt.model, "GEMINI_API_KEY": "", "ANTHROPIC_API_KEY": tt.claudeKey,
				"CLAUDE_MODEL": "", "CLAUDE_EFFORT": "",
			} {
				t.Setenv(key, value)
			}
			est, err := newEstimator(context.Background(), tt.dev)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err=%v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if est.Name() != tt.want {
				t.Fatalf("name=%q, want %q", est.Name(), tt.want)
			}
		})
	}
}

func TestCoachSelection(t *testing.T) {
	for _, tt := range []struct {
		name, kind, token, apiKey, claudeKey, want string
	}{
		{"only OpenAI despite Claude key", "openai", "test-token", "", "test-claude", "*coach.OpenAI"},
		{"OpenAI standard key", "openai", "", "test-key", "test-claude", "*coach.OpenAI"},
		{"OpenAI missing key never calls Claude", "openai", "", "", "test-claude", "<nil>"},
		{"OpenAI automatic", "", "test-token", "", "", "*coach.OpenAI"},
		{"existing Claude", "", "test-token", "", "test-claude", "*coach.Claude"},
		{"no keys", "", "", "", "", "<nil>"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range map[string]string{
				"ESTIMATOR": tt.kind, "OPENAI_TOKEN": tt.token, "OPENAI_API_KEY": tt.apiKey,
				"ANTHROPIC_API_KEY": tt.claudeKey, "OPENAI_MODEL": "", "CLAUDE_MODEL": "",
			} {
				t.Setenv(key, value)
			}
			if got := fmt.Sprintf("%T", newCoach()); got != tt.want {
				t.Fatalf("coach=%s, want %s", got, tt.want)
			}
		})
	}
}

func TestOpenAIKeyPriority(t *testing.T) {
	t.Setenv("OPENAI_TOKEN", "personal-token")
	t.Setenv("OPENAI_API_KEY", "other-key")
	if openAIKey() != "personal-token" {
		t.Fatal("OPENAI_TOKEN did not take precedence")
	}
}
