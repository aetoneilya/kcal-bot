package bot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/analysis"
	"github.com/aetoneilya/kcal-bot/api/internal/estimate"
	"github.com/aetoneilya/kcal-bot/api/internal/model"
	"github.com/aetoneilya/kcal-bot/api/internal/store"
	"github.com/aetoneilya/kcal-bot/api/internal/telegram"
)

// A photo or a plain-text description sent to the bot is estimated like an
// upload in the app: the bot
// answers with the breakdown, a button per ready answer to the model's
// question, and Save / Cancel. The draft waits in memory until a button is
// pressed; a restart forgets it, and the photo is simply sent again.

// Analyzer estimates one meal within the person's limits.
type Analyzer interface {
	Analyze(ctx context.Context, user model.User, in estimate.Input) (model.Draft, error)
}

// Meals saves a confirmed draft.
type Meals interface {
	CreateMeal(ctx context.Context, userID int64, in store.MealInput) (model.Meal, error)
}

const (
	mealData = "meal:" // meal:<save|drop|ans>:<draft id>[:<option>]

	// Photos of one album arrive as separate messages; wait this long for the
	// rest before estimating.
	defaultAlbumDelay = 1500 * time.Millisecond
	// A draft nobody pressed a button on is dropped after this long.
	draftTTL = 24 * time.Hour
	// Telegram resizes photos into several sizes; the model gains nothing
	// past this side, and smaller uploads are faster and cheaper.
	maxPhotoSide = 1600
)

type pendingMeal struct {
	user    model.User
	chatID  int64
	message int64 // the bot's answer, edited as things change
	photos  []estimate.Photo
	note    string
	eatenAt time.Time
	draft   model.Draft
	created time.Time
	busy    bool // a re-estimate is running
}

// album collects the photos of one media group until its timer fires.
type album struct {
	user    model.User
	chatID  int64
	sizes   []telegram.PhotoSize
	caption string
	date    int64
}

type drafts struct {
	mu      sync.Mutex
	pending map[string]*pendingMeal
	albums  map[string]*album
	running sync.WaitGroup
}

func (b *Bot) albumDelay() time.Duration {
	if b.AlbumDelay > 0 {
		return b.AlbumDelay
	}
	return defaultAlbumDelay
}

// Wait blocks until the estimates started so far have answered. For tests
// and shutdown.
func (b *Bot) Wait() { b.drafts.running.Wait() }

