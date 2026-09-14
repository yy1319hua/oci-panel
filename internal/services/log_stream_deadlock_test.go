package services

import (
	"testing"
	"time"

	"github.com/adiecho/oci-panel/internal/logger"
)

// TestLogStreamSubscribeDoesNotDeadlockWithBroadcaster 是一处真实线上死锁的回归测试。
//
// 【背景】main.go 会执行 logger.SetBroadcaster(...)，把所有 log.Print* 的输出
// 转发进 LogStreamService.SendLog → BroadcastMessage，而 BroadcastMessage 内部
// 要获取 s.mu 写锁。
//
// 【曾经的 bug】Subscribe/Unsubscribe 在**持有 s.mu 的情况下**调用 log.Printf，
// 于是 log → broadcaster → SendLog → BroadcastMessage → s.mu.Lock() 重入同一把
// 不可重入的锁，永久死锁；且锁不释放会让此后所有 HTTP 请求（AccessLogger 每个
// /api 请求都会写一条日志）全部阻塞，表现就是「整个面板请求超时」。
//
// 这个测试必须在**接上 broadcaster 之后**再订阅，才能覆盖到这条重入路径 ——
// 不接 broadcaster 的普通单测永远测不出来。
func TestLogStreamSubscribeDoesNotDeadlockWithBroadcaster(t *testing.T) {
	svc := NewLogStreamService()

	// 复刻运行时接线。两步缺一不可：
	//  1. logger.Setup 把标准库 log 的输出接到 slog handler；
	//  2. SetBroadcaster 让 handler 的输出再转发进日志广播。
	// 少了任何一步，log.Printf 都不会触发广播，也就复现不出这条死锁链路。
	logger.Setup("info")
	logger.SetBroadcaster(func(level, message string) {
		svc.SendLog(level, message)
	})
	t.Cleanup(func() { logger.SetBroadcaster(nil) })

	done := make(chan struct{})
	go func() {
		defer close(done)
		sub, ok := svc.Subscribe()
		if !ok {
			t.Error("订阅应成功")
			return
		}
		svc.Unsubscribe(sub)
	}()

	select {
	case <-done:
		// 正常返回，未死锁。
	case <-time.After(3 * time.Second):
		t.Fatal("Subscribe/Unsubscribe 在接上 broadcaster 后死锁：持锁路径里调用了 log.Print*")
	}
}

// TestLogStreamSubscribeThenBroadcastStillWorks 验证把日志挪到锁外后，
// 订阅与广播的核心功能没有被破坏。
func TestLogStreamSubscribeThenBroadcastStillWorks(t *testing.T) {
	svc := NewLogStreamService()
	logger.Setup("info")
	logger.SetBroadcaster(func(level, message string) {
		svc.SendLog(level, message)
	})
	t.Cleanup(func() { logger.SetBroadcaster(nil) })

	sub, ok := svc.Subscribe()
	if !ok {
		t.Fatal("订阅应成功")
	}
	defer svc.Unsubscribe(sub)

	// 订阅动作本身会产生日志（会进入缓冲 channel），因此这里持续读取，
	// 直到取到我们手动广播的那条，而不是断言「第一条就是它」。
	svc.BroadcastMessage([]byte("hello"))

	deadline := time.After(2 * time.Second)
	for {
		select {
		case got := <-sub.Messages():
			if got == "hello" {
				return
			}
		case <-deadline:
			t.Fatal("广播后未在超时内收到 hello")
		}
	}
}

// TestLogStreamConcurrentSubscribeUnsubscribe 并发建连/断开，验证不再死锁且无竞态。
func TestLogStreamConcurrentSubscribeUnsubscribe(t *testing.T) {
	svc := NewLogStreamService()
	logger.Setup("info")
	logger.SetBroadcaster(func(level, message string) {
		svc.SendLog(level, message)
	})
	t.Cleanup(func() { logger.SetBroadcaster(nil) })

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			sub, ok := svc.Subscribe()
			if !ok {
				continue
			}
			svc.BroadcastMessage([]byte("tick"))
			svc.Unsubscribe(sub)
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("并发订阅/注销死锁")
	}
}
