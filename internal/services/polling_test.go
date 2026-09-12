package services

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestWaitForStateReadyAfterRetries 验证 C2：轮询若干次后就绪即返回该值。
func TestWaitForStateReadyAfterRetries(t *testing.T) {
	calls := 0
	v, ok := waitForState(context.Background(), 10, 0, func() (int, error) {
		calls++
		return calls, nil
	}, func(n int) bool { return n >= 3 })
	if !ok {
		t.Fatal("expected ready")
	}
	if v != 3 {
		t.Errorf("v = %d, want 3", v)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

// TestWaitForStateTimeout 验证 C2：用尽 maxAttempts 仍未就绪则返回 false，且恰好调用 maxAttempts 次。
func TestWaitForStateTimeout(t *testing.T) {
	calls := 0
	_, ok := waitForState(context.Background(), 5, 0, func() (int, error) {
		calls++
		return calls, nil
	}, func(int) bool { return false })
	if ok {
		t.Fatal("expected not ready")
	}
	if calls != 5 {
		t.Errorf("calls = %d, want 5", calls)
	}
}

// TestWaitForStateIgnoresErrors 验证 C2：getState 出错时视为「尚未就绪」继续重试（与原循环 if err==nil 一致）。
func TestWaitForStateIgnoresErrors(t *testing.T) {
	calls := 0
	v, ok := waitForState(context.Background(), 10, 0, func() (int, error) {
		calls++
		if calls < 3 {
			return 0, errors.New("transient")
		}
		return calls, nil
	}, func(n int) bool { return n >= 3 })
	if !ok {
		t.Fatal("expected ready despite early errors")
	}
	if v != 3 {
		t.Errorf("v = %d, want 3", v)
	}
}

// TestWaitForStateImmediate 验证 C2：首次即就绪则只调用一次。
func TestWaitForStateImmediate(t *testing.T) {
	calls := 0
	_, ok := waitForState(context.Background(), 10, 0, func() (string, error) {
		calls++
		return "AVAILABLE", nil
	}, func(s string) bool { return s == "AVAILABLE" })
	if !ok {
		t.Fatal("expected ready")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestWaitForStateCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, ok := waitForState(ctx, 30, time.Hour, func() (int, error) {
			close(called)
			return 1, nil
		}, func(int) bool { return false })
		if ok {
			t.Error("canceled polling returned ready")
		}
	}()
	<-called
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt polling delay")
	}
}

func TestWaitForStateDoesNotWaitAfterFinalAttempt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	waitForState(ctx, 1, time.Hour, func() (int, error) { return 0, errors.New("unavailable") }, func(int) bool { return false })
	if ctx.Err() != nil {
		t.Fatal("waited after exhausting attempts")
	}
}
