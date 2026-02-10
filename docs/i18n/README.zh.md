# SAC - Sandbox Agent Cluster

<p align="center">
  <a href="../../README.md">🇺🇸 English</a> •
  <a href="README.zh.md">🇨🇳 中文</a>
</p>

SAC 是一个开源平台，让 [Claude Code](https://docs.anthropic.com/en/docs/claude-code) 触手可及 —— 不仅仅面向开发者。它为每个用户在 Kubernetes 中运行独立的 AI Agent 环境，只需一个浏览器即可使用。在组织内部，团队成员可以高效分享和安装精心打造的技能，共同构建一个能解决各种实际问题的知识库。

## 为什么选择 SAC？

Claude Code 是一个革命性的 AI Agent，它几乎能做一切，但使用它需要终端操作经验和本地环境搭建。SAC 彻底消除了这些门槛：

- **零门槛使用** — 组织内任何人都可以通过浏览器使用 Claude Code，无需命令行经验
- **技能共享** — 将你精妙的想法创建为可复用的斜杠命令，一键分享给团队
- **协作知识库** — 构建共享的提示词、模板和参考资料工作区，让每个 Agent 都更智能
- **多供应商灵活切换** — Anthropic、OpenRouter、GLM（智谱 AI）、通义千问、Nebula，或任何兼容的 API
- **安全隔离** — 每个 Agent 运行在独立的 K8s StatefulSet 中，资源独享，互不干扰

## 架构

```
浏览器 ──HTTP──▶ Envoy Gateway ──▶ API Gateway (Go, :8080)
                                 ──▶ WS Proxy (Go, :8081)
                                 ──▶ Frontend (Vue 3, :80)
                                      │
WS Proxy ──WebSocket──▶ ttyd (:7681) K8s Pod 内
                                      │
API Gateway ──K8s API──▶ 每个用户/Agent 一个 StatefulSet
            ──OSS SDK──▶ 阿里云 OSS（工作区文件）
            ──SQL─────▶ PostgreSQL + TimescaleDB
```

每个用户-Agent 组合运行为一个独立的 StatefulSet：

```
claude-code-{userID}-{agentID}-0
  └── ttyd → claude (CLI)
      ├── /workspace/private    ← 从 OSS 同步（Agent 级私有）
      ├── /workspace/public     ← 从 OSS 同步（共享）
      └── /root/.claude/commands ← 技能 .md 文件
```

## 功能特性

### Agent 管理
- 每个用户最多创建 N 个 Agent（可配置），各自拥有独立的 LLM 配置
- 内置 OpenRouter、GLM、通义千问和自定义提供商预设
- Agent 级资源限制（CPU/内存），管理员可配置
- 一键重启 Pod，实时状态监控

### Web 终端
- 通过 [xterm.js](https://xtermjs.org/) 实现完整的 PTY 访问，支持 WebGL 渲染
- 两种交互模式：**终端**（原始按键）和 **聊天**（消息输入）
- 二进制 WebSocket 代理，支持 ttyd 协议转换
- 自动重连、窗口调整、Unicode/CJK 宽字符渲染

### 技能市场
- 创建、Fork 和共享可复用的斜杠命令
- 支持参数化技能，动态表单输入（文本、数字、日期、下拉选择）
- 技能以 `.md` 文件同步到 Pod 的 `/root/.claude/commands/`
- 侧边栏一键执行

### 工作区文件
- 基于阿里云 OSS 的 Agent 级私有存储
- 共享公共工作区（管理员管理）
- 上传、下载、创建目录、删除
- 浏览器内预览：文本（可编辑）、图片、二进制信息
- 配额限制（默认每 Agent 1GB / 1000 个文件）
- 创建会话时自动同步到 Pod

### 对话历史
- 通过每个 Pod 内运行的 `conversation-sync.mjs` Hook 采集
- 存储在 TimescaleDB hypertable 中，高效时序查询
- 游标分页、会话过滤、CSV 导出
- 管理员可跨用户搜索和导出

### 管理面板
- 全局系统设置（Agent 限制、资源默认值）
- 用户管理，基于角色的访问控制（user/admin）
- 用户级设置覆盖
- Agent 生命周期管理（重启、删除、资源调整）
- 跨用户对话搜索和导出

## 技术栈

| 层级 | 技术 |
|------|------|
| 前端 | Vue 3, TypeScript, Naive UI, xterm.js, Pinia, Vite |
| 后端 | Go, Gin, Bun ORM, gorilla/websocket |
| 数据库 | PostgreSQL 17 + TimescaleDB |
| 存储 | 阿里云 OSS（或 S3 兼容存储） |
| 容器 | Kubernetes, 每 Agent 一个 StatefulSet, ttyd |
| 入口网关 | 任意 Ingress 控制器（可选内置 Envoy Gateway 子 Chart） |
| 部署 | Helm 3, Docker 多阶段构建 |

## 快速开始

### 前置要求

- Kubernetes 集群
- PostgreSQL 17+ 并启用 TimescaleDB 扩展
- 阿里云 OSS 存储桶（或 S3 兼容存储）
- Docker 镜像仓库访问权限
- Helm 3
- 任意 Ingress 控制器，配置以下路由即可：
  - `/api/*` → `api-gateway:8080`
  - `/ws/*` → `ws-proxy:8081`（WebSocket）
  - `/*` → `frontend:80`
  - Helm Chart 包含可选的 [Envoy Gateway](https://gateway.envoyproxy.io/) 子 Chart（`envoyGateway.enabled: true`），也可自行使用 Nginx / Traefik / Istio 等

### 1. 构建镜像

```bash
make docker-build    # 构建全部 4 个镜像（自动递增版本号）
make docker-push     # 推送到镜像仓库
```

构建的镜像包括：
- `api-gateway` — REST API 服务
- `ws-proxy` — WebSocket 终端代理
- `frontend` — Vue 3 SPA（nginx 托管）
- `cc` — Claude Code 容器（含 ttyd）

### 2. 配置

编辑 `helm/sac/values.yaml`：

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

OSS 设置在运行时通过管理面板（系统设置）配置。

### 3. 部署

```bash
# 首次安装
make helm-deploy

# 升级已有版本
make helm-upgrade
```

### 4. 初始化数据库

```bash
# 执行数据库迁移
make migrate-up

# 初始化管理员账号 (admin / admin123)
make migrate-seed
```

### 5. 访问

在浏览器中打开 `http://sac.your-domain.com`，使用 `admin` / `admin123` 登录，然后：

1. 在管理面板 → 系统设置中配置 OSS
2. 创建你的第一个 Agent（配置 LLM 提供商）
3. 启动一个会话 — 系统将创建一个专属 Pod
4. 使用终端或聊天模式与 Claude Code 交互

## 本地开发

SAC 使用 [Telepresence](https://www.telepresence.io/) 将本地机器连接到 K8s 集群网络，使本地服务可以直接访问 Pod IP。

```bash
# 一键启动所有服务
make dev

# 或分步操作：
make telepresence          # 连接 K8s 集群网络
make build                 # 编译 Go 二进制
make restart SVC=api       # 重启 API Gateway
make restart SVC=ws        # 重启 WS Proxy
make restart SVC=fe        # 重启前端开发服务器

# 实用工具
make status                # 查看服务状态
make logs SVC=api          # 查看 API Gateway 日志
make stop                  # 停止所有服务
```

各服务端口：
| 服务 | 端口 | 日志 |
|------|------|------|
| API Gateway | 8080 | `/tmp/sac-api-gateway.log` |
| WS Proxy | 8081 | `/tmp/sac-ws-proxy.log` |
| Frontend (Vite) | 5173 | `/tmp/sac-frontend.log` |

## 项目结构

```
sac/
├── backend/
│   ├── cmd/
│   │   ├── api-gateway/          # HTTP API 服务
│   │   ├── ws-proxy/             # WebSocket 终端代理
│   │   └── migrate/              # 数据库迁移工具
│   ├── internal/
│   │   ├── admin/                # 管理面板处理器 + 设置
│   │   ├── agent/                # Agent CRUD + K8s 生命周期
│   │   ├── auth/                 # JWT 认证 + bcrypt 密码
│   │   ├── container/            # K8s StatefulSet 管理
│   │   ├── database/             # PostgreSQL 连接 (bun ORM)
│   │   ├── history/              # 对话历史 (TimescaleDB)
│   │   ├── models/               # 数据模型
│   │   ├── session/              # 会话生命周期
│   │   ├── skill/                # 技能 CRUD + Pod 同步
│   │   ├── storage/              # OSS 客户端 + 提供者
│   │   └── websocket/            # ttyd WebSocket 代理
│   ├── migrations/               # 12 个数据库迁移
│   └── pkg/
│       ├── config/               # 基于环境变量的配置
│       └── response/             # 标准化 HTTP 响应
├── frontend/
│   └── src/
│       ├── components/
│       │   ├── Terminal/         # xterm.js WebGL 终端
│       │   ├── ChatInput/        # 聊天模式输入栏
│       │   ├── Agent/            # Agent 选择器 + 创建器
│       │   ├── SkillPanel/       # Agent 仪表板侧边栏
│       │   ├── SkillMarketplace/ # 技能浏览/创建/Fork
│       │   └── Workspace/        # 文件浏览器 + 预览
│       ├── services/             # API 客户端层
│       ├── stores/               # Pinia 认证 Store
│       ├── views/                # 登录、注册、主界面、管理面板
│       └── utils/                # 错误处理、文件类型
├── docker/
│   ├── api-gateway/              # Go 多阶段 Dockerfile
│   ├── ws-proxy/                 # Go 多阶段 Dockerfile
│   ├── frontend/                 # Vue 构建 + nginx
│   └── claude-code/              # Ubuntu + ttyd + Claude Code CLI
├── helm/sac/                     # Helm Chart
│   ├── templates/                # K8s 资源清单
│   ├── files/                    # Hook 脚本 + 设置文件
│   └── charts/                   # Envoy Gateway 子 Chart
├── Makefile                      # 开发、构建、部署命令
└── .version                      # 当前版本号
```

## API 概览

<details>
<summary>公开接口</summary>

```
POST /api/auth/register
POST /api/auth/login
GET  /health
```
</details>

<details>
<summary>需认证接口（JWT）</summary>

```
# 认证
GET  /api/auth/me

# Agent
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

# 会话
POST   /api/sessions
GET    /api/sessions
GET    /api/sessions/:sessionId
DELETE /api/sessions/:sessionId

# 技能
GET    /api/skills
POST   /api/skills
GET    /api/skills/:id
PUT    /api/skills/:id
DELETE /api/skills/:id
POST   /api/skills/:id/fork
GET    /api/skills/public

# 对话历史
GET    /api/conversations
GET    /api/conversations/sessions
GET    /api/conversations/export

# 工作区
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
<summary>管理员接口（需 admin 角色）</summary>

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

## 配置项

所有后端配置通过环境变量设置（支持 `.env` 文件）：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `API_GATEWAY_PORT` | `8080` | API 服务端口 |
| `WS_PROXY_PORT` | `8081` | WebSocket 代理端口 |
| `DB_HOST` | `localhost` | PostgreSQL 地址 |
| `DB_PORT` | `5432` | PostgreSQL 端口 |
| `DB_USER` | `sandbox` | 数据库用户名 |
| `DB_PASSWORD` | — | 数据库密码 |
| `DB_NAME` | `sandbox` | 数据库名称 |
| `JWT_SECRET` | — | JWT 签名密钥 (HS256) |
| `KUBECONFIG_PATH` | — | kubeconfig 路径（集群内自动检测） |
| `K8S_NAMESPACE` | `sac` | Kubernetes 命名空间 |
| `DOCKER_REGISTRY` | — | 容器镜像仓库 |
| `DOCKER_IMAGE` | — | Claude Code 容器镜像 |

## 开源协议

MIT
