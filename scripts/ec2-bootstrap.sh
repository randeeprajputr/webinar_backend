#!/usr/bin/env bash
# One-time EC2 server setup: install Docker and Docker Compose.
# Copy to server and run, or run via: ssh -i backend/editorial-go.pem ubuntu@<IP> 'bash -s' < backend/scripts/ec2-bootstrap.sh
# Supports: Ubuntu 22.04, Amazon Linux 2

set -e

if [ -f /etc/os-release ]; then
  . /etc/os-release
  OS="$ID"
else
  echo "Cannot detect OS"
  exit 1
fi

case "$OS" in
  ubuntu)
    sudo apt update && sudo apt upgrade -y
    sudo apt install -y docker.io docker-compose-plugin
    sudo systemctl enable docker && sudo systemctl start docker
    sudo usermod -aG docker "$USER"
    echo "Docker installed. Log out and back in (or run 'newgrp docker') so docker works without sudo."
    ;;
  amzn)
    sudo yum update -y
    sudo yum install -y docker
    sudo systemctl enable docker && sudo systemctl start docker
    sudo usermod -aG docker "$USER"
    # Docker Compose standalone
    sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
    sudo chmod +x /usr/local/bin/docker-compose
    echo "Docker and docker-compose installed. Log out and back in (or run 'newgrp docker') so docker works without sudo."
    ;;
  *)
    echo "Unsupported OS: $OS. Install Docker manually."
    exit 1
    ;;
esac
