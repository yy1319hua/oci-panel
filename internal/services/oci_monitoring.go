package services

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

func (s *OCIService) GetTrafficData(ctx context.Context, user *models.OciUser, vnicId string, startTime string, endTime string) (*models.TrafficData, error) {
	monitoringClient, err := s.GetMonitoringClient(user)
	if err != nil {
		return nil, fmt.Errorf("创建监控客户端失败: %w", err)
	}

	trafficData := &models.TrafficData{
		Time:     []string{},
		Inbound:  []string{},
		Outbound: []string{},
	}

	// 按 VNIC 维度查流量必须使用 oci_vcn 命名空间（resourceId = VNIC OCID）。
	// oci_computeagent 的 NetworksBytesIn/Out 的 resourceId 是「实例 OCID」且为全 VNIC 聚合，
	// 用 VNIC ID 过滤会查不到任何数据流（表现为“无数据”）。
	// 分辨率按查询跨度自适应：跨度越大用越粗的聚合间隔，避免数据点过密导致表格冗长。
	compartmentId := user.OciTenantID
	start := parseTime(startTime)
	end := parseTime(endTime)
	interval, timeLayout := pickInterval(start, end)

	inboundQuery := fmt.Sprintf("VnicFromNetworkBytes[%s]{resourceId = \"%s\"}.sum()", interval, vnicId)
	outboundQuery := fmt.Sprintf("VnicToNetworkBytes[%s]{resourceId = \"%s\"}.sum()", interval, vnicId)

	// 记录查询要素，便于排查「无数据」问题（真实 OCI 报错会随函数返回，不再被吞掉）。
	log.Printf("[流量查询] vnicId=%s compartment=%s start=%s end=%s interval=%s", vnicId, compartmentId, start.Format(time.RFC3339), end.Format(time.RFC3339), interval)

	// 获取入站数据
	inReq := monitoring.SummarizeMetricsDataRequest{
		CompartmentId: &compartmentId,
		SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
			Namespace: stringPtr("oci_vcn"),
			Query:     &inboundQuery,
			StartTime: &common.SDKTime{Time: start},
			EndTime:   &common.SDKTime{Time: end},
		},
	}
	inResp, err := monitoringClient.SummarizeMetricsData(ctx, inReq)
	if err != nil {
		return nil, fmt.Errorf("入站流量查询失败(vnic=%s): %w", vnicId, err)
	}

	// 获取出站数据
	outReq := monitoring.SummarizeMetricsDataRequest{
		CompartmentId: &compartmentId,
		SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
			Namespace: stringPtr("oci_vcn"),
			Query:     &outboundQuery,
			StartTime: &common.SDKTime{Time: start},
			EndTime:   &common.SDKTime{Time: end},
		},
	}
	outResp, err := monitoringClient.SummarizeMetricsData(ctx, outReq)
	if err != nil {
		return nil, fmt.Errorf("出站流量查询失败(vnic=%s): %w", vnicId, err)
	}

	// 入站/出站数据点可能数量不同（某一方向无流量时为空），按时间戳对齐到同一时间轴。
	inboundMap := map[string]string{}
	outboundMap := map[string]string{}
	timeSet := map[string]bool{}

	for _, item := range inResp.Items {
		for _, dp := range item.AggregatedDatapoints {
			if dp.Timestamp == nil || dp.Value == nil {
				continue
			}
			t := dp.Timestamp.Format(timeLayout)
			inboundMap[t] = fmt.Sprintf("%.2f", *dp.Value/1024/1024)
			timeSet[t] = true
		}
	}
	for _, item := range outResp.Items {
		for _, dp := range item.AggregatedDatapoints {
			if dp.Timestamp == nil || dp.Value == nil {
				continue
			}
			t := dp.Timestamp.Format(timeLayout)
			outboundMap[t] = fmt.Sprintf("%.2f", *dp.Value/1024/1024)
			timeSet[t] = true
		}
	}

	times := make([]string, 0, len(timeSet))
	for t := range timeSet {
		times = append(times, t)
	}
	sort.Strings(times) // MM-DD HH:mm / HH:mm 字典序即时间序

	trafficData.Time = times
	for _, t := range times {
		trafficData.Inbound = append(trafficData.Inbound, inboundMap[t])
		trafficData.Outbound = append(trafficData.Outbound, outboundMap[t])
	}

	return trafficData, nil
}

