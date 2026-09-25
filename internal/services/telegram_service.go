package services

import (
        "bytes"
        "context"
        "encoding/json"
        "fmt"
        "io"
        "log"
        "net/http"
        "net/url"
        "strconv"
        "strings"
        "sync"
        "time"

        "github.com/adiecho/oci-panel/internal/database"
        "github.com/adiecho/oci-panel/internal/models"
        "github.com/adiecho/oci-panel/internal/version"
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
                // 【为什么用「调用前是否已在运行」来决定要不要补注册】
                // StartBot 首次启动时会做 setMyCommands + setMenuButton；若 bot 已在运行，
                // 它直接 return，不做任何注册。所以只有「已在运行」这一种情况下，
                // 改配置才需要额外补一次注册（可能是换了 bot token 或反代地址，
                // 命令菜单与菜单按钮的归属随之改变）。
                //
                // 此前是无条件再起一个 goroutine 补注册，导致「从停止状态保存配置」时
                // 一次操作注册两遍（StartBot 一次 + 补注册一次）——表现为日志里
                // 「Telegram 命令菜单已注册」成对出现，且徒增 Telegram 限频风险。
                wasRunning := s.IsRunning()
                s.StartBot()
                if wasRunning {
                        go func() {
                                s.setMyCommands()
                                s.setMenuButton()
                        }()
                }
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

        s.setMyCommands()
        s.setMenuButton()
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
                                "❌ 无权限操作此机器人🤖\n项目地址: https://github.com/yy1319hua/oci-panel", nil)
                        return
                }

                s.handleCommand(update.Message.Chat.ID, update.Message.Text)
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

// handleCommand 处理文本命令。无命令（普通文本）时回落到主菜单，避免用户不知所措。
func (s *TelegramService) handleCommand(chatID int64, text string) {
        chat := fmt.Sprintf("%d", chatID)
        cmd := strings.TrimSpace(strings.ToLower(text))
        // 去掉 @botname 后缀（群聊中形如 /traffic@mybot）
        if i := strings.Index(cmd, "@"); i > 0 {
                cmd = cmd[:i]
        }

        var reply, help string
        switch cmd {
        case "/start", "/menu":
                s.handleStartCommand(chatID)
                return
        case "/traffic":
                reply = s.getTrafficStats()
        case "/cost":
                reply = s.getCostStats()
        case "/instances":
                reply = s.getInstanceStats()
        case "/alive":
                reply = s.checkAlive()
        case "/configs":
                reply = s.getConfigList()
        case "/keepalive":
                reply = s.getKeepaliveStats()
        case "/grab":
                reply = s.getGrabStats()
        case "/backup":
                reply = s.getBackupStats()
        case "/version":
                reply = s.getVersionInfo()
        default:
                help = "可用命令：\n/start 开始 / 打开面板\n/traffic 流量统计\n/cost 每日成本\n/instances 实例统计\n/alive 一键测活\n/configs 配置列表\n/version 版本信息\n/menu 打开按钮菜单"
        }

        if reply == "" {
                reply = help
        }
        s.doSendMessage(chat, reply, nil)
}

// telegramCommands 返回面板对外暴露的命令列表。
func telegramCommands() []map[string]string {
        return []map[string]string{
                {"command": "start", "description": "开始 / 打开面板"},
                {"command": "menu", "description": "打开按钮菜单"},
                {"command": "traffic", "description": "流量统计（账号月度总量）"},
                {"command": "cost", "description": "每日成本（发现扣费）"},
                {"command": "instances", "description": "实例统计"},
                {"command": "alive", "description": "一键测活"},
                {"command": "configs", "description": "配置列表"},
                {"command": "keepalive", "description": "保活任务状态"},
                {"command": "grab", "description": "抢机任务状态"},
                {"command": "backup", "description": "备份任务状态"},
                {"command": "version", "description": "版本信息"},
        }
}

// commandScopes 返回需要注册命令的作用域列表。
//
// 关键点（踩过的坑）：Telegram 命令列表按**作用域**分层，在同一个会话里取命令时
// 只采用「优先级最高的那一层」，优先级从高到低为：
//
//      chat（指定会话） > all_private_chats > default
//
// 如果别的程序（例如早先共用同一个 bot 的 AI agent）曾在 all_private_chats 或
// chat 等更高作用域注册过命令，那么即便我们在 default 作用域注册了面板命令，
// 用户点开菜单看到的仍是那一层残留的旧命令 —— 表现为「菜单里根本没有
// traffic / cost」。只改命令列表、不覆盖作用域是修不好的。
//
// 因此这里把同一份命令注册到所有与私聊相关的作用域，直接覆盖旧命令；
// 其中 chat 作用域优先级最高，注册后用户必定看到面板命令。
func (s *TelegramService) commandScopes(chatID string) []map[string]any {
        // nil 表示不带 scope 字段，即 default 作用域。
        scopes := []map[string]any{
                nil,
                {"type": "all_private_chats"},
        }
        // chat 作用域仅接受私聊用户 id（正整数）。chatID 可能是群组/空值，故先校验。
        if id, err := strconv.ParseInt(strings.TrimSpace(chatID), 10, 64); err == nil && id > 0 {
                scopes = append(scopes, map[string]any{"type": "chat", "chat_id": id})
        }
        return scopes
}

