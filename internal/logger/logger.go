package logger

import (
	"log"
	"log/slog"
	"os"
	"strings"
)

// levelVar 是全局可变的日志级别。UI 修改日志级别时调用 SetLevel 即时生效，
// 无需重启进程；同时会写回 config.toml 持久化，重启后仍保留。
var levelVar = &slog.LevelVar{}

// Setup 用 config.toml 中的初始级别初始化全局 slog 日志器，并把标准库 log 也接到
// 同一个 handler 上（标准库 log.Print* 统一视为 Info 级，从而同样受级别开关控制）。
// 这样无需改动任何业务代码里的 log.Print* 调用即可获得分级过滤能力。
func Setup(level string) {
	if strings.TrimSpace(level) == "" {
		level = "info"
	}
	_ = levelVar.UnmarshalText([]byte(level))

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: levelVar})
	slog.SetDefault(slog.New(handler))

	// 让标准库 log 的输出也经过 slog handler（Info 级），受 levelVar 过滤。
	// NewLogLogger 会把 log.Print* 以指定级别写入 handler，从而被 levelVar 统一过滤。
	log.SetOutput(slog.NewLogLogger(handler, slog.LevelInfo).Writer())
	log.SetFlags(0)
}

// SetLevel 运行时调整日志级别。level 须为 debug/info/warn/error 之一。
func SetLevel(level string) error {
	if strings.TrimSpace(level) == "" {
		level = "info"
	}
	return levelVar.UnmarshalText([]byte(level))
}
