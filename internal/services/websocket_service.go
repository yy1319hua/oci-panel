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
	for {
		select {
		case message := <-ws.broadcast:
			// 广播分支会在写入失败时从 clients 中删除连接，属于写操作，
			// 必须持有写锁（此前用 RLock 下 delete map 属错误锁纪律）。
			ws.mu.Lock()
			for client := range ws.clients {
				_ = client.SetWriteDeadline(time.Now().Add(5 * time.Second))
				err := client.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					log.Printf("Error writing to client: %v", err)
					client.Close()
					delete(ws.clients, client)
				}
			}
			ws.mu.Unlock()
		}
	}
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
	ws.appendHistory(logMsg)
	ws.BroadcastMessage([]byte(logMsg))
}

// appendHistory 把一行日志写入历史环形缓冲（受写锁保护，供 GetHistory 回放）。
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
