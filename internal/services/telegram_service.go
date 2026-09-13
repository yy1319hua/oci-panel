package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
)

const (
	SettingKeyTgBotToken = "tg_bot_token"
	SettingKeyTgChatID   = "tg_chat_id"
	SettingKeyTgEnabled  = "tg_enabled"
	SettingKeyTgApiBase  = "tg_api_base"
)

// DefaultTelegramAPIBase 是 Telegram Bot API 官方地址；国内网络不通时可在设置里
// 配置反代地址（tg_api_base），服务会改用该地址访问。
const DefaultTelegramAPIBase = "https://api.telegram.org"

type TelegramService struct {
	botToken   string
	chatID     string
	enabled    bool
	apiBase    string
	ociService *OCIService
	mu         sync.RWMutex
	stopChan   chan struct{}
	running    bool
}

// baseURL 返回实际使用的 Telegram API 基址：配置了反代则用反代，否则用官方地址。
func (s *TelegramService) baseURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.TrimSpace(s.apiBase) != "" {
		return strings.TrimRight(s.apiBase, "/")
	}
	return DefaultTelegramAPIBase
}

type TelegramUpdate struct {
	UpdateID int `json:"update_id"`
	Message  *struct {
		MessageID int `json:"message_id"`
		From      struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
			Username  string `json:"username"`
		} `json:"from"`
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		Date int    `json:"date"`
		Text string `json:"text"`
	} `json:"message"`
	CallbackQuery *struct {
		ID   string `json:"id"`
		From struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Message struct {
			MessageID int `json:"message_id"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
		Data string `json:"data"`
	} `json:"callback_query"`
}

type TelegramResponse struct {
	Ok     bool             `json:"ok"`
	Result []TelegramUpdate `json:"result"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

func NewTelegramService(ociService *OCIService) *TelegramService {
	ts := &TelegramService{
		ociService: ociService,
		stopChan:   make(chan struct{}),
	}
	ts.loadConfig()
	return ts
}

func (s *TelegramService) loadConfig() {
	db := database.GetDB()

	var tokenSetting, chatIDSetting, enabledSetting, apiBaseSetting models.SysSetting
	db.Where("key = ?", SettingKeyTgBotToken).First(&tokenSetting)
	db.Where("key = ?", SettingKeyTgChatID).First(&chatIDSetting)
	db.Where("key = ?", SettingKeyTgEnabled).First(&enabledSetting)
	db.Where("key = ?", SettingKeyTgApiBase).First(&apiBaseSetting)

	s.mu.Lock()
	s.botToken = tokenSetting.Value
	s.chatID = chatIDSetting.Value
	s.enabled = enabledSetting.Value == "true"
	s.apiBase = apiBaseSetting.Value
	s.mu.Unlock()
}

func (s *TelegramService) UpdateConfig(botToken, chatID string, enabled bool, apiBase string) error {
	settings := []struct {
		key   string
		value string
	}{
		{SettingKeyTgBotToken, botToken},
		{SettingKeyTgChatID, chatID},
		{SettingKeyTgEnabled, fmt.Sprintf("%t", enabled)},
		{SettingKeyTgApiBase, strings.TrimSpace(apiBase)},
	}
	for _, setting := range settings {
		if err := database.UpsertSysSetting(setting.key, setting.value); err != nil {
			return err
		}
	}

	s.mu.Lock()
	s.botToken = botToken
	s.chatID = chatID
	s.enabled = enabled
	s.apiBase = strings.TrimSpace(apiBase)
	s.mu.Unlock()

	if enabled && botToken != "" && chatID != "" {
		s.StartBot()
	} else {
		s.StopBot()
	}

	return nil
}

func (s *TelegramService) GetConfig() (botToken, chatID string, enabled bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.botToken, s.chatID, s.enabled
}

// GetApiBase 返回当前配置的反代地址（为空表示使用官方地址）。
func (s *TelegramService) GetApiBase() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.apiBase
}

func (s *TelegramService) SendMessage(message string) error {
	s.mu.RLock()
	botToken := s.botToken
	chatID := s.chatID
	enabled := s.enabled
	s.mu.RUnlock()

	if !enabled || botToken == "" || chatID == "" {
		return fmt.Errorf("telegram not configured or disabled")
	}

	return s.doSendMessage(chatID, message, nil)
}

func (s *TelegramService) doSendMessage(chatID, text string, replyMarkup *InlineKeyboardMarkup) error {
	s.mu.RLock()
	botToken := s.botToken
	s.mu.RUnlock()

	apiURL := fmt.Sprintf("%s/bot%s/%s", s.baseURL(), botToken, "sendMessage")

	params := url.Values{}
	params.Set("chat_id", chatID)
	params.Set("text", text)
	params.Set("parse_mode", "HTML")

	if replyMarkup != nil {
		markupJSON, _ := json.Marshal(replyMarkup)
		params.Set("reply_markup", string(markupJSON))
	}

	resp, err := http.PostForm(apiURL, params)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status: %d", resp.StatusCode)
	}

	return nil
}

