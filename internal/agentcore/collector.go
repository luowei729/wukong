// 系统指标采集器
// 采集 CPU/内存/磁盘/网络上下行/系统版本
package agentcore

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	pb "wukong/proto/gen"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// SystemCollector 系统指标采集器
type SystemCollector struct {
	lastNetCounters map[string]net.IOCountersStat
	lastNetTime     time.Time
	region          string
}

func (c *SystemCollector) Name() string { return "system" }

func isPublicNetInterface(name string) bool {
	name = strings.ToLower(name)
	if name == "" || name == "lo" {
		return false
	}
	ignoredPrefixes := []string{"docker", "veth", "br-", "virbr", "cni", "flannel", "tailscale", "wg", "tun", "tap"}
	for _, prefix := range ignoredPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

func (c *SystemCollector) Collect() (*CollectResult, error) {
	result := &CollectResult{}
	sys := &pb.SystemMetric{}
	result.System = sys

	// CPU 使用率（取 1 秒平均）
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil {
		log.Printf("采集 CPU 失败: %v", err)
	} else if len(cpuPercent) > 0 {
		sys.CpuPercent = cpuPercent[0]
	}
	if cpuInfos, err := cpu.Info(); err == nil && len(cpuInfos) > 0 {
		// CPU 型号和核心数用于公开详情页规格展示，不包含敏感信息。
		sys.CpuModel = cpuInfos[0].ModelName
		var cores int32
		for _, item := range cpuInfos {
			cores += item.Cores
		}
		if cores == 0 {
			cores = int32(len(cpuInfos))
		}
		sys.CpuCores = cores
	} else if err != nil {
		log.Printf("采集 CPU 型号失败: %v", err)
	}

	// 内存使用率和总量
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("采集内存失败: %v", err)
	} else {
		sys.MemPercent = memInfo.UsedPercent
		sys.MemTotalBytes = int64(memInfo.Total)
	}

	// 磁盘使用率和总量（根分区）
	diskInfo, err := disk.Usage("/")
	if err != nil {
		log.Printf("采集磁盘失败: %v", err)
	} else {
		sys.DiskPercent = diskInfo.UsedPercent
		sys.DiskTotalBytes = int64(diskInfo.Total)
	}

	// 网络上下行（字节/秒）。
	// 必须按网卡明细过滤掉 lo/docker/veth/br 等虚拟接口，否则 Docker/本机回环流量会把公网速率统计错。
	netCounters, err := net.IOCounters(true)
	if err != nil {
		log.Printf("采集网络失败: %v", err)
	} else if len(netCounters) > 0 {
		now := time.Now()
		current := make(map[string]net.IOCountersStat)
		var totalSent, totalRecv uint64
		var deltaSent, deltaRecv uint64
		for _, item := range netCounters {
			if !isPublicNetInterface(item.Name) {
				continue
			}
			current[item.Name] = item
			totalSent += item.BytesSent
			totalRecv += item.BytesRecv
			if c.lastNetCounters != nil {
				if prev, ok := c.lastNetCounters[item.Name]; ok {
					if item.BytesSent >= prev.BytesSent {
						deltaSent += item.BytesSent - prev.BytesSent
					}
					if item.BytesRecv >= prev.BytesRecv {
						deltaRecv += item.BytesRecv - prev.BytesRecv
					}
				}
			}
		}
		sys.NetUpTotalBytes = int64(totalSent)
		sys.NetDownTotalBytes = int64(totalRecv)
		if c.lastNetCounters != nil {
			elapsed := now.Sub(c.lastNetTime).Seconds()
			if elapsed > 0 {
				sys.NetUpBps = int64(float64(deltaSent) / elapsed)
				sys.NetDownBps = int64(float64(deltaRecv) / elapsed)
			}
		}
		c.lastNetCounters = current
		c.lastNetTime = now
	}

	// 操作系统版本：使用 Platform + PlatformVersion 组合显示
	// hostInfo.OS 返回 "linux"，hostInfo.Platform 返回 "ubuntu"，hostInfo.PlatformVersion 返回 "22.04"
	// 组合后显示为 "Ubuntu 22.04" 而不是 "linux 22.04"
	hostInfo, err := host.Info()
	if err != nil {
		log.Printf("采集系统版本失败: %v", err)
	} else {
		if hostInfo.Platform != "" {
			// 首字母大写：ubuntu → Ubuntu，centos → CentOS，debian → Debian
			plat := strings.Title(hostInfo.Platform)
			if hostInfo.PlatformVersion != "" {
				sys.OsVersion = fmt.Sprintf("%s %s", plat, hostInfo.PlatformVersion)
			} else {
				sys.OsVersion = plat
			}
		} else {
			// 回退到 OS 字段（如 "linux"）
			sys.OsVersion = hostInfo.OS
		}
		sys.Platform = hostInfo.Platform
		sys.UptimeSeconds = int64(hostInfo.Uptime)
		sys.BootTime = int64(hostInfo.BootTime)
	}

	// 系统负载用于详情页展示 1m/5m/15m；不支持的平台只记录日志并保持 0。
	if avg, err := load.Avg(); err == nil {
		sys.Load1 = avg.Load1
		sys.Load5 = avg.Load5
		sys.Load15 = avg.Load15
	} else {
		log.Printf("采集系统负载失败: %v", err)
	}

	// 区域只读取手动配置或环境变量，避免探针自动访问第三方定位服务造成隐私和可用性问题。
	sys.Region = c.region
	if sys.Region == "" {
		sys.Region = os.Getenv("WUKONG_AGENT_REGION")
	}

	return result, nil
}
