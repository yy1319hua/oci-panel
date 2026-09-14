package controllers

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type WebSocketController struct {
	wsService      *services.WebSocketService
	allowedOrigins []string
	upgrader       websocket.Upgrader
}

func (wc *WebSocketController) IssueTicket(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Unauthorized"))
		return
	}
	usernameString, ok := username.(string)
	if !ok || usernameString == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Unauthorized"))
		return
	}
	ticket, err := wc.wsService.IssueTicket(usernameString)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse(503, "Unable to create websocket ticket"))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{"ticket": ticket}, "success"))
}

func NewWebSocketController(wsService *services.WebSocketService, allowedOrigins []string) *WebSocketController {
	wc := &WebSocketController{
		wsService:      wsService,
		allowedOrigins: allowedOrigins,
	}
	wc.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     wc.checkOrigin,
	}
	return wc
}

// 历史日志接口的参数边界。行数上限与环形缓冲容量（historyMax=2000）保持一致，
// 避免出现「请求 5000 行却只能返回 2000 行」这种前端无法解释的静默截断。
const (
	recentLogsDefaultLines = 300
	recentLogsMaxLines     = 2000
)

// validLogLevels 供按级别过滤时做白名单校验，避免把任意字符串拼进匹配逻辑。
var recentLogsLevels = map[string]struct{}{
	"INFO": {}, "WARN": {}, "ERROR": {}, "DEBUG": {}, "SUCCESS": {},
}

// GetRecentLogs 返回服务端环形缓冲中的历史日志（JSON），供日志页首屏立即渲染。
//
// 为什么需要它：WebSocket 建连要经历「取 ticket → 升级 → 回放历史」三步，
// 期间前端只能显示「正在连接日志流...」的空白。改为此接口先同步拉一次历史，
// 页面一打开就有内容，随后 WebSocket 再无缝接上实时增量。
//
// 查询参数：
//   - lines：返回最后 N 行，默认 300，上限 2000。
//   - level：可选，按级别精确过滤（INFO/WARN/ERROR/DEBUG/SUCCESS）。
func (wc *WebSocketController) GetRecentLogs(c *gin.Context) {
	lines := recentLogsDefaultLines
	if raw := strings.TrimSpace(c.Query("lines")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			lines = parsed
		}
	}
	if lines > recentLogsMaxLines {
		lines = recentLogsMaxLines
	}

	level := strings.ToUpper(strings.TrimSpace(c.Query("level")))
	levelFilter := ""
	if level != "" && level != "ALL" {
		if _, ok := recentLogsLevels[level]; !ok {
			c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "invalid level"))
			return
		}
		levelFilter = level
	}

	history := wc.wsService.GetHistory()

	// 从尾部往前取，命中足够行数即停：避免为了最后 300 行而遍历并复制全部历史。
	// 反向收集后再翻转，可保证返回顺序仍是「旧 → 新」，与实时推送的追加方向一致。
	collected := make([]string, 0, lines)
	for i := len(history) - 1; i >= 0 && len(collected) < lines; i-- {
		line := history[i]
		if levelFilter != "" && !logLineHasLevel(line, levelFilter) {
			continue
		}
		collected = append(collected, line)
	}

	total := len(collected)
	result := make([]string, total)
	for i := 0; i < total; i++ {
		result[i] = collected[total-1-i]
	}

	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"lines": result,
		"count": total,
	}, "success"))
}

// logLineHasLevel 判断一行日志的级别。
//
// 历史缓冲里可能混有两种格式：
//   - SendLog 产出的文本行 "[2006-01-02 15:04:05] LEVEL: 内容"；
//   - SendStructuredLog 产出的 JSON `{"time":..,"level":"LEVEL","message":..}`。
//
// 因此不能只按 `"] "` 前缀匹配 —— 那样结构化日志会被级别过滤整片丢掉。
func logLineHasLevel(line, level string) bool {
	// 结构化 JSON 格式：直接找 "level":"XXX" 字段。
	if strings.HasPrefix(strings.TrimSpace(line), "{") {
		return jsonHasLevel(line, level)
	}

	// 文本格式：剥掉开头的 "[时间戳] " 前缀后再比对级别。
	if strings.HasPrefix(line, "[") {
		if idx := strings.Index(line, "] "); idx >= 0 {
			line = line[idx+2:]
		}
	}
	if len(line) < len(level) || !strings.EqualFold(line[:len(level)], level) {
		return false
	}
	// 级别后必须紧跟分隔符（":" / 引号 / 空格 或已到行尾），
	// 避免 "ERRORX" 这类前缀被误判成 ERROR。
	if len(line) == len(level) {
		return true
	}
	switch line[len(level)] {
	case ':', '"', ' ', '\t':
		return true
	default:
		return false
	}
}

// jsonHasLevel 判断结构化日志 JSON 里的 level 字段是否等于目标级别。
// 用 Unmarshal 而不是字符串匹配，避免 message 正文里出现 "level":"ERROR"
// 这类内容造成误命中。
func jsonHasLevel(line, level string) bool {
	var parsed struct {
		Level string `json:"level"`
	}
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		return false
	}
	return strings.EqualFold(parsed.Level, level)
}

// checkOrigin 仅允许同源（Origin 的 host 与请求 Host 一致）或显式白名单内的来源建立连接，
// 替换此前无条件返回 true 的实现——否则任意网站都能跨站发起 WebSocket 连接（CSWSH 风险）。
func (wc *WebSocketController) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// 非浏览器客户端（如本机脚本）通常没有 Origin 头；放行，鉴权仍由一次性 ticket 保证。
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	for _, a := range wc.allowedOrigins {
		if a == "*" || strings.EqualFold(strings.TrimSpace(a), origin) {
			return true
		}
	}
	return false
}

func (wc *WebSocketController) HandleWebSocket(c *gin.Context) {
	// 升级前先消费一次性短时 ticket。浏览器无法为 WebSocket 设置自定义请求头，
	// 由已认证的 API 请求先取得 ticket，避免把长期 JWT 放进 URL 和反代日志。
	ticket := c.Query("ticket")
	if ticket == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Unauthorized"))
		return
	}
	if !wc.wsService.ConsumeTicket(ticket) {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Invalid websocket ticket"))
		return
	}

	conn, err := wc.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	if !wc.wsService.RegisterClient(conn) {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "too many connections"), time.Now().Add(time.Second))
		_ = conn.Close()
		return
	}
	defer wc.wsService.UnregisterClient(conn)

	const (
		maxLogMessageSize = 4 << 10
		pongWait          = 90 * time.Second
		pingPeriod        = 30 * time.Second
	)
	conn.SetReadLimit(maxLogMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	wc.wsService.SendInfo("Connected to log stream")

	// 回放历史日志：把服务端已缓冲的最近日志先发给新连接，
	// 这样进入页面或重连后能看到「连接前」的历史，而不只是连接后的实时日志。
	//
	// 必须走 wsService.WriteHistory（内部按连接加写锁），绝不能直接对 conn
	// 调 WriteMessage —— 那会与广播 goroutine 并发写同一条连接，触发
	// gorilla 的 "concurrent write to websocket connection" panic 崩掉整个进程。
	wc.wsService.WriteHistory(conn, wc.wsService.GetHistory())

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}
