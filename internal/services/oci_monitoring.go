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
	// 这里用 .sum() 聚合该时段字节数（与 OCI 控制台 VNIC 默认统计一致），分辨率 1 分钟。
	inboundQuery := fmt.Sprintf("VnicFromNetworkBytes[1m]{resourceId = \"%s\"}.sum()", vnicId)
	outboundQuery := fmt.Sprintf("VnicToNetworkBytes[1m]{resourceId = \"%s\"}.sum()", vnicId)

	compartmentId := user.OciTenantID
	start := parseTime(startTime)
	end := parseTime(endTime)

	// 记录查询要素，便于排查「无数据」问题（真实 OCI 报错会随函数返回，不再被吞掉）。
	log.Printf("[流量查询] vnicId=%s compartment=%s start=%s end=%s", vnicId, compartmentId, start.Format(time.RFC3339), end.Format(time.RFC3339))

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
			t := dp.Timestamp.Format("15:04")
			inboundMap[t] = fmt.Sprintf("%.2f", *dp.Value/1024/1024)
			timeSet[t] = true
		}
	}
	for _, item := range outResp.Items {
		for _, dp := range item.AggregatedDatapoints {
			if dp.Timestamp == nil || dp.Value == nil {
				continue
			}
			t := dp.Timestamp.Format("15:04")
			outboundMap[t] = fmt.Sprintf("%.2f", *dp.Value/1024/1024)
			timeSet[t] = true
		}
	}

	times := make([]string, 0, len(timeSet))
	for t := range timeSet {
		times = append(times, t)
	}
	sort.Strings(times) // 15:04 字典序即时间序（同一日内）

	trafficData.Time = times
	for _, t := range times {
		trafficData.Inbound = append(trafficData.Inbound, inboundMap[t])
		trafficData.Outbound = append(trafficData.Outbound, outboundMap[t])
	}

	return trafficData, nil
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

// MonthlyTrafficStats 月度流量统计结果
type MonthlyTrafficStats struct {
	InstanceCount   int
	InboundTraffic  int64
	OutboundTraffic int64
}

// GetMonthlyTrafficStats 获取指定配置的月度流量统计
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
	stats := &MonthlyTrafficStats{}

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

	// 遍历每个实例获取VNIC流量
	for _, instance := range instances {
		if instance.Id == nil {
			continue
		}

		// 获取实例的VNIC附件
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

			// 获取VNIC信息
			vnicReq := core.GetVnicRequest{VnicId: attach.VnicId}
			vnicResp, err := vnClient.GetVnic(ctx, vnicReq)
			if err != nil {
				continue
			}

			if vnicResp.Id == nil {
				continue
			}

			vnicId := *vnicResp.Id

			// 查询入站流量 (VnicToNetworkBytes)
			inQuery := fmt.Sprintf("VnicToNetworkBytes[1d]{resourceId = \"%s\"}.sum()", vnicId)
			inReq := monitoring.SummarizeMetricsDataRequest{
				CompartmentId: &compartmentId,
				SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
					Namespace: stringPtr("oci_vcn"),
					Query:     &inQuery,
					StartTime: &common.SDKTime{Time: startOfMonth},
					EndTime:   &common.SDKTime{Time: endOfMonth},
				},
			}
			inResp, err := monitoringClient.SummarizeMetricsData(ctx, inReq)
			if err == nil {
				for _, item := range inResp.Items {
					for _, dp := range item.AggregatedDatapoints {
						if dp.Value != nil {
							stats.InboundTraffic += int64(*dp.Value)
						}
					}
				}
			}

			// 查询出站流量 (VnicFromNetworkBytes)
			outQuery := fmt.Sprintf("VnicFromNetworkBytes[1d]{resourceId = \"%s\"}.sum()", vnicId)
			outReq := monitoring.SummarizeMetricsDataRequest{
				CompartmentId: &compartmentId,
				SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
					Namespace: stringPtr("oci_vcn"),
					Query:     &outQuery,
					StartTime: &common.SDKTime{Time: startOfMonth},
					EndTime:   &common.SDKTime{Time: endOfMonth},
				},
			}
			outResp, err := monitoringClient.SummarizeMetricsData(ctx, outReq)
			if err == nil {
				for _, item := range outResp.Items {
					for _, dp := range item.AggregatedDatapoints {
						if dp.Value != nil {
							stats.OutboundTraffic += int64(*dp.Value)
						}
					}
				}
			}
		}
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
