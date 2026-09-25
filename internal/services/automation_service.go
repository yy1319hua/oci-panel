package services

import (
        "context"
        "encoding/json"
        "fmt"
        "log"
        "strings"
        "sync"
        "time"

        "github.com/adiecho/oci-panel/internal/database"
        "github.com/adiecho/oci-panel/internal/models"
        "github.com/google/uuid"
        "github.com/oracle/oci-go-sdk/v65/common"
        "github.com/oracle/oci-go-sdk/v65/computeinstanceagent"
        "github.com/oracle/oci-go-sdk/v65/core"
)

// AutomationService 承载三类自动化任务：保活（防回收）、抢机（OOC 重试）、定时备份。
// 任务循环挂在自己的 goroutine 上，每分钟扫一次到期任务；执行结果写回任务行并按需 TG 推送。
type AutomationService struct {
        oci      *OCIService
        telegram *TelegramService
        stopChan chan struct{}
        running  bool
        mutex    sync.Mutex
}

func NewAutomationService(oci *OCIService, telegram *TelegramService) *AutomationService {
        return &AutomationService{
                oci:      oci,
                telegram: telegram,
                stopChan: make(chan struct{}),
        }
}

// 免费形状白名单：抢机只允许 Always Free 资格内的形状，从源头杜绝意外计费。
var freeShapeWhitelist = map[string]bool{
        "VM.Standard.A1.Flex":     true,
        "VM.Standard.E2.1.Micro":  true,
}

func IsFreeShape(shape string) bool { return freeShapeWhitelist[shape] }

// BackupRetentionHardLimit 免费备份槽位硬上限（引导卷+块卷共享）。
const BackupRetentionHardLimit = 5

func (s *AutomationService) Start() {
        s.mutex.Lock()
        if s.running {
                s.mutex.Unlock()
                return
        }
        s.running = true
        s.stopChan = make(chan struct{})
        s.mutex.Unlock()
        go s.run()
        log.Println("Automation service started")
}

func (s *AutomationService) Stop() {
        s.mutex.Lock()
        defer s.mutex.Unlock()
        if !s.running {
                return
        }
        close(s.stopChan)
        s.running = false
        log.Println("Automation service stopped")
}

func (s *AutomationService) run() {
        // 启动后先等 30s，让面板自身初始化（DB、TG）稳定，再开始扫任务。
        select {
        case <-time.After(30 * time.Second):
        case <-s.stopChan:
                return
        }
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for {
                select {
                case <-ticker.C:
                        s.tick()
                case <-s.stopChan:
                        return
                }
        }
}

func (s *AutomationService) tick() {
        db := database.GetDB()
        now := time.Now()

        // ---- 保活任务 ----
        var kts []models.KeepaliveTask
        db.Where("enabled = ?", true).Find(&kts)
        for _, t := range kts {
                if t.LastRunAt == nil || now.Sub(*t.LastRunAt).Minutes() >= float64(maxInt(t.IntervalMin, 30)) {
                        go s.RunKeepaliveOnce(t)
                }
        }

        // ---- 抢机任务 ----
        var gts []models.GrabTask
        db.Where("enabled = ? AND status = ?", true, "running").Find(&gts)
        for _, t := range gts {
                if t.LastTryAt == nil || now.Sub(*t.LastTryAt).Minutes() >= float64(maxInt(t.IntervalMin, 1)) {
                        go s.TryGrabOnce(t)
                }
        }

        // ---- 备份任务 ----
        var bts []models.BackupTask
        db.Where("enabled = ?", true).Find(&bts)
        for _, t := range bts {
                if t.LastRunAt == nil || now.Sub(*t.LastRunAt).Hours() >= float64(maxInt(t.IntervalHour, 6)) {
                        go s.RunBackupOnce(t)
                }
        }

        // ---- 流量超额告警 ----（只读本地缓存，不打 OCI API，每分钟检查无压力）
        s.CheckTrafficAlerts()
}

