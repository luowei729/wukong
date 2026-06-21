# ============================================
# wukong 监控系统 - Docker 编译 + 运行
# 全量 multistage: Vue3 前端编译 → Go 后端编译 → 运行
# 直接暴露 64443，不依赖 nginx
# ============================================

# ---- Stage 1: 构建前端 ----
FROM node:22-alpine AS frontend-builder

WORKDIR /app
COPY web/package.json web/package-lock.json* ./web/
RUN cd web && npm install
COPY web/ ./web/
RUN cd web && npm run build
# vite outDir: ../internal/webapi/dist 即 /app/internal/webapi/dist/

# ---- Stage 2: 编译后端 ----
FROM golang:1.25-alpine AS backend-builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /build

# 依赖缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制全部源码
COPY . .

# 从前端构建产物复制（vite outDir 为 ../internal/webapi/dist，即 /app/internal/webapi/dist/）
COPY --from=frontend-builder /app/internal/webapi/dist/ ./internal/webapi/dist/

# 编译主控（CGO_ENABLED=1 因为 SQLite 需要 CGO）
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /build/wukong-server ./cmd/server

# ---- Stage 3: 运行 ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

# 创建运行目录
RUN mkdir -p /opt/wukong/data

# 复制二进制
COPY --from=backend-builder /build/wukong-server /opt/wukong/wukong

# 环境变量覆盖配置（无需配置文件即可运行）
ENV WUKONG_LISTEN_ADDR=0.0.0.0:64443
ENV WUKONG_ADMIN_PASSWORD=""
ENV WUKONG_JWT_SECRET=""
ENV WUKONG_DATA_DIR=/opt/wukong/data
ENV WUKONG_DB_PATH=/opt/wukong/data/wukong.db
ENV WUKONG_LOG_LEVEL=info

VOLUME ["/opt/wukong/data"]

EXPOSE 64443

# 没有配置文件也能启动，所有配置通过环境变量覆盖
ENTRYPOINT ["/opt/wukong/wukong"]
# 不传 --config，使用默认路径 /opt/wukong/wukong.conf
# 配置文件不存在时自动使用默认值 + 环境变量覆盖
CMD ["--config", ""]