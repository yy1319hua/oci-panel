package services

import (
        "bytes"
        "encoding/json"
        "fmt"
        "log"
        "net/http"
        "strings"
        "time"

        "github.com/adiecho/oci-panel/internal/database"
        "github.com/adiecho/oci-panel/internal/models"
        "github.com/google/uuid"
)

// PushPlus 推送通道（pushplus.plus，走微信通知，比 TG 及时）。
//
// 设计为 TG 的并行通道而非替代：AutomationService.notify() 会同时向两个通道推送，
// 哪个配置了就用哪个，互不影响。Token 存 sys_setting（key: pushplus_token），
// 不进数据库日志、不出现在调用记录中。

const (
        SettingPushplusToken = "pushplus_token"
        pushplusAPI          = "https://www.pushplus.plus/send"
)

// GetPushplusToken 读取已配置的 PushPlus token（未配置返回空串）。
func GetPushplusToken() string {
        var setting models.SysSetting
        if err := database.GetDB().Where("`key` = ?", SettingPushplusToken).First(&setting).Error; err != nil {
                return ""
        }
        return strings.TrimSpace(setting.Value)
}

// SetPushplusToken 保存/清除 PushPlus token（空串即关闭该通道）。
func SetPushplusToken(token string) error {
        token = strings.TrimSpace(token)
        db := database.GetDB()
        var setting models.SysSetting
        err := db.Where("`key` = ?", SettingPushplusToken).First(&setting).Error
        if err != nil {
                // sys_setting 主键是字符串 id，新建必须显式生成，否则空 id 二次插入触发 UNIQUE 冲突
                setting = models.SysSetting{ID: uuid.New().String(), Key: SettingPushplusToken}
        }
        setting.Value = token
        return db.Save(&setting).Error
}

// SendPushplus 向 PushPlus 发送一条 markdown 通知。
// 官方接口：POST {token, title, content, template:"markdown"}。
func SendPushplus(token, title, content string) error {
        if token == "" {
                return fmt.Errorf("pushplus token 未配置")
        }
        payload, _ := json.Marshal(map[string]string{
                "token":    token,
                "title":    title,
                "content":  content,
                "template": "markdown",
        })
        client := &http.Client{Timeout: 15 * time.Second}
        resp, err := client.Post(pushplusAPI, "application/json", bytes.NewReader(payload))
        if err != nil {
                return fmt.Errorf("pushplus 请求失败: %w", err)
        }
        defer resp.Body.Close()

        var out struct {
                Code int    `json:"code"`
                Msg  string `json:"msg"`
        }
        if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
                return fmt.Errorf("pushplus 响应解析失败: %w", err)
        }
        if out.Code != 200 {
                return fmt.Errorf("pushplus 返回错误: %d %s", out.Code, out.Msg)
        }
        log.Println("PushPlus 推送成功:", title)
        return nil
}

// notify 双通道推送：TG + PushPlus，任一通道失败不影响另一个。
// 这是自动化任务（保活/抢机/备份/告警）统一的通知出口。
func (s *AutomationService) notify(title, message string) {
        if s.telegram != nil {
                if err := s.telegram.SendNotification(title, message); err != nil {
                        log.Println("TG 推送失败:", err)
                }
        }
        if token := GetPushplusToken(); token != "" {
                if err := SendPushplus(token, title, message); err != nil {
                        log.Println("PushPlus 推送失败:", err)
                }
        }
}
