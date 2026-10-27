package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/estimate"
	"github.com/aetoneilya/kcal-bot/api/internal/foodfacts"
	"github.com/aetoneilya/kcal-bot/api/internal/model"
	"github.com/aetoneilya/kcal-bot/api/internal/store"
	"github.com/aetoneilya/kcal-bot/api/internal/treats"
)

const testToken = "123456:test-token"

type stubEstimator struct {
	err   error
	cost  float64
	calls int
	last  estimate.Input
}

func (s *stubEstimator) Name() string { return "stub" }

func (s *stubEstimator) Priced() bool { return true }

func (s *stubEstimator) Estimate(_ context.Context, in estimate.Input) (estimate.Result, error) {
	s.calls++
	s.last = in
	if s.err != nil {
		return estimate.Result{Cost: s.cost}, s.err
	}
	return estimate.Result{
		Draft: model.Draft{Title: "Тест", Items: []model.Item{{Name: "яблоко", Grams: 150, Kcal100: 52, Confidence: model.ConfidenceHigh}}},
		Cost:  s.cost,
	}, nil
}

type recordingNotifier struct {
	newUsers []int64
	spent    []float64
	total    []float64
}

func (n *recordingNotifier) NewUser(_ context.Context, user model.User) {
	n.newUsers = append(n.newUsers, user.ID)
}

func (n *recordingNotifier) SpendLimitReached(_ context.Context, _ model.User, spent, _ float64) {
	n.spent = append(n.spent, spent)
}

func (n *recordingNotifier) TotalSpendLimitReached(_ context.Context, spent, _ float64) {
	n.total = append(n.total, spent)
}

func newTestServer(t *testing.T, est estimate.Estimator, cfg Config) http.Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "kcal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg.BotToken = testToken
	cfg.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(st, est, cfg).Handler()
}

// signInitData mirrors how Telegram signs a Mini App payload, so tests go
// through real signature verification rather than a stub.
func signInitData(userID int64) string {
	fields := map[string]string{
		"auth_date": strconv.FormatInt(time.Now().Unix(), 10),
		"user":      fmt.Sprintf(`{"id":%d,"first_name":"T"}`, userID),
	}
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
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(testToken))
	final := hmac.New(sha256.New, secret.Sum(nil))
	final.Write([]byte(b.String()))

	v := url.Values{}
	for k, val := range fields {
		v.Set(k, val)
	}
	v.Set("hash", hex.EncodeToString(final.Sum(nil)))
	return v.Encode()
}

func do(t *testing.T, h http.Handler, userID int64, method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, body)
	if userID != 0 {
		r.Header.Set("X-Telegram-Init-Data", signInitData(userID))
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func jsonBody(t *testing.T, v any) io.Reader {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(raw)
}

// A 1x1 JPEG is enough for content sniffing.
var tinyJPEG = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xD9}

func analyzeForm(t *testing.T, photo []byte, note string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	if photo != nil {
		part, err := form.CreateFormFile("photo", "meal.jpg")
		if err != nil {
			t.Fatal(err)
		}
		part.Write(photo)
	}
	if note != "" {
		form.WriteField("note", note)
	}
	form.Close()
	return &buf, form.FormDataContentType()
}

func TestRequestsWithoutValidInitDataAreRejected(t *testing.T) {
	h := newTestServer(t, &stubEstimator{}, Config{})
	if w := do(t, h, 0, "GET", "/api/me", nil, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned request: %d", w.Code)
	}
	r := httptest.NewRequest("GET", "/api/me", nil)
	r.Header.Set("X-Telegram-Init-Data", strings.Replace(signInitData(7), "%22id%22%3A7", "%22id%22%3A8", 1))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("tampered user id must fail the signature: %d", w.Code)
	}
	if w := do(t, h, 0, "GET", "/api/health", nil, ""); w.Code != http.StatusOK {
		t.Fatalf("health needs no auth: %d", w.Code)
	}
}

