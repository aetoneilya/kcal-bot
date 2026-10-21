package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientCallsAndClassifies(t *testing.T) {
	var paths []string
	var bodies []string
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		paths = append(paths, r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		switch {
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":7,"message":{"text":"/start","chat":{"id":42,"type":"private"},"from":{"id":42,"username":"ilya","first_name":"Ilya"}}}]}`))
		case calls == 2:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`))
		case calls == 3:
			w.WriteHeader(http.StatusBadGateway)
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		}
	}))
	t.Cleanup(server.Close)

	client := &Client{Token: "123:secret", BaseURL: server.URL}
	ctx := context.Background()

	updates, err := client.GetUpdates(ctx, 5, 30)
	if err != nil || len(updates) != 1 || updates[0].UpdateID != 7 || updates[0].Message.From.Username != "ilya" {
		t.Fatalf("getUpdates = %#v, err = %v", updates, err)
	}
	if paths[0] != "/bot123:secret/getUpdates" || !strings.Contains(bodies[0], `"offset":5`) {
		t.Fatalf("getUpdates request = %s %s", paths[0], bodies[0])
	}

	err = client.SendMessage(ctx, SendMessageRequest{ChatID: 42, Text: "hi", ReplyMarkup: OpenAppKeyboard("Open", "https://kcal.example")})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Retryable() || apiErr.Status != 403 {
		t.Fatalf("403 should be a permanent APIError, got %v", err)
	}
	var sent SendMessageRequest
	if err := json.Unmarshal([]byte(bodies[1]), &sent); err != nil || sent.ReplyMarkup.InlineKeyboard[0][0].WebApp.URL != "https://kcal.example" {
		t.Fatalf("sendMessage body = %s", bodies[1])
	}

	err = client.SendMessage(ctx, SendMessageRequest{ChatID: 42, Text: "hi"})
	if !errors.As(err, &apiErr) || !apiErr.Retryable() {
		t.Fatalf("502 should be retryable, got %v", err)
	}

	if err := client.SetChatMenuButton(ctx, "Открыть", "https://kcal.example"); err != nil {
		t.Fatalf("setChatMenuButton err = %v", err)
	}
	if !strings.Contains(bodies[3], `"type":"web_app"`) || !strings.Contains(bodies[3], `"url":"https://kcal.example"`) {
		t.Fatalf("setChatMenuButton body = %s", bodies[3])
	}

	// Transport errors must not leak the token through the URL.
	server.Close()
	err = client.SendMessage(ctx, SendMessageRequest{ChatID: 42, Text: "hi"})
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("transport error leaks token: %v", err)
	}

	if err := (&Client{}).SendMessage(ctx, SendMessageRequest{}); !errors.Is(err, ErrNoToken) {
		t.Fatalf("missing token err = %v", err)
	}
}