// pickInterval 按查询时间跨度选择监控聚合间隔与时间轴显示格式。
// 参考 OCI 控制台默认图表：跨度越大，间隔越粗，避免返回过多数据点导致表格冗长。
// 返回 (interval, timeLayout)，interval 直接用于查询语句如 VnicFromNetworkBytes[5m]。
func pickInterval(start, end time.Time) (string, string) {
	span := end.Sub(start)
	switch {
	case span <= 6*time.Hour:
		return "5m", "15:04"
	case span <= 24*time.Hour:
		return "15m", "15:04"
	case span <= 7*24*time.Hour:
		return "1h", "01-02 15:04"
	default:
		return "1d", "01-02"
	}
}

func parseTime(timeStr string) time.Time {
	// 兼容前端 dateTime-local（YYYY-MM-DDTHH:mm）与带时区的 ISO 格式，
	// 并统一转为 UTC（OCI Monitoring 以 UTC 为准），避免时间错位或无数据。
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, timeStr); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC().Add(-1 * time.Hour)
}

// InstanceTrafficStat 单实例流量明细（单位：字节）。
type InstanceTrafficStat struct {
	InstanceID  string `json:"instanceId"`
	DisplayName string `json:"displayName"`
	Inbound     int64  `json:"inbound"`  // 实际入站
	Outbound    int64  `json:"outbound"` // 实际出站
	Billable    int64  `json:"billable"` // 计费出站（免费额度外）
}

// FreeTierAllowanceBytes Oracle 出站流量免费额度：10TB。
// 注意：甲骨文仅对「出站」流量计费，入站流量免费；超此额度才按量计费。
const FreeTierAllowanceBytes int64 = 10 * 1024 * 1024 * 1024 * 1024

// MonthlyTrafficStats 月度流量统计结果（账号级汇总 + 每实例明细 + 实际/计费区分）。
type MonthlyTrafficStats struct {
	InstanceCount   int                   `json:"instanceCount"`
	InboundTraffic  int64                 `json:"inboundTraffic"`
	OutboundTraffic int64                 `json:"outboundTraffic"`
	BillableTraffic int64                 `json:"billableTraffic"`
	FreeAllowance   int64                 `json:"freeAllowance"`
	Instances       []InstanceTrafficStat `json:"instances"`
	DailyLabels     []string              `json:"dailyLabels"`
	DailyInbound    []int64               `json:"dailyInbound"`
	DailyOutbound   []int64               `json:"dailyOutbound"`
}

