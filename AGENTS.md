# wukong 监控系统 - 开发规范与提示

> 最后更新: 2026-06-21 15:05 (北京时间)

## 开发原则

1. **使用 codegraph MCP 检索和 semantic_search 向量索引**来检索代码库
2. **调用智能体和 Worktree 并行工作**，确保阅读过项目所有 md
3. **维护项目的所有 md 文档**，有些文档内容可能过时要分辨
4. **要求代码里每步都要中文注释**，说明功能的实现和实现的原因，为后期排查问题和开发打好基础
5. **按照项目代码功能结构、功能区域划分规则**进行开发修改，不要擅自改变代码架构和功能划分结构
6. **先规划架构，不明白的细节先提问确认**再写代码，开发阶段可以打开无头 Chrome 访问主站页面调试验证
7. **每次写代码前先给"改动前总结"**，写完后给"改动后总结"
8. **每次更改变动要按照格式中文写入**项目根目录下的 AGENTS.md / CHANGELOG.md / DEPLOY_CREDENTIALS.md / PROJECT_PLAN.md，记录北京时间和日期
9. **把在本项目需要长期记住的开发提示**写到本文件的下方

## 目录结构

```
wukong/
├── cmd/
│   ├── server/        # 主控入口（Go embed Vue3）
│   ├── agent/         # 探针入口
│   └── signer/        # 签名服务入口
├── internal/
│   ├── config/        # 配置加载
│   ├── store/         # 存储层（MetricsStore 接口 + SQLite 实现）
│   ├── grpcapi/       # gRPC server（探针通道）
│   ├── webapi/        # Web API（REST + SSE + nginx 配置生成 + embed）
│   ├── signer/        # 签名客户端
│   ├── alert/         # 告警引擎
│   ├── notify/        # 通知渠道（Telegram + 接口）
│   ├── auth/          # JWT + 2FA 鉴权
│   └── agentcore/     # 探针核心（采集 + gRPC client + 缓冲）
├── proto/             # gRPC proto 定义 + 生成 Go 代码
├── web/               # Vue3 前端源码
├── deploy/
│   ├── nginx/         # nginx 反代配置
│   ├── systemd/       # systemd unit 文件
│   └── scripts/       # 安装脚本
├── build/             # 构建产物（.gitignore）
└── Makefile           # 构建 + 交叉编译
```

## 已确认的 21 项架构决策

| # | 分支 | 决策 |
|---|------|------|
| 1 | 通信架构 | **方案B gRPC 双向流**：探针主动连主控建长连接 |
| 2 | 节点安全 | **B2 指令白名单 + 预置公钥验签**：签名私钥与 web 后端物理隔离，web 被打穿拿不到私钥无法伪造指令 |
| 3 | 指令白名单 | **窄档（修订）**：仅①更新配置 ②重启探针进程。升级由探针主动自检完成 |
| 4 | 后端栈 | **Go 单二进制**（主控+探针同语言，共享 proto） |
| 5 | 前端栈 | **Vue3 + Element Plus + ECharts**，Go embed 进单二进制 |
| 6 | 存储 | **SQLite**（WAL + 按小时分表 + ping_agg_1min 预聚合 + DROP 清理）+ 内存 latest map |
| 7 | 部署目录 | **/opt/wukong**，主控单二进制 + 探针单二进制 |
| 8 | 规模 | **不设硬限**，全套优化 + 背压 + 预留 MetricsStore 接口 |
| 9 | 采集频率 | **默认 1s，后端可改**，D4 三级回退（探针 > 分组 > 全局）；公开首页和后台设备页也按 1s 轮询刷新 |
| 10 | 运营商 Ping | **E1 全局 IP 池 + 双模式**（默认 ICMP，回退 TCP） |
| 11 | 自定义主题 | **F3 预设 + CSS 变量微调 + Logo/站名/页脚**，全局一份，改后刷新 |
| 12 | 实时推送 | **SSE 增量帧**（浏览器自动重连，不通回退轮询） |
| 13 | 管理员鉴权 | **H2 单管理员 + bcrypt + TOTP 2FA + JWT**（access 15min/refresh 7d 可主动失效）+ 可选 IP 白名单 + 登录限流 |
| 14 | 安装 key | **一次性 token**：后台生成，30 分钟过期，用后作废。注册后发个体 agent_secret，gRPC 用个体凭证。不绑分组，后台手动命名分组 |
| 15 | 告警阈值 | **J3 固定 6 指标**（离线/CPU/内存/磁盘/Ping延时/Ping丢包）+ 阈值 + 持续 + 三级回退 + 滞回防抖 |
| 16 | 告警去重 | **抑制期 30min + 恢复通知 + 静默窗口 + alerts 表记录** |
| 17 | Telegram | **L2 多机器人按分组路由 + Notifier 接口抽象**预留多渠道，bot_token 加密存储 |
| 18 | 升级机制 | **N3 主控 web 一键升级**（确认→备份→验签→替换）+ **P3 探针目标版本自检**（主控设目标，探针 10min 自检下载验签替换）+ **自动回滚** |
| 19 | 高可用 | **Q1 单主控 + Q2 定时备份**（systemd timer 每 6h 在线备份 + 探针 10min 缓冲补传） |
| 20 | 端口与反代 | **M-1 主控单端口 64443 cmux 双协议 + nginx 443 统一反代**，TLS 全交 nginx |
| 21 | 前端 UI | **U3 双主题**（默认暗黑科技风 + 浅色可切），Element Plus 深度定制 |

