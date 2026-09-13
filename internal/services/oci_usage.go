package services

import (
	"context"
	"fmt"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/usageapi"
)

// oci_usage.go — 每日成本查询（Oracle Usage API / RequestSummarizedUsages）。
//
// 目的：第一时间发现「超免费额度产生的扣费」。甲骨文账单按天聚合，数据本身有数小时延迟。
// 依赖权限：调用方所属用户组需 USAGE_REPORT_READ（或 USAGE_ANALYSIS_READ）。

// DailyCost 单日成本（金额为账号/租户维度合计，币种见 Currency）。
type DailyCost struct {
	Date     string  `json:"date"`
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// CostStats 成本统计结果。
type CostStats struct {
	Days        []DailyCost `json:"days"`
	MonthToDate float64     `json:"monthToDate"` // 本月累计
	Currency    string      `json:"currency"`
	DaysCount   int         `json:"daysCount"`
}

// GetDailyCost 查询近 days 天的每日成本（默认含今日）。
// granularity=DAILY + queryType=COST，按 timeUsageStarted 汇总 computedAmount。
// 若账号未授予 usage-report 读取权限，返回的 error 会明确提示，便于前端引导授权。
func (s *OCIService) GetDailyCost(ctx context.Context, user *models.OciUser, days int) (*CostStats, error) {
	if days <= 0 || days > 90 {
		days = 30 // Usage API DAILY 粒度最长 90 天
	}

	client, err := s.GetUsageapiClient(user)
	if err != nil {
		return nil, fmt.Errorf("创建用量客户端失败: %w", err)
	}

	// 时间窗口：从 days-1 天前的 00:00 UTC 到今天结束（含当日）。
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1))
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)

	tenantID := user.OciTenantID
	depth := float32(1) // 覆盖根 compartment 及其直属子 compartment

	req := usageapi.RequestSummarizedUsagesRequest{
		RequestSummarizedUsagesDetails: usageapi.RequestSummarizedUsagesDetails{
			TenantId:         &tenantID,
			TimeUsageStarted: &common.SDKTime{Time: start},
			TimeUsageEnded:   &common.SDKTime{Time: end},
			Granularity:      usageapi.RequestSummarizedUsagesDetailsGranularityDaily,
			QueryType:        usageapi.RequestSummarizedUsagesDetailsQueryTypeCost,
			GroupBy:          []string{"service"},
			CompartmentDepth: &depth,
		},
	}

	// 分页拉全（Usage API 默认每页有限，可能被截断）。
	dailyMap := map[string]float64{}
	currency := ""
	var page *string
	for {
		req.Page = page
		resp, err := client.RequestSummarizedUsages(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("用量查询失败(需 USAGE_REPORT_READ 权限): %w", err)
		}
		for _, item := range resp.Items {
			if item.TimeUsageStarted == nil {
				continue
			}
			day := item.TimeUsageStarted.UTC().Format("2006-01-02")
			if item.ComputedAmount != nil {
				dailyMap[day] += float64(*item.ComputedAmount)
			}
			if currency == "" && item.Currency != nil {
				currency = *item.Currency
			}
		}
		page = resp.OpcNextPage
		if page == nil {
			break
		}
	}

	// 补齐无数据的日期为 0，保证趋势连续。
	stats := &CostStats{Days: []DailyCost{}, Currency: currency}
	if stats.Currency == "" {
		stats.Currency = "USD" // 甲骨文计费默认美元
	}
	for i := 0; i < days; i++ {
		key := start.AddDate(0, 0, i).Format("2006-01-02")
		amount := dailyMap[key]
		stats.Days = append(stats.Days, DailyCost{Date: key, Amount: amount, Currency: stats.Currency})
		stats.MonthToDate += amount
	}
	stats.DaysCount = len(stats.Days)

	return stats, nil
}
