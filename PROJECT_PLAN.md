# wukong 监控系统 部署方案

> 版本: v0.1.0
> 创建日期: 2026-06-21 09:49 (北京时间)
> 状态: **骨架搭建完成**，三个二进制编译通过，待 Phase 1 功能完善

## 一、项目概述

wukong 监控是一个类似哪吒探针的服务器探针系统，实时探测服务器状态，gRPC 双向流通信，单二进制极简部署。整个系统包括：

- **主控（server）**：Go 单二进制，embed Vue3 前端，cmux 单端口同时服务 gRPC（探针通道）和 HTTP（Web API + SSE + 前端静态资源）
- **探针（agent）**：Go 单二进制，采集 CPU/内存/磁盘/网络/Ping，gRPC 上报，本地 10min 缓冲
- **签名服务（signer）**：ed25519 独立进程，私钥与 web 后端物理隔离，Unix Socket 通信

## 二、技术栈

| 层面 | 技术 | 说明 |
|------|------|------|
| 后端 | Go 1.22+ | 单二进制，cmux 双协议 |
| 前端 | Vue3 + Element Plus + ECharts | Go embed 进单二进制，暗黑科技风双主题 |
| 存储 | SQLite | WAL 模式，按小时分表，1 分钟预聚合 |
| 探针 | Go | gopsutil 采集，gRPC 上报 |
| 通信 | gRPC 双向流 | 探针个体凭证认证，指令 ed25519 签名 |
| 部署 | systemd + nginx | 裸跑反代 |

## 三、部署架构

```
浏览器 ──HTTPS 443──→ nginx ──┬→ proxy_pass http://127.0.0.1:64443 (Web REST/SSE)
                              └→ grpc_pass  127.0.0.1:64443 (gRPC双向流)
                                    │ cmux 同端口区分 HTTP/1.1 与 HTTP/2
                                    ▼
                              主控 wukong-server
                              ├── SQLite (/opt/wukong/data/wukong.db)
                              ├── 内存 agents_latest map (SSE源)
                              └── Unix Socket → 签名服务 signer
                                    ▲ gRPC over TLS (个体凭证认证, 指令需签名验签)
                              探针 N 台 (各 /opt/wukong/agent/)
```

## 四、核心安全架构

1. **双层鉴权**：Web 后台走 JWT+TOTP（管理员），探针走个体凭证（agent_id + agent_secret）+ 指令签名
2. **签名私钥隔离**：ed25519 私钥在 signer 进程中，web 后端只能通过 Unix Socket 请求签名，无法直接拿私钥
3. **指令白名单**：探针只接受白名单内的签名指令（更新配置、重启探针），不执行任意 shell
4. **一次性安装 token**：30 分钟有效，注册即作废，防范未授权注册
5. **二进制签名**：安装脚本和探针二进制用 B2 私钥签名，防中间人攻击

## 五、测试验证

```bash
# 启动主控
export WUKONG_ADMIN_PASSWORD='<bcrypt hash>'
./build/wukong-server --config /opt/wukong/wukong.conf

# 验证 API
curl http://127.0.0.1:64443/api/health
# → {"status":"ok","version":"0.1.0"}

# 启动签名服务
./build/wukong-signer --socket /opt/wukong/data/signer.sock

# 启动探针
./build/wukong-agent --config /opt/wukong/agent/agent.conf
```

## 六、目录布局

```
/opt/wukong/
├── wukong                # 主控二进制
├── wukong.conf           # 主控配置
├── data/
│   ├── wukong.db         # SQLite 数据库
│   ├── uploads/          # Logo 等上传文件
│   ├── signing/          # ed25519 密钥对（权限 400）
│   └── signer.sock       # 签名服务 Unix Socket
├── agent/
│   ├── wukong-agent      # 探针二进制
│   ├── agent.conf        # 探针配置（权限 600）
│   └── data/             # 探针本地数据
└── deploy/
    ├── nginx/            # nginx 反代配置
    ├── systemd/          # systemd unit 文件
    └── scripts/          # 安装脚本
```