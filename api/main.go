// Command kcal-bot serves the food-diary Mini App API, the Telegram bot that
// launches it and, with -static, the built app itself.
//
//	go run . -db ./kcal.db -dev
//	go run . -db ./kcal.db -backup ./backups/kcal-2026-10-19.db
//
// Production needs BOT_TOKEN: its secret signs Mini App init data for
// authentication. Development mode (-dev) disables authentication and
// attributes every request to a fixed local user.
//
// The estimator is picked by ESTIMATOR (gemini, claude or fake). With no
// setting it is gemini when GEMINI_API_KEY is set, claude when
// ANTHROPIC_API_KEY is set, and fake otherwise — fake needs no key and runs
// outside -dev only when ESTIMATOR=fake is set explicitly.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // reminders run on each person's clock; do not rely on the host's zone files

	"github.com/aetoneilya/kcal-bot/api/internal/bot"
	"github.com/aetoneilya/kcal-bot/api/internal/coach"
	"github.com/aetoneilya/kcal-bot/api/internal/estimate"
	"github.com/aetoneilya/kcal-bot/api/internal/evalset"
	"github.com/aetoneilya/kcal-bot/api/internal/foodfacts"
	"github.com/aetoneilya/kcal-bot/api/internal/model"
	"github.com/aetoneilya/kcal-bot/api/internal/server"
	"github.com/aetoneilya/kcal-bot/api/internal/store"
	"github.com/aetoneilya/kcal-bot/api/internal/telegram"
	"github.com/aetoneilya/kcal-bot/api/internal/treats"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "kcal.db", "path to the SQLite file")
	backupTo := flag.String("backup", "", "write an online-safe snapshot of the database to this path and exit")
	devMode := flag.Bool("dev", false, "disable Telegram auth; every request is the dev user. Never use in production")
	origin := flag.String("origin", os.Getenv("APP_ORIGIN"), "public origin of the Mini App, e.g. https://kcal.example.com")
	staticDir := flag.String("static", os.Getenv("STATIC_DIR"), "directory with the built Mini App (web/dist) to serve at /; empty serves the API only")
	probe := flag.String("probe", "", "estimate this photo once with the configured model, print the raw answer and cost, and exit")
	probeNote := flag.String("note", "", "with -probe: the note to send alongside the photo")
	coachProbe := flag.Bool("coach", false, "write one sample weekly-summary message with the configured Claude key, print it and its cost, and exit")
	evalDir := flag.String("eval", "", "run the configured model over a directory of weighed meals (see internal/evalset) and print the error report")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if *evalDir != "" {
		cases, err := evalset.Load(*evalDir)
		if err == nil {
			var est estimate.Estimator
			if est, err = newEstimator(context.Background(), *devMode); err == nil {
				err = evalset.Run(context.Background(), est, cases, os.Stdout)
			}
		}
		if err != nil {
			log.Error("eval failed", "err", err)
			os.Exit(1)
		}
		return
	}
	if *coachProbe {
		if err := runCoachProbe(); err != nil {
			log.Error("coach probe failed", "err", err)
			os.Exit(1)
		}
		return
	}
	if *probe != "" {
		if err := runProbe(*probe, *probeNote, *devMode); err != nil {
			log.Error("probe failed", "err", err)
			os.Exit(1)
		}
		return
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Error("cannot open database", "path", *dbPath, "err", err)
		os.Exit(1)
	}
	defer st.Close()

	if *backupTo != "" {
		if err := st.Backup(context.Background(), *backupTo); err != nil {
			log.Error("backup failed", "err", err)
			os.Exit(1)
		}
		log.Info("backup written", "path", *backupTo)
		return
	}

	// Fail closed: production must have a bot token. Dev mode is opt-in and
	// never inferred, so a forgotten BOT_TOKEN can't silently trust everyone.
	botToken := os.Getenv("BOT_TOKEN")
	if *devMode {
		log.Warn("DEV MODE — Telegram authentication is disabled; every request is the dev user")
	} else if botToken == "" {
		log.Error("BOT_TOKEN is required to start; pass -dev to run locally without Telegram auth")
		os.Exit(1)
	}

	allowed, err := parseAllowList(os.Getenv("ALLOW_USER_IDS"))
	if err != nil {
		log.Error("ALLOW_USER_IDS is not a comma-separated list of Telegram ids", "err", err)
		os.Exit(1)
	}
	ownerID, err := parseID(os.Getenv("ADMIN_USER_ID"))
	if err != nil {
		log.Error("ADMIN_USER_ID must be a Telegram user id", "err", err)
		os.Exit(1)
	}
	if allowed == nil && !*devMode && ownerID == 0 {
		log.Warn("ALLOW_USER_IDS and ADMIN_USER_ID are empty — anyone can use the bot and nobody hears about it")
	}

	dailyLimit := 30
	if raw := os.Getenv("DAILY_ANALYSIS_LIMIT"); raw != "" {
		if dailyLimit, err = strconv.Atoi(raw); err != nil || dailyLimit < 0 {
			log.Error("DAILY_ANALYSIS_LIMIT must be a non-negative integer", "value", raw)
			os.Exit(1)
		}
	}

	// A day of normal use costs a few cents; the cap stops one person from
	// draining the API budget.
	spendLimit := 0.5
	if raw := os.Getenv("DAILY_SPEND_LIMIT_USD"); raw != "" {
		if spendLimit, err = strconv.ParseFloat(raw, 64); err != nil || spendLimit < 0 {
			log.Error("DAILY_SPEND_LIMIT_USD must be a non-negative number of dollars", "value", raw)
			os.Exit(1)
		}
	}

	totalSpendLimit := 5.0
	if raw := os.Getenv("DAILY_TOTAL_SPEND_LIMIT_USD"); raw != "" {
		if totalSpendLimit, err = strconv.ParseFloat(raw, 64); err != nil || totalSpendLimit < 0 {
			log.Error("DAILY_TOTAL_SPEND_LIMIT_USD must be a non-negative number of dollars", "value", raw)
			os.Exit(1)
		}
	}

	// Compliments and cat videos for the people named in TREAT_USERS.
	gifts := treats.New(os.Getenv("TREATS_DIR"), os.Getenv("TREAT_USERS"))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	estimator, err := newEstimator(ctx, *devMode)
	if err != nil {
		log.Error("cannot set up the estimator", "err", err)
		os.Exit(1)
	}
	if spendLimit > 0 && !estimator.Priced() {
		log.Warn("the estimator has no price table — DAILY_SPEND_LIMIT_USD is not enforced, only DAILY_ANALYSIS_LIMIT", "estimator", estimator.Name())
	}

	if *staticDir != "" {
		if _, err := os.Stat(*staticDir + "/index.html"); err != nil {
			log.Error("static directory has no index.html — run `npm --prefix web run build` first", "dir", *staticDir, "err", err)
			os.Exit(1)
		}
	}

	client := &telegram.Client{Token: botToken}
	var owner *bot.Owner
	var notifier server.Notifier
	if !*devMode && ownerID != 0 {
		owner = &bot.Owner{Client: client, ID: ownerID, Log: log}
		notifier = owner
	}

	api := server.New(st, estimator, server.Config{
		BotToken:             botToken,
		DevMode:              *devMode,
		DevUser:              model.User{ID: 1, Username: "dev", FirstName: "Dev"},
		Allowed:              allowed,
		DailyLimit:           dailyLimit,
		DailySpendLimit:      spendLimit,
		TotalDailySpendLimit: totalSpendLimit,
		Notifier:             notifier,
		Treats:               gifts,
		Products:             &foodfacts.Client{UserAgent: "KalorieKitten/1.0 (+" + *origin + ")"},
		AllowedOrigin:        *origin,
		StaticDir:            *staticDir,
		Log:                  log,
	})
	srv := &http.Server{Addr: *addr, Handler: api.Handler(), ReadHeaderTimeout: 10 * time.Second}

	// Without the public URL the bot still answers (so people can send you
	// their id), just without the Open button.
	botDone := make(chan struct{})
	if !*devMode {
		launcher := &bot.Bot{Client: client, AppURL: *origin, Allowed: allowed, Users: st, Owner: owner,
			Analyzer: api.Analyzer(), Meals: st, Treats: gifts, Log: log}
		reminders := &bot.Reminders{Client: client, Store: st, AppURL: *origin, Coach: newCoach(), Treats: gifts, Log: log}
		go reminders.Run(ctx)
		go func() {
			defer close(botDone)
			_ = launcher.Run(ctx)
		}()
	} else {
		close(botDone)
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", *addr, "db", *dbPath, "static", *staticDir,
			"estimator", estimator.Name(), "daily_limit", dailyLimit, "daily_spend_usd", spendLimit, "daily_total_spend_usd", totalSpendLimit, "owner", ownerID)
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server stopped", "err", err)
			os.Exit(1)
		}
	case <-ctx.Done():
	}

	// An estimate can take tens of seconds; let in-flight ones finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", "err", err)
	}
	stop()
	select {
	case <-botDone:
	case <-shutdownCtx.Done():
		log.Warn("bot did not stop in time")
	}
	log.Info("bye")
}

