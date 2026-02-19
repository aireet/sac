#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CLUSTER_NAME="sac"
NAMESPACE="sac"
TAG="quickstart"

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

# ── Phase 1: Prerequisites ──────────────────────────────────────────

info "Checking prerequisites..."

for cmd in docker kind kubectl helm; do
  command -v "$cmd" &>/dev/null || fail "$cmd is not installed. Please install it first."
done

docker info &>/dev/null || fail "Docker daemon is not running."

ok "All prerequisites met."

# ── Phase 2: Build Docker images ─────────────────────────────────────

info "Building Docker images (this may take a few minutes)..."

docker build -f "$SCRIPT_DIR/docker/api-gateway.Dockerfile" -t "sac-local/api-gateway:$TAG" "$ROOT_DIR"
ok "api-gateway"

docker build -f "$SCRIPT_DIR/docker/ws-proxy.Dockerfile" -t "sac-local/ws-proxy:$TAG" "$ROOT_DIR"
ok "ws-proxy"

docker build -f "$SCRIPT_DIR/docker/frontend.Dockerfile" -t "sac-local/frontend:$TAG" "$ROOT_DIR"
ok "frontend"

docker build -f "$SCRIPT_DIR/docker/claude-code.Dockerfile" -t "sac-local/cc:$TAG" "$ROOT_DIR/docker/claude-code"
ok "claude-code"

docker build -f "$SCRIPT_DIR/docker/output-watcher.Dockerfile" -t "sac-local/output-watcher:$TAG" "$ROOT_DIR"
ok "output-watcher"

docker build -f "$SCRIPT_DIR/docker/migrate.Dockerfile" -t "sac-local/migrate:$TAG" "$ROOT_DIR"
ok "migrate"

# ── Phase 3: Create kind cluster ─────────────────────────────────────

if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  warn "kind cluster '$CLUSTER_NAME' already exists, reusing it."
else
  info "Creating kind cluster '$CLUSTER_NAME'..."
  kind create cluster --name "$CLUSTER_NAME" --config "$SCRIPT_DIR/kind-config.yaml"
fi

kubectl cluster-info --context "kind-$CLUSTER_NAME" &>/dev/null || fail "Cannot connect to kind cluster."
ok "kind cluster ready."

# ── Phase 4: Load images into kind ───────────────────────────────────

info "Loading images into kind (this may take a minute)..."

for img in api-gateway ws-proxy frontend cc output-watcher migrate; do
  kind load docker-image "sac-local/${img}:$TAG" --name "$CLUSTER_NAME" 2>/dev/null
done

ok "All images loaded."

# ── Phase 5: Deploy infrastructure ───────────────────────────────────

info "Creating namespace and deploying infrastructure..."

kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f "$SCRIPT_DIR/postgres.yaml"
kubectl apply -f "$SCRIPT_DIR/minio.yaml"

info "Waiting for PostgreSQL to be ready..."
kubectl wait --for=condition=available deployment/postgres -n "$NAMESPACE" --timeout=180s
ok "PostgreSQL ready."

info "Waiting for MinIO to be ready..."
kubectl wait --for=condition=available deployment/minio -n "$NAMESPACE" --timeout=120s
ok "MinIO ready."

# ── Phase 6: Run database migrations ─────────────────────────────────

info "Running database migrations..."

kubectl run sac-migrate --rm -i --restart=Never -n "$NAMESPACE" \
  --image="sac-local/migrate:$TAG" \
  --image-pull-policy=Never \
  --env="DB_HOST=postgres.sac.svc.cluster.local" \
  --env="DB_PORT=5432" \
  --env="DB_USER=sac" \
  --env="DB_PASSWORD=sac-quickstart-pass" \
  --env="DB_NAME=sac" \
  -- -action=up

ok "Migrations complete."

info "Seeding admin user..."

kubectl run sac-seed --rm -i --restart=Never -n "$NAMESPACE" \
  --image="sac-local/migrate:$TAG" \
  --image-pull-policy=Never \
  --env="DB_HOST=postgres.sac.svc.cluster.local" \
  --env="DB_PORT=5432" \
  --env="DB_USER=sac" \
  --env="DB_PASSWORD=sac-quickstart-pass" \
  --env="DB_NAME=sac" \
  -- -action=seed

ok "Admin user seeded."

# ── Phase 7: Create MinIO bucket & seed storage settings ─────────────

info "Creating MinIO bucket..."

kubectl run minio-setup --rm -i --restart=Never -n "$NAMESPACE" \
  --image=minio/mc:latest \
  -- bash -c "
    mc alias set local http://minio:9000 minioadmin minioadmin123 &&
    mc mb local/sac-workspace --ignore-existing &&
    echo 'Bucket created.'
  "

ok "MinIO bucket ready."

info "Seeding storage settings..."

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

ok "Storage configured."

# ── Phase 8: Deploy SAC via Helm ──────────────────────────────────────

info "Updating Helm dependencies..."
helm dependency update "$ROOT_DIR/helm/sac" 2>/dev/null || helm dependency build "$ROOT_DIR/helm/sac"

info "Installing SAC..."
helm upgrade --install sac "$ROOT_DIR/helm/sac" \
  -n "$NAMESPACE" \
  -f "$SCRIPT_DIR/values-quickstart.yaml" \
  --wait --timeout 300s

ok "SAC deployed."

# ── Phase 9: Deploy nginx proxy ───────────────────────────────────────

info "Deploying nginx reverse proxy..."
kubectl apply -f "$SCRIPT_DIR/nginx-proxy.yaml"
kubectl wait --for=condition=available deployment/nginx-proxy -n "$NAMESPACE" --timeout=60s
ok "Nginx proxy ready."

# ── Done ──────────────────────────────────────────────────────────────

echo ""
echo -e "${GREEN}============================================${NC}"
echo -e "${GREEN}  SAC is ready!${NC}"
echo -e "${GREEN}============================================${NC}"
echo ""
echo -e "  URL:      ${CYAN}http://localhost:8080${NC}"
echo -e "  Login:    ${CYAN}admin / admin123${NC}"
echo ""
echo "  Create an agent and set your Anthropic API key"
echo "  in the agent's config to start using Claude Code."
echo ""
echo -e "  To tear down: ${YELLOW}./quickstart/cleanup.sh${NC}"
echo -e "${GREEN}============================================${NC}"
