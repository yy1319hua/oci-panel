package services

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
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
	// 解析顺序很关键：带时区的格式必须先试，否则 "2026-01-02T15:04:05Z"
	// 会被无时区的 "2006-01-02T15:04:05" 抢先匹配掉（后者只是恰好不匹配 Z 后缀，
	// 但带偏移量的形式如 "+08:00" 就危险了）。
	zonedLayouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05Z07:00",
	}
	for _, layout := range zonedLayouts {
		if t, err := time.Parse(layout, timeStr); err == nil {
			return t.UTC()
		}
	}

	// 无时区的格式：前端 dateTime-local 输入控件产出的是**用户本地时间**，
	// 必须按本地时区解释后再转 UTC。若沿用 time.Parse（按 UTC 解释），
	// 服务器时区非 UTC 时用户选的 15:00 会被当成 UTC 15:00，
	// 与实际期望相差一整个时区，表现为「查不到数据」或区间错位。
	localLayouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04",
	}
	for _, layout := range localLayouts {
		if t, err := time.ParseInLocation(layout, timeStr, time.Local); err == nil {
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

	// 实例之间完全独立，改为并发处理。
	// 此前串行遍历：每个实例要发 ListVnicAttachments + GetVnic + 3 次
	// SummarizeMetricsData 共 5+ 次网络往返，多实例时耗时线性叠加（实测约 10s）。
	// 这里用带并发上限的 goroutine 池，把总耗时压到接近单个实例的水平。
	type instResult struct {
		stat      InstanceTrafficStat
		dailyIn   map[string]int64
		dailyOut  map[string]int64
		inbound   int64
		outbound  int64
		billable  int64
		hasResult bool
	}

	const trafficConcurrency = 4 // 并发上限：兼顾速度与 OCI API 限流
	sem := make(chan struct{}, trafficConcurrency)
	results := make([]instResult, len(instances))
	var wg sync.WaitGroup

	for i, instance := range instances {
		if instance.Id == nil {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}

		go func(idx int, inst core.Instance) {
			defer wg.Done()
			defer func() { <-sem }()

			res := instResult{
				dailyIn:  map[string]int64{},
				dailyOut: map[string]int64{},
			}
			res.stat = InstanceTrafficStat{InstanceID: *inst.Id}
			if inst.DisplayName != nil {
				res.stat.DisplayName = *inst.DisplayName
			}

			// 获取实例的 VNIC 附件
			vnicAttachReq := core.ListVnicAttachmentsRequest{
				CompartmentId: &compartmentId,
				InstanceId:    inst.Id,
			}
			vnicAttachResp, err := computeClient.ListVnicAttachments(ctx, vnicAttachReq)
			if err != nil {
				results[idx] = res
				return
			}

			for _, attach := range vnicAttachResp.Items {
				if attach.VnicId == nil {
					continue
				}

				vnicReq := core.GetVnicRequest{VnicId: attach.VnicId}
				vnicResp, err := vnClient.GetVnic(ctx, vnicReq)
				if err != nil || vnicResp.Id == nil {
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
							res.inbound += v
							res.stat.Inbound += v
							if dp.Timestamp != nil {
								res.dailyIn[dp.Timestamp.UTC().Format("2006-01-02")] += v
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
							res.outbound += v
							res.stat.Outbound += v
							if dp.Timestamp != nil {
								res.dailyOut[dp.Timestamp.UTC().Format("2006-01-02")] += v
							}
						}
					}
				}

				// 计费出站（best-effort）：VnicBillableBytesOut（免费额度外才计费）
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
								res.billable += v
								res.stat.Billable += v
							}
						}
					}
				}
			}

			res.hasResult = true
			results[idx] = res
		}(i, instance)
	}

	wg.Wait()

	// 按原顺序汇总并发结果，保证输出稳定（并发写 map 不安全，故在汇总阶段串行合并）。
	for _, res := range results {
		if res.stat.InstanceID == "" {
			continue
		}
		stats.InboundTraffic += res.inbound
		stats.OutboundTraffic += res.outbound
		stats.BillableTraffic += res.billable
		stats.Instances = append(stats.Instances, res.stat)
		for d, v := range res.dailyIn {
			dailyInbound[d] += v
		}
		for d, v := range res.dailyOut {
			dailyOutbound[d] += v
		}
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
