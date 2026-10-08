// netutil IP 可用性判定单元测试
// 起因：net1上海 把运营商 NAT64 合成地址 64:ff9b::aff:fb01 当成"可用公网 IPv6"上报，
// 导致它被纳入 IPv6 探测线路后长期 100% 丢包并误触告警。这些用例把判定口径钉死，防止回归。
package netutil

import "testing"

func TestIsUsablePublicIPv6(t *testing.T) {
	cases := []struct {
		addr string
		want bool
		desc string
	}{
		{"2409:8088::a", true, "中国移动真实公网 IPv6"},
		{"2603:c020:800e:26ee:6909:f434:199c:c2cd", true, "Oracle Cloud 真实 IPv6"},
		{"2400:38e0:1:404a::b8", true, "KDDI 真实 IPv6"},
		{"64:ff9b::aff:fb01", false, "NAT64 well-known 合成地址（net1上海 实测值）"},
		{"64:ff9b:1::1", false, "NAT64 本地用途前缀"},
		{"2001:0::1", false, "Teredo 隧道"},
		{"2002:c000:204::1", false, "6to4 隧道"},
		{"100:64::1", false, "DS-Lite 地址共享"},
		{"fe80::1", false, "链路本地"},
		{"fd12:3456::1", false, "ULA 私有段"},
		{"::1", false, "IPv6 回环"},
		{"::", false, "未指定地址"},
		{"ff02::1", false, "组播"},
		{"111.229.196.230", false, "IPv4 不算可用 IPv6"},
		{"", false, "空值"},
		{"not-an-ip", false, "非法字符串"},
	}
	for _, c := range cases {
		if got := IsUsablePublicIPv6(c.addr); got != c.want {
			t.Errorf("IsUsablePublicIPv6(%q) = %v, 期望 %v（%s）", c.addr, got, c.want, c.desc)
		}
	}
}

func TestIsPublicIPv4(t *testing.T) {
	cases := []struct {
		addr string
		want bool
		desc string
	}{
		{"111.229.196.230", true, "腾讯云公网 IPv4"},
		{"146.56.173.198", true, "公网 IPv4"},
		{"10.0.0.5", false, "RFC1918 私有"},
		{"172.16.0.1", false, "RFC1918 私有"},
		{"192.168.1.1", false, "RFC1918 私有"},
		{"100.64.0.1", false, "CGNAT（RFC6598）不是真公网"},
		{"169.254.169.254", false, "链路本地/云元数据"},
		{"127.0.0.1", false, "回环"},
		{"2409:8088::a", false, "IPv6 不算 IPv4"},
		{"", false, "空值"},
	}
	for _, c := range cases {
		if got := IsPublicIPv4(c.addr); got != c.want {
			t.Errorf("IsPublicIPv4(%q) = %v, 期望 %v（%s）", c.addr, got, c.want, c.desc)
		}
	}
}

func TestIsIPv6Literal(t *testing.T) {
	cases := []struct {
		addr string
		want bool
		desc string
	}{
		{"2409:8088::a", true, "IPv6 目标地址"},
		{"[2409:8088::a]", true, "带方括号写法"},
		{"fe80::1%eth0", true, "带 zone 写法"},
		{"1.2.3.4", false, "IPv4 目标"},
		{"example.com", false, "域名不是 IPv6 字面量"},
		{"", false, "空值"},
	}
	for _, c := range cases {
		if got := IsIPv6Literal(c.addr); got != c.want {
			t.Errorf("IsIPv6Literal(%q) = %v, 期望 %v（%s）", c.addr, got, c.want, c.desc)
		}
	}
}
