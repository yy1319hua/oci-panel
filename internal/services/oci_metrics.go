package services

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

// CpuMemoryMetrics 实例 CPU / 内存使用率时序（百分比）。
// 数据源：oci_computeagent 命名空间（实例需装 Oracle Cloud Agent，官方镜像默认自带）。
// 内存指标 CpuUtilization 无需额外配置；MemoryUtilization 需要 agent 的
// "Memory Utilization" 插件开启（控制台默认开启）。查不到时返回空数组，前端显示「无数据」。
type CpuMemoryMetrics struct {
	Time     []string  `json:"time"`
	Cpu      []float64 `json:"cpu"`
	Memory   []float64 `json:"memory"`
	HasMemory bool     `json:"hasMemory"` // 内存插件是否启用
}

// GetCpuMemoryMetrics 查询指定实例近 N 小时的 CPU/内存使用率曲线。
func (s *OCIService) GetCpuMemoryMetrics(ctx context.Context, user *models.OciUser, instanceId string, hours int) (*CpuMemoryMetrics, error) {
	client, err := s.GetMonitoringClient(user)
	if err != nil {
		return nil, fmt.Errorf("创建监控客户端失败: %w", err)
	}
	if hours <= 0 || hours > 168 {
		hours = 24
	}
	end := time.Now()
	start := end.Add(-time.Duration(hours) * time.Hour)
	interval, timeLayout := pickInterval(start, end)

	cpuQuery := fmt.Sprintf("CpuUtilization[%s]{resourceId = \"%s\"}.max()", interval, instanceId)
	memQuery := fmt.Sprintf("MemoryUtilization[%s]{resourceId = \"%s\"}.max()", interval, instanceId)

	out := &CpuMemoryMetrics{Time: []string{}, Cpu: []float64{}, Memory: []float64{}}
	cpuMap := map[string]float64{}
	memMap := map[string]float64{}

	for _, m := range []struct {
		query string
		dst   map[string]float64
		isMem bool
	}{
		{cpuQuery, cpuMap, false},
		{memQuery, memMap, true},
	} {
		resp, err := client.SummarizeMetricsData(ctx, monitoring.SummarizeMetricsDataRequest{
			CompartmentId: &user.OciTenantID,
			SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
				Namespace: stringPtr("oci_computeagent"),
				Query:     &m.query,
				StartTime: &common.SDKTime{Time: start},
				EndTime:   &common.SDKTime{Time: end},
			},
		})
		if err != nil {
			if m.isMem {
				continue // 内存插件未开启时该查询会报错，降级为仅 CPU 曲线
			}
			return nil, fmt.Errorf("CPU 指标查询失败: %w", err)
		}
		for _, item := range resp.Items {
			for _, dp := range item.AggregatedDatapoints {
				if dp.Timestamp == nil || dp.Value == nil {
					continue
				}
				m.dst[dp.Timestamp.Format(timeLayout)] = *dp.Value
				if m.isMem {
					out.HasMemory = true
				}
			}
		}
	}

	times := make([]string, 0, len(cpuMap)+len(memMap))
	seen := map[string]bool{}
	for t := range cpuMap {
		times = append(times, t)
		seen[t] = true
	}
	for t := range memMap {
		if !seen[t] {
			times = append(times, t)
		}
	}
	sort.Strings(times)

	for _, t := range times {
		out.Time = append(out.Time, t)
		out.Cpu = append(out.Cpu, cpuMap[t])
		out.Memory = append(out.Memory, memMap[t])
	}
	return out, nil
}
