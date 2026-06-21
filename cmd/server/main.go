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

	"wukong/internal/config"
	"wukong/internal/store"
	"wukong/internal/webapi"
	"wukong/internal/grpcapi"
	"wukong/internal/alert"
	"wukong/internal/notify"
	"wukong/internal/auth"

	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
)

func main() {
	configPath := flag.String("config", config.DefaultConfigFile, "配置文件路径")
	flag.Parse()

	cfg, err := config.LoadServerConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	log.Printf("wukong 主控启动中，监听 %s ...", cfg.ListenAddr)

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

	// === 初始化告警引擎 ===
	alertEngine := alert.NewEngine(s, cfg)

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