func maxInt(a, b int) int {
        if a > b {
                return a
        }
        return b
}

// ======================= 保活（防回收） =======================

// keepaliveCommand 生成保活负载脚本：CPU（dd 单核打满）+ 网络（Cloudflare 下载）。
// nohup + & 立即返回，agent 不会等到负载结束。
func keepaliveCommand(durationSec int) string {
        if durationSec <= 0 {
                durationSec = 120
        }
        if durationSec > 600 {
                durationSec = 600
        }
        return fmt.Sprintf(
                `nohup sh -c 'timeout %d dd if=/dev/zero of=/dev/null 2>/dev/null; timeout %d curl -s -o /dev/null "https://speed.cloudflare.com/__down?bytes=1048576000"' >/dev/null 2>&1 &`,
                durationSec, durationSec)
}

// getAgentCommandClient 构建 Oracle Cloud Agent（Run Command）客户端。
// 该客户端不做长期缓存：保活任务频率低，每次重建的成本可以接受。
func (s *AutomationService) getAgentCommandClient(user *models.OciUser) (computeinstanceagent.ComputeInstanceAgentClient, error) {
        provider, err := s.oci.GetConfigProvider(user)
        if err != nil {
                return computeinstanceagent.ComputeInstanceAgentClient{}, err
        }
        client, err := computeinstanceagent.NewComputeInstanceAgentClientWithConfigurationProvider(provider)
        if err != nil {
                return computeinstanceagent.ComputeInstanceAgentClient{}, err
        }
        return client, nil
}

func (s *AutomationService) RunKeepaliveOnce(task models.KeepaliveTask) {
        db := database.GetDB()
        var user models.OciUser
        if err := db.Where("id = ?", task.ConfigID).First(&user).Error; err != nil {
                db.Model(&task).Updates(map[string]interface{}{"status": "failed", "last_result": "配置不存在", "last_run_at": time.Now()})
                return
        }

        now := time.Now()
        db.Model(&task).Updates(map[string]interface{}{"status": "running", "last_run_at": now})

        client, err := s.getAgentCommandClient(&user)
        result := "ok"
        if err != nil {
                result = "客户端创建失败: " + err.Error()
        } else {
                cmdStr := keepaliveCommand(task.DurationSec)
                content := &computeinstanceagent.InstanceAgentCommandContent{
                        Source:        computeinstanceagent.InstanceAgentCommandSourceViaTextDetails{Text: &cmdStr},
                        Output:        computeinstanceagent.InstanceAgentCommandOutputViaTextDetails{},
                }
                req := computeinstanceagent.CreateInstanceAgentCommandRequest{
                        CreateInstanceAgentCommandDetails: computeinstanceagent.CreateInstanceAgentCommandDetails{
                                CompartmentId:             common.String(user.OciTenantID),
                                ExecutionTimeOutInSeconds: common.Int(task.DurationSec + 60),
                                Target:                    &computeinstanceagent.InstanceAgentCommandTarget{InstanceId: common.String(task.InstanceID)},
                                Content:                   content,
                                DisplayName:               common.String("oci-panel-keepalive"),
                        },
                }
                if _, e := client.CreateInstanceAgentCommand(context.Background(), req); e != nil {
                        result = strings.ReplaceAll(e.Error(), "\n", " ")
                        if len(result) > 250 {
                                result = result[:250]
                        }
                }
        }

        status := "idle"
        if result != "ok" {
                status = "failed"
        }
        db.Model(&task).Updates(map[string]interface{}{"status": status, "last_result": result})

        emoji := "✅"
        if status == "failed" {
                emoji = "⚠️"
        }
        msg := fmt.Sprintf("%s 保活执行\n实例: %s\n时长: %ds\n结果: %s", emoji, task.InstanceName, task.DurationSec, result)
        s.notify("保活任务", msg)
}

// ======================= 抢机（OOC 重试） =======================