// receivePhoto starts estimating a photo, or adds it to an album that is
// still arriving.
func (b *Bot) receivePhoto(ctx context.Context, message *telegram.Message, user model.User) {
	size := pickSize(message.Photo)
	d := &b.drafts
	if message.MediaGroupID == "" {
		d.running.Add(1)
		go func() {
			defer d.running.Done()
			b.estimate(ctx, user, message.Chat.ID, []telegram.PhotoSize{size}, message.Caption, message.Date)
		}()
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.albums == nil {
		d.albums = map[string]*album{}
	}
	if a, ok := d.albums[message.MediaGroupID]; ok {
		if len(a.sizes) < estimate.MaxPhotos {
			a.sizes = append(a.sizes, size)
		}
		if a.caption == "" {
			a.caption = message.Caption
		}
		return
	}
	d.albums[message.MediaGroupID] = &album{user: user, chatID: message.Chat.ID, sizes: []telegram.PhotoSize{size},
		caption: message.Caption, date: message.Date}
	d.running.Add(1)
	time.AfterFunc(b.albumDelay(), func() {
		defer d.running.Done()
		d.mu.Lock()
		a := d.albums[message.MediaGroupID]
		delete(d.albums, message.MediaGroupID)
		d.mu.Unlock()
		b.estimate(ctx, a.user, a.chatID, a.sizes, a.caption, a.date)
	})
}

// isDescription is any text that is not a command: "овсянка 200 г с бананом".
func isDescription(text string) bool {
	text = strings.TrimSpace(text)
	return text != "" && !strings.HasPrefix(text, "/")
}

// receiveText estimates a meal described in words.
func (b *Bot) receiveText(ctx context.Context, message *telegram.Message, user model.User) {
	d := &b.drafts
	d.running.Add(1)
	go func() {
		defer d.running.Done()
		b.estimate(ctx, user, message.Chat.ID, nil, message.Text, message.Date)
	}()
}

// pickSize takes the largest version that is not needlessly big.
func pickSize(sizes []telegram.PhotoSize) telegram.PhotoSize {
	best := sizes[0]
	for _, s := range sizes[1:] {
		if max(s.Width, s.Height) <= maxPhotoSide {
			best = s
		}
	}
	return best
}

func (b *Bot) estimate(ctx context.Context, user model.User, chatID int64, sizes []telegram.PhotoSize, caption string, date int64) {
	_ = b.Client.SendChatAction(ctx, chatID, "typing")
	messageID, err := b.Client.SendMessageID(ctx, telegram.SendMessageRequest{ChatID: chatID, Text: "Считаю калории… 🐱"})
	if err != nil {
		b.log().Warn("could not answer a photo", "chat", chatID, "err", err)
		return
	}
	edit := func(text string, keyboard *telegram.InlineKeyboardMarkup) {
		b.edit(ctx, chatID, messageID, text, keyboard)
	}

	note := strings.TrimSpace(caption)
	if err := model.ValidateText("note", note, model.MaxNoteRunes); err != nil {
		note = string([]rune(note)[:model.MaxNoteRunes])
	}
	in := estimate.Input{Note: note}
	for _, size := range sizes {
		data, err := b.Client.DownloadFile(ctx, size.FileID)
		if err != nil {
			b.log().Warn("could not download a photo", "chat", chatID, "err", err)
			edit("Не получилось скачать фото — пришли его ещё раз.", nil)
			return
		}
		in.Photos = append(in.Photos, estimate.Photo{Data: data, MIME: http.DetectContentType(data)})
	}

	draft, err := b.Analyzer.Analyze(ctx, user, in)
	if err != nil {
		edit(analysisFailure(err), b.openKeyboard())
		return
	}
	if len(draft.Items) == 0 {
		if len(sizes) == 0 {
			edit("Не понял, что за еда 🤔 Напиши, например: «гречка 150 г, котлета» — или пришли фото.", b.openKeyboard())
		} else {
			edit("Не нашёл на фото еды 🤔 Попробуй другое фото или подпиши, что на нём.", b.openKeyboard())
		}
		return
	}

	eatenAt := time.Unix(date, 0)
	if date == 0 {
		eatenAt = time.Now()
	}
	meal := &pendingMeal{user: user, chatID: chatID, message: messageID, photos: in.Photos, note: note,
		eatenAt: eatenAt, draft: draft, created: time.Now()}
	id := b.keep(meal)
	edit(describeDraft(draft, true), draftKeyboard(id, draft))
}

func analysisFailure(err error) string {
	switch {
	case errors.Is(err, store.ErrLimit), errors.Is(err, analysis.ErrSpendLimit):
		return "На сегодня лимит распознаваний исчерпан. Добавить вручную можно в дневнике."
	case errors.Is(err, analysis.ErrPaused):
		return "Распознавание на сегодня приостановлено. Добавить вручную можно в дневнике."
	case errors.Is(err, estimate.ErrUnusable):
		return "Не получилось разобрать блюдо — попробуй другое фото или подпиши, что на нём."
	default:
		return "Модель не ответила — пришли фото ещё раз."
	}
}

// keep stores a draft under a fresh id and drops stale ones.
func (b *Bot) keep(meal *pendingMeal) string {
	raw := make([]byte, 6)
	_, _ = rand.Read(raw)
	id := hex.EncodeToString(raw)
	d := &b.drafts
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pending == nil {
		d.pending = map[string]*pendingMeal{}
	}
	for key, old := range d.pending {
		if time.Since(old.created) > draftTTL {
			delete(d.pending, key)
		}
	}
	d.pending[id] = meal
	return id
}

// handleMealPress serves Save, Cancel and the answers to the model's question.
func (b *Bot) handleMealPress(ctx context.Context, press *telegram.CallbackQuery) {
	answer := func(text string) {
		if err := b.Client.AnswerCallbackQuery(ctx, press.ID, text); err != nil {
			b.log().Warn("could not answer a button press", "err", err)
		}
	}
	parts := strings.Split(strings.TrimPrefix(press.Data, mealData), ":")
	if len(parts) < 2 {
		answer("")
		return
	}
	action, id := parts[0], parts[1]

	d := &b.drafts
	d.mu.Lock()
	meal := d.pending[id]
	if meal == nil || meal.user.ID != press.From.ID {
		d.mu.Unlock()
		answer("Это фото уже устарело — пришли его ещё раз")
		return
	}
	if meal.busy {
		d.mu.Unlock()
		answer("Ещё считаю…")
		return
	}
	switch action {
	case "save", "drop":
		delete(d.pending, id)
	case "ans":
		meal.busy = true
	}
	d.mu.Unlock()

	switch action {
	case "save":
		_, err := b.Meals.CreateMeal(ctx, meal.user.ID, store.MealInput{
			EatenAt: meal.eatenAt, Title: meal.draft.Title, Note: meal.note, Items: meal.draft.Items,
		})
		if err != nil {
			b.log().Warn("could not save a meal from the bot", "user", meal.user.ID, "err", err)
			d.mu.Lock()
			d.pending[id] = meal
			d.mu.Unlock()
			answer("Не получилось записать — попробуй ещё раз")
			return
		}
		answer("Записано")
		text := describeDraft(meal.draft, false) + "\n\n✓ Записано в дневник. Поправить можно там."
		if b.Treats.For(meal.user) {
			if compliment := b.Treats.Compliment(); compliment != "" {
				text += "\n\n" + compliment
			}
		}
		b.edit(ctx, meal.chatID, meal.message, text, b.openKeyboard())
	case "drop":
		answer("")
		b.edit(ctx, meal.chatID, meal.message, describeDraft(meal.draft, false)+"\n\nНе записано.", nil)
	case "ans":
		option := -1
		if len(parts) == 3 {
			option, _ = strconv.Atoi(parts[2])
		}
		if option < 0 || option >= len(meal.draft.Options) {
			b.release(meal)
			answer("")
			return
		}
		answer("Пересчитываю…")
		reply := meal.draft.Options[option]
		b.edit(ctx, meal.chatID, meal.message, describeDraft(meal.draft, false)+"\n\n"+reply+" — пересчитываю…", nil)
		d.running.Add(1)
		go func() {
			defer d.running.Done()
			b.reestimate(ctx, id, meal, reply)
		}()
	default:
		answer("")
	}
}

func (b *Bot) reestimate(ctx context.Context, id string, meal *pendingMeal, reply string) {
	defer b.release(meal)
	draft, err := b.Analyzer.Analyze(ctx, meal.user, estimate.Input{
		Photos: meal.photos, Note: meal.note, Question: meal.draft.Question, Answer: reply,
	})
	if err == nil && len(draft.Items) == 0 {
		err = estimate.ErrUnusable
	}
	if err != nil {
		b.edit(ctx, meal.chatID, meal.message, describeDraft(meal.draft, true)+"\n\n"+analysisFailure(err), draftKeyboard(id, meal.draft))
		return
	}
	b.drafts.mu.Lock()
	meal.draft = draft
	b.drafts.mu.Unlock()
	b.edit(ctx, meal.chatID, meal.message, describeDraft(draft, true), draftKeyboard(id, draft))
}

func (b *Bot) release(meal *pendingMeal) {
	b.drafts.mu.Lock()
	meal.busy = false
	b.drafts.mu.Unlock()
}

func (b *Bot) edit(ctx context.Context, chatID, messageID int64, text string, keyboard *telegram.InlineKeyboardMarkup) {
	err := b.Client.EditMessageText(ctx, telegram.EditMessageTextRequest{
		ChatID: chatID, MessageID: messageID, Text: text, ReplyMarkup: keyboard,
	})
	if err != nil {
		b.log().Warn("could not update the answer to a photo", "chat", chatID, "err", err)
	}
}

func (b *Bot) openKeyboard() *telegram.InlineKeyboardMarkup {
	if b.AppURL == "" {
		return nil
	}
	return telegram.OpenAppKeyboard(buttonText, b.AppURL)
}

// describeDraft writes a draft out as plain text; with ask, the model's question
// goes under it.
func describeDraft(draft model.Draft, ask bool) string {
	var text strings.Builder
	title := strings.TrimSpace(draft.Title)
	if title == "" {
		title = "Приём пищи"
	}
	text.WriteString(title + "\n")
	var kcal, protein, fat, carbs float64
	for _, item := range draft.Items {
		k := item.Grams / 100
		kcal += item.Kcal()
		protein += k * item.Protein100
		fat += k * item.Fat100
		carbs += k * item.Carbs100
		fmt.Fprintf(&text, "\n• %s — %s г · %s ккал", item.Name, thousands(item.Grams), thousands(item.Kcal()))
	}
	fmt.Fprintf(&text, "\n\nИтого %s ккал · Б %.0f · Ж %.0f · У %.0f г", thousands(kcal), protein, fat, carbs)
	if ask && strings.TrimSpace(draft.Question) != "" {
		text.WriteString("\n\n❓ " + draft.Question)
	}
	return text.String()
}

// draftKeyboard is a row per ready answer, then Save and Cancel.
func draftKeyboard(id string, draft model.Draft) *telegram.InlineKeyboardMarkup {
	var rows [][]telegram.InlineKeyboardButton
	if strings.TrimSpace(draft.Question) != "" {
		for i, option := range draft.Options {
			rows = append(rows, []telegram.InlineKeyboardButton{{Text: option, CallbackData: fmt.Sprintf("%sans:%s:%d", mealData, id, i)}})
		}
	}
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "✅ Записать", CallbackData: mealData + "save:" + id},
		{Text: "Отмена", CallbackData: mealData + "drop:" + id},
	})
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
}