// setMyCommands 把命令菜单注册到所有相关作用域（见 commandScopes 说明）。
//
// Telegram 服务端会**缓存**命令菜单，只有再次调用 setMyCommands 才会刷新，
// 故必须在「bot 启动」和「配置变更（尤其是换 bot token）」两个时机都调用。
func (s *TelegramService) setMyCommands() {
        s.mu.RLock()
        botToken := s.botToken
        chatID := s.chatID
        s.mu.RUnlock()
        if botToken == "" {
                return
        }

        commands := telegramCommands()
        for _, scope := range s.commandScopes(chatID) {
                s.postSetMyCommands(botToken, commands, scope)
        }
}

// postSetMyCommands 在单个作用域注册命令，失败重试。
//
// 这是启动路径上的非关键调用，失败不应阻断 bot 运行，但也绝不能一次失败就永久
// 放弃 —— 那样菜单会静默停留在旧状态，很难排查。
func (s *TelegramService) postSetMyCommands(botToken string, commands []map[string]string, scope map[string]any) bool {
        payload := map[string]any{"commands": commands}
        label := "default"
        if scope != nil {
                payload["scope"] = scope
                if t, _ := scope["type"].(string); t != "" {
                        label = t
                }
        }
        body, err := json.Marshal(payload)
        if err != nil {
                log.Printf("setMyCommands[%s]: 序列化命令失败: %v", label, err)
                return false
        }

        // 重试：反代/网络抖动都可能导致单次失败。命令菜单是幂等操作，重试无副作用。
        const maxAttempts = 3
        var lastErr error
        for attempt := 1; attempt <= maxAttempts; attempt++ {
                if attempt > 1 {
                        time.Sleep(time.Duration(attempt-1) * 2 * time.Second)
                }

                apiURL := fmt.Sprintf("%s/bot%s/setMyCommands", s.baseURL(), botToken)
                resp, err := http.Post(apiURL, "application/json", bytes.NewReader(body))
                if err != nil {
                        lastErr = err
                        log.Printf("setMyCommands[%s] 第 %d/%d 次失败: %v", label, attempt, maxAttempts, err)
                        continue
                }

                // 必须读 body：Telegram 会用 HTTP 200 返回 {"ok":false,"description":...}，
                // 只看状态码会把「token 无效」当成成功。
                respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
                resp.Body.Close()

                if resp.StatusCode != http.StatusOK {
                        lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
                        log.Printf("setMyCommands[%s] 第 %d/%d 次失败: %v", label, attempt, maxAttempts, lastErr)
                        continue
                }

                var parsed struct {
                        Ok          bool   `json:"ok"`
                        Description string `json:"description"`
                }
                if err := json.Unmarshal(respBody, &parsed); err != nil {
                        lastErr = fmt.Errorf("解析响应失败: %w", err)
                        log.Printf("setMyCommands[%s] 第 %d/%d 次失败: %v", label, attempt, maxAttempts, lastErr)
                        continue
                }
                if !parsed.Ok {
                        lastErr = fmt.Errorf("telegram 返回失败: %s", parsed.Description)
                        log.Printf("setMyCommands[%s] 第 %d/%d 次失败: %v", label, attempt, maxAttempts, lastErr)
                        continue
                }

                log.Printf("Telegram 命令菜单已注册（作用域 %s，%d 个命令）", label, len(commands))
                return true
        }

        log.Printf("setMyCommands[%s] 连续 %d 次失败，命令菜单可能仍为旧版本: %v", label, maxAttempts, lastErr)
        return false
}

