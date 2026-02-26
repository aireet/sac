#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CLUSTER_NAME="sac"
NAMESPACE="sac"
TAG="0.0.33"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[INFO]${NC} $*"; }
ok()    { echo -e "${GREEN}[OK]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
fail()  { echo -e "${RED}[FAIL]${NC} $*"; exit 1; }

# ── Interactive Configuration ────────────────────────────────────────

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  SAC Quickstart Setup${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Language selection
echo "请选择语言 / Please select language:"
echo "  1) 简体中文 (Chinese)"
echo "  2) English"
read -r -p "> " LANG_CHOICE

case "$LANG_CHOICE" in
  1)
    LANG="zh"
    ;;
  2|*)
    LANG="en"
    ;;
esac

# Registry selection
if [ "$LANG" = "zh" ]; then
  echo ""
  echo "请选择镜像仓库 / Please select image registry:"
  echo "  1) Docker Hub (docker.io) - 海外用户推荐"
  echo "  2) 华为云 (Huawei Cloud) - 国内用户推荐"
else
  echo ""
  echo "Please select image registry:"
  echo "  1) Docker Hub (docker.io) - Recommended for international users"
  echo "  2) Huawei Cloud - Recommended for China users"
fi
read -r -p "> " REG_CHOICE

case "$REG_CHOICE" in
  2)
    REGISTRY="swr.cn-east-3.myhuaweicloud.com/open-sac"
    REGISTRY_NAME="huaweicloud"
    POSTGRES_IMAGE="swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/timescale/timescaledb:2.22.1-pg17"
    REDIS_IMAGE="swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/bitnami/redis:7.4.2-debian-12-r0"
    MINIO_IMAGE="swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/minio/minio:RELEASE.2025-02-28T09-56-22Z"
    ;;
  1|*)
    REGISTRY="docker.io/opensac"
    REGISTRY_NAME="dockerhub"
    POSTGRES_IMAGE="timescale/timescaledb:latest-pg17"
    REDIS_IMAGE=""
    MINIO_IMAGE=""
    ;;
esac

# Set localized messages
if [ "$LANG" = "zh" ]; then
  MSG_CHECKING_PREQS="检查前置条件..."
  MSG_PREQS_MET="所有前置条件已满足"
  MSG_DOWNLOAD_IMAGES="正在下载镜像（首次需要几分钟）..."
  MSG_IMG_API="API 网关"
  MSG_IMG_WS="WebSocket 代理"
  MSG_IMG_FE="前端"
  MSG_IMG_CC="Claude Code"
  MSG_IMG_OW="Output Watcher"
  MSG_IMG_MIGRATE="迁移工具"
  MSG_CLUSTER_EXISTS="kind 集群已存在，复用之"
  MSG_CREATING_CLUSTER="创建 kind 集群..."
  MSG_CLUSTER_READY="kind 集群就绪"
  MSG_LOADING_IMAGES="加载镜像到 kind（可能需要一分钟）..."
  MSG_IMAGES_LOADED="所有镜像已加载"
  MSG_DEPLOY_INFRA="部署基础设施..."
  MSG_WAITING_PG="等待 PostgreSQL 就绪..."
  MSG_PG_READY="PostgreSQL 就绪"
  MSG_WAITING_MINIO="等待 MinIO 就绪..."
  MSG_MINIO_READY="MinIO 就绪"
  MSG_RUNNING_MIGRATIONS="执行数据库迁移..."
  MSG_MIGRATIONS_DONE="迁移完成"
  MSG_SEEDING_ADMIN="创建管理员账号..."
  MSG_ADMIN_SEEDED="管理员账号已创建"
  MSG_CREATING_BUCKET="创建 MinIO 存储桶..."
  MSG_BUCKET_READY="存储桶已创建"
  MSG_CONFIGURING_STORAGE="配置存储..."
  MSG_STORAGE_CONFIGURED="存储配置完成"
  MSG_UPDATING_HELM="更新 Helm 依赖..."
  MSG_INSTALLING="部署 SAC..."
  MSG_INSTALLED="SAC 部署完成"
  MSG_DEPLOYING_PROXY="部署 nginx 反向代理..."
  MSG_PROXY_READY="反向代理就绪"
  MSG_READY="SAC 已就绪！"
  MSG_URL="访问地址"
  MSG_LOGIN="登录账号"
  MSG_API_KEY_HINT="创建 Agent 时请设置 Anthropic API Key 以开始使用 Claude Code"
  MSG_TEARDOWN="清理命令"
