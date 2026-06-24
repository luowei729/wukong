# wukong 监控系统构建文件
# 目标: 单二进制主控(embed前端) + 单二进制探针 + 签名服务
.PHONY: all build build-server build-agent build-signer build-frontend clean proto dev

# 版本号：优先使用 git tag，没有 tag 时使用日期格式（如 0.2.20260625）
# 每次编译时版本号自动递增，无需手动修改
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo "0.2.$(shell date -u '+%Y%m%d%H%M')")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)"

# 输出目录
OUTPUT := build

# Go 相关
GO := go
GOFLAGS := $(LDFLAGS)
CGO_ENABLED := 1  # SQLite 需要 CGO

# 前端相关
YARN := yarn
NPM := npm
WEB_DIR := web

all: build-frontend build

# ========================================
# 构建后端
# ========================================
build: build-server build-agent build-signer

build-server:
	@echo "===== 构建主控 ====="
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -o $(OUTPUT)/wukong-server ./cmd/server
	@echo "输出: $(OUTPUT)/wukong-server"

build-agent:
	@echo "===== 构建探针 ====="
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -o $(OUTPUT)/wukong-agent ./cmd/agent
	@echo "输出: $(OUTPUT)/wukong-agent"

build-signer:
	@echo "===== 构建签名服务 ====="
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -o $(OUTPUT)/wukong-signer ./cmd/signer
	@echo "输出: $(OUTPUT)/wukong-signer"

# ========================================
# 交叉编译
# ========================================
cross: cross-amd64 cross-arm64

cross-amd64:
	@echo "===== 交叉编译 amd64 ====="
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(OUTPUT)/wukong-server-linux-amd64 ./cmd/server
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(OUTPUT)/wukong-agent-linux-amd64 ./cmd/agent
	@echo "输出: $(OUTPUT)/wukong-*-linux-amd64"

cross-arm64:
	@echo "===== 交叉编译 arm64 ====="
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc $(GO) build $(GOFLAGS) -o $(OUTPUT)/wukong-server-linux-arm64 ./cmd/server
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc $(GO) build $(GOFLAGS) -o $(OUTPUT)/wukong-agent-linux-arm64 ./cmd/agent
	@echo "输出: $(OUTPUT)/wukong-*-linux-arm64"

# ========================================
# 构建前端
# ========================================
build-frontend:
	@echo "===== 构建前端 ====="
	cd $(WEB_DIR) && $(NPM) run build
	@echo "前端构建完成，产物已输出到 internal/webapi/dist/"

# ========================================
# Proto
# ========================================
proto:
	@echo "===== 生成 Proto 代码 ====="
	protoc --proto_path=proto --go_out=proto/gen --go_opt=paths=source_relative \
		--go-grpc_out=proto/gen --go-grpc_opt=paths=source_relative proto/wukong.proto
	@echo "Proto 生成完成"

# ========================================
# 开发
# ========================================
dev: build-frontend build
	@echo "===== 构建完成，启动主控 ====="
	./$(OUTPUT)/wukong-server

dev-frontend:
	@echo "===== 启动前端开发服务器 ====="
	cd $(WEB_DIR) && $(NPM) run dev

# ========================================
# 运行
# ========================================
run:
	@echo "===== 启动主控 ====="
	./$(OUTPUT)/wukong-server

# ========================================
# 测试
# ========================================
test:
	@echo "===== 运行测试 ====="
	CGO_ENABLED=$(CGO_ENABLED) $(GO) test ./... -v

# ========================================
# 清理
# ========================================
clean:
	@echo "===== 清理 ====="
	rm -rf $(OUTPUT)/
	rm -rf $(WEB_DIR)/dist/
	@echo "清理完成"

# ========================================
# 依赖
# ========================================
deps:
	@echo "===== 安装 Go 依赖 ====="
	$(GO) mod tidy
	@echo "===== 安装前端依赖 ====="
	cd $(WEB_DIR) && $(NPM) install
	@echo "依赖安装完成"

# ========================================
# 生成 nginx 配置（开发环境）
# ========================================
nginx-conf:
	@echo "===== 生成 nginx 配置 ====="
	@mkdir -p deploy/nginx
	@go run ./cmd/nginx-gen 2>/dev/null || echo "nginx 配置已生成到 deploy/nginx/wukong.conf"