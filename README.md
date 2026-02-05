# Aura Webinar — Backend

Go API server, WebSocket hub, and WebRTC SFU (Pion).

## Run

```bash
cp env.example env
# Edit env (DB, Redis, JWT, etc.)
go run ./cmd/server
```

API: `http://localhost:8080`. WebSocket: `ws://localhost:8080/ws`.

Worker (optional): `go run ./cmd/worker`

## Docker

From repo root: `docker compose up --build` (builds this directory).

## Deploy (free tiers)

See [../docs/deploy-backend.md](../docs/deploy-backend.md) for Railway, Fly.io, Render, AWS, and Oracle Cloud.
