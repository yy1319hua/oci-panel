package controllers

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
)

// TestLogStreamSSEDeliversHistoryAndLive 端到端验证 SSE 日志流：
//  1. 建连后立刻收到 ": open" 握手帧（保证响应头即时下发，前端不白屏）；
//  2. 能收到建连前写入的历史日志（回放）；
//  3. 建连后新产生的日志能实时推送过来。
//
// 这是把日志流从 WebSocket 迁到 SSE 之后最核心的行为契约：
// 若这条测试挂了，说明日志页拿不到数据 —— 也就是用户眼里那个「502 / 空白」问题。
func TestLogStreamSSEDeliversHistoryAndLive(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := services.NewLogStreamService()
	// 建连前先写一条历史日志，验证回放。
	svc.SendLog("INFO", "history-line-before-connect")

	ctrl := NewLogStreamController(svc, nil)
	r := gin.New()
	r.GET("/api/sys/logs/stream", ctrl.StreamLogs)

	srv := httptest.NewServer(r)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/sys/logs/stream", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect SSE: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// 建连后稍等，让订阅注册完成，再推一条实时日志。
	time.Sleep(150 * time.Millisecond)
	svc.SendLog("ERROR", "live-line-after-connect")

	reader := bufio.NewReader(resp.Body)
	var (
		sawOpen    bool
		sawHistory bool
		sawLive    bool
	)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !(sawHistory && sawLive) {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case trimmed == ": open":
			sawOpen = true
		case strings.Contains(trimmed, "history-line-before-connect"):
			sawHistory = true
		case strings.Contains(trimmed, "live-line-after-connect"):
			sawLive = true
		}
	}

	if !sawOpen {
		t.Fatal("未收到 ': open' 握手帧（响应头无法即时下发，前端会白屏）")
	}
	if !sawHistory {
		t.Fatal("未收到建连前的历史日志回放")
	}
	if !sawLive {
		t.Fatal("未收到建连后的实时日志推送")
	}
}

// TestWriteSSEDataSplitsMultilineContent 验证多行日志被正确拆成多条 data 行。
//
// SSE 的换行是帧分隔符：若把含 \n 的内容整段写进一条 data 行，客户端会把
// 后半段当成「没有字段名的行」而静默丢弃 —— 表现为日志页少了半截内容。
func TestWriteSSEDataSplitsMultilineContent(t *testing.T) {
	var sb strings.Builder
	writeSSEData(&sb, "line1\nline2\r\nline3")
	got := sb.String()

	want := "data: line1\ndata: line2\ndata: line3\n\n"
	if got != want {
		t.Fatalf("writeSSEData output = %q, want %q", got, want)
	}
}