func (s *TelegramService) editMessage(chatID string, messageID int, text string, replyMarkup *InlineKeyboardMarkup) error {
	s.mu.RLock()
	botToken := s.botToken
	s.mu.RUnlock()

	apiURL := fmt.Sprintf("%s/bot%s/%s", s.baseURL(), botToken, "editMessageText")

	params := url.Values{}
	params.Set("chat_id", chatID)
	params.Set("message_id", fmt.Sprintf("%d", messageID))
	params.Set("text", text)
	params.Set("parse_mode", "HTML")

	if replyMarkup != nil {
		markupJSON, _ := json.Marshal(replyMarkup)
		params.Set("reply_markup", string(markupJSON))
	}

	resp, err := http.PostForm(apiURL, params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (s *TelegramService) deleteMessage(chatID string, messageID int) error {
	s.mu.RLock()
	botToken := s.botToken
	s.mu.RUnlock()

	apiURL := fmt.Sprintf("%s/bot%s/%s", s.baseURL(), botToken, "deleteMessage")

	params := url.Values{}
	params.Set("chat_id", chatID)
	params.Set("message_id", fmt.Sprintf("%d", messageID))

	resp, err := http.PostForm(apiURL, params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (s *TelegramService) answerCallbackQuery(callbackQueryID string) error {
	s.mu.RLock()
	botToken := s.botToken
	s.mu.RUnlock()

	apiURL := fmt.Sprintf("%s/bot%s/%s", s.baseURL(), botToken, "answerCallbackQuery")

	params := url.Values{}
	params.Set("callback_query_id", callbackQueryID)

	resp, err := http.PostForm(apiURL, params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (s *TelegramService) StartBot() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	go s.pollUpdates()
	log.Println("Telegram bot started")
}

func (s *TelegramService) StopBot() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopChan)
	s.mu.Unlock()
	log.Println("Telegram bot stopped")
}

func (s *TelegramService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

func (s *TelegramService) pollUpdates() {
	var offset int

	for {
		select {
		case <-s.stopChan:
			return
		default:
			updates, err := s.getUpdates(offset)
			if err != nil {
				log.Printf("Error getting updates: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			for _, update := range updates {
				s.handleUpdate(update)
				offset = update.UpdateID + 1
			}

			time.Sleep(1 * time.Second)
		}
	}
}

func (s *TelegramService) getUpdates(offset int) ([]TelegramUpdate, error) {
	s.mu.RLock()
	botToken := s.botToken
	s.mu.RUnlock()

	apiURL := fmt.Sprintf("%s/bot%s/%s", s.baseURL(), botToken, "getUpdates")

	params := url.Values{}
	params.Set("offset", fmt.Sprintf("%d", offset))
	params.Set("timeout", "30")

	resp, err := http.PostForm(apiURL, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result TelegramResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Ok {
		return nil, fmt.Errorf("telegram API error")
	}

	return result.Result, nil
}

func (s *TelegramService) handleUpdate(update TelegramUpdate) {
	s.mu.RLock()
	chatID := s.chatID
	s.mu.RUnlock()

	if update.Message != nil {
		if fmt.Sprintf("%d", update.Message.Chat.ID) != chatID {
			s.doSendMessage(fmt.Sprintf("%d", update.Message.Chat.ID),
				"❌ 无权限操作此机器人🤖\n项目地址: https://github.com/adiecho/oci-panel", nil)
			return
		}

		if update.Message.Text == "/start" {
			s.handleStartCommand(update.Message.Chat.ID)
		}
	}

	if update.CallbackQuery != nil {
		if fmt.Sprintf("%d", update.CallbackQuery.From.ID) != chatID {
			s.answerCallbackQuery(update.CallbackQuery.ID)
			return
		}

		s.answerCallbackQuery(update.CallbackQuery.ID)
		s.handleCallback(update.CallbackQuery)
	}
}

func (s *TelegramService) handleStartCommand(chatID int64) {
	keyboard := s.getMainKeyboard()
	s.doSendMessage(fmt.Sprintf("%d", chatID), "请选择需要执行的操作：", keyboard)
}

func (s *TelegramService) getMainKeyboard() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🔍 一键测活", CallbackData: "check_alive"},
				{Text: "📋 任务详情", CallbackData: "task_details"},
			},
			{
				{Text: "🖥️ 实例统计", CallbackData: "instance_stats"},
				{Text: "📂 配置列表", CallbackData: "config_list"},
			},
			{
				{Text: "ℹ️ 版本信息", CallbackData: "version_info"},
				{Text: "📊 流量统计", CallbackData: "traffic_stats"},
			},
			{
				{Text: "⭐ 开源地址（欢迎Star）", URL: "https://github.com/adiecho/oci-panel"},
			},
			{
				{Text: "❌ 关闭窗口", CallbackData: "cancel"},
			},
		},
	}
}

func (s *TelegramService) handleCallback(callback *struct {
	ID   string `json:"id"`
	From struct {
		ID int64 `json:"id"`
	} `json:"from"`
	Message struct {
		MessageID int `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
	Data string `json:"data"`
}) {
	chatID := fmt.Sprintf("%d", callback.Message.Chat.ID)
	messageID := callback.Message.MessageID

	switch callback.Data {
	case "check_alive":
		text := s.checkAlive()
		s.editMessage(chatID, messageID, text, s.getMainKeyboard())

	case "task_details":
		text := s.getTaskDetails()
		s.editMessage(chatID, messageID, text, s.getMainKeyboard())

	case "instance_stats":
		text := s.getInstanceStats()
		s.editMessage(chatID, messageID, text, s.getMainKeyboard())

	case "config_list":
		text := s.getConfigList()
		s.editMessage(chatID, messageID, text, s.getMainKeyboard())

	case "version_info":
		text := s.getVersionInfo()
		s.editMessage(chatID, messageID, text, s.getMainKeyboard())

	case "traffic_stats":
		text := s.getTrafficStats()
		s.editMessage(chatID, messageID, text, s.getMainKeyboard())

	case "cancel":
		s.deleteMessage(chatID, messageID)
	}
}

// telegramConcurrency 限制 Telegram 汇总命令对每个配置的并发查询数，避免一次性对 OCI 发起过多请求。
const telegramConcurrency = 5

// parallelMapUsers 以受限并发对每个 user 执行 fn，结果按 users 原顺序返回。
// 单个 fn panic 不影响其余配置（该项保留零值结果）。
func parallelMapUsers[T any](users []models.OciUser, concurrency int, fn func(user models.OciUser) T) []T {
	results := make([]T, len(users))
	if concurrency < 1 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := range users {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() {
				<-sem
				_ = recover() // 单配置查询 panic 不应杀进程或影响其余统计
			}()
			results[idx] = fn(users[idx])
		}(i)
	}
	wg.Wait()
	return results
}

func (s *TelegramService) checkAlive() string {
	db := database.GetDB()

	var users []models.OciUser
	if err := db.Find(&users).Error; err != nil {
		return "❌ 获取配置失败"
	}

	if len(users) == 0 {
		return "【API测活结果】\n\n暂无配置"
	}

	type aliveResult struct {
		valid bool
		name  string
	}
	results := parallelMapUsers(users, telegramConcurrency, func(user models.OciUser) aliveResult {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := s.ociService.ListInstances(ctx, &user, user.OciTenantID)
		return aliveResult{valid: err == nil, name: user.Username}
	})

	var validCount, invalidCount int
	var invalidNames []string
	for _, r := range results {
		if r.valid {
			validCount++
		} else {
			invalidCount++
			if r.name != "" {
				invalidNames = append(invalidNames, r.name)
			}
		}
	}

	result := fmt.Sprintf("【API测活结果】\n\n✅ 有效配置数：%d\n❌ 失效配置数：%d\n🔑 总配置数：%d",
		validCount, invalidCount, len(users))

	if len(invalidNames) > 0 {
		result += fmt.Sprintf("\n\n⚠️ 失效配置：\n%s", strings.Join(invalidNames, "\n"))
	}

	return result
}

func (s *TelegramService) getTaskDetails() string {
	db := database.GetDB()

	var tasks []models.OciCreateTask
	if err := db.Find(&tasks).Error; err != nil {
		return "❌ 获取任务失败"
	}

	if len(tasks) == 0 {
		return "【任务详情】\n\n🕐 时间：" + time.Now().Format("2006-01-02 15:04:05") + "\n\n🛎 正在执行的开机任务：无"
	}

	var taskInfos []string
	for _, task := range tasks {
		info := fmt.Sprintf("[%s] [%s] [%.0f核/%.0fGB/%dGB] [%d台] [%s] [执行%d次]",
			task.Username, task.Architecture,
			task.Ocpus, task.Memory, task.Disk,
			task.CreateNumbers, task.Status, task.ExecuteCount)
		taskInfos = append(taskInfos, info)
	}

	return fmt.Sprintf("【任务详情】\n\n🕐 时间：%s\n\n🛎 正在执行的开机任务：\n%s",
		time.Now().Format("2006-01-02 15:04:05"),
		strings.Join(taskInfos, "\n"))
}

func (s *TelegramService) getInstanceStats() string {
	db := database.GetDB()

	var users []models.OciUser
	if err := db.Find(&users).Error; err != nil {
		return "❌ 获取配置失败"
	}

	if len(users) == 0 {
		return "【实例统计】\n\n暂无配置"
	}

	type statResult struct {
		line    string
		total   int
		running int
	}
	results := parallelMapUsers(users, telegramConcurrency, func(user models.OciUser) statResult {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		instances, err := s.ociService.ListInstances(ctx, &user, user.OciTenantID)
		if err != nil {
			return statResult{line: fmt.Sprintf("❌ %s: 获取失败", user.Username)}
		}
		running := 0
		for _, inst := range instances {
			if inst.LifecycleState == "RUNNING" {
				running++
			}
		}
		return statResult{
			line:    fmt.Sprintf("🔑 %s [%s]: %d台 (运行中: %d)", user.Username, user.OciRegion, len(instances), running),
			total:   len(instances),
			running: running,
		}
	})

	var totalInstances, runningInstances int
	var stats []string
	for _, r := range results {
		if r.line != "" {
			stats = append(stats, r.line)
		}
		totalInstances += r.total
		runningInstances += r.running
	}

	return fmt.Sprintf("【实例统计】\n\n🕐 时间：%s\n📊 总实例数：%d\n🟢 运行中：%d\n\n%s",
		time.Now().Format("2006-01-02 15:04:05"),
		totalInstances, runningInstances,
		strings.Join(stats, "\n"))
}

func (s *TelegramService) getConfigList() string {
	db := database.GetDB()

	var users []models.OciUser
	if err := db.Find(&users).Error; err != nil {
		return "❌ 获取配置失败"
	}

	if len(users) == 0 {
		return "【配置列表】\n\n暂无配置"
	}

	var configs []string
	for i, user := range users {
		configs = append(configs, fmt.Sprintf("%d. %s\n   区域: %s\n   租户: %s",
			i+1, user.Username, user.OciRegion, user.TenantName))
	}

	return fmt.Sprintf("【配置列表】\n\n🔑 总配置数：%d\n\n%s",
		len(users), strings.Join(configs, "\n\n"))
}

func (s *TelegramService) getVersionInfo() string {
	return fmt.Sprintf("【版本信息】\n\n📦 应用名称：OCI Panel\n🏷️ 当前版本：v1.0.3\n🔧 后端框架：Gin (Go)\n🎨 前端框架：Vue 3 + Vite\n💾 数据库：SQLite\n\n🕐 查询时间：%s",
		time.Now().Format("2006-01-02 15:04:05"))
}

func (s *TelegramService) getTrafficStats() string {
	db := database.GetDB()

	var users []models.OciUser
	if err := db.Find(&users).Error; err != nil {
		return "❌ 获取配置失败"
	}

	if len(users) == 0 {
		return "【流量统计】\n\n暂无配置"
	}

	lines := parallelMapUsers(users, telegramConcurrency, func(user models.OciUser) string {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		trafficStats, err := s.ociService.GetMonthlyTrafficStats(ctx, &user)
		if err != nil {
			return fmt.Sprintf("❌ %s: 获取失败", user.Username)
		}
		return fmt.Sprintf("🔑 配置名：【%s】\n🌏 主区域：【%s】\n🖥️ 实例数量：【%d】台\n⬇️ 本月入站流量：%s\n⬆️ 本月出站流量：%s",
			user.Username, user.OciRegion, trafficStats.InstanceCount,
			FormatBytes(trafficStats.InboundTraffic),
			FormatBytes(trafficStats.OutboundTraffic))
	})

	var stats []string
	for _, line := range lines {
		if line != "" {
			stats = append(stats, line)
		}
	}

	return fmt.Sprintf("【流量统计】\n\n🕐 时间：%s\n\n%s",
		time.Now().Format("2006-01-02 15:04:05"),
		strings.Join(stats, "\n\n"))
}

func (s *TelegramService) SendNotification(title, message string) error {
	text := fmt.Sprintf("<b>%s</b>\n\n%s\n\n🕐 %s",
		title, message, time.Now().Format("2006-01-02 15:04:05"))
	return s.SendMessage(text)
}

func (s *TelegramService) TestConnection() error {
	s.mu.RLock()
	botToken := s.botToken
	s.mu.RUnlock()

	if botToken == "" {
		return fmt.Errorf("bot token not configured")
	}

	apiURL := fmt.Sprintf("%s/bot%s/%s", s.baseURL(), botToken, "getMe")
	resp, err := http.Get(apiURL)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid bot token")
	}

	return nil
}
