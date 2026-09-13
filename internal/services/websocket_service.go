package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WebSocketService struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	tickets    map[string]webSocketTicket
	mu         sync.RWMutex
	history    []string // 环形缓冲：保存最近 N 条日志，供新连接回放历史
	historyMax int
}

const (
	maxWebSocketClients = 64
	webSocketTicketTTL  = time.Minute
	maxWebSocketTickets = 1024
	// webSocketWriteTimeout 限制单个客户端的写耗时。客户端不可达时写会阻塞，
	// 超时后该连接被回收，不会影响其他客户端与 HTTP 请求处理。
	webSocketWriteTimeout = 3 * time.Second
)

type webSocketTicket struct {
	username  string
	expiresAt time.Time
}

func NewWebSocketService() *WebSocketService {
	ws := &WebSocketService{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte, 256),
		tickets:    make(map[string]webSocketTicket),
		history:    make([]string, 0, 512),
		historyMax: 2000,
	}
	go ws.run()
	return ws
}

func (ws *WebSocketService) IssueTicket(username string) (string, error) {
	if username == "" {
		return "", fmt.Errorf("missing authenticated username")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	ticket := hex.EncodeToString(raw)
	now := time.Now()

	ws.mu.Lock()
	defer ws.mu.Unlock()
	for existing, value := range ws.tickets {
		if !now.Before(value.expiresAt) {
			delete(ws.tickets, existing)
		}
	}
	if len(ws.tickets) >= maxWebSocketTickets {
		return "", fmt.Errorf("too many pending websocket tickets")
	}
	ws.tickets[ticket] = webSocketTicket{username: username, expiresAt: now.Add(webSocketTicketTTL)}
	return ticket, nil
}

func (ws *WebSocketService) ConsumeTicket(ticket string) bool {
	if ticket == "" {
		return false
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()
	value, ok := ws.tickets[ticket]
	delete(ws.tickets, ticket)
	return ok && time.Now().Before(value.expiresAt)
}

func (ws *WebSocketService) run() {
	for message := range ws.broadcast {
		ws.fanOut(message)
	}
}

// fanOut 把一条消息发给所有客户端。
//
// 关键并发约束：网络写（WriteMessage）可能因客户端不可达而阻塞到写超时，
// 因此**绝不能持有 ws.mu 做网络 IO**。此前在写锁内直接 WriteMessage，
// 一旦有客户端卡住（手机切后台/断网，TCP 未断但不可写），写锁就会被占用
// 数秒；而 AccessLogger 对每个 HTTP 请求都会调 SendLog → appendHistory
// 抢同一把写锁，导致所有 API 请求排队直至前端超时。
//
// 现在的策略：锁内只做「快照客户端列表」，锁外并发写，失败的连接再单独加锁回收。
func (ws *WebSocketService) fanOut(message []byte) {
	// 历史缓冲只在这里维护：run() 是唯一消费者，天然串行，无需额外同步。
	ws.appendHistory(string(message))

	ws.mu.RLock()
	targets := make([]*websocket.Conn, 0, len(ws.clients))
	for client := range ws.clients {
		targets = append(targets, client)
	}
	ws.mu.RUnlock()

	if len(targets) == 0 {
		return
	}

	// 共享同一份 message 切片是只读的，并发 WriteMessage 可分发给不同连接。
	// 每个连接由独立的 goroutine 写，互不阻塞，单个慢客户端不会拖垮其他客户端。
	var wg sync.WaitGroup
	var failedMu sync.Mutex
	var failed []*websocket.Conn

	for _, client := range targets {
		wg.Add(1)
		go func(conn *websocket.Conn) {
			defer wg.Done()
			_ = conn.SetWriteDeadline(time.Now().Add(webSocketWriteTimeout))
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				failedMu.Lock()
				failed = append(failed, conn)
				failedMu.Unlock()
			}
		}(client)
	}
	wg.Wait()

	if len(failed) == 0 {
		return
	}
	ws.dropClients(failed)
}

// dropClients 关闭并移除写失败的连接（仅在真正持有失败连接时短暂持锁）。
func (ws *WebSocketService) dropClients(conns []*websocket.Conn) {
	ws.mu.Lock()
	for _, conn := range conns {
		if _, ok := ws.clients[conn]; ok {
			delete(ws.clients, conn)
			_ = conn.Close()
			log.Printf("WebSocket client dropped (write failed)")
		}
	}
	remaining := len(ws.clients)
	ws.mu.Unlock()
	log.Printf("Client disconnected. Total clients: %d", remaining)
}

func (ws *WebSocketService) RegisterClient(conn *websocket.Conn) bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if len(ws.clients) >= maxWebSocketClients {
		return false
	}
	ws.clients[conn] = true
	log.Printf("Client connected. Total clients: %d", len(ws.clients))
	return true
}

func (ws *WebSocketService) UnregisterClient(conn *websocket.Conn) {
	ws.mu.Lock()
	if _, ok := ws.clients[conn]; ok {
		delete(ws.clients, conn)
		conn.Close()
	}
	count := len(ws.clients)
	ws.mu.Unlock()
	log.Printf("Client disconnected. Total clients: %d", count)
}

func (ws *WebSocketService) BroadcastMessage(message []byte) {
	select {
	case ws.broadcast <- message:
	default:
		// Dropping a log message is preferable to blocking OCI/background work
		// when a burst of logs fills the bounded broadcast queue.
		log.Printf("WebSocket broadcast queue full; dropping log message")
	}
}

func (ws *WebSocketService) SendLog(level string, message string) {
	logMsg := fmt.Sprintf("[%s] %s: %s", time.Now().Format("2006-01-02 15:04:05"), level, message)
	// 只投递到有界 channel 后立即返回：不抢锁、不做任何网络 IO。
	// 这样 HTTP 请求路径（AccessLogger 每个请求都会调到这里）永远不会
	// 被 WebSocket 广播或某个慢客户端阻塞。
	ws.BroadcastMessage([]byte(logMsg))
}

// appendHistory 把一行日志写入历史环形缓冲（受写锁保护，供 GetHistory 回放）。
// 注意：只在广播 goroutine（run/fanOut）中调用，不处于任何请求处理路径上。
func (ws *WebSocketService) appendHistory(line string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.history = append(ws.history, line)
	if len(ws.history) > ws.historyMax {
		ws.history = ws.history[len(ws.history)-ws.historyMax:]
	}
}

// GetHistory 返回历史日志的快照副本（供新连接回放连接前的历史日志）。
func (ws *WebSocketService) GetHistory() []string {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	out := make([]string, len(ws.history))
	copy(out, ws.history)
	return out
}

func (ws *WebSocketService) SendInfo(message string) {
	ws.SendLog("INFO", message)
}

func (ws *WebSocketService) SendError(message string) {
	ws.SendLog("ERROR", message)
}

func (ws *WebSocketService) SendWarning(message string) {
	ws.SendLog("WARN", message)
}

func (ws *WebSocketService) SendDebug(message string) {
	ws.SendLog("DEBUG", message)
}

func (ws *WebSocketService) SendSuccess(message string) {
	ws.SendLog("SUCCESS", message)
}

type LogMessage struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

func (ws *WebSocketService) SendStructuredLog(logMsg LogMessage) {
	data, err := json.Marshal(logMsg)
	if err != nil {
		ws.SendError(fmt.Sprintf("failed to marshal log message: %v", err))
		return
	}
	ws.BroadcastMessage(data)
}
