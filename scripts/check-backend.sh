#!/usr/bin/env bash
# Check if Go backend is deployed and running on the server.
# Usage: ./backend/scripts/check-backend.sh <EC2_HOST_OR_IP> [ubuntu|ec2-user]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PEM="$BACKEND_DIR/editorial-go.pem"

if [ -z "$1" ]; then
  echo "Usage: $0 <EC2_HOST_OR_IP> [ubuntu|ec2-user]"
  echo "Example: $0 webinar-backend.worldcue.news ubuntu"
  exit 1
fi

EC2_HOST="$1"
REMOTE_USER="${2:-ubuntu}"

if [ ! -f "$PEM" ]; then
  echo "PEM key not found: $PEM"
  exit 1
fi

chmod -f 400 "$PEM" 2>/dev/null || true

echo "Checking backend on $REMOTE_USER@$EC2_HOST ..."
echo ""

ssh -i "$PEM" -o StrictHostKeyChecking=accept-new "$REMOTE_USER@$EC2_HOST" bash -s << 'REMOTE'
  echo "=== Docker Compose services ==="
  if command -v docker >/dev/null 2>&1; then
    (cd ~/webinar_backend 2>/dev/null && docker compose ps) || (cd ~/aura_webinar 2>/dev/null && docker compose ps) || (cd ~/aura_webinar/backend 2>/dev/null && docker compose ps) || echo "No compose found in webinar_backend or aura_webinar."
  else
    echo "Docker not installed."
  fi

  echo ""
  echo "=== Listening on 8080 / 8081 ==="
  (ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null) | grep -E ':(8080|8081)\s' || true

  echo ""
  echo "=== Backend health (localhost) ==="
  for port in 8081 8080; do
    out=$(curl -sf -w "\n%{http_code}" --connect-timeout 2 "http://127.0.0.1:$port/health" 2>/dev/null) || true
    if [ -n "$out" ]; then
      echo "  Port $port: $out"
    fi
  done
REMOTE

echo ""
echo "=== Public URL (if nginx/DNS points here) ==="
echo "  https://webinar-backend.worldcue.news (or http://$EC2_HOST)"
