// Package telegram is the thin Bot API client kcal-bot needs: send and edit a
// message, long-poll updates, answer a button press, set the menu button.
//
// The bot token lives in every request URL, so no error leaving this package
// may carry that URL — net/http's *url.Error would, and it is unwrapped.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	Token string
	// HTTP defaults to a client with a 45s timeout — long enough for a 30s
	// getUpdates long poll plus slack.
	HTTP *http.Client
	// BaseURL defaults to https://api.telegram.org; tests point it at a fake.
	BaseURL string
}

// APIError is a non-2xx answer from the Bot API.
type APIError struct {
	Status      int
	Description string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram: %d %s", e.Status, e.Description)
}

// Retryable is true for rate limits and server-side failures. A 400 (chat
// not found), 401 (bad token) or 403 (user blocked the bot) will not change
// on retry.
func (e *APIError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// ErrNoToken is returned when the client has no token configured.
var ErrNoToken = errors.New("telegram: bot token is not configured")

type WebAppInfo struct {
	URL string `json:"url"`
}

type InlineKeyboardButton struct {
	Text   string      `json:"text"`
	WebApp *WebAppInfo `json:"web_app,omitempty"`
	// CallbackData comes back in a CallbackQuery when the button is pressed.
	CallbackData string `json:"callback_data,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// OpenAppKeyboard is a one-button keyboard that launches the Mini App.
func OpenAppKeyboard(text, appURL string) *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: text, WebApp: &WebAppInfo{URL: appURL}}}}}
}

// ActionKeyboard is a one-button keyboard that reports a press to the bot.
func ActionKeyboard(text, data string) *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: text, CallbackData: data}}}}
}

type SendMessageRequest struct {
	ChatID      int64                 `json:"chat_id"`
	Text        string                `json:"text"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

type EditMessageTextRequest struct {
	ChatID      int64                 `json:"chat_id"`
	MessageID   int64                 `json:"message_id"`
	Text        string                `json:"text"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	Chat      Chat   `json:"chat"`
	From      *User  `json:"from"`
}

// CallbackQuery is a press on a button with CallbackData.
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

func (c *Client) SendMessage(ctx context.Context, request SendMessageRequest) error {
	return c.call(ctx, "sendMessage", request, nil)
}

func (c *Client) EditMessageText(ctx context.Context, request EditMessageTextRequest) error {
	return c.call(ctx, "editMessageText", request, nil)
}

// AnswerCallbackQuery stops the button's spinner and shows text as a toast.
func (c *Client) AnswerCallbackQuery(ctx context.Context, id, text string) error {
	return c.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id, "text": text}, nil)
}

// GetUpdates long-polls for up to timeoutSec seconds. offset is the id of the
// first update to return; pass the last seen id + 1 to acknowledge the rest.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]Update, error) {
	var updates []Update
	err := c.call(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         timeoutSec,
		"allowed_updates": []string{"message", "callback_query"},
	}, &updates)
	return updates, err
}

// SetChatMenuButton makes the bot's menu button open the Mini App for every
// chat that has no button of its own.
func (c *Client) SetChatMenuButton(ctx context.Context, text, appURL string) error {
	return c.call(ctx, "setChatMenuButton", map[string]any{
		"menu_button": map[string]any{
			"type":    "web_app",
			"text":    text,
			"web_app": WebAppInfo{URL: appURL},
		},
	}, nil)
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 45 * time.Second}
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return "https://api.telegram.org"
}

type envelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

func (c *Client) call(ctx context.Context, method string, payload, out any) error {
	if c.Token == "" {
		return ErrNoToken
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: encode %s: %w", method, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL()+"/bot"+c.Token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: build %s: %w", method, redact(err))
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("telegram: %s: %w", method, redact(err))
	}
	defer response.Body.Close()

	var decoded envelope
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	_ = json.Unmarshal(raw, &decoded)
	if response.StatusCode != http.StatusOK || !decoded.OK {
		description := decoded.Description
		if description == "" {
			description = http.StatusText(response.StatusCode)
		}
		status := response.StatusCode
		if status == http.StatusOK && decoded.ErrorCode != 0 {
			status = decoded.ErrorCode
		}
		return &APIError{Status: status, Description: description}
	}
	if out != nil && len(decoded.Result) > 0 {
		if err := json.Unmarshal(decoded.Result, out); err != nil {
			return fmt.Errorf("telegram: decode %s result: %w", method, err)
		}
	}
	return nil
}

// redact strips the request URL, which carries the bot token, from transport
// errors before they reach a log line.
func redact(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("%s: %w", urlErr.Op, urlErr.Err)
	}
	return err
}
