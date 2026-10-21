// Package bot is the launch flow: /start answers with a button that opens the
// Mini App, and the menu button points at it too. A photo sent to the bot is
// estimated and saved from the chat (see meals.go). Everything else happens
// in the app — except the owner's controls: notices about new people and
// spend, with block and unblock buttons, and /block, /unblock commands.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/model"
	"github.com/aetoneilya/kcal-bot/api/internal/store"
	"github.com/aetoneilya/kcal-bot/api/internal/telegram"
	"github.com/aetoneilya/kcal-bot/api/internal/treats"
)

// Users is the part of the store the bot needs.
type Users interface {
	EnsureUser(ctx context.Context, user model.User) (store.Account, error)
	User(ctx context.Context, id int64) (model.User, bool, error)
	SetBlocked(ctx context.Context, id int64, blocked bool) error
}

type Bot struct {
	Client *telegram.Client
	// AppURL is the HTTPS origin of the Mini App, used in the web_app buttons.
	// Empty means the app is not reachable yet: the bot still answers, so a
	// new user can learn their id, but without an Open button.
	AppURL string
	// Allowed reports whether a Telegram user may use the app. Nil allows all.
	Allowed func(userID int64) bool
	// Users records who pressed /start and holds blocks. Nil skips both.
	Users Users
	// Owner gets notices and may block people. Nil or a zero ID disables it.
	Owner *Owner
	// Analyzer and Meals turn photos sent to the bot into diary entries.
	// With either nil, a photo gets the usual "open the app" answer.
	Analyzer Analyzer
	Meals    Meals
	// Treats adds a compliment when a meal is saved from the chat.
	Treats *treats.Treats
	Log    *slog.Logger
	// PollTimeout is the getUpdates long-poll length in seconds. Default 30.
	PollTimeout int
	// RetryDelay is the pause after a failed poll. Default 3s.
	RetryDelay time.Duration
	// AlbumDelay is how long to wait for the rest of an album. Default 1.5s.
	AlbumDelay time.Duration

	drafts drafts
}

const (
	welcomeText = "Мяу! Я KalorieKitten 🐱\n\n" +
		"Как это работает:\n" +
		"1. Сфотографируй еду — до трёх фото, можно вместе с этикеткой.\n" +
		"2. Проверь граммы и нажми «Записать».\n\n" +
		"Фото можно кинуть прямо сюда, в чат, или просто написать, что было на тарелке: «овсянка 200 г с бананом». " +
		"Или открой дневник кнопкой ниже. " +
		"В дневнике в «Норме» укажи рост, вес и цель — посчитаю, сколько тебе есть в день. Там же включается итог недели.\n\n" +
		"Калории считает нейросеть по фото, так что цифры примерные — особенно соусы и масло."
	otherText = "Пришли фото еды или напиши, что было на тарелке, — посчитаю калории. Или открой дневник кнопкой."
	// The bot's profile texts, set on every start so they live with the code.
	botShortDescription = "Сфотографируй еду — посчитаю калории, белки, жиры и углеводы. Дневник, норма и вес в одном приложении 🐱"
	botDescription      = "Мяу! Я KalorieKitten 🐱\n\n" +
		"📸 Сфотографируй тарелку или опиши её словами — разложу на продукты с граммами и посчитаю калории, белки, жиры и углеводы.\n" +
		"✏️ Цифры легко поправить, а частые блюда добавляются в один тап.\n" +
		"🎯 Посчитаю дневную норму под твою цель и покажу, сколько осталось.\n" +
		"⚖️ Вес с графиком, календарь по дням и напоминания — по желанию.\n\n" +
		"Нажми «Старт» и открой дневник."
	noAppText   = "Приложение ещё не подключено — владелец его настраивает. Твой id: %d"
	blockedText = "Доступ к дневнику закрыт."
	buttonText  = "Открыть дневник"
	menuText    = "Дневник"
)

func (b *Bot) log() *slog.Logger {
	if b.Log != nil {
		return b.Log
	}
	return slog.Default()
}

func (b *Bot) pollTimeout() int {
	if b.PollTimeout > 0 {
		return b.PollTimeout
	}
	return 30
}

func (b *Bot) retryDelay() time.Duration {
	if b.RetryDelay > 0 {
		return b.RetryDelay
	}
	return 3 * time.Second
}

// Run installs the menu button and long-polls updates until ctx is cancelled.
// A failed poll is logged and retried; the loop never gives up on its own.
func (b *Bot) Run(ctx context.Context) error {
	if b.AppURL == "" {
		b.log().Warn("no app URL — the bot answers without an Open button")
	} else if err := b.Client.SetChatMenuButton(ctx, menuText, b.AppURL); err != nil {
		b.log().Warn("could not set the bot menu button", "err", err)
	} else {
		b.log().Info("bot menu button points at the app", "url", b.AppURL)
	}
	if err := b.Client.SetMyDescription(ctx, botDescription); err != nil {
		b.log().Warn("could not set the bot description", "err", err)
	}
	if err := b.Client.SetMyShortDescription(ctx, botShortDescription); err != nil {
		b.log().Warn("could not set the bot short description", "err", err)
	}

	var offset int64
	for {
		updates, err := b.Client.GetUpdates(ctx, offset, b.pollTimeout())
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			b.log().Warn("getUpdates failed", "err", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(b.retryDelay()):
			}
			continue
		}
		for _, update := range updates {
			offset = update.UpdateID + 1
			b.Handle(ctx, update)
		}
	}
}

