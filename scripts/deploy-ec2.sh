#!/usr/bin/env bash
# Deploy backend to EC2 using editorial-go.pem.
# Syncs backend to ~/webinar_backend on the server and runs docker compose there.
# Run from project root: ./backend/scripts/deploy-ec2.sh <EC2_IP> [ubuntu|ec2-user]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PEM="$BACKEND_DIR/editorial-go.pem"
REMOTE_DIR="webinar_backend"

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

echo "Deploying backend to $REMOTE_USER@$EC2_HOST:~/$REMOTE_DIR/"
echo "Syncing backend..."
rsync -avz --delete \
  --exclude '.git' \
  -e "ssh -i $PEM -o StrictHostKeyChecking=accept-new" \
  "$BACKEND_DIR/" "$REMOTE_USER@$EC2_HOST:~/$REMOTE_DIR/"

echo "Building and starting on EC2..."
ssh -i "$PEM" "$REMOTE_USER@$EC2_HOST" \
  "cd ~/$REMOTE_DIR && docker compose build app && docker compose up -d"

echo "Done. Backend should be at http://$EC2_HOST:8081 (or via nginx)"
