package estimate

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// The request must carry the schema in its declared order and ask for
// thinking; without either the model answered a food photo with empty zeros.
func TestClaudeRequestKeepsSchemaOrderAndThinks(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","model":"claude-sonnet-5-5","stop_reason":"end_turn",
			"content":[{"type":"text","text":"{\"items\":[{\"name\":\"рис\",\"grams\":150,\"confidence\":\"medium\",\"kcal100\":130,\"protein100\":2.7,\"fat100\":0.3,\"carbs100\":28}],\"title\":\"Рис\",\"question\":\"\",\"options\":[]}"}],
			"usage":{"input_tokens":3000,"output_tokens":1000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClaude("test-key", "", "")
	c.client = anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))

	result, err := c.Estimate(context.Background(), Input{Note: "рис"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Draft.Items) != 1 || result.Draft.Items[0].Name != "рис" {
		t.Fatalf("draft not decoded: %+v", result.Draft)
	}
	if want := (3000*2.0 + 1000*10.0) / 1e6; result.Cost != want {
		t.Fatalf("cost = %v, want %v", result.Cost, want)
	}
	if !strings.Contains(body, `"thinking":{"type":"adaptive"`) {
		t.Fatalf("request does not ask for adaptive thinking:\n%s", body)
	}
	schema := body[strings.Index(body, `"schema"`):]
	if strings.Index(schema, `"name"`) > strings.Index(schema, `"carbs100"`) || !strings.Contains(schema, `"enum"`) {
		t.Fatalf("schema in the request lost its order or content:\n%s", schema)
	}
}
