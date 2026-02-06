#!/bin/bash

set -e

# Configuration
REGISTRY="docker-register-registry-vpc.cn-shanghai.cr.aliyuncs.com/dev"
VERSION="${VERSION:-latest}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if logged in to registry
check_registry_login() {
    log_info "Checking registry login status..."
    if ! docker info | grep -q "$REGISTRY"; then
        log_warn "Not logged in to registry. Please login first:"
        echo "  docker login $REGISTRY"
        exit 1
    fi
}

# Build and push API Gateway
build_api_gateway() {
    log_info "Building API Gateway image..."
    cd backend
    docker build -f Dockerfile.api-gateway \
        -t ${REGISTRY}/sac-api-gateway:${VERSION} \
        -t ${REGISTRY}/sac-api-gateway:latest \
        .

    log_info "Pushing API Gateway image..."
    docker push ${REGISTRY}/sac-api-gateway:${VERSION}
    docker push ${REGISTRY}/sac-api-gateway:latest

    cd ..
}

# Build and push WebSocket Proxy
build_ws_proxy() {
    log_info "Building WebSocket Proxy image..."
    cd backend
    docker build -f Dockerfile.ws-proxy \
        -t ${REGISTRY}/sac-ws-proxy:${VERSION} \
        -t ${REGISTRY}/sac-ws-proxy:latest \
        .

    log_info "Pushing WebSocket Proxy image..."
    docker push ${REGISTRY}/sac-ws-proxy:${VERSION}
    docker push ${REGISTRY}/sac-ws-proxy:latest

    cd ..
}

# Build and push Claude Code user container
build_claude_code() {
    log_info "Building Claude Code container image..."
    cd docker/claude-code
    docker build \
        -t ${REGISTRY}/sac-claude-code:${VERSION} \
        -t ${REGISTRY}/sac-claude-code:latest \
        .

    log_info "Pushing Claude Code container image..."
    docker push ${REGISTRY}/sac-claude-code:${VERSION}
    docker push ${REGISTRY}/sac-claude-code:latest

    cd ../..
}

# Main execution
main() {
    log_info "SAC Platform - Docker Image Builder"
    log_info "Registry: $REGISTRY"
    log_info "Version: $VERSION"
    echo ""

    # Check registry login
    check_registry_login

    # Build images based on arguments
    if [ "$1" == "api-gateway" ]; then
        build_api_gateway
    elif [ "$1" == "ws-proxy" ]; then
        build_ws_proxy
    elif [ "$1" == "claude-code" ]; then
        build_claude_code
    elif [ "$1" == "all" ] || [ -z "$1" ]; then
        build_api_gateway
        build_ws_proxy
        build_claude_code
    else
        log_error "Unknown target: $1"
        echo "Usage: $0 [api-gateway|ws-proxy|claude-code|all]"
        exit 1
    fi

    log_info "Build completed successfully!"
}

# Run main
main "$@"
