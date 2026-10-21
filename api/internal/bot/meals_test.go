package bot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/estimate"
	"github.com/aetoneilya/kcal-bot/api/internal/model"
	"github.com/aetoneilya/kcal-bot/api/internal/store"
	"github.com/aetoneilya/kcal-bot/api/internal/telegram"
)

// photoAPI is a fake Bot API that serves files and records what the bot
// says, keyed by method.
type photoAPI struct {
	mu    sync.Mutex
	calls []struct {
		method string
		body   map[string]any
	}
}

func (a *photoAPI) last(method string) map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := len(a.calls) - 1; i >= 0; i-- {
		if a.calls[i].method == method {
			return a.calls[i].body
		}
	}
	return nil
}

// files lists the file ids the bot fetched, in order.
func (a *photoAPI) files() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var ids []string
	for _, c := range a.calls {
		if c.method == "getFile" {
			ids = append(ids, c.body["file_id"].(string))
		}
	}
	return ids
}

func (a *photoAPI) count(method string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, c := range a.calls {
		if c.method == method {
			n++
		}
	}
	return n
}

// jpeg is enough of a JPEG for http.DetectContentType.
var jpeg = []byte("\xff\xd8\xff\xe0 a photo")

func newPhotoAPI(t *testing.T) (*telegram.Client, *photoAPI) {
	t.Helper()
	api := &photoAPI{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/file/bott/") {
			w.Write(jpeg)
			return
		}
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		api.mu.Lock()
		api.calls = append(api.calls, struct {
			method string
			body   map[string]any
		}{method, body})
		api.mu.Unlock()
		switch method {
		case "getFile":
			w.Write([]byte(`{"ok":true,"result":{"file_path":"photos/1.jpg"}}`))
		case "sendMessage":
			w.Write([]byte(`{"ok":true,"result":{"message_id":500}}`))
		default:
			w.Write([]byte(`{"ok":true,"result":true}`))
		}
	}))
	t.Cleanup(srv.Close)
	return &telegram.Client{Token: "t", BaseURL: srv.URL}, api
}

// scriptedAnalyzer answers with a question first and a final draft once the
// question is answered.
type scriptedAnalyzer struct {
	mu     sync.Mutex
	inputs []estimate.Input
}

func (s *scriptedAnalyzer) Analyze(_ context.Context, _ model.User, in estimate.Input) (model.Draft, error) {
	s.mu.Lock()
	s.inputs = append(s.inputs, in)
	s.mu.Unlock()
	rice := model.Item{Name: "рис", Grams: 200, Kcal100: 130, Protein100: 2.7, Fat100: 0.3, Carbs100: 28}
	if in.Answer == "" {
		return model.Draft{Title: "Рис с маслом", Items: []model.Item{rice}, Question: "Сколько масла?", Options: []string{"Без масла", "Ложка"}}, nil
	}
	butter := model.Item{Name: "масло", Grams: 10, Kcal100: 750, Fat100: 82}
	return model.Draft{Title: "Рис с маслом", Items: []model.Item{rice, butter}}, nil
}

func photoMessage(from, chat int64, group, caption string) telegram.Update {
	return telegram.Update{Message: &telegram.Message{
		MessageID: 1, Date: time.Date(2026, 10, 21, 13, 0, 0, 0, time.UTC).Unix(),
		Chat: telegram.Chat{ID: chat, Type: "private"}, From: &telegram.User{ID: from},
		Photo: []telegram.PhotoSize{
			{FileID: "small", Width: 90, Height: 90},
			{FileID: "right", Width: 1280, Height: 960},
			{FileID: "huge", Width: 2560, Height: 1920},
		},
		Caption: caption, MediaGroupID: group,
	}}
}

func press(from int64, data string) telegram.Update {
	return telegram.Update{CallbackQuery: &telegram.CallbackQuery{ID: "q", From: telegram.User{ID: from}, Data: data,
		Message: &telegram.Message{MessageID: 500, Chat: telegram.Chat{ID: from}}}}
}

// buttons returns the callback data of the keyboard in a request body.
func buttons(body map[string]any) []string {
	var data []string
	markup, _ := body["reply_markup"].(map[string]any)
	rows, _ := markup["inline_keyboard"].([]any)
	for _, row := range rows {
		for _, button := range row.([]any) {
			if d, ok := button.(map[string]any)["callback_data"].(string); ok {
				data = append(data, d)
			}
		}
	}
	return data
}

