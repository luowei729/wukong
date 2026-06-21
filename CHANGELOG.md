# wukong 监控系统 变更日志

所有变更记录使用北京时间（UTC+8）。

## [2026-06-21 09:49] - 项目初始化：完整骨架搭建 + 可编译单二进制

### 改动前总结
wukong 监控是一个全新项目，仓库内仅包含另一个量化交易项目的演示文档模板（PROJECT_PLAN.md / CHANGELOG.md / DEPLOY_CREDENTIALS.md），README 只有一行，无实际代码。项目定位为类似哪吒探针的服务器监控系统。

### 改动后总结
**1. 21 项架构决策已完成（grill-me 方式逐项确认）**
- 通信：gRPC 双向流（方案B）+ B2 指令白名单公钥验签 + 窄档（仅更新配置/重启探针）
- 后端：Go 单二进制（cmux 单端口 64443 双协议）
- 前端：Vue3 + Element Plus + ECharts，Go embed 进单二进制，暗黑科技风双主题
- 存储：SQLite（WAL + 按小时分表 + ping_agg_1min 预聚合）+ 内存 latest map，不设硬限
- 部署：/opt/wukong，systemd 裸跑 + nginx 反代（`deploy/nginx/wukong.conf`）
- 探针：单二进制，gopsutil 采集，gRPC 上报，本地 10min 缓冲
- 鉴权：JWT + TOTP 2FA + 登录限流
- 告警：6 指标 + 阈值 + 持续判定 + 三级回退 + 滞回防抖 + 抑制期 + 恢复通知 + 静默
- Telegram：多机器人按分组路由 + Notifier 接口预留多渠道
- 注册：一次性 token（30 分钟过期），注册后发个体 agent_secret
- 升级：主控 web 一键升级 + 探针目标版本自检 + 自动回滚
- 签名服务：ed25519 独立进程，私钥与 web 后端物理隔离

**2. 项目骨架已搭建完毕，全部编译通过**
```
wukong/
├── cmd/
│   ├── server/main.go       # 主控入口 (Go embed Vue3)
│   ├── agent/main.go        # 探针入口
│   └── signer/main.go       # 签名服务入口
├── internal/
│   ├── config/config.go     # 配置加载
│   ├── store/
│   │   ├── store.go         # MetricsStore 接口
│   │   └── sqlite.go        # SQLite 实现 (715行)
│   ├── grpcapi/agent_server.go  # gRPC 探针通道
│   ├── webapi/
│   │   ├── handler.go       # Web 路由 (40+ 端点)
│   │   ├── handlers.go      # 请求处理 (676行)
│   │   ├── embed.go         # Go embed 占位
│   │   ├── nginx.go         # nginx 配置生成
│   │   └── dist/            # Vue3 构建产物
│   ├── auth/auth.go         # JWT+2FA 鉴权
│   ├── signer/service.go    # ed25519 签名
│   ├── alert/engine.go      # 告警引擎
│   ├── notify/notify.go     # Telegram+接口
│   └── agentcore/
│       ├── agent.go         # 探针核心 (347行)
│       └── collector.go     # 系统采集器
├── proto/
│   ├── wukong.proto         # gRPC 协议定义
│   └── gen/                 # 生成的 pb Go 代码
├── web/                     # Vue3 源码 (6页面)
├── deploy/
│   ├── nginx/wukong.conf    # nginx 反代配置
│   ├── systemd/             # systemd unit 文件
│   └── scripts/             # 安装脚本生成逻辑
└── Makefile                 # 构建 + 交叉编译
```

**3. 验证结果**
- `wukong-server` 24MB：cmux 监听 64443，REST API 返回正常，前端页面可访问，auth 中间件正确拦截未授权请求
- `wukong-agent` 15MB：gRPC client + 采集器 + 10min 缓冲，可独立编译
- `wukong-signer` 15MB：ed25519 签名服务，Unix Socket 通信

**4. nginx 配置已生成**
- `deploy/nginx/wukong.conf`：443→64443 反代，gRPC `grpc_pass`，SSE `proxy_buffering off`
- `internal/webapi/nginx.go`：编程化生成 nginx 配置

### 涉及文件
全部新增文件，约 4000+ 行代码。

### 待完善
- Web API 登录/2FA/主题/上传等部分路由端点需完善请求体解析
- 探针本地缓冲 flush、Ping 多运营商探测、自动升级逻辑需补充
- 告警引擎需要集成到 gRPC 心跳判定中
- 前端需要接入真实 API 数据替换模拟数据