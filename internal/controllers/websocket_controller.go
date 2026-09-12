package controllers

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type WebSocketController struct {
	wsService      *services.WebSocketService
	allowedOrigins []string
	upgrader       websocket.Upgrader
}

func (wc *WebSocketController) IssueTicket(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Unauthorized"))
		return
	}
	usernameString, ok := username.(string)
	if !ok || usernameString == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Unauthorized"))
		return
	}
	ticket, err := wc.wsService.IssueTicket(usernameString)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse(503, "Unable to create websocket ticket"))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{"ticket": ticket}, "success"))
}

func NewWebSocketController(wsService *services.WebSocketService, allowedOrigins []string) *WebSocketController {
	wc := &WebSocketController{
		wsService:      wsService,
		allowedOrigins: allowedOrigins,
	}
	wc.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     wc.checkOrigin,
	}
	return wc
}

// checkOrigin 仅允许同源（Origin 的 host 与请求 Host 一致）或显式白名单内的来源建立连接，
// 替换此前无条件返回 true 的实现——否则任意网站都能跨站发起 WebSocket 连接（CSWSH 风险）。
func (wc *WebSocketController) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// 非浏览器客户端（如本机脚本）通常没有 Origin 头；放行，鉴权仍由一次性 ticket 保证。
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	for _, a := range wc.allowedOrigins {
		if a == "*" || strings.EqualFold(strings.TrimSpace(a), origin) {
			return true
		}
	}
	return false
}

func (wc *WebSocketController) HandleWebSocket(c *gin.Context) {
	// 升级前先消费一次性短时 ticket。浏览器无法为 WebSocket 设置自定义请求头，
	// 由已认证的 API 请求先取得 ticket，避免把长期 JWT 放进 URL 和反代日志。
	ticket := c.Query("ticket")
	if ticket == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Unauthorized"))
		return
	}
	if !wc.wsService.ConsumeTicket(ticket) {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Invalid websocket ticket"))
		return
	}

	conn, err := wc.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	if !wc.wsService.RegisterClient(conn) {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "too many connections"), time.Now().Add(time.Second))
		_ = conn.Close()
		return
	}
	defer wc.wsService.UnregisterClient(conn)

	const (
		maxLogMessageSize = 4 << 10
		pongWait          = 90 * time.Second
		pingPeriod        = 30 * time.Second
	)
	conn.SetReadLimit(maxLogMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	wc.wsService.SendInfo("Connected to log stream")

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}
