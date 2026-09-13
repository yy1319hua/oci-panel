package services

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketTicketIsSingleUse(t *testing.T) {
	ws := &WebSocketService{tickets: make(map[string]webSocketTicket)}
	ticket, err := ws.IssueTicket("admin")
	if err != nil {
		t.Fatal(err)
	}
	if !ws.ConsumeTicket(ticket) {
		t.Fatal("fresh websocket ticket was rejected")
	}
	if ws.ConsumeTicket(ticket) {
		t.Fatal("websocket ticket was accepted more than once")
	}
}

func TestWebSocketTicketRequiresUser(t *testing.T) {
	ws := &WebSocketService{tickets: make(map[string]webSocketTicket)}
	if _, err := ws.IssueTicket(""); err == nil {
		t.Fatal("websocket ticket was issued without an authenticated user")
	}
}

// TestSendLogNeverBlocksOnStalledBroadcast 回归测试：广播消费端被卡住时，
// SendLog 也必须立即返回。此前 run() 在持有 ws.mu 写锁的情况下做网络写，
// 慢客户端会让写锁被占用数秒，而 AccessLogger 对每个 HTTP 请求都调 SendLog，
// 结果所有 API 请求一起排队直至前端超时。
//
// 这里不启动 run()，人为占满 broadcast 通道后测量 SendLog 的耗时：
// 必须远小于网络写超时（webSocketWriteTimeout），证明请求路径不再被广播阻塞。
func TestSendLogNeverBlocksOnStalledBroadcast(t *testing.T) {
	ws := &WebSocketService{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan []byte, 1), // 故意设成极小容量以模拟"消费端卡住"
		tickets:   make(map[string]webSocketTicket),
		history:   make([]string, 0, 8),
	}

	// 占满通道，且不启动消费者。
	ws.broadcast <- []byte("filler")

	start := time.Now()
	for i := 0; i < 200; i++ {
		ws.SendLog("INFO", "burst log line")
	}
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Fatalf("SendLog blocked on a stalled broadcast consumer: took %v", elapsed)
	}
}

// TestSendLogDoesNotTouchHistoryBuffer 确认 SendLog 不再在请求路径上抢锁写历史缓冲
// （历史缓冲改由广播 goroutine 维护）。
func TestSendLogDoesNotTouchHistoryBuffer(t *testing.T) {
	ws := &WebSocketService{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan []byte, 64),
		tickets:   make(map[string]webSocketTicket),
		history:   make([]string, 0, 8),
		historyMax: 8,
	}
	ws.SendLog("INFO", "line-1")
	if len(ws.GetHistory()) != 0 {
		t.Fatal("SendLog should not write history directly; run()/fanOut() owns it")
	}
}
