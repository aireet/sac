# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SAC (Sandbox Agent Cluster) — an open-source platform that makes Claude Code accessible to everyone via web browser. Each user's agent runs in an isolated K8s StatefulSet. Users can share/install skills and collaboratively build a knowledge base.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.25, Gin, Bun ORM (pgdialect), gorilla/websocket, go-redis/v9, client-go v0.35 |
| Frontend | Vue 3, TypeScript, Naive UI, xterm.js (WebGL), Pinia, Vite 7 |
| Database | PostgreSQL 17 + TimescaleDB |
| Cache/PubSub | Redis (standalone, via bitnami Helm subchart) |
| Storage | S3-compatible (Alibaba Cloud OSS, MinIO, AWS S3) |
| Infra | K8s StatefulSets, Envoy Gateway v1.6, Helm 3 |

## Development Commands

```bash
# Full dev environment (Telepresence + build + start all on 8080, 8081, 5173)
make dev

# Individual service management
make restart SVC=api    # Rebuild + restart api-gateway (also: ws, fe)
make logs SVC=api       # Tail service log (also: ws, fe)
make stop               # Kill all dev services
make status             # Show running services

# Build Go binaries only
make build              # Builds api-gateway + ws-proxy to backend/bin/
make build-api          # cd backend && go build -o bin/api-gateway ./cmd/api-gateway
make build-ws           # cd backend && go build -o bin/ws-proxy ./cmd/ws-proxy

# Frontend (from frontend/ directory)
npm run dev             # Vite dev server (proxies /api→:8080, /ws→:8081)
npm run build           # vue-tsc -b && vite build

# Database
make migrate-up         # Run all pending migrations
make migrate-seed       # Seed admin user (admin/admin123)

# Docker & Deploy
make docker-build       # Build all 5 images (auto version bump from .version)
make docker-push        # Push all images to registry
make helm-dep-update    # Update Helm chart dependencies
make helm-upgrade       # Helm upgrade release
```

## Development Rules

1. **Use Telepresence** to connect to K8s cluster — never port-forward
2. **Bind to 0.0.0.0** — remote dev environment
3. Go module path: `g.echo.tech/dev/sac`
4. No vendor directory; use Go module cache
5. No tests exist yet — manual testing via Telepresence against live cluster

## Architecture

### Four Binaries (backend/cmd/)

| Binary | Port | Purpose |
|--------|------|---------|
| `api-gateway` | 8080 | Main REST API — all CRUD, file sync, SSE output watch, Redis pub/sub |
| `ws-proxy` | 8081 | WebSocket proxy: browser ↔ ttyd in pod (binary protocol) |
| `migrate` | — | DB migrations (up/down/status/seed) |
| `output-watcher` | — | Sidecar in pods: watches `/workspace/output/` via fsnotify, POSTs to API |

### Route Layers (api-gateway/main.go)

Routes are registered in this order in `main.go`:
1. **Internal** (`/api/internal/*`) — no JWT, pod-to-API calls only
   - `POST /api/internal/conversations/events` — conversation sync from pods
   - `POST /api/internal/output/upload` and `/delete` — sidecar file events
2. **Public** — no auth: `/auth/login`, `/auth/register`, `/health`, SSE output watch
3. **Protected** (`/api/*` + JWT) — agents, sessions, skills, workspace, groups, history
4. **Admin** (`/api/admin/*` + JWT + role=admin) — settings, user management

### Per-Agent StatefulSet

- Each user-agent pair → 1 StatefulSet + 1 headless Service
- Naming: `claude-code-{userID}-{agentID}`
- Pod DNS: `claude-code-{userID}-{agentID}-0.claude-code-{userID}-{agentID}.sac.svc.cluster.local`
- Two containers: main (claude-code + ttyd on :7681) + sidecar (output-watcher)
- Volumes: workspace (emptyDir), settings.json + conversation-sync.mjs (ConfigMap)
- Pod entrypoint uses `dtach` for session persistence + auto-restart loop for claude CLI

### Session Lifecycle

```
Frontend: createSession(agentId)
  → Backend: check/create StatefulSet → wait pod ready (300s timeout)
  → Sync workspace files from S3 (private + public + claude-commands)
  → Sync installed skills as .md files to /root/.claude/commands/
  → Return session ID + Pod IP
Frontend: waitForSessionReady(sessionId) [poll 60x, 2s intervals]
  → connectWebSocket → xterm.js terminal
```

### WebSocket Protocol (ws-proxy ↔ ttyd)