// setMenuButton 配置 Telegram 聊天框右下角的「菜单按钮」（Menu Button）。
//
// 这是与 / 命令列表（setMyCommands）**完全独立**的机制：命令列表只在输入框输入 / 时
// 浮现，而菜单按钮是常驻的一个入口，点击即弹出命令列表（traffic/cost/...），
// 让用户无需记忆命令也能用，比纯靠输入 / 更直观。
//
// 行为：固定设为 commands 类型（点击弹出命令列表）。不设为打开面板 Web App。
//
// Telegram 会缓存菜单按钮，故在「bot 启动」与「配置变更」两个时机都要调用，
// 否则换了 bot，用户看到的仍是旧按钮。菜单按钮同样分作用域：不带 chat_id 是全局默认，
// 带 chat_id 是会话专属；若旧程序在会话专属作用域设过菜单按钮，全局默认会被它遮蔽，
// 故全局默认与私聊专属各设一次。
func (s *TelegramService) setMenuButton() {
        s.mu.RLock()
        botToken := s.botToken
        chatID := s.chatID
        s.mu.RUnlock()
        if botToken == "" {
                return
        }

        // 菜单按钮同样分作用域：不带 chat_id 是全局默认，带 chat_id 是某个会话专属。
        // 若旧程序在「会话专属」作用域设过菜单按钮，全局默认会被它遮蔽，
        // 故这里全局默认与私聊专属各设一次（0 表示全局默认）。
        targets := []int64{0}
        if id, err := strconv.ParseInt(strings.TrimSpace(chatID), 10, 64); err == nil && id > 0 {
                targets = append(targets, id)
        }
        for _, chat := range targets {
                s.applyMenuButton(botToken, chat)
        }
}

// applyMenuButton 在指定作用域（chatID=0 表示全局默认）应用菜单按钮。
//
// 菜单按钮固定设为 commands 类型：点击按钮即弹出命令列表（traffic/cost/...），
// 让用户无需记忆命令也能用。不设为打开面板 Web App（用户不需要）。
// 该动作幂等，失败仅记日志，不阻断 bot 启动。
func (s *TelegramService) applyMenuButton(botToken string, chatID int64) {
        payload := map[string]any{"menu_button": map[string]any{"type": "commands"}}
        if chatID != 0 {
                payload["chat_id"] = chatID
        }
        body, _ := json.Marshal(payload)
        if s.postMenuButton(botToken, body) {
                log.Printf("Telegram 菜单按钮已设为 commands 类型（chat=%d），点击弹出命令列表", chatID)
                return
        }
        log.Printf("Telegram 菜单按钮设置失败（chat=%d）", chatID)
}

