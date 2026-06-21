// 系统指标采集器
// 采集 CPU/内存/磁盘/网络上下行/系统版本
package agentcore

import (
	"fmt"
	"log"
	"runtime"
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

	// 内存使用率
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("采集内存失败: %v", err)
	} else {
		sys.MemPercent = memInfo.UsedPercent
	}

	// 磁盘使用率（根分区）
	diskInfo, err := disk.Usage("/")
	if err != nil {
		log.Printf("采集磁盘失败: %v", err)
	} else {
		sys.DiskPercent = diskInfo.UsedPercent
	}

	// 网络上下行（bps）
	netCounters, err := net.IOCounters(false)
	if err != nil {
		log.Printf("采集网络失败: %v", err)
	} else if len(netCounters) > 0 {
		now := time.Now()
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

	// 操作系统版本
	hostInfo, err := host.Info()
	if err != nil {
		log.Printf("采集系统版本失败: %v", err)
	} else {
		sys.OsVersion = fmt.Sprintf("%s %s", hostInfo.OS, hostInfo.PlatformVersion)
	}

	return result, nil
}