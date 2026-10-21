#!/usr/bin/env bash
# Checks the app from outside its own server and tells the owner on Telegram
# when it stops answering, and again when it is back. Run by
# kcal-watch.timer every two minutes; two failures in a row count as down,
# so a single slow answer does not wake anyone.
#
# /etc/kcal-watch.env (root only):
#   BOT_TOKEN=…      the bot that writes the alert
#   CHAT_ID=…        the owner's Telegram id
#   URL=https://kcal.example.com/api/health
#   TELEGRAM_IPS="…"  optional: Bot API addresses to try when the usual one
#                    is blocked from this host (some Russian networks)
set -euo pipefail
. /etc/kcal-watch.env
STATE_DIR=/var/lib/kcal-watch
mkdir -p "$STATE_DIR"
fails=$(cat "$STATE_DIR/fails" 2>/dev/null || echo 0)
alerted=$(cat "$STATE_DIR/alerted" 2>/dev/null || echo 0)

send() {
  local via
  for via in "" ${TELEGRAM_IPS:-}; do
    if curl -fsS --max-time 15 -o /dev/null ${via:+--resolve "api.telegram.org:443:$via"} \
      "https://api.telegram.org/bot${BOT_TOKEN}/sendMessage" \
      --data-urlencode "chat_id=${CHAT_ID}" --data-urlencode "text=$1" 2>/dev/null; then
      return 0
    fi
  done
  echo "could not reach Telegram" >&2
}

if curl -fsS --max-time 15 "$URL" 2>/dev/null | grep -q '"status":"ok"'; then
  if [ "$alerted" = 1 ]; then
    send "✅ KalorieKitten снова работает."
  fi
  echo 0 >"$STATE_DIR/fails"
  echo 0 >"$STATE_DIR/alerted"
  exit 0
fi

fails=$((fails + 1))
echo "$fails" >"$STATE_DIR/fails"
if [ "$fails" -ge 2 ] && [ "$alerted" = 0 ]; then
  send "🔴 KalorieKitten не отвечает уже $((fails * 2)) мин: $URL. Логи: journalctl -u kcal-bot -n 50"
  echo 1 >"$STATE_DIR/alerted"
fi
