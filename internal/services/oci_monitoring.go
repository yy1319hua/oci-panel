package services

import (
	"context"
	"fmt"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

func (s *OCIService) GetTrafficData(ctx context.Context, user *models.OciUser, vnicId string, startTime string, endTime string) (*models.TrafficData, error) {
	monitoringClient, err := s.GetMonitoringClient(user)
	if err != nil {
		return nil, err
	}

	trafficData := &models.TrafficData{
		Time:     []string{},
		Inbound:  []string{},
		Outbound: []string{},
	}

	// 构建查询 - 入站流量
	inboundQuery := fmt.Sprintf("NetworksBytesIn[1m]{resourceId = \"%s\"}.mean()", vnicId)
	outboundQuery := fmt.Sprintf("NetworksBytesOut[1m]{resourceId = \"%s\"}.mean()", vnicId)

	compartmentId := user.OciTenantID

	// 获取入站数据
	inReq := monitoring.SummarizeMetricsDataRequest{
		CompartmentId: &compartmentId,
		SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
			Namespace: stringPtr("oci_computeagent"),
			Query:     &inboundQuery,
			StartTime: &common.SDKTime{Time: parseTime(startTime)},
			EndTime:   &common.SDKTime{Time: parseTime(endTime)},
		},
	}

	inResp, err := monitoringClient.SummarizeMetricsData(ctx, inReq)
	if err == nil {
		for _, item := range inResp.Items {
			for _, dp := range item.AggregatedDatapoints {
				if dp.Timestamp != nil {
					trafficData.Time = append(trafficData.Time, dp.Timestamp.Format("15:04"))
				}
				if dp.Value != nil {
					trafficData.Inbound = append(trafficData.Inbound, fmt.Sprintf("%.2f", *dp.Value/1024/1024))
				}
			}
		}
	}

	// 获取出站数据
	outReq := monitoring.SummarizeMetricsDataRequest{
		CompartmentId: &compartmentId,
		SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
			Namespace: stringPtr("oci_computeagent"),
			Query:     &outboundQuery,
			StartTime: &common.SDKTime{Time: parseTime(startTime)},
			EndTime:   &common.SDKTime{Time: parseTime(endTime)},
		},
	}

	outResp, err := monitoringClient.SummarizeMetricsData(ctx, outReq)
	if err == nil {
		for _, item := range outResp.Items {
			for _, dp := range item.AggregatedDatapoints {
				if dp.Value != nil {
					trafficData.Outbound = append(trafficData.Outbound, fmt.Sprintf("%.2f", *dp.Value/1024/1024))
				}
			}
		}
	}

	return trafficData, nil
}

func parseTime(timeStr string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", timeStr)
	if err != nil {
		return time.Now().Add(-1 * time.Hour)
	}
	return t
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
