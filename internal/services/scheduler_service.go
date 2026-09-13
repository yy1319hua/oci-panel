package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/google/uuid"
	"github.com/oracle/oci-go-sdk/v65/core"
)

const (
	SettingCacheEnabled  = "cache_enabled"
	SettingCacheInterval = "cache_interval"
)

// TrafficRefreshFactor 决定流量缓存的刷新频率相对普通缓存的倍数。
//
// 月度流量查询要为每个实例打 5+ 次 OCI API（实测约 10s），成本比实例列表高一个
// 数量级。若与实例同频刷新，缓存间隔设为 5 分钟时会变成持续压 OCI 接口、且每次
// 都占用调度协程。因此流量按「间隔 × 本倍数」刷新，默认值 6 表示
// 30 分钟间隔 → 流量约每 3 小时刷新一次。
//
// 这不影响用户看到最新数据：首页手动点「刷新」会带 forceRefresh 实时查询。
//
// ⚠️ 与之配对的是 controllers 里的 trafficCacheTTL（缓存读取侧的有效期）。
// 两者必须满足 trafficCacheTTL > 间隔 × TrafficRefreshFactor，否则控制器会在
// 调度器尚未刷新时就判定缓存过期，每个请求都穿透到实时查询。改这里时请同步核对。
const TrafficRefreshFactor = 6

// TrafficRefreshInterval 返回流量的实际刷新间隔。
// 单独抽出来是为了让「调度器何时刷」与「控制器何时认为过期」有唯一权威来源。
func (s *SchedulerService) TrafficRefreshInterval() time.Duration {
	return time.Duration(s.GetCacheInterval()*TrafficRefreshFactor) * time.Minute
}

type SchedulerService struct {
	ociService *OCIService
	stopChan   chan struct{}
	running    bool
	mutex      sync.Mutex
}

func NewSchedulerService(ociService *OCIService) *SchedulerService {
	return &SchedulerService{
		ociService: ociService,
		stopChan:   make(chan struct{}),
	}
}

func (s *SchedulerService) Start() {
	s.mutex.Lock()
	if s.running {
		s.mutex.Unlock()
		return
	}
	s.running = true
	s.stopChan = make(chan struct{})
	s.mutex.Unlock()

	go s.run()
	log.Println("Scheduler service started")
}

func (s *SchedulerService) Stop() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.running {
		return
	}
	close(s.stopChan)
	s.running = false
	log.Println("Scheduler service stopped")
}

func (s *SchedulerService) run() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.checkAndRunTask()
		}
	}
}

func (s *SchedulerService) checkAndRunTask() {
	if !s.IsCacheEnabled() {
		return
	}

	interval := s.GetCacheInterval()
	if interval <= 0 {
		interval = 30
	}

	db := database.GetDB()
	var configs []models.OciUser
	db.Find(&configs)

	if len(configs) == 0 {
		return
	}

	now := time.Now()
	needUpdate := false

	for _, cfg := range configs {
		var cache models.OciConfigCache
		result := db.Where("config_id = ?", cfg.ID).First(&cache)
		if result.Error != nil || now.Sub(cache.UpdateTime).Minutes() >= float64(interval) {
			needUpdate = true
			break
		}
	}

	if needUpdate {
		go s.updateAllCaches(configs)
	}
}

func (s *SchedulerService) updateAllCaches(configs []models.OciUser) {
	rand.Shuffle(len(configs), func(i, j int) {
		configs[i], configs[j] = configs[j], configs[i]
	})

	semaphore := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, cfg := range configs {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(config models.OciUser) {
			defer wg.Done()
			defer func() { <-semaphore }()

			delay := time.Duration(1+rand.Intn(10)) * time.Second
			time.Sleep(delay)

			s.UpdateConfigCache(config.ID)
		}(cfg)
	}

	wg.Wait()
	log.Println("All config caches updated")
}

