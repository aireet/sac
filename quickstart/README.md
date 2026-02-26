# SAC Quickstart (kind)

> **Warning**: This quickstart is for **local development and evaluation only**. Do NOT use this setup in production. See [Production Deployment](#production-deployment) for details.
>
> **警告**：本快速启动仅用于**本地开发和功能评估**，请勿在生产环境中使用此部署方式。详见 [生产环境部署](#生产环境部署)。

Get SAC running locally in a [kind](https://kind.sigs.k8s.io/) (Kubernetes in Docker) cluster. One script, ~3 minutes.

使用 [kind](https://kind.sigs.k8s.io/)（Kubernetes in Docker）在本地运行 SAC。一个脚本，约 3 分钟。

## Prerequisites / 前置条件

| Tool / 工具 | Version / 版本 | Install / 安装 |
|------|---------|---------|
| Docker | 20.10+ | https://docs.docker.com/get-docker/ |
| kind | 0.20+ | `brew install kind` or https://kind.sigs.k8s.io |
| kubectl | 1.28+ | https://kubernetes.io/docs/tasks/tools/ |
| Helm | 3.12+ | https://helm.sh/docs/intro/install/ |

**System requirements / 系统要求：**
- Docker needs at least **8 GB RAM** allocated (Docker Desktop → Settings → Resources)
  Docker 至少分配 **8 GB 内存**（Docker Desktop → Settings → Resources）
- ~10 GB free disk space (Docker images + PVCs)
  约 10 GB 可用磁盘空间
- x86_64 architecture (ARM/Apple Silicon not yet supported for the claude-code image)
  仅支持 x86_64 架构（ARM/Apple Silicon 暂不支持 claude-code 镜像）

You'll also need an **Anthropic API key** from https://console.anthropic.com — you'll enter it when creating your first agent.

你还需要一个 **Anthropic API Key**（从 https://console.anthropic.com 获取），在创建第一个 Agent 时填入。

## Quick Start / 快速开始

```bash
git clone https://github.com/anthropics/sac.git
cd sac
./quickstart/quickstart.sh
```

The script will guide you through / 脚本将引导你完成：

1. **Select Language / 选择语言** — English or 简体中文
2. **Select Image Registry / 选择镜像仓库** — Docker Hub or Huawei Cloud (for China users)
   - Docker Hub — for international users
   - 华为云 — 国内用户推荐，访问速度更快
3. Pull pre-built images (no local build needed) / 拉取预构建镜像（无需本地构建）
4. Create a single-node kind cluster with port mapping / 创建单节点 kind 集群并映射端口
5. Deploy PostgreSQL (TimescaleDB), MinIO (S3 storage), and Redis / 部署 PostgreSQL、MinIO、Redis
6. Run database migrations and seed the admin user / 执行数据库迁移并创建管理员账号
7. Deploy SAC services via Helm / 通过 Helm 部署 SAC 服务
8. Deploy an nginx reverse proxy as the single entry point / 部署 nginx 反向代理作为统一入口

When it's done / 完成后：

```
  URL:      http://localhost:8080
  Login:    admin / admin123
```

## Creating Your First Agent / 创建第一个 Agent

1. Open http://localhost:8080 and log in with `admin / admin123`
   打开 http://localhost:8080，使用 `admin / admin123` 登录
2. Click the **+** button next to "Current Agent" to create a new agent
   点击 "Current Agent" 旁边的 **+** 按钮创建新 Agent
3. Give it a name (e.g. "My Agent")
   输入名称（如 "My Agent"）
4. In the agent config section, set:
   在 Agent 配置中设置：
   - `ANTHROPIC_AUTH_TOKEN` — your Anthropic API key (required) / 你的 Anthropic API Key（必填）
   - `ANTHROPIC_BASE_URL` — custom API endpoint (optional, for proxy users) / 自定义 API 地址（可选，代理用户使用）
5. Click Create, then select the agent from the dropdown
   点击创建，然后从下拉菜单中选择该 Agent
6. A terminal session will start — you're now talking to Claude Code in the browser
   终端会话将启动 — 你现在可以在浏览器中使用 Claude Code 了

## Architecture Overview / 架构概览

```
localhost:8080 (your browser / 浏览器)
    │
    ▼
┌─────────────────────────────────────────────────┐
│  kind cluster "sac"                             │
│  namespace: sac                                 │
│                                                 │
│  ┌──────────────┐                               │
│  │ nginx-proxy  │ NodePort 30080                │
│  │ (reverse     │──┬── /api/*  → api-gateway    │
│  │  proxy)      │  ├── /ws/*   → ws-proxy       │
│  │              │  └── /*      → frontend       │
│  └──────────────┘                               │
│                                                 │
│  ┌─────────────┐ ┌───────────┐ ┌────────────┐  │
│  │ api-gateway │ │ ws-proxy  │ │  frontend   │  │
│  │ (Go, :8080) │ │ (Go,:8081)│ │ (Vue,:80)   │  │
│  └──────┬──────┘ └───────────┘ └────────────┘  │
│         │                                       │
│         │ creates per-agent StatefulSets         │
│         ▼                                       │
│  ┌──────────────────────────────────┐           │
│  │ claude-code-{user}-{agent}-0    │ (dynamic) │
│  │  ├─ claude-code (ttyd + claude) │           │
│  │  └─ output-watcher (sidecar)    │           │
│  └──────────────────────────────────┘           │
│                                                 │
│  ┌──────────┐ ┌───────┐ ┌───────┐              │
│  │ postgres │ │ minio │ │ redis │              │
│  │ (PG 17)  │ │ (S3)  │ │       │              │
│  └──────────┘ └───────┘ └───────┘              │
└─────────────────────────────────────────────────┘
```

## Image Registries / 镜像仓库

The quickstart supports two image registries / 快速启动支持两个镜像仓库：

| Registry / 仓库 | Address | Best For / 适用场景 |
|----------------|---------|-------------------|
| Docker Hub | `docker.io/opensac` | International users / 海外用户 |
| Huawei Cloud / 华为云 | `swr.cn-east-3.myhuaweicloud.com/open-sac` | China users / 国内用户 |

You can switch between them during setup. Both provide the same pre-built images.

你可以在设置过程中切换。两者提供相同的预构建镜像。

## Useful Commands / 常用命令

```bash
# Check all pods / 查看所有 Pod
kubectl get pods -n sac

# View api-gateway logs / 查看 api-gateway 日志
kubectl logs -n sac -l app=api-gateway --tail=50 -f

# View a specific agent pod's logs / 查看特定 Agent Pod 日志
kubectl logs -n sac claude-code-1-1-0 -c claude-code --tail=50

# Restart SAC services / 重启 SAC 服务
./quickstart/quickstart.sh   # re-running is safe / 重复运行是安全的，会复用已有集群

# Access MinIO console (optional) / 访问 MinIO 控制台（可选）
kubectl port-forward -n sac svc/minio 9001:9001
# Then open / 然后打开 http://localhost:9001 (minioadmin / minioadmin123)
```

## Cleanup / 清理

```bash
./quickstart/cleanup.sh
```

This deletes the Helm release, all infrastructure pods, the namespace, and the kind cluster.

该命令会删除 Helm release、所有基础设施 Pod、命名空间和 kind 集群。

## Troubleshooting / 故障排查

**Images fail to pull / 镜像拉取失败**

If you're in China and Docker Hub is slow, select Huawei Cloud registry during setup.

如果在国内且 Docker Hub 速度慢，请在设置时选择华为云镜像仓库。

```bash
# Manual pull test / 手动拉取测试
docker pull swr.cn-east-3.myhuaweicloud.com/open-sac/api-gateway:0.0.33
```

**Pod stuck in Pending / Pod 卡在 Pending 状态**

The kind node is likely out of resources. Check with `kubectl describe node` and increase Docker's RAM allocation to at least 8 GB.

kind 节点资源不足。使用 `kubectl describe node` 检查，并将 Docker 内存分配增加到至少 8 GB。

**Agent pod won't start / Agent Pod 无法启动**
- Verify the agent config has a valid `ANTHROPIC_AUTH_TOKEN` / 确认 Agent 配置中有有效的 `ANTHROPIC_AUTH_TOKEN`
- Check api-gateway logs / 查看 api-gateway 日志：`kubectl logs -n sac -l app=api-gateway --tail=50`
- Check the agent pod events / 查看 Agent Pod 事件：`kubectl describe pod -n sac claude-code-{user}-{agent}-0`

**Terminal connects but Claude doesn't respond / 终端已连接但 Claude 无响应**

Your Anthropic API key may be invalid or rate-limited. Check the claude-code container logs:

API Key 可能无效或已达到速率限制。查看 claude-code 容器日志：

```bash
kubectl logs -n sac claude-code-{user}-{agent}-0 -c claude-code --tail=20
```

**File upload/download errors / 文件上传下载错误**

MinIO must be running with the bucket created:

MinIO 必须正常运行且 bucket 已创建：

```bash
kubectl get pods -n sac -l app=minio
```

If MinIO was restarted, re-create the bucket / 如果 MinIO 重启过，重新创建 bucket：

```bash
kubectl run minio-fix --rm -it --restart=Never -n sac --image=minio/mc:latest -- \
  bash -c "mc alias set local http://minio:9000 minioadmin minioadmin123 && mc mb local/sac-workspace --ignore-existing"
```

**Database connection errors / 数据库连接错误**

```bash
kubectl get pods -n sac -l app=postgres
kubectl logs -n sac -l app=postgres --tail=20
```

## Default Credentials / 默认凭据

> ⚠️ **Security Warning / 安全警告**: These are hardcoded for quickstart only. Change them for any non-local use.

| Service | Username | Password |
|---------|----------|----------|
| SAC Web | `admin` | `admin123` |
| PostgreSQL | `sac` | `sac-quickstart-pass` |
| MinIO | `minioadmin` | `minioadmin123` |
| Redis | (no auth) | (no auth) |

## Production Deployment / 生产环境部署

This quickstart uses a **standalone single-node setup** that is NOT suitable for production:

本快速启动使用的是**单机独立部署**，不适用于生产环境：

- **No high availability / 无高可用** — single-replica PostgreSQL, MinIO, and Redis with no failover / 单副本 PostgreSQL、MinIO、Redis，无故障转移
- **No data durability / 无数据持久性** — kind PVCs are ephemeral; deleting the cluster loses all data / kind PVC 是临时的，删除集群将丢失所有数据
- **No TLS / 无 TLS** — all traffic is unencrypted HTTP / 所有流量均为未加密的 HTTP
- **No backup / 无备份** — no automated database or storage backups / 无自动数据库或存储备份
- **Weak credentials / 弱凭据** — hardcoded passwords, default JWT secret / 硬编码密码，默认 JWT 密钥
- **No resource isolation / 无资源隔离** — all workloads share a single kind node / 所有工作负载共享单个 kind 节点
- **No ingress controller / 无 Ingress 控制器** — nginx reverse proxy replaces Envoy Gateway / nginx 反向代理替代 Envoy Gateway

For production, you should / 生产环境建议：

| Component / 组件 | Quickstart / 快速启动 | Production / 生产环境 |
|-----------|-----------|------------|
| Kubernetes | kind (single node / 单节点) | Managed K8s (EKS, GKE, AKS, ACK) |
| PostgreSQL | Single pod + PVC / 单 Pod | Managed RDS (with TimescaleDB), multi-AZ / 托管 RDS，多可用区 |
| Storage / 存储 | MinIO (single pod / 单 Pod) | Alibaba OSS / AWS S3 / managed MinIO |
| Redis | Bitnami standalone / 单机 | Managed Redis (ElastiCache, Tair) / 托管 Redis |
| Ingress / 入口 | nginx reverse proxy / 反向代理 | Envoy Gateway with TLS termination / 带 TLS 的 Envoy Gateway |
| Secrets / 密钥 | Hardcoded in YAML / 硬编码 | K8s Secrets + external secret manager / 外部密钥管理 |
| Images / 镜像 | Pre-built from registry / 预构建镜像 | Container registry (ECR, ACR, GHCR) / 容器镜像仓库 |
| Monitoring / 监控 | None / 无 | Prometheus + Grafana |
| Backup / 备份 | None / 无 | Automated DB snapshots + S3 versioning / 自动快照 + S3 版本控制 |

Refer to the main [README](../README.md) and `helm/sac/values.yaml` for production Helm configuration.

生产环境 Helm 配置请参考主 [README](../README.md) 和 `helm/sac/values.yaml`。
