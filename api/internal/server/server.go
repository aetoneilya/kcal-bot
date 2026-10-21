// Package server is kcal-bot's HTTP API and, optionally, the static Mini App.
//
// The API is small enough to stay hand-written on net/http: six routes, JSON
// in and out, and one multipart upload.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/analysis"
	"github.com/aetoneilya/kcal-bot/api/internal/estimate"
	"github.com/aetoneilya/kcal-bot/api/internal/foodfacts"
	"github.com/aetoneilya/kcal-bot/api/internal/model"
	"github.com/aetoneilya/kcal-bot/api/internal/store"
	"github.com/aetoneilya/kcal-bot/api/internal/tgauth"
	"github.com/aetoneilya/kcal-bot/api/internal/treats"
)

type Config struct {
	// BotToken is the Telegram bot token whose secret signs init data.
	// Required unless DevMode is set.
	BotToken string
	// DevMode disables Telegram authentication and attributes every request
	// to DevUser. It is never inferred from a missing token, so a production
	// deploy that forgets BOT_TOKEN fails closed.
	DevMode bool
	DevUser model.User
	// Allowed reports whether a Telegram user may use the app. Nil allows all.
	Allowed func(userID int64) bool
	// DailyLimit caps model calls per user per UTC day; 0 means no cap.
	DailyLimit int
	// DailySpendLimit caps what one user's model calls may cost per UTC day,
	// in USD; 0 means no cap. Only enforceable with a priced estimator.
	DailySpendLimit float64
	// TotalDailySpendLimit caps everyone's model calls together per UTC day,
	// in USD; 0 means no cap. It stops many accounts from adding up to a bill.
	TotalDailySpendLimit float64
	// Products looks barcodes up. Nil turns the barcode route off.
	Products Products
	// Notifier hears about new people and spent budgets. Nil drops them.
	Notifier Notifier
	// Treats are compliments and cat videos for chosen people. Nil gives
	// them to nobody.
	Treats *treats.Treats
	// AllowedOrigin is the single browser origin permitted cross-origin calls
	// in production. Empty means same-origin only. Ignored in DevMode.
	AllowedOrigin string
	// StaticDir, when set, is the built Mini App served at "/".
	StaticDir string
	Log       *slog.Logger
}

// Products finds packaged food by barcode.
type Products interface {
	Lookup(ctx context.Context, code string) (foodfacts.Product, error)
}

// Notifier tells the owner about events worth a look. Implementations must
// return quickly; they are called from request handlers.
type Notifier interface {
	NewUser(ctx context.Context, user model.User)
	SpendLimitReached(ctx context.Context, user model.User, spent, limit float64)
	TotalSpendLimitReached(ctx context.Context, spent, limit float64)
}

type Server struct {
	st        *store.Store
	estimator estimate.Estimator
	analyzer  *analysis.Analyzer
	cfg       Config
}

func New(st *store.Store, estimator estimate.Estimator, cfg Config) *Server {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	analyzer := &analysis.Analyzer{
		Store:                st,
		Estimator:            estimator,
		DailyLimit:           cfg.DailyLimit,
		DailySpendLimit:      cfg.DailySpendLimit,
		TotalDailySpendLimit: cfg.TotalDailySpendLimit,
		Log:                  cfg.Log,
	}
	if cfg.Notifier != nil {
		analyzer.Notifier = cfg.Notifier
	}
	return &Server{st: st, estimator: estimator, analyzer: analyzer, cfg: cfg}
}

// Analyzer is the server's estimate runner with its limits, for the bot to
// share.
func (s *Server) Analyzer() *analysis.Analyzer {
	return s.analyzer
}

// Uploads are downscaled in the browser to ~200 KB; the cap only stops abuse.
const (
	maxUpload = 8 << 20
	maxJSON   = 1 << 20
)

// Meals worth repeating are looked for this far back.
const regularWindow = 60 * 24 * time.Hour