func (s *AutomationService) TryGrabOnce(task models.GrabTask) {
        db := database.GetDB()
        var user models.OciUser
        if err := db.Where("id = ?", task.ConfigID).First(&user).Error; err != nil {
                db.Model(&task).Updates(map[string]interface{}{"enabled": false, "status": "paused", "last_error": "配置不存在"})
                return
        }

        now := time.Now()
        db.Model(&task).Update("last_try_at", now)

        // 防御：非免费形状直接暂停任务。
        if !IsFreeShape(task.Shape) {
                db.Model(&task).Updates(map[string]interface{}{"enabled": false, "status": "paused", "last_error": "非免费形状，任务已暂停"})
                return
        }

        instance, err := s.oci.LaunchInstance(context.Background(), &user, LaunchInstanceParams{
                CompartmentId:      user.OciTenantID,
                AvailabilityDomain: task.Ad,
                DisplayName:        task.InstanceName,
                ImageId:            task.ImageID,
                Shape:              task.Shape,
                SubnetId:           task.SubnetID,
                Ocpus:              float32(task.Ocpus),
                MemoryInGBs:        float32(task.MemoryGB),
                SshPublicKey:       task.SSHKeyContent,
                BootVolumeSizeGBs:  int64(minInt(task.BootVolumeGB, 200)),
        })
        if err != nil {
                errMsg := strings.ReplaceAll(err.Error(), "\n", " ")
                if len(errMsg) > 490 {
                        errMsg = errMsg[:490]
                }
                updates := map[string]interface{}{"last_error": errMsg}
                db.Model(&task).Updates(updates)
                db.Model(&task).UpdateColumn("try_count", task.TryCount+1)
                return
        }

        // 抢到了！
        db.Model(&task).Updates(map[string]interface{}{
                "status":      "success",
                "enabled":     false,
                "instance_id": instance.Id,
                "last_error":  "",
        })
        db.Model(&task).UpdateColumn("try_count", task.TryCount+1)
        s.notify("抢机成功🎉",
                fmt.Sprintf("任务: %s\n实例: %s\n形状: %s\nOCID: %s\n\n请尽快登录面板设置 SSH 密钥并完成初始化。", task.Name, task.InstanceName, task.Shape, *instance.Id))
}

func minInt(a, b int) int {
        if a < b {
                return a
        }
        return b
}

// ======================= 定时备份 =======================

