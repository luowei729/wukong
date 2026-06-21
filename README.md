# wukong 监控系统

> 类似哪吒探针的服务器监控系统：实时探测服务器状态，gRPC 双向流通信，单二进制极简部署。
> 架构与决策详见 [PROJECT_PLAN.md](./PROJECT_PLAN.md)。

## 快速开始

```bash
# 主控安装（Ubuntu/Debian）
curl -fsSL https://你的域名/api/install-server.sh | bash

# 节点安装（在后台复制一次性 token 命令）
curl -fsSL https://你的域名/api/install-agent.sh -k token-xxxxxxxx
```

## 技术栈

- **后端/探针**：Go（单二进制，共享 proto）
- **前端**：Vue3 + Element Plus + ECharts（Go embed 进单二进制）
- **存储**：SQLite（WAL + 按小时分表 + 预聚合）
- **部署**：/opt/wukong，systemd 裸跑，nginx 反代 443

## 目录结构

```
wukong/
├── cmd/                    # 入口
│   ├── server/             # 主控入口
│   ├── agent/              # 探针入口
│   └── signer/             # 签名服务入口
├── internal/               # 内部包
│   ├── config/             # 配置加载
│   ├── store/              # 存储层(MetricsStore接口+SQLite实现)
│   ├── grpcapi/            # gRPC server(探针通道)
│   ├── webapi/             # Web API(REST+SSE)
│   ├── signer/             # 签名客户端
│   ├── alert/              # 告警引擎
│   ├── notify/             # 通知渠道(Telegram等)
│   ├── auth/               # JWT+2FA鉴权
│   └── agentcore/          # 探针核心(采集+gRPC client+缓冲)
├── proto/                  # gRPC proto 定义
├── web/                    # Vue3 前端源码
├── deploy/                 # 部署文件
│   ├── nginx/              # nginx 反代配置
│   ├── systemd/            # systemd unit
│   └── scripts/            # 安装脚本
└── Makefile
```
