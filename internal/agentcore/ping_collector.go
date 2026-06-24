// 运营商 Ping 采集器
// 按主控下发的 ISP 目标执行 TCP/ICMP(auto) 探测，并通过 MetricsReport.Pings 上报。
package agentcore

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"wukong/internal/config"
	pb "wukong/proto/gen"
)

var pingStatsPattern = regexp.MustCompile(`rtt [^=]+= ([0-9.]+)/([0-9.]+)/([0-9.]+)/([0-9.]+) ms`)

// PingCollector 按独立频率限流执行运营商探测。
type PingCollector struct {
	interval time.Duration
	targets  []config.PingTargetConfig
	lastRun  time.Time
}

func NewPingCollector(intervalSec int, targets []config.PingTargetConfig) *PingCollector {
	if intervalSec <= 0 {
		intervalSec = 1
	}
	return &PingCollector{interval: time.Duration(intervalSec) * time.Second, targets: targets}
}

func (c *PingCollector) Name() string { return "ping" }

func (c *PingCollector) Collect() (*CollectResult, error) {
	// 系统指标默认 1 秒采集，Ping 目标按自己的周期执行，未到周期时返回空结果。
	if !c.lastRun.IsZero() && time.Since(c.lastRun) < c.interval {
		return &CollectResult{}, nil
	}
	c.lastRun = time.Now()

	result := &CollectResult{}
	for _, target := range c.targets {
		if !target.Enabled || target.Name == "" || target.IP == "" {
			continue
		}
		metric := c.probe(target)
		metric.Timestamp = time.Now().Unix()
		result.Pings = append(result.Pings, metric)
	}
	return result, nil
}

func (c *PingCollector) probe(target config.PingTargetConfig) *pb.PingMetric {
	mode := strings.ToLower(strings.TrimSpace(target.Mode))
	if mode == "" {
		mode = "auto"
	}
	if target.Port <= 0 {
		target.Port = 80
	}
	metric := &pb.PingMetric{IspName: target.Name, TargetIp: target.IP, LossRate: 1}

	if mode == "icmp" || mode == "auto" {
		latency, jitter, loss, err := probeICMP(target.IP)
		if err == nil {
			metric.LatencyMs = latency
			metric.JitterMs = jitter
			metric.LossRate = loss
			return metric
		}
		if mode == "icmp" {
			log.Printf("ICMP Ping 失败: isp=%s target=%s err=%v", target.Name, target.IP, err)
			return metric
		}
	}

	latency, err := probeTCP(target.IP, target.Port)
	if err != nil {
		log.Printf("TCP Ping 失败: isp=%s target=%s:%d err=%v", target.Name, target.IP, target.Port, err)
		return metric
	}
	metric.LatencyMs = latency
	metric.LossRate = 0
	return metric
}

func probeTCP(host string, port int) (float64, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return 0, err
	}
	_ = conn.Close()
	return float64(time.Since(start).Microseconds()) / 1000, nil
}

func probeICMP(host string) (latency, jitter, loss float64, err error) {
	// 第一阶段使用系统 ping 命令，避免引入 raw socket 权限要求；auto 模式失败后会回退 TCP。
	// IPv6 目标需要使用 ping6 或 ping -6，否则系统 ping 无法正确处理 IPv6 地址。
	pingCmd := "ping"
	pingArgs := []string{"-c", "3", "-W", "1"}
	if isIPv6(host) {
		// 优先尝试 ping -6（Linux iputils），回退到 ping6（BSD/旧 Linux）
		if _, lookupErr := exec.LookPath("ping6"); lookupErr == nil {
			pingCmd = "ping6"
		} else {
			pingArgs = append([]string{"-6"}, pingArgs...)
		}
	}
	pingArgs = append(pingArgs, host)
	cmd := exec.Command(pingCmd, pingArgs...)
	out, runErr := cmd.CombinedOutput()
	text := string(out)
	loss = parsePacketLoss(text)
	matches := pingStatsPattern.FindStringSubmatch(text)
	if len(matches) >= 5 {
		avg, _ := strconv.ParseFloat(matches[2], 64)
		mdev, _ := strconv.ParseFloat(matches[4], 64)
		return avg, mdev, loss, nil
	}
	if runErr != nil {
		return 0, 0, 1, runErr
	}
	return 0, 0, loss, fmt.Errorf("无法解析 ping 输出")
}

// isIPv6 判断目标地址是否为 IPv6（包含冒号且不是 IPv4 映射地址）
func isIPv6(host string) bool {
	// 先尝试直接解析为 IP
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.To4() == nil
	}
	// 可能是域名，尝试 DNS 解析查看是否有 AAAA 记录
	addrs, err := net.LookupHost(host)
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if ip := net.ParseIP(addr); ip != nil && ip.To4() == nil {
			return true
		}
	}
	return false
}

func parsePacketLoss(text string) float64 {
	idx := strings.Index(text, "% packet loss")
	if idx < 0 {
		return 1
	}
	start := strings.LastIndex(text[:idx], " ")
	if start < 0 {
		return 1
	}
	value := strings.TrimSpace(text[start:idx])
	percent, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 1
	}
	return percent / 100
}
