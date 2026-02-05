# SAC 平台待办事项清单

## 🔴 高优先级（下次首先处理）

### 1. 数据库连接（阻塞性问题）
- [ ] 选择数据库访问方案：
  - [ ] 方案A: 配置 VPN/堡垒机访问阿里云 RDS 内网
  - [ ] 方案B: 部署公网可访问的测试 PostgreSQL
  - [ ] 方案C: 在 K8s 集群内部署 PostgreSQL StatefulSet
- [ ] 运行数据库迁移: `./bin/migrate -action=up`
- [ ] 填充种子数据: `./bin/migrate -action=seed`
- [ ] 验证数据库连接和表结构

### 2. 本地服务测试
- [ ] 启动 API Gateway: `cd backend && go run ./cmd/api-gateway`
- [ ] 启动 WebSocket Proxy: `cd backend && go run ./cmd/ws-proxy`
- [ ] 测试 Skill API: `curl http://localhost:8080/api/skills`
- [ ] 启动前端: `cd frontend && npm run dev`
- [ ] 在浏览器中测试完整流程

### 3. Docker 镜像构建
- [ ] 获取阿里云镜像仓库登录凭证
- [ ] 创建后端服务 Dockerfile (api-gateway, ws-proxy)
- [ ] 构建用户容器镜像: `cd docker/claude-code && docker build -t ...`
- [ ] 推送所有镜像到仓库

## 🟡 中优先级（本周内完成）

### 4. Kubernetes 部署
- [ ] 验证 kubeconfig 可用性
- [ ] 创建 namespace: `kubectl create namespace sac`
- [ ] 应用数据库 Secret: `kubectl apply -f k8s/secrets/db-secret.yaml`
- [ ] 部署后端服务: `kubectl apply -f k8s/deployments/`
- [ ] 部署 Istio 配置: `kubectl apply -f k8s/istio/`
- [ ] 验证 Pod 状态: `kubectl get pods -n sac`
- [ ] 测试 Ingress 访问

### 5. 端到端测试
- [ ] 测试 WebSocket 连接到用户 Pod
- [ ] 测试终端交互功能
- [ ] 测试 Skill 创建和执行
- [ ] 测试 Skill 分享和 Fork
- [ ] 测试参数化 Skill 执行
- [ ] 测试 Pod 自动创建

## 🟢 低优先级（后续迭代）

### 6. 认证系统
- [ ] 设计认证方案（JWT/OAuth2）
- [ ] 实现用户注册/登录
- [ ] 替换 mock auth middleware
- [ ] 添加 WebSocket 认证
- [ ] 实现 RBAC 权限控制

### 7. 监控和日志
- [ ] 部署 Prometheus + Grafana
- [ ] 配置应用 metrics 端点
- [ ] 设置告警规则
- [ ] 部署 ELK/Loki 日志聚合
- [ ] 创建监控面板

### 8. 生产优化
- [ ] 实现 Pod 生命周期管理（2小时闲置暂停，7天删除）
- [ ] 添加 API 速率限制
- [ ] 实现对话日志采集
- [ ] 优化前端 bundle 大小
- [ ] 添加单元测试
- [ ] 编写集成测试
- [ ] 性能和负载测试

### 9. 高级功能
- [ ] Skill 版本管理
- [ ] 对话历史回放
- [ ] 终端会话录制
- [ ] Skill 市场和评分
- [ ] 团队协作功能
- [ ] 管理后台
- [ ] 使用分析

### 10. 文档和运维
- [ ] API 文档（Swagger）
- [ ] 用户使用手册
- [ ] 运维 Runbook
- [ ] 灾难恢复演练
- [ ] CI/CD Pipeline
- [ ] 安全加固检查清单

---

## 当前阻塞问题

### 数据库访问
**问题**: 无法从本地连接到阿里云 RDS (`pgm-uf68x0dfyoth4u5g.pg.rds.aliyuncs.com:1921`)
**错误**: `dial tcp 10.18.105.166:1921: i/o timeout`
**原因**: RDS 在 VPC 内网，需要 VPN 或堡垒机访问
**影响**: 无法运行数据库迁移和后端服务测试

**下次工作第一步**: 解决数据库访问问题

---

## 已完成 ✅

- [x] 后端项目结构和 Go modules
- [x] 数据库模型定义（bun ORM）
- [x] 数据库连接实现
- [x] Kubernetes Pod 管理器
- [x] WebSocket 代理服务
- [x] Skill Registry API
- [x] API Gateway 服务
- [x] 前端 Vue 3 项目初始化
- [x] Terminal 组件（xterm.js）
- [x] WebSocket 服务模块
- [x] Skill Panel 组件
- [x] Skill Register 组件
- [x] Docker 镜像定义
- [x] Kubernetes 部署清单
- [x] 数据库迁移工具
- [x] 项目文档（README, DEPLOYMENT, TESTING, IMPLEMENTATION_SUMMARY）
- [x] Git 提交和推送

---

## 备注

- 所有代码已提交到 git: `g.echo.tech:dev/sac.git`
- Commit: `53805b1` - "feat: implement complete Claude Code Sandbox (SAC) platform"
- 总计: 57 个文件，8,250+ 行代码
- 数据库密码: `4SOZfo6t6Oyj9A==`
- Kubeconfig: `/root/workspace/code-echotech/sac/kubeconfig.yaml`
