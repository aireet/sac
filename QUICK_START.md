# 快速启动指南 - 回家后继续开发

## 一键启动脚本

### 1. 启动后端服务
```bash
#!/bin/bash
# 文件名: start-backend.sh

cd /path/to/sac/backend

# 数据库配置
export DB_HOST=pgm-uf68x0dfyoth4u5g.pg.rds.aliyuncs.com
export DB_PORT=5432
export DB_USER=sandbox
export DB_PASSWORD="4SOZfo6t6Oyj9A=="
export DB_NAME=sandbox
export KUBECONFIG=/path/to/kubeconfig.yaml

# 启动API Gateway (后台运行)
nohup go run ./cmd/api-gateway > /tmp/api-gateway.log 2>&1 &
echo "API Gateway started, PID: $!"

# 等待1秒
sleep 1

# 启动WebSocket Proxy (后台运行)
nohup go run ./cmd/ws-proxy > /tmp/ws-proxy.log 2>&1 &
echo "WebSocket Proxy started, PID: $!"

echo ""
echo "✅ 后端服务已启动"
echo "API Gateway: http://localhost:8080"
echo "WebSocket Proxy: ws://localhost:8081"
echo ""
echo "查看日志:"
echo "  tail -f /tmp/api-gateway.log"
echo "  tail -f /tmp/ws-proxy.log"
```

### 2. 启动前端
```bash
#!/bin/bash
# 文件名: start-frontend.sh

cd /path/to/sac/frontend
npm run dev
```

### 3. 停止所有服务
```bash
#!/bin/bash
# 文件名: stop-all.sh

# 停止API Gateway
pkill -f "go run ./cmd/api-gateway"

# 停止WebSocket Proxy
pkill -f "go run ./cmd/ws-proxy"

# 停止前端
pkill -f "vite"

echo "✅ 所有服务已停止"
```

## 快速测试命令

### 测试后端API
```bash
# 健康检查
curl http://localhost:8080/health

# 获取Agents
curl http://localhost:8080/api/agents | jq '.'

# 获取Skills
curl http://localhost:8080/api/skills | jq '.'

# 创建Session（需要agentID）
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"agent_id": 5}' | jq '.'
```

### 查看Kubernetes资源
```bash
export KUBECONFIG=/path/to/kubeconfig.yaml

# 查看所有Claude Code资源
kubectl -n sac get all -l app=claude-code

# 查看某个Agent的Pod日志
kubectl -n sac logs -f deployment/claude-code-1-5
```

## 当前测试数据

### Agents (数据库中)
- Agent ID=5: "Test Agent" (完整配置: haiku+opus+sonnet)
- Agent ID=6: "Code Assistant" (只有sonnet)

### Deployments (K8s中)
- `claude-code-1-5`: User 1 + Agent 5
- `claude-code-1-6`: User 1 + Agent 6

## 常见问题

### Q: 数据库连接失败
```bash
# 检查VPN是否连接
ping pgm-uf68x0dfyoth4u5g.pg.rds.aliyuncs.com

# 重新设置环境变量
export DB_HOST=pgm-uf68x0dfyoth4u5g.pg.rds.aliyuncs.com
export DB_PORT=5432
# ... 其他变量
```

### Q: Port already in use
```bash
# 查找占用端口的进程
lsof -i :8080
lsof -i :8081

# 杀死进程
kill -9 <PID>
```

### Q: Kubernetes连接失败
```bash
# 确认kubeconfig路径
export KUBECONFIG=/path/to/kubeconfig.yaml

# 测试连接
kubectl cluster-info
kubectl -n sac get pods
```

## 下一步开发建议

1. **前端集成** (1-2小时)
   - 在MainView中测试Agent选择自动创建Session
   - 验证Terminal组件WebSocket连接
   - 测试多个Agent之间的切换

2. **Agent生命周期管理** (2-3小时)
   - 实现Agent删除时清理Deployment
   - 实现Agent更新时重启Deployment
   - 添加Deployment健康状态检查

3. **前端优化** (1-2小时)
   - 添加Agent状态显示（Running/Creating/Error）
   - 优化Session管理UI
   - 添加错误处理和用户提示

4. **后端容器化** (2-3小时)
   - 构建Docker镜像
   - 推送到阿里云镜像仓库
   - 在K8s中部署

---
**创建时间**: 2026-02-06 晚
**适用场景**: 换电脑后快速恢复开发环境
