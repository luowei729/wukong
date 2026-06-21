# wukong 监控系统 部署方案

> 版本: v0.1.0
> 创建日期: 2026-06-21 09:49 (北京时间)
> 最后更新: 2026-06-21 14:10 (北京时间)
> 状态: **公开首页、安装 token、在线二进制下载、站点域名保存固化已修复；本机节点已通过 server.lkz.pub:64443 加入远程主控**

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

## 七、2026-06-21 11:57（北京时间）首页白屏修复记录

### 改动前总结
部署后首页返回 HTML，但浏览器显示空白。无头 Chrome 复现显示 Vue 已挂载但 `#app` 内容为空；网络检查发现 Vite 生成的 `_plugin-vue_export-helper-*.js` 下划线资源返回 404。

### 改动后总结
Go embed 从 `dist/*` 调整为 `all:dist`，保证下划线开头资源进入单二进制；新增 SPA 静态处理器，刷新前端 history 路由时回退到 `index.html`，但缺失的 JS/CSS 仍返回 404，方便后续排查真实资源问题。

### 验证要求
- `/assets/_plugin-vue_export-helper-*.js` 返回 200。
- 无头 Chrome 打开首页后 DOM 出现“管理员登录 / 用户名 / 密码”。
- `/dashboard` 刷新返回前端入口，交给 Vue Router 鉴权跳转登录。

## 八、2026-06-21 12:39（北京时间）公开首页与安装命令修复记录

### 改动前总结
系统首页默认进入后台登录页，未登录无法看到服务器状态；安装命令在没有配置域名时仍可复制占位命令，并错误使用 `curl -k token`，导致 token 没有传到安装脚本。

### 改动后总结
- `/` 改为公开服务器状态首页，未登录可访问。
- `/server/:id` 新增公开服务器详情页，可从首页服务器卡片点击进入。
- 新增 `/api/public/servers*` 脱敏只读接口，管理接口仍保持 JWT 鉴权。
- 修复最新指标查询的 `UpdatedAt`，公开页面可展示最近更新时间和数据延迟状态。
- 安装命令必须先配置 `site_domain`；正确格式为 `curl -fsSL "https://域名/api/install-agent.sh?k=<token>" | bash`。
- 安装脚本缺少 `?k=` 时直接报错，避免生成空 TOKEN 脚本。

### 验证要求
- 未登录打开 `/` 能看到公开首页，不跳 `/login`。
- 未登录打开 `/server/:id` 能看到公开详情或友好空态。
- 未登录打开 `/dashboard` 必须跳 `/login?redirect=/dashboard`。
- 未设置 `site_domain` 时后台不能复制安装命令。
- 设置站点地址后生成的安装脚本中 `TOKEN` 非空。
- 本机探针使用生成的 token 能注册进系统，不能再出现 `token is malformed`。

### 验证结果
- `go test ./...` 已通过。
- `cd web && npm run build` 已通过，Vite chunk size warning 不影响构建。
- 本地主控 `127.0.0.1:18080` 启动成功，真实登录接口可签发 JWT。
- 未配置 `site_domain` 时，安装 token 接口返回 `ready=false`、`script_url=""`，前端无可复制命令。
- 设置 `site_domain=http://127.0.0.1:18080` 后，安装命令为 `curl -fsSL "http://127.0.0.1:18080/api/install-agent.sh?k=token-..." | bash`，不再使用错误的 `curl -k token`。
- 请求安装脚本后确认 `TOKEN="token-..."` 非空，`SERVER_ADDR="127.0.0.1:18080"` 正确。
- 本机探针使用生成的 token 注册成功，节点 `home-pc` 已加入系统并在线；主控和探针日志未出现 `token is malformed`。
- 无头 Chrome 打开 `/`、`/server/:id`、`/dashboard` 均非白屏；`/dashboard` 未登录会显示登录页，公开首页和公开详情未登录可访问。
- 公开 API `/api/public/servers` 返回 1 台本机节点，不包含 `secret`、`token` 等敏感字段；未登录访问 `/api/agents` 仍返回 401。

