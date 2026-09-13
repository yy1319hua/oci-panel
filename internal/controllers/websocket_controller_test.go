package controllers

import "testing"

// TestLogLineHasLevel 覆盖历史日志级别过滤的两种格式与若干边界。
//
// 背景：历史缓冲里混有 SendLog 的文本行与 SendStructuredLog 的 JSON 行。
// 早期实现只按 "] " 前缀匹配，导致 JSON 行永远无法被级别过滤命中 ——
// 用户选「ERROR」时会发现一批错误日志凭空消失。这个测试锁定修复后的行为。
func TestLogLineHasLevel(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		level string
		want  bool
	}{
		// 标准文本格式
		{"文本行 INFO 命中", "[2026-09-13 10:00:00] INFO: 服务已启动", "INFO", true},
		{"文本行 ERROR 命中", "[2026-09-13 10:00:00] ERROR: OCI 调用失败", "ERROR", true},
		{"文本行级别不匹配", "[2026-09-13 10:00:00] INFO: 服务已启动", "ERROR", false},

		// 结构化 JSON 格式：必须能被过滤到
		{"JSON 行 INFO 命中", `{"time":"2026-09-13 10:00:00","level":"INFO","message":"ok"}`, "INFO", true},
		{"JSON 行 ERROR 命中", `{"time":"2026-09-13 10:00:00","level":"ERROR","message":"boom"}`, "ERROR", true},
		{"JSON 行级别不匹配", `{"time":"2026-09-13 10:00:00","level":"INFO","message":"ok"}`, "WARN", false},

		// 无时间戳前缀的裸行
		{"裸行命中", "WARN: 磁盘空间不足", "WARN", true},
		{"裸行前缀更长不应误判", "WARNING: 磁盘空间不足", "WARN", false},

		// 前缀相同但并非该级别（旧实现会误判为命中）
		{"ERRORX 不应算 ERROR", "[2026-09-13 10:00:00] ERRORX: 未知", "ERROR", false},

		// 内容里出现 "] " 不应把正文误当级别
		{"正文含方括号", "[2026-09-13 10:00:00] INFO: 返回 [a] b", "INFO", true},

		// 大小写不敏感
		{"小写级别命中", "[2026-09-13 10:00:00] info: 小写", "INFO", true},

		// 短行不应越界 panic
		{"空行", "", "INFO", false},
		{"极短行", "x", "INFO", false},
		{"只有左括号", "[", "INFO", false},
		{"级别名比行还长", "IN", "INFO", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := logLineHasLevel(tc.line, tc.level); got != tc.want {
				t.Fatalf("logLineHasLevel(%q, %q) = %v, want %v", tc.line, tc.level, got, tc.want)
			}
		})
	}
}