else
  MSG_CHECKING_PREQS="Checking prerequisites..."
  MSG_PREQS_MET="All prerequisites met"
  MSG_DOWNLOAD_IMAGES="Downloading images (first time may take a few minutes)..."
  MSG_IMG_API="API Gateway"
  MSG_IMG_WS="WebSocket Proxy"
  MSG_IMG_FE="Frontend"
  MSG_IMG_CC="Claude Code"
  MSG_IMG_OW="Output Watcher"
  MSG_IMG_MIGRATE="Migrate"
  MSG_CLUSTER_EXISTS="kind cluster already exists, reusing it"
  MSG_CREATING_CLUSTER="Creating kind cluster..."
  MSG_CLUSTER_READY="kind cluster ready"
  MSG_LOADING_IMAGES="Loading images into kind (this may take a minute)..."
  MSG_IMAGES_LOADED="All images loaded"
  MSG_DEPLOY_INFRA="Deploying infrastructure..."
  MSG_WAITING_PG="Waiting for PostgreSQL..."
  MSG_PG_READY="PostgreSQL ready"
  MSG_WAITING_MINIO="Waiting for MinIO..."
  MSG_MINIO_READY="MinIO ready"
  MSG_RUNNING_MIGRATIONS="Running database migrations..."
  MSG_MIGRATIONS_DONE="Migrations complete"
  MSG_SEEDING_ADMIN="Seeding admin user..."
  MSG_ADMIN_SEEDED="Admin user created"
  MSG_CREATING_BUCKET="Creating MinIO bucket..."
  MSG_BUCKET_READY="Bucket created"
  MSG_CONFIGURING_STORAGE="Configuring storage..."
  MSG_STORAGE_CONFIGURED="Storage configured"
  MSG_UPDATING_HELM="Updating Helm dependencies..."
  MSG_INSTALLING="Installing SAC..."
  MSG_INSTALLED="SAC deployed"
  MSG_DEPLOYING_PROXY="Deploying nginx reverse proxy..."
  MSG_PROXY_READY="Nginx proxy ready"
  MSG_READY="SAC is ready!"
  MSG_URL="URL"
  MSG_LOGIN="Login"
  MSG_API_KEY_HINT="Create an agent and set your Anthropic API key to start using Claude Code"
  MSG_TEARDOWN="To tear down"
fi

# ── Phase 1: Prerequisites ──────────────────────────────────────────

info "$MSG_CHECKING_PREQS"

for cmd in docker kind kubectl helm; do
  command -v "$cmd" &>/dev/null || fail "$cmd is not installed. Please install it first."
done

docker info &>/dev/null || fail "Docker daemon is not running."

ok "$MSG_PREQS_MET"

# ── Phase 2: Pull Docker images ──────────────────────────────────────

info "$MSG_DOWNLOAD_IMAGES"

for img in api-gateway ws-proxy frontend cc output-watcher; do
  docker pull "${REGISTRY}/${img}:${TAG}" || warn "Failed to pull ${img}, will try local build"
done

# Tag images for local use
for img in api-gateway ws-proxy frontend cc output-watcher; do
  docker tag "${REGISTRY}/${img}:${TAG}" "sac-local/${img}:quickstart" 2>/dev/null || true
done

# Build migrate image locally (not in registry)
docker build -f "$ROOT_DIR/docker/migrate/Dockerfile" -t "sac-local/migrate:quickstart" "$ROOT_DIR"

ok "api-gateway"
ok "ws-proxy"
ok "frontend"
ok "claude-code"
ok "output-watcher"
ok "migrate"

# ── Phase 3: Create kind cluster ─────────────────────────────────────

if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  warn "$MSG_CLUSTER_EXISTS"
else
  info "$MSG_CREATING_CLUSTER"
  kind create cluster --name "$CLUSTER_NAME" --config "$SCRIPT_DIR/kind-config.yaml"
fi

kubectl cluster-info --context "kind-$CLUSTER_NAME" &>/dev/null || fail "Cannot connect to kind cluster."
ok "$MSG_CLUSTER_READY"

# ── Phase 4: Load images into kind ───────────────────────────────────

info "$MSG_LOADING_IMAGES"

for img in api-gateway ws-proxy frontend cc output-watcher migrate; do
  kind load docker-image "sac-local/${img}:quickstart" --name "$CLUSTER_NAME" 2>/dev/null || \
    kind load docker-image "${REGISTRY}/${img}:${TAG}" --name "$CLUSTER_NAME" 2>/dev/null || \
    warn "Could not load image: ${img}"
done

ok "$MSG_IMAGES_LOADED"

# ── Phase 5: Deploy infrastructure ───────────────────────────────────

info "$MSG_DEPLOY_INFRA"

kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# Deploy PostgreSQL with appropriate image for selected registry
cat "$SCRIPT_DIR/postgres.yaml" | sed "s|image: timescale/timescaledb:latest-pg17|image: ${POSTGRES_IMAGE}|" | kubectl apply -f -

# Deploy MinIO with appropriate image for selected registry
if [ -n "$MINIO_IMAGE" ]; then
  cat "$SCRIPT_DIR/minio.yaml" | sed "s|image: minio/minio:latest|image: ${MINIO_IMAGE}|" | kubectl apply -f -