func TestProfileIsSavedAndReturnedWithMe(t *testing.T) {
	h := newTestServer(t, &stubEstimator{}, Config{})
	var me struct {
		Profile *model.Profile `json:"profile"`
	}
	w := do(t, h, 7, "GET", "/api/me", nil, "")
	if err := json.Unmarshal(w.Body.Bytes(), &me); err != nil || me.Profile != nil {
		t.Fatalf("a new user has no profile: %v %s", err, w.Body)
	}

	profile := map[string]any{
		"sex": "female", "birthYear": 1990, "heightCm": 168, "weightKg": 70,
		"goalWeightKg": 64, "activity": "light", "paceKgWeek": 0.5,
	}
	if w := do(t, h, 7, "PUT", "/api/profile", jsonBody(t, profile), "application/json"); w.Code != http.StatusOK {
		t.Fatalf("save profile: %d %s", w.Code, w.Body)
	}
	w = do(t, h, 7, "GET", "/api/me", nil, "")
	if err := json.Unmarshal(w.Body.Bytes(), &me); err != nil || me.Profile == nil || me.Profile.GoalWeightKg != 64 {
		t.Fatalf("profile not returned: %v %s", err, w.Body)
	}
	if w := do(t, h, 8, "GET", "/api/me", nil, ""); strings.Contains(w.Body.String(), `"heightCm"`) {
		t.Fatalf("another user sees the profile: %s", w.Body)
	}

	profile["activity"] = "couch"
	if w := do(t, h, 7, "PUT", "/api/profile", jsonBody(t, profile), "application/json"); w.Code != http.StatusBadRequest {
		t.Fatalf("an unknown activity must be rejected: %d", w.Code)
	}
}

func TestNewUsersAreAnnouncedOnce(t *testing.T) {
	notifier := &recordingNotifier{}
	h := newTestServer(t, &stubEstimator{}, Config{Notifier: notifier})
	for range 2 {
		do(t, h, 7, "GET", "/api/me", nil, "")
	}
	do(t, h, 8, "GET", "/api/me", nil, "")
	if fmt.Sprint(notifier.newUsers) != "[7 8]" {
		t.Fatalf("want one notice per new person, got %v", notifier.newUsers)
	}
}

func TestSpendLimitStopsAnalysisAndNotifiesOnce(t *testing.T) {
	notifier := &recordingNotifier{}
	est := &stubEstimator{cost: 0.3}
	h := newTestServer(t, est, Config{DailySpendLimit: 0.5, Notifier: notifier})

	for i, want := range []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests} {
		body, ct := analyzeForm(t, nil, "яблоко")
		if w := do(t, h, 7, "POST", "/api/analyze", body, ct); w.Code != want {
			t.Fatalf("call %d: %d, want %d", i+1, w.Code, want)
		}
	}
	if est.calls != 2 {
		t.Fatalf("the model must not be called past the cap, calls=%d", est.calls)
	}
	if len(notifier.spent) != 1 || notifier.spent[0] != 0.6 {
		t.Fatalf("want one notice at $0.60, got %v", notifier.spent)
	}
	// Another person has their own budget.
	body, ct := analyzeForm(t, nil, "яблоко")
	if w := do(t, h, 8, "POST", "/api/analyze", body, ct); w.Code != http.StatusOK {
		t.Fatalf("another user hit someone else's cap: %d", w.Code)
	}
}

func TestTotalSpendLimitStopsEveryone(t *testing.T) {
	notifier := &recordingNotifier{}
	est := &stubEstimator{cost: 0.4}
	h := newTestServer(t, est, Config{DailySpendLimit: 1, TotalDailySpendLimit: 1, Notifier: notifier})
	for i, c := range []struct {
		user int64
		want int
	}{{7, http.StatusOK}, {8, http.StatusOK}, {9, http.StatusOK}, {10, http.StatusTooManyRequests}} {
		body, ct := analyzeForm(t, nil, "яблоко")
		if w := do(t, h, c.user, "POST", "/api/analyze", body, ct); w.Code != c.want {
			t.Fatalf("call %d: %d, want %d", i+1, w.Code, c.want)
		}
	}
	if len(notifier.total) != 1 || len(notifier.spent) != 0 {
		t.Fatalf("want one total-cap notice and no per-user ones, got %v / %v", notifier.total, notifier.spent)
	}
}

