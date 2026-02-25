.PHONY: dev stop build build-api build-ws kill-port frontend backend telepresence status \
       docker-build docker-push docker-build-api docker-build-ws docker-build-fe docker-build-cc \
       docker-build-ow docker-push-api docker-push-ws docker-push-fe docker-push-cc docker-push-ow \
       helm-deploy helm-upgrade helm-dry-run helm-uninstall helm-dep-update \
       proto proto-go proto-ts

# Ports
API_PORT  := 8080
WS_PORT   := 8081
VITE_PORT := 5173

# Dirs
BACKEND  := backend
FRONTEND := frontend

# Docker
REGISTRY   ?= docker-register-registry-vpc.cn-shanghai.cr.aliyuncs.com/prod/sac
VERSION    := $(shell cat .version 2>/dev/null || echo "0.0.4")
KUBECONFIG ?= $(HOME)/.kube/config
NAMESPACE  ?= sac

# --- Main targets ---

## Start full dev environment (telepresence + build + all services)
dev: telepresence build stop
	@echo "==> Starting API Gateway on :$(API_PORT)"
	@cd $(BACKEND) && nohup ./bin/api-gateway > /tmp/sac-api-gateway.log 2>&1 & echo "API Gateway PID: $$!"
	@echo "==> Starting WS Proxy on :$(WS_PORT)"
	@cd $(BACKEND) && nohup ./bin/ws-proxy > /tmp/sac-ws-proxy.log 2>&1 & echo "WS Proxy PID: $$!"
	@echo "==> Starting Frontend on :$(VITE_PORT)"
	@cd $(FRONTEND) && nohup npm run dev -- --host 0.0.0.0 > /tmp/sac-frontend.log 2>&1 & echo "Frontend PID: $$!"
	@sleep 2
	@$(MAKE) status --no-print-directory

## Stop all dev services
stop:
	@echo "==> Stopping services on ports $(API_PORT), $(WS_PORT), $(VITE_PORT)"
	@for port in $(API_PORT) $(WS_PORT) $(VITE_PORT); do \
		pid=$$(ss -tlnp | grep ":$$port " | sed -n 's/.*pid=\([0-9]*\).*/\1/p'); \
		if [ -n "$$pid" ]; then \
			kill -9 $$pid 2>/dev/null && echo "  Killed PID $$pid on :$$port"; \
		fi; \
	done

## Show service status
status:
	@echo "==> Service Status:"
	@for port in $(API_PORT) $(WS_PORT) $(VITE_PORT); do \
		info=$$(ss -tlnp | grep ":$$port "); \
		if [ -n "$$info" ]; then \
			name=$$(echo "$$info" | sed -n 's/.*("\([^"]*\)".*/\1/p'); \
			pid=$$(echo "$$info" | sed -n 's/.*pid=\([0-9]*\).*/\1/p'); \
			echo "  :$$port  ✓  $$name (pid=$$pid)"; \
		else \
			echo "  :$$port  ✗  not running"; \
		fi; \
	done

## Show logs (usage: make logs SVC=api-gateway|ws-proxy|frontend)
logs:
	@case "$(SVC)" in \
		api-gateway|api) tail -f /tmp/sac-api-gateway.log ;; \
		ws-proxy|ws)     tail -f /tmp/sac-ws-proxy.log ;; \
		frontend|fe)     tail -f /tmp/sac-frontend.log ;; \
		*) echo "Usage: make logs SVC=api-gateway|ws-proxy|frontend" ;; \
	esac

# --- Build targets ---

build: build-api build-ws build-maintenance

build-api:
	@echo "==> Building API Gateway"
	@cd $(BACKEND) && go build -o bin/api-gateway ./cmd/api-gateway

build-ws:
	@echo "==> Building WS Proxy"
	@cd $(BACKEND) && go build -o bin/ws-proxy ./cmd/ws-proxy

build-maintenance:
	@echo "==> Building Maintenance"
	@cd $(BACKEND) && go build -o bin/maintenance ./cmd/maintenance

# --- Infra targets ---

telepresence:
	@if telepresence status 2>/dev/null | grep -q "Connected"; then \
		echo "==> Telepresence: already connected"; \
	else \
		echo "==> Connecting Telepresence..."; \
		KUBECONFIG=../kubeconfig.yaml telepresence connect; \
	fi

## Database migration
migrate-up:
	@cd $(BACKEND) && go run ./cmd/migrate -action=up

migrate-seed:
	@cd $(BACKEND) && go run ./cmd/migrate -action=seed

