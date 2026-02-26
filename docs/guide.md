# SAC 使用指南

本文档面向 SAC 平台的管理员和终端用户，涵盖从部署到日常使用的完整流程。README 是项目简介，本文档是操作手册。

---

## 目录

- [快速体验（kind 一键部署）](#快速体验kind-一键部署)
- [生产部署](#生产部署)
- [初始配置](#初始配置)
- [用户指南](#用户指南)
  - [注册与登录](#注册与登录)
  - [Agent 管理](#agent-管理)
  - [终端与聊天](#终端与聊天)
  - [工作区文件](#工作区文件)
  - [技能市场](#技能市场)
  - [对话历史](#对话历史)
  - [文件分享](#文件分享)
- [管理员指南](#管理员指南)
  - [系统设置](#系统设置)
  - [存储配置](#存储配置)
  - [用户管理](#用户管理)
  - [资源管控](#资源管控)
  - [镜像升级](#镜像升级)
  - [维护任务](#维护任务)
- [概念说明](#概念说明)
  - [Agent 与 Pod 的关系](#agent-与-pod-的关系)
  - [会话生命周期](#会话生命周期)
  - [技能可见性模型](#技能可见性模型)
  - [工作区类型](#工作区类型)
  - [Output 实时推送](#output-实时推送)
- [常见问题](#常见问题)

---

## 快速体验（kind 一键部署）

适合本地试用，不需要云上 K8s 集群。前置条件：Docker、kind、kubectl、Helm 3。

```bash
# 一键启动（构建镜像 + 创建集群 + 部署全部组件）
./quickstart/quickstart.sh

# 访问
open http://localhost:8080
# 登录：admin / admin123

# 清理
./quickstart/cleanup.sh
```

quickstart 会自动完成：
1. 本地构建 6 个 Docker 镜像（不需要外部镜像仓库）
2. 创建单节点 kind 集群
3. 部署 PostgreSQL 17 + TimescaleDB、MinIO（S3 存储）、Redis
4. 执行数据库迁移 + 初始化管理员账号
5. 通过 Helm 部署 SAC 全部服务
6. 启动 nginx 反向代理，统一暴露到 localhost:8080

> quickstart 使用单副本、最小资源配置，仅用于体验和开发，不适合生产环境。

---

## 生产部署

### 前置条件

| 组件 | 要求 |
|------|------|
| Kubernetes | 1.28+，推荐托管集群（EKS / GKE / ACK） |
| PostgreSQL | 17+ 并启用 TimescaleDB 扩展 |
| S3 兼容存储 | 阿里云 OSS / AWS S3 / MinIO / R2 等 |
| Redis | standalone 模式即可，可选（不配置则 SSE 降级为手动刷新） |
| Helm | 3.x |
| 镜像仓库 | 能被 K8s 集群拉取的 Docker Registry |
| Ingress | 任意 Ingress 控制器，或使用内置的 Envoy Gateway 子 Chart |

### 步骤

#### 1. 构建并推送镜像

```bash
# 编辑 Makefile 中的 REGISTRY 变量，或通过环境变量覆盖
make docker-build    # 构建 5 个镜像，自动递增 .version
make docker-push     # 推送到镜像仓库
```

构建的镜像：

| 镜像 | 用途 |
|------|------|
| `api-gateway` | REST API + gRPC-Gateway 服务 |
| `ws-proxy` | WebSocket 终端代理 |
| `frontend` | Vue 3 SPA（nginx 托管） |
| `cc` | Claude Code 容器（Ubuntu + ttyd + Claude CLI + Node.js + Python） |
| `output-watcher` | Sidecar，监听 /workspace/output 并上传到 S3 |

#### 2. 配置 Helm Values

编辑 `helm/sac/values.yaml`：

```yaml
global:
  registry: your-registry.example.com/sac    # 镜像仓库地址
  namespace: sac

database:
  host: your-postgres-host
  port: "5432"
  user: sandbox
  password: your-strong-password             # 务必修改
  name: sandbox

auth:
  jwtSecret: your-random-jwt-secret          # 务必修改，建议 32+ 字符随机串

redis:
  enabled: true          # true = 使用内置 Redis 子 Chart
  externalURL: ""        # 如果 enabled=false，填外部 Redis 地址

envoyGateway:
  enabled: true          # 使用内置 Envoy Gateway
  host: sac.your-domain.com
```

如果使用自己的 Ingress 控制器（Nginx / Traefik / Istio），设置 `envoyGateway.enabled: false`，并配置路由：

| 路径 | 后端 | 协议 |
|------|------|------|
| `/api/*` | api-gateway:8080 | HTTP |
| `/ws/*` | ws-proxy:8081 | WebSocket |
| `/*` | frontend:80 | HTTP |

#### 3. 部署

```bash
make helm-dep-update   # 更新 Helm 依赖（Redis 子 Chart）
make helm-deploy       # 首次安装
# 或
make helm-upgrade      # 升级已有版本
```

#### 4. 初始化数据库

```bash
make migrate-up        # 执行全部 26 个迁移
make migrate-seed      # 创建管理员账号 admin / admin123
```

#### 5. 首次登录配置

1. 浏览器打开 `https://sac.your-domain.com`
2. 使用 `admin` / `admin123` 登录
3. 进入管理面板 → 系统设置，配置存储后端（见[存储配置](#存储配置)）
4. 修改管理员密码

---

## 初始配置

首次部署后，管理员需要完成以下配置才能正常使用：

1. **配置存储后端** — 在管理面板 → 系统设置中选择存储类型并填写凭据（必须）
2. **修改管理员密码** — 点击右上角钥匙图标修改默认密码
3. **设置注册模式** — 默认为 `invite`（仅管理员可创建用户），改为 `open` 允许自助注册
4. **调整资源限制** — 根据集群规模调整默认 CPU/内存限制和每用户 Agent 数量上限

---

## 用户指南

### 注册与登录

- 如果管理员开启了开放注册（`registration_mode: open`），用户可自行注册
- 注册需要：用户名、邮箱、密码（6 位以上）
- 如果是邀请制（`registration_mode: invite`），需要管理员在后台创建账号
- 登录后 JWT 有效期 24 小时，过期需重新登录

### Agent 管理

Agent 是你在 SAC 中的 AI 助手实例。每个 Agent 运行在独立的 K8s Pod 中，拥有独立的 LLM 配置和工作区。

#### 创建 Agent

1. 点击左侧边栏底部的「New Agent」按钮，或顶栏的「+」按钮
2. 填写基本信息：
   - **名称**：Agent 显示名
   - **描述**：用途说明
   - **图标**：选择一个 emoji
3. 配置 LLM 提供商：
   - 选择预设：Anthropic / OpenRouter / GLM（智谱）/ Qwen（通义千问）/ Kimi / 自定义
   - 填写 **Auth Token**（API Key，必填）
   - 填写 **Base URL**（API 端点，预设会自动填充）
   - 可选配置 Haiku / Opus / Sonnet 模型名称
4. 高级选项（可选）：
   - HTTP/HTTPS 代理
   - 自定义环境变量（键值对）
5. 点击「Create」

每个用户默认最多创建 3 个 Agent（管理员可调整）。

#### Agent 状态

| 状态 | 含义 |
|------|------|
| Running | Pod 运行中，可以连接终端 |
| Pending | Pod 正在启动，等待资源调度 |
| Not Deployed | 尚未创建过会话，Pod 不存在 |
| Failed / Error | Pod 启动失败，检查 LLM 配置或联系管理员 |

#### 管理操作

- **编辑**：修改 Agent 配置（名称、LLM 设置等）
- **删除**：删除 Agent 及其 Pod（工作区文件保留在 S3）
- **重启**：重启 Agent Pod（不丢失已安装的技能配置）
- **CLAUDE.md**：编辑 Agent 级别的指令，控制 Claude Code 的行为

### 终端与聊天

选择一个 Agent 后，系统会自动创建会话并连接终端。

#### 终端模式

- 完整的 PTY 终端，支持所有终端操作
- 直接输入命令与 Claude Code 交互
- 支持 Ctrl+C 中断、Tab 补全、方向键历史
- 支持中文和 CJK 宽字符显示
- 窗口大小自适应，支持拖拽调整

#### 聊天模式

- 底部输入框，按 Enter 发送消息
- 适合不熟悉终端的用户
- 消息会自动发送到终端并执行

#### 连接机制

- 首次选择 Agent 时，后端会创建 StatefulSet 并等待 Pod 就绪（最长 5 分钟）
- 连接建立后，通过 WebSocket 实时传输终端数据
- 断线自动重连（最多 10 次，指数退避）
- 切换 Agent 时，终端会显示切换提示横幅

### 工作区文件

右侧面板的「Output」标签页管理工作区文件。SAC 有四种工作区：

#### Output 工作区（Working 标签）

- Claude Code 生成的文件（代码、图片、文档等）自动出现在这里
- 实时更新：文件变更通过 Sidecar → S3 → Redis → SSE 推送到浏览器
- 点击文件可预览：
  - 文本文件：语法高亮，可编辑（私有工作区）或只读（Output）
  - CSV/TSV：数据表格，支持虚拟滚动
  - HTML：沙箱 iframe 预览
  - 图片：响应式图片查看器
  - 二进制文件：显示文件信息和下载按钮
- 支持批量选择、下载（ZIP 打包）、删除
- 支持拖拽上传文件和文件夹

#### 私有工作区

- 每个 Agent 独立的文件存储
- 会话启动时自动同步到 Pod 的 `/workspace/private/`
- 适合存放 Agent 需要的参考资料、配置文件等
- 默认配额：1GB / 1000 个文件

#### 公共工作区

- 管理员管理的共享文件空间
- 所有 Agent 的 Pod 都能访问（同步到 `/workspace/public/`）
- 适合存放团队共享的模板、数据集等

#### 团队工作区

- 按用户组隔离的共享空间
- 组内成员可上传和访问
- 支持按组配额管理

### 技能市场

技能（Skill）是可复用的斜杠命令，本质上是一个 SKILL.md 文件加上可选的附件资源。

#### 浏览与安装

1. 点击右侧面板「Skills」标签页的「Install」按钮，或顶栏的市场图标
2. 在「Browse Marketplace」标签页浏览所有可见技能
3. 技能卡片显示：图标、名称、命令名、描述、可见性标签、附件数量
4. 点击卡片查看详情（SKILL.md 内容和附件文件树）
5. 点击「Install」安装到当前 Agent

安装后，技能会同步到 Pod 的 `/root/.claude/skills/{command_name}/` 目录，包含 SKILL.md 和所有附件。

#### 创建技能

1. 进入技能市场 →「My Skills」标签页 → 点击「+ Create Skill」
2. 左侧面板填写技能信息：
   - **名称**：技能显示名
   - **命令名**：斜杠命令名（如 `review-code`，使用时输入 `/review-code`）
   - **描述**：技能用途说明
   - **图标**：选择 emoji
   - **可见性**：Private（仅自己）/ Public（所有人）/ Group（指定团队）
3. 右侧编辑器编写 SKILL.md（技能的核心 prompt）
4. 可选：通过左侧文件树添加附件文件
   - 支持拖拽上传文件和文件夹
   - 支持创建子目录
   - 附件会随技能一起同步到 Pod
5. 点击「Save」保存

#### 使用技能

- 在终端中输入 `/command-name` 执行技能
- 或在右侧「Skills」标签页点击已安装技能的命令名
- 如果技能定义了参数，会弹出表单让你填写

#### 技能同步

- 技能安装后，需要同步到 Pod 才能使用
- 点击「Skills」标签页的「Sync」按钮手动同步
- 如果技能有更新，会显示「NEW」标签和待更新数量
- 同步过程实时显示进度（写入 SKILL.md → 下载附件 → 重启 Claude Code 进程）
- 系统也会通过维护任务定期自动同步（默认每 10 分钟）

#### Fork 技能

- 在市场中看到别人的技能，可以点击「Fork」复制一份到自己名下
- Fork 后可以自由修改，不影响原技能

### 对话历史

1. 在左侧「Agent Dashboard」标签页点击「View History」
2. 可按会话筛选
3. 支持游标分页浏览
4. 支持导出为 CSV

对话通过 Pod 内的 `conversation-sync.mjs` Hook 自动采集，存储在 TimescaleDB 中，默认保留 30 天。

### 文件分享

Output 工作区中的文件可以生成公开分享链接：

1. 在文件列表中找到目标文件
2. 点击分享按钮
3. 系统生成一个公开 URL，无需登录即可访问
4. 分享链接支持在线预览和下载

---

## 管理员指南

管理员通过顶栏右侧的「Admin」按钮进入管理面板。

### 系统设置

管理面板 → System Settings 标签页，可配置以下全局参数：

| 设置项 | 默认值 | 说明 |
|--------|--------|------|
| `max_agents_per_user` | 3 | 每用户最大 Agent 数 |
| `default_cpu_request` | 2 | Agent Pod 默认 CPU 请求 |
| `default_cpu_limit` | 2 | Agent Pod 默认 CPU 上限 |
| `default_memory_request` | 4Gi | Agent Pod 默认内存请求 |
| `default_memory_limit` | 4Gi | Agent Pod 默认内存上限 |
| `docker_image` | prod/sac/cc:x.x.x | 新建 Agent 使用的 Claude Code 镜像 |
| `registration_mode` | invite | 注册模式：`open`（开放注册）或 `invite`（仅管理员创建） |
| `conversation_retention_days` | 30 | 对话历史保留天数 |
| `skill_sync_interval` | 10m | 维护任务中技能同步的间隔 |
| `agent_system_instructions` | (内置) | 注入到所有 Agent CLAUDE.md 的系统级指令 |

### 存储配置

在系统设置页面配置 S3 兼容存储，这是 SAC 正常运行的必要条件。

#### 阿里云 OSS

选择「OSS」后端，填写：
- Endpoint（如 `oss-cn-shanghai.aliyuncs.com`）
- Bucket 名称
- Access Key ID
- Access Key Secret

#### AWS S3

选择「S3」后端，填写：
- Region（如 `us-east-1`）
- Bucket 名称
- Access Key ID
- Secret Access Key

#### S3 兼容存储（MinIO / R2 / RustFS 等）

选择「S3-Compatible」后端，填写：
- Endpoint（如 `http://minio.sac:9000`）
- Bucket 名称
- Access Key ID
- Secret Access Key
- Use SSL（是否启用 HTTPS）

### 用户管理

管理面板 → User Management 标签页：

- 查看所有用户列表（用户名、邮箱、角色、创建时间）
- **修改角色**：将用户提升为 admin 或降级为 user
- **重置密码**：为用户设置新密码
- **资源设置**：为特定用户覆盖全局默认值（Agent 数量、CPU/内存限制）
- **查看用户 Agent**：查看用户的所有 Agent 及 Pod 状态
- **管理 Agent**：重启、删除用户的 Agent，调整资源限制

### 资源管控

SAC 采用三级资源配置：

```
系统默认值（system_settings）
  ↓ 被覆盖
用户级覆盖（user_settings）
  ↓ 被覆盖
Agent 级覆盖（agents 表）
```

管理员可以：
1. 在系统设置中调整全局默认值
2. 为特定用户设置独立的资源限制
3. 为特定 Agent 调整资源（通过用户管理 → 查看 Agent → 调整资源）

### 镜像升级

当发布新版本的 Claude Code 容器镜像时：

#### 单个 Agent 升级

管理面板 → 用户管理 → 查看用户 Agent → 更新镜像

#### 批量升级所有 Agent

```bash
# 通过 API 批量更新
curl -X PUT https://sac.your-domain.com/api/admin/agents/batch-image \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"image": "your-registry/sac/cc:0.0.29"}'
```

或在系统设置中更新 `docker_image`，新创建的 Agent 会使用新镜像。已有 Agent 需要通过批量更新 API 或逐个更新。

### 维护任务

SAC 通过 K8s CronJob 定期执行维护任务（默认每 10 分钟）：

| 任务 | 说明 |
|------|------|
| 技能同步 | 将所有 Agent 已安装的技能同步到 Pod |
| 对话清理 | 删除超过保留期的对话历史 |
| 会话清理 | 删除已停止/空闲超过阈值的会话记录 |
| 孤儿文件清理 | 删除已删除 Agent 遗留的工作区文件和 S3 对象 |

也可以通过 API 手动触发：

```bash
curl -X POST https://sac.your-domain.com/api/admin/maintenance/trigger \
  -H "Authorization: Bearer $JWT_TOKEN"
```

### 对话审计

管理员可以跨用户搜索和导出对话历史：

- 管理面板 → 对话管理（如有）
- 或通过 API：
  ```bash
  # 搜索对话
  curl "https://sac.your-domain.com/api/admin/conversations?user_id=1" \
    -H "Authorization: Bearer $JWT_TOKEN"

  # 导出 CSV
  curl "https://sac.your-domain.com/api/admin/conversations/export?user_id=1" \
    -H "Authorization: Bearer $JWT_TOKEN" -o conversations.csv
  ```

---

## 概念说明

### Agent 与 Pod 的关系

```
Agent（数据库记录）          Pod（K8s 资源）
├── 名称、描述、图标          ├── StatefulSet: claude-code-{uid}-{aid}
├── LLM 配置                 ├── 主容器: ttyd → claude CLI
├── 资源限制                  ├── Sidecar: output-watcher
├── 已安装技能                ├── Volume: /workspace (emptyDir)
└── CLAUDE.md 指令            └── ConfigMap: settings.json + hooks
```

- Agent 是逻辑概念，Pod 是运行实体
- 一个 Agent 对应一个 StatefulSet（最多 1 个 Pod）
- Pod 在首次创建会话时启动，长期运行（不随会话结束销毁）
- Pod 重启会清空 emptyDir（工作区文件从 S3 重新同步）

### 会话生命周期

```
用户选择 Agent
  → 创建会话（POST /api/sessions）
  → 后端检查 Pod 是否存在
    → 不存在：创建 StatefulSet → 等待 Pod Ready（最长 5 分钟）
    → 存在：直接使用
  → 同步工作区文件（S3 → Pod）
  → 同步已安装技能（S3 → Pod）
  → 返回会话 ID + Pod IP
  → 前端轮询等待就绪（最多 60 次，每次 2 秒）
  → WebSocket 连接建立
  → 终端可用
```

### 技能可见性模型

SAC 采用 4 级可见性：

| 级别 | 谁能看到 | 谁能安装 | 标签颜色 |
|------|---------|---------|---------|
| Official | 所有人 | 所有人 | 蓝色 |
| Public | 所有人 | 所有人 | 绿色 |
| Group | 同组成员 | 同组成员 | 橙色 |
| Private | 仅创建者 | 仅创建者 | 灰色 |

- `is_public` 和 `group_id` 互斥
- 安装时后端会校验可见性权限
- 同步时会自动清理无权访问的技能（懒撤销）

### 工作区类型

| 类型 | Pod 路径 | 同步方向 | 说明 |
|------|---------|---------|------|
| Private | `/workspace/private/` | S3 → Pod（会话启动时） | Agent 级私有文件 |
| Public | `/workspace/public/` | S3 → Pod（会话启动时） | 全局共享文件 |
| Output | `/workspace/output/` | Pod → S3（实时） | Claude Code 产出文件 |
| Skills | `/root/.claude/skills/` | S3 → Pod（同步时） | 已安装技能 |

### Output 实时推送

```
Claude Code 写入文件到 /workspace/output/
  → Sidecar (output-watcher) 通过 fsnotify 检测变更
  → POST /api/internal/output/upload（上传到 S3 + 写入数据库）
  → API Gateway PUBLISH 到 Redis channel sac:output:{userID}:{agentID}
  → 所有 API Gateway 副本 PSUBSCRIBE sac:output:*
  → 匹配的 SSE 连接收到事件
  → 浏览器实时更新文件列表
```

如果 Redis 不可用，SSE 返回 503，前端降级为手动刷新。

---

## 常见问题

### Pod 一直 Pending

- 检查集群资源是否充足（CPU/内存）
- 检查 Agent 配置的资源请求是否超过节点容量
- `kubectl describe pod claude-code-{uid}-{aid}-0 -n sac` 查看事件

### 终端连不上

1. 确认 Pod 状态为 Running
2. 检查 WebSocket 代理日志：`kubectl logs -l app=ws-proxy -n sac`
3. 确认 Ingress 正确转发 `/ws/*` 到 ws-proxy:8081
4. 如果使用 Envoy Gateway，确认 WebSocket 升级配置正确

### 文件不出现在 Output 标签

1. 确认 Sidecar 容器运行正常：`kubectl logs claude-code-{uid}-{aid}-0 -c output-watcher -n sac`
2. 确认存储后端配置正确（管理面板 → 系统设置）
3. 检查 Redis 连接：如果 Redis 不可用，需要手动刷新

### 技能同步失败

1. 确认 Pod 正在运行
2. 检查 API Gateway 日志中的同步错误
3. 手动触发同步：右侧面板 → Skills → Sync 按钮
4. 确认 S3 存储可访问

### 注册页面提示「邀请制」

管理员需要在系统设置中将 `registration_mode` 改为 `open`，或手动在后台创建用户账号。

### 如何备份数据

- **数据库**：定期备份 PostgreSQL（推荐使用托管数据库的自动快照）
- **文件**：S3 存储自带持久化，建议开启版本控制
- **配置**：Helm values.yaml 纳入版本控制

### 本地开发环境

SAC 使用 Telepresence 连接本地开发环境到 K8s 集群：

```bash
make dev              # 一键启动（Telepresence + 编译 + 启动全部服务）
make restart SVC=api  # 重启单个服务（api / ws / fe）
make logs SVC=api     # 查看日志
make status           # 查看服务状态
make stop             # 停止所有服务
```

| 服务 | 端口 | 日志文件 |
|------|------|---------|
| API Gateway | 8080 | /tmp/sac-api-gateway.log |
| WS Proxy | 8081 | /tmp/sac-ws-proxy.log |
| Frontend (Vite) | 5173 | /tmp/sac-frontend.log |
