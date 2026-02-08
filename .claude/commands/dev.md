SAC 项目开发环境管理。通过项目根目录的 Makefile 驱动所有操作。

项目根目录: `/root/workspace/code-echotech/sac/`

## Makefile 目标说明

### 主要目标
- `make dev` — 一键启动完整开发环境。依次执行: 检查/连接 Telepresence → 编译后端 → 停止已有服务 → 启动全部三个服务 → 显示状态
- `make stop` — 停止所有服务（杀掉占用 8080、8081、5173 端口的进程）
- `make status` — 显示三个服务的运行状态（端口、进程名、PID）

### 单服务操作
- `make restart SVC=<name>` — 重启单个服务。SVC 可选值:
  - `api-gateway` 或 `api` — 重新编译并重启 API Gateway (:8080)
  - `ws-proxy` 或 `ws` — 重新编译并重启 WebSocket Proxy (:8081)
  - `frontend` 或 `fe` — 重启前端 Vite 开发服务器 (:5173)

### 日志查看
- `make logs SVC=<name>` — 查看指定服务的实时日志（tail -f），SVC 可选值同上
- 日志文件位置:
  - API Gateway: `/tmp/sac-api-gateway.log`
  - WS Proxy: `/tmp/sac-ws-proxy.log`
  - Frontend: `/tmp/sac-frontend.log`

### 编译目标
- `make build` — 编译全部后端二进制（api-gateway + ws-proxy）
- `make build-api` — 只编译 API Gateway
- `make build-ws` — 只编译 WS Proxy

### 基础设施
- `make telepresence` — 检查 Telepresence 连接状态，未连接则自动连接（使用 `../kubeconfig.yaml`）
- `make migrate-up` — 执行数据库迁移
- `make migrate-seed` — 执行数据库种子数据

## 服务端口
| 服务 | 端口 |
|------|------|
| API Gateway | 8080 |
| WS Proxy | 8081 |
| Frontend (Vite) | 5173 |

## 使用规则
1. 用户说"开始调试"或"启动开发环境" → 执行 `make dev`
2. 用户说"重启后端" → 执行 `make restart SVC=api && make restart SVC=ws`
3. 用户说"重启前端" → 执行 `make restart SVC=fe`
4. 用户说"停止服务" → 执行 `make stop`
5. 用户说"看日志" → 执行 `make logs SVC=<对应服务>`
6. 启动后如果某个端口显示 ✗，检查对应日志排查问题
7. 执行完毕后将 status 输出报告给用户