func TestBlockedUsersAreTurnedAway(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "kcal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, &stubEstimator{}, Config{BotToken: testToken, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()

	if w := do(t, h, 7, "GET", "/api/me", nil, ""); w.Code != http.StatusOK {
		t.Fatalf("before the block: %d", w.Code)
	}
	if err := st.SetBlocked(context.Background(), 7, true); err != nil {
		t.Fatal(err)
	}
	if w := do(t, h, 7, "GET", "/api/me", nil, ""); w.Code != http.StatusForbidden {
		t.Fatalf("a blocked user must get 403, got %d", w.Code)
	}
}

type stubProducts map[string]foodfacts.Product

func (p stubProducts) Lookup(_ context.Context, code string) (foodfacts.Product, error) {
	if product, ok := p[code]; ok {
		return product, nil
	}
	return foodfacts.Product{}, foodfacts.ErrNotFound
}

func TestBarcodeBecomesAOneLineDraft(t *testing.T) {
	h := newTestServer(t, &stubEstimator{}, Config{Products: stubProducts{
		"4600000000001": {Name: "Кефир 2,5%", Kcal100: 53, Protein100: 2.9, Fat100: 2.5, Carbs100: 4, PackageGrams: 900},
		"4600000000002": {Name: "Йогурт", Kcal100: 80, PackageGrams: 125},
	}})
	var draft model.Draft
	w := do(t, h, 7, "GET", "/api/barcode/4600000000001", nil, "")
	if err := json.Unmarshal(w.Body.Bytes(), &draft); err != nil || w.Code != http.StatusOK ||
		len(draft.Items) != 1 || draft.Items[0].Grams != 100 || draft.Items[0].Kcal100 != 53 {
		t.Fatalf("a big pack starts at 100 g: %d %s", w.Code, w.Body)
	}
	w = do(t, h, 7, "GET", "/api/barcode/4600000000002", nil, "")
	if err := json.Unmarshal(w.Body.Bytes(), &draft); err != nil || draft.Items[0].Grams != 125 {
		t.Fatalf("a small pack starts whole: %s", w.Body)
	}
	if w := do(t, h, 7, "GET", "/api/barcode/4600000000009", nil, ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown product: %d", w.Code)
	}
	if w := do(t, h, 7, "GET", "/api/barcode/12ab", nil, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("not a barcode: %d", w.Code)
	}
}

func TestAllowListBlocksStrangers(t *testing.T) {
	h := newTestServer(t, &stubEstimator{}, Config{Allowed: func(id int64) bool { return id == 7 }})
	if w := do(t, h, 7, "GET", "/api/me", nil, ""); w.Code != http.StatusOK {
		t.Fatalf("allowed user: %d %s", w.Code, w.Body)
	}
	w := do(t, h, 8, "GET", "/api/me", nil, "")
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "8") {
		t.Fatalf("stranger must get 403 with their id: %d %s", w.Code, w.Body)
	}
}

func TestAnalyzeReturnsDraftAndCountsAllowance(t *testing.T) {
	est := &stubEstimator{}
	h := newTestServer(t, est, Config{DailyLimit: 1})

	body, ct := analyzeForm(t, tinyJPEG, "без масла")
	w := do(t, h, 7, "POST", "/api/analyze", body, ct)
	if w.Code != http.StatusOK {
		t.Fatalf("analyze: %d %s", w.Code, w.Body)
	}
	var draft model.Draft
	if err := json.Unmarshal(w.Body.Bytes(), &draft); err != nil || len(draft.Items) != 1 {
		t.Fatalf("bad draft: %v %s", err, w.Body)
	}
	if len(est.last.Photos) != 1 || est.last.Photos[0].MIME != "image/jpeg" || est.last.Note != "без масла" {
		t.Fatalf("estimator got %+v / %q", est.last.Photos, est.last.Note)
	}

	body, ct = analyzeForm(t, nil, "яблоко")
	if w := do(t, h, 7, "POST", "/api/analyze", body, ct); w.Code != http.StatusTooManyRequests {
		t.Fatalf("second call over the limit: %d", w.Code)
	}
	if est.calls != 1 {
		t.Fatalf("the model must not be called over the limit, calls=%d", est.calls)
	}
}