// UpdateConfigCache 刷新单个配置的缓存。
//
// 关键语义（修复「缓存空壳」问题）：只有**成功拿到数据**的部分才写入缓存。
// 任何一项 OCI 调用失败都保留该字段的旧值；若所有调用都失败，则完全不写库、
// 不推进 UpdateTime —— 这样下一次调度会立即重试，而不是把空数据当成
// 「刚更新过」缓存 30 分钟，导致首页长期读到空列表。
//
// 并发执行互相独立的 OCI 调用（实例/卷/VCN/租户，外加按低频刷新的流量），
// 把串行的多段网络往返压缩为一段，显著缩短刷新耗时。
func (s *SchedulerService) UpdateConfigCache(configID string) error {
	db := database.GetDB()
	var user models.OciUser
	if err := db.Where("id = ?", configID).First(&user).Error; err != nil {
		return err
	}

	ctx := context.Background()
	compartmentId := user.OciTenantID

	var cache models.OciConfigCache
	result := db.Where("config_id = ?", configID).First(&cache)
	if result.Error != nil {
		cache = models.OciConfigCache{
			ID:       uuid.New().String(),
			ConfigID: configID,
		}
	}

	// 并发抓取四类资源，各自独立成败。
	var (
		wg          sync.WaitGroup
		instCount   int
		runningCnt  int
		instancesJS string
		volumesJS   string
		vcnsJS      string
		tenantJS    string
		trafficJS   string
		tenantInfo  *models.TenantInfo
		okInstances bool
		okVolumes   bool
		okVcns      bool
		okTenant    bool
		okTraffic   bool
	)

	// 流量单独计时：距上次刷新不足 TrafficRefreshInterval 则跳过，
	// 避免每次调度都触发一次高成本的 OCI 流量查询。
	refreshTraffic := cache.TrafficData == "" ||
		time.Since(cache.TrafficUpdateTime) >= s.TrafficRefreshInterval()

	wg.Add(4)
	if refreshTraffic {
		wg.Add(1)
	}

	go func() {
		defer wg.Done()
		instances, err := s.ociService.ListInstances(ctx, &user, compartmentId)
		if err != nil {
			return
		}
		running := 0
		for _, inst := range instances {
			if inst.LifecycleState == core.InstanceLifecycleStateRunning {
				running++
			}
		}
		instanceInfos := s.ociService.GetInstancesDetailsConcurrent(ctx, &user, instances)
		data, err := json.Marshal(instanceInfos)
		if err != nil {
			return
		}
		instCount = len(instances)
		runningCnt = running
		instancesJS = string(data)
		okInstances = true
	}()

	go func() {
		defer wg.Done()
		volumes, err := s.ociService.ListBootVolumes(ctx, &user, compartmentId)
		if err != nil {
			return
		}
		data, err := json.Marshal(volumes)
		if err != nil {
			return
		}
		volumesJS = string(data)
		okVolumes = true
	}()

	go func() {
		defer wg.Done()
		vcns, err := s.ociService.ListVCNs(ctx, &user, compartmentId)
		if err != nil {
			return
		}
		data, err := json.Marshal(vcns)
		if err != nil {
			return
		}
		vcnsJS = string(data)
		okVcns = true
	}()

	go func() {
		defer wg.Done()
		info, err := s.ociService.GetTenantInfo(ctx, &user)
		if err != nil {
			return
		}
		data, err := json.Marshal(info)
		if err != nil {
			return
		}
		tenantJS = string(data)
		tenantInfo = info
		okTenant = true
	}()

	if refreshTraffic {
		go func() {
			defer wg.Done()
			traffic, err := s.ociService.GetMonthlyTrafficStats(ctx, &user)
			if err != nil {
				return
			}
			data, err := json.Marshal(traffic)
			if err != nil {
				return
			}
			trafficJS = string(data)
			okTraffic = true
		}()
	}

	wg.Wait()

	// 全失败：不写库、不推进时间戳，让下次调度尽快重试。
	//
	// 这里刻意不把 okTraffic 写进判定条件：refreshTraffic=false 时流量调用
	// 根本没发起，它恒为 false，若算进去会让「四项基础调用其实成功了、
	// 只是流量被跳过」的情形被误报成全部失败，把排查方向带偏。
	if !okInstances && !okVolumes && !okVcns && !okTenant {
		return fmt.Errorf("refresh cache for %s failed: all base OCI calls errored", configID)
	}

	// 逐项按「成功才覆盖」写入，失败项保留旧值。
	if okInstances {
		cache.InstanceCount = instCount
		cache.RunningInstances = runningCnt
		cache.InstancesData = instancesJS
	}
	if okVolumes {
		cache.VolumesData = volumesJS
	}
	if okVcns {
		cache.VcnsData = vcnsJS
	}
	if okTenant {
		cache.TenantData = tenantJS
		// 同步更新 OciUser 表中的租户名称和创建时间
		updateFields := map[string]interface{}{}
		if tenantInfo != nil && tenantInfo.Name != "" && tenantInfo.Name != user.TenantName {
			updateFields["tenant_name"] = tenantInfo.Name
		}
		if tenantInfo != nil && tenantInfo.CreateTime != "" && user.TenantCreateTime == nil {
			if parsedTime, err := time.Parse("2006-01-02 15:04:05", tenantInfo.CreateTime); err == nil {
				updateFields["tenant_create_time"] = parsedTime
			}
		}
		if len(updateFields) > 0 {
			db.Model(&user).Updates(updateFields)
		}
	}

	if okTraffic {
		cache.TrafficData = trafficJS
		cache.TrafficUpdateTime = time.Now()
	}

	cache.UpdateTime = time.Now()

	if result.Error != nil {
		return db.Create(&cache).Error
	}
	return db.Save(&cache).Error
}