# ============================================================
# Docker Build & Push
# ============================================================

## Bump patch version in .version file (0.0.4 -> 0.0.5)
version-bump:
	@CURRENT=$$(cat .version); \
	MAJOR=$$(echo $$CURRENT | cut -d. -f1); \
	MINOR=$$(echo $$CURRENT | cut -d. -f2); \
	PATCH=$$(echo $$CURRENT | cut -d. -f3); \
	NEW_PATCH=$$((PATCH + 1)); \
	NEW_VER="$$MAJOR.$$MINOR.$$NEW_PATCH"; \
	echo "$$NEW_VER" > .version; \
	echo "==> Version bumped: $$CURRENT -> $$NEW_VER"

## Show current version
version:
	@echo "Current version: $(VERSION)"

## Build all Docker images (auto bumps version)
docker-build: version-bump docker-build-api docker-build-ws docker-build-fe docker-build-cc docker-build-ow
	@echo "==> All images built with tag: $$(cat .version)"

## Push all Docker images
docker-push: docker-push-api docker-push-ws docker-push-fe docker-push-cc docker-push-ow
	@echo "==> All images pushed with tag: $$(cat .version)"

docker-build-api:
	@echo "==> Building API Gateway image ($$(cat .version))"
	@docker build -f docker/api-gateway/Dockerfile \
		-t $(REGISTRY)/api-gateway:$$(cat .version) .

docker-build-ws:
	@echo "==> Building WS Proxy image ($$(cat .version))"
	@docker build -f docker/ws-proxy/Dockerfile \
		-t $(REGISTRY)/ws-proxy:$$(cat .version) .

docker-build-fe:
	@echo "==> Building Frontend image ($$(cat .version))"
	@docker build -f docker/frontend/Dockerfile \
		-t $(REGISTRY)/frontend:$$(cat .version) .

docker-build-cc:
	@echo "==> Building Claude Code image ($$(cat .version))"
	@docker build -f docker/claude-code/Dockerfile \
		-t $(REGISTRY)/cc:$$(cat .version) docker/claude-code

docker-push-api:
	@echo "==> Pushing API Gateway image ($$(cat .version))"
	@docker push $(REGISTRY)/api-gateway:$$(cat .version)

docker-push-ws:
	@echo "==> Pushing WS Proxy image ($$(cat .version))"
	@docker push $(REGISTRY)/ws-proxy:$$(cat .version)

docker-push-fe:
	@echo "==> Pushing Frontend image ($$(cat .version))"
	@docker push $(REGISTRY)/frontend:$$(cat .version)

docker-push-cc:
	@echo "==> Pushing Claude Code image ($$(cat .version))"
	@docker push $(REGISTRY)/cc:$$(cat .version)

docker-build-ow:
	@echo "==> Building Output Watcher image ($$(cat .version))"
	@docker build -f docker/output-watcher/Dockerfile \
		-t $(REGISTRY)/output-watcher:$$(cat .version) .

docker-push-ow:
	@echo "==> Pushing Output Watcher image ($$(cat .version))"
	@docker push $(REGISTRY)/output-watcher:$$(cat .version)

## Build + Push all images in one step (auto bumps version)
docker-all: docker-build docker-push

# ============================================================
# Helm Deploy
# ============================================================

## Deploy with Helm (first install)
helm-deploy:
	@echo "==> Deploying SAC with Helm (tag: $$(cat .version))"
	@KUBECONFIG=$(KUBECONFIG) helm install sac helm/sac \
		--namespace $(NAMESPACE) --create-namespace \
		--set global.registry=$(REGISTRY) \
		--set apiGateway.image.tag=$$(cat .version) \
		--set wsProxy.image.tag=$$(cat .version) \
		--set frontend.image.tag=$$(cat .version)

## Upgrade existing Helm release
helm-upgrade:
	@echo "==> Upgrading SAC with Helm (tag: $$(cat .version))"
	@KUBECONFIG=$(KUBECONFIG) helm upgrade sac helm/sac \
		--namespace $(NAMESPACE) \
		--set global.registry=$(REGISTRY) \
		--set apiGateway.image.tag=$$(cat .version) \
		--set wsProxy.image.tag=$$(cat .version) \
		--set frontend.image.tag=$$(cat .version)

## Dry-run to preview Helm templates
helm-dry-run:
	@KUBECONFIG=$(KUBECONFIG) helm install sac helm/sac \
		--namespace $(NAMESPACE) --create-namespace \
		--dry-run --debug

