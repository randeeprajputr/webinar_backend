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
    sudo apt install -y ca-certificates curl lsb-release
    # Add Docker's official GPG key and repo (docker-compose-plugin is not in default Ubuntu repos)
    sudo install -m 0755 -d /etc/apt/keyrings
    sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
    sudo chmod a+r /etc/apt/keyrings/docker.asc
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
    sudo apt update
    sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
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
