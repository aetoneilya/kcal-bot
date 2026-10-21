package bot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aetoneilya/kcal-bot/api/internal/model"
	"github.com/aetoneilya/kcal-bot/api/internal/store"
	"github.com/aetoneilya/kcal-bot/api/internal/telegram"
)

// fakeAPI records the sendMessage and editMessageText bodies the bot posts.
func fakeAPI(t *testing.T) (*telegram.Client, *[]telegram.SendMessageRequest) {
	t.Helper()
	var sent []telegram.SendMessageRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") || strings.HasSuffix(r.URL.Path, "/editMessageText") {
			var req telegram.SendMessageRequest
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("bad sendMessage body: %v", err)
			}
			sent = append(sent, req)
		}
		w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	t.Cleanup(srv.Close)
	return &telegram.Client{Token: "t", BaseURL: srv.URL}, &sent
}

func start(from int64) telegram.Update {
	return telegram.Update{Message: &telegram.Message{
		Text: "/start",
		Chat: telegram.Chat{ID: from, Type: "private"},
		From: &telegram.User{ID: from},
	}}
}

func TestStartOpensTheApp(t *testing.T) {
	client, sent := fakeAPI(t)
	b := &Bot{Client: client, AppURL: "https://kcal.example"}
	b.Handle(context.Background(), start(7))
	if len(*sent) != 1 || (*sent)[0].ReplyMarkup == nil || (*sent)[0].ReplyMarkup.InlineKeyboard[0][0].WebApp.URL != "https://kcal.example" {
		t.Fatalf("want one message with an Open button, got %+v", *sent)
	}
}

func TestStrangersAndMissingAppGetTheirID(t *testing.T) {
	cases := map[string]*Bot{
		"not allowed": {AppURL: "https://kcal.example", Allowed: func(id int64) bool { return id == 1 }},
		"no app yet":  {},
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			client, sent := fakeAPI(t)
			b.Client = client
			b.Handle(context.Background(), start(42))
			if len(*sent) != 1 || !strings.Contains((*sent)[0].Text, "42") || (*sent)[0].ReplyMarkup != nil {
				t.Fatalf("want the id and no button, got %+v", *sent)
			}
		})
	}
}

func TestGroupChatsAreIgnored(t *testing.T) {
	client, sent := fakeAPI(t)
	update := start(7)
	update.Message.Chat.Type = "group"
	(&Bot{Client: client, AppURL: "https://kcal.example"}).Handle(context.Background(), update)
	if len(*sent) != 0 {
		t.Fatalf("the bot must stay quiet in groups, sent %+v", *sent)
	}
}

type memUsers struct {
	known   map[int64]bool
	blocked map[int64]bool
}

func (m *memUsers) EnsureUser(_ context.Context, u model.User) (store.Account, error) {
	created := !m.known[u.ID]
	m.known[u.ID] = true
	return store.Account{Created: created, Blocked: m.blocked[u.ID]}, nil
}

func (m *memUsers) User(_ context.Context, id int64) (model.User, bool, error) {
	if !m.known[id] {
		return model.User{}, false, store.ErrNotFound
	}
	return model.User{ID: id, FirstName: "Гость"}, m.blocked[id], nil
}

func (m *memUsers) SetBlocked(_ context.Context, id int64, blocked bool) error {
	if !m.known[id] {
		return store.ErrNotFound
	}
	m.blocked[id] = blocked
	return nil
}

func TestOwnerBlocksFromTheNoticeButton(t *testing.T) {
	client, sent := fakeAPI(t)
	users := &memUsers{known: map[int64]bool{1: true, 42: true}, blocked: map[int64]bool{}}
	b := &Bot{Client: client, AppURL: "https://kcal.example", Users: users, Owner: &Owner{Client: client, ID: 1}}

	press := func(from int64, data string) {
		b.Handle(context.Background(), telegram.Update{CallbackQuery: &telegram.CallbackQuery{
			ID: "q", From: telegram.User{ID: from}, Data: data,
			Message: &telegram.Message{MessageID: 5, Chat: telegram.Chat{ID: 1}, Text: "Новый пользователь: Гость, id 42"},
		}})
	}
	press(42, "block:42") // not the owner
	if users.blocked[42] {
		t.Fatal("only the owner may block")
	}
	press(1, "block:42")
	if !users.blocked[42] {
		t.Fatal("owner's press did not block")
	}
	edit := (*sent)[len(*sent)-1]
	if !strings.HasSuffix(edit.Text, "Статус: заблокирован") || edit.ReplyMarkup.InlineKeyboard[0][0].CallbackData != "unblock:42" {
		t.Fatalf("notice must flip to Unblock, got %+v", edit)
	}

	b.Handle(context.Background(), start(42))
	if got := (*sent)[len(*sent)-1]; got.Text != blockedText || got.ReplyMarkup != nil {
		t.Fatalf("a blocked user must not get the app button, got %+v", got)
	}

	press(1, "unblock:42")
	if users.blocked[42] {
		t.Fatal("unblock did not stick")
	}
}

func TestOwnerCommandsAndNewUserNotice(t *testing.T) {
	client, sent := fakeAPI(t)
	users := &memUsers{known: map[int64]bool{1: true}, blocked: map[int64]bool{}}
	b := &Bot{Client: client, AppURL: "https://kcal.example", Users: users}

	b.Handle(context.Background(), start(77))
	if !users.known[77] || len(*sent) != 1 {
		t.Fatalf("/start must record the person and answer once, sent %+v", *sent)
	}

	b.Owner = &Owner{Client: client, ID: 1}
	command := start(1)
	command.Message.Text = "/block 77"
	b.Handle(context.Background(), command)
	if !users.blocked[77] || !strings.Contains((*sent)[len(*sent)-1].Text, "Заблокирован") {
		t.Fatalf("/block from the owner failed, sent %+v", (*sent)[len(*sent)-1])
	}
	command.Message.Text = "/block 5"
	b.Handle(context.Background(), command)
	if !strings.Contains((*sent)[len(*sent)-1].Text, "ещё не было") {
		t.Fatalf("unknown id must be reported, sent %+v", (*sent)[len(*sent)-1])
	}
}