func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/me", s.handleMe)
	api.HandleFunc("PUT /api/profile", s.handleSaveProfile)
	api.HandleFunc("GET /api/reminders", s.handleGetReminders)
	api.HandleFunc("PUT /api/reminders", s.handleSaveReminders)
	api.HandleFunc("GET /api/weights", s.handleListWeights)
	api.HandleFunc("PUT /api/weights/{day}", s.handleSaveWeight)
	api.HandleFunc("DELETE /api/weights/{day}", s.handleDeleteWeight)
	api.HandleFunc("POST /api/analyze", s.handleAnalyze)
	api.HandleFunc("GET /api/meals", s.handleListMeals)
	api.HandleFunc("GET /api/meals/regular", s.handleRegularMeals)
	api.HandleFunc("GET /api/barcode/{code}", s.handleBarcode)
	api.HandleFunc("POST /api/meals", s.handleCreateMeal)
	api.HandleFunc("PUT /api/meals/{id}", s.handleUpdateMeal)
	api.HandleFunc("DELETE /api/meals/{id}", s.handleDeleteMeal)
	api.HandleFunc("GET /api/meals/{id}/photo", s.handleMealPhoto)
	api.HandleFunc("GET /api/treats", s.handleTreats)
	api.HandleFunc("GET /api/treats/cats/{name}", s.handleCat)

	root := http.NewServeMux()
	// Health bypasses authentication and request logging (probes are noise).
	root.HandleFunc("GET /api/health", s.handleHealth)
	root.Handle("/api/", s.authenticate(s.logRequests(api)))
	if s.cfg.StaticDir != "" {
		root.Handle("/", staticHandler(s.cfg.StaticDir))
	}
	return s.cors(root)
}

// --- handlers ----------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.st.Ping(r.Context()); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type meResponse struct {
	model.User
	Estimator    string         `json:"estimator"`
	AnalysesLeft *int           `json:"analysesLeft"` // nil when there is no cap
	Profile      *model.Profile `json:"profile"`      // nil until filled in
	Treats       bool           `json:"treats"`
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	response := meResponse{User: user, Estimator: s.estimator.Name(), Treats: s.cfg.Treats.For(user)}
	if s.cfg.DailyLimit > 0 {
		used, err := s.st.AnalysesUsed(r.Context(), user.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		left := max(s.cfg.DailyLimit-used, 0)
		response.AnalysesLeft = &left
	}
	profile, err := s.st.Profile(r.Context(), user.ID)
	switch {
	case err == nil:
		response.Profile = &profile
	case !errors.Is(err, store.ErrNotFound):
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type treatsResponse struct {
	Compliments []string `json:"compliments"`
	Cats        []string `json:"cats"`
}

// handleTreats lists what there is; to anyone without treats it is a 404,
// the same as a route that does not exist.
func (s *Server) handleTreats(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Treats.For(userFrom(r.Context())) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	compliments, err := s.cfg.Treats.Compliments()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cats, err := s.cfg.Treats.Cats()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, treatsResponse{Compliments: append([]string{}, compliments...), Cats: append([]string{}, cats...)})
}

func (s *Server) handleCat(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Treats.For(userFrom(r.Context())) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	path, mediaType, err := s.cfg.Treats.Cat(r.PathValue("name"))
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Revalidated each time (cheap 304), so a replaced video shows up.
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "private, no-cache")
	http.ServeFile(w, r, path)
}

// profileRequest is what the form sends; UpdatedAt is the server's to set.
type profileRequest struct {
	Sex          model.Sex      `json:"sex"`
	BirthYear    int            `json:"birthYear"`
	HeightCm     float64        `json:"heightCm"`
	WeightKg     float64        `json:"weightKg"`
	GoalWeightKg float64        `json:"goalWeightKg"`
	Activity     model.Activity `json:"activity"`
	PaceKgWeek   float64        `json:"paceKgWeek"`
}

func (s *Server) handleGetReminders(w http.ResponseWriter, r *http.Request) {
	reminders, err := s.st.Reminders(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reminders)
}

