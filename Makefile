.PHONY: dev stop build build-api build-ws kill-port frontend backend telepresence status

# Ports
API_PORT  := 8080
WS_PORT   := 8081
VITE_PORT := 5173

# Dirs
BACKEND  := backend
FRONTEND := frontend

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

build: build-api build-ws

build-api:
	@echo "==> Building API Gateway"
	@cd $(BACKEND) && go build -o bin/api-gateway ./cmd/api-gateway

build-ws:
	@echo "==> Building WS Proxy"
	@cd $(BACKEND) && go build -o bin/ws-proxy ./cmd/ws-proxy

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