// RunBackupOnce 执行一次备份：先滚动删除最旧的超额备份，再创建增量备份。
// 滚动删除保证任意时刻该卷的备份数 <= Retention <= 5，永不触碰付费区间。
func (s *AutomationService) RunBackupOnce(task models.BackupTask) {
        db := database.GetDB()
        var user models.OciUser
        if err := db.Where("id = ?", task.ConfigID).First(&user).Error; err != nil {
                db.Model(&task).Updates(map[string]interface{}{"enabled": false, "last_result": "配置不存在"})
                return
        }

        now := time.Now()
        db.Model(&task).Update("last_run_at", now)

        bsClient, err := s.oci.GetBlockstorageClient(&user)
        if err != nil {
                db.Model(&task).Update("last_result", "客户端创建失败: "+err.Error())
                return
        }
        ctx := context.Background()

        // 判断该卷是引导卷还是块卷：查询租户引导卷列表比对 OCID。
        isBoot := false
        {
                resp, e := bsClient.ListBootVolumes(ctx, core.ListBootVolumesRequest{
                        CompartmentId: common.String(user.OciTenantID),
                })
                if e == nil {
                        for _, v := range resp.Items {
                                if *v.Id == task.VolumeID {
                                        isBoot = true
                                        break
                                }
                        }
                }
        }

        retention := task.Retention
        if retention > BackupRetentionHardLimit {
                retention = BackupRetentionHardLimit
        }
        if retention < 1 {
                retention = 1
        }

        // 现有备份（按时间升序），超出保留数的最旧部分先删。
        type backupItem struct{ id, name string }
        var existing []backupItem
        if isBoot {
                resp, e := bsClient.ListBootVolumeBackups(ctx, core.ListBootVolumeBackupsRequest{
                        CompartmentId: common.String(user.OciTenantID),
                        BootVolumeId:  common.String(task.VolumeID),
                })
                if e != nil {
                        db.Model(&task).Update("last_result", "列出备份失败: "+e.Error())
                        return
                }
                for _, b := range resp.Items {
                        existing = append(existing, backupItem{*b.Id, *b.DisplayName})
                }
        } else {
                resp, e := bsClient.ListVolumeBackups(ctx, core.ListVolumeBackupsRequest{
                        CompartmentId: common.String(user.OciTenantID),
                        VolumeId:      common.String(task.VolumeID),
                })
                if e != nil {
                        db.Model(&task).Update("last_result", "列出备份失败: "+e.Error())
                        return
                }
                for _, b := range resp.Items {
                        existing = append(existing, backupItem{*b.Id, *b.DisplayName})
                }
        }

        // 保留最近 retention 份，其余删除。
        delCount := len(existing) - retention + 1 // +1：待会还要新建一份
        for i := 0; i < delCount && i < len(existing); i++ {
                if isBoot {
                        _ , _ = bsClient.DeleteBootVolumeBackup(ctx, core.DeleteBootVolumeBackupRequest{BootVolumeBackupId: common.String(existing[i].id)})
                } else {
                        _, _ = bsClient.DeleteVolumeBackup(ctx, core.DeleteVolumeBackupRequest{VolumeBackupId: common.String(existing[i].id)})
                }
        }

        // 创建增量备份。
        displayName := fmt.Sprintf("%s-%s", task.VolumeName, now.Format("0102-1504"))
        var createErr error
        if isBoot {
                _, createErr = bsClient.CreateBootVolumeBackup(ctx, core.CreateBootVolumeBackupRequest{
                        CreateBootVolumeBackupDetails: core.CreateBootVolumeBackupDetails{
                                BootVolumeId: common.String(task.VolumeID),
                                DisplayName:  common.String(displayName),
                                Type:         core.CreateBootVolumeBackupDetailsTypeIncremental,
                        },
                })
        } else {
                _, createErr = bsClient.CreateVolumeBackup(ctx, core.CreateVolumeBackupRequest{
                        CreateVolumeBackupDetails: core.CreateVolumeBackupDetails{
                                VolumeId:    common.String(task.VolumeID),
                                DisplayName: common.String(displayName),
                                Type:        core.CreateVolumeBackupDetailsTypeIncremental,
                        },
                })
        }

        result := "备份创建成功: " + displayName
        if createErr != nil {
                result = "备份创建失败: " + strings.ReplaceAll(createErr.Error(), "\n", " ")
        }
        if len(result) > 250 {
                result = result[:250]
        }
        db.Model(&task).Update("last_result", result)

        emoji := "✅"
        if createErr != nil {
                emoji = "⚠️"
        }
        s.notify("定时备份", fmt.Sprintf("%s 卷: %s\n结果: %s", emoji, task.VolumeName, result))
}

// ======================= 流量超额告警 =======================

// CheckTrafficAlerts 在月度流量缓存刷新后调用：超过阈值则 TG 告警。
// 每月每配置只告警一次，状态记录在 sys_setting（key: traffic_alert_state）。
func (s *AutomationService) CheckTrafficAlerts() {
        db := database.GetDB()
        var configs []models.OciUser
        db.Find(&configs)

        for _, cfg := range configs {
                var cache models.OciConfigCache
                if err := db.Where("config_id = ?", cfg.ID).First(&cache).Error; err != nil || cache.TrafficData == "" {
                        continue
                }
                var stats MonthlyTrafficStats
                if err := json.Unmarshal([]byte(cache.TrafficData), &stats); err != nil {
                        continue
                }
                ratio := s.trafficUsageRatio(&stats)
                if ratio < s.trafficAlertThreshold() {
                        // 未超阈值：若本月曾告警，清除标记（下月自动重新计）。
                        s.clearAlertState(cfg.ID)
                        continue
                }
                if s.alertAlreadySent(cfg.ID) {
                        continue
                }
                s.notify("⚠️ 流量超额告警",
                        fmt.Sprintf("配置: %s\n本月流量已达 %.1f%%（%s）\n请注意控制用量，避免产生账单。", cfg.TenantName, ratio*100, FormatBytes(stats.OutboundTraffic)))
                s.markAlertSent(cfg.ID)
        }
}

