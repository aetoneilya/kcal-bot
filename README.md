# kcal-bot

**KalorieKitten** (`@KalorieKittenBot`) — a Telegram Mini App food diary. Take a photo of a meal (or describe it); a vision model breaks it into components with grams and per-100 g nutrition; you fix the numbers and save. Several people can use one bot — each sees only their own diary, identified by Telegram's signed `initData`, no logins.

Built on the same stack as `gymbro`: Go `net/http` API + SQLite, Svelte 5/Vite Mini App, one binary serving both, Caddy for HTTPS.

## Why the model returns a breakdown, not a number

Studies of LLM calorie estimation agree: models recognise food well and misjudge portion weight by 35–50%. So the model returns `name, grams, kcal/protein/fat/carbs per 100 g, confidence` per component, the totals are computed from that, and the editor puts the weight front and centre. Correcting one weight fixes most of the error without another model call. Likely hidden calories (cooking oil, dressing, sugar) come back as separate low-confidence lines, and the model asks one question when something unseen would move the total by more than ~20%.

## Flow

1. `/start` in the bot → button that opens the app (also the menu button).
2. **Сегодня** — day totals, meals, ‹ › to move between days.
3. **Добавить** — camera or gallery, optional note («жарено на масле», «рис 200 г»), or text only. The photo is downscaled to 1280 px in the browser (~200 KB) and never stored.
4. **Editor** — weight ±10 g or typed, per-100 g values behind a tap, add/remove lines, time, title. Nothing is saved until **Записать**.

## Model backends

`ESTIMATOR` picks one; with it unset, whichever key is present wins (Gemini first).

| Backend | Env | Default model | Notes |
|---|---|---|---|
| `gemini` | `GEMINI_API_KEY`, `GEMINI_MODEL` | `gemini-3.5-flash` | Best portion error and price in 2026 benchmarks. Free tier exists, but Google trains on free-tier data. |
| `claude` | `ANTHROPIC_API_KEY`, `CLAUDE_MODEL`, `CLAUDE_EFFORT` | `claude-sonnet-5-5`, effort `medium` | Structured outputs guarantee the JSON shape. Effort is the main cost knob. |
| `fake` | — | — | Fixed plate after 1.5 s. Default under `-dev` with no key; elsewhere only when `ESTIMATOR=fake` is set explicitly. |

Both real backends use the same prompt and JSON schema (`api/internal/estimate`). Rough cost: ~$0.01 per photo, $1.5–3 per person per month at five meals a day. Neither API is reachable from Russia, so the server must run abroad.

## Beyond the photo

- **Several photos per meal** — the plate and its label together; up to three.
- **Photo or text in the chat** — a photo (an album of up to three), or any text that is not a command ("овсянка 200 г с бананом"), sent to the bot gets the same estimate, within the same limits (`api/internal/analysis`), answered with the breakdown, buttons for the model's question, and Save / Cancel. Drafts wait in memory; a restart forgets them.
- **Barcode** — a photo of the barcode is decoded in the browser and looked up in Open Food Facts; no model call.
- **Ещё раз** — meals eaten in the last 60 days, grouped by their foods, most frequent first; a tap copies one.
- **Норма** — sex, birth year, height, weight, goal weight, activity and pace give a daily target (Mifflin–St Jeor with guard rails: a deficit of at most a quarter, a floor of 1200/1500 kcal). The formula lives in `web/src/lib/targets.ts` and, for the bot, in `api/internal/model/targets.go`; their tests pin the same numbers.
- **Вес** — one weigh-in a day with a chart; the newest becomes the profile's weight, so the target follows the scale.
- **Календарь** — each finished day against the target its weight called for: short, on target (±10%), over.
- **Bot messages** — opt-in, on the phone's time zone: a Sunday summary of the week (days logged, average against the target, weight trend) closed by a few warm lines and one healthy habit to try, written by Claude (`api/internal/coach`, `kcal-bot -coach` prints a sample) or a fixed tip without a key; and a midday nudge when nothing is logged.
- **Previews** — the first photo of a meal is kept as a 320 px JPEG (~20 KB) and shown in the diary; from the chat it is Telegram's own small size.
- **Your portions** — the last 60 days of saved items go into the prompt as the person's usual weights.
- **Treats** — for the people in `TREAT_USERS` only: a compliment of the day in the diary, a dancing cat with a compliment after each new meal and on a «Котик» button, and a compliment in the bot's messages. Compliments and videos sit in `TREATS_DIR` on the server, outside git (`api/internal/treats`); `scripts/push-treats.sh` copies a local `treats/` there.

