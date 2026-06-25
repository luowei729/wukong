// wukong 监控系统 - 主控入口
// 单二进制：Web API + gRPC 探针通道 + 前端静态资源（Go embed）
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wukong/internal/alert"
	"wukong/internal/auth"
	"wukong/internal/config"
	"wukong/internal/grpcapi"
	"wukong/internal/notify"
	"wukong/internal/store"
	"wukong/internal/webapi"

	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
)

// 构建时通过 -ldflags 注入的版本信息
var (
	version   = "dev"   // 版本号，如 0.2.0
	commit    = "none"  // Git commit hash
	buildTime = "none"  // 构建时间
)

func main() {
	configPath := flag.String("config", config.DefaultConfigFile, "配置文件路径")
	flag.Parse()

	cfg, err := config.LoadServerConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	log.Printf("wukong 主控启动中，版本: %s (commit: %s, 构建: %s)，监听 %s ...", version, commit, buildTime, cfg.ListenAddr)

	// === 初始化 SQLite 存储层 ===
	s, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer s.Close()

	if err := s.InitSchema(); err != nil {
		log.Fatalf("初始化表结构失败: %v", err)
	}
	log.Println("数据库初始化完成")

	// === 主控启动时自动将自己的 commit 同步为探针目标版本 ===
	// 原因：主控和探针在同一 Docker 镜像中编译，commit hash 完全一致；
	// 启动时自动写入，不再需要手动设置 agent_target_version，也不会因
	// 本地 push 的 hash 与 Actions 构建的 hash 不同导致升级死循环。
	if commit != "none" && commit != "" {
		existingTarget, _ := s.GetSetting("agent_target_version")
		if existingTarget != commit {
			if err := s.SetSetting("agent_target_version", commit); err != nil {
				log.Printf("同步探针目标版本失败: %v", err)
			} else {
				log.Printf("探针目标版本已同步为: %s", commit)
			}
		}
	}

	// === 启动时序聚合与清理任务 ===
	maintenanceCtx, maintenanceCancel := context.WithCancel(context.Background())
	defer maintenanceCancel()
	go runMaintenanceLoop(maintenanceCtx, s)

	// === 初始化并启动告警引擎 ===
	alertEngine := alert.NewEngine(s, cfg)
	go alertEngine.Run()

	// === 初始化通知渠道 ===
	notifier := notify.NewManager()
	// 后续从配置加载 Telegram bot token

	// === 初始化鉴权服务 ===
	authSvc := auth.NewService(cfg)

	// === 启动 gRPC 服务（探针通道） ===
	grpcServer := grpc.NewServer()

	// === 启动 Web API 服务（REST + SSE + embed 前端） ===
	webHandler := webapi.NewHandler(s, authSvc, alertEngine, notifier, cfg)

	// === cmux 的监听器 ===
	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		log.Fatalf("监听端口失败: %v", err)
	}

	// 创建 cmux 复用器
	m := cmux.New(listener)
	grpcL := m.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
	httpL := m.Match(cmux.HTTP1Fast())

	// 启动 gRPC server
	go func() {
		grpcapi.RegisterService(grpcServer, s, alertEngine, cfg)
		if err := grpcServer.Serve(grpcL); err != nil {
			log.Printf("gRPC server 退出: %v", err)
		}
	}()

	// 启动 HTTP server（Web API + embed 前端）
	httpServer := &http.Server{
		Handler: webHandler,
	}
	go func() {
		if err := httpServer.Serve(httpL); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server 退出: %v", err)
		}
	}()

	// 启动 cmux（主循环）
	go func() {
		if err := m.Serve(); err != nil {
			log.Printf("cmux 退出: %v", err)
		}
	}()

	log.Println("wukong 主控启动完成")

	// === 等待退出信号 ===
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("收到退出信号，正在关闭服务...")
	grpcServer.GracefulStop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
	listener.Close()
	log.Println("wukong 主控已停止")
}

func runMaintenanceLoop(ctx context.Context, s store.MetricsStore) {
	// Ping 原始数据按小时表写入，前端 24h 图表读取分钟聚合；这里每分钟滚动聚合上一分钟数据。
	pingTicker := time.NewTicker(time.Minute)
	defer pingTicker.Stop()

	// 历史清理低频执行，避免 SQLite 文件无限增长；失败只记录日志，不影响主控服务。
	cleanupTicker := time.NewTicker(6 * time.Hour)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pingTicker.C:
			if err := s.AggregatePingMin(); err != nil {
				log.Printf("聚合 Ping 分钟数据失败: %v", err)
			}
		case <-cleanupTicker.C:
			if err := s.CleanOldAggData(24 * 30); err != nil {
				log.Printf("清理旧 Ping 聚合数据失败: %v", err)
			}
			if err := s.DropOldHourlyTables(24 * 30); err != nil {
				log.Printf("清理旧小时表失败: %v", err)
			}
		}
	}
}
