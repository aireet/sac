# SAC 平台远程调试信息

**启动时间**: 2026-02-06 05:19
**服务器IP**: 192.168.12.60

---

## 🌐 服务访问地址

### 前端服务
- **URL**: http://192.168.12.60:5173
- **状态**: ✅ 运行中
- **框架**: Vite + Vue 3
- **热重载**: 已启用

### 后端 API Gateway
- **URL**: http://192.168.12.60:8080
- **健康检查**: http://192.168.12.60:8080/health
- **Skills API**: http://192.168.12.60:8080/api/skills
- **状态**: ✅ 运行中

### 后端 WebSocket Proxy
- **URL**: ws://192.168.12.60:8081
- **健康检查**: http://192.168.12.60:8081/health
- **WebSocket路径**: ws://192.168.12.60:8081/ws/:userId/:sessionId
- **状态**: ✅ 运行中

---

## 🔧 配置信息

### 前端环境变量 (.env)
```env
VITE_API_URL=http://localhost:8080/api
VITE_WS_URL=ws://localhost:8081
```

**注意**: 前端代码中的API地址使用相对路径，会自动适配远程访问。

### 后端监听配置
```
API Gateway:  0.0.0.0:8080
WS Proxy:     0.0.0.0:8081
Frontend:     0.0.0.0:5173
```

### 数据库连接
```
Host: pgm-uf68x0dfyoth4u5g.pg.rds.aliyuncs.com
Port: 5432
Database: sandbox
Status: ✅ Connected
```

---

## 🧪 快速测试

### 1. 测试后端健康状态
```bash
# API Gateway
curl http://192.168.12.60:8080/health
# 期望输出: {"status":"healthy"}

# WebSocket Proxy
curl http://192.168.12.60:8081/health
# 期望输出: {"status":"healthy"}
```

### 2. 测试 Skills API
```bash
curl http://192.168.12.60:8080/api/skills
# 返回技能列表JSON
```

### 3. 访问前端界面
浏览器打开: http://192.168.12.60:5173

---

## 📋 正在运行的服务

| 服务 | 端口 | 进程ID | 状态 |
|------|------|--------|------|
| Frontend Dev Server | 5173 | b30dd0e | ✅ Running |
| API Gateway | 8080 | b57d710 | ✅ Running |
| WebSocket Proxy | 8081 | be8ca11 | ✅ Running |

---

## 🛠️ 调试工具

### 查看服务日志
```bash
# API Gateway 日志
cat /tmp/claude-0/-root-workspace-code-echotech-sac/tasks/b57d710.output

# WebSocket Proxy 日志
cat /tmp/claude-0/-root-workspace-code-echotech-sac/tasks/be8ca11.output

# Frontend Dev Server 日志
cat /tmp/claude-0/-root-workspace-code-echotech-sac/tasks/b30dd0e.output
```

### 停止服务
如需停止服务，可以使用以下命令：
```bash
# 停止特定服务
kill <进程PID>

# 或者通过进程名
pkill -f api-gateway
pkill -f ws-proxy
pkill -f vite
```

### 重启服务
```bash
cd /root/workspace/code-echotech/sac/backend
./bin/api-gateway &    # 启动 API Gateway
./bin/ws-proxy &       # 启动 WebSocket Proxy

cd /root/workspace/code-echotech/sac/frontend
npm run dev &          # 启动前端
```

---

## 🔍 网络检查

### 验证端口监听
```bash
ss -tlnp | grep -E ':(8080|8081|5173)'
```

### 检查网络连通性
```bash
# 从客户端测试
ping 192.168.12.60
telnet 192.168.12.60 8080
telnet 192.168.12.60 8081
telnet 192.168.12.60 5173
```

---

## 🚀 已完成的功能

- ✅ 数据库连接和迁移
- ✅ 用户和技能种子数据
- ✅ API Gateway RESTful API
- ✅ WebSocket Proxy 代理
- ✅ 前端界面构建
- ✅ CORS 跨域配置
- ✅ 健康检查接口

---

## 📝 待测试功能

- [ ] WebSocket 双向通信
- [ ] Terminal 终端连接
- [ ] Skill 执行功能
- [ ] Pod 创建和管理
- [ ] 用户认证流程

---

## ⚠️ 注意事项

1. **防火墙**: 确保防火墙允许 5173、8080、8081 端口
2. **网络**: 确保客户端和服务器在同一网络或可路由
3. **安全**: 当前使用 mock 认证（userID=1），生产环境需要真实认证
4. **CORS**: 已配置允许所有来源（*），生产环境需要限制

---

**调试愉快！** 🎉