func (s *Server) handleSaveReminders(w http.ResponseWriter, r *http.Request) {
	var body model.Reminders
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "request body is not valid reminder settings")
		return
	}
	saved, err := s.st.SaveReminders(r.Context(), userFrom(r.Context()).ID, body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleListWeights takes from and to as YYYY-MM-DD, both inclusive.
func (s *Server) handleListWeights(w http.ResponseWriter, r *http.Request) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	for _, day := range []string{from, to} {
		if _, err := time.Parse(time.DateOnly, day); err != nil {
			writeError(w, http.StatusBadRequest, "from and to must be YYYY-MM-DD")
			return
		}
	}
	weights, err := s.st.Weights(r.Context(), userFrom(r.Context()).ID, from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, weights)
}

func (s *Server) handleSaveWeight(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WeightKg float64 `json:"weightKg"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "request body is not a valid weight")
		return
	}
	saved, err := s.st.SaveWeight(r.Context(), userFrom(r.Context()).ID, model.Weight{Day: r.PathValue("day"), WeightKg: body.WeightKg})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteWeight(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteWeight(r.Context(), userFrom(r.Context()).ID, r.PathValue("day")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSaveProfile(w http.ResponseWriter, r *http.Request) {
	var body profileRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "request body is not a valid profile")
		return
	}
	saved, err := s.st.SaveProfile(r.Context(), userFrom(r.Context()).ID, model.Profile{
		Sex: body.Sex, BirthYear: body.BirthYear, HeightCm: body.HeightCm, WeightKg: body.WeightKg,
		GoalWeightKg: body.GoalWeightKg, Activity: body.Activity, PaceKgWeek: body.PaceKgWeek,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleAnalyze takes multipart form fields "photo" (optional image) and
// "note" (optional text) and returns a Draft. Nothing is saved: the draft is
// the user's to edit, and only the edited meal reaches the diary. The photo
// is never stored.
func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeError(w, http.StatusBadRequest, "expected a multipart form with a photo and/or a note")
		return
	}
	defer r.MultipartForm.RemoveAll()

	in := estimate.Input{
		Note:     strings.TrimSpace(r.FormValue("note")),
		Question: strings.TrimSpace(r.FormValue("question")),
		Answer:   strings.TrimSpace(r.FormValue("answer")),
	}
	for _, check := range []struct {
		field, value string
		max          int
	}{
		{"note", in.Note, model.MaxNoteRunes},
		{"question", in.Question, model.MaxQuestionRunes},
		{"answer", in.Answer, model.MaxAnswerRunes},
	} {
		if err := model.ValidateText(check.field, check.value, check.max); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	headers := r.MultipartForm.File["photo"]
	if len(headers) > estimate.MaxPhotos {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("at most %d photos per meal", estimate.MaxPhotos))
		return
	}
	for _, header := range headers {
		photo, err := readPhoto(header)
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not read the photo")
			return
		}
		if !supportedImage(photo.MIME) {
			s.cfg.Log.Warn("rejected upload", "type", photo.MIME, "declared", header.Header.Get("Content-Type"))
			writeError(w, http.StatusUnsupportedMediaType, "the photo must be JPEG, PNG or WebP")
			return
		}
		in.Photos = append(in.Photos, photo)
	}
	if in.Empty() {
		writeError(w, http.StatusBadRequest, "send a photo, a description, or both")
		return
	}

	draft, err := s.analyzer.Analyze(r.Context(), userFrom(r.Context()), in)
	switch {
	case errors.Is(err, analysis.ErrSpendLimit):
		writeError(w, http.StatusTooManyRequests, "на сегодня лимит распознаваний исчерпан — можно ввести вручную")
		return
	case errors.Is(err, analysis.ErrPaused):
		writeError(w, http.StatusTooManyRequests, "распознавание на сегодня приостановлено — можно ввести вручную")
		return
	case errors.Is(err, estimate.ErrUnusable):
		writeError(w, http.StatusBadGateway, "не получилось разобрать блюдо — попробуй другое фото или опиши словами")
		return
	case errors.Is(err, analysis.ErrModel):
		writeError(w, http.StatusBadGateway, "модель не ответила — попробуй ещё раз")
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

// recordSpend adds a call's cost to the user's day and tells the owner the
// moment the user's or everyone's total crosses its cap. Caps are checked
// before a call, so the crossing call is the last one that day.
// readPhoto reads one uploaded file and sniffs its type from the bytes; the
// declared Content-Type is the client's word and is not trusted.
func readPhoto(header *multipart.FileHeader) (estimate.Photo, error) {
	file, err := header.Open()
	if err != nil {
		return estimate.Photo{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return estimate.Photo{}, err
	}
	return estimate.Photo{Data: data, MIME: http.DetectContentType(data)}, nil
}

func supportedImage(mime string) bool {
	switch mime {
	case "image/jpeg", "image/png", "image/webp":
		return true
	}
	return false
}

// handleListMeals takes from/to as unix seconds: the client's local day.
// handleBarcode turns a product's label data into a one-line draft. The
// per-100 g values come from the label; the weight is a starting guess the
// person adjusts — a serving when the label names one, the whole pack when
// it is small enough to be one, 100 g otherwise.
func (s *Server) handleBarcode(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if len(code) < 8 || len(code) > 14 || strings.Trim(code, "0123456789") != "" {
		writeError(w, http.StatusBadRequest, "штрихкод — это 8–14 цифр")
		return
	}
	if s.cfg.Products == nil {
		writeError(w, http.StatusNotFound, "поиск по штрихкоду выключен")
		return
	}
	product, err := s.cfg.Products.Lookup(r.Context(), code)
	if errors.Is(err, foodfacts.ErrNotFound) {
		writeError(w, http.StatusNotFound, "этого продукта нет в базе — сфотографируй этикетку, посчитаю по ней")
		return
	}
	if err != nil {
		s.cfg.Log.Warn("barcode lookup failed", "code", code, "err", err)
		writeError(w, http.StatusBadGateway, "база продуктов не ответила — попробуй ещё раз или сфотографируй этикетку")
		return
	}
	grams, confidence := 100.0, model.ConfidenceLow
	switch {
	case product.ServingGrams >= 5 && product.ServingGrams <= 1000:
		grams, confidence = product.ServingGrams, model.ConfidenceMedium
	case product.PackageGrams > 0 && product.PackageGrams <= 250:
		grams, confidence = product.PackageGrams, model.ConfidenceMedium
	}
	draft := model.Draft{
		Title: product.Name,
		Items: []model.Item{{
			Name: product.Name, Grams: grams, Kcal100: product.Kcal100,
			Protein100: product.Protein100, Fat100: product.Fat100, Carbs100: product.Carbs100,
			Confidence: confidence,
		}},
	}.Normalize()
	if draft.Items[0].Name == "" {
		draft.Items[0].Name = "Продукт " + code
	}
	writeJSON(w, http.StatusOK, draft)
}

// handleRegularMeals lists the meals worth offering to repeat.
func (s *Server) handleRegularMeals(w http.ResponseWriter, r *http.Request) {
	regulars, err := s.st.RegularMeals(r.Context(), userFrom(r.Context()).ID, time.Now().Add(-regularWindow), 12)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, regulars)
}

func (s *Server) handleListMeals(w http.ResponseWriter, r *http.Request) {
	from, errFrom := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, errTo := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if errFrom != nil || errTo != nil {
		writeError(w, http.StatusBadRequest, "from and to must be unix seconds")
		return
	}
	meals, err := s.st.Meals(r.Context(), userFrom(r.Context()).ID, time.Unix(from, 0), time.Unix(to, 0))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if meals == nil {
		meals = []model.Meal{}
	}
	writeJSON(w, http.StatusOK, meals)
}

type mealRequest struct {
	EatenAt time.Time    `json:"eatenAt"`
	Title   string       `json:"title"`
	Note    string       `json:"note"`
	Items   []model.Item `json:"items"`
	Photo   []byte       `json:"photo"` // base64 preview; optional
}

func (s *Server) decodeMeal(w http.ResponseWriter, r *http.Request) (store.MealInput, bool) {
	var body mealRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "request body is not a valid meal")
		return store.MealInput{}, false
	}
	return store.MealInput{EatenAt: body.EatenAt, Title: body.Title, Note: body.Note, Items: body.Items, Photo: body.Photo}, true
}

func (s *Server) handleCreateMeal(w http.ResponseWriter, r *http.Request) {
	in, ok := s.decodeMeal(w, r)
	if !ok {
		return
	}
	meal, err := s.st.CreateMeal(r.Context(), userFrom(r.Context()).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, meal)
}

func (s *Server) handleMealPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := mealID(w, r)
	if !ok {
		return
	}
	photo, err := s.st.MealPhoto(r.Context(), userFrom(r.Context()).ID, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// A meal's preview never changes once saved.
	w.Header().Set("Content-Type", http.DetectContentType(photo))
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	_, _ = w.Write(photo)
}

func (s *Server) handleUpdateMeal(w http.ResponseWriter, r *http.Request) {
	id, ok := mealID(w, r)
	if !ok {
		return
	}
	in, ok := s.decodeMeal(w, r)
	if !ok {
		return
	}
	meal, err := s.st.UpdateMeal(r.Context(), userFrom(r.Context()).ID, id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, meal)
}

func (s *Server) handleDeleteMeal(w http.ResponseWriter, r *http.Request) {
	id, ok := mealID(w, r)
	if !ok {
		return
	}
	if err := s.st.DeleteMeal(r.Context(), userFrom(r.Context()).ID, id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func mealID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

// --- auth ------------------------------------------------------------------

type userContextKey struct{}

func userFrom(ctx context.Context) model.User {
	user, _ := ctx.Value(userContextKey{}).(model.User)
	return user
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var user model.User
		if s.cfg.DevMode {
			user = s.cfg.DevUser
		} else {
			// Never log the init data itself — it carries the signed user
			// payload. Only the sentinel error and path are recorded.
			validated, err := tgauth.Validate(r.Header.Get("X-Telegram-Init-Data"), s.cfg.BotToken, time.Now())
			if err != nil {
				s.cfg.Log.Warn("rejected request", "err", err, "path", r.URL.Path)
				writeError(w, http.StatusUnauthorized, "открой приложение из Telegram")
				return
			}
			if s.cfg.Allowed != nil && !s.cfg.Allowed(validated.ID) {
				s.cfg.Log.Warn("user not on the allow list", "user", validated.ID)
				writeError(w, http.StatusForbidden, "бот закрытый — попроси владельца добавить твой id: "+strconv.FormatInt(validated.ID, 10))
				return
			}
			user = validated
		}
		account, err := s.st.EnsureUser(r.Context(), user)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if account.Created && s.cfg.Notifier != nil {
			s.cfg.Notifier.NewUser(r.Context(), user)
		}
		if account.Blocked {
			writeError(w, http.StatusForbidden, "доступ к дневнику закрыт")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	})
}

// cors emits permissive headers only in development. In production it
// reflects exactly one configured origin, or nothing: a same-origin Mini App
// needs no CORS header, so omitting it locks out cross-origin callers.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allow := ""
		if s.cfg.DevMode {
			allow = "*"
		} else if origin := r.Header.Get("Origin"); s.cfg.AllowedOrigin != "" && origin == s.cfg.AllowedOrigin {
			allow = origin
		}
		if allow != "" {
			w.Header().Set("Access-Control-Allow-Origin", allow)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Telegram-Init-Data")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			if allow != "*" {
				w.Header().Add("Vary", "Origin")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- responses -------------------------------------------------------------

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrLimit):
		writeError(w, http.StatusTooManyRequests, "на сегодня лимит распознаваний исчерпан — можно ввести вручную")
	default:
		s.cfg.Log.Error("request failed", "err", err, "path", r.URL.Path)
		writeError(w, http.StatusInternalServerError, "something went wrong on our side")
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
