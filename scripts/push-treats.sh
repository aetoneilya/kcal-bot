#!/usr/bin/env bash
# Copy the local treats/ directory (compliments.txt, cats/*.mp4) to the
# server, replacing what is there. It is not in git; the server reads it on
# every request, so no restart is needed.
#
#   scripts/push-treats.sh
#
# The host comes from HOST, else from DEPLOY_HOST in the local .env.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
HOST="${HOST:-$(sed -n 's/^DEPLOY_HOST=//p' "$root/.env" 2>/dev/null)}"
: "${HOST:?set HOST or DEPLOY_HOST in .env, e.g. root@your-vps}"
DIR=/var/lib/kcal-bot/treats

[ -d "$root/treats" ] || { echo "no treats/ directory" >&2; exit 1; }

echo "→ treats to $HOST:$DIR ($(find "$root/treats/cats" -type f 2>/dev/null | wc -l | tr -d ' ') cats)"
COPYFILE_DISABLE=1 tar -C "$root/treats" --no-xattrs --no-mac-metadata --exclude .DS_Store -czf - . |
  ssh -o BatchMode=yes "$HOST" DIR="$DIR" bash -c "'
    set -e
    rm -rf \"\$DIR.new\" && mkdir -p \"\$DIR.new\"
    tar -C \"\$DIR.new\" -xzf -
    chown -R kcal:kcal \"\$DIR.new\" && chmod -R u=rwX,go= \"\$DIR.new\"
    rm -rf \"\$DIR\" && mv \"\$DIR.new\" \"\$DIR\"
  '"
echo "✓ done"
