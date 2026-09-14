package services

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// LogStreamService 维护「实时日志广播」与「历史日志环形缓冲」。
//
// 【为什么从 WebSocket 换成 SSE 订阅者模型】
// 此前日志流走 WebSocket：服务端持有 *websocket.Conn，广播时向每条连接写数据。
// 这带来两个绕不开的问题：
//  1. gorilla/websocket 规定同一连接同一时刻只能有一个写者，广播（多 goroutine）
//     与建连回放会并发写同一条连接，触发 panic("concurrent write to websocket
//     connection") 直接崩掉整个面板进程 —— 表现为 /api/sys/wsTicket 被重置、前端 502；
//  2. WebSocket 需要 HTTP Upgrade 后维持双向长连接，经 Cloudflare 隧道（尤其跨境链路）
//     时非常脆弱，容易被中途掐断成 502。
//
// 改为 SSE（Server-Sent Events）后，每个日志页就是一条**普通 HTTP 长连接**：
// 由请求自己的 goroutine 往 http.ResponseWriter 写数据，天然不存在「多写者」问题；
// 且走标准 HTTP，Cloudflare 支持远比 WebSocket 稳。
//
// 服务端只需维护一组订阅者 channel：订阅者注册时拿到一个 chan string，
// 广播时把日志投递给每个订阅者；订阅者（HTTP 请求 goroutine）负责把收到的内容
// 写成 SSE 帧并 Flush。投递是非阻塞的 —— 单个慢客户端不会拖垮其他人。
type LogStreamService struct {
	mu      sync.RWMutex
	subs    map[*logSubscriber]struct{}
	history []string // 环形缓冲：保存最近 N 条日志，供新连接回放

	historyMax int
	subMax     int
}

// logSubscriber 是一次 SSE 订阅。ch 由服务端投递日志，消费端负责写出。
//
// ch 采用有界缓冲：客户端太慢（网络卡顿/SF 隧道抖动）时，缓冲写满即丢弃该条，
// 绝不阻塞广播路径 —— 日志是「尽力而为」的实时视图，丢几条远好过拖垮整个进程。
type logSubscriber struct {
	ch     chan string
	closed bool
}

const (
	maxLogSubscribers = 64
	logSubBufferSize  = 256
)

func NewLogStreamService() *LogStreamService {
	return &LogStreamService{
		subs:       make(map[*logSubscriber]struct{}),
		history:    make([]string, 0, 512),
		historyMax: 2000,
		subMax:     maxLogSubscribers,
	}
}

// Subscribe 注册一个新的 SSE 订阅者。
//
// 返回的 ch 仅供消费，调用方必须在退出时调用 Unsubscribe，否则订阅者会泄漏
// （订阅者集合持续增长 → 广播时遍历开销变大）。
func (s *LogStreamService) Subscribe() (*logSubscriber, bool) {
	s.mu.Lock()
	if len(s.subs) >= s.subMax {
		s.mu.Unlock()
		return nil, false
	}
	sub := &logSubscriber{ch: make(chan string, logSubBufferSize)}
	s.subs[sub] = struct{}{}
	total := len(s.subs)
	s.mu.Unlock()

	// 【必须在释放锁之后再写日志】logger 已把 log.Print* 接到广播回调，
	// 而广播回调会回头调用 BroadcastMessage 去拿同一把 s.mu。
	// 若在持锁状态下 log.Printf，就会重入这把不可重入的锁而永久死锁 ——
	// 且锁不释放会让此后所有 HTTP 请求（AccessLogger 每条都写日志）全部卡死。
	s.logOutsideLock("Log subscriber connected. Total subscribers: %d", total)
	return sub, true
}

// Unsubscribe 注销订阅者并关闭其 channel，让消费端 select 立刻退出。
//
// 必须加锁且置 closed 标记：置标记能保证「广播投递」与「注销关闭 channel」
// 不会竞争同一条 channel —— 否则可能出现向已关闭 channel 发送而 panic。
func (s *LogStreamService) Unsubscribe(sub *logSubscriber) {
	if sub == nil {
		return
	}
	s.mu.Lock()
	total := 0
	if _, ok := s.subs[sub]; ok {
		delete(s.subs, sub)
		if !sub.closed {
			sub.closed = true
			close(sub.ch)
		}
	}
	total = len(s.subs)
	s.mu.Unlock()

	// 同 Subscribe：日志必须在锁外写，否则会与广播回调争同一把锁而死锁。
	s.logOutsideLock("Log subscriber disconnected. Total subscribers: %d", total)
}