## 九、2026-06-21 13:32（北京时间）在线安装探针二进制下载 401 修复记录

### 改动前总结
安装脚本已经能通过 `/api/install-agent.sh?k=<token>` 拿到非空 TOKEN，但脚本继续下载 `/api/agent/binary/latest/$ARCH` 时，该二进制下载路由仍被 JWT 鉴权中间件拦截，远程服务器会返回 401 并提示“探针二进制下载失败”。

### 改动后总结
- `/api/agent/binary/{version}/{arch}` 改为无需 JWT 的只读二进制下载接口，仅允许 `amd64` 和 `arm64`。
- Docker 镜像构建时同时编译 `wukong-agent-amd64` 与 `wukong-agent-arm64`，运行镜像内置到 `/opt/wukong/bin/`。
- 下载接口只从固定发布目录读取 agent 二进制，不读取任意路径，不返回管理配置、token 或 secret。

### 验证结果
- `go test ./...` 已通过。
- 本地 `GET /api/agent/binary/latest/amd64` 返回 `HTTP/1.1 200 OK`，响应体为 ELF 二进制，不再返回 401。


```
/opt/wukong/
├── wukong                # 主控二进制
├── wukong-signer         # 签名服务二进制
├── wukong.conf           # 主控配置
├── deploy/
│   ├── nginx/wukong.conf # nginx 反代配置
│   └── scripts/
│       ├── install-server.sh  # 主控安装脚本
│       └── install-agent.sh   # 探针安装脚本
├── data/
│   ├── wukong.db         # SQLite 数据库
│   ├── uploads/          # Logo 等上传文件
│   ├── signing/          # ed25519 密钥对（权限 400）
│   ├── signer.sock       # 签名服务 Unix Socket
│   └── .admin_password   # 初始密码（首次登录后删除）
└── agent/                # （仅探针节点）
    ├── wukong-agent      # 探针二进制
    ├── agent.conf        # 探针配置（权限 600）
    ├── data/             # 探针本地数据
    └── server.txt        # 主控地址
```

## 十、2026-06-21 14:10（北京时间）远程探针 gRPC 地址与本机节点加入验证

### 改动前总结
`site_domain=https://server.lkz.pub` 能让 Web/API 与探针二进制下载正常工作，但安装脚本会把探针 gRPC 地址自动推导为 `server.lkz.pub:443`。实测该地址当前 gRPC 注册超时，说明 443 反代尚未支持探针 gRPC；而 Docker 主控已暴露 `64443`，直连 `server.lkz.pub:64443` 可以完成注册和上报。

### 改动后总结
- 新增 SQLite 设置项 `agent_server_addr`，后台设置页可填写“探针 gRPC 地址”，格式必须为 `host:port`。
- `site_domain` 继续负责安装脚本 URL 和二进制下载 BASE_URL；`agent_server_addr` 负责脚本内 `SERVER_ADDR`，未配置时才回退按站点域名推导。
- 安装 token 接口会预先校验探针 gRPC 地址，格式错误时返回 `ready=false`，避免用户复制不可用命令。
- 本机临时探针已使用 `server.lkz.pub:64443` 注册到远程主控并上报指标，公开 API 与后台 API 均可看到该节点。

### 验证结果
- `server.lkz.pub:443` TCP 可达但 gRPC 注册超时。
- `server.lkz.pub:64443` gRPC 注册成功，节点名 `home-pc-server-lkz-e2e`。
- `https://server.lkz.pub/api/public/servers` 返回 1 台在线节点，包含 CPU/内存/磁盘/流量指标且不含敏感字段。
- 后台 `/api/agents` 和 `/api/agents/latest` 均可看到本机节点和最新指标。
- `go test ./...` 通过。
- `cd web && npm run build` 通过。