func (s *AutomationService) trafficUsageRatio(stats *MonthlyTrafficStats) float64 {
        // Always Free 出站月额度 10TB。
        const freeLimitBytes = int64(10 * 1024 * 1024 * 1024 * 1024)
        if stats == nil || stats.OutboundTraffic <= 0 {
                return 0
        }
        return float64(stats.OutboundTraffic) / float64(freeLimitBytes)
}

func (s *AutomationService) trafficAlertThreshold() float64 {
        // 阈值存 sys_setting，默认 80%。
        var setting models.SysSetting
        if err := database.GetDB().Where("`key` = ?", "traffic_alert_threshold").First(&setting).Error; err == nil {
                var v float64
                if err := json.Unmarshal([]byte(setting.Value), &v); err == nil && v > 0 && v <= 100 {
                        return v / 100
                }
        }
        return 0.8
}

func (s *AutomationService) alertKeyMonth() string { return time.Now().Format("2006-01") }

func (s *AutomationService) alertStateKey(configID string) string {
        return "traffic_alert_state:" + configID
}

func (s *AutomationService) alertAlreadySent(configID string) bool {
        var setting models.SysSetting
        if err := database.GetDB().Where("`key` = ?", s.alertStateKey(configID)).First(&setting).Error; err != nil {
                return false
        }
        return strings.Contains(setting.Value, s.alertKeyMonth())
}

func (s *AutomationService) markAlertSent(configID string) {
        db := database.GetDB()
        val := s.alertKeyMonth()
        var setting models.SysSetting
        if err := db.Where("`key` = ?", s.alertStateKey(configID)).First(&setting).Error; err != nil {
                setting = models.SysSetting{ID: uuid.New().String(), Key: s.alertStateKey(configID)}
        }
        setting.Value = val
        db.Save(&setting)
}

func (s *AutomationService) clearAlertState(configID string) {
        database.GetDB().Where("`key` = ?", s.alertStateKey(configID)).Delete(&models.SysSetting{})
}

// ResetAlertState 清除告警状态（configID 为空则清除全部），供设置页手动重置。
func (s *AutomationService) ResetAlertState(configID string) {
        if configID == "" {
                database.GetDB().Where("`key` LIKE ?", "traffic_alert_state:%").Delete(&models.SysSetting{})
                return
        }
        s.clearAlertState(configID)
}

// ListVolumesForBackup 列出可备份的卷（引导卷 + 块卷），供创建备份任务时选择。
func (s *AutomationService) ListVolumesForBackup(userID string) ([]map[string]interface{}, error) {
        var user models.OciUser
        if err := database.GetDB().Where("id = ?", userID).First(&user).Error; err != nil {
                return nil, err
        }
        client, err := s.oci.GetBlockstorageClient(&user)
        if err != nil {
                return nil, err
        }
        ctx := context.Background()
        out := make([]map[string]interface{}, 0, 8)

        if resp, err := client.ListBootVolumes(ctx, core.ListBootVolumesRequest{
                CompartmentId: common.String(user.OciTenantID)}); err == nil {
                for _, v := range resp.Items {
                        out = append(out, map[string]interface{}{
                                "id": *v.Id, "name": *v.DisplayName, "type": "boot", "sizeGB": *v.SizeInGBs,
                        })
                }
        }
        if resp, err := client.ListVolumes(ctx, core.ListVolumesRequest{
                CompartmentId: common.String(user.OciTenantID)}); err == nil {
                for _, v := range resp.Items {
                        out = append(out, map[string]interface{}{
                                "id": *v.Id, "name": *v.DisplayName, "type": "block", "sizeGB": *v.SizeInGBs,
                        })
                }
        }
        return out, nil
}