func (s *SchedulerService) IsCacheEnabled() bool {
	db := database.GetDB()
	var setting models.SysSetting
	if err := db.Where("key = ?", SettingCacheEnabled).First(&setting).Error; err != nil {
		return false
	}
	return setting.Value == "true"
}

func (s *SchedulerService) GetCacheInterval() int {
	db := database.GetDB()
	var setting models.SysSetting
	if err := db.Where("key = ?", SettingCacheInterval).First(&setting).Error; err != nil {
		return 30
	}
	var interval int
	if _, err := json.Marshal(setting.Value); err == nil {
		json.Unmarshal([]byte(setting.Value), &interval)
	}
	if interval <= 0 {
		interval = 30
	}
	return interval
}

func (s *SchedulerService) SetCacheEnabled(enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	return database.UpsertSysSetting(SettingCacheEnabled, value)
}

func (s *SchedulerService) SetCacheInterval(minutes int) error {
	value := "30"
	if minutes > 0 {
		data, _ := json.Marshal(minutes)
		value = string(data)
	}
	return database.UpsertSysSetting(SettingCacheInterval, value)
}

func (s *SchedulerService) GetConfigCache(configID string) (*models.OciConfigCache, error) {
	db := database.GetDB()
	var cache models.OciConfigCache
	if err := db.Where("config_id = ?", configID).First(&cache).Error; err != nil {
		return nil, err
	}
	return &cache, nil
}

// SaveTrafficCache 只更新流量相关的两列，供实时查询命中后回写缓存。
//
// 为什么单独一个方法、且必须用 Updates（列级）而不是 Save（整行）：
//   - 整行 Save 需要先把整行读出来再写回，期间若调度器更新了实例列表、
//     卷列表等字段，回写会把它们覆盖成旧值（lost update）；
//   - 列级 Updates 只生成 UPDATE ... SET traffic_data=?, traffic_update_time=?，
//     不触碰其他列，因此与调度器的并发写入天然不冲突。
//
// 缓存行不存在时（例如用户刚添加配置、调度器还没跑第一轮）直接跳过：
// 这里不值得为了写缓存而插入一行只有两个字段的记录，等调度器建行即可。
func (s *SchedulerService) SaveTrafficCache(configID string, stats *MonthlyTrafficStats) {
	if stats == nil {
		return
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return
	}
	db := database.GetDB()
	db.Model(&models.OciConfigCache{}).
		Where("config_id = ?", configID).
		Updates(map[string]interface{}{
			"traffic_data":        string(data),
			"traffic_update_time": time.Now(),
		})
}

func (s *SchedulerService) RefreshAllCaches() {
	db := database.GetDB()
	var configs []models.OciUser
	db.Find(&configs)

	if len(configs) > 0 {
		go s.updateAllCaches(configs)
	}
}
