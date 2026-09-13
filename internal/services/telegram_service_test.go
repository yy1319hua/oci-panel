package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// newTestTelegramService 构造一个只配好 botToken/apiBase 的服务实例，
// 不走数据库（loadConfig 会查 DB），用于隔离测试 setMyCommands 的网络行为。
func newTestTelegramService(token, apiBase string) *TelegramService {
	return &TelegramService{
		botToken: token,
		apiBase:  apiBase,
	}
}

// TestSetMyCommandsRegistersAndRefreshes 验证两个关键行为：
//  1. 会把全部命令注册到 /bot<token>/setMyCommands；
//  2. 每次调用都会真正发出请求 —— 这是「更换 token / 改反代后菜单能刷新」的前提。
//
// 背景：Telegram 服务端缓存命令菜单，只有再次调用接口才会更新。
// 早期实现只在 StartBot 时注册一次，且在 bot 已在运行时 StartBot 会提前返回，
// 导致保存配置后菜单停留在旧版本。
func TestSetMyCommandsRegistersAndRefreshes(t *testing.T) {
	var (
		mu       sync.Mutex
		gotPaths []string
		lastBody []map[string]string
		calls    int32
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		raw, _ := io.ReadAll(r.Body)

		var parsed struct {
			Commands []map[string]string `json:"commands"`
		}
		_ = json.Unmarshal(raw, &parsed)

		mu.Lock()
		gotPaths = append(gotPaths, r.URL.Path)
		lastBody = parsed.Commands
		mu.Unlock()

		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()

	svc := newTestTelegramService("123:ABC", srv.URL)

	// 调用两次，模拟「启动时注册 + 保存配置后再次注册」。
	svc.setMyCommands()
	svc.setMyCommands()

	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("期望发出 2 次注册请求，实际 %d 次（重试不应在成功路径触发）", n)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, p := range gotPaths {
		if p != "/bot123:ABC/setMyCommands" {
			t.Fatalf("请求路径错误: %q", p)
		}
	}
	if len(lastBody) == 0 {
		t.Fatal("注册的命令列表为空")
	}
	// 确认菜单里包含用户可见的关键命令
	names := map[string]bool{}
	for _, c := range lastBody {
		names[c["command"]] = true
	}
	for _, want := range []string{"menu", "traffic", "cost", "instances", "alive", "configs", "version"} {
		if !names[want] {
			t.Fatalf("命令菜单缺少 %q（实际: %v）", want, names)
		}
	}
}

// TestSetMyCommandsRetriesOnFailure 验证首次失败会重试而非放弃。
//
// 这是修复的核心动机之一：一次网络抖动不应让命令菜单永久停留在旧版本。
func TestSetMyCommandsRetriesOnFailure(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			// 第一次模拟反代返回 502
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("bad gateway"))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()

	svc := newTestTelegramService("123:ABC", srv.URL)
	svc.setMyCommands()

	if n := atomic.LoadInt32(&calls); n < 2 {
		t.Fatalf("首次失败后应当重试，实际只调用了 %d 次", n)
	}
}

// TestSetMyCommandsTreatsOkFalseAsFailure 验证 HTTP 200 + {"ok":false} 被视为失败。
//
// Telegram 在「token 无效」等情况下依然返回 200，只看状态码会把失败当成功，
// 于是菜单静默不更新且日志显示一切正常 —— 这是最难排查的情形。
func TestSetMyCommandsTreatsOkFalseAsFailure(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Unauthorized"}`))
	}))
	defer srv.Close()

	svc := newTestTelegramService("123:ABC", srv.URL)
	svc.setMyCommands()

	// ok:false 必须触发完整重试（3 次）
	if n := atomic.LoadInt32(&calls); n != 3 {
		t.Fatalf("ok:false 应触发 3 次重试，实际 %d 次", n)
	}
}

// TestSetMyCommandsSkipsWhenNoToken 确认未配置 token 时不发请求（避免无意义报错）。
func TestSetMyCommandsSkipsWhenNoToken(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer srv.Close()

	svc := newTestTelegramService("", srv.URL)
	svc.setMyCommands()

	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("无 token 时不应发起请求，实际 %d 次", n)
	}
}

// TestBaseURLPrefersReverseProxy 确认配置了反代时走反代、并去掉尾部斜杠。
// 用户使用自建反代访问 Telegram，这里的行为直接决定请求能否到达。
func TestBaseURLPrefersReverseProxy(t *testing.T) {
	cases := []struct {
		apiBase string
		want    string
	}{
		{"https://tg.example.com/", "https://tg.example.com"},
		{"https://tg.example.com", "https://tg.example.com"},
		{"  ", DefaultTelegramAPIBase},
		{"", DefaultTelegramAPIBase},
	}
	for _, tc := range cases {
		svc := newTestTelegramService("t", tc.apiBase)
		if got := svc.baseURL(); got != tc.want {
			t.Fatalf("baseURL(%q) = %q, want %q", tc.apiBase, got, tc.want)
		}
	}
}

// TestSetMyCommandsUsesReverseProxyPath 确认注册请求确实打到反代域名上。
func TestSetMyCommandsUsesReverseProxyPath(t *testing.T) {
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	svc := newTestTelegramService("999:XYZ", srv.URL+"/")
	svc.setMyCommands()

	if !strings.HasPrefix(seenPath, "/bot999:XYZ/setMyCommands") {
		t.Fatalf("注册路径错误: %q", seenPath)
	}
}
