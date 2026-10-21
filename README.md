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

## Access and limits

- `ALLOW_USER_IDS` — comma-separated Telegram ids. Anyone else gets a 403 in the app, and `/start` tells them their id to send you. Empty means open to anyone (the server warns).
- `DAILY_ANALYSIS_LIMIT` — model calls per user per UTC day, default 30. A failed call is refunded. Manual entry is never limited.

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

1. Create the bot in @BotFather; point a domain at the VPS.
2. `cp .env.example .env` and fill in `BOT_TOKEN`, `APP_DOMAIN`, `ALLOW_USER_IDS` and a model key.
3. `docker compose up -d --build` — Caddy gets the certificate, the app sets the bot's menu button and starts answering `/start`.

Backup while running: `docker compose exec kcal /app/kcal-bot -db /data/kcal.db -backup /data/kcal-$(date +%F).db`.

## API

All routes but health need the `X-Telegram-Init-Data` header.

| Route | |
|---|---|
| `GET /api/health` | Readiness, pings the database |
| `GET /api/me` | User, estimator name, analyses left today |
| `POST /api/analyze` | Multipart `photo` and/or `note` → draft (nothing saved) |
| `GET /api/meals?from=&to=` | Meals in `[from, to)`, unix seconds — the client's local day |
| `POST /api/meals` · `PUT /api/meals/{id}` · `DELETE /api/meals/{id}` | Save, replace, delete |