// ======================= 配额总览 =======================

// QuotaOverview 一个配置的 Always Free 资源使用总览。
type QuotaOverview struct {
        ConfigID       string `json:"configId"`
        TenantName     string `json:"tenantName"`
        A1OcpusUsed    int    `json:"a1OcpusUsed"`    // A1.Flex 已用 OCPU（上限 4）
        A1MemoryUsed   int    `json:"a1MemoryUsed"`   // A1.Flex 已用内存 GB（上限 24）
        E2MicroUsed    int    `json:"e2MicroUsed"`    // E2.1.Micro 实例数（上限 2）
        BootVolumeGB   int64  `json:"bootVolumeGB"`   // 引导卷总容量（上限 200）
        BackupCount    int    `json:"backupCount"`    // 现有备份总数（上限 5）
        InstanceCount  int    `json:"instanceCount"`  // 运行中实例数
}

// GetQuotaOverview 汇总单个配置的资源使用情况（数据全部来自缓存/轻量 API，避免压 OCI）。
func (s *AutomationService) GetQuotaOverview(user models.OciUser) (*QuotaOverview, error) {
        db := database.GetDB()
        ov := &QuotaOverview{ConfigID: user.ID, TenantName: user.TenantName}

        var cache models.OciConfigCache
        if err := db.Where("config_id = ?", user.ID).First(&cache).Error; err != nil {
                return nil, fmt.Errorf("缓存不存在，请先刷新配置缓存")
        }

        // 从实例缓存统计 A1/E2 使用量。
        if cache.InstancesData != "" {
                var infos []models.InstanceInfo
                if err := json.Unmarshal([]byte(cache.InstancesData), &infos); err == nil {
                        for _, in := range infos {
                                ov.InstanceCount++
                                switch {
                                case strings.Contains(in.Shape, "A1.Flex"):
                                        ov.A1OcpusUsed += int(in.Ocpus)
                                        ov.A1MemoryUsed += int(in.Memory)
                                case strings.Contains(in.Shape, "E2.1.Micro"):
                                        ov.E2MicroUsed++
                                }
                                ov.BootVolumeGB += in.BootVolumeSize
                        }
                }
        }

        // 备份计数走轻量 API。
        bsClient, err := s.oci.GetBlockstorageClient(&user)
        if err == nil {
                ctx := context.Background()
                if resp, e := bsClient.ListBootVolumeBackups(ctx, core.ListBootVolumeBackupsRequest{
                        CompartmentId: common.String(user.OciTenantID)}); e == nil {
                        ov.BackupCount += len(resp.Items)
                }
                if resp, e := bsClient.ListVolumeBackups(ctx, core.ListVolumeBackupsRequest{
                        CompartmentId: common.String(user.OciTenantID)}); e == nil {
                        ov.BackupCount += len(resp.Items)
                }
        }
        return ov, nil
}

// ListQuotaOverviews 返回全部配置的配额总览。
func (s *AutomationService) ListQuotaOverviews() []QuotaOverview {
        db := database.GetDB()
        var configs []models.OciUser
        db.Find(&configs)
        out := make([]QuotaOverview, 0, len(configs))
        for _, cfg := range configs {
                if ov, err := s.GetQuotaOverview(cfg); err == nil {
                        out = append(out, *ov)
                }
        }
        return out
}

// GetCpuMemoryMetrics 透传到 OCIService 的 CPU/内存监控查询。
func (s *AutomationService) GetCpuMemoryMetrics(userID, instanceId string, hours int) (*CpuMemoryMetrics, error) {
        var user models.OciUser
        if err := database.GetDB().Where("id = ?", userID).First(&user).Error; err != nil {
                return nil, err
        }
        return s.oci.GetCpuMemoryMetrics(context.Background(), &user, instanceId, hours)
}