## 开发提示

- **2026-06-21 09:30（北京时间）**：推 GitHub 前排除 DEPLOY_CREDENTIALS.md 和 .codegraph/。DEPLOY_CREDENTIALS.md 是本项目敏感凭证文档，不允许提交公网仓库；.codegraph/ 是本地索引，可重建。
- **2026-06-21 10:20（北京时间）**：补全 Docker 部署、GHCR 自动构建（GitHub Actions）。GitHub Actions 在 push main 或 tag v* 时自动构建并推送到 `ghcr.io/luowei729/wukong`。已实测拉取 GHCR 镜像并 docker run 验证成功。tldr: `docker run -p 64443:64443 -e WUKONG_ADMIN_PASSWORD=xxx -e WUKONG_JWT_SECRET=xxx ghcr.io/luowei729/wukong:latest`
- **2026-06-21 11:30（北京时间）**：`WUKONG_ADMIN_PASSWORD` 和 `WUKONG_JWT_SECRET` 不设时自动生成随机值并打印到日志。`randomHex` 改用 `crypto/rand`。管理员默认用户名为 `admin`。Docker 不加环境变量也能启动。
- **2026-06-21 11:57（北京时间）**：部署后首页白屏根因是 Go `//go:embed dist/*` 未嵌入 Vite 生成的 `_plugin-vue_export-helper-*.js` 下划线资源，浏览器动态加载登录页 404。静态资源嵌入必须用 `//go:embed all:dist`，并用 SPA fallback 支持 history 路由刷新；同次修复 `log.Println` 格式化占位符导致的 go vet 失败。
- **2026-06-21 12:39（北京时间）**：公开首页改为未登录可访问的服务器状态展示，公开详情路径为 `/server/:id`，后端新增 `/api/public/servers*` 脱敏只读接口；安装命令必须先配置 `site_domain`，token 必须通过 `?k=` 传给 `/api/install-agent.sh`，禁止再用 `curl -k token` 这种错误格式。
- **2026-06-21 12:55（北京时间）**：本机端到端验证通过：未配置 `site_domain` 时安装命令 `ready=false` 且 `script_url` 为空；配置 `http://127.0.0.1:18080` 后脚本内 `TOKEN` 非空、`SERVER_ADDR` 正确；本机探针 `home-pc` 使用生成 token 注册上线，日志无 `token is malformed`；无头 Chrome 验证 `/`、`/server/:id`、`/dashboard` 均非白屏，公开 API 不含 `secret`/`token`，管理 API 未登录仍 401。
- **2026-06-21 13:32（北京时间）**：在线安装脚本下载探针失败 401 的根因是 `/api/agent/binary/{version}/{arch}` 路由注释写无需鉴权但实际包了 JWT `authMiddleware`。已改成只读公开二进制下载接口，仅允许 `amd64`/`arm64`，并由 Docker 镜像内置 `/opt/wukong/bin/wukong-agent-amd64` 与 `wukong-agent-arm64`；本地验证 `/api/agent/binary/latest/amd64` 返回 200 ELF，不再 401。
- **2026-06-21 13:45（北京时间）**：配置必须写入 SQLite `settings` 表固化，不能只保存在前端或内存；`site_domain` 每次保存都要写库（允许保存为空来关闭安装命令复制），并检查 `SetSetting` 错误。前端读取 `/api/theme` 时必须用后端返回值覆盖输入框，避免清空或保存失败后显示旧值。
- **2026-06-21 14:10（北京时间）**：生产环境 `server.lkz.pub:443` 当前只验证 HTTP/API 正常，gRPC 注册会超时；`server.lkz.pub:64443` 直连 gRPC 注册和上报已验证成功。因此安装脚本的 Web 下载地址继续使用 `site_domain`，探针注册/上报地址改由 SQLite `agent_server_addr` 固化（格式必须 `host:port`），未配置时才回退按站点域名推导。
- **2026-06-21 14:43（北京时间）**：页面和 API 响应必须全站带 `Cache-Control: no-store, no-cache, must-revalidate, max-age=0`、`Pragma: no-cache`、`Expires: 0`，公开首页、公开详情页、后台总览、后台设备页每秒静默轮询并给请求加 `?_=${Date.now()}`；默认采集间隔改为 1 秒。管理员修改密码接口为 `PUT /api/auth/password`，必须 JWT 鉴权、校验当前密码，新密码 bcrypt hash 写入 SQLite `settings.admin_password_hash` 固化，登录前优先读取该设置。
- **2026-06-21 15:05（北京时间）**：生产要求探针只能通过 `server.lkz.pub:443` 连接，不再使用 `64443` 对外直连；探针客户端在目标端口为 443 时使用 TLS gRPC，经 nginx `listen 443 ssl http2` 的 `/wukong.AgentService/` 反代转发到本机 64443，其他端口仍保持明文 gRPC。后台 `agent_server_addr` 生产值应固化为 `server.lkz.pub:443`，安装脚本必须输出 `SERVER_ADDR="server.lkz.pub:443"`。Telegram 设置页不回显 token、不使用 password 类型并提供测试通知；告警阈值页必须显示离线阈值；告警中心空列表要返回/兜底成数组；agent 安装需支持 amd64/arm64，注册后退出并由 systemd 常驻和开机自启；服务器节点名称支持后台自定义修改。
- **开发后续优先级**：① 探针 Ping 多运营商探测完善 ② Web API 端点完整实现 ③ 告警引擎集成 gRPC 心跳 ④ 前端接入真实 API 数据 ⑤ 安装脚本与升级流程端到端原型

