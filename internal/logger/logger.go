package logger

import (
	"context"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
)

// levelVar 是全局可变的日志级别。UI 修改日志级别时调用 SetLevel 即时生效，
// 无需重启进程；同时会写回 config.toml 持久化，重启后仍保留。
var levelVar = &slog.LevelVar{}

// broadcaster 是可选的日志广播回调（由 main 注册为 WebSocket 推送）。
// 用 atomic.Value 保存，保证运行时可并发替换。
var broadcaster atomic.Value // func(level, message string)

// broadcasting 防止「广播函数内部又写日志 → 再次触发广播」形成的死循环。
var broadcasting atomic.Bool

// multiHandler 把日志同时写到 stdout 和可选的广播回调（实时日志页面）。
type multiHandler struct {
	console slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.console.Enabled(ctx, l)
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	// 防循环：广播回调内部产生的日志（如 WebSocket 写失败提示）不再回灌广播。
	if !broadcasting.Load() {
		if fn, ok := broadcaster.Load().(func(level, message string)); ok && fn != nil {
			broadcasting.Store(true)
			fn(levelName(r.Level), r.Message)
			broadcasting.Store(false)
		}
	}
	return h.console.Handle(ctx, r)
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &multiHandler{console: h.console.WithAttrs(attrs)}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	return &multiHandler{console: h.console.WithGroup(name)}
}

func levelName(l slog.Level) string {
	switch {
	case l <= slog.LevelDebug:
		return "DEBUG"
	case l < slog.LevelWarn:
		return "INFO"
	case l < slog.LevelError:
		return "WARN"
	default:
		return "ERROR"
	}
}

// Setup 用 config.toml 中的初始级别初始化全局 slog 日志器，并把标准库 log 也接到
// 同一个 handler 上（标准库 log.Print* 统一视为 Info 级，从而同样受级别开关控制）。
// 这样无需改动任何业务代码里的 log.Print* 调用即可获得分级过滤能力。
func Setup(level string) {
	if strings.TrimSpace(level) == "" {
		level = "info"
	}
	_ = levelVar.UnmarshalText([]byte(level))

	console := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: levelVar})
	handler := &multiHandler{console: console}
	slog.SetDefault(slog.New(handler))

	// 让标准库 log 的输出也经过 slog handler（Info 级），受 levelVar 过滤。
	// NewLogLogger 会把 log.Print* 以指定级别写入 handler，从而被 levelVar 统一过滤。
	log.SetOutput(slog.NewLogLogger(handler, slog.LevelInfo).Writer())
	log.SetFlags(0)
}

// SetBroadcaster 注册日志广播回调（通常是 WebSocketService.SendLog 的包装）。
// 注册后，所有 log.Print* / slog 输出都会同时推送到实时日志页面。传 nil 取消广播。
func SetBroadcaster(fn func(level, message string)) {
	if fn == nil {
		broadcaster.Store((func(level, message string))(nil))
		return
	}
	broadcaster.Store(fn)
}

// SetLevel 运行时调整日志级别。level 须为 debug/info/warn/error 之一。
func SetLevel(level string) error {
	if strings.TrimSpace(level) == "" {
		level = "info"
	}
	return levelVar.UnmarshalText([]byte(level))
}
