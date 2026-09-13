package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// accessLogSink 是 API 访问日志的输出目标（由 router 注册为 WebSocket 实时日志）。
// 保持为可替换变量，避免 middleware → services 的循环依赖。
var accessLogSink func(level, message string)

// SetAccessLogSink 注册 API 访问日志的接收器。传 nil 表示关闭访问日志。
func SetAccessLogSink(fn func(level, message string)) {
	accessLogSink = fn
}

// AccessLogger 记录每个 /api/* 请求的方法、路径、状态码与耗时，便于在实时日志页
// 观察第三方（机器人 / API 令牌）的调用行为。
// 跳过 /ws/logs 与静态资源，避免噪音；OPTIONS 预检也跳过。
func AccessLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/api") || c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()
		latency := time.Since(start)

		level := "INFO"
		status := c.Writer.Status()
		if status >= 500 {
			level = "ERROR"
		} else if status >= 400 {
			level = "WARN"
		}

		authType := c.GetString("authType")
		user := c.GetString("username")
		who := ""
		if user != "" {
			who = " user=" + user
		}
		if authType != "" {
			who += " auth=" + authType
		}

		msg := c.Request.Method + " " + path +
			" → " + itoa(status) + " (" + itoa(int(latency.Milliseconds())) + "ms)" +
			who + " ip=" + clientIP(c)

		// 走与业务日志相同的广播通道，实时日志页即可看到 API 调用。
		if accessLogSink != nil {
			accessLogSink(level, "[API] "+msg)
		}
	}
}

// clientIP 优先取反代传入的 X-Forwarded-For 首个地址，其次 X-Real-IP，最后 RemoteAddr。
func clientIP(c *gin.Context) string {
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	return c.ClientIP()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
