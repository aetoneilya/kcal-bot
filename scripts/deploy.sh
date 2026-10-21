#!/usr/bin/env bash
# Build KalorieKitten and roll it out to the VPS that runs it as a systemd
# service (see README → Deployment on a VPS).
#
#   scripts/deploy.sh            # build, back up the database, swap, restart
#   HOST=root@other scripts/deploy.sh
#
# The previous binary and web build stay next to the new ones as *.old. If
# the restarted service does not answer /api/health, they are put back.
set -euo pipefail

HOST="${HOST:-root@your-vps}"
APP=/opt/kcal-bot
DB=/var/lib/kcal-bot/kcal.db
HEALTH=http://127.0.0.1:8098/api/health

root="$(cd "$(dirname "$0")/.." && pwd)"
out="$(mktemp -d)"
trap 'rm -rf "$out"' EXIT

# The corporate proxy on the dev machine breaks npm and module downloads.
clean_env=(env -u HTTP_PROXY -u HTTPS_PROXY -u http_proxy -u https_proxy)

echo "→ checks"
(cd "$root/api" && go vet ./... && go test ./...) >/dev/null
"${clean_env[@]}" npm --prefix "$root/web" test --silent >/dev/null

echo "→ build"
"${clean_env[@]}" npm --prefix "$root/web" run build --silent >/dev/null
(cd "$root/api" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o "$out/kcal-bot" .)
COPYFILE_DISABLE=1 tar -C "$root/web/dist" -czf "$out/web.tgz" .

echo "→ upload to $HOST"
scp -q "$out/kcal-bot" "$HOST:$APP/kcal-bot.new"
scp -q "$out/web.tgz" "$HOST:/tmp/kcal-web.tgz"

echo "→ swap and restart"
ssh -o BatchMode=yes "$HOST" APP="$APP" DB="$DB" HEALTH="$HEALTH" bash -s <<'REMOTE'
set -euo pipefail
cd "$APP"
backup="${DB%.db}-$(date -u +%Y%m%dT%H%M).db"
sudo -u kcal ./kcal-bot -db "$DB" -backup "$backup" >/dev/null 2>&1
echo "  database backed up to $backup"

chmod 755 kcal-bot.new
rm -rf web.new
mkdir web.new
tar -C web.new -xzf /tmp/kcal-web.tgz 2>/dev/null
rm -f /tmp/kcal-web.tgz

cp -p kcal-bot kcal-bot.old
rm -rf web.old
mv web web.old
mv kcal-bot.new kcal-bot
mv web.new web
systemctl restart kcal-bot

for _ in 1 2 3 4 5 6 7 8 9 10; do
  if curl -fsS "$HEALTH" >/dev/null 2>&1; then
    echo "  healthy: $(journalctl -u kcal-bot -n 20 --no-pager | grep -o 'estimator=[^ ]*' | tail -1)"
    exit 0
  fi
  sleep 1
done

echo "  !! not healthy after restart — rolling back" >&2
journalctl -u kcal-bot -n 20 --no-pager >&2
mv kcal-bot kcal-bot.failed
mv kcal-bot.old kcal-bot
rm -rf web.failed
mv web web.failed
mv web.old web
systemctl restart kcal-bot
exit 1
REMOTE
echo "✓ deployed"
