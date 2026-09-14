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
		mu        sync.Mutex
		gotPaths  []string
		gotScopes []string
		lastBody  []map[string]string
		calls     int32
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		raw, _ := io.ReadAll(r.Body)

		var parsed struct {
			Commands []map[string]string `json:"commands"`
			Scope    map[string]any      `json:"scope"`
		}
		_ = json.Unmarshal(raw, &parsed)

		scopeName := "default"
		if parsed.Scope != nil {
			if s, _ := parsed.Scope["type"].(string); s != "" {
				scopeName = s
			}
		}

		mu.Lock()
		gotPaths = append(gotPaths, r.URL.Path)
		gotScopes = append(gotScopes, scopeName)
		lastBody = parsed.Commands
		mu.Unlock()

		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()

	// 带 chatID：应当覆盖 default + all_private_chats + chat 三个作用域。
	svc := &TelegramService{botToken: "123:ABC", apiBase: srv.URL, chatID: "12345"}

	// 调用两次，模拟「启动时注册 + 保存配置后再次注册」。
	svc.setMyCommands()
	svc.setMyCommands()

	// 3 个作用域 × 2 次调用 = 6 次请求（成功路径不应触发重试）。
	if n := atomic.LoadInt32(&calls); n != 6 {
		t.Fatalf("期望发出 6 次注册请求，实际 %d 次（重试不应在成功路径触发）", n)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, p := range gotPaths {
		if p != "/bot123:ABC/setMyCommands" {
			t.Fatalf("请求路径错误: %q", p)
		}
	}
	seen := map[string]bool{}
	for _, s := range gotScopes {
		seen[s] = true
	}
	// 这是本次修复的核心：必须覆盖到高优先级作用域，否则会被旧程序残留的命令遮蔽。
	for _, want := range []string{"default", "all_private_chats", "chat"} {
		if !seen[want] {
			t.Fatalf("命令未注册到作用域 %q（实际: %v）", want, gotScopes)
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
	for _, want := range []string{"start", "menu", "traffic", "cost", "instances", "alive", "configs", "version"} {
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

	// ok:false 必须对每个作用域各触发完整重试（无 chatID 时为 2 个作用域 × 3 次 = 6 次）。
	if n := atomic.LoadInt32(&calls); n != 6 {
		t.Fatalf("ok:false 应在每个作用域重试 3 次（共 6 次），实际 %d 次", n)
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

// TestSetMenuButtonUsesCommands 验证：菜单按钮固定设为 commands 类型（点击弹出命令列表），
// 且只发一次请求、body 正确。
func TestSetMenuButtonUsesCommands(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(raw, &parsed)
		mu.Lock()
		bodies = append(bodies, parsed)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()

	svc := &TelegramService{botToken: "123:ABC", apiBase: srv.URL}
	svc.setMenuButton()

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("菜单按钮只应发 1 次请求，实际 %d", len(bodies))
	}
	btn, _ := bodies[0]["menu_button"].(map[string]any)
	if btn["type"] != "commands" {
		t.Fatalf("期望 commands，实际 %v", btn["type"])
	}
}

// TestSetMenuButtonCoversChatScope 验证配置了 chatID 时，菜单按钮会同时应用到
// 全局默认与「该会话专属」两个作用域 —— 后者优先级更高，用于覆盖旧程序残留的按钮。
func TestSetMenuButtonCoversChatScope(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(raw, &parsed)
		mu.Lock()
		bodies = append(bodies, parsed)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()

	svc := &TelegramService{botToken: "123:ABC", apiBase: srv.URL, chatID: "12345"}
	svc.setMenuButton()

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("期望全局默认 + 会话专属共 2 次请求，实际 %d", len(bodies))
	}
	var hasGlobal, hasChat bool
	for _, b := range bodies {
		if _, ok := b["chat_id"]; ok {
			hasChat = true
		} else {
			hasGlobal = true
		}
	}
	if !hasGlobal || !hasChat {
		t.Fatalf("菜单按钮应同时覆盖全局默认与会话专属作用域（global=%v chat=%v）", hasGlobal, hasChat)
	}
}

// TestStartBotRegistersMenuOnlyOncePerStart 守住「菜单不再重复注册」这个修复。
//
// 背景：UpdateConfig 此前在调用 StartBot() 之后又无条件起一个 goroutine 补注册。
// 从「停止状态」保存配置时，StartBot 本身就会注册一次，补注册再来一次 ——
// 一次操作发两遍，日志里「Telegram 命令菜单已注册」成对出现，还徒增 Telegram 限频风险。
//
// 修复后由「调用前是否已在运行」决定：只有已运行（StartBot 提前返回、没注册）才补注册。
// 本测试锁定 StartBot 本身的语义：**首次启动注册一次；重复调用（已运行）不再注册**。
func TestStartBotRegistersMenuOnlyOncePerStart(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()

	svc := &TelegramService{botToken: "123:ABC", apiBase: srv.URL, chatID: "12345"}
	defer svc.StopBot()

	svc.StartBot()
	afterFirst := atomic.LoadInt32(&calls)
	if afterFirst == 0 {
		t.Fatal("StartBot 首次启动应当注册命令菜单与菜单按钮")
	}

	// 记录首次的请求数，再调一次 StartBot —— 已运行时它应当直接返回，不再注册。
	svc.StartBot()
	if got := atomic.LoadInt32(&calls); got != afterFirst {
		t.Fatalf("已在运行时 StartBot 不应再次注册：首次 %d 次，重复调用后 %d 次", afterFirst, got)
	}
	if !svc.IsRunning() {
		t.Fatal("StartBot 之后 IsRunning 应为 true")
	}
}
