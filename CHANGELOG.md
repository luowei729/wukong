# xxxx项目 变更日志

所有变更记录使用北京时间（UTC+8）。

## [2026-06-21 09:30:42] - GitHub 首次完整推送准备：忽略敏感文件与本地索引

### 改动前总结
仓库当前只有初始提交，README 已扩展为 wukong 监控系统说明，源码、部署脚本、前端、Go 模块文件和项目文档均为未跟踪状态；DEPLOY_CREDENTIALS.md 明确写有“严禁 push 到公网”，.codegraph/ 为本地 MCP 索引目录。

### 改动后总结
新增 .gitignore，排除 DEPLOY_CREDENTIALS.md、.codegraph/、Go 构建产物、前端 node_modules/dist、SQLite/日志/临时文件与编辑器目录；同步记录本次推送准备，后续只提交可公开的源码、部署脚本、前端源码、项目说明和依赖清单。

### 验收
- 已确认 origin 指向 https://github.com/luowei729/wukong。
- 已确认当前分支为 main。
- 已确认 DEPLOY_CREDENTIALS.md 与 .codegraph/ 不进入暂存范围。

### 涉及文件
- .gitignore
- AGENTS.md
- CHANGELOG.md
- PROJECT_PLAN.md
- DEPLOY_CREDENTIALS.md（仅本地记录，不提交）

---

## [2026-06-12 00:58] - Phase 2 W12.2 完成：POST /system/trading-mode 切换 API + 审计

### 改动前总结 以下为演示 自己删除演示文案
W12.1 落地了 trading_mode 跨进程热切换的底层能力（runtime_mode helper + 工厂
reload + 订阅），但没有对外暴露切换入口。运维只能直接写 Redis 触发，不满足
PROJECT_PLAN 要求的"二次确认 / API key 验证 / 审计留痕"。

### 改动后总结
**1. api/routes/system.py 新增两个端点**
- `GET /system/trading-mode`：返回当前 effective mode / settings_mode /
  live_enabled_at / auto_fallback / risk limits，供前端 System.vue 卡片展示
- `POST /system/trading-mode`：核心切换入口，6 步流程：
  1. 入参 + mode 合法性校验
  2. 当前已是目标模式 → 直接返回 switched=false（幂等）
  3. 切 live 必须 confirm=="我确认实盘交易"（防误触）
  4. 切 live 必须 limits 在 (0, 100000] 范围
  5. 切 live 前临时建 BinanceAdapter mainnet 实例调 fetch_balance 验证 API key
  6. 写 PG trading_mode_history 审计记录（含 api_key_verified / verification_error）
  7. set_runtime_trading_mode 写 Redis + publish system:command 触发跨进程热切

**2. 安全字符串常量 LIVE_CONFIRM_PHRASE = "我确认实盘交易"**
- 前端必须中文完全匹配才放行
- 切回 paper 不要求 confirm（降级是安全操作）

**3. backend/main.py 启动时自动调 init_db**
- 修复历史问题：init_db 之前只在初次部署调一次，新增 ORM 表（如 TradingModeHistory）
  不会自动建
- create_all 幂等，已存在表自动跳过；失败仅 warning 不阻塞启动

### 验收（4 场景 + 审计）
- 容器重启 + 60s ERROR=0
- 缺 confirm 切 live → 400 拒绝 ✅
- 错 confirm 切 live → 400 拒绝 ✅
- 正确 confirm + 100/1000/200 limits 切 live → 200 成功 + API key 验证通过
  + adapter 热切到 BinanceAdapter + Redis runtime/live_enabled_at 写入 ✅
- 切回 paper（无需 confirm） → 200 成功 + adapter 切回 PaperAdapter ✅
- PG trading_mode_history 2 条审计记录（含 api_key_verified=t / reason）✅
- GET /trading-mode 返回 mode=live + live_enabled_at=2026-06-12T00:57:25.85+08:00 ✅

### 涉及文件
- backend/api/routes/system.py（+TradingModeSwitchRequest schema + GET/POST trading-mode）
- backend/main.py（启动调 init_db 自动建新表）

### 待实现（W12.3）
- 前端 System.vue 交易模式卡片 + 切换按钮 + 二次确认弹窗 + limits 输入

---

## [2026-06-12 00:54] - Phase 2 W12.1 完成：trading_mode 跨进程热切换基础设施

### 改动前总结
W11 完成 Level0 实盘风控后，需要实现"实盘启用按钮"。但 backend / risk 是独立容器
进程，settings 是各自内存副本，无法直接共享 trading_mode 切换状态。原工厂层
`get_adapter()` 启动时按 env 决定 paper/live 后缓存，无法运行时切换。

##########  演示以下省略