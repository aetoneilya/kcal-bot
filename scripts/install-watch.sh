#!/usr/bin/env bash
# Install the outside health check (deploy/watch) on a host other than the
# app's own — it must keep running when the app's server is down.
#
#   scripts/install-watch.sh
#
# The host comes from WATCH_HOST, else from WATCH_HOST in the local .env.
# /etc/kcal-watch.env on that host is left alone if it exists; create it by
# hand the first time (see deploy/watch/kcal-watch.sh).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
WATCH_HOST="${WATCH_HOST:-$(sed -n 's/^WATCH_HOST=//p' "$root/.env" 2>/dev/null)}"
: "${WATCH_HOST:?set WATCH_HOST or WATCH_HOST in .env, e.g. root@other-vps}"

scp -q "$root/deploy/watch/kcal-watch.sh" "$WATCH_HOST:/usr/local/bin/kcal-watch.sh"
scp -q "$root/deploy/watch/kcal-watch.service" "$root/deploy/watch/kcal-watch.timer" "$WATCH_HOST:/etc/systemd/system/"
ssh -o BatchMode=yes "$WATCH_HOST" '
  set -e
  chmod 755 /usr/local/bin/kcal-watch.sh
  test -f /etc/kcal-watch.env || { echo "!! create /etc/kcal-watch.env first (BOT_TOKEN, CHAT_ID, URL)" >&2; exit 1; }
  chmod 600 /etc/kcal-watch.env
  systemctl daemon-reload
  systemctl enable --now kcal-watch.timer
  systemctl start kcal-watch.service
  systemctl list-timers kcal-watch.timer --no-pager | head -3
'
echo "✓ watching"