// logOutsideLock 是「绝不持锁写日志」这一约束的统一出口。
//
// 背景：logger.SetBroadcaster 会把所有 log.Print* 转发到 BroadcastMessage，
// 而 BroadcastMessage 内部要拿 s.mu。因此 LogStreamService 中任何持锁路径
// 都不能调用 log.Printf —— 必须收敛到这里，在锁外统一输出。
// 这个方法强制调用方先在锁内算好参数、释放锁后再传入，避免再次踩坑。
func (s *LogStreamService) logOutsideLock(format string, args ...any) {
	log.Printf(format, args...)
}

// Messages 暴露订阅者的接收 channel 供消费端 select。
func (sub *logSubscriber) Messages() <-chan string {
	return sub.ch
}

// BroadcastMessage 把一条日志投递给所有订阅者，并写入历史缓冲。
//
// 全程**不阻塞**：向有界 channel 做非阻塞发送，满了就丢该条。
// 这样后台任务/HTTP 请求路径绝不会被某个卡住的客户端拖住。
//
// 【为什么投递也要在锁内】Unsubscribe 会在持锁状态下 close(ch)。若这里先在锁内
// 快照订阅者、释放锁后再发送，就存在「快照后、发送前该订阅者被注销并关闭 channel」
// 的窗口，导致 send on closed channel panic。改为全程持锁做非阻塞发送即可根除：
// 发送本身是 O(1) 且不阻塞，持锁代价可以忽略。
func (s *LogStreamService) BroadcastMessage(message []byte) {
	line := string(message)

	s.mu.Lock()
	defer s.mu.Unlock()

	// 历史缓冲与订阅者共用同一把锁，天然保证「先入历史、再广播」的顺序。
	s.appendHistoryLocked(line)

	for sub := range s.subs {
		if sub.closed {
			continue
		}
		select {
		case sub.ch <- line:
		default:
			// 客户端消费太慢，丢弃该条而非阻塞广播路径。
		}
	}
}

// appendHistoryLocked 把一行日志写入历史环形缓冲。
// 调用方必须已持有 s.mu（BroadcastMessage 与 GetHistory 共用同一把锁）。
func (s *LogStreamService) appendHistoryLocked(line string) {
	s.history = append(s.history, line)
	if len(s.history) > s.historyMax {
		s.history = s.history[len(s.history)-s.historyMax:]
	}
}

// GetHistory 返回历史日志的快照副本（供新订阅者回放连接前的历史日志）。
func (s *LogStreamService) GetHistory() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.history))
	copy(out, s.history)
	return out
}

func (s *LogStreamService) SendLog(level string, message string) {
	logMsg := fmt.Sprintf("[%s] %s: %s", time.Now().Format("2006-01-02 15:04:05"), level, message)
	// 只投递到订阅者 channel，不做任何网络 IO。
	// 这样 HTTP 请求路径（AccessLogger 每个请求都会调到这里）永远不会被阻塞。
	s.BroadcastMessage([]byte(logMsg))
}

func (s *LogStreamService) SendInfo(message string)    { s.SendLog("INFO", message) }
func (s *LogStreamService) SendError(message string)   { s.SendLog("ERROR", message) }
func (s *LogStreamService) SendWarning(message string) { s.SendLog("WARN", message) }
func (s *LogStreamService) SendDebug(message string)   { s.SendLog("DEBUG", message) }
func (s *LogStreamService) SendSuccess(message string) { s.SendLog("SUCCESS", message) }

type LogMessage struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

func (s *LogStreamService) SendStructuredLog(logMsg LogMessage) {
	data, err := json.Marshal(logMsg)
	if err != nil {
		s.SendError(fmt.Sprintf("failed to marshal log message: %v", err))
		return
	}
	s.BroadcastMessage(data)
}
