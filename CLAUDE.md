# CLAUDE.md

What the README doesn't say: how to work in this repo. Read the README first for the product, the API and the env.

## Shape of a change

A feature usually goes all the way through, in this order:

1. `api/internal/store/migrations/NNN_*.sql` — a new numbered file, never an edit to an old one; a header comment says what the table holds.
2. `api/internal/model` — the type with its JSON tags, limits as named constants, `Validate…` returning `invalid(…)` (wraps `ErrInvalid` → 400).
3. `api/internal/store` — queries scoped by `user_id`; someone else's id is `ErrNotFound`, never a 403.
4. `api/internal/server` — the route in `Handler()`, a handler that decodes with `DisallowUnknownFields` and reports through `s.fail`.
5. Tests next to each: `store_test.go` (`openTest`, users 1 and 2), `server_test.go` (`newTestServer`, `do(t, h, userID, …)`); always check that another user can't see or change it.
6. `web/src/lib/api.ts` — the type and the call; then a component in `web/src/components`, wired into a screen.
7. A line in the README's feature list and API table.

## Conventions

- Comments say why, in full sentences; a component opens with a comment on what it is and how it behaves.
- Days are the phone's local `YYYY-MM-DD` strings (`dayKey` in `web/src/lib/day.ts`); meals use unix seconds.
- Saves are optimistic: change the screen at once, send one request at a time in order (a promise chain), put back what the server last confirmed on failure, show «Не сохранилось — проверь связь».
- UI text is Russian, informal «ты», no emoji. Code, comments and commits are English.

## Look and motion

- Colours come from tokens in `web/src/app.css`: Telegram's theme (`--tg-theme-*`), the glass (`.card`, `.group`, `--glass`, `--blur`), and meaning colours (`--protein`, `--fat`, `--carbs`, `--water`, `--ok`, `--over`, `--treat`). Macro colours appear only where macros are drawn.
- Dark mode follows `Telegram.WebApp.colorScheme` via `data-theme`, else the system; check both.
- Icons are SVG paths in `components/Icon.svelte`; no icon fonts, no emoji.
- Motion is feedback or continuity, never a show on load. Everything must collapse under Reduce Motion: CSS animations are covered by the rule in `app.css`, JS durations go through `ms()` from `lib/motion.ts`.
- Touch targets ≥ 44 px. Horizontal swipes on the diary change the day — a control that drags sideways stops `touchstart`/`touchend` from reaching it.

## Checks

```bash
make check      # Go tests + vet, Vitest, svelte-check and the production build
gofmt -l api    # must print nothing
```

All green before a commit. To see a change, run the API with `-dev` and Vite (see the README), open it in a browser at phone width and look — in both themes. Only Chrome is available to agents; say so when something can only be judged on a phone in Telegram (iOS WKWebView, haptics, View Transitions).

If other dev servers run on this machine, stop only this repo's: `pkill -f 'kcal-bot/web/node_modules/.bin/vite'`, not `pkill -f vite`.

## Shipping

- Work happens on `main`. Commit a finished, checked piece and push; no AI attribution in commits.
- `scripts/deploy.sh` ships `main` to the server (host from `DEPLOY_HOST` in the local `.env`). It backs up the database before the swap and rolls back on a failed health check; migrations run on start.
- Never commit server addresses, tokens, user ids or anything from `treats/` — the repo is public. Secrets travel host to host through a pipe, never printed.
