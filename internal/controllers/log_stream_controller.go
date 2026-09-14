package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
)

// LogStreamController 提供实时日志的两条接口：
//
//	GET /api/sys/recentLogs  首屏历史快照（JSON，一次性）
//	GET /api/sys/logs/stream SSE 实时流（text/event-stream，长连接）
//
// 【为什么用 SSE 而不是 WebSocket】
// 日志流此前走 WebSocket，问题有二：
//  1. gorilla/websocket 要求同一连接同一时刻只能有一个写者，广播与建连回放并发写
//     会 panic 崩掉整个进程（表现即 /api/sys/wsTicket 被重置、前端 502）；
//  2. WebSocket 需 HTTP Upgrade 后维持双向长连接，经 Cloudflare 隧道（尤其跨境链路）
//     时非常脆弱，易被中途掐断成 502。
//
// SSE 是**单向普通 HTTP 长连接**：每个请求在自己的 goroutine 里往 ResponseWriter 写，
// 天然不存在多写者问题；且走标准 HTTP，Cloudflare 支持远比 WebSocket 稳。
// 由于 SSE 是普通 HTTP 请求，可以直接携带 Authorization 头，因此也**不再需要**
// WebSocket 时代那套「先取一次性 ticket 再连接」的迂回方案。
type LogStreamController struct {
	logService     *services.LogStreamService
	allowedOrigins []string
}

func NewLogStreamController(logService *services.LogStreamService, allowedOrigins []string) *LogStreamController {
	return &LogStreamController{
		logService:     logService,
		allowedOrigins: allowedOrigins,
	}
}

// 历史日志接口的参数边界。行数上限与环形缓冲容量（historyMax=2000）保持一致，
// 避免出现「请求 5000 行却只能返回 2000 行」这种前端无法解释的静默截断。
const (
	recentLogsDefaultLines = 300
	recentLogsMaxLines     = 2000
)

// recentLogsLevels 供按级别过滤时做白名单校验，避免把任意字符串拼进匹配逻辑。
var recentLogsLevels = map[string]struct{}{
	"INFO": {}, "WARN": {}, "ERROR": {}, "DEBUG": {}, "SUCCESS": {},
}

// GetRecentLogs 返回服务端环形缓冲中的历史日志（JSON），供日志页首屏立即渲染。
//
// 为什么需要它：SSE 建连后虽然会回放历史，但从「发起请求」到「首帧到达」仍有往返延迟。
// 先用这个接口同步拉一次历史，页面一打开就有内容，随后 SSE 再无缝接上实时增量。
//
// 查询参数：
//   - lines：返回最后 N 行，默认 300，上限 2000。
//   - level：可选，按级别精确过滤（INFO/WARN/ERROR/DEBUG/SUCCESS）。
func (lc *LogStreamController) GetRecentLogs(c *gin.Context) {
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

	history := lc.logService.GetHistory()

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

// sseKeepAlive 是长连接静默期的注释心跳间隔。
//
// 静默不代表没有日志在产生，只是恰好这段时间没有新日志。不发心跳的话，
// Cloudflare 隧道 / 中间反代可能因「空闲超时」主动断开这条长连接。
// SSE 规范里以 ":" 开头的行是注释，客户端解析时会直接忽略，是无副作用的 no-op。
const sseKeepAlive = 30 * time.Second

// StreamLogs 以 SSE 推送实时日志。
//
// 协议要点：
//   - 响应头 Content-Type: text/event-stream，Cache-Control: no-cache，
//     X-Accel-Buffering: no（禁用 nginx 类反代的缓冲，否则日志会被攒着不发）；
//   - 建连后**先写一个 ": open" 注释帧并 Flush**，让响应头立刻到达客户端 ——
//     否则在回放历史 / 等待首条日志期间，前端 fetch 一直是 pending，
//     页面看起来像「点了没反应」；
//   - 每条日志写成 `data: <内容>\n\n`；
//   - 静默 30s 发一次 ": keepalive" 心跳维持连接；
//   - 客户端断开（ctx.Done）即退出，注销订阅者。
func (lc *LogStreamController) StreamLogs(c *gin.Context) {
	sub, ok := lc.logService.Subscribe()
	if !ok {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse(503, "日志订阅数已达上限，请稍后重试"))
		return
	}
	defer lc.logService.Unsubscribe(sub)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 先推响应头，让前端能立刻进入「已连接」状态而不是一直转圈。
	fmt.Fprint(c.Writer, ": open\n\n")
	c.Writer.Flush()

	// 回放连接前的历史日志，进入页面或重连后能看到「连接前」的内容。
	// 注意：回放与订阅之间的新日志不会丢 —— 订阅已在上面先完成，这期间的日志
	// 会进入 sub 的缓冲 channel，回放结束后由下面的循环继续消费。
	for _, line := range lc.logService.GetHistory() {
		writeSSEData(c.Writer, line)
	}
	c.Writer.Flush()

	ctx := c.Request.Context()
	ticker := time.NewTicker(sseKeepAlive)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// 客户端断开（关页面/切标签/网络断）。
			return
		case line, ok := <-sub.Messages():
			if !ok {
				// 服务端注销了订阅者（如进程收尾），结束这条流。
				fmt.Fprint(c.Writer, "event: done\ndata: closed\n\n")
				c.Writer.Flush()
				return
			}
			writeSSEData(c.Writer, line)
			c.Writer.Flush()
		case <-ticker.C:
			fmt.Fprint(c.Writer, ": keepalive\n\n")
			c.Writer.Flush()
		}
	}
}

// writeSSEData 把一行日志写成 SSE data 帧。
//
// 【为什么不能直接写裸内容】SSE 的线格式里，换行是帧的分隔符：若内容本身含 \n，
// 客户端会把后半段当成「没有字段名的行」而静默丢弃。因此必须先把 \n / \r 拆开，
// 每一行都单独加 "data: " 前缀，最后以一个空行结束整帧。
func writeSSEData(w io.Writer, data string) {
	// 统一换行符，避免 \r\n 与裸 \r 造成重复空行或丢内容。
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")

	lines := strings.Split(data, "\n")
	for _, line := range lines {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}