// GetMonthlyTrafficStats 获取指定配置的月度流量统计（账号级汇总 + 每实例明细 + 实际/计费区分）。
// 按账号维度遍历所有实例与其 VNIC，累加实际入站/出站与计费出站，并采集逐日序列用于趋势。
func (s *OCIService) GetMonthlyTrafficStats(ctx context.Context, user *models.OciUser) (*MonthlyTrafficStats, error) {
	computeClient, err := s.GetComputeClient(user)
	if err != nil {
		return nil, err
	}

	vnClient, err := s.GetVirtualNetworkClient(user)
	if err != nil {
		return nil, err
	}

	monitoringClient, err := s.GetMonitoringClient(user)
	if err != nil {
		return nil, err
	}

	compartmentId := user.OciTenantID
	stats := &MonthlyTrafficStats{FreeAllowance: FreeTierAllowanceBytes}

	// 获取实例列表
	instances, err := s.ListInstances(ctx, user, compartmentId)
	if err != nil {
		return nil, err
	}
	stats.InstanceCount = len(instances)

	// 获取本月时间范围
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Second)

	// 逐日累计（用于趋势 sparkline）
	dailyInbound := map[string]int64{}
	dailyOutbound := map[string]int64{}

	// 遍历每个实例获取 VNIC 流量
	for _, instance := range instances {
		if instance.Id == nil {
			continue
		}

		instStat := InstanceTrafficStat{InstanceID: *instance.Id}
		if instance.DisplayName != nil {
			instStat.DisplayName = *instance.DisplayName
		}

		// 获取实例的 VNIC 附件
		vnicAttachReq := core.ListVnicAttachmentsRequest{
			CompartmentId: &compartmentId,
			InstanceId:    instance.Id,
		}
		vnicAttachResp, err := computeClient.ListVnicAttachments(ctx, vnicAttachReq)
		if err != nil {
			continue
		}

		for _, attach := range vnicAttachResp.Items {
			if attach.VnicId == nil {
				continue
			}

			// 获取 VNIC 信息
			vnicReq := core.GetVnicRequest{VnicId: attach.VnicId}
			vnicResp, err := vnClient.GetVnic(ctx, vnicReq)
			if err != nil {
				continue
			}

			if vnicResp.Id == nil {
				continue
			}

			vnicId := *vnicResp.Id

			// 入站（实际）：VnicFromNetworkBytes（从网络到 VNIC = 入站）
			inQuery := fmt.Sprintf("VnicFromNetworkBytes[1d]{resourceId = \"%s\"}.sum()", vnicId)
			inReq := monitoring.SummarizeMetricsDataRequest{
				CompartmentId: &compartmentId,
				SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
					Namespace: stringPtr("oci_vcn"),
					Query:     &inQuery,
					StartTime: &common.SDKTime{Time: startOfMonth},
					EndTime:   &common.SDKTime{Time: endOfMonth},
				},
			}
			if inResp, e := monitoringClient.SummarizeMetricsData(ctx, inReq); e == nil {
				for _, item := range inResp.Items {
					for _, dp := range item.AggregatedDatapoints {
						if dp.Value == nil {
							continue
						}
						v := int64(*dp.Value)
						stats.InboundTraffic += v
						instStat.Inbound += v
						if dp.Timestamp != nil {
							dailyInbound[dp.Timestamp.UTC().Format("2006-01-02")] += v
						}
					}
				}
			}

			// 出站（实际）：VnicToNetworkBytes（从 VNIC 到网络 = 出站）
			outQuery := fmt.Sprintf("VnicToNetworkBytes[1d]{resourceId = \"%s\"}.sum()", vnicId)
			outReq := monitoring.SummarizeMetricsDataRequest{
				CompartmentId: &compartmentId,
				SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
					Namespace: stringPtr("oci_vcn"),
					Query:     &outQuery,
					StartTime: &common.SDKTime{Time: startOfMonth},
					EndTime:   &common.SDKTime{Time: endOfMonth},
				},
			}
			if outResp, e := monitoringClient.SummarizeMetricsData(ctx, outReq); e == nil {
				for _, item := range outResp.Items {
					for _, dp := range item.AggregatedDatapoints {
						if dp.Value == nil {
							continue
						}
						v := int64(*dp.Value)
						stats.OutboundTraffic += v
						instStat.Outbound += v
						if dp.Timestamp != nil {
							dailyOutbound[dp.Timestamp.UTC().Format("2006-01-02")] += v
						}
					}
				}
			}

			// 计费出站（best-effort）：VnicBillableBytesOut（免费额度外才计费，额度内为 0）
			billQuery := fmt.Sprintf("VnicBillableBytesOut[1d]{resourceId = \"%s\"}.sum()", vnicId)
			billReq := monitoring.SummarizeMetricsDataRequest{
				CompartmentId: &compartmentId,
				SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
					Namespace: stringPtr("oci_vcn"),
					Query:     &billQuery,
					StartTime: &common.SDKTime{Time: startOfMonth},
					EndTime:   &common.SDKTime{Time: endOfMonth},
				},
			}
			if billResp, e := monitoringClient.SummarizeMetricsData(ctx, billReq); e == nil {
				for _, item := range billResp.Items {
					for _, dp := range item.AggregatedDatapoints {
						if dp.Value != nil {
							v := int64(*dp.Value)
							stats.BillableTraffic += v
							instStat.Billable += v
						}
					}
				}
			}
		}

		stats.Instances = append(stats.Instances, instStat)
	}

	// 构建按天排序的日序列
	labels := make([]string, 0, len(dailyInbound))
	for d := range dailyInbound {
		labels = append(labels, d)
	}
	sort.Strings(labels)
	stats.DailyLabels = labels
	for _, l := range labels {
		stats.DailyInbound = append(stats.DailyInbound, dailyInbound[l])
		stats.DailyOutbound = append(stats.DailyOutbound, dailyOutbound[l])
	}

	return stats, nil
}

// FormatBytes 格式化字节数为人类可读格式
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