// Handle answers one update. Exported so tests can drive it without polling.
func (b *Bot) Handle(ctx context.Context, update telegram.Update) {
	if update.CallbackQuery != nil {
		b.handlePress(ctx, update.CallbackQuery)
		return
	}
	message := update.Message
	if message == nil || message.From == nil || message.Chat.Type != "private" {
		return
	}
	if b.isOwner(message.From.ID) && b.handleOwnerCommand(ctx, message) {
		return
	}
	blocked := false
	user := model.User{ID: message.From.ID, Username: message.From.Username, FirstName: message.From.FirstName}
	if b.Users != nil {
		account, err := b.Users.EnsureUser(ctx, user)
		if err != nil {
			b.log().Warn("could not record user", "user", user.ID, "err", err)
		}
		if account.Created {
			b.Owner.NewUser(ctx, user)
		}
		blocked = account.Blocked
	}
	request := telegram.SendMessageRequest{ChatID: message.Chat.ID}
	switch {
	case blocked:
		request.Text = blockedText
	case b.Allowed != nil && !b.Allowed(message.From.ID):
		// The id is what the owner needs to add someone, so say it.
		request.Text = fmt.Sprintf("Бот пока закрытый. Попроси владельца добавить твой id: %d", message.From.ID)
	case b.AppURL == "":
		request.Text = fmt.Sprintf(noAppText, message.From.ID)
	case len(message.Photo) > 0 && b.Analyzer != nil && b.Meals != nil:
		b.receivePhoto(ctx, message, user)
		return
	case isDescription(message.Text) && b.Analyzer != nil && b.Meals != nil:
		b.receiveText(ctx, message, user)
		return
	case strings.HasPrefix(strings.TrimSpace(message.Text), "/start"):
		request.Text = welcomeText
		request.ReplyMarkup = telegram.OpenAppKeyboard(buttonText, b.AppURL)
	default:
		request.Text = otherText
		request.ReplyMarkup = telegram.OpenAppKeyboard(buttonText, b.AppURL)
	}
	if err := b.Client.SendMessage(ctx, request); err != nil && !errors.Is(err, context.Canceled) {
		b.log().Warn("could not answer message", "chat", message.Chat.ID, "err", err)
	}
}

func (b *Bot) isOwner(id int64) bool { return b.Owner != nil && b.Owner.ID != 0 && id == b.Owner.ID }

// handlePress serves the buttons under a meal from a photo, and the owner's
// Block / Unblock buttons on notices.
func (b *Bot) handlePress(ctx context.Context, press *telegram.CallbackQuery) {
	if strings.HasPrefix(press.Data, mealData) && b.Meals != nil {
		b.handleMealPress(ctx, press)
		return
	}
	answer := func(text string) {
		if err := b.Client.AnswerCallbackQuery(ctx, press.ID, text); err != nil {
			b.log().Warn("could not answer a button press", "err", err)
		}
	}
	if !b.isOwner(press.From.ID) || b.Users == nil {
		answer("")
		return
	}
	var (
		raw     string
		blocked bool
	)
	switch {
	case strings.HasPrefix(press.Data, blockData):
		raw, blocked = strings.TrimPrefix(press.Data, blockData), true
	case strings.HasPrefix(press.Data, unblockData):
		raw = strings.TrimPrefix(press.Data, unblockData)
	default:
		answer("")
		return
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err == nil {
		err = b.Users.SetBlocked(ctx, id, blocked)
	}
	if err != nil {
		b.log().Warn("could not change a block", "data", press.Data, "err", err)
		answer("Не получилось — попробуй ещё раз")
		return
	}
	if blocked {
		answer("Заблокирован")
	} else {
		answer("Доступ открыт")
	}
	if press.Message != nil {
		err := b.Client.EditMessageText(ctx, telegram.EditMessageTextRequest{
			ChatID:      press.Message.Chat.ID,
			MessageID:   press.Message.MessageID,
			Text:        withStatus(press.Message.Text, blocked),
			ReplyMarkup: blockKeyboard(id, blocked),
		})
		if err != nil {
			b.log().Warn("could not update the notice", "err", err)
		}
	}
}

// handleOwnerCommand runs /block <id> and /unblock <id>. It reports false
// for anything else, which then gets the usual answer.
func (b *Bot) handleOwnerCommand(ctx context.Context, message *telegram.Message) bool {
	fields := strings.Fields(message.Text)
	if len(fields) == 0 || (fields[0] != "/block" && fields[0] != "/unblock") {
		return false
	}
	reply := func(text string) {
		if err := b.Client.SendMessage(ctx, telegram.SendMessageRequest{ChatID: message.Chat.ID, Text: text}); err != nil {
			b.log().Warn("could not answer the owner", "err", err)
		}
	}
	if b.Users == nil {
		reply("Блокировки не настроены.")
		return true
	}
	if len(fields) != 2 {
		reply("Формат: " + fields[0] + " <id>")
		return true
	}
	id, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		reply("id — это число, например " + fields[0] + " 123456789")
		return true
	}
	blocked := fields[0] == "/block"
	user, _, err := b.Users.User(ctx, id)
	if err == nil {
		err = b.Users.SetBlocked(ctx, id, blocked)
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		reply(fmt.Sprintf("Пользователя с id %d ещё не было.", id))
	case err != nil:
		b.log().Warn("could not change a block", "user", id, "err", err)
		reply("Не получилось — попробуй ещё раз.")
	case blocked:
		reply("Заблокирован: " + describe(user))
	default:
		reply("Доступ открыт: " + describe(user))
	}
	return true
}
