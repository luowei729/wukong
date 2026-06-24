// wukong 监控系统 - 探针入口
// 单二进制：采集 + Ping + gRPC 上报 + 指令接收 + 本地缓冲
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"wukong/internal/agentcore"
	"wukong/internal/config"
)

// 构建时通过 -ldflags 注入的版本信息
var (
	version   = "dev"   // 版本号，如 0.2.0
	commit    = "none"  // Git commit hash
	buildTime = "none"  // 构建时间
)

func main() {
	configPath := flag.String("config", "", "探针配置文件路径（默认 /opt/wukong/agent/agent.conf）")
	serverAddr := flag.String("server", "", "主控地址（如 xxx.com:443，覆盖配置文件）")
	token := flag.String("token", "", "注册 token（首次安装用，注册后自动保存个体凭证）")
	flag.Parse()

	// 加载探针配置
	cfg, err := config.LoadAgentConfig(*configPath)
	if err != nil {
		log.Fatalf("加载探针配置失败: %v", err)
	}
	if *serverAddr != "" {
		cfg.ServerAddr = *serverAddr
	}

	log.Printf("wukong 探针启动中，版本: %s (commit: %s, 构建: %s)，主控地址: %s", version, commit, buildTime, cfg.ServerAddr)

	// 创建探针核心，传入构建时注入的版本号
	agent, err := agentcore.NewAgentWithVersion(cfg, version)
	if err != nil {
		log.Fatalf("创建探针实例失败: %v", err)
	}

	// 如果是首次安装（带了 token），只执行注册并退出。
	// 原因：安装脚本后续还要写入 systemd 并由 systemd 常驻启动；注册命令不能阻塞在前台，避免用户 Ctrl+C 后才继续安装。
	if *token != "" {
		if err := agent.Register(*token); err != nil {
			log.Fatalf("注册到主控失败: %v", err)
		}
		log.Println("探针注册成功，个体凭证已保存")
		return
	}

	// 启动探针（采集 + 上报 + 指令接收）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := agent.Start(ctx); err != nil {
		log.Fatalf("启动探针失败: %v", err)
	}
	log.Println("wukong 探针运行中")

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("收到退出信号，正在停止探针...")
	agent.Stop()
	log.Println("wukong 探针已停止")
}
