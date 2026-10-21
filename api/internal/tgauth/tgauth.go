// Package tgauth validates the signed payload Telegram hands to a Mini App.
//
// This is the whole authentication story: the frontend sends window.Telegram
// .WebApp.initData, we recompute its HMAC with the bot token, and a match proves
// Telegram vouched for that user id. Never trust a user id that arrived any
// other way.
package tgauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/model"
)

var (
	ErrInvalid = errors.New("tgauth: signature mismatch")
	ErrExpired = errors.New("tgauth: init data expired")
)

// MaxAge bounds replay of a captured initData string.
const MaxAge = 24 * time.Hour

// clockSkew is how far into the future an auth_date may sit before it reads as
// forged rather than as honest clock drift between Telegram and this host.
const clockSkew = 5 * time.Minute

func mac(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// Validate checks the signature and returns the user Telegram attests to.
func Validate(initData, botToken string, now time.Time) (model.User, error) {
	values, err := url.ParseQuery(initData)
	if err != nil {
		return model.User{}, ErrInvalid
	}

	got := values.Get("hash")
	if got == "" {
		return model.User{}, ErrInvalid
	}
	values.Del("hash")
	// Newer clients also send `signature` (Ed25519, for third-party checks).
	// Only that third-party check leaves it out; the bot-token HMAC covers
	// every field but `hash`, so it stays in the data-check string.

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(values.Get(k))
	}

	// secret_key = HMAC_SHA256(key: "WebAppData", data: bot_token)
	secret := mac([]byte("WebAppData"), []byte(botToken))
	want := hex.EncodeToString(mac(secret, []byte(b.String())))

	if !hmac.Equal([]byte(want), []byte(got)) {
		return model.User{}, ErrInvalid
	}

	// auth_date is mandatory: without it there is no freshness bound and a
	// captured initData string could be replayed forever.
	ts, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return model.User{}, ErrInvalid
	}
	age := now.Sub(time.Unix(ts, 0))
	if age > MaxAge {
		return model.User{}, ErrExpired
	}
	// A timestamp meaningfully in the future is forged — a legitimate one can
	// only drift by clock skew. Reject beyond a small tolerance.
	if age < -clockSkew {
		return model.User{}, ErrExpired
	}

	var u struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		FirstName string `json:"first_name"`
	}
	if err := json.Unmarshal([]byte(values.Get("user")), &u); err != nil {
		return model.User{}, ErrInvalid
	}
	if u.ID == 0 {
		return model.User{}, ErrInvalid
	}

	return model.User{ID: u.ID, Username: u.Username, FirstName: u.FirstName}, nil
}
