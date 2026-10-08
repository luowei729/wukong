// 公网 IP 可用性判定（探针与主控共用同一套规则）
//
// 为什么要单独成包：判定逻辑原先只在 internal/agentcore 里有一份 isPublicIP，
// 服务端与探针各写各的、或者干脆复用不到一起，是这次 net1上海 误判的根源之一。
// 探针上报、主控落库校验、目标下发过滤三处必须用同一个判断，否则会出现
// "探针认为可用、主控认为不可用"的口径分裂。
package netutil

import (
	"net"
	"strings"
)

// 这些前缀都不是"节点真实可直连的公网 IPv6"，必须排除：
//
//	64:ff9b::/96    NAT64 well-known 前缀（RFC 6052）。外部服务看到的"客户端 IPv6"其实是
//	                运营商 NAT64 网关合成的地址，只能 v6→v4，访问真正的 IPv6 目标必然不通。
//	64:ff9b:1::/48  NAT64 本地用途前缀（RFC 8215），同上。
//	2001::/32       Teredo 隧道（RFC 4380），依赖 UDP 中继，连通性不保证。
//	2002::/16       6to4 隧道（RFC 3964），anycast 中继早已废弃，普遍不可达。
//	100:64::/10     DS-Lite 地址共享（RFC 6333），运营商级 NAT，不可入站。
var unusableIPv6Prefixes []*net.IPNet

// cgnatBlock 是运营商级 NAT（RFC 6598）段，不是真公网，单独判
var cgnatBlock *net.IPNet

func init() {
	for _, cidr := range []string{
		"64:ff9b::/96",
		"64:ff9b:1::/48",
		"2001::/32",
		"2002::/16",
		"100:64::/10",
	} {
		if _, block, err := net.ParseCIDR(cidr); err == nil {
			unusableIPv6Prefixes = append(unusableIPv6Prefixes, block)
		}
	}
	// 预计算而不是每次调用时 ParseCIDR + 写 map：
	// 判定函数会被探针与主控多个 goroutine 并发调用，无锁写 map 会 fatal error
	if _, block, err := net.ParseCIDR("100.64.0.0/10"); err == nil {
		cgnatBlock = block
	}
}

// IsPublicIPv4 判断是否为可对外使用的公网 IPv4（排除私有、回环、链路本地、未指定、组播）。
// 额外排除 169.254.0.0/16（云厂商元数据/链路本地段）与 CGNAT 100.64.0.0/10：
// 这些地址在 VPS 上常被网卡误当成"出口 IP"上报。
func IsPublicIPv4(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return false
	}
	v4 := ip.To4()
	if v4 == nil {
		return false // 明确只接受 IPv4
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsPrivate() {
		return false
	}
	// CGNAT 段（RFC 6598）：运营商内网地址，不是真正公网
	if cgnatBlock != nil && cgnatBlock.Contains(v4) {
		return false
	}
	return true
}

// IsUsablePublicIPv6 判断是否为"节点真实可用"的公网 IPv6。
// 除了常规的非公网/链路本地/回环过滤外，重点排除 NAT64、Teredo、6to4、DS-Lite
// 这类"看起来是全局单播、实际无法直连任意 IPv6 目标"的合成或隧道地址。
func IsUsablePublicIPv6(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return false
	}
	if ip.To4() != nil {
		return false // 明确只接受 IPv6（Go 会把 IPv4 映射成 v4-in-v6 形式）
	}
	if !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || ip.IsPrivate() {
		return false
	}
	// ULA（fc00::/7）已被 IsPrivate 覆盖，这里再显式检查隧道与合成前缀
	for _, block := range unusableIPv6Prefixes {
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

// IsIPv6Literal 判断字符串是否写成 IPv6 地址（用于区分探测目标是 v4 还是 v6）。
// 带 zone 的写法（fe80::1%eth0）与方括号写法（[2409::a]）都能识别。
func IsIPv6Literal(value string) bool {
	ip := parseLoose(value)
	if ip == nil {
		return false
	}
	return ip.To4() == nil
}

// parseLoose 宽松解析：允许 "[2409:8088::a]" 与带 zone 的写法
func parseLoose(value string) net.IP {
	v := strings.TrimSpace(value)
	v = strings.TrimPrefix(v, "[")
	v = strings.TrimSuffix(v, "]")
	// 去掉 IPv6 zone 标识，否则 ParseIP 会失败
	if i := strings.IndexByte(v, '%'); i >= 0 {
		v = v[:i]
	}
	return net.ParseIP(v)
}
