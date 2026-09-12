package services

import "testing"

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
