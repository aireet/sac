# SAC - Sandbox Agent Cluster

## Project Overview

An open-source platform providing isolated Claude Code environments per user-agent in Kubernetes. Vue 3 + Go + PostgreSQL/TimescaleDB + K8s StatefulSets + Envoy Gateway.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.25, Gin, Bun ORM (pgdialect), gorilla/websocket, client-go v0.35 |
| Frontend | Vue 3, TypeScript, Naive UI, xterm.js (WebGL), Pinia, Vite |
| Database | PostgreSQL 17 + TimescaleDB |
| Storage | Alibaba Cloud OSS (or S3-compatible) |
| Infra | K8s StatefulSets, Envoy Gateway v1.6, Helm 3 |

## Project Structure

```
sac/
├── backend/
│   ├── cmd/{api-gateway, ws-proxy, migrate}/
│   ├── internal/
│   │   ├── admin/       # Admin handlers + settings service
│   │   ├── agent/       # Agent CRUD + K8s StatefulSet lifecycle
│   │   ├── auth/        # JWT (HS256, 24h) + bcrypt + middleware
│   │   ├── container/   # K8s client: StatefulSet, Service, exec, file sync
│   │   ├── database/    # bun ORM + pgdialect connection
│   │   ├── history/     # Conversation history (TimescaleDB hypertable)
│   │   ├── models/      # All data models (User, Agent, Session, Skill, etc.)
│   │   ├── session/     # Session lifecycle (create → pod ready → sync → connect)
│   │   ├── skill/       # Skill CRUD + sync as .md to pod /root/.claude/commands/
│   │   ├── storage/     # OSS client wrapper + lazy provider from system_settings
│   │   ├── websocket/   # ttyd binary protocol proxy (0x30 input/output)
│   │   └── workspace/   # File upload/download/sync, quota enforcement
│   ├── migrations/      # 000001-000012 (bun/migrate)
│   └── pkg/{config, response}/
├── frontend/src/
│   ├── components/
│   │   ├── Terminal/          # xterm.js WebGL + binary WS + resize
│   │   ├── ChatInput/         # Chat-mode message bar
│   │   ├── Agent/             # AgentSelector + AgentCreator (LLM presets)
│   │   ├── SkillPanel/        # Dashboard sidebar (status, skills, workspace, history)
│   │   ├── SkillMarketplace/  # Browse, create, fork, install skills
│   │   └── Workspace/         # File browser + text/image/binary preview
│   ├── services/              # Typed API clients (axios + interceptors)
│   ├── stores/auth.ts         # Pinia auth store
│   └── views/                 # MainView, LoginView, RegisterView, AdminView
├── docker/                    # 4 Dockerfiles (api-gateway, ws-proxy, frontend, claude-code)
├── helm/sac/                  # Helm chart + Envoy Gateway subchart
│   ├── files/                 # conversation-sync.mjs hook, settings.json
│   └── templates/             # Deployments, Services, RBAC, HTTPRoutes, ConfigMap
├── Makefile                   # Dev, build, deploy, migrate
└── .version
```

## Architecture Key Points

### Per-Agent StatefulSet
- Each user-agent pair → 1 StatefulSet + 1 headless Service
- Naming: `claude-code-{userID}-{agentID}`
- Pod DNS: `claude-code-{userID}-{agentID}-0.claude-code-{userID}-{agentID}.sac.svc.cluster.local`
- Mounted volumes: workspace (emptyDir), settings.json + conversation-sync.mjs (ConfigMap)

### Session Flow
1. CreateSession → check/create StatefulSet → wait pod ready (300s timeout)
2. Sync workspace files from OSS (private + public + claude-commands)
3. Sync installed skills as .md files
4. Return session ID + Pod IP → frontend connects WS proxy → ttyd

### WebSocket Protocol
- Browser ↔ WS Proxy (`:8081`) ↔ ttyd (`:7681` in pod)
- Binary protocol: `0x30` = I/O data, `0x31` = resize, `0x7B` = JSON auth
- JWT auth via query param

### Conversation Sync
- `conversation-sync.mjs` hook runs on Stop/SubagentStop/UserPromptSubmit events inside pods
- Reads JSONL transcript, POSTs to `POST /api/internal/conversations/events` (cluster-internal)
- Stored in `conversation_histories` TimescaleDB hypertable

### Resource Configuration Hierarchy
1. System defaults (system_settings table)
2. Per-user overrides (user_settings table)
3. Per-agent overrides (agents table columns)

## Development

### Rules
1. **Use Telepresence** to connect to K8s cluster — never port-forward
2. **Bind to 0.0.0.0** — remote dev environment

### Commands
```bash
make dev              # Telepresence + build + start all (8080, 8081, 5173)
make stop             # Kill all dev services
make status           # Show running services
make restart SVC=api  # Rebuild + restart one service (api|ws|fe)
make logs SVC=api     # Tail service log
make migrate-up       # Run DB migrations
make migrate-seed     # Seed admin user (admin/admin123)
```

### Build & Deploy
```bash
make docker-build     # Build all 4 images (auto version bump)
make docker-push      # Push to registry
make helm-upgrade     # Helm upgrade release
```

## Coding Conventions

### Go Backend
- ORM: `bun` with `pgdialect.New()` — NOT `pgdriver.New()` for dialect
- No vendor directory; use Go module cache
- Standardized responses via `pkg/response` (OK, BadRequest, NotFound, etc.)
- Config from env vars with `.env` support (godotenv)
- Routes: public (no auth) → internal (pod-to-API) → protected (JWT) → admin (JWT + role)

### Frontend
- Vue 3 Composition API with `<script setup lang="ts">`
- Naive UI components, dark theme throughout
- API layer: typed services in `services/`, axios instance with JWT interceptor + 401 redirect
- State: Pinia store for auth, component-level refs for everything else
- Terminal: raw binary WebSocket (ArrayBuffer), not the WebSocketManager class

## Database Migrations

| # | Name | Purpose |
|---|------|---------|
| 001 | init_schema | users, sessions, skills, conversation_logs |
| 002 | add_agents | agents + agent_skills junction |
| 003 | remove_agent_system_prompt_model | Move to JSONB config |
| 004 | add_session_agent_id | Bind sessions to agents |
| 005 | add_skill_command_name | Kebab-case command names |
| 006 | add_auth_and_settings | JWT auth, password, roles, system/user settings |
| 007 | add_agent_resources | Per-agent CPU/memory limits |
| 008 | enable_timescaledb | TimescaleDB extension |
| 009 | conversation_history | Hypertable for conversation events |
| 010 | workspace_files | File metadata + quotas |
| 011 | seed_oss_settings | OSS config in system_settings |
| 012 | workspace_per_agent | Add agent_id to workspace tables |

## Troubleshooting

| Issue | Fix |
|-------|-----|
| bun dialect error | `go get github.com/uptrace/bun/dialect/pgdialect@v1.2.16` |
| vendor inconsistency | `rm -rf vendor && go mod tidy` |
| npm modules missing | `rm -rf node_modules && npm install` |
| Pod won't start | Check agent config (API token/URL), `make logs SVC=api` for errors |
| WS connection fails | Verify Telepresence connected, pod is Running, check WS proxy logs |
