package services

import (
	"context"
	"encoding/json"
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

	instances, err := s.ociService.ListInstances(ctx, &user, compartmentId)
	if err == nil {
		cache.InstanceCount = len(instances)
		cache.RunningInstances = 0
		for _, inst := range instances {
			if inst.LifecycleState == core.InstanceLifecycleStateRunning {
				cache.RunningInstances++
			}
		}

		instanceInfos := s.ociService.GetInstancesDetailsConcurrent(ctx, &user, instances)
		if data, err := json.Marshal(instanceInfos); err == nil {
			cache.InstancesData = string(data)
		}
	}

	volumes, err := s.ociService.ListBootVolumes(ctx, &user, compartmentId)
	if err == nil {
		if data, err := json.Marshal(volumes); err == nil {
			cache.VolumesData = string(data)
		}
	}

	vcns, err := s.ociService.ListVCNs(ctx, &user, compartmentId)
	if err == nil {
		if data, err := json.Marshal(vcns); err == nil {
			cache.VcnsData = string(data)
		}
	}

	tenantInfo, err := s.ociService.GetTenantInfo(ctx, &user)
	if err == nil {
		if data, err := json.Marshal(tenantInfo); err == nil {
			cache.TenantData = string(data)
		}
		// 同步更新 OciUser 表中的租户名称和创建时间
		updateFields := map[string]interface{}{}
		if tenantInfo.Name != "" && tenantInfo.Name != user.TenantName {
			updateFields["tenant_name"] = tenantInfo.Name
		}
		if tenantInfo.CreateTime != "" && user.TenantCreateTime == nil {
			if parsedTime, err := time.Parse("2006-01-02 15:04:05", tenantInfo.CreateTime); err == nil {
				updateFields["tenant_create_time"] = parsedTime
			}
		}
		if len(updateFields) > 0 {
			db.Model(&user).Updates(updateFields)
		}
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

func (s *SchedulerService) RefreshAllCaches() {
	db := database.GetDB()
	var configs []models.OciUser
	db.Find(&configs)

	if len(configs) > 0 {
		go s.updateAllCaches(configs)
	}
}
