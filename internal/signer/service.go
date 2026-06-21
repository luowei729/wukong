// ed25519 签名服务
// 独立进程持有私钥，通过 gRPC Unix Socket 接收签名请求
// 与 web 后端物理隔离：web 后端被打穿也拿不到私钥
package signer

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"

	pb "wukong/proto/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Service struct {
	pb.UnimplementedSignerServiceServer
	privKey ed25519.PrivateKey
	pubKey  ed25519.PublicKey
}

func NewService(privKey ed25519.PrivateKey, pubKey ed25519.PublicKey) *Service {
	return &Service{
		privKey: privKey,
		pubKey:  pubKey,
	}
}

func RegisterService(grpcServer *grpc.Server, svc *Service) {
	pb.RegisterSignerServiceServer(grpcServer, svc)
	log.Println("签名服务已注册")
}

// Sign 对指令进行 ed25519 签名
func (s *Service) Sign(ctx context.Context, req *pb.SignRequest) (*pb.SignResponse, error) {
	// 生成指令 ID
	cmdID := newUUID()

	// 计算时间戳
	issuedAt := time.Now()
	expiresIn := time.Duration(req.ExpiresInSec) * time.Second
	if expiresIn <= 0 || expiresIn > 300*time.Second {
		expiresIn = 60 * time.Second // 默认 60 秒有效期，防止重放攻击
	}
	expiresAt := issuedAt.Add(expiresIn)

	// 构造待签名数据：cmdID + commandType + payload + issuedAt + expiresAt
	signData := append([]byte(cmdID+req.CommandType), req.Payload...)
	signData = append(signData, []byte(fmt.Sprintf("%d%d", issuedAt.Unix(), expiresAt.Unix()))...)

	// 使用 ed25519 签名
	signature := ed25519.Sign(s.privKey, signData)

	return &pb.SignResponse{
		Signature:  signature,
		CommandId:  cmdID,
		IssuedAt:   issuedAt.Unix(),
		ExpiresAt:  expiresAt.Unix(),
		PublicKey:  s.pubKey,
	}, nil
}

// Verify 验证签名（探针侧使用此函数）
func Verify(publicKey ed25519.PublicKey, cmdID, commandType string, payload []byte, issuedAt, expiresAt int64, signature []byte) bool {
	signData := append([]byte(cmdID+commandType), payload...)
	signData = append(signData, []byte(fmt.Sprintf("%d%d", issuedAt, expiresAt))...)

	// 检查是否过期
	now := time.Now().Unix()
	if now > expiresAt {
		log.Printf("签名验证失败：指令已过期 (now=%d expires=%d)", now, expiresAt)
		return false
	}

	return ed25519.Verify(publicKey, signData, signature)
}

// LoadOrGenerateKey 加载或生成 ed25519 密钥对
func LoadOrGenerateKey(keyPath string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	// 尝试从文件加载
	if data, err := os.ReadFile(keyPath); err == nil {
		privKey := ed25519.PrivateKey(data)
		if len(privKey) != ed25519.PrivateKeySize {
			return nil, nil, fmt.Errorf("密钥文件大小不合法: 期望 %d 字节, 实际 %d 字节", ed25519.PrivateKeySize, len(privKey))
		}
		pubKey := privKey.Public().(ed25519.PublicKey)
		log.Printf("已加载已有签名密钥: %s", keyPath)
		return privKey, pubKey, nil
	}

	// 生成新密钥对
	log.Printf("未找到密钥文件 %s，正在生成新密钥对...", keyPath)
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("生成 ed25519 密钥失败: %w", err)
	}

	// 保存私钥
	if err := os.MkdirAll(ownerDir(keyPath), 0700); err != nil {
		return nil, nil, fmt.Errorf("创建密钥目录失败: %w", err)
	}
	if err := os.WriteFile(keyPath, privKey, 0400); err != nil {
		return nil, nil, fmt.Errorf("保存密钥文件失败: %w", err)
	}

	// 保存公钥
	pubKeyPath := keyPath + ".pub"
	if err := os.WriteFile(pubKeyPath, pubKey, 0444); err != nil {
		return nil, nil, fmt.Errorf("保存公钥文件失败: %w", err)
	}

	log.Printf("已生成新签名密钥: %s", keyPath)
	log.Printf("公钥指纹: %s", hex.EncodeToString(pubKey[:16]))
	return privKey, pubKey, nil
}

func ownerDir(path string) string {
	// 简单取目录路径
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return ""
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b)
}