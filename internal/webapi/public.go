// 公开状态页 API
// 仅返回脱敏后的只读服务器状态，供未登录首页和公开详情页使用。
package webapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"wukong/internal/store"
)

type publicServersResponse struct {
	GeneratedAt time.Time             `json:"generated_at"`
	Summary     publicServersSummary  `json:"summary"`
	Servers     []publicServerSummary `json:"servers"`
}

type publicServersSummary struct {
	Total   int     `json:"total"`
	Online  int     `json:"online"`
	Offline int     `json:"offline"`
	AvgCPU  float64 `json:"avg_cpu"`
	AvgMem  float64 `json:"avg_mem"`
	AvgDisk float64 `json:"avg_disk"`
}

type publicServerSummary struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Online     bool       `json:"online"`
	Status     string     `json:"status"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
	OSVersion  string     `json:"os_version,omitempty"`
	CPU        *float64   `json:"cpu,omitempty"`
	Mem        *float64   `json:"mem,omitempty"`
	Disk       *float64   `json:"disk,omitempty"`
	NetUp      *int64     `json:"net_up,omitempty"`
	NetDown    *int64     `json:"net_down,omitempty"`
}

type publicServerDetailResponse struct {
	Server publicServerSummary `json:"server"`
}

type publicMetricPoint struct {
	Timestamp time.Time `json:"timestamp"`
	CPU       float64   `json:"cpu"`
	Mem       float64   `json:"mem"`
	Disk      float64   `json:"disk"`
	NetUp     int64     `json:"net_up"`
	NetDown   int64     `json:"net_down"`
}

type publicMetricResponse struct {
	Points []publicMetricPoint `json:"points"`
}

type publicPingPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Count     int       `json:"count"`
	AvgLat    float64   `json:"avg_lat"`
	MinLat    float64   `json:"min_lat"`
	MaxLat    float64   `json:"max_lat"`
	LossRate  float64   `json:"loss_rate"`
}

type publicPingResponse struct {
	ISP    string            `json:"isp"`
	Points []publicPingPoint `json:"points"`
}

func (h *Handler) handlePublicListServers(w http.ResponseWriter, r *http.Request) {
	// 首页公开列表只组合探针元数据和最新指标，不返回 secret、分组配置等管理字段。
	agents, err := h.store.ListAgents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务暂时不可用")
		return
	}
	latest, err := h.store.GetAllLatestMetrics()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务暂时不可用")
		return
	}

	resp := publicServersResponse{GeneratedAt: time.Now(), Servers: make([]publicServerSummary, 0, len(agents))}
	var cpuSum, memSum, diskSum float64
	var metricCount int
	for _, agent := range agents {
		metric := latest[agent.ID]
		server := buildPublicServerSummary(agent, metric)
		resp.Servers = append(resp.Servers, server)
		resp.Summary.Total++
		if server.Online {
			resp.Summary.Online++
		} else {
			resp.Summary.Offline++
		}
		if metric != nil {
			cpuSum += metric.CPU
			memSum += metric.Mem
			diskSum += metric.Disk
			metricCount++
		}
	}
	if metricCount > 0 {
		resp.Summary.AvgCPU = roundMetric(cpuSum / float64(metricCount))
		resp.Summary.AvgMem = roundMetric(memSum / float64(metricCount))
		resp.Summary.AvgDisk = roundMetric(diskSum / float64(metricCount))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handlePublicGetServer(w http.ResponseWriter, r *http.Request) {
	// 详情页先确认探针存在，再尝试读取最新指标；没有指标时仍返回基础信息。
	id := getPathValue(r, "id")
	agent, err := h.store.GetAgent(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "服务器不存在")
		return
	}
	metric, _ := h.store.GetLatestMetrics(id)
	writeJSON(w, http.StatusOK, publicServerDetailResponse{Server: buildPublicServerSummary(agent, metric)})
}

func (h *Handler) handlePublicGetServerMetrics(w http.ResponseWriter, r *http.Request) {
	// 公开趋势查询限制最大时间范围，防止未登录接口被大范围扫历史数据。
	id := getPathValue(r, "id")
	if _, err := h.store.GetAgent(id); err != nil {
		writeError(w, http.StatusNotFound, "服务器不存在")
		return
	}
	since, until, err := parsePublicTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	metrics, err := h.store.GetSystemMetrics(id, since, until)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务暂时不可用")
		return
	}
	points := make([]publicMetricPoint, 0, len(metrics))
	for _, metric := range metrics {
		points = append(points, publicMetricPoint{
			Timestamp: metric.Timestamp,
			CPU:       metric.CPU,
			Mem:       metric.Mem,
			Disk:      metric.Disk,
			NetUp:     metric.NetUp,
			NetDown:   metric.NetDown,
		})
	}
	writeJSON(w, http.StatusOK, publicMetricResponse{Points: points})
}

func (h *Handler) handlePublicGetServerPingAgg(w http.ResponseWriter, r *http.Request) {
	// Ping 公开查询需要明确 ISP，避免后端猜测错误线路；详情页可在无 ISP 时展示空态。
	id := getPathValue(r, "id")
	isp := r.URL.Query().Get("isp")
	if isp == "" {
		writeError(w, http.StatusBadRequest, "缺少 isp 参数")
		return
	}
	if _, err := h.store.GetAgent(id); err != nil {
		writeError(w, http.StatusNotFound, "服务器不存在")
		return
	}
	since, until, err := parsePublicTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := h.store.GetPingAgg(id, isp, since, until)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务暂时不可用")
		return
	}
	points := make([]publicPingPoint, 0, len(items))
	for _, item := range items {
		points = append(points, publicPingPoint{
			Timestamp: item.BucketMin,
			Count:     item.Count,
			AvgLat:    item.AvgLat,
			MinLat:    item.MinLat,
			MaxLat:    item.MaxLat,
			LossRate:  item.LossRate,
		})
	}
	writeJSON(w, http.StatusOK, publicPingResponse{ISP: isp, Points: points})
}

func buildPublicServerSummary(agent *store.Agent, metric *store.LatestMetric) publicServerSummary {
	name := agent.Name
	if name == "" {
		name = agent.Hostname
	}
	server := publicServerSummary{
		ID:         agent.ID,
		Name:       name,
		Online:     agent.Online,
		Status:     publicStatus(agent, metric),
		LastSeenAt: agent.LastSeenAt,
	}
	if metric != nil {
		updatedAt := metric.UpdatedAt
		cpu := roundMetric(metric.CPU)
		mem := roundMetric(metric.Mem)
		disk := roundMetric(metric.Disk)
		server.UpdatedAt = &updatedAt
		server.OSVersion = metric.OSVersion
		server.CPU = &cpu
		server.Mem = &mem
		server.Disk = &disk
		server.NetUp = &metric.NetUp
		server.NetDown = &metric.NetDown
	} else if agent.OSVersion != "" {
		server.OSVersion = agent.OSVersion
	}
	server.Online = server.Status == "online"
	return server
}

func publicStatus(agent *store.Agent, metric *store.LatestMetric) string {
	// 没有任何指标时显示 unknown；有指标但超过 5 分钟显示 stale。
	if metric == nil || metric.UpdatedAt.IsZero() {
		if agent.Online {
			return "stale"
		}
		return "unknown"
	}
	if time.Since(metric.UpdatedAt) > 5*time.Minute {
		return "stale"
	}
	if agent.Online {
		return "online"
	}
	return "offline"
}

func parsePublicTimeRange(r *http.Request) (time.Time, time.Time, error) {
	now := time.Now()
	until := now
	since := now.Add(-24 * time.Hour)
	if rangeText := r.URL.Query().Get("range"); rangeText != "" {
		d, err := time.ParseDuration(rangeText)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("range 参数无效")
		}
		since = until.Add(-d)
	}
	if sinceText := r.URL.Query().Get("since"); sinceText != "" {
		t, err := time.Parse(time.RFC3339, sinceText)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("since 参数无效")
		}
		since = t
	}
	if untilText := r.URL.Query().Get("until"); untilText != "" {
		t, err := time.Parse(time.RFC3339, untilText)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("until 参数无效")
		}
		until = t
	}
	if !since.Before(until) {
		return time.Time{}, time.Time{}, fmt.Errorf("时间范围无效")
	}
	if until.Sub(since) > 72*time.Hour {
		since = until.Add(-72 * time.Hour)
	}
	return since, until, nil
}

func roundMetric(v float64) float64 {
	value, _ := strconv.ParseFloat(fmt.Sprintf("%.1f", v), 64)
	return value
}