func newEstimator(ctx context.Context, devMode bool) (estimate.Estimator, error) {
	geminiKey, claudeKey := os.Getenv("GEMINI_API_KEY"), os.Getenv("ANTHROPIC_API_KEY")
	kind := os.Getenv("ESTIMATOR")
	if kind == "" {
		switch {
		case geminiKey != "":
			kind = "gemini"
		case claudeKey != "":
			kind = "claude"
		default:
			kind = "fake"
		}
	}
	switch kind {
	case "gemini":
		if geminiKey == "" {
			return nil, errors.New("ESTIMATOR=gemini needs GEMINI_API_KEY")
		}
		return estimate.NewGemini(ctx, geminiKey, os.Getenv("GEMINI_MODEL"))
	case "claude":
		if claudeKey == "" {
			return nil, errors.New("ESTIMATOR=claude needs ANTHROPIC_API_KEY")
		}
		return estimate.NewClaude(claudeKey, os.Getenv("CLAUDE_MODEL"), os.Getenv("CLAUDE_EFFORT")), nil
	case "fake":
		// Only ever picked on purpose outside -dev: a production server that
		// silently fell back to a fixed plate would look broken, not free.
		if !devMode && os.Getenv("ESTIMATOR") != "fake" {
			return nil, errors.New("no model API key is set; set GEMINI_API_KEY or ANTHROPIC_API_KEY, or ESTIMATOR=fake to try the app without a model")
		}
		return estimate.Fake{Delay: 1500 * time.Millisecond}, nil
	default:
		return nil, fmt.Errorf("unknown ESTIMATOR %q: use gemini, claude or fake", kind)
	}
}

