package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/model"
	"github.com/aetoneilya/kcal-bot/api/internal/telegram"
)

// Owner sends the bot's owner notices about who uses it — a new person, a
// person who hit the daily spend cap — each with a button to block them.
// With no ID set every notice is dropped.
type Owner struct {
	Client *telegram.Client
	ID     int64
	Log    *slog.Logger
}

const (
	blockData   = "block:"
	unblockData = "unblock:"
	statusLine  = "Статус: "
)

// Notices go out from request handlers, so they run detached and bounded: a
// slow Telegram must never hold up someone's diary.
func (o *Owner) send(text string, userID int64) {
	if o == nil || o.ID == 0 || userID == o.ID {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err := o.Client.SendMessage(ctx, telegram.SendMessageRequest{
			ChatID:      o.ID,
			Text:        text,
			ReplyMarkup: blockKeyboard(userID, false),
		})
		if err != nil {
			o.log().Warn("could not notify the owner", "err", err)
		}
	}()
}

func (o *Owner) log() *slog.Logger {
	if o.Log != nil {
		return o.Log
	}
	return slog.Default()
}

// NewUser tells the owner someone opened the bot or the app for the first time.
func (o *Owner) NewUser(_ context.Context, user model.User) {
	o.send("Новый пользователь: "+describe(user), user.ID)
}

// SpendLimitReached tells the owner a person used up today's model budget.
func (o *Owner) SpendLimitReached(_ context.Context, user model.User, spent, limit float64) {
	o.send(fmt.Sprintf("%s потратил(а) сегодня $%.2f при лимите $%.2f. Распознавание для него остановлено до завтра (UTC).",
		describe(user), spent, limit), user.ID)
}

func describe(user model.User) string {
	name := strings.TrimSpace(user.FirstName)
	if name == "" {
		name = "Без имени"
	}
	if user.Username != "" {
		name += " (@" + user.Username + ")"
	}
	return fmt.Sprintf("%s, id %d", name, user.ID)
}

func blockKeyboard(userID int64, blocked bool) *telegram.InlineKeyboardMarkup {
	if blocked {
		return telegram.ActionKeyboard("Разблокировать", unblockData+strconv.FormatInt(userID, 10))
	}
	return telegram.ActionKeyboard("Заблокировать", blockData+strconv.FormatInt(userID, 10))
}

// withStatus replaces the status line at the end of a notice.
func withStatus(text string, blocked bool) string {
	if i := strings.LastIndex(text, "\n\n"+statusLine); i >= 0 {
		text = text[:i]
	}
	if blocked {
		return text + "\n\n" + statusLine + "заблокирован"
	}
	return text + "\n\n" + statusLine + "доступ открыт"
}
