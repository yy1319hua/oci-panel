package services

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLogStreamSubscribeAndBroadcast 验证订阅者能收到广播，且未订阅时不 panic。
func TestLogStreamSubscribeAndBroadcast(t *testing.T) {
	svc := NewLogStreamService()

	// 无订阅者时广播不应 panic。
	svc.SendLog("INFO", "no subscriber yet")

	sub, ok := svc.Subscribe()
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer svc.Unsubscribe(sub)

	svc.SendLog("INFO", "hello subscriber")

	select {
	case line := <-sub.Messages():
		if !strings.Contains(line, "hello subscriber") {
			t.Fatalf("unexpected log line: %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not receive broadcast")
	}
}

// TestLogStreamUnsubscribeClosesChannel 验证注销后 channel 被关闭，
// 消费端可以通过 ok=false 立即感知并退出 —— 若不关闭，SSE 请求 goroutine
// 会一直阻塞在 select 上直到客户端断开，造成 goroutine 泄漏。
func TestLogStreamUnsubscribeClosesChannel(t *testing.T) {
	svc := NewLogStreamService()
	sub, ok := svc.Subscribe()
	if !ok {
		t.Fatal("subscribe failed")
	}
	svc.Unsubscribe(sub)

	select {
	case _, open := <-sub.Messages():
		if open {
			t.Fatal("channel should be closed after unsubscribe")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel was not closed after unsubscribe")
	}

	// 重复注销必须安全（SSE handler 里 defer Unsubscribe 与显式注销可能并存）。
	svc.Unsubscribe(sub)
	svc.Unsubscribe(nil)
}

// TestLogStreamBroadcastAfterUnsubscribeDoesNotPanic 是关键的并发回归测试：
// 广播与注销并发时，绝不允许出现 "send on closed channel" panic。
//
// 此前的 WebSocket 实现会因为并发写同一条连接而 panic；重写为订阅者模型后，
// 这里要守住的新约束是：BroadcastMessage 持锁投递、Unsubscribe 持锁关闭 channel，
// 两者互斥，因此不会向已关闭的 channel 发送。
func TestLogStreamBroadcastAfterUnsubscribeDoesNotPanic(t *testing.T) {
	svc := NewLogStreamService()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 持续的广播压力。
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					svc.SendLog("INFO", "hammer")
				}
			}
		}()
	}

	// 反复订阅/注销，制造「广播与关闭 channel」的高频竞争。
	for i := 0; i < 500; i++ {
		sub, ok := svc.Subscribe()
		if !ok {
			continue
		}
		svc.Unsubscribe(sub)
	}

	close(stop)
	wg.Wait()
	// 能正常跑到这里，说明没有发生 send on closed channel panic。
}

// TestLogStreamSlowSubscriberDoesNotBlock 验证慢订阅者不会阻塞广播：
// 缓冲写满后新日志被丢弃，而不是卡住 SendLog —— 否则一个卡住的浏览器
// （手机切后台/隧道抖动）就会拖垮所有 HTTP 请求路径（AccessLogger 每个请求都调 SendLog）。
func TestLogStreamSlowSubscriberDoesNotBlock(t *testing.T) {
	svc := NewLogStreamService()
	sub, ok := svc.Subscribe()
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer svc.Unsubscribe(sub)

	// 故意不消费，写远超缓冲容量的日志。
	start := time.Now()
	for i := 0; i < logSubBufferSize*4; i++ {
		svc.SendLog("INFO", "flood")
	}
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Fatalf("SendLog blocked on a slow subscriber: took %v", elapsed)
	}
}

// TestLogStreamHistoryReplay 验证历史缓冲按「旧 → 新」保留、且受容量上限约束。
func TestLogStreamHistoryReplay(t *testing.T) {
	svc := NewLogStreamService()
	svc.historyMax = 3 // 收窄上限便于断言

	for _, msg := range []string{"a", "b", "c", "d"} {
		svc.BroadcastMessage([]byte(msg))
	}

	history := svc.GetHistory()
	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3", len(history))
	}
	if history[0] != "b" || history[2] != "d" {
		t.Fatalf("history = %v, want [b c d]", history)
	}
}

// TestLogStreamSubscribeLimit 验证超过订阅上限时拒绝新订阅（返回 ok=false），
// 避免恶意/异常客户端耗尽服务端资源。
func TestLogStreamSubscribeLimit(t *testing.T) {
	svc := NewLogStreamService()
	svc.subMax = 2

	s1, ok1 := svc.Subscribe()
	if !ok1 {
		t.Fatal("first subscribe should succeed")
	}
	defer svc.Unsubscribe(s1)

	s2, ok2 := svc.Subscribe()
	if !ok2 {
		t.Fatal("second subscribe should succeed")
	}
	defer svc.Unsubscribe(s2)

	if _, ok := svc.Subscribe(); ok {
		t.Fatal("subscribe beyond limit should be rejected")
	}
}