func TestPhotoInChatBecomesAMealAfterAnAnswer(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "kcal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	client, api := newPhotoAPI(t)
	analyzer := &scriptedAnalyzer{}
	b := &Bot{Client: client, AppURL: "https://kcal.example", Users: st, Analyzer: analyzer, Meals: st}
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)

	b.Handle(ctx, photoMessage(7, 7, "", "  на ужин "))
	b.Wait()
	if got := api.files(); len(got) != 2 || got[0] != "right" || got[1] != "small" {
		t.Fatalf("downloaded %v, want the photo within %dpx and then its preview", got, maxPhotoSide)
	}
	if got := api.last("getFile")["file_id"]; got != "small" {
		t.Fatalf("downloaded %v, want the largest size within %dpx", got, maxPhotoSide)
	}
	if len(analyzer.inputs) != 1 || len(analyzer.inputs[0].Photos) != 1 || analyzer.inputs[0].Note != "на ужин" ||
		analyzer.inputs[0].Photos[0].MIME != "image/jpeg" {
		t.Fatalf("analyzer got %+v", analyzer.inputs)
	}
	shown := api.last("editMessageText")
	if text := shown["text"].(string); !strings.Contains(text, "рис — 200 г · 260 ккал") || !strings.Contains(text, "❓ Сколько масла?") {
		t.Fatalf("breakdown: %q", text)
	}
	keys := buttons(shown)
	if len(keys) != 4 || !strings.HasPrefix(keys[1], mealData+"ans:") || !strings.HasPrefix(keys[2], mealData+"save:") {
		t.Fatalf("keyboard: %v", keys)
	}

	// Someone else cannot press it.
	b.Handle(ctx, press(8, keys[2]))
	if meals, _ := st.Meals(ctx, 7, day, day.Add(48*time.Hour)); len(meals) != 0 {
		t.Fatal("a stranger saved the meal")
	}

	b.Handle(ctx, press(7, keys[1])) // "Ложка"
	b.Wait()
	if len(analyzer.inputs) != 2 || analyzer.inputs[1].Question != "Сколько масла?" || analyzer.inputs[1].Answer != "Ложка" ||
		len(analyzer.inputs[1].Photos) != 1 {
		t.Fatalf("re-estimate got %+v", analyzer.inputs[1:])
	}
	shown = api.last("editMessageText")
	if text := shown["text"].(string); !strings.Contains(text, "масло — 10 г · 75 ккал") || strings.Contains(text, "❓") {
		t.Fatalf("after the answer: %q", text)
	}
	keys = buttons(shown)
	if len(keys) != 2 {
		t.Fatalf("after the answer only Save and Cancel, got %v", keys)
	}

	b.Handle(ctx, press(7, keys[0]))
	meals, err := st.Meals(ctx, 7, day, day.Add(48*time.Hour))
	if err != nil || len(meals) != 1 || len(meals[0].Items) != 2 || meals[0].Note != "на ужин" || !meals[0].HasPhoto ||
		!meals[0].EatenAt.Equal(time.Date(2026, 10, 21, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("saved %+v %v", meals, err)
	}
	if text := api.last("editMessageText")["text"].(string); !strings.Contains(text, "Записано") {
		t.Fatalf("after save: %q", text)
	}

	// The draft is gone: a second press saves nothing.
	b.Handle(ctx, press(7, keys[0]))
	if meals, _ := st.Meals(ctx, 7, day, day.Add(48*time.Hour)); len(meals) != 1 {
		t.Fatalf("saved twice: %d", len(meals))
	}
	if got := api.last("answerCallbackQuery")["text"]; !strings.Contains(got.(string), "устарело") {
		t.Fatalf("stale press answer: %v", got)
	}
}

func TestAlbumIsOneMeal(t *testing.T) {
	client, api := newPhotoAPI(t)
	analyzer := &scriptedAnalyzer{}
	b := &Bot{Client: client, AppURL: "https://kcal.example", Analyzer: analyzer, Meals: noMeals{}, AlbumDelay: 50 * time.Millisecond}
	for _, caption := range []string{"", "обед", "", ""} {
		b.Handle(context.Background(), photoMessage(7, 7, "album-1", caption))
	}
	b.Wait()
	if len(analyzer.inputs) != 1 || len(analyzer.inputs[0].Photos) != estimate.MaxPhotos || analyzer.inputs[0].Note != "обед" {
		t.Fatalf("want one estimate of %d photos with the caption, got %d inputs", estimate.MaxPhotos, len(analyzer.inputs))
	}
	if api.count("sendMessage") != 1 {
		t.Fatalf("want one answer, got %d", api.count("sendMessage"))
	}
}

func TestPhotoWithoutAnalyzerGetsTheAppButton(t *testing.T) {
	client, api := newPhotoAPI(t)
	b := &Bot{Client: client, AppURL: "https://kcal.example"}
	b.Handle(context.Background(), photoMessage(7, 7, "", ""))
	if api.count("getFile") != 0 || api.last("sendMessage")["text"] != otherText {
		t.Fatalf("got %+v", api.calls)
	}
}

type noMeals struct{}

func (noMeals) CreateMeal(context.Context, int64, store.MealInput) (model.Meal, error) {
	return model.Meal{}, nil
}

func TestTextInChatIsAMealDescription(t *testing.T) {
	client, api := newPhotoAPI(t)
	analyzer := &scriptedAnalyzer{}
	b := &Bot{Client: client, AppURL: "https://kcal.example", Analyzer: analyzer, Meals: noMeals{}}
	update := start(7)
	update.Message.Text = " рис 200 г с маслом "
	b.Handle(context.Background(), update)
	b.Wait()
	if api.count("getFile") != 0 || len(analyzer.inputs) != 1 || analyzer.inputs[0].Note != "рис 200 г с маслом" || len(analyzer.inputs[0].Photos) != 0 {
		t.Fatalf("got %+v", analyzer.inputs)
	}
	if text := api.last("editMessageText")["text"].(string); !strings.Contains(text, "рис — 200 г") {
		t.Fatalf("breakdown: %q", text)
	}

	update.Message.Text = "/help"
	b.Handle(context.Background(), update)
	b.Wait()
	if len(analyzer.inputs) != 1 || api.last("sendMessage")["text"] != otherText {
		t.Fatal("a command must not be estimated")
	}
}
