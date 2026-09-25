package models

import "time"

// KeepaliveTask 空闲防回收保活任务。
//
// 背景：OCI 会回收长期空闲（CPU/内存/网络均低于 20%）的 Always Free 实例。
// 本面板通过 OCI Run Command（免 agent、免费）定期向实例下发一条 CPU + 网络负载命令，
// 将实例从「空闲」判定中拉出来。Run Command 需要 OCI 侧 IAM 放行，UI 中提供可复制的策略文本。
type KeepaliveTask struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	ConfigID     string     `gorm:"size:64;not null;index" json:"configId"`
	InstanceID   string     `gorm:"size:128;not null;index" json:"instanceId"`
	InstanceName string     `gorm:"size:128" json:"instanceName"`
	Enabled      bool       `gorm:"default:false" json:"enabled"`
	IntervalMin  int        `gorm:"default:60" json:"intervalMin"`  // 执行间隔（分钟），最小 30
	DurationSec  int        `gorm:"default:120" json:"durationSec"` // 单次负载时长（秒），最长 600
	LastRunAt    *time.Time `json:"lastRunAt"`
	LastResult   string     `gorm:"size:255" json:"lastResult"` // 最近一次执行结果摘要
	Status       string     `gorm:"size:16;default:'idle'" json:"status"` // idle/running/failed
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func (KeepaliveTask) TableName() string { return "keepalive_task" }

// GrabTask 自动抢机任务（Out of Capacity 重试）。
//
// 热门区域常见 "Out of host capacity"，本任务按固定间隔反复调用 LaunchInstance，
// 成功后立即 TG 通知。仅允许 Always Free 资格内的形状，防止意外计费。
type GrabTask struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	ConfigID      string     `gorm:"size:64;not null;index" json:"configId"`
	Name          string     `gorm:"size:128;not null" json:"name"`
	Ad            string     `gorm:"size:128;not null" json:"ad"` // 可用域名，如 AP-CHUNCHEON-1-AD-1
	Shape         string     `gorm:"size:64;not null" json:"shape"`
	Ocpus         int        `gorm:"default:1" json:"ocpus"`
	MemoryGB      int        `gorm:"default:6" json:"memoryGB"`
	BootVolumeGB  int        `gorm:"default:50" json:"bootVolumeGB"` // 上限 200（免费总额）
	ImageID       string     `gorm:"size:128;not null" json:"imageId"`
	SubnetID      string     `gorm:"size:128;not null" json:"subnetId"`
	SSHKeyContent string     `gorm:"text" json:"-"` // 公钥内容（非路径），仅创建时写入
	InstanceName  string     `gorm:"size:128" json:"instanceName"`
	IntervalMin   int        `gorm:"default:5" json:"intervalMin"` // 重试间隔（分钟），最小 1
	Enabled       bool       `gorm:"default:true" json:"enabled"`
	LastTryAt     *time.Time `json:"lastTryAt"`
	LastError     string     `gorm:"size:500" json:"lastError"`
	TryCount      int64      `gorm:"default:0" json:"tryCount"`
	InstanceID    string     `gorm:"size:128" json:"instanceId"` // 抢到后回填
	Status        string     `gorm:"size:16;default:'running'" json:"status"` // running/success/paused
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

func (GrabTask) TableName() string { return "grab_task" }

// BackupTask 定时卷备份任务。
//
// ⚠️ 免费额度：每租户最多 5 个卷备份槽（引导卷+块卷共享），超出即计费。
// 因此 Retention 硬上限 5（默认 3），执行时先删最旧再建新，滚动窗口永不超额。
type BackupTask struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	ConfigID     string     `gorm:"size:64;not null;index" json:"configId"`
	VolumeID     string     `gorm:"size:128;not null;index" json:"volumeId"`
	VolumeName   string     `gorm:"size:128" json:"volumeName"`
	Retention    int        `gorm:"default:3" json:"retention"`   // 保留份数，硬上限 5
	IntervalHour int        `gorm:"default:24" json:"intervalHour"` // 执行间隔（小时），最小 6
	Enabled      bool       `gorm:"default:false" json:"enabled"`
	LastRunAt    *time.Time `json:"lastRunAt"`
	LastResult   string     `gorm:"size:255" json:"lastResult"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func (BackupTask) TableName() string { return "backup_task" }