## Uninstall Helm release
helm-uninstall:
	@echo "==> Uninstalling SAC Helm release"
	@KUBECONFIG=$(KUBECONFIG) helm uninstall sac --namespace $(NAMESPACE)

## Update Helm chart dependencies
helm-dep-update:
	@echo "==> Updating Helm dependencies"
	@helm dependency update helm/sac

# ============================================================
# Dev Targets
# ============================================================

## Restart a single service (usage: make restart SVC=api-gateway|ws-proxy|frontend)
restart:
	@case "$(SVC)" in \
		api-gateway|api) \
			pid=$$(ss -tlnp | grep ":$(API_PORT) " | sed -n 's/.*pid=\([0-9]*\).*/\1/p'); \
			[ -n "$$pid" ] && kill -9 $$pid 2>/dev/null; \
			$(MAKE) build-api --no-print-directory; \
			cd $(BACKEND) && nohup ./bin/api-gateway > /tmp/sac-api-gateway.log 2>&1 & echo "API Gateway restarted (PID: $$!)"; \
			;; \
		ws-proxy|ws) \
			pid=$$(ss -tlnp | grep ":$(WS_PORT) " | sed -n 's/.*pid=\([0-9]*\).*/\1/p'); \
			[ -n "$$pid" ] && kill -9 $$pid 2>/dev/null; \
			$(MAKE) build-ws --no-print-directory; \
			cd $(BACKEND) && nohup ./bin/ws-proxy > /tmp/sac-ws-proxy.log 2>&1 & echo "WS Proxy restarted (PID: $$!)"; \
			;; \
		frontend|fe) \
			pid=$$(ss -tlnp | grep ":$(VITE_PORT) " | sed -n 's/.*pid=\([0-9]*\).*/\1/p'); \
			[ -n "$$pid" ] && kill -9 $$pid 2>/dev/null; \
			cd $(FRONTEND) && nohup npm run dev -- --host 0.0.0.0 > /tmp/sac-frontend.log 2>&1 & echo "Frontend restarted (PID: $$!)"; \
			;; \
		*) echo "Usage: make restart SVC=api-gateway|ws-proxy|frontend" ;; \
	esac

# ============================================================
# Protobuf Code Generation
# ============================================================

PROTO_DIR   := backend/proto
PROTO_FILES := $(shell find $(PROTO_DIR)/sac -name '*.proto' 2>/dev/null)
GO_GEN_DIR  := backend/gen
TS_GEN_DIR  := frontend/src/generated

## Generate Go code from proto files (messages + gRPC services + gRPC-gateway)
proto-go:
	@rm -rf $(GO_GEN_DIR) && mkdir -p $(GO_GEN_DIR)
	@protoc -I$(PROTO_DIR) \
		--go_out=$(GO_GEN_DIR) --go_opt=paths=source_relative \
		--go-grpc_out=$(GO_GEN_DIR) --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=$(GO_GEN_DIR) --grpc-gateway_opt=paths=source_relative \
		$(PROTO_FILES)
	@echo "==> Go proto generated"

## Generate TypeScript types from proto files
proto-ts:
	@rm -rf $(TS_GEN_DIR) && mkdir -p $(TS_GEN_DIR)
	@protoc -I$(PROTO_DIR) \
		--plugin=./frontend/node_modules/.bin/protoc-gen-ts_proto \
		--ts_proto_out=$(TS_GEN_DIR) \
		--ts_proto_opt=onlyTypes=true,snakeToCamel=false,useOptionals=messages,useDate=string,outputJsonMethods=false,outputEncodeMethods=false,outputClientImpl=false \
		$(PROTO_FILES)
	@echo "==> TypeScript proto generated"

## Generate both Go and TypeScript from proto files
proto: proto-go proto-ts

# ============================================================
# Testing
# ============================================================

## Run all tests
test:
	@echo "==> Running tests"
	@cd $(BACKEND) && go test ./... -v

## Run tests with coverage report
test-coverage:
	@echo "==> Running tests with coverage"
	@cd $(BACKEND) && go test ./... -coverprofile=coverage.out
	@cd $(BACKEND) && go tool cover -html=coverage.out -o coverage.html
	@echo "==> Coverage report: backend/coverage.html"

## Run tests with verbose output
test-verbose:
	@echo "==> Running tests (verbose)"
	@cd $(BACKEND) && go test ./... -v -count=1

## Run tests for specific package (usage: make test-pkg PKG=auth)
test-pkg:
	@echo "==> Testing package: $(PKG)"
	@cd $(BACKEND) && go test ./internal/$(PKG)/... -v
