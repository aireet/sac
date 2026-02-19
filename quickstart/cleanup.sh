#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="sac"
NAMESPACE="sac"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[INFO]${NC} $*"; }
ok()    { echo -e "${GREEN}[OK]${NC} $*"; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

info "Tearing down SAC quickstart..."

# Uninstall Helm release
helm uninstall sac -n "$NAMESPACE" 2>/dev/null && ok "Helm release removed." || true

# Delete infrastructure
kubectl delete -f "$SCRIPT_DIR/nginx-proxy.yaml" 2>/dev/null || true
kubectl delete -f "$SCRIPT_DIR/minio.yaml" 2>/dev/null || true
kubectl delete -f "$SCRIPT_DIR/postgres.yaml" 2>/dev/null || true

# Delete namespace
kubectl delete namespace "$NAMESPACE" 2>/dev/null || true

# Delete kind cluster
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  kind delete cluster --name "$CLUSTER_NAME"
  ok "kind cluster deleted."
fi

echo ""
echo -e "${GREEN}Cleanup complete.${NC}"
echo "Local Docker images (sac-local/*) retained. Remove manually if desired:"
echo "  docker rmi \$(docker images 'sac-local/*' -q) 2>/dev/null"
