package controllers

import (
        "log"
        "strconv"

        "github.com/adiecho/oci-panel/internal/models"
        "github.com/adiecho/oci-panel/internal/services"
        "github.com/gin-gonic/gin"
        "github.com/google/uuid"
)

// AutomationController 自动化任务（保活/抢机/备份/告警/配额）HTTP 接口。
type AutomationController struct {
        auto *services.AutomationService
}

func NewAutomationController(auto *services.AutomationService) *AutomationController {
        return &AutomationController{auto: auto}
}

func resp(c *gin.Context, data interface{}, err error) {
        if err != nil {
                c.JSON(200, models.ErrorResponse(500, err.Error()))
                return
        }
        c.JSON(200, models.SuccessResponse(data, ""))
}

// ---------- 保活 ----------

// GET /api/automation/keepalive/list
func (ctl *AutomationController) ListKeepalive(c *gin.Context) {
        var tasks []models.KeepaliveTask
        err := db().Find(&tasks).Error
        resp(c, tasks, err)
}

// POST /api/automation/keepalive/save
func (ctl *AutomationController) SaveKeepalive(c *gin.Context) {
        var in struct {
                ID           uint   `json:"id"`
                ConfigID     string `json:"configId" binding:"required"`
                InstanceID   string `json:"instanceId" binding:"required"`
                InstanceName string `json:"instanceName"`
                Enabled      bool   `json:"enabled"`
                IntervalMin  int    `json:"intervalMin"`
                DurationSec  int    `json:"durationSec"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
                resp(c, nil, err)
                return
        }
        if in.IntervalMin < 30 {
                in.IntervalMin = 30
        }
        if in.DurationSec <= 0 {
                in.DurationSec = 120
        }
        if in.DurationSec > 600 {
                in.DurationSec = 600
        }
        db := db()
        var t models.KeepaliveTask
        if in.ID > 0 {
                if err := db.First(&t, in.ID).Error; err != nil {
                        resp(c, nil, err)
                        return
                }
        } else {
                t = models.KeepaliveTask{ConfigID: in.ConfigID, InstanceID: in.InstanceID}
        }
        t.ConfigID = in.ConfigID
        t.InstanceID = in.InstanceID
        t.InstanceName = in.InstanceName
        t.Enabled = in.Enabled
        t.IntervalMin = in.IntervalMin
        t.DurationSec = in.DurationSec
        err := db.Save(&t).Error
        resp(c, t, err)
}

// POST /api/automation/keepalive/delete
func (ctl *AutomationController) DeleteKeepalive(c *gin.Context) {
        var in struct {
                ID uint `json:"id"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.ID == 0 {
                resp(c, nil, errIDRequired())
                return
        }
        resp(c, nil, db().Delete(&models.KeepaliveTask{}, in.ID).Error)
}

// POST /api/automation/keepalive/run
func (ctl *AutomationController) RunKeepalive(c *gin.Context) {
        var in struct {
                ID uint `json:"id"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.ID == 0 {
                resp(c, nil, errIDRequired())
                return
        }
        var t models.KeepaliveTask
        if err := db().First(&t, in.ID).Error; err != nil {
                resp(c, nil, err)
                return
        }
        go ctl.auto.RunKeepaliveOnce(t)
        resp(c, nil, nil)
}

// ---------- 抢机 ----------

// GET /api/automation/grab/list
func (ctl *AutomationController) ListGrab(c *gin.Context) {
        var tasks []models.GrabTask
        err := db().Find(&tasks).Error
        resp(c, tasks, err)
}

// POST /api/automation/grab/save
func (ctl *AutomationController) SaveGrab(c *gin.Context) {
        var in struct {
                ID            uint    `json:"id"`
                ConfigID      string  `json:"configId" binding:"required"`
                Name          string  `json:"name" binding:"required"`
                Ad            string  `json:"ad" binding:"required"`
                Shape         string  `json:"shape" binding:"required"`
                Ocpus         int     `json:"ocpus"`
                MemoryGB      int     `json:"memoryGB"`
                BootVolumeGB  int     `json:"bootVolumeGB"`
                ImageID       string  `json:"imageId" binding:"required"`
                SubnetID      string  `json:"subnetId" binding:"required"`
                SSHKeyContent string  `json:"sshKeyContent"`
                InstanceName  string  `json:"instanceName"`
                IntervalMin   int     `json:"intervalMin"`
                Enabled       bool    `json:"enabled"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
                resp(c, nil, err)
                return
        }
        // 防收费硬校验：形状必须在 Always Free 白名单内
        if !services.IsFreeShape(in.Shape) {
                resp(c, nil, errFreeShape())
                return
        }
        if in.Ocpus < 1 {
                in.Ocpus = 1
        }
        if in.Ocpus > 4 {
                in.Ocpus = 4
        }
        if in.MemoryGB < 1 {
                in.MemoryGB = 6
        }
        if in.MemoryGB > 24 {
                in.MemoryGB = 24
        }
        if in.BootVolumeGB <= 0 {
                in.BootVolumeGB = 50
        }
        if in.BootVolumeGB > 200 {
                in.BootVolumeGB = 200
        }
        if in.IntervalMin < 1 {
                in.IntervalMin = 5
        }
        if in.InstanceName == "" {
                in.InstanceName = in.Name
        }
        db := db()
        var t models.GrabTask
        if in.ID > 0 {
                if err := db.First(&t, in.ID).Error; err != nil {
                        resp(c, nil, err)
                        return
                }
        } else {
                t = models.GrabTask{ConfigID: in.ConfigID, Name: in.Name}
        }
        t.ConfigID = in.ConfigID
        t.Ad, t.Shape, t.Ocpus, t.MemoryGB, t.BootVolumeGB = in.Ad, in.Shape, in.Ocpus, in.MemoryGB, in.BootVolumeGB
        t.ImageID, t.SubnetID, t.InstanceName, t.IntervalMin, t.Enabled = in.ImageID, in.SubnetID, in.InstanceName, in.IntervalMin, in.Enabled
        if in.SSHKeyContent != "" {
                t.SSHKeyContent = in.SSHKeyContent
        }
        err := db.Save(&t).Error
        resp(c, t, err)
}

// POST /api/automation/grab/delete
func (ctl *AutomationController) DeleteGrab(c *gin.Context) {
        var in struct {
                ID uint `json:"id"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.ID == 0 {
                resp(c, nil, errIDRequired())
                return
        }
        resp(c, nil, db().Delete(&models.GrabTask{}, in.ID).Error)
}

// POST /api/automation/grab/run
func (ctl *AutomationController) RunGrab(c *gin.Context) {
        var in struct {
                ID uint `json:"id"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.ID == 0 {
                resp(c, nil, errIDRequired())
                return
        }
        var t models.GrabTask
        if err := db().First(&t, in.ID).Error; err != nil {
                resp(c, nil, err)
                return
        }
        go ctl.auto.TryGrabOnce(t)
        resp(c, nil, nil)
}

// ---------- 备份 ----------

// GET /api/automation/backup/list
func (ctl *AutomationController) ListBackup(c *gin.Context) {
        var tasks []models.BackupTask
        err := db().Find(&tasks).Error
        resp(c, tasks, err)
}

// POST /api/automation/backup/save
func (ctl *AutomationController) SaveBackup(c *gin.Context) {
        var in struct {
                ID           uint   `json:"id"`
                ConfigID     string `json:"configId" binding:"required"`
                VolumeID     string `json:"volumeId" binding:"required"`
                VolumeName   string `json:"volumeName"`
                Retention    int    `json:"retention"`
                IntervalHour int    `json:"intervalHour"`
                Enabled      bool   `json:"enabled"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
                resp(c, nil, err)
                return
        }
        // ⚠️ 免费额度硬校验：全租户最多 5 个备份槽，超出即计费。
        if in.Retention < 1 {
                in.Retention = 3
        }
        if in.Retention > 5 {
                resp(c, nil, errBackupLimit())
                return
        }
        if in.IntervalHour < 6 {
                in.IntervalHour = 24
        }
        db := db()
        var t models.BackupTask
        if in.ID > 0 {
                if err := db.First(&t, in.ID).Error; err != nil {
                        resp(c, nil, err)
                        return
                }
        } else {
                t = models.BackupTask{ConfigID: in.ConfigID, VolumeID: in.VolumeID}
        }
        t.ConfigID = in.ConfigID
        t.VolumeID = in.VolumeID
        t.VolumeName, t.Retention, t.IntervalHour, t.Enabled = in.VolumeName, in.Retention, in.IntervalHour, in.Enabled
        err := db.Save(&t).Error
        resp(c, t, err)
}

// POST /api/automation/backup/delete
func (ctl *AutomationController) DeleteBackup(c *gin.Context) {
        var in struct {
                ID uint `json:"id"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.ID == 0 {
                resp(c, nil, errIDRequired())
                return
        }
        resp(c, nil, db().Delete(&models.BackupTask{}, in.ID).Error)
}

// POST /api/automation/backup/run
func (ctl *AutomationController) RunBackup(c *gin.Context) {
        var in struct {
                ID uint `json:"id"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.ID == 0 {
                resp(c, nil, errIDRequired())
                return
        }
        var t models.BackupTask
        if err := db().First(&t, in.ID).Error; err != nil {
                resp(c, nil, err)
                return
        }
        go ctl.auto.RunBackupOnce(t)
        resp(c, nil, nil)
}

// ---------- 告警阈值 / 配额 ----------

// GET /api/automation/settings
func (ctl *AutomationController) GetSettings(c *gin.Context) {
        out := map[string]interface{}{"trafficAlertThreshold": 80, "cpuAlertThreshold": 0}
        var setting models.SysSetting
        if err := db().Where("`key` = ?", "traffic_alert_threshold").First(&setting).Error; err == nil {
                var v float64
                if jsonUnmarshal(setting.Value, &v) == nil && v > 0 && v <= 100 {
                        out["trafficAlertThreshold"] = v
                }
        }
        if err := db().Where("`key` = ?", "cpu_alert_threshold").First(&setting).Error; err == nil {
                var v float64
                if jsonUnmarshal(setting.Value, &v) == nil && v >= 0 && v <= 100 {
                        out["cpuAlertThreshold"] = v
                }
        }
        resp(c, out, nil)
}

// POST /api/automation/settings
func (ctl *AutomationController) SaveSettings(c *gin.Context) {
        var in struct {
                TrafficAlertThreshold float64 `json:"trafficAlertThreshold"`
                CpuAlertThreshold     *float64 `json:"cpuAlertThreshold"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.TrafficAlertThreshold <= 0 || in.TrafficAlertThreshold > 100 {
                resp(c, nil, errInvalidThreshold())
                return
        }
        db := db()
        saveSetting := func(key string, val float64) {
                var setting models.SysSetting
                if err := db.Where("`key` = ?", key).First(&setting).Error; err != nil {
                        setting = models.SysSetting{ID: uuid.New().String(), Key: key}
                } else if setting.ID == "" {
                        // 历史数据可能存在空主键行：空 ID 会让 Save() 退化为 INSERT 并因主键冲突静默失败
                        setting.ID = uuid.New().String()
                }
                setting.Value = strconv.FormatFloat(val, 'f', -1, 64)
                if err := db.Save(&setting).Error; err != nil {
                        log.Printf("save setting %s failed: %v", key, err)
                }
        }
        saveSetting("traffic_alert_threshold", in.TrafficAlertThreshold)
        if in.CpuAlertThreshold != nil && *in.CpuAlertThreshold >= 0 && *in.CpuAlertThreshold <= 100 {
                saveSetting("cpu_alert_threshold", *in.CpuAlertThreshold)
        }
        resp(c, nil, nil)
}

// GET /api/automation/quota
func (ctl *AutomationController) GetQuota(c *gin.Context) {
        resp(c, ctl.auto.ListQuotaOverviews(), nil)
}

// GET /api/automation/backup/volumes?configId=
func (ctl *AutomationController) ListVolumes(c *gin.Context) {
        vols, err := ctl.auto.ListVolumesForBackup(c.Query("configId"))
        resp(c, vols, err)
}

// POST /api/automation/alert/clear
// 重置告警状态（例如阈值调高后想让本月重新检查）
func (ctl *AutomationController) ClearAlertState(c *gin.Context) {
        configID := c.Query("configId")
        ctl.auto.ResetAlertState(configID)
        resp(c, nil, nil)
}

// POST /api/automation/metrics/cpu-memory  {configId, instanceId, hours}
func (ctl *AutomationController) GetCpuMemory(c *gin.Context) {
        var in struct {
                ConfigID   string `json:"configId"`
                InstanceID string `json:"instanceId"`
                Hours      int    `json:"hours"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.InstanceID == "" || in.ConfigID == "" {
                resp(c, nil, errIDRequired())
                return
        }
        m, err := ctl.auto.GetCpuMemoryMetrics(in.ConfigID, in.InstanceID, in.Hours)
        resp(c, m, err)
}

// ---------- PushPlus 推送通道 ----------

// GET /api/automation/pushplus/get
func (ctl *AutomationController) GetPushplus(c *gin.Context) {
        token := services.GetPushplusToken()
        masked := ""
        if len(token) > 8 {
                masked = token[:4] + "****" + token[len(token)-4:]
        } else if token != "" {
                masked = "已配置"
        }
        resp(c, gin.H{"enabled": token != "", "tokenMasked": masked}, nil)
}

// POST /api/automation/pushplus/save  {token}
func (ctl *AutomationController) SavePushplus(c *gin.Context) {
        var in struct {
                Token string `json:"token"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
                resp(c, nil, err)
                return
        }
        resp(c, nil, services.SetPushplusToken(in.Token))
}

// POST /api/automation/pushplus/test
func (ctl *AutomationController) TestPushplus(c *gin.Context) {
        token := services.GetPushplusToken()
        if token == "" {
                resp(c, nil, errPushplusNotConfigured())
                return
        }
        resp(c, nil, services.SendPushplus(token, "OCI Panel 测试推送", "这是一条测试消息，收到即表示 PushPlus 通道配置成功 ✅"))
}

// ---------- 抢机表单下拉数据 ----------

// lookupService 复用全局 OCIService 查询下拉选项（构造时注入）。
var lookupService *services.OCIService

// SetLookupService 由 router.Setup 注入 ociService。
func SetLookupService(oci *services.OCIService) { lookupService = oci }

// GET /api/automation/lookup/ads?configId=
func (ctl *AutomationController) ListAds(c *gin.Context) {
        opts, err := lookupService.ListAvailabilityDomains(c.Query("configId"))
        resp(c, opts, err)
}

// GET /api/automation/lookup/images?configId=&shape=
func (ctl *AutomationController) ListImages(c *gin.Context) {
        opts, err := lookupService.ListImagesByShape(c.Query("configId"), c.Query("shape"), "")
        resp(c, opts, err)
}

// GET /api/automation/lookup/subnets?configId=
func (ctl *AutomationController) ListSubnets(c *gin.Context) {
        opts, err := lookupService.ListSubnets(c.Query("configId"))
        resp(c, opts, err)
}