func TestAnalyzePassesAnswerAndPortionHistory(t *testing.T) {
	est := &stubEstimator{}
	h := newTestServer(t, est, Config{})

	meal := map[string]any{
		"eatenAt": time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
		"items": []model.Item{
			{Name: "Овсянка", Grams: 240, Kcal100: 88, Confidence: model.ConfidenceHigh},
		},
	}
	for _, grams := range []float64{240, 260} {
		meal["items"].([]model.Item)[0].Grams = grams
		if w := do(t, h, 7, "POST", "/api/meals", jsonBody(t, meal), "application/json"); w.Code != http.StatusCreated {
			t.Fatalf("create meal: %d %s", w.Code, w.Body)
		}
	}
	// Someone else's oatmeal must not leak into this person's portions.
	if w := do(t, h, 8, "POST", "/api/meals", jsonBody(t, meal), "application/json"); w.Code != http.StatusCreated {
		t.Fatalf("create meal: %d %s", w.Code, w.Body)
	}

	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	form.WriteField("note", "овсянка")
	form.WriteField("question", "На молоке или на воде?")
	form.WriteField("answer", "На воде")
	form.Close()
	if w := do(t, h, 7, "POST", "/api/analyze", &buf, form.FormDataContentType()); w.Code != http.StatusOK {
		t.Fatalf("analyze: %d %s", w.Code, w.Body)
	}

	if est.last.Question != "На молоке или на воде?" || est.last.Answer != "На воде" {
		t.Fatalf("clarification not passed: %+v", est.last)
	}
	if len(est.last.History) != 1 || est.last.History[0].Grams != 250 || est.last.History[0].Times != 2 {
		t.Fatalf("want one usual portion of 250 g from two meals, got %+v", est.last.History)
	}
}

func TestAnalyzeFailureRefundsAllowance(t *testing.T) {
	est := &stubEstimator{err: errors.New("upstream timeout")}
	h := newTestServer(t, est, Config{DailyLimit: 1})
	for i := 0; i < 2; i++ {
		body, ct := analyzeForm(t, nil, "яблоко")
		if w := do(t, h, 7, "POST", "/api/analyze", body, ct); w.Code != http.StatusBadGateway {
			t.Fatalf("attempt %d: %d", i, w.Code)
		}
	}
	if est.calls != 2 {
		t.Fatalf("a failed call must be refunded so the retry reaches the model, calls=%d", est.calls)
	}
}

func TestAnalyzeTakesSeveralPhotosUpToTheCap(t *testing.T) {
	est := &stubEstimator{}
	h := newTestServer(t, est, Config{})
	form := func(n int) (io.Reader, string) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for i := 0; i < n; i++ {
			part, _ := mw.CreateFormFile("photo", fmt.Sprintf("p%d.jpg", i))
			part.Write(tinyJPEG)
		}
		mw.Close()
		return &buf, mw.FormDataContentType()
	}
	body, ct := form(3)
	if w := do(t, h, 7, "POST", "/api/analyze", body, ct); w.Code != http.StatusOK || len(est.last.Photos) != 3 {
		t.Fatalf("three photos: %d, estimator got %d", w.Code, len(est.last.Photos))
	}
	body, ct = form(4)
	if w := do(t, h, 7, "POST", "/api/analyze", body, ct); w.Code != http.StatusBadRequest {
		t.Fatalf("four photos must be refused, got %d", w.Code)
	}
}

func TestAnalyzeRejectsBadUploads(t *testing.T) {
	h := newTestServer(t, &stubEstimator{}, Config{})
	body, ct := analyzeForm(t, nil, "")
	if w := do(t, h, 7, "POST", "/api/analyze", body, ct); w.Code != http.StatusBadRequest {
		t.Fatalf("empty form: %d", w.Code)
	}
	body, ct = analyzeForm(t, []byte("%PDF-1.7 not a photo"), "")
	if w := do(t, h, 7, "POST", "/api/analyze", body, ct); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("non-image upload: %d", w.Code)
	}
}

