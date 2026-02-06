# SAC 平台待办事项清单

## 🔴 高优先级（下次首先处理）

### 1. ~~数据库连接（阻塞性问题）~~ ✅ 已解决
- [x] 选择数据库访问方案：方案A - VPC网络访问阿里云 RDS
- [x] 运行数据库迁移: `./bin/migrate -action=up`
- [x] 填充种子数据: `./bin/migrate -action=seed`
- [x] 验证数据库连接和表结构
- **数据库连接**: `pgm-uf68x0dfyoth4u5g.pg.rds.aliyuncs.com:5432/sandbox`

### 2. ~~本地服务测试~~ ✅ 已完成
- [x] 启动 API Gateway: 运行在 0.0.0.0:8080 (PID: 1021221) ✅
- [x] 启动 WebSocket Proxy: 运行在 0.0.0.0:8081 (PID: 1022700) ✅
- [x] 配置远程调试: 所有服务监听 0.0.0.0 ✅
- [x] 测试 Skill API: `curl http://localhost:8080/api/skills` ✅
- [x] 测试 Agent API (CRUD): 创建/删除/列表全部正常 ✅
- [x] 测试 Agent ANTHROPIC config: JSONB 存储成功 ✅
- [x] 启动前端: `npm run dev` 运行在 0.0.0.0:5173 ✅
- [ ] 在浏览器中测试完整流程（前后端集成测试）

### 2.1 前端UI改进 ✅ 已完成 (2026-02-06)
- [x] 在顶部 Header 添加 Agent 快速切换下拉选择器
- [x] 实现与左侧 AgentSelector 的双向联动
- [x] 移除前端 Agent 接口中废弃的 system_prompt 和 model 字段
- [x] 更新所有组件以使用新的 Agent 数据结构
- [x] Installed Skills 标签页默认选中第一个 category tab

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

~~**数据库访问问题已解决** ✅~~
- 正确的端口是 5432（不是1921）
- 数据库名是 sandbox（不是sac）
- VPC网络已配置，迁移和种子数据已成功执行

**下次工作第一步**: 前后端集成测试 & Docker 镜像构建

---

## 已完成 ✅

### 基础设施
- [x] 后端项目结构和 Go modules
- [x] 数据库模型定义（bun ORM）
- [x] 数据库连接实现 + 迁移和种子数据
- [x] Kubernetes Pod 管理器（支持ANTHROPIC环境变量注入）
- [x] WebSocket 代理服务
- [x] Docker 镜像定义
- [x] Kubernetes 部署清单
- [x] 数据库迁移工具

### 后端 API
- [x] Skill Registry API (CRUD + 分享/Fork)
- [x] Agent API (CRUD + 最多3个限制)
- [x] Agent-Skill 关联 API (Install/Uninstall)
- [x] API Gateway 服务（8080端口，PID: 987724）
- [x] WebSocket Proxy（8081端口）

### 前端实现
- [x] Vue 3 项目初始化 + Naive UI
- [x] Terminal 组件（xterm.js + WebSocket）
- [x] Skill Panel 组件（已安装技能面板）
- [x] Skill Register 组件（技能市场和编辑器）
- [x] Agent Selector 组件（选择Agent）
- [x] Agent Creator 组件（创建/编辑Agent）
- [x] 前端开发服务器（5173端口）

### Agent 架构重构（2026-02-06）
- [x] 移除 Agent 的 `model` 和 `system_prompt` 字段
- [x] 使用 ANTHROPIC 环境变量配置（AUTH_TOKEN, BASE_URL, HAIKU/OPUS/SONNET_MODEL）
- [x] Agent.config JSONB 存储 ANTHROPIC 配置
- [x] Container Manager 自动将配置转换为 Pod 环境变量
- [x] 前端 AgentCreator 只需配置 ANTHROPIC 参数
- [x] Skill 作为唯一的"个性"定义方式（不需要额外的 System Prompt）
- [x] **修复 JSONB 序列化问题**: AgentConfig.Value() 返回 string 而非 []byte
- [x] 数据库迁移: 删除 system_prompt 和 model 列
- [x] 后端 API 测试通过: 创建/删除/查询 Agent 成功

### 文档
- [x] 项目文档（README, DEPLOYMENT, TESTING, IMPLEMENTATION_SUMMARY）
- [x] Git 提交和推送（commit: 53805b1）
- [x] MEMORY.md 项目记忆维护

---

## 备注

- 所有代码已提交到 git: `g.echo.tech:dev/sac.git`
- 最新 Commit: `06d35e3` - "docs: add TODO list and update project memory"
- 总计: 57+ 个文件，8,500+ 行代码
- 数据库: `pgm-uf68x0dfyoth4u5g.pg.rds.aliyuncs.com:5432/sandbox`
- 数据库密码: `4SOZfo6t6Oyj9A==`
- Kubeconfig: `/root/workspace/code-echotech/sac/kubeconfig.yaml`
- API Gateway PID: 987724 (运行中)
- WebSocket Proxy: 8081端口
- 前端开发服务器: 5173端口

## 当前架构亮点

1. **Agent = ANTHROPIC配置 + Skills组合**
   - Agent 只存储 API 连接配置（token, base_url, models）
   - Agent 行为完全由安装的 Skills 决定
   - 无需预定义 System Prompt，灵活性最大化

2. **每用户最多3个Agent**
   - 数据分析 Agent（安装 SQL + 可视化 Skills）
   - 代码助手 Agent（安装 代码生成 + Review Skills）
   - 通用助手 Agent（安装各种通用 Skills）

3. **Skill作为核心抽象**
   - Markdown 格式提示词
   - 可分享、可Fork
   - 按类别组织（数据查询、数据分析、报表生成等）