- Binary protocol: `0x30` = I/O data, `0x31` = resize, `0x7B` = JSON auth
- Frontend uses raw `ArrayBuffer` WebSocket, NOT the `WebSocketManager` class in `services/websocket.ts`
- JWT auth via query param

### Output Workspace Pipeline

```
Pod sidecar (fsnotify /workspace/output/)
  → POST /api/internal/output/upload or /delete
  → API Gateway uploads to S3, upserts DB
  → PUBLISH to Redis channel sac:output:{userID}:{agentID}
  → All API Gateway replicas PSUBSCRIBE sac:output:*
  → Dispatch to local SSE subscribers
  → Frontend GET /api/workspace/output/watch?agent_id=X (SSE)
```

Graceful degradation: if Redis unavailable, SSE returns 503, frontend falls back to manual refresh.

### Conversation Sync

- `conversation-sync.mjs` hook (zero external deps, Node.js 22 built-ins only)
- Triggers: Stop, SubagentStop, UserPromptSubmit events inside pods
- Incremental sync via `/tmp/.last_sync_line_{sessionId}` tracking
- POSTs to `POST /api/internal/conversations/events` (cluster-internal)
- Stored in `conversation_histories` TimescaleDB hypertable

### Resource Configuration Hierarchy

1. System defaults (`system_settings` table)
2. Per-user overrides (`user_settings` table)
3. Per-agent overrides (`agents` table columns)

### Storage Abstraction (internal/storage/)

- Interface: `StorageBackend` with Upload/Download/Delete/List/Copy/PresignedURL
- Backends: `TypeOSS` (Alibaba), `TypeS3` (AWS), `TypeS3Compat` (MinIO, RustFS, R2)
- Lazy init from `system_settings` table, cached with config fingerprint hash
- Returns `nil` if not configured (graceful degradation)

## Coding Conventions

### Go Backend

- ORM: `bun` with `pgdialect.New()` — NOT `pgdriver.New()` for dialect
- Standardized responses via `pkg/response` (OK, BadRequest, NotFound, etc.)
- Config from env vars with `.env` support (godotenv) — see `pkg/config/config.go`
- Handler pattern: constructor injection (`NewHandler(db, containerMgr, storage, ...)`)
- Each handler registers its own routes via `RegisterRoutes(group, authMiddleware)`

### Frontend

- Vue 3 Composition API with `<script setup lang="ts">`
- Naive UI components, dark theme throughout
- API layer: typed services in `services/`, axios instance with JWT interceptor + 401 redirect
- State: Pinia store for auth only, component-level `ref()` for everything else
- API base URL auto-detects: localhost → separate ports, production → same-origin via Envoy
- Vite dev proxy: `/api` → `:8080`, `/ws` → `:8081`, `/api/workspace/output/watch` → WS `:8080`
- Strict TypeScript: `noUnusedLocals`, `noUnusedParameters`, `erasableSyntaxOnly`

### Key Environment Variables (backend)

| Variable | Default | Purpose |
|----------|---------|---------|
| `API_GATEWAY_PORT` | 8080 | API server port |
| `WS_PROXY_PORT` | 8081 | WebSocket proxy port |
| `DB_HOST/PORT/USER/PASSWORD/NAME` | — | PostgreSQL connection |
| `JWT_SECRET` | — | HS256 signing key (24h expiry) |
| `K8S_NAMESPACE` | sac | Kubernetes namespace |
| `KUBECONFIG_PATH` | ../kubeconfig.yaml | Local kubeconfig (in-cluster auto-detected) |
| `DOCKER_REGISTRY` | — | Container image registry |
| `DOCKER_IMAGE` | — | Claude Code pod image |
| `SIDECAR_IMAGE` | — | Output watcher sidecar image |
| `REDIS_URL` | — | Redis connection (optional, graceful degradation) |

## Database Migrations

18 migrations in `backend/migrations/` (000001–000018), run via `make migrate-up`.
Latest: `000018_add_agent_instructions` — adds instructions field to agents table.

## Troubleshooting

| Issue | Fix |
|-------|-----|
| bun dialect error | `go get github.com/uptrace/bun/dialect/pgdialect@v1.2.16` |
| vendor inconsistency | `rm -rf vendor && go mod tidy` |
| npm modules missing | `cd frontend && rm -rf node_modules && npm install` |
| Pod won't start | Check agent config (API token/URL), `make logs SVC=api` for errors |
| WS connection fails | Verify Telepresence connected, pod is Running, check WS proxy logs |