else
  kubectl apply -f "$SCRIPT_DIR/minio.yaml"
fi

info "$MSG_WAITING_PG"
kubectl wait --for=condition=available deployment/postgres -n "$NAMESPACE" --timeout=180s
ok "$MSG_PG_READY"

info "$MSG_WAITING_MINIO"
kubectl wait --for=condition=available deployment/minio -n "$NAMESPACE" --timeout=120s
ok "$MSG_MINIO_READY"

# ── Phase 6: Run database migrations ─────────────────────────────────

info "$MSG_RUNNING_MIGRATIONS"

kubectl run sac-migrate --rm -i --restart=Never -n "$NAMESPACE" \
  --image="sac-local/migrate:quickstart" \
  --image-pull-policy=Never \
  --env="DB_HOST=postgres.sac.svc.cluster.local" \
  --env="DB_PORT=5432" \
  --env="DB_USER=sac" \
  --env="DB_PASSWORD=sac-quickstart-pass" \
  --env="DB_NAME=sac" \
  -- -action=up

ok "$MSG_MIGRATIONS_DONE"

info "$MSG_SEEDING_ADMIN"

kubectl run sac-seed --rm -i --restart=Never -n "$NAMESPACE" \
  --image="sac-local/migrate:quickstart" \
  --image-pull-policy=Never \
  --env="DB_HOST=postgres.sac.svc.cluster.local" \
  --env="DB_PORT=5432" \
  --env="DB_USER=sac" \
  --env="DB_PASSWORD=sac-quickstart-pass" \
  --env="DB_NAME=sac" \
  -- -action=seed

ok "$MSG_ADMIN_SEEDED"

# ── Phase 7: Create MinIO bucket & seed storage settings ─────────────

info "$MSG_CREATING_BUCKET"

kubectl run minio-setup --rm -i --restart=Never -n "$NAMESPACE" \
  --image=minio/mc:latest \
  -- bash -c "
    mc alias set local http://minio:9000 minioadmin minioadmin123 &&
    mc mb local/sac-workspace --ignore-existing &&
    echo 'Bucket created.'
  "

ok "$MSG_BUCKET_READY"

info "$MSG_CONFIGURING_STORAGE"

kubectl run seed-storage --rm -i --restart=Never -n "$NAMESPACE" \
  --image=postgres:17-alpine \
  --env="PGPASSWORD=sac-quickstart-pass" \
  -- psql -h postgres -U sac -d sac -c "
    UPDATE system_settings SET value = '\"s3compat\"' WHERE key = 'storage_type';
    UPDATE system_settings SET value = '\"minio.sac.svc.cluster.local:9000\"' WHERE key = 's3compat_endpoint';
    UPDATE system_settings SET value = '\"minioadmin\"' WHERE key = 's3compat_access_key_id';
    UPDATE system_settings SET value = '\"minioadmin123\"' WHERE key = 's3compat_secret_access_key';
    UPDATE system_settings SET value = '\"sac-workspace\"' WHERE key = 's3compat_bucket';
    UPDATE system_settings SET value = '\"false\"' WHERE key = 's3compat_use_ssl';
  "

ok "$MSG_STORAGE_CONFIGURED"

# ── Phase 8: Deploy SAC via Helm ──────────────────────────────────────

info "$MSG_UPDATING_HELM"
helm dependency update "$ROOT_DIR/helm/sac" 2>/dev/null || helm dependency build "$ROOT_DIR/helm/sac"

info "$MSG_INSTALLING"
HELM_REDIS_OPTS=""
if [ -n "$REDIS_IMAGE" ]; then
  HELM_REDIS_OPTS="--set redis.image.repository=${REDIS_IMAGE%:*} --set redis.image.tag=${REDIS_IMAGE##*:}"
fi

helm upgrade --install sac "$ROOT_DIR/helm/sac" \
  -n "$NAMESPACE" \
  -f "$SCRIPT_DIR/values-quickstart.yaml" \
  $HELM_REDIS_OPTS \
  --wait --timeout 300s

ok "$MSG_INSTALLED"

# ── Phase 9: Deploy nginx proxy ───────────────────────────────────────

info "$MSG_DEPLOYING_PROXY"
kubectl apply -f "$SCRIPT_DIR/nginx-proxy.yaml"
kubectl wait --for=condition=available deployment/nginx-proxy -n "$NAMESPACE" --timeout=60s
ok "$MSG_PROXY_READY"

# ── Done ──────────────────────────────────────────────────────────────

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  ${MSG_READY}${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "  ${MSG_URL}:      ${CYAN}http://localhost:8080${NC}"
echo -e "  ${MSG_LOGIN}:    ${CYAN}admin / admin123${NC}"
echo ""
echo "  ${MSG_API_KEY_HINT}"
echo ""
echo -e "  ${MSG_TEARDOWN}: ${YELLOW}./quickstart/cleanup.sh${NC}"
echo -e "${GREEN}========================================${NC}"