func TestMealLifecycleIsPerUser(t *testing.T) {
	h := newTestServer(t, &stubEstimator{}, Config{})
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	meal := map[string]any{
		"eatenAt": at,
		"title":   "Обед",
		"items":   []model.Item{{Name: "суп", Grams: 300, Kcal100: 40, Confidence: model.ConfidenceMedium}},
	}

	w := do(t, h, 7, "POST", "/api/meals", jsonBody(t, meal), "application/json")
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var created model.Meal
	json.Unmarshal(w.Body.Bytes(), &created)
	path := "/api/meals/" + strconv.FormatInt(created.ID, 10)
	day := fmt.Sprintf("/api/meals?from=%d&to=%d", at.Add(-12*time.Hour).Unix(), at.Add(12*time.Hour).Unix())

	if w := do(t, h, 8, "PUT", path, jsonBody(t, meal), "application/json"); w.Code != http.StatusNotFound {
		t.Fatalf("another user must not edit: %d", w.Code)
	}
	if w := do(t, h, 8, "DELETE", path, nil, ""); w.Code != http.StatusNotFound {
		t.Fatalf("another user must not delete: %d", w.Code)
	}
	if w := do(t, h, 8, "GET", day, nil, ""); strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("another user must see an empty day: %s", w.Body)
	}

	meal["title"] = "Обед, без хлеба"
	if w := do(t, h, 7, "PUT", path, jsonBody(t, meal), "application/json"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "без хлеба") {
		t.Fatalf("owner update: %d %s", w.Code, w.Body)
	}
	if w := do(t, h, 7, "GET", day, nil, ""); !strings.Contains(w.Body.String(), "без хлеба") {
		t.Fatalf("owner list: %s", w.Body)
	}
	if w := do(t, h, 7, "DELETE", path, nil, ""); w.Code != http.StatusNoContent {
		t.Fatalf("owner delete: %d", w.Code)
	}
}

func TestMealRejectsUnknownFields(t *testing.T) {
	h := newTestServer(t, &stubEstimator{}, Config{})
	body := `{"eatenAt":"2026-10-19T12:00:00Z","items":[{"name":"x","grams":1,"kcal100":1}],"userId":8}`
	if w := do(t, h, 7, "POST", "/api/meals", strings.NewReader(body), "application/json"); w.Code != http.StatusBadRequest {
		t.Fatalf("a body naming another user must be rejected, got %d", w.Code)
	}
}

func TestComplimentsAreForChosenPeopleCatsForAll(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cats"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compliments.txt"), []byte("Ты чудо\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cats", "dance.mp4"), []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newTestServer(t, &stubEstimator{}, Config{Treats: treats.New(dir, "7")})

	var me struct{ Treats bool }
	_ = json.NewDecoder(do(t, h, 7, "GET", "/api/me", nil, "").Body).Decode(&me)
	if !me.Treats {
		t.Fatal("chosen person: me.treats is false")
	}
	w := do(t, h, 7, "GET", "/api/treats", nil, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Ты чудо") || !strings.Contains(w.Body.String(), "dance.mp4") {
		t.Fatalf("treats: %d %s", w.Code, w.Body)
	}

	_ = json.NewDecoder(do(t, h, 8, "GET", "/api/me", nil, "").Body).Decode(&me)
	if me.Treats {
		t.Fatal("someone else: me.treats is true")
	}
	if w := do(t, h, 8, "GET", "/api/treats", nil, ""); w.Code != http.StatusNotFound {
		t.Errorf("someone else gets compliments: %d", w.Code)
	}
	for _, id := range []int64{7, 8} {
		w := do(t, h, id, "GET", "/api/cats", nil, "")
		if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `["dance.mp4"]` {
			t.Fatalf("user %d cats: %d %s", id, w.Code, w.Body)
		}
		w = do(t, h, id, "GET", "/api/cats/dance.mp4", nil, "")
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "video/mp4" || w.Body.String() != "not really a video" {
			t.Fatalf("user %d cat: %d %q", id, w.Code, w.Header().Get("Content-Type"))
		}
	}
	if w := do(t, h, 8, "GET", "/api/cats/..%2Fcompliments.txt", nil, ""); w.Code != http.StatusNotFound {
		t.Fatalf("path outside cats: %d", w.Code)
	}
}