// parseAllowList returns nil (allow everyone) for an empty list.
func parseAllowList(raw string) (func(int64) bool, error) {
	ids := map[int64]bool{}
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		id, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return nil, err
		}
		ids[id] = true
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return func(id int64) bool { return ids[id] }, nil
}

// runProbe sends one photo through the real estimator, without the server,
// so a model's behaviour can be checked on the host that has its key.
func runProbe(path, note string, devMode bool) error {
	photo, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	ctx := context.Background()
	estimator, err := newEstimator(ctx, devMode)
	if err != nil {
		return err
	}
	started := time.Now()
	result, err := estimator.Estimate(ctx, estimate.Input{
		Photos: []estimate.Photo{{Data: photo, MIME: http.DetectContentType(photo)}},
		Note:   note,
	})
	fmt.Printf("estimator: %s\nms: %d\ntokens in/out: %d/%d\nusd: %.4f\nraw:\n%s\n",
		estimator.Name(), time.Since(started).Milliseconds(), result.Usage.Input, result.Usage.Output, result.Cost, result.Raw)
	return err
}

// newCoach writes the weekly summary's warm lines with Claude when there is
// a key; nil falls back to fixed tips.
func newCoach() coach.Coach {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil
	}
	return coach.NewClaude(key, os.Getenv("CLAUDE_MODEL"))
}

// runCoachProbe prints what the coach writes for a made-up, middling week.
func runCoachProbe() error {
	c := newCoach()
	if c == nil {
		return errors.New("ANTHROPIC_API_KEY is not set")
	}
	weight, change := 61.2, -0.4
	text, cost, err := c.Motivate(context.Background(), coach.Week{
		Name: "Соня", Sex: model.SexFemale, Logged: 5, Kcal: 1720, Protein: 62,
		Target: 1800, OnTarget: 3, Weight: &weight, Change: &change,
	})
	fmt.Printf("usd: %.4f\n\n%s\n", cost, text)
	return err
}

// parseID reads an optional Telegram id; empty is 0.
func parseID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}
