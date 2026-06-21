# wukong 监控系统 - 开发规范与提示

> 最后更新: 2026-06-21 09:49 (北京时间)

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
| 9 | 采集频率 | **默认 5s，后端可改**，D4 三级回退（探针 > 分组 > 全局） |
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
- **2026-06-21 09:49（北京时间）**：项目完整骨架已搭建，21 项架构决策已 grill-me 确认。三个二进制全部编译通过：`wukong-server`（24MB，embed Vue3）、`wukong-agent`（15MB）、`wukong-signer`（15MB）。主控监听 64443，cmux 双协议（gRPC + HTTP），nginx 反代配置已生成到 `deploy/nginx/wukong.conf`。前端 6 页面（Login/Dashboard/Nodes/NodeDetail/Alerts/Settings），暗黑科技风双主题，已构建并 embed 进主控。
- **开发后续优先级**：① 探针 Ping 多运营商探测完善 ② Web API 端点完整实现 ③ 告警引擎集成 gRPC 心跳 ④ 前端接入真实 API 数据 ⑤ 安装脚本与升级流程端到端原型