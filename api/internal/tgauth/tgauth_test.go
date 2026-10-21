package tgauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

const testToken = "123456:AAH-fake-token-for-tests"

// sign builds a valid initData string the way Telegram would, so the test
// exercises parsing and ordering rather than trusting the code under test.
func sign(t *testing.T, token string, fields map[string]string) string {
	t.Helper()

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s=%s", k, fields[k])
	}

	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	secretMAC.Write([]byte(token))
	final := hmac.New(sha256.New, secretMAC.Sum(nil))
	final.Write([]byte(b.String()))

	v := url.Values{}
	for k, val := range fields {
		v.Set(k, val)
	}
	v.Set("hash", hex.EncodeToString(final.Sum(nil)))
	return v.Encode()
}

func validFields(authDate time.Time) map[string]string {
	return map[string]string{
		"auth_date": fmt.Sprintf("%d", authDate.Unix()),
		"query_id":  "AAHdF6IQAAAAAN0XohDhrOrc",
		"user":      `{"id":42,"username":"aetoneilya","first_name":"Ilya"}`,
	}
}

func TestValidateAcceptsGenuineInitData(t *testing.T) {
	now := time.Now()
	initData := sign(t, testToken, validFields(now))

	user, err := Validate(initData, testToken, now)
	if err != nil {
		t.Fatalf("expected valid init data to pass, got %v", err)
	}
	if user.ID != 42 {
		t.Errorf("user id = %d, want 42", user.ID)
	}
	if user.Username != "aetoneilya" {
		t.Errorf("username = %q, want %q", user.Username, "aetoneilya")
	}
}

// Current clients add an Ed25519 `signature` field, and Telegram's HMAC covers
// it like any other field. Dropping it before hashing rejected every real user.
func TestValidateAcceptsInitDataWithSignatureField(t *testing.T) {
	now := time.Now()
	fields := validFields(now)
	fields["signature"] = "aGVsbG8tZWQyNTUxOS1zaWduYXR1cmU"

	if _, err := Validate(sign(t, testToken, fields), testToken, now); err != nil {
		t.Fatalf("init data with a signature field was rejected: %v", err)
	}
}

func TestValidateRejectsTamperedUser(t *testing.T) {
	now := time.Now()
	initData := sign(t, testToken, validFields(now))

	// Swap in a different user id while keeping the original signature — the
	// exact attack the HMAC exists to stop.
	v, err := url.ParseQuery(initData)
	if err != nil {
		t.Fatal(err)
	}
	v.Set("user", `{"id":9999,"username":"attacker","first_name":"Nope"}`)

	if _, err := Validate(v.Encode(), testToken, now); err == nil {
		t.Fatal("tampered user id was accepted")
	}
}

func TestValidateRejectsWrongToken(t *testing.T) {
	now := time.Now()
	initData := sign(t, testToken, validFields(now))

	if _, err := Validate(initData, "654321:different-token", now); err == nil {
		t.Fatal("signature from a different bot token was accepted")
	}
}

func TestValidateRejectsStaleInitData(t *testing.T) {
	issued := time.Now().Add(-MaxAge - time.Hour)
	initData := sign(t, testToken, validFields(issued))

	if _, err := Validate(initData, testToken, time.Now()); err == nil {
		t.Fatal("expired init data was accepted")
	}
}

func TestValidateRejectsMissingHash(t *testing.T) {
	if _, err := Validate("user=%7B%22id%22%3A1%7D&auth_date=1", testToken, time.Now()); err == nil {
		t.Fatal("init data without a hash was accepted")
	}
}

func TestValidateRejectsFutureAuthDate(t *testing.T) {
	// A correctly-signed payload dated well into the future is forged, not drift.
	issued := time.Now().Add(time.Hour)
	initData := sign(t, testToken, validFields(issued))

	if _, err := Validate(initData, testToken, time.Now()); err == nil {
		t.Fatal("far-future auth_date was accepted")
	}
}

func TestValidateAllowsSmallClockSkew(t *testing.T) {
	// A minute of drift between Telegram and this host must still pass.
	issued := time.Now().Add(time.Minute)
	initData := sign(t, testToken, validFields(issued))

	if _, err := Validate(initData, testToken, time.Now()); err != nil {
		t.Fatalf("rejected a payload within clock skew: %v", err)
	}
}

func TestValidateRejectsMissingAuthDate(t *testing.T) {
	// Sign a payload that has a valid hash but no auth_date, so freshness cannot
	// be bounded and the string could be replayed forever.
	fields := map[string]string{
		"query_id": "AAHdF6IQAAAAAN0XohDhrOrc",
		"user":     `{"id":42,"username":"aetoneilya","first_name":"Ilya"}`,
	}
	initData := sign(t, testToken, fields)

	if _, err := Validate(initData, testToken, time.Now()); err == nil {
		t.Fatal("init data without auth_date was accepted")
	}
}
