// 单项告警规则模型（全局一份，落 SQLite settings 表）
//
// 为什么要重构这一层：旧实现里每项告警只有一个阈值 key，
//   - 持续时间全局共用一个 alert_metric_duration_seconds（只有磁盘单独，且是引擎里硬编码的 key）
//   - 恢复滞回值硬编码 85，界面上没有入口
//   - 抑制期只有配置文件 AlertSuppressMinutes 一个全局值
//   - ThresholdConfig.Enabled 字段定义了但引擎从未读取，所以任何一项告警都关不掉
//
// 现在改为"每个指标一条规则"，开关 / 阈值 / 持续时间 / 滞回 / 抑制期 全部独立可调。
package alert

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"wukong/internal/store"
)

// 六项告警的标识：与 alerts 表的 metric 列、告警中心展示、探针指标名一一对应
const (
	MetricOffline     = "offline"
	MetricCPU         = "cpu"
	MetricMem         = "mem"
	MetricDisk        = "disk"
	MetricPingLatency = "ping_latency"
	MetricPingLoss    = "ping_loss"
)

// RuleSettingPrefix 是每条规则在 settings 表里的 key 前缀（alert_rule_cpu 等）
const RuleSettingPrefix = "alert_rule_"

// Rule 单项告警规则
type Rule struct {
	Metric      string  `json:"metric"`
	Enabled     bool    `json:"enabled"`
	Warning     float64 `json:"warning"`      // 触发阈值；offline 表示"无心跳多少秒算离线"
	Duration    int     `json:"duration"`     // 持续超阈多少秒才触发；offline 不适用（恒 0）
	Recovery    float64 `json:"recovery"`     // 滞回：回落到该值以下才判恢复；0 表示不适用
	SuppressMin int     `json:"suppress_min"` // 抑制期（分钟）：期间不重复触发同一告警
}