func TestMealPhotoIsSavedOnceAndOnlyForItsOwner(t *testing.T) {
	h := newTestServer(t, &stubEstimator{}, Config{})
	jpeg := []byte("\xff\xd8\xff\xe0 a tiny preview")
	meal := map[string]any{
		"eatenAt": time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
		"title":   "Обед",
		"items":   []model.Item{{Name: "суп", Grams: 300, Kcal100: 40, Confidence: model.ConfidenceMedium}},
		"photo":   jpeg, // encoding/json sends []byte as base64, as the app does
	}
	large := []byte("\xff\xd8\xff\xe0 the same, bigger")
	meal["photoLarge"] = large
	w := do(t, h, 7, "POST", "/api/meals", jsonBody(t, meal), "application/json")
	var created model.Meal
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil || w.Code != http.StatusCreated || !created.HasPhoto {
		t.Fatalf("create: %d %+v %v", w.Code, created, err)
	}
	path := fmt.Sprintf("/api/meals/%d/photo", created.ID)
	w = do(t, h, 7, "GET", path, nil, "")
	if w.Code != http.StatusOK || w.Body.String() != string(jpeg) || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("photo: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if w := do(t, h, 8, "GET", path, nil, ""); w.Code != http.StatusNotFound {
		t.Fatalf("someone else's photo: %d", w.Code)
	}
	if w := do(t, h, 7, "GET", path+"?size=large", nil, ""); w.Body.String() != string(large) {
		t.Fatalf("large photo: %d %q", w.Code, w.Body)
	}
	delete(meal, "photoLarge")

	// An edit without a photo keeps it.
	delete(meal, "photo")
	meal["title"] = "Суп"
	if w := do(t, h, 7, "PUT", fmt.Sprintf("/api/meals/%d", created.ID), jsonBody(t, meal), "application/json"); w.Code != http.StatusOK {
		t.Fatalf("update: %d %s", w.Code, w.Body)
	}
	if w := do(t, h, 7, "GET", path, nil, ""); w.Code != http.StatusOK {
		t.Fatalf("photo after an edit: %d", w.Code)
	}

	meal["photo"] = []byte("not an image")
	if w := do(t, h, 7, "POST", "/api/meals", jsonBody(t, meal), "application/json"); w.Code != http.StatusBadRequest {
		t.Fatalf("a non-image photo: %d", w.Code)
	}
	delete(meal, "photo")
	w = do(t, h, 7, "POST", "/api/meals", jsonBody(t, meal), "application/json")
	_ = json.NewDecoder(w.Body).Decode(&created)
	if created.HasPhoto {
		t.Fatal("a meal without a photo says it has one")
	}
	if w := do(t, h, 7, "GET", fmt.Sprintf("/api/meals/%d/photo", created.ID), nil, ""); w.Code != http.StatusNotFound {
		t.Fatalf("no photo: %d", w.Code)
	}
}

