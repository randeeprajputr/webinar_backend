#!/usr/bin/env bash
# Deploy backend to EC2 using editorial-go.pem.
# Run from project root: ./backend/scripts/deploy-ec2.sh <EC2_IP> [ubuntu|ec2-user]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROJECT_ROOT="$(cd "$BACKEND_DIR/.." && pwd)"
PEM="$BACKEND_DIR/editorial-go.pem"

if [ -z "$1" ]; then
  echo "Usage: $0 <EC2_HOST_OR_IP> [ubuntu|ec2-user]"
  echo "Example: $0 54.123.45.67 ubuntu"
  exit 1
fi

EC2_HOST="$1"
REMOTE_USER="${2:-ubuntu}"

if [ ! -f "$PEM" ]; then
  echo "PEM key not found: $PEM"
  exit 1
fi

chmod -f 400 "$PEM" 2>/dev/null || true

echo "Deploying to $REMOTE_USER@$EC2_HOST (project root: $PROJECT_ROOT)"
echo "Syncing project..."
rsync -avz --delete \
  --exclude 'node_modules' \
  --exclude '.git' \
  --exclude 'frontend/.next' \
  --exclude 'frontend/out' \
  --exclude '.env*.local' \
  -e "ssh -i $PEM -o StrictHostKeyChecking=accept-new" \
  "$PROJECT_ROOT/" "$REMOTE_USER@$EC2_HOST:~/aura_webinar/"

echo "Building and starting on EC2 (app + Postgres + Redis)..."
ssh -i "$PEM" "$REMOTE_USER@$EC2_HOST" \
  'cd ~/aura_webinar/backend && docker compose build app && docker compose up -d'

echo "Done. Backend at http://$EC2_HOST:8080 (Postgres and Redis run on EC2 via Docker)."