// RuleSpec 单项规则的元信息：默认值、取值范围、单位与旧 key。
// 目的是让"默认值/范围/标签"只存在这一处——引擎兜底、API 校验、前端渲染都从这里取，
// 避免同一套数字散落在 Go 和 Vue 两边互相漂移（旧版就是这样：85 写在引擎里没人知道）。
type RuleSpec struct {
	Metric      string  `json:"metric"`
	Label       string  `json:"label"`
	Unit        string  `json:"unit"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Step        float64 `json:"step"`
	DefWarning  float64 `json:"default_warning"`
	DefDuration int     `json:"default_duration"`
	DefRecovery float64 `json:"default_recovery"`
	// HasDuration=false 表示该项没有"持续时间"语义（离线本身就是时长判断）
	HasDuration bool `json:"has_duration"`
	// HasRecovery=false 表示滞回无意义（离线恢复 = 重新上线，没有"回落到某个值"）
	HasRecovery bool `json:"has_recovery"`
	// LegacyKey 是旧版扁平设置的 key，用于首次读取时把用户已配的值迁移过来
	LegacyKey string `json:"legacy_key"`
	// LegacyDurationKey 是旧版该项单独使用的持续时间 key（磁盘例外，历史上就有独立 key）
	LegacyDurationKey string `json:"legacy_duration_key"`
}

// ruleSpecs 六项规则的定义表。顺序即前端展示顺序。
var ruleSpecs = []RuleSpec{
	{
		Metric: MetricOffline, Label: "节点离线", Unit: "s",
		Min: 5, Max: 3600, Step: 1, DefWarning: 600,
		// 离线没有"持续时间"与"滞回"：Warning 本身就是"无心跳秒数"，恢复即上线
		HasDuration: false, HasRecovery: false,
		LegacyKey: "alert_offline_seconds",
	},
	{
		Metric: MetricCPU, Label: "CPU 使用率", Unit: "%",
		Min: 1, Max: 100, Step: 1, DefWarning: 90, DefDuration: 60, DefRecovery: 85,
		HasDuration: true, HasRecovery: true,
		LegacyKey: "alert_cpu_threshold",
	},
	{
		Metric: MetricMem, Label: "内存使用率", Unit: "%",
		Min: 1, Max: 100, Step: 1, DefWarning: 90, DefDuration: 60, DefRecovery: 85,
		HasDuration: true, HasRecovery: true,
		LegacyKey: "alert_mem_threshold",
	},
	{
		Metric: MetricDisk, Label: "磁盘使用率", Unit: "%",
		Min: 1, Max: 100, Step: 1, DefWarning: 90, DefDuration: 300, DefRecovery: 85,
		HasDuration: true, HasRecovery: true,
		LegacyKey:         "alert_disk_threshold",
		LegacyDurationKey: "alert_disk_duration_seconds",
	},
	{
		Metric: MetricPingLatency, Label: "Ping 延迟", Unit: "ms",
		Min: 1, Max: 10000, Step: 1, DefWarning: 200, DefDuration: 60, DefRecovery: 150,
		HasDuration: true, HasRecovery: true,
		LegacyKey: "alert_ping_latency_threshold",
	},
	{
		Metric: MetricPingLoss, Label: "Ping 丢包率", Unit: "%",
		Min: 1, Max: 100, Step: 1, DefWarning: 20, DefDuration: 60, DefRecovery: 5,
		HasDuration: true, HasRecovery: true,
		LegacyKey: "alert_ping_loss_threshold",
	},
}

// RuleSpecs 返回定义表副本，供 API 下发给前端渲染（前端不再自己写死范围与默认值）
func RuleSpecs() []RuleSpec {
	out := make([]RuleSpec, len(ruleSpecs))
	copy(out, ruleSpecs)
	return out
}

// SpecOf 按指标名取定义；找不到返回 false（调用方据此跳过而不是 panic）
func SpecOf(metric string) (RuleSpec, bool) {
	for _, s := range ruleSpecs {
		if s.Metric == metric {
			return s, true
		}
	}
	return RuleSpec{}, false
}

// settingGetter 只依赖读设置的能力，便于单元测试里塞假数据
type settingGetter interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}

// LoadRules 读取全部规则。
// 缺失的规则会从旧版扁平设置迁移（保留用户已配好的阈值，不因为改版而回到默认值），
// 并立即落库，这样设置页能直接看到并修改这条规则。
func LoadRules(s store.MetricsStore, defaultSuppressMin int) map[string]Rule {
	rules := make(map[string]Rule, len(ruleSpecs))
	for _, spec := range ruleSpecs {
		rules[spec.Metric] = loadRule(s, spec, defaultSuppressMin)
	}
	return rules
}

// loadRule 读取单项规则，必要时执行迁移
func loadRule(s settingGetter, spec RuleSpec, defaultSuppressMin int) Rule {
	raw, err := s.GetSetting(RuleSettingPrefix + spec.Metric)
	if err == nil && strings.TrimSpace(raw) != "" {
		var r Rule
		if jsonErr := json.Unmarshal([]byte(raw), &r); jsonErr == nil && r.Metric != "" {
			// 库里存的是历史值，仍要按当前定义夹紧范围与补齐不适用的字段，
			// 防止以后调整范围后旧值越界导致引擎行为异常
			return normalize(r, spec, defaultSuppressMin)
		}
	}
	return migrateRule(s, spec, defaultSuppressMin)
}

// migrateRule 从旧版扁平 key 构造规则并落库。
// 原因：直接给默认值会把用户已经调好的阈值悄悄抹掉，必须读旧 key 兜底。
func migrateRule(s settingGetter, spec RuleSpec, defaultSuppressMin int) Rule {
	rule := Rule{
		Metric:      spec.Metric,
		Enabled:     true, // 旧版没有开关概念，迁移后保持"全部开启"，行为与升级前一致
		Warning:     legacyFloat(s, spec.LegacyKey, spec.DefWarning),
		Duration:    legacyDuration(s, spec),
		Recovery:    spec.DefRecovery,
		SuppressMin: defaultSuppressMin,
	}
	rule = normalize(rule, spec, defaultSuppressMin)
	// 落库失败只影响下次要不要重复迁移，不阻塞告警检查，所以忽略错误但保留返回值
	if data, err := json.Marshal(rule); err == nil {
		_ = s.SetSetting(RuleSettingPrefix+spec.Metric, string(data))
	}
	return rule
}

// legacyDuration 取该项的持续时间：优先该项专属旧 key（磁盘），其次全局旧 key，最后定义默认值
func legacyDuration(s settingGetter, spec RuleSpec) int {
	if !spec.HasDuration {
		return 0
	}
	if spec.LegacyDurationKey != "" {
		if v, err := s.GetSetting(spec.LegacyDurationKey); err == nil && strings.TrimSpace(v) != "" {
			if n, convErr := strconv.Atoi(strings.TrimSpace(v)); convErr == nil {
				return n
			}
		}
	}
	// 旧版所有资源类指标共用这一个 key，迁移时按它给每项铺底
	if v, err := s.GetSetting("alert_metric_duration_seconds"); err == nil && strings.TrimSpace(v) != "" {
		if n, convErr := strconv.Atoi(strings.TrimSpace(v)); convErr == nil {
			return n
		}
	}
	return spec.DefDuration
}

// legacyFloat 读取旧扁平 key，缺失或非法时用定义里的默认值
func legacyFloat(s settingGetter, key string, fallback float64) float64 {
	if key == "" {
		return fallback
	}
	raw, err := s.GetSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return fallback
	}
	v, convErr := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if convErr != nil {
		return fallback
	}
	return v
}

// normalize 按定义夹紧取值，并把"不适用"的字段强制归零。
// 单独抽出来是为了让"读库/迁移/保存"三条路径共用同一套清洗规则。
func normalize(r Rule, spec RuleSpec, defaultSuppressMin int) Rule {
	out := r
	out.Metric = spec.Metric
	if out.Warning < spec.Min || out.Warning > spec.Max {
		out.Warning = clamp(spec.DefWarning, spec.Min, spec.Max)
	}
	if spec.HasDuration {
		if out.Duration < 1 {
			out.Duration = spec.DefDuration
		}
		if out.Duration > 3600 {
			out.Duration = 3600
		}
	} else {
		// 离线项不接受持续时间，统一置 0 以免前端/引擎对语义产生分歧
		out.Duration = 0
	}
	if spec.HasRecovery {
		if out.Recovery <= 0 || out.Recovery >= out.Warning {
			// 滞回必须低于触发阈值，否则永远无法判定恢复
			out.Recovery = spec.DefRecovery
		}
	} else {
		out.Recovery = 0
	}
	if out.SuppressMin < 1 {
		out.SuppressMin = defaultSuppressMin
	}
	if out.SuppressMin > 1440 {
		out.SuppressMin = 1440
	}
	return out
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// ValidateRule 校验一条待保存的规则，返回中文错误信息供 API 直接回传
func ValidateRule(in Rule) error {
	spec, ok := SpecOf(in.Metric)
	if !ok {
		return fmt.Errorf("未知的告警项: %s", in.Metric)
	}
	if in.Warning < spec.Min || in.Warning > spec.Max {
		return fmt.Errorf("%s 阈值必须在 %g-%g %s 之间", spec.Label, spec.Min, spec.Max, spec.Unit)
	}
	if spec.HasDuration {
		if in.Duration < 1 || in.Duration > 3600 {
			return fmt.Errorf("%s 持续时间必须在 1-3600 秒之间", spec.Label)
		}
	}
	if spec.HasRecovery {
		if in.Recovery <= 0 {
			return fmt.Errorf("%s 恢复阈值必须大于 0（指标回落到该值以下才判恢复）", spec.Label)
		}
		if in.Recovery >= in.Warning {
			return fmt.Errorf("%s 恢复阈值必须低于告警阈值，否则告警永远无法自动恢复", spec.Label)
		}
	}
	if in.SuppressMin < 1 || in.SuppressMin > 1440 {
		return fmt.Errorf("%s 抑制期必须在 1-1440 分钟之间", spec.Label)
	}
	return nil
}

// SaveRule 清洗并写入 settings 表。
// 落库为 JSON 而不是多个扁平 key：一项规则的字段天然成组，整体读写不会出现
// "改了阈值没改滞回"这种半更新状态。
func SaveRule(s store.MetricsStore, in Rule, defaultSuppressMin int) error {
	spec, ok := SpecOf(in.Metric)
	if !ok {
		return fmt.Errorf("未知的告警项: %s", in.Metric)
	}
	if err := ValidateRule(in); err != nil {
		return err
	}
	rule := normalize(in, spec, defaultSuppressMin)
	data, err := json.Marshal(rule)
	if err != nil {
		return fmt.Errorf("序列化规则失败: %w", err)
	}
	return s.SetSetting(RuleSettingPrefix+spec.Metric, string(data))
}