func TestWaterIsADayTotalWithAGoalFromWeight(t *testing.T) {
	h := newTestServer(t, &stubEstimator{}, Config{})
	var got struct {
		Day    string
		Ml     int
		GoalMl int
	}
	_ = json.NewDecoder(do(t, h, 7, "GET", "/api/water/2026-10-21", nil, "").Body).Decode(&got)
	if got.Ml != 0 || got.GoalMl != 2000 {
		t.Fatalf("empty day without a profile: %+v", got)
	}
	profile := map[string]any{"sex": "female", "birthYear": 1998, "heightCm": 165, "weightKg": 60,
		"goalWeightKg": 58, "activity": "moderate", "paceKgWeek": 0.25}
	if w := do(t, h, 7, "PUT", "/api/profile", jsonBody(t, profile), "application/json"); w.Code != http.StatusOK {
		t.Fatalf("profile: %d %s", w.Code, w.Body)
	}
	w := do(t, h, 7, "PUT", "/api/water/2026-10-21", jsonBody(t, map[string]int{"ml": 750}), "application/json")
	_ = json.NewDecoder(w.Body).Decode(&got)
	if w.Code != http.StatusOK || got.Ml != 750 || got.GoalMl != 1750 {
		t.Fatalf("save: %d %+v", w.Code, got)
	}
	_ = json.NewDecoder(do(t, h, 7, "GET", "/api/water/2026-10-21", nil, "").Body).Decode(&got)
	if got.Ml != 750 {
		t.Fatalf("read back: %+v", got)
	}
	_ = json.NewDecoder(do(t, h, 8, "GET", "/api/water/2026-10-21", nil, "").Body).Decode(&got)
	if got.Ml != 0 {
		t.Fatal("someone else sees the water")
	}
	for _, bad := range []string{`{"ml": -250}`, `{"ml": 20000}`} {
		if w := do(t, h, 7, "PUT", "/api/water/2026-10-21", strings.NewReader(bad), "application/json"); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	if w := do(t, h, 7, "GET", "/api/water/yesterday", nil, ""); w.Code != http.StatusBadRequest {
		t.Errorf("bad day: %d", w.Code)
	}
}

func TestSupplementsOverHTTP(t *testing.T) {
	h := newTestServer(t, &stubEstimator{}, Config{})
	var sup struct {
		ID    int64
		Name  string
		Dose  string
		Taken bool
	}
	w := do(t, h, 7, "POST", "/api/supplements", jsonBody(t, map[string]string{"name": "Магний", "dose": "400 мг"}), "application/json")
	_ = json.NewDecoder(w.Body).Decode(&sup)
	if w.Code != http.StatusCreated || sup.ID == 0 || sup.Dose != "400 мг" {
		t.Fatalf("add: %d %+v", w.Code, sup)
	}
	path := fmt.Sprintf("/api/supplements/%d", sup.ID)
	if w := do(t, h, 7, "PUT", path+"/days/2026-10-27", jsonBody(t, map[string]bool{"taken": true}), "application/json"); w.Code != http.StatusNoContent {
		t.Fatalf("mark: %d %s", w.Code, w.Body)
	}
	var list []struct {
		ID    int64
		Taken bool
	}
	_ = json.NewDecoder(do(t, h, 7, "GET", "/api/supplements?day=2026-10-27", nil, "").Body).Decode(&list)
	if len(list) != 1 || !list[0].Taken {
		t.Fatalf("list: %+v", list)
	}
	if w := do(t, h, 8, "PUT", path+"/days/2026-10-27", jsonBody(t, map[string]bool{"taken": false}), "application/json"); w.Code != http.StatusNotFound {
		t.Fatalf("someone else's mark: %d", w.Code)
	}
	if w := do(t, h, 8, "DELETE", path, nil, ""); w.Code != http.StatusNotFound {
		t.Fatalf("someone else's delete: %d", w.Code)
	}
	if w := do(t, h, 7, "PUT", path+"/days/today", jsonBody(t, map[string]bool{"taken": true}), "application/json"); w.Code != http.StatusBadRequest {
		t.Errorf("bad day: %d", w.Code)
	}
	if w := do(t, h, 7, "GET", "/api/supplements", nil, ""); w.Code != http.StatusBadRequest {
		t.Errorf("no day: %d", w.Code)
	}
	if w := do(t, h, 7, "POST", "/api/supplements", jsonBody(t, map[string]string{"name": ""}), "application/json"); w.Code != http.StatusBadRequest {
		t.Errorf("empty name: %d", w.Code)
	}
	if w := do(t, h, 7, "DELETE", path, nil, ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}
}

func TestLargePhotoFallsBackToThePreview(t *testing.T) {
	h := newTestServer(t, &stubEstimator{}, Config{})
	jpeg := []byte("\xff\xd8\xff\xe0 only a preview")
	meal := map[string]any{
		"eatenAt": time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
		"items":   []model.Item{{Name: "суп", Grams: 300, Kcal100: 40, Confidence: model.ConfidenceMedium}},
		"photo":   jpeg,
	}
	var created model.Meal
	_ = json.NewDecoder(do(t, h, 7, "POST", "/api/meals", jsonBody(t, meal), "application/json").Body).Decode(&created)
	if w := do(t, h, 7, "GET", fmt.Sprintf("/api/meals/%d/photo?size=large", created.ID), nil, ""); w.Body.String() != string(jpeg) {
		t.Fatalf("fallback: %d %q", w.Code, w.Body)
	}
}