## Access and limits

- Open to anyone by default. With `ADMIN_USER_ID` set, the owner gets a message about each new person with a Block button, and `/block <id>`, `/unblock <id>` in the bot. A blocked person keeps their diary but cannot open it.
- `ALLOW_USER_IDS` — optional allow list instead; anyone else gets a 403 and `/start` tells them their id.
- `DAILY_ANALYSIS_LIMIT` — model calls per user per UTC day, default 30. A failed call is refunded. Manual entry is never limited.
- `DAILY_SPEND_LIMIT_USD` — model spend per user per UTC day, default $0.50, from the token counts each answer reports and the prices in `api/internal/estimate/cost.go`. The owner hears when someone reaches it.
- `DAILY_TOTAL_SPEND_LIMIT_USD` — everyone together, default $5; analysis pauses for all until the next UTC day.

## Local development

Go 1.27, Node 22+.

```bash
make dev                      # API on :8080, dev user, fake estimator
npm --prefix web install
npm --prefix web run dev      # Mini App on :5173, proxies /api to :8080
```

Export `GEMINI_API_KEY` or `ANTHROPIC_API_KEY` before `make dev` to try a real model. `-dev` disables Telegram auth and attributes everything to one local user; without it the server refuses to start unless `BOT_TOKEN` is set.

On this machine npm's global registry points at a corporate Nexus, so `web/.npmrc` pins the public registry; behind the corporate proxy run npm and `go get` under `env -u HTTP_PROXY -u HTTPS_PROXY -u http_proxy -u https_proxy`.

```bash
make check                    # Go tests + vet, Vitest, svelte-check, production build
```

To try it inside Telegram before a VPS exists, run the API with `BOT_TOKEN` and `-static web/dist`, expose it with a tunnel (`cloudflared tunnel --url http://localhost:8080`) and pass the tunnel URL as `-origin`.

## Deployment

The live bot runs as a systemd service on a VPS: `scripts/deploy.sh` runs the checks, builds the binary and the Mini App, backs up the database, swaps both (keeping `*.old`) and rolls back if `/api/health` does not answer after the restart.

Two tools run the configured model on the host that has its key, without Telegram:

- `kcal-bot -probe photo.jpg [-note …]` — one photo; prints the raw answer, tokens and cost.
- `kcal-bot -eval evals/` — a set of weighed meals; prints the error report (see `evals/README.md`).

With Docker instead:

1. Create the bot in @BotFather; point a domain at the VPS.
2. `cp .env.example .env` and fill in `BOT_TOKEN`, `APP_DOMAIN`, `ALLOW_USER_IDS` and a model key.
3. `docker compose up -d --build` — Caddy gets the certificate, the app sets the bot's menu button and starts answering `/start`.

Backup while running: `docker compose exec kcal /app/kcal-bot -db /data/kcal.db -backup /data/kcal-$(date +%F).db`.

## API

All routes but health need the `X-Telegram-Init-Data` header.

| Route | |
|---|---|
| `GET /api/health` | Readiness, pings the database |
| `GET /api/me` | User, estimator name, analyses left today, profile |
| `POST /api/analyze` | Multipart: up to three `photo`, `note`, and `question` + `answer` for a re-estimate → draft (nothing saved) |
| `GET /api/barcode/{code}` | Open Food Facts lookup → one-line draft |
| `GET /api/meals?from=&to=` | Meals in `[from, to)`, unix seconds — the client's local day |
| `GET /api/meals/regular` | Meals worth repeating, most frequent first |
| `POST /api/meals` · `PUT /api/meals/{id}` · `DELETE /api/meals/{id}` | Save (with an optional base64 `photo` preview), replace (keeps the preview), delete |
| `GET /api/meals/{id}/photo` | The meal's preview, ~20 KB JPEG |
| `PUT /api/profile` | Save the profile |
| `GET /api/weights?from=&to=` · `PUT /api/weights/{day}` · `DELETE /api/weights/{day}` | Weigh-ins by local day, `YYYY-MM-DD` |
| `GET /api/reminders` · `PUT /api/reminders` | Bot message settings |
| `GET /api/treats` · `GET /api/treats/cats/{name}` | Compliments and cat videos; 404 for anyone without treats |
