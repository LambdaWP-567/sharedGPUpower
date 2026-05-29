package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
	log     *zap.Logger
}

func NewHub(log *zap.Logger) *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]struct{}),
		log:     log,
	}
}

func (hub *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		hub.log.Error("ws upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	hub.mu.Lock()
	hub.clients[conn] = struct{}{}
	hub.mu.Unlock()
	defer func() {
		hub.mu.Lock()
		delete(hub.clients, conn)
		hub.mu.Unlock()
	}()

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// snapshot returns a copy of current clients under read lock.
func (hub *Hub) snapshot() []*websocket.Conn {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	conns := make([]*websocket.Conn, 0, len(hub.clients))
	for conn := range hub.clients {
		conns = append(conns, conn)
	}
	return conns
}

func (hub *Hub) remove(conn *websocket.Conn) {
	hub.mu.Lock()
	delete(hub.clients, conn)
	hub.mu.Unlock()
}

// Broadcast sends an event to all connected WebSocket clients.
// It copies the client list first so the lock is not held during writes.
func (hub *Hub) Broadcast(event string, data any) {
	msg, err := json.Marshal(map[string]any{"event": event, "data": data})
	if err != nil {
		return
	}
	for _, conn := range hub.snapshot() {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			hub.log.Debug("ws write failed, removing client", zap.Error(err))
			hub.remove(conn)
			conn.Close()
		}
	}
}

func (hub *Hub) StartPing(interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			for _, conn := range hub.snapshot() {
				conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					hub.remove(conn)
					conn.Close()
				}
			}
		}
	}()
}