// postMenuButton 向 setChatMenuButton 发送一次性请求，返回是否成功（ok=true）。
func (s *TelegramService) postMenuButton(botToken string, body []byte) bool {
        apiURL := fmt.Sprintf("%s/bot%s/setChatMenuButton", s.baseURL(), botToken)
        resp, err := http.Post(apiURL, "application/json", bytes.NewReader(body))
        if err != nil {
                log.Printf("setChatMenuButton 请求失败: %v", err)
                return false
        }
        respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
        resp.Body.Close()
        if resp.StatusCode != http.StatusOK {
                log.Printf("setChatMenuButton HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
                return false
        }
        var parsed struct {
                Ok          bool   `json:"ok"`
                Description string `json:"description"`
        }
        if err := json.Unmarshal(respBody, &parsed); err != nil {
                return false
        }
        if !parsed.Ok {
                log.Printf("setChatMenuButton 返回失败: %s", parsed.Description)
                return false
        }
        return true
}

func (s *TelegramService) getMainKeyboard() *InlineKeyboardMarkup {
        return &InlineKeyboardMarkup{
                InlineKeyboard: [][]InlineKeyboardButton{
                        {
                                {Text: "🔍 一键测活", CallbackData: "check_alive"},
                                {Text: "💰 每日成本", CallbackData: "cost_stats"},
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
                                {Text: "⭐ 开源地址（欢迎Star）", URL: "https://github.com/yy1319hua/oci-panel"},
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

        case "cost_stats":
                text := s.getCostStats()
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

// getCostStats 汇总各配置的近 3 日成本，重点提示「是否已产生扣费」。
func (s *TelegramService) getCostStats() string {
        db := database.GetDB()

        var users []models.OciUser
        if err := db.Find(&users).Error; err != nil {
                return "❌ 获取配置失败"
        }

        if len(users) == 0 {
                return "【每日成本】\n\n暂无配置"
        }

        lines := parallelMapUsers(users, telegramConcurrency, func(user models.OciUser) string {
                ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
                defer cancel()
                stats, err := s.ociService.GetDailyCost(ctx, &user, 3)
                if err != nil {
                        return fmt.Sprintf("🔑 %s: ❌ 获取失败（需 usage-report 读取权限）", user.Username)
                }
                billable := stats.MonthToDate > 0
                flag := "✅ 免费额度内"
                if billable {
                        flag = "⚠️ 已产生费用"
                }
                var dayLines []string
                for _, d := range stats.Days {
                        dayLines = append(dayLines, fmt.Sprintf("   %s：%.4f %s", d.Date, d.Amount, d.Currency))
                }
                return fmt.Sprintf("🔑 %s【%s】\n   近3日：\n%s\n   累计：%.4f %s %s",
                        user.Username, flag, strings.Join(dayLines, "\n"), stats.MonthToDate, stats.Currency, "")
        })

        var out []string
        for _, l := range lines {
                if l != "" {
                        out = append(out, l)
                }
        }

        return fmt.Sprintf("【每日成本】\n\n🕐 时间：%s\n（免费额度内为 0，出现金额即为扣费）\n\n%s",
                time.Now().Format("2006-01-02 15:04:05"),
                strings.Join(out, "\n\n"))
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
        return fmt.Sprintf("【版本信息】\n\n📦 应用名称：OCI Panel\n🏷️ 当前版本：%s\n🔧 后端框架：Gin (Go)\n🎨 前端框架：Vue 3 + Vite\n💾 数据库：SQLite\n\n🕐 查询时间：%s",
                version.AppVersion, time.Now().Format("2006-01-02 15:04:05"))
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
                return fmt.Sprintf("🔑 配置名：【%s】\n🌏 主区域：【%s】\n🖥️ 实例数量：【%d】台\n⬇️ 本月入站流量：%s\n⬆️ 本月出站流量：%s\n💰 本月计费流量：%s",
                        user.Username, user.OciRegion, trafficStats.InstanceCount,
                        FormatBytes(trafficStats.InboundTraffic),
                        FormatBytes(trafficStats.OutboundTraffic),
                        FormatBytes(trafficStats.BillableTraffic))
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

// getKeepaliveStats /keepalive 命令：保活任务状态。
func (s *TelegramService) getKeepaliveStats() string {
        db := database.GetDB()
        var tasks []models.KeepaliveTask
        db.Find(&tasks)
        if len(tasks) == 0 {
                return "当前没有保活任务。可在面板「自动化 → 保活」中创建。"
        }
        var b strings.Builder
        b.WriteString("🫀 <b>保活任务</b>\n")
        for _, t := range tasks {
                state := "⛔ 停用"
                if t.Enabled {
                        state = "✅ 运行中"
                }
                last := "从未执行"
                if t.LastRunAt != nil {
                        last = t.LastRunAt.Format("01-02 15:04")
                }
                fmt.Fprintf(&b, "\n• %s (%s)\n  %s · 每%d分钟 · 上次: %s\n  %s",
                        t.InstanceName, state, t.LastResult, t.IntervalMin, last, "")
        }
        return b.String()
}

// getGrabStats /grab 命令：抢机任务状态。
func (s *TelegramService) getGrabStats() string {
        db := database.GetDB()
        var tasks []models.GrabTask
        db.Find(&tasks)
        if len(tasks) == 0 {
                return "当前没有抢机任务。可在面板「自动化 → 抢机」中创建。"
        }
        var b strings.Builder
        b.WriteString("🎯 <b>抢机任务</b>\n")
        for _, t := range tasks {
                state := "⛔ 停用"
                if t.Enabled {
                        state = "✅ 重试中"
                }
                if t.Status == "success" {
                        state = "🎉 已抢到"
                }
                last := "从未尝试"
                if t.LastTryAt != nil {
                        last = t.LastTryAt.Format("01-02 15:04")
                }
                fmt.Fprintf(&b, "\n• %s (%s)\n  %s · %d OCPU / %dGB · 每%d分钟\n  上次: %s\n  %s",
                        t.Name, state, t.Shape, t.Ocpus, t.MemoryGB, t.IntervalMin, last, t.LastError)
        }
        return b.String()
}

// getBackupStats /backup 命令：备份任务状态。
func (s *TelegramService) getBackupStats() string {
        db := database.GetDB()
        var tasks []models.BackupTask
        db.Find(&tasks)
        if len(tasks) == 0 {
                return "当前没有备份任务。可在面板「自动化 → 备份」中创建。"
        }
        var b strings.Builder
        b.WriteString("💾 <b>备份任务</b>\n")
        for _, t := range tasks {
                state := "⛔ 停用"
                if t.Enabled {
                        state = "✅ 运行中"
                }
                last := "从未执行"
                if t.LastRunAt != nil {
                        last = t.LastRunAt.Format("01-02 15:04")
                }
                fmt.Fprintf(&b, "\n• %s (%s)\n  保留%d份 · 每%d小时 · 上次: %s\n  %s",
                        t.VolumeName, state, t.Retention, t.IntervalHour, last, t.LastResult)
        }
        return b.String()
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