## 部署相关长期提示

- **部署目录**: `/opt/wukong/`，主控 wukong.conf 权限 600，signing/ 权限 400
- **nginx**: 443 → 64443 反代，gRPC 用 `grpc_pass`，SSE 必须 `proxy_buffering off`
- **密钥安全**: ed25519 私钥 /opt/wukong/data/signing/ed25519.key 权限 400，首次登录后删除 .admin_password
- **探针注册**: 一次性 token 30 分钟过期，注册即作废；注册后服务器下发 agent_secret 个体凭证
- **签名校验**: 安装脚本 web 后端不接触私钥，签名请求通过 Unix Socket 发送到 signer 进程
- **更新记录**: 2026-06-21 09:49 骨架完成；2026-06-21 10:05 完成安装脚本+部署文档+DEPLOY_CREDENTIALS.md；2026-06-21 10:20 补Docker/GHCR/GitHub Actions
- **Docker 部署**: `deploy/Dockerfile` multistage 全量编译（Vue3→Go→alpine 运行），直接暴露 64443，不依赖 nginx
- **GHCR 自动构建**: `.github/workflows/docker.yml` 在 push main 或 tag v* 时自动构建推 ghcr.io
- **快速运行**: `docker run -p 64443:64443 -e WUKONG_ADMIN_PASSWORD=xxx ghcr.io/wukong-monitor/wukong-server:latest`
- **docker compose**: `WUKONG_ADMIN_PASSWORD=xxx WUKONG_JWT_SECRET=xxx docker compose -f deploy/docker-compose.yml up -d`