package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte, 64),
		tickets:    make(map[string]webSocketTicket),
		history:    make([]string, 0, 8),
		historyMax: 8,
	}
	ws.SendLog("INFO", "line-1")
	if len(ws.GetHistory()) != 0 {
		t.Fatal("SendLog should not write history directly; run()/fanOut() owns it")
	}
}

// TestConcurrentWritesAreSerialized 回归测试：广播（fanOut 的多 goroutine）与
// 建连回放（WriteHistory）会并发向同一条连接写日志。gorilla/websocket 规定同一连接
// 同一时刻只能有一个写者，此前两者并发写会 panic("concurrent write to websocket
// connection")，把整个面板进程打崩 —— 表现就是 /api/sys/wsTicket 被重置、前端 502。
//
// 复现要点：gorilla 的并发写检测依赖 isWriting 标志在「一次完整写」期间保持为 true。
// 若对端读取极快，临界区只有纳秒级，几乎撞不上；因此这里让客户端**故意不读取**，
// 服务端写缓冲区很快填满，写操作阻塞在 flush 上，把写窗口从纳秒拉长到毫秒级，
// 未加锁的并发写必然撞车崩溃。
//
// 若并发写未被串行化，本测试进程会因 panic 直接崩溃（表现为整个测试二进制挂掉，
// 而不是某个用例 FAIL）；能正常跑完即证明写操作已按连接安全序列化。
func TestConcurrentWritesAreSerialized(t *testing.T) {
	serverConnCh := make(chan *websocket.Conn, 1)
	releaseCh := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConnCh <- conn
		// 关键：服务端拿到连接后既不发也不读，仅保持存活，
		// 直到测试结束再关闭，确保写缓冲区始终是满的。
		<-releaseCh
		_ = conn.Close()
	}))
	defer srv.Close()
	defer close(releaseCh)

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer client.Close()
	// 注意：这里刻意不给客户端启动读取 goroutine —— 让服务端写缓冲区填满，
	// 从而把每次写的临界区拉长，稳定暴露并发写问题。

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(3 * time.Second):
		t.Fatal("server-side websocket connection was not established")
	}

	ws := NewWebSocketService()
	ws.mu.Lock()
	ws.clients[serverConn] = true
	ws.mu.Unlock()
	defer func() {
		ws.mu.Lock()
		delete(ws.clients, serverConn)
		ws.mu.Unlock()
	}()

	// 并发写：多个广播写协程 + 一个回放写协程，全部指向同一条服务端连接。
	// 写操作会因对端不读而阻塞，临界区被显著拉长。
	// 用大消息把对端接收缓冲迅速填满，迫使每次写在 flush 阶段阻塞，
	// 从而把 isWriting 窗口从纳秒级拉长到毫秒级。
	bigPayload := make([]byte, 1<<20) // 1 MiB
	bigLine := string(bigPayload)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				ws.fanOut(bigPayload)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 20; j++ {
			ws.WriteHistory(serverConn, []string{bigLine})
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 正常结束：写操作已被按连接串行化。
		// 若未串行化，前面任一 goroutine 早就会 panic 崩掉整个测试进程。
	case <-time.After(15 * time.Second):
		// 写入因对端不读、写缓冲区满而长时间阻塞，属预期；
		// 只要没发生 panic 就说明并发写已被消除。
	}
}
