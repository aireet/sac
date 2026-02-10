# SAC - Sandbox Agent Cluster

<p align="center">
  <a href="README.md">🇺🇸 English</a> •
  <a href="docs/i18n/README.zh.md">🇨🇳 中文</a>
</p>

SAC is an open-source platform that gives every user their own isolated [Claude Code](https://docs.anthropic.com/en/docs/claude-code) environment running in Kubernetes. It provides a web-based terminal with agent management, a skill marketplace, workspace file storage, and conversation history — all behind a clean Vue 3 dashboard.

## Why SAC?

Claude Code is a powerful CLI tool, but deploying it for teams is non-trivial. SAC solves this by:

- **Isolating each agent** in its own K8s StatefulSet with stable DNS — no noisy neighbors
- **Supporting multiple LLM providers** — Anthropic, OpenRouter, GLM (ZhiPu AI), Qwen, or any compatible API
- **Making skills sharable** — create reusable slash commands and share them across your org
- **Syncing conversation history** — hook-based capture stored in TimescaleDB with full export
- **Managing workspace files** — OSS-backed per-agent private storage plus shared public files

## Architecture

```
Browser ──HTTP──▶ Envoy Gateway ──▶ API Gateway (Go, :8080)
                                  ──▶ WS Proxy (Go, :8081)
                                  ──▶ Frontend (Vue 3, :80)
                                       │
WS Proxy ──WebSocket──▶ ttyd (:7681) in K8s Pod
                                       │
API Gateway ──K8s API──▶ StatefulSet per user/agent
            ──OSS SDK──▶ Alibaba Cloud OSS (workspace files)
            ──SQL─────▶ PostgreSQL + TimescaleDB
```

Each user-agent pair runs as a dedicated StatefulSet:

```
claude-code-{userID}-{agentID}-0
  └── ttyd → claude (CLI)
      ├── /workspace/private    ← synced from OSS (per-agent)
      ├── /workspace/public     ← synced from OSS (shared)
      └── /root/.claude/commands ← skill .md files
```

## Features

### Agent Management
- Create up to N agents per user (configurable), each with independent LLM configuration
- Built-in presets for OpenRouter, GLM, Qwen, and custom providers
- Per-agent resource limits (CPU/memory), configurable by admin
- One-click pod restart, real-time status monitoring

### Web Terminal
- Full PTY access via [xterm.js](https://xtermjs.org/) with WebGL rendering
- Two interaction modes: **terminal** (raw keystrokes) and **chat** (message-based input)
- Binary WebSocket proxy with ttyd protocol translation
- Auto-reconnect, resize support, Unicode/CJK wide-character rendering

### Skill Marketplace
- Create, fork, and share reusable slash commands
- Parameterized skills with dynamic form inputs (text, number, date, select)
- Skills sync to pods as `.md` files in `/root/.claude/commands/`
- One-click execution from the sidebar

### Workspace Files
- Per-agent private storage backed by Alibaba Cloud OSS
- Shared public workspace (admin-managed)
- Upload, download, create directories, delete
- In-browser preview: text (editable), images, binary info
- Quota enforcement (1GB / 1000 files per agent by default)
- Auto-sync to pod on session creation

### Conversation History
- Hook-based capture via `conversation-sync.mjs` running inside each pod
- Stored in TimescaleDB hypertable for efficient time-series queries
- Cursor-based pagination, session filtering, CSV export
- Admin can search and export across all users

### Admin Panel
- System-wide settings (agent limits, resource defaults)
- User management with role-based access (user/admin)
- Per-user setting overrides
- Agent lifecycle management (restart, delete, resource adjustment)
- Cross-user conversation search and export

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Frontend | Vue 3, TypeScript, Naive UI, xterm.js, Pinia, Vite |
| Backend | Go, Gin, Bun ORM, gorilla/websocket |
| Database | PostgreSQL 17 + TimescaleDB |
| Storage | Alibaba Cloud OSS (or S3-compatible) |
| Container | Kubernetes, StatefulSet per agent, ttyd |
| Ingress | Envoy Gateway v1.6 |
| Deploy | Helm 3, Docker multi-stage builds |

## Quick Start

### Prerequisites

- Kubernetes cluster with Gateway API CRDs
- PostgreSQL 17+ with TimescaleDB extension
- Alibaba Cloud OSS bucket (or S3-compatible storage)
- Docker registry access
- Helm 3

### 1. Build Images

```bash
make docker-build    # builds all 4 images (auto-bumps version)
make docker-push     # pushes to registry
```

This builds:
- `api-gateway` — REST API server
- `ws-proxy` — WebSocket terminal proxy
- `frontend` — Vue 3 SPA served by nginx
- `cc` — Claude Code container with ttyd

### 2. Configure

Edit `helm/sac/values.yaml`:

```yaml
global:
  registry: your-registry.example.com/sac

database:
  host: your-postgres-host
  port: 5432
  user: sandbox
  password: your-password
  name: sandbox

auth:
  jwtSecret: your-jwt-secret

envoyGateway:
  host: sac.your-domain.com
```

OSS settings are configured at runtime via the admin panel (System Settings).

### 3. Deploy

```bash
# First install
make helm-deploy

# Or upgrade existing release
make helm-upgrade
```

### 4. Initialize Database

```bash
# Run migrations
make migrate-up

# Seed admin user (admin / admin123)
make migrate-seed
```

### 5. Access

Open `http://sac.your-domain.com` in your browser. Log in with `admin` / `admin123`, then:

1. Configure OSS in Admin → System Settings
2. Create your first agent (configure LLM provider)
3. Start a session — a dedicated pod will be created
4. Use the terminal or chat mode to interact with Claude Code

## Local Development

SAC uses [Telepresence](https://www.telepresence.io/) to connect your local machine to the K8s cluster network, so local services can reach pod IPs directly.

```bash
# One command to start everything
make dev

# Or step by step:
make telepresence          # connect to K8s network
make build                 # compile Go binaries
make restart SVC=api       # restart API Gateway
make restart SVC=ws        # restart WS Proxy
make restart SVC=fe        # restart frontend dev server

# Utilities
make status                # show service status
make logs SVC=api          # tail API Gateway logs
make stop                  # stop all services
```

Services:
| Service | Port | Log |
|---------|------|-----|
| API Gateway | 8080 | `/tmp/sac-api-gateway.log` |
| WS Proxy | 8081 | `/tmp/sac-ws-proxy.log` |
| Frontend (Vite) | 5173 | `/tmp/sac-frontend.log` |

## Project Structure

```
sac/
├── backend/
│   ├── cmd/
│   │   ├── api-gateway/          # HTTP API server
│   │   ├── ws-proxy/             # WebSocket terminal proxy
│   │   └── migrate/              # Database migration CLI
│   ├── internal/
│   │   ├── admin/                # Admin panel handlers + settings
│   │   ├── agent/                # Agent CRUD + K8s lifecycle
│   │   ├── auth/                 # JWT auth + bcrypt passwords
│   │   ├── container/            # K8s StatefulSet management
│   │   ├── database/             # PostgreSQL connection (bun ORM)
│   │   ├── history/              # Conversation history (TimescaleDB)
│   │   ├── models/               # Data models
│   │   ├── session/              # Session lifecycle
│   │   ├── skill/                # Skill CRUD + pod sync
│   │   ├── storage/              # OSS client + provider
│   │   └── websocket/            # ttyd WebSocket proxy
│   ├── migrations/               # 12 database migrations
│   └── pkg/
│       ├── config/               # Environment-based configuration
│       └── response/             # Standardized HTTP responses
├── frontend/
│   └── src/
│       ├── components/
│       │   ├── Terminal/         # xterm.js WebGL terminal
│       │   ├── ChatInput/        # Chat-mode input bar
│       │   ├── Agent/            # Agent selector + creator
│       │   ├── SkillPanel/       # Agent dashboard sidebar
│       │   ├── SkillMarketplace/ # Skill browse/create/fork
│       │   └── Workspace/        # File browser with preview
│       ├── services/             # API client layer
│       ├── stores/               # Pinia auth store
│       ├── views/                # Login, Register, Main, Admin
│       └── utils/                # Error handling, file types
├── docker/
│   ├── api-gateway/              # Go multi-stage Dockerfile
│   ├── ws-proxy/                 # Go multi-stage Dockerfile
│   ├── frontend/                 # Vue build + nginx
│   └── claude-code/              # Ubuntu + ttyd + Claude Code CLI
├── helm/sac/                     # Helm chart
│   ├── templates/                # K8s manifests
│   ├── files/                    # Hook scripts + settings
│   └── charts/                   # Envoy Gateway subchart
├── Makefile                      # Dev, build, deploy commands
└── .version                      # Current version
```

## API Overview

<details>
<summary>Public endpoints</summary>

```
POST /api/auth/register
POST /api/auth/login
GET  /health
```
</details>

<details>
<summary>Protected endpoints (JWT required)</summary>

```
# Auth
GET  /api/auth/me

# Agents
GET    /api/agents
POST   /api/agents
GET    /api/agents/:id
PUT    /api/agents/:id
DELETE /api/agents/:id
POST   /api/agents/:id/restart
POST   /api/agents/:id/skills
DELETE /api/agents/:id/skills/:skillId
POST   /api/agents/:id/sync-skills
GET    /api/agent-statuses

# Sessions
POST   /api/sessions
GET    /api/sessions
GET    /api/sessions/:sessionId
DELETE /api/sessions/:sessionId

# Skills
GET    /api/skills
POST   /api/skills
GET    /api/skills/:id
PUT    /api/skills/:id
DELETE /api/skills/:id
POST   /api/skills/:id/fork
GET    /api/skills/public

# Conversations
GET    /api/conversations
GET    /api/conversations/sessions
GET    /api/conversations/export

# Workspace
GET    /api/workspace/status
POST   /api/workspace/upload
GET    /api/workspace/files
GET    /api/workspace/files/download
DELETE /api/workspace/files
POST   /api/workspace/directories
GET    /api/workspace/quota
GET    /api/workspace/public/files
GET    /api/workspace/public/files/download
POST   /api/workspace/public/upload
POST   /api/workspace/public/directories
DELETE /api/workspace/public/files

# WebSocket
WS     /ws/:sessionId?token=<jwt>&agent_id=<id>
```
</details>

<details>
<summary>Admin endpoints (admin role required)</summary>

```
GET    /api/admin/settings
PUT    /api/admin/settings/:key
GET    /api/admin/users
PUT    /api/admin/users/:id/role
GET    /api/admin/users/:id/settings
PUT    /api/admin/users/:id/settings/:key
DELETE /api/admin/users/:id/settings/:key
GET    /api/admin/users/:id/agents
DELETE /api/admin/users/:id/agents/:agentId
POST   /api/admin/users/:id/agents/:agentId/restart
PUT    /api/admin/users/:id/agents/:agentId/resources
GET    /api/admin/conversations
GET    /api/admin/conversations/export
```
</details>

## Configuration

All backend configuration is via environment variables (with `.env` file support):

| Variable | Default | Description |
|----------|---------|-------------|
| `API_GATEWAY_PORT` | `8080` | API server port |
| `WS_PROXY_PORT` | `8081` | WebSocket proxy port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `sandbox` | Database user |
| `DB_PASSWORD` | — | Database password |
| `DB_NAME` | `sandbox` | Database name |
| `JWT_SECRET` | — | Secret for JWT signing (HS256) |
| `KUBECONFIG_PATH` | — | Path to kubeconfig (auto-detects in-cluster) |
| `K8S_NAMESPACE` | `sac` | Kubernetes namespace |
| `DOCKER_REGISTRY` | — | Container image registry |
| `DOCKER_IMAGE` | — | Claude Code container image |

## License

MIT
