// wukong 签名服务入口
// 独立进程，持 ed25519 私钥，通过 Unix Socket 接收签名请求
// 与 web 后端物理隔离：web 后端被打穿也拿不到私钥
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"wukong/internal/signer"
	"google.golang.org/grpc"
)

func main() {
	socketPath := flag.String("socket", "/opt/wukong/data/signer.sock", "Unix Socket 路径")
	keyPath := flag.String("key", "/opt/wukong/data/signing/ed25519.key", "签名私钥路径")
	flag.Parse()

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(*socketPath), 0711); err != nil {
		log.Fatalf("创建 socket 目录失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*keyPath), 0700); err != nil {
		log.Fatalf("创建密钥目录失败: %v", err)
	}

	// 加载或生成 ed25519 密钥对
	privKey, pubKey, err := signer.LoadOrGenerateKey(*keyPath)
	if err != nil {
		log.Fatalf("加载密钥失败: %v", err)
	}
	log.Printf("签名公钥: %x", pubKey)

	// 清理旧的 socket 文件
	os.Remove(*socketPath)

	// 监听 Unix Socket
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("监听 socket %s 失败: %v", *socketPath, err)
	}
	defer listener.Close()

	// 权限设置为 600，仅 wukong 用户可访问
	os.Chmod(*socketPath, 0600)

	// 启动 gRPC 签名服务
	grpcServer := grpc.NewServer()
	signerService := signer.NewService(privKey, pubKey)
	signer.RegisterService(grpcServer, signerService)

	log.Printf("签名服务已启动，socket: %s", *socketPath)

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("签名服务正在关闭...")
	grpcServer.GracefulStop()
	log.Println("签名服务已停止")
}