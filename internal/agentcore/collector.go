// 系统指标采集器
// 采集 CPU/内存/磁盘/网络上下行/系统版本
package agentcore

import (
	"fmt"
	"log"
	"os"
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
	lastNetCounters []net.IOCountersStat
	lastNetTime     time.Time
	region          string
}

func (c *SystemCollector) Name() string { return "system" }

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

	// 网络上下行（bps）
	netCounters, err := net.IOCounters(false)
	if err != nil {
		log.Printf("采集网络失败: %v", err)
	} else if len(netCounters) > 0 {
		now := time.Now()
		sys.NetUpTotalBytes = int64(netCounters[0].BytesSent)
		sys.NetDownTotalBytes = int64(netCounters[0].BytesRecv)
		if c.lastNetCounters != nil {
			elapsed := now.Sub(c.lastNetTime).Seconds()
			if elapsed > 0 {
				bytesUp := netCounters[0].BytesSent - c.lastNetCounters[0].BytesSent
				bytesDown := netCounters[0].BytesRecv - c.lastNetCounters[0].BytesRecv
				sys.NetUpBps = int64(float64(bytesUp) / elapsed * 8)
				sys.NetDownBps = int64(float64(bytesDown) / elapsed * 8)
			}
		}
		c.lastNetCounters = netCounters
		c.lastNetTime = now
	}

	// 操作系统版本、启动时间与运行时长
	hostInfo, err := host.Info()
	if err != nil {
		log.Printf("采集系统版本失败: %v", err)
	} else {
		sys.OsVersion = fmt.Sprintf("%s %s", hostInfo.OS, hostInfo.PlatformVersion)
